// Package mexc provides WebSocket trading client for MEXC exchange.
package mexc

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocket private URL
const (
	WSPrivateURL = "wss://contract.mexc.com/edge"
)

// TradingHandler handles trading responses from WebSocket
type TradingHandler interface {
	OnOrderPlaced(order *WSOrderResponse)
	OnOrderCanceled(orderID string, success bool, message string)
	OnOrderFilled(order *WSOrderUpdate)
	OnOrderError(err error)
	OnConnected()
	OnDisconnected()
}

// WSOrderResponse represents response to order placement
type WSOrderResponse struct {
	OrderID    string `json:"orderId"`
	ExternalID string `json:"externalOid"`
	Success    bool   `json:"success"`
	Code       int    `json:"code"`
	Message    string `json:"msg"`
}

// TradingWSClient handles WebSocket connections for trading operations
type TradingWSClient struct {
	url       string
	conn      *websocket.Conn
	handler   TradingHandler
	apiKey    string
	secretKey string

	writeMu sync.Mutex
	done    chan struct{}
	wg      sync.WaitGroup

	reconnect      bool
	reconnectWait  time.Duration
	maxReconnect   int
	reconnectCount int

	authenticated int32 // atomic
	authChan      chan bool

	requestID   int64
	pendingReqs map[int64]chan json.RawMessage
	pendingMu   sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

// TradingWSConfig holds configuration for trading WebSocket client
type TradingWSConfig struct {
	APIKey        string
	SecretKey     string
	Handler       TradingHandler
	ReconnectWait time.Duration
	MaxReconnect  int
}

// NewTradingWSClient creates a new trading WebSocket client
func NewTradingWSClient(cfg TradingWSConfig) *TradingWSClient {
	if cfg.ReconnectWait == 0 {
		cfg.ReconnectWait = 5 * time.Second
	}
	if cfg.MaxReconnect == 0 {
		cfg.MaxReconnect = 3
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TradingWSClient{
		url:           WSPrivateURL,
		handler:       cfg.Handler,
		apiKey:        cfg.APIKey,
		secretKey:     cfg.SecretKey,
		done:          make(chan struct{}),
		reconnect:     true,
		reconnectWait: cfg.ReconnectWait,
		maxReconnect:  cfg.MaxReconnect,
		authChan:      make(chan bool, 1),
		pendingReqs:   make(map[int64]chan json.RawMessage),
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
	c.reconnectCount = 0
	atomic.StoreInt32(&c.authenticated, 0)

	// Start read loop
	c.wg.Add(1)
	go c.readLoop()

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

// authenticate sends authentication request
func (c *TradingWSClient) authenticate() error {
	timestamp := time.Now().UnixMilli()
	signStr := fmt.Sprintf("%s%d", c.apiKey, timestamp)
	signature := c.sign(signStr)

	authReq := map[string]interface{}{
		"method": "login",
		"param": map[string]interface{}{
			"apiKey":    c.apiKey,
			"reqTime":   timestamp,
			"signature": signature,
		},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(authReq)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to send auth request: %w", err)
	}

	// Wait for auth response
	select {
	case success := <-c.authChan:
		if !success {
			return fmt.Errorf("authentication rejected")
		}
	case <-time.After(10 * time.Second):
		return fmt.Errorf("authentication timeout")
	}

	return nil
}

// sign creates HMAC-SHA256 signature
func (c *TradingWSClient) sign(message string) string {
	mac := hmac.New(sha256.New, []byte(c.secretKey))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// Close closes the WebSocket connection
func (c *TradingWSClient) Close() error {
	c.reconnect = false
	c.cancel()

	if c.conn != nil {
		close(c.done)
		c.writeMu.Lock()
		_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.writeMu.Unlock()
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
						c.handler.OnOrderError(fmt.Errorf("WebSocket read error: %w", err))
					}
				}
				return
			}

			c.handleMessage(message)
		}
	}
}

// handleDisconnect handles disconnection and reconnection
func (c *TradingWSClient) handleDisconnect() {
	atomic.StoreInt32(&c.authenticated, 0)

	if c.handler != nil {
		c.handler.OnDisconnected()
	}

	// Attempt reconnection if enabled
	if c.reconnect && c.reconnectCount < c.maxReconnect {
		c.reconnectCount++
		time.Sleep(c.reconnectWait)

		if err := c.Connect(); err != nil {
			if c.handler != nil {
				c.handler.OnOrderError(fmt.Errorf("reconnection failed: %w", err))
			}
		}
	}
}

// handleMessage processes incoming WebSocket messages
func (c *TradingWSClient) handleMessage(data []byte) {
	var msg struct {
		Channel string          `json:"channel"`
		Data    json.RawMessage `json:"data"`
		ID      int64           `json:"id,omitempty"`
		Code    int             `json:"code,omitempty"`
		Success bool            `json:"success,omitempty"`
		Msg     string          `json:"msg,omitempty"`
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		if c.handler != nil {
			c.handler.OnOrderError(fmt.Errorf("failed to parse message: %w", err))
		}
		return
	}

	// Handle auth response
	if msg.Channel == "rs.login" {
		success := msg.Code == 0 || msg.Success
		atomic.StoreInt32(&c.authenticated, boolToInt32(success))
		select {
		case c.authChan <- success:
		default:
		}
		return
	}

	// Handle pong
	if msg.Channel == "pong" {
		return
	}

	// Handle request-response pattern
	if msg.ID != 0 {
		c.pendingMu.RLock()
		ch, ok := c.pendingReqs[msg.ID]
		c.pendingMu.RUnlock()
		if ok {
			select {
			case ch <- data:
			default:
			}
			return
		}
	}

	// Route by channel
	switch msg.Channel {
	case "push.personal.order":
		c.handleOrderUpdate(msg.Data)
	case "rs.order.place":
		c.handleOrderPlaceResponse(data)
	case "rs.order.cancel":
		c.handleOrderCancelResponse(data)
	}
}

// handleOrderUpdate processes order update pushes
func (c *TradingWSClient) handleOrderUpdate(data json.RawMessage) {
	if c.handler == nil {
		return
	}

	var update WSOrderUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		c.handler.OnOrderError(fmt.Errorf("failed to parse order update: %w", err))
		return
	}

	c.handler.OnOrderFilled(&update)
}

