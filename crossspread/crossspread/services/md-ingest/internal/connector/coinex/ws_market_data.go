// Package coinex provides WebSocket market data client for CoinEx Futures exchange.
package coinex

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// WSMarketDataClient handles public WebSocket connections for market data
type WSMarketDataClient struct {
	url       string
	conn      *websocket.Conn
	connected atomic.Bool
	reqID     atomic.Int64
	mu        sync.RWMutex
	done      chan struct{}
	reconnect chan struct{}

	// Callbacks
	onDepth     func(*WSDepthUpdate)
	onDeals     func(*WSDealsUpdate)
	onBBO       func(*WSBBOUpdate)
	onState     func(*WSStateUpdate)
	onIndex     func(*WSIndexUpdate)
	onError     func(error)
	onConnected func()

	// Subscriptions tracking
	depthSubs map[string][]interface{} // market -> [limit, interval, is_full]
	dealsSubs map[string]bool
	bboSubs   map[string]bool
	stateSubs map[string]bool
	indexSubs map[string]bool
}

// WSMarketDataConfig holds configuration for market data WebSocket client
type WSMarketDataConfig struct {
	URL            string
	ReconnectDelay time.Duration
	PingInterval   time.Duration
}

// NewWSMarketDataClient creates a new market data WebSocket client
func NewWSMarketDataClient(cfg WSMarketDataConfig) *WSMarketDataClient {
	if cfg.URL == "" {
		cfg.URL = WSFuturesURL
	}
	if cfg.ReconnectDelay == 0 {
		cfg.ReconnectDelay = 5 * time.Second
	}
	if cfg.PingInterval == 0 {
		cfg.PingInterval = 20 * time.Second
	}

	return &WSMarketDataClient{
		url:       cfg.URL,
		done:      make(chan struct{}),
		reconnect: make(chan struct{}, 1),
		depthSubs: make(map[string][]interface{}),
		dealsSubs: make(map[string]bool),
		bboSubs:   make(map[string]bool),
		stateSubs: make(map[string]bool),
		indexSubs: make(map[string]bool),
	}
}

// SetDepthHandler sets the callback for depth updates
func (c *WSMarketDataClient) SetDepthHandler(handler func(*WSDepthUpdate)) {
	c.onDepth = handler
}

// SetDealsHandler sets the callback for deals updates
func (c *WSMarketDataClient) SetDealsHandler(handler func(*WSDealsUpdate)) {
	c.onDeals = handler
}

// SetBBOHandler sets the callback for BBO updates
func (c *WSMarketDataClient) SetBBOHandler(handler func(*WSBBOUpdate)) {
	c.onBBO = handler
}

// SetStateHandler sets the callback for market state updates
func (c *WSMarketDataClient) SetStateHandler(handler func(*WSStateUpdate)) {
	c.onState = handler
}

// SetIndexHandler sets the callback for index updates
func (c *WSMarketDataClient) SetIndexHandler(handler func(*WSIndexUpdate)) {
	c.onIndex = handler
}

// SetErrorHandler sets the callback for errors
func (c *WSMarketDataClient) SetErrorHandler(handler func(error)) {
	c.onError = handler
}

// SetConnectedHandler sets the callback for connection established
func (c *WSMarketDataClient) SetConnectedHandler(handler func()) {
	c.onConnected = handler
}

// Connect establishes WebSocket connection
func (c *WSMarketDataClient) Connect(ctx context.Context) error {
	log.Info().Str("url", c.url).Msg("Connecting to CoinEx WebSocket")

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

	log.Info().Msg("Connected to CoinEx WebSocket")

	if c.onConnected != nil {
		c.onConnected()
	}

	// Start goroutines
	go c.readLoop(ctx)
	go c.pingLoop(ctx)

	return nil
}

