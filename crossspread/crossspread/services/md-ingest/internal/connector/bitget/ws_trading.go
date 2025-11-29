// Package bitget provides WebSocket trading client for Bitget exchange.
// Supports low-latency order placement and cancellation via WebSocket.
package bitget

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// TradingHandler handles trading operation responses
type TradingHandler interface {
	OnOrderResponse(id string, result *WSTradeResponse)
	OnError(err error)
	OnConnected()
	OnDisconnected()
}

// TradingWSClient handles WebSocket connections for trading operations
type TradingWSClient struct {
	url        string
	conn       *websocket.Conn
	handler    TradingHandler
	instType   string
	apiKey     string
	secretKey  string
	passphrase string

	requestID  uint64   // Atomic counter for request IDs
	pendingOps sync.Map // id -> chan *WSTradeResponse

	writeMu sync.Mutex
	done    chan struct{}
	wg      sync.WaitGroup

	reconnect     bool
	reconnectWait time.Duration
	maxReconnect  int

	pingInterval time.Duration
	pongWait     time.Duration

	ctx    context.Context
	cancel context.CancelFunc

	isAuthenticated bool
	authMu          sync.Mutex
}

// TradingWSConfig holds configuration for trading WebSocket client
type TradingWSConfig struct {
	InstType      string // Required: USDT-FUTURES, USDC-FUTURES, COIN-FUTURES
	APIKey        string
	SecretKey     string
	Passphrase    string
	Handler       TradingHandler
	PingInterval  time.Duration
	PongWait      time.Duration
	ReconnectWait time.Duration
	MaxReconnect  int
}

// NewTradingWSClient creates a new trading WebSocket client
func NewTradingWSClient(cfg TradingWSConfig) *TradingWSClient {
	if cfg.PingInterval == 0 {
		cfg.PingInterval = 25 * time.Second
	}
	if cfg.PongWait == 0 {
		cfg.PongWait = 30 * time.Second
	}
	if cfg.ReconnectWait == 0 {
		cfg.ReconnectWait = 5 * time.Second
	}
	if cfg.MaxReconnect == 0 {
		cfg.MaxReconnect = 3
	}
	if cfg.InstType == "" {
		cfg.InstType = ProductTypeUSDTFutures
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TradingWSClient{
		url:           WSPrivateURL,
		handler:       cfg.Handler,
		instType:      cfg.InstType,
		apiKey:        cfg.APIKey,
		secretKey:     cfg.SecretKey,
		passphrase:    cfg.Passphrase,
		done:          make(chan struct{}),
		reconnect:     true,
		reconnectWait: cfg.ReconnectWait,
		maxReconnect:  cfg.MaxReconnect,
		pingInterval:  cfg.PingInterval,
		pongWait:      cfg.PongWait,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Connect establishes WebSocket connection and authenticates
func (c *TradingWSClient) Connect() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(c.url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.conn = conn
	c.done = make(chan struct{})
	c.isAuthenticated = false

	// Start goroutines
	c.wg.Add(2)
	go c.readLoop()
	go c.pingLoop()

	// Authenticate
	if err := c.authenticate(); err != nil {
		c.Close()
		return fmt.Errorf("authentication failed: %w", err)
	}

	if c.handler != nil {
		c.handler.OnConnected()
	}

	return nil
}

// authenticate sends login request
func (c *TradingWSClient) authenticate() error {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	sign := c.sign(timestamp)

	req := WSLoginRequest{
		Op: WSOpLogin,
		Args: []WSLoginArg{{
			APIKey:     c.apiKey,
			Passphrase: c.passphrase,
			Timestamp:  timestamp,
			Sign:       sign,
		}},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return err
	}

	// Wait for authentication response (handled in readLoop)
	// In production, should use a channel to wait for auth confirmation
	time.Sleep(500 * time.Millisecond)

	c.authMu.Lock()
	c.isAuthenticated = true
	c.authMu.Unlock()

	return nil
}

// sign generates HMAC-SHA256 signature for WebSocket authentication
// Bitget WS auth: Base64(HMAC_SHA256(timestamp + 'GET' + '/user/verify', secretKey))
func (c *TradingWSClient) sign(timestamp string) string {
	message := timestamp + "GET" + "/user/verify"
	h := hmac.New(sha256.New, []byte(c.secretKey))
	h.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// Close closes the WebSocket connection
func (c *TradingWSClient) Close() error {
	c.reconnect = false
	c.cancel()

	if c.conn != nil {
		close(c.done)
		c.writeMu.Lock()
		err := c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.writeMu.Unlock()
		if err != nil {
			return err
		}
		c.conn.Close()
	}

	c.wg.Wait()
	return nil
}

// readLoop reads messages from WebSocket
func (c *TradingWSClient) readLoop() {
	defer c.wg.Done()
	defer c.handleDisconnect()

	for {
		select {
		case <-c.done:
			return
		case <-c.ctx.Done():
			return
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					if c.handler != nil {
						c.handler.OnError(fmt.Errorf("WebSocket read error: %w", err))
					}
				}
				return
			}

			c.handleMessage(message)
		}
	}
}

// pingLoop sends periodic ping messages
func (c *TradingWSClient) pingLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.writeMu.Lock()
			err := c.conn.WriteMessage(websocket.TextMessage, []byte("ping"))
			c.writeMu.Unlock()
			if err != nil {
				if c.handler != nil {
					c.handler.OnError(fmt.Errorf("ping error: %w", err))
				}
				return
			}
		}
	}
}