// handleOrderPlaceResponse processes order placement responses
func (c *TradingWSClient) handleOrderPlaceResponse(data []byte) {
	if c.handler == nil {
		return
	}

	var resp struct {
		Code    int    `json:"code"`
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
		Data    struct {
			OrderID string `json:"orderId"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		c.handler.OnOrderError(fmt.Errorf("failed to parse order response: %w", err))
		return
	}

	c.handler.OnOrderPlaced(&WSOrderResponse{
		OrderID: resp.Data.OrderID,
		Success: resp.Code == 0 || resp.Success,
		Code:    resp.Code,
		Message: resp.Msg,
	})
}

// handleOrderCancelResponse processes order cancellation responses
func (c *TradingWSClient) handleOrderCancelResponse(data []byte) {
	if c.handler == nil {
		return
	}

	var resp struct {
		Code    int    `json:"code"`
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
		Data    struct {
			OrderID string `json:"orderId"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		c.handler.OnOrderError(fmt.Errorf("failed to parse cancel response: %w", err))
		return
	}

	c.handler.OnOrderCanceled(resp.Data.OrderID, resp.Code == 0 || resp.Success, resp.Msg)
}

// =============================================================================
// Trading Methods
// =============================================================================

// PlaceOrder places a new order via WebSocket
func (c *TradingWSClient) PlaceOrder(req *PlaceOrderRequest) (*WSOrderResponse, error) {
	if atomic.LoadInt32(&c.authenticated) != 1 {
		return nil, fmt.Errorf("not authenticated")
	}

	id := atomic.AddInt64(&c.requestID, 1)
	respChan := make(chan json.RawMessage, 1)

	c.pendingMu.Lock()
	c.pendingReqs[id] = respChan
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pendingReqs, id)
		c.pendingMu.Unlock()
	}()

	orderReq := map[string]interface{}{
		"method": "order.place",
		"id":     id,
		"param": map[string]interface{}{
			"symbol":      req.Symbol,
			"price":       req.Price,
			"vol":         req.Volume,
			"leverage":    req.Leverage,
			"side":        req.Side,
			"type":        req.OrderType,
			"openType":    req.OpenType,
			"externalOid": req.ExternalOID,
		},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(orderReq)
	c.writeMu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to send order: %w", err)
	}

	// Wait for response
	select {
	case respData := <-respChan:
		var resp struct {
			Code    int    `json:"code"`
			Success bool   `json:"success"`
			Msg     string `json:"msg"`
			Data    struct {
				OrderID string `json:"orderId"`
			} `json:"data"`
		}
		if err := json.Unmarshal(respData, &resp); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		return &WSOrderResponse{
			OrderID: resp.Data.OrderID,
			Success: resp.Code == 0 || resp.Success,
			Code:    resp.Code,
			Message: resp.Msg,
		}, nil
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("order request timeout")
	case <-c.ctx.Done():
		return nil, fmt.Errorf("context canceled")
	}
}

