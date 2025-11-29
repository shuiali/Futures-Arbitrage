// Package coinex provides WebSocket user data client for CoinEx Futures exchange.
package coinex

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// WSUserDataClient handles private WebSocket connections for user data
type WSUserDataClient struct {
	url           string
	apiKey        string
	apiSecret     string
	conn          *websocket.Conn
	connected     atomic.Bool
	authenticated atomic.Bool
	reqID         atomic.Int64
	mu            sync.RWMutex
	done          chan struct{}

	// Callbacks
	onOrder         func(*WSOrderUpdate)
	onPosition      func(*WSPositionUpdate)
	onBalance       func(*WSBalanceUpdate)
	onError         func(error)
	onConnected     func()
	onAuthenticated func()

	// Subscription tracking
	orderSubs    map[string]bool
	positionSubs map[string]bool
	balanceSubs  bool
}

// WSUserDataConfig holds configuration for user data WebSocket client
type WSUserDataConfig struct {
	URL       string
	APIKey    string
	APISecret string
}

// NewWSUserDataClient creates a new user data WebSocket client
func NewWSUserDataClient(cfg WSUserDataConfig) *WSUserDataClient {
	if cfg.URL == "" {
		cfg.URL = WSFuturesURL
	}

	return &WSUserDataClient{
		url:          cfg.URL,
		apiKey:       cfg.APIKey,
		apiSecret:    cfg.APISecret,
		done:         make(chan struct{}),
		orderSubs:    make(map[string]bool),
		positionSubs: make(map[string]bool),
	}
}

// SetOrderHandler sets the callback for order updates
func (c *WSUserDataClient) SetOrderHandler(handler func(*WSOrderUpdate)) {
	c.onOrder = handler
}

// SetPositionHandler sets the callback for position updates
func (c *WSUserDataClient) SetPositionHandler(handler func(*WSPositionUpdate)) {
	c.onPosition = handler
}

// SetBalanceHandler sets the callback for balance updates
func (c *WSUserDataClient) SetBalanceHandler(handler func(*WSBalanceUpdate)) {
	c.onBalance = handler
}

// SetErrorHandler sets the callback for errors
func (c *WSUserDataClient) SetErrorHandler(handler func(error)) {
	c.onError = handler
}

// SetConnectedHandler sets the callback for connection established
func (c *WSUserDataClient) SetConnectedHandler(handler func()) {
	c.onConnected = handler
}

// SetAuthenticatedHandler sets the callback for successful authentication
func (c *WSUserDataClient) SetAuthenticatedHandler(handler func()) {
	c.onAuthenticated = handler
}

// Connect establishes WebSocket connection and authenticates
func (c *WSUserDataClient) Connect(ctx context.Context) error {
	log.Info().Str("url", c.url).Msg("Connecting to CoinEx User Data WebSocket")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, c.url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	c.connected.Store(true)

	log.Info().Msg("Connected to CoinEx User Data WebSocket")

	if c.onConnected != nil {
		c.onConnected()
	}

	// Authenticate
	if err := c.authenticate(); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Start goroutines
	go c.readLoop(ctx)
	go c.pingLoop(ctx)

	return nil
}

// authenticate sends authentication request
func (c *WSUserDataClient) authenticate() error {
	timestamp := time.Now().UnixMilli()

	// Create signature: HMAC-SHA256(secret_key, timestamp)
	mac := hmac.New(sha256.New, []byte(c.apiSecret))
	mac.Write([]byte(fmt.Sprintf("%d", timestamp)))
	signature := hex.EncodeToString(mac.Sum(nil))

	req := WSRequest{
		Method: WSMethodServerSign,
		Params: WSAuthParams{
			AccessID:  c.apiKey,
			SignedStr: signature,
			Timestamp: timestamp,
		},
		ID: int(c.reqID.Add(1)),
	}

	return c.sendMessage(req)
}