// handleDisconnect handles disconnection and reconnection
func (c *TradingWSClient) handleDisconnect() {
	c.authMu.Lock()
	c.isAuthenticated = false
	c.authMu.Unlock()

	if c.handler != nil {
		c.handler.OnDisconnected()
	}

	if !c.reconnect {
		return
	}

	// Attempt reconnection
	for i := 0; i < c.maxReconnect; i++ {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(c.reconnectWait):
		}

		if err := c.Connect(); err != nil {
			if c.handler != nil {
				c.handler.OnError(fmt.Errorf("reconnect attempt %d failed: %w", i+1, err))
			}
			continue
		}

		return
	}

	if c.handler != nil {
		c.handler.OnError(fmt.Errorf("max reconnection attempts reached"))
	}
}

// handleMessage processes incoming WebSocket messages
func (c *TradingWSClient) handleMessage(data []byte) {
	// Handle pong response
	if string(data) == "pong" {
		return
	}

	var resp WSTradeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		if c.handler != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal message: %w", err))
		}
		return
	}

	// Check if this is a response to a pending operation
	if resp.ID != "" {
		if ch, ok := c.pendingOps.Load(resp.ID); ok {
			respCh := ch.(chan *WSTradeResponse)
			select {
			case respCh <- &resp:
			default:
			}
			c.pendingOps.Delete(resp.ID)
		}

		// Also notify handler
		if c.handler != nil {
			c.handler.OnOrderResponse(resp.ID, &resp)
		}
	}
}

// nextRequestID generates a unique request ID
func (c *TradingWSClient) nextRequestID() string {
	id := atomic.AddUint64(&c.requestID, 1)
	return strconv.FormatUint(id, 10)
}

// =============================================================================
// Order Operations
// =============================================================================