// Disconnect closes the WebSocket connection
func (c *WSMarketDataClient) Disconnect() error {
	c.connected.Store(false)

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
func (c *WSMarketDataClient) IsConnected() bool {
	return c.connected.Load()
}

// getNextReqID returns the next request ID
func (c *WSMarketDataClient) getNextReqID() int {
	return int(c.reqID.Add(1))
}

// sendMessage sends a WebSocket message
func (c *WSMarketDataClient) sendMessage(msg interface{}) error {
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

// SubscribeDepth subscribes to orderbook depth updates
func (c *WSMarketDataClient) SubscribeDepth(markets []string, limit int, interval string, isFull bool) error {
	if len(markets) == 0 {
		return fmt.Errorf("no markets to subscribe")
	}

	// Build market list with parameters
	marketList := make([][]interface{}, 0, len(markets))
	for _, market := range markets {
		marketList = append(marketList, []interface{}{market, limit, interval, isFull})
		c.depthSubs[market] = []interface{}{limit, interval, isFull}
	}

	req := WSRequest{
		Method: WSMethodDepthSubscribe,
		Params: WSDepthSubscribeParams{MarketList: marketList},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// UnsubscribeDepth unsubscribes from orderbook depth updates
func (c *WSMarketDataClient) UnsubscribeDepth(markets []string) error {
	for _, market := range markets {
		delete(c.depthSubs, market)
	}

	req := WSRequest{
		Method: WSMethodDepthUnsubscribe,
		Params: WSMarketListParams{MarketList: markets},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// SubscribeDeals subscribes to trade updates
func (c *WSMarketDataClient) SubscribeDeals(markets []string) error {
	if len(markets) == 0 {
		return fmt.Errorf("no markets to subscribe")
	}

	for _, market := range markets {
		c.dealsSubs[market] = true
	}

	req := WSRequest{
		Method: WSMethodDealsSubscribe,
		Params: WSMarketListParams{MarketList: markets},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// UnsubscribeDeals unsubscribes from trade updates
func (c *WSMarketDataClient) UnsubscribeDeals(markets []string) error {
	for _, market := range markets {
		delete(c.dealsSubs, market)
	}

	req := WSRequest{
		Method: WSMethodDealsUnsubscribe,
		Params: WSMarketListParams{MarketList: markets},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// SubscribeBBO subscribes to best bid/offer updates
func (c *WSMarketDataClient) SubscribeBBO(markets []string) error {
	if len(markets) == 0 {
		return fmt.Errorf("no markets to subscribe")
	}

	for _, market := range markets {
		c.bboSubs[market] = true
	}

	req := WSRequest{
		Method: WSMethodBBOSubscribe,
		Params: WSMarketListParams{MarketList: markets},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// UnsubscribeBBO unsubscribes from best bid/offer updates
func (c *WSMarketDataClient) UnsubscribeBBO(markets []string) error {
	for _, market := range markets {
		delete(c.bboSubs, market)
	}

	req := WSRequest{
		Method: WSMethodBBOUnsubscribe,
		Params: WSMarketListParams{MarketList: markets},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// SubscribeState subscribes to market state (ticker) updates
func (c *WSMarketDataClient) SubscribeState(markets []string) error {
	for _, market := range markets {
		c.stateSubs[market] = true
	}

	req := WSRequest{
		Method: WSMethodStateSubscribe,
		Params: WSMarketListParams{MarketList: markets},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// UnsubscribeState unsubscribes from market state updates
func (c *WSMarketDataClient) UnsubscribeState(markets []string) error {
	for _, market := range markets {
		delete(c.stateSubs, market)
	}

	req := WSRequest{
		Method: WSMethodStateUnsubscribe,
		Params: WSMarketListParams{MarketList: markets},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// SubscribeIndex subscribes to index price updates
func (c *WSMarketDataClient) SubscribeIndex(markets []string) error {
	for _, market := range markets {
		c.indexSubs[market] = true
	}

	req := WSRequest{
		Method: WSMethodIndexSubscribe,
		Params: WSMarketListParams{MarketList: markets},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// UnsubscribeIndex unsubscribes from index price updates
func (c *WSMarketDataClient) UnsubscribeIndex(markets []string) error {
	for _, market := range markets {
		delete(c.indexSubs, market)
	}

	req := WSRequest{
		Method: WSMethodIndexUnsubscribe,
		Params: WSMarketListParams{MarketList: markets},
		ID:     c.getNextReqID(),
	}

	return c.sendMessage(req)
}

// =============================================================================
// Internal Methods
// =============================================================================

func (c *WSMarketDataClient) readLoop(ctx context.Context) {
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

			// Decompress message if needed (CoinEx uses deflate compression)
			decompressed, err := c.decompressMessage(message)
			if err != nil {
				// If decompression fails, try using raw message
				decompressed = message
			}

			c.handleMessage(decompressed)
		}
	}
}

func (c *WSMarketDataClient) decompressMessage(data []byte) ([]byte, error) {
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
	reader := flate.NewReader(io.NopCloser(&byteReader{data: data}))
	defer reader.Close()

	return io.ReadAll(reader)
}

// byteReader wraps a byte slice to implement io.Reader
type byteReader struct {
	data []byte
	pos  int
}

func (r *byteReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (c *WSMarketDataClient) pingLoop(ctx context.Context) {
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

func (c *WSMarketDataClient) handleMessage(data []byte) {
	var msg WSResponse
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Debug().Err(err).Str("data", string(data)).Msg("Failed to parse WebSocket message")
		return
	}

	// Handle response to subscription requests
	if msg.ID > 0 {
		if !msg.IsSuccess() {
			if c.onError != nil {
				c.onError(fmt.Errorf("subscription error: code=%d, msg=%s", msg.Code, msg.Message))
			}
		}
		return
	}

	// Handle push messages
	switch msg.Method {
	case WSMethodDepthUpdate:
		c.handleDepthUpdate(msg.Data)
	case WSMethodDealsUpdate:
		c.handleDealsUpdate(msg.Data)
	case WSMethodBBOUpdate:
		c.handleBBOUpdate(msg.Data)
	case WSMethodStateUpdate:
		c.handleStateUpdate(msg.Data)
	case WSMethodIndexUpdate:
		c.handleIndexUpdate(msg.Data)
	case WSMethodServerPong:
		// Ignore pong response
	default:
		log.Debug().Str("method", msg.Method).Msg("Unknown WebSocket method")
	}
}

func (c *WSMarketDataClient) handleDepthUpdate(data json.RawMessage) {
	if c.onDepth == nil {
		return
	}

	var update WSDepthUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		log.Error().Err(err).Msg("Failed to parse depth update")
		return
	}

	c.onDepth(&update)
}

func (c *WSMarketDataClient) handleDealsUpdate(data json.RawMessage) {
	if c.onDeals == nil {
		return
	}

	var update WSDealsUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		log.Error().Err(err).Msg("Failed to parse deals update")
		return
	}

	c.onDeals(&update)
}

func (c *WSMarketDataClient) handleBBOUpdate(data json.RawMessage) {
	if c.onBBO == nil {
		return
	}

	var update WSBBOUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		log.Error().Err(err).Msg("Failed to parse BBO update")
		return
	}

	c.onBBO(&update)
}

func (c *WSMarketDataClient) handleStateUpdate(data json.RawMessage) {
	if c.onState == nil {
		return
	}

	var update WSStateUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		log.Error().Err(err).Msg("Failed to parse state update")
		return
	}

	c.onState(&update)
}

func (c *WSMarketDataClient) handleIndexUpdate(data json.RawMessage) {
	if c.onIndex == nil {
		return
	}

	var update WSIndexUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		log.Error().Err(err).Msg("Failed to parse index update")
		return
	}

	c.onIndex(&update)
}

// ResubscribeAll resubscribes to all previously subscribed channels
func (c *WSMarketDataClient) ResubscribeAll() error {
	// Resubscribe depth
	if len(c.depthSubs) > 0 {
		marketList := make([][]interface{}, 0, len(c.depthSubs))
		for market, params := range c.depthSubs {
			if len(params) >= 3 {
				marketList = append(marketList, []interface{}{market, params[0], params[1], params[2]})
			}
		}
		if len(marketList) > 0 {
			req := WSRequest{
				Method: WSMethodDepthSubscribe,
				Params: WSDepthSubscribeParams{MarketList: marketList},
				ID:     c.getNextReqID(),
			}
			if err := c.sendMessage(req); err != nil {
				return err
			}
		}
	}

	// Resubscribe deals
	if len(c.dealsSubs) > 0 {
		markets := make([]string, 0, len(c.dealsSubs))
		for market := range c.dealsSubs {
			markets = append(markets, market)
		}
		req := WSRequest{
			Method: WSMethodDealsSubscribe,
			Params: WSMarketListParams{MarketList: markets},
			ID:     c.getNextReqID(),
		}
		if err := c.sendMessage(req); err != nil {
			return err
		}
	}

	// Resubscribe BBO
	if len(c.bboSubs) > 0 {
		markets := make([]string, 0, len(c.bboSubs))
		for market := range c.bboSubs {
			markets = append(markets, market)
		}
		req := WSRequest{
			Method: WSMethodBBOSubscribe,
			Params: WSMarketListParams{MarketList: markets},
			ID:     c.getNextReqID(),
		}
		if err := c.sendMessage(req); err != nil {
			return err
		}
	}

	// Resubscribe state
	if len(c.stateSubs) > 0 {
		markets := make([]string, 0, len(c.stateSubs))
		for market := range c.stateSubs {
			markets = append(markets, market)
		}
		req := WSRequest{
			Method: WSMethodStateSubscribe,
			Params: WSMarketListParams{MarketList: markets},
			ID:     c.getNextReqID(),
		}
		if err := c.sendMessage(req); err != nil {
			return err
		}
	}

	// Resubscribe index
	if len(c.indexSubs) > 0 {
		markets := make([]string, 0, len(c.indexSubs))
		for market := range c.indexSubs {
			markets = append(markets, market)
		}
		req := WSRequest{
			Method: WSMethodIndexSubscribe,
			Params: WSMarketListParams{MarketList: markets},
			ID:     c.getNextReqID(),
		}
		if err := c.sendMessage(req); err != nil {
			return err
		}
	}

	return nil
}