// Disconnect closes the WebSocket connection
func (c *WSUserDataClient) Disconnect() error {
	c.connected.Store(false)
	c.authenticated.Store(false)

	select {
	case <-c.done:
	default:
		close(c.done)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// IsConnected returns whether the client is connected
func (c *WSUserDataClient) IsConnected() bool {
	return c.connected.Load()
}

// IsAuthenticated returns whether the client is authenticated
func (c *WSUserDataClient) IsAuthenticated() bool {
	return c.authenticated.Load()
}

// getNextReqID returns the next request ID
func (c *WSUserDataClient) getNextReqID() int {
	return int(c.reqID.Add(1))
}

// sendMessage sends a WebSocket message
func (c *WSUserDataClient) sendMessage(msg interface{}) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	return conn.WriteJSON(msg)
}

// =============================================================================
// Subscription Methods
// =============================================================================

// SubscribeOrders subscribes to order updates for specific markets
func (c *WSUserDataClient) SubscribeOrders(markets []string) error {
	if !c.authenticated.Load() {
		return fmt.Errorf("not authenticated")
	}

	for _, market := range markets {
		c.orderSubs[market] = true
	}

	req := WSRequest{
		Method: WSMethodOrderSubscribe,
		Params: WSMarketListParams{MarketList: markets},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// UnsubscribeOrders unsubscribes from order updates
func (c *WSUserDataClient) UnsubscribeOrders(markets []string) error {
	for _, market := range markets {
		delete(c.orderSubs, market)
	}

	req := WSRequest{
		Method: WSMethodOrderUnsubscribe,
		Params: WSMarketListParams{MarketList: markets},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// SubscribePositions subscribes to position updates
func (c *WSUserDataClient) SubscribePositions(markets []string) error {
	if !c.authenticated.Load() {
		return fmt.Errorf("not authenticated")
	}

	for _, market := range markets {
		c.positionSubs[market] = true
	}

	req := WSRequest{
		Method: WSMethodPositionSubscribe,
		Params: WSMarketListParams{MarketList: markets},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// UnsubscribePositions unsubscribes from position updates
func (c *WSUserDataClient) UnsubscribePositions(markets []string) error {
	for _, market := range markets {
		delete(c.positionSubs, market)
	}

	req := WSRequest{
		Method: WSMethodPositionUnsubscribe,
		Params: WSMarketListParams{MarketList: markets},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// SubscribeBalance subscribes to balance updates
func (c *WSUserDataClient) SubscribeBalance() error {
	if !c.authenticated.Load() {
		return fmt.Errorf("not authenticated")
	}

	c.balanceSubs = true

	req := WSRequest{
		Method: WSMethodBalanceSubscribe,
		Params: map[string]interface{}{},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// UnsubscribeBalance unsubscribes from balance updates
func (c *WSUserDataClient) UnsubscribeBalance() error {
	c.balanceSubs = false

	req := WSRequest{
		Method: WSMethodBalanceUnsubscribe,
		Params: map[string]interface{}{},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// =============================================================================
// Internal Methods
// =============================================================================

func (c *WSUserDataClient) readLoop(ctx context.Context) {
	defer c.connected.Store(false)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		default:
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn == nil {
				return
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				if c.onError != nil {
					c.onError(fmt.Errorf("websocket read error: %w", err))
				}
				return
			}

			// Decompress message if needed
			decompressed, err := c.decompressMessage(message)
			if err != nil {
				decompressed = message
			}

			c.handleMessage(decompressed)
		}
	}
}

func (c *WSUserDataClient) decompressMessage(data []byte) ([]byte, error) {
	// Check for gzip magic bytes (0x1f 0x8b)
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		// Gzip compressed
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		defer reader.Close()
		return io.ReadAll(reader)
	}

	// Try deflate compression
	reader := flate.NewReader(io.NopCloser(&userDataByteReader{data: data}))
	defer reader.Close()

	return io.ReadAll(reader)
}

type userDataByteReader struct {
	data []byte
	pos  int
}

func (r *userDataByteReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (c *WSUserDataClient) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-ticker.C:
			if !c.connected.Load() {
				continue
			}

			req := WSRequest{
				Method: WSMethodServerPing,
				Params: map[string]interface{}{},
				ID:     c.getNextReqID(),
			}

			if err := c.sendMessage(req); err != nil {
				log.Error().Err(err).Msg("Failed to send ping")
			}
		}
	}
}

func (c *WSUserDataClient) handleMessage(data []byte) {
	var msg WSResponse
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Debug().Err(err).Str("data", string(data)).Msg("Failed to parse WebSocket message")
		return
	}

	// Handle response to requests
	if msg.ID > 0 {
		// Check for authentication response
		if msg.Method == WSMethodServerSign || c.isAuthResponse(msg) {
			if msg.IsSuccess() {
				c.authenticated.Store(true)
				log.Info().Msg("CoinEx WebSocket authenticated successfully")
				if c.onAuthenticated != nil {
					c.onAuthenticated()
				}
			} else {
				if c.onError != nil {
					c.onError(fmt.Errorf("authentication failed: code=%d, msg=%s", msg.Code, msg.Message))
				}
			}
			return
		}

		// Handle other subscription responses
		if !msg.IsSuccess() {
			if c.onError != nil {
				c.onError(fmt.Errorf("request failed: code=%d, msg=%s", msg.Code, msg.Message))
			}
		}
		return
	}

	// Handle push messages
	switch msg.Method {
	case WSMethodOrderUpdate:
		c.handleOrderUpdate(msg.Data)
	case WSMethodPositionUpdate:
		c.handlePositionUpdate(msg.Data)
	case WSMethodBalanceUpdate:
		c.handleBalanceUpdate(msg.Data)
	case WSMethodServerPong:
		// Ignore pong response
	default:
		log.Debug().Str("method", msg.Method).Msg("Unknown WebSocket method")
	}
}

func (c *WSUserDataClient) isAuthResponse(msg WSResponse) bool {
	// Check if this is a response to the auth request (typically ID 1)
	return msg.ID == 1 && msg.Method == ""
}

func (c *WSUserDataClient) handleOrderUpdate(data json.RawMessage) {
	if c.onOrder == nil {
		return
	}

	var update WSOrderUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		log.Error().Err(err).Msg("Failed to parse order update")
		return
	}

	c.onOrder(&update)
}

func (c *WSUserDataClient) handlePositionUpdate(data json.RawMessage) {
	if c.onPosition == nil {
		return
	}

	var update WSPositionUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		log.Error().Err(err).Msg("Failed to parse position update")
		return
	}

	c.onPosition(&update)
}

func (c *WSUserDataClient) handleBalanceUpdate(data json.RawMessage) {
	if c.onBalance == nil {
		return
	}

	var update WSBalanceUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		log.Error().Err(err).Msg("Failed to parse balance update")
		return
	}

	c.onBalance(&update)
}

// ResubscribeAll resubscribes to all previously subscribed channels
func (c *WSUserDataClient) ResubscribeAll() error {
	if !c.authenticated.Load() {
		return fmt.Errorf("not authenticated")
	}

	// Resubscribe orders
	if len(c.orderSubs) > 0 {
		markets := make([]string, 0, len(c.orderSubs))
		for market := range c.orderSubs {
			markets = append(markets, market)
		}
		req := WSRequest{
			Method: WSMethodOrderSubscribe,
			Params: WSMarketListParams{MarketList: markets},
			ID:     c.getNextReqID(),
		}
		if err := c.sendMessage(req); err != nil {
			return err
		}
	}

	// Resubscribe positions
	if len(c.positionSubs) > 0 {
		markets := make([]string, 0, len(c.positionSubs))
		for market := range c.positionSubs {
			markets = append(markets, market)
		}
		req := WSRequest{
			Method: WSMethodPositionSubscribe,
			Params: WSMarketListParams{MarketList: markets},
			ID:     c.getNextReqID(),
		}
		if err := c.sendMessage(req); err != nil {
			return err
		}
	}

	// Resubscribe balance
	if c.balanceSubs {
		req := WSRequest{
			Method: WSMethodBalanceSubscribe,
			Params: map[string]interface{}{},
			ID:     c.getNextReqID(),
		}
		if err := c.sendMessage(req); err != nil {
			return err
		}
	}

	return nil
}