// PlaceOrder places a single order via WebSocket
func (c *TradingWSClient) PlaceOrder(ctx context.Context, req *WSPlaceOrderArg) (*WSTradeResponse, error) {
	c.authMu.Lock()
	if !c.isAuthenticated {
		c.authMu.Unlock()
		return nil, fmt.Errorf("not authenticated")
	}
	c.authMu.Unlock()

	id := c.nextRequestID()
	respCh := make(chan *WSTradeResponse, 1)
	c.pendingOps.Store(id, respCh)
	defer c.pendingOps.Delete(id)

	wsReq := WSTradeRequest{
		ID: id,
		Op: WSOpPlaceOrder,
		Args: []interface{}{map[string]interface{}{
			"instId":     req.InstID,
			"marginCoin": req.MarginCoin,
			"size":       req.Size,
			"price":      req.Price,
			"side":       req.Side,
			"tradeSide":  req.TradeSide,
			"orderType":  req.OrderType,
			"force":      req.Force,
			"clientOid":  req.ClientOID,
			"reduceOnly": req.ReduceOnly,
		}},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(wsReq)
	c.writeMu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to send order: %w", err)
	}

	// Wait for response with timeout
	select {
	case resp := <-respCh:
		if resp.Code != ErrCodeSuccess {
			return resp, &APIError{Code: resp.Code, Message: resp.Msg}
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("order response timeout")
	}
}

// PlaceOrderAsync places a single order asynchronously (fire and forget)
func (c *TradingWSClient) PlaceOrderAsync(req *WSPlaceOrderArg) (string, error) {
	c.authMu.Lock()
	if !c.isAuthenticated {
		c.authMu.Unlock()
		return "", fmt.Errorf("not authenticated")
	}
	c.authMu.Unlock()

	id := c.nextRequestID()

	wsReq := WSTradeRequest{
		ID: id,
		Op: WSOpPlaceOrder,
		Args: []interface{}{map[string]interface{}{
			"instId":     req.InstID,
			"marginCoin": req.MarginCoin,
			"size":       req.Size,
			"price":      req.Price,
			"side":       req.Side,
			"tradeSide":  req.TradeSide,
			"orderType":  req.OrderType,
			"force":      req.Force,
			"clientOid":  req.ClientOID,
			"reduceOnly": req.ReduceOnly,
		}},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(wsReq)
	c.writeMu.Unlock()

	if err != nil {
		return "", fmt.Errorf("failed to send order: %w", err)
	}

	return id, nil
}

// BatchPlaceOrder places multiple orders via WebSocket
func (c *TradingWSClient) BatchPlaceOrder(ctx context.Context, reqs []WSPlaceOrderArg) (*WSTradeResponse, error) {
	c.authMu.Lock()
	if !c.isAuthenticated {
		c.authMu.Unlock()
		return nil, fmt.Errorf("not authenticated")
	}
	c.authMu.Unlock()

	id := c.nextRequestID()
	respCh := make(chan *WSTradeResponse, 1)
	c.pendingOps.Store(id, respCh)
	defer c.pendingOps.Delete(id)

	args := make([]interface{}, len(reqs))
	for i, req := range reqs {
		args[i] = map[string]interface{}{
			"instId":     req.InstID,
			"marginCoin": req.MarginCoin,
			"size":       req.Size,
			"price":      req.Price,
			"side":       req.Side,
			"tradeSide":  req.TradeSide,
			"orderType":  req.OrderType,
			"force":      req.Force,
			"clientOid":  req.ClientOID,
			"reduceOnly": req.ReduceOnly,
		}
	}

	wsReq := WSTradeRequest{
		ID:   id,
		Op:   WSOpBatchPlaceOrder,
		Args: args,
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(wsReq)
	c.writeMu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to send batch order: %w", err)
	}

	// Wait for response with timeout
	select {
	case resp := <-respCh:
		if resp.Code != ErrCodeSuccess {
			return resp, &APIError{Code: resp.Code, Message: resp.Msg}
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("batch order response timeout")
	}
}

// CancelOrder cancels a single order via WebSocket
func (c *TradingWSClient) CancelOrder(ctx context.Context, req *WSCancelOrderArg) (*WSTradeResponse, error) {
	c.authMu.Lock()
	if !c.isAuthenticated {
		c.authMu.Unlock()
		return nil, fmt.Errorf("not authenticated")
	}
	c.authMu.Unlock()

	id := c.nextRequestID()
	respCh := make(chan *WSTradeResponse, 1)
	c.pendingOps.Store(id, respCh)
	defer c.pendingOps.Delete(id)

	wsReq := WSTradeRequest{
		ID: id,
		Op: WSOpCancelOrder,
		Args: []interface{}{map[string]interface{}{
			"instId":    req.InstID,
			"orderId":   req.OrderID,
			"clientOid": req.ClientOID,
		}},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(wsReq)
	c.writeMu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to send cancel: %w", err)
	}

	// Wait for response with timeout
	select {
	case resp := <-respCh:
		if resp.Code != ErrCodeSuccess {
			return resp, &APIError{Code: resp.Code, Message: resp.Msg}
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("cancel response timeout")
	}
}

// CancelOrderAsync cancels a single order asynchronously
func (c *TradingWSClient) CancelOrderAsync(req *WSCancelOrderArg) (string, error) {
	c.authMu.Lock()
	if !c.isAuthenticated {
		c.authMu.Unlock()
		return "", fmt.Errorf("not authenticated")
	}
	c.authMu.Unlock()

	id := c.nextRequestID()

	wsReq := WSTradeRequest{
		ID: id,
		Op: WSOpCancelOrder,
		Args: []interface{}{map[string]interface{}{
			"instId":    req.InstID,
			"orderId":   req.OrderID,
			"clientOid": req.ClientOID,
		}},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(wsReq)
	c.writeMu.Unlock()

	if err != nil {
		return "", fmt.Errorf("failed to send cancel: %w", err)
	}

	return id, nil
}

// BatchCancelOrder cancels multiple orders via WebSocket
func (c *TradingWSClient) BatchCancelOrder(ctx context.Context, reqs []WSCancelOrderArg) (*WSTradeResponse, error) {
	c.authMu.Lock()
	if !c.isAuthenticated {
		c.authMu.Unlock()
		return nil, fmt.Errorf("not authenticated")
	}
	c.authMu.Unlock()

	id := c.nextRequestID()
	respCh := make(chan *WSTradeResponse, 1)
	c.pendingOps.Store(id, respCh)
	defer c.pendingOps.Delete(id)

	args := make([]interface{}, len(reqs))
	for i, req := range reqs {
		args[i] = map[string]interface{}{
			"instId":    req.InstID,
			"orderId":   req.OrderID,
			"clientOid": req.ClientOID,
		}
	}

	wsReq := WSTradeRequest{
		ID:   id,
		Op:   WSOpBatchCancelOrder,
		Args: args,
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(wsReq)
	c.writeMu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to send batch cancel: %w", err)
	}

	// Wait for response with timeout
	select {
	case resp := <-respCh:
		if resp.Code != ErrCodeSuccess {
			return resp, &APIError{Code: resp.Code, Message: resp.Msg}
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("batch cancel response timeout")
	}
}

// IsAuthenticated returns whether the client is authenticated
func (c *TradingWSClient) IsAuthenticated() bool {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	return c.isAuthenticated
}

// IsConnected returns whether the client is connected
func (c *TradingWSClient) IsConnected() bool {
	return c.conn != nil
}