// CancelOrder cancels an order via WebSocket
func (c *TradingWSClient) CancelOrder(symbol string, orderID string) error {
	if atomic.LoadInt32(&c.authenticated) != 1 {
		return fmt.Errorf("not authenticated")
	}

	id := atomic.AddInt64(&c.requestID, 1)
	respChan := make(chan json.RawMessage, 1)

	c.pendingMu.Lock()
	c.pendingReqs[id] = respChan
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pendingReqs, id)
		c.pendingMu.Unlock()
	}()

	cancelReq := map[string]interface{}{
		"method": "order.cancel",
		"id":     id,
		"param": []map[string]interface{}{
			{
				"symbol":  symbol,
				"orderId": orderID,
			},
		},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(cancelReq)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to send cancel: %w", err)
	}

	// Wait for response
	select {
	case respData := <-respChan:
		var resp struct {
			Code    int    `json:"code"`
			Success bool   `json:"success"`
			Msg     string `json:"msg"`
		}
		if err := json.Unmarshal(respData, &resp); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
		if resp.Code != 0 && !resp.Success {
			return fmt.Errorf("cancel failed: %s", resp.Msg)
		}
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("cancel request timeout")
	case <-c.ctx.Done():
		return fmt.Errorf("context canceled")
	}
}

// CancelAllOrders cancels all orders for a symbol via WebSocket
func (c *TradingWSClient) CancelAllOrders(symbol string) error {
	if atomic.LoadInt32(&c.authenticated) != 1 {
		return fmt.Errorf("not authenticated")
	}

	id := atomic.AddInt64(&c.requestID, 1)
	respChan := make(chan json.RawMessage, 1)

	c.pendingMu.Lock()
	c.pendingReqs[id] = respChan
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pendingReqs, id)
		c.pendingMu.Unlock()
	}()

	cancelReq := map[string]interface{}{
		"method": "order.cancel.all",
		"id":     id,
		"param": map[string]interface{}{
			"symbol": symbol,
		},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(cancelReq)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to send cancel all: %w", err)
	}

	// Wait for response
	select {
	case respData := <-respChan:
		var resp struct {
			Code    int    `json:"code"`
			Success bool   `json:"success"`
			Msg     string `json:"msg"`
		}
		if err := json.Unmarshal(respData, &resp); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
		if resp.Code != 0 && !resp.Success {
			return fmt.Errorf("cancel all failed: %s", resp.Msg)
		}
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("cancel all request timeout")
	case <-c.ctx.Done():
		return fmt.Errorf("context canceled")
	}
}

// SubscribeOrders subscribes to personal order updates
func (c *TradingWSClient) SubscribeOrders() error {
	if atomic.LoadInt32(&c.authenticated) != 1 {
		return fmt.Errorf("not authenticated")
	}

	req := map[string]interface{}{
		"method": "sub.personal.order",
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to subscribe orders: %w", err)
	}

	return nil
}

// IsAuthenticated returns true if WebSocket is authenticated
func (c *TradingWSClient) IsAuthenticated() bool {
	return atomic.LoadInt32(&c.authenticated) == 1
}

// IsConnected returns true if WebSocket is connected
func (c *TradingWSClient) IsConnected() bool {
	return c.conn != nil
}

// Helper function
func boolToInt32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// PlaceOrderRequest for WebSocket order placement
type PlaceOrderRequest struct {
	Symbol      string `json:"symbol"`
	Price       string `json:"price"`
	Volume      string `json:"vol"`
	Leverage    int    `json:"leverage,omitempty"`
	Side        int    `json:"side"`        // 1=open_long, 2=close_short, 3=open_short, 4=close_long
	OrderType   int    `json:"type"`        // 1=limit, 5=market
	OpenType    int    `json:"openType"`    // 1=isolated, 2=cross
	ExternalOID string `json:"externalOid"` // Client order ID
}
