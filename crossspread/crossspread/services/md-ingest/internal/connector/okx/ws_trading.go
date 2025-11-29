// Package okx provides WebSocket trading client for low-latency order operations.
package okx

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

// WebSocket trading endpoint
const (
	WSPrivateURL     = "wss://ws.okx.com:8443/ws/v5/private"
	WSPrivateDemoURL = "wss://wspap.okx.com:8443/ws/v5/private"
)

// Trading operations
const (
	OpOrder             = "order"
	OpBatchOrders       = "batch-orders"
	OpCancelOrder       = "cancel-order"
	OpCancelBatchOrders = "batch-cancel-orders"
	OpAmendOrder        = "amend-order"
	OpAmendBatchOrders  = "batch-amend-orders"
)

// TradingHandler handles trading responses
type TradingHandler interface {
	OnOrderResult(id string, result *OrderResult)
	OnBatchOrderResult(id string, results []OrderResult)
	OnCancelResult(id string, result *CancelResult)
	OnBatchCancelResult(id string, results []CancelResult)
	OnAmendResult(id string, result *AmendResult)
	OnError(id string, err error)
	OnConnected()
	OnDisconnected()
	OnAuthenticated()
}

// TradingWSClient handles WebSocket trading operations
type TradingWSClient struct {
	url        string
	apiKey     string
	secretKey  string
	passphrase string
	demoMode   bool

	conn    *websocket.Conn
	handler TradingHandler

	writeMu sync.Mutex
	done    chan struct{}
	wg      sync.WaitGroup

	requestID uint64

	// Pending requests waiting for response
	pending   map[string]chan *WSTradeResponse
	pendingMu sync.RWMutex

	reconnect     bool
	reconnectWait time.Duration
	maxReconnect  int

	pingInterval  time.Duration
	authenticated bool
	authMu        sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

// TradingWSConfig holds configuration for trading WebSocket client
type TradingWSConfig struct {
	APIKey        string
	SecretKey     string
	Passphrase    string
	DemoMode      bool
	Handler       TradingHandler
	PingInterval  time.Duration
	ReconnectWait time.Duration
	MaxReconnect  int
}

// NewTradingWSClient creates a new trading WebSocket client
func NewTradingWSClient(cfg TradingWSConfig) *TradingWSClient {
	url := WSPrivateURL
	if cfg.DemoMode {
		url = WSPrivateDemoURL
	}

	if cfg.PingInterval == 0 {
		cfg.PingInterval = 25 * time.Second
	}
	if cfg.ReconnectWait == 0 {
		cfg.ReconnectWait = 5 * time.Second
	}
	if cfg.MaxReconnect == 0 {
		cfg.MaxReconnect = 3
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TradingWSClient{
		url:           url,
		apiKey:        cfg.APIKey,
		secretKey:     cfg.SecretKey,
		passphrase:    cfg.Passphrase,
		demoMode:      cfg.DemoMode,
		handler:       cfg.Handler,
		done:          make(chan struct{}),
		pending:       make(map[string]chan *WSTradeResponse),
		reconnect:     true,
		reconnectWait: cfg.ReconnectWait,
		maxReconnect:  cfg.MaxReconnect,
		pingInterval:  cfg.PingInterval,
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
	c.authenticated = false

	// Start read loop
	c.wg.Add(2)
	go c.readLoop()
	go c.pingLoop()

	if c.handler != nil {
		c.handler.OnConnected()
	}

	// Authenticate
	if err := c.authenticate(); err != nil {
		c.Close()
		return fmt.Errorf("authentication failed: %w", err)
	}

	return nil
}

// authenticate sends login request
func (c *TradingWSClient) authenticate() error {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	// Sign: timestamp + "GET" + "/users/self/verify"
	message := timestamp + "GET" + "/users/self/verify"
	h := hmac.New(sha256.New, []byte(c.secretKey))
	h.Write([]byte(message))
	sign := base64.StdEncoding.EncodeToString(h.Sum(nil))

	loginArg := WSLoginArg{
		APIKey:     c.apiKey,
		Passphrase: c.passphrase,
		Timestamp:  timestamp,
		Sign:       sign,
	}

	req := WSRequest{
		Op:   "login",
		Args: []interface{}{loginArg},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to send login: %w", err)
	}

	// Wait for authentication response (handled in readLoop)
	// The handler.OnAuthenticated() will be called upon successful login
	return nil
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

// IsAuthenticated returns whether the client is authenticated
func (c *TradingWSClient) IsAuthenticated() bool {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return c.authenticated
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
						c.handler.OnError("", fmt.Errorf("WebSocket read error: %w", err))
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
					c.handler.OnError("", fmt.Errorf("ping error: %w", err))
				}
				return
			}
		}
	}
}

// handleDisconnect handles disconnection
func (c *TradingWSClient) handleDisconnect() {
	c.authMu.Lock()
	c.authenticated = false
	c.authMu.Unlock()

	// Clear pending requests
	c.pendingMu.Lock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()

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
				c.handler.OnError("", fmt.Errorf("reconnect attempt %d failed: %w", i+1, err))
			}
			continue
		}
		return
	}

	if c.handler != nil {
		c.handler.OnError("", fmt.Errorf("max reconnection attempts reached"))
	}
}

// handleMessage processes incoming WebSocket messages
func (c *TradingWSClient) handleMessage(data []byte) {
	// Handle pong response
	if string(data) == "pong" {
		return
	}

	// Try to parse as event response first
	var eventResp struct {
		Event string `json:"event"`
		Code  string `json:"code"`
		Msg   string `json:"msg"`
	}
	if err := json.Unmarshal(data, &eventResp); err == nil && eventResp.Event != "" {
		c.handleEventResponse(eventResp.Event, eventResp.Code, eventResp.Msg)
		return
	}

	// Parse as trade response
	var resp WSTradeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		if c.handler != nil {
			c.handler.OnError("", fmt.Errorf("failed to unmarshal message: %w", err))
		}
		return
	}

	// Route to pending request or handle directly
	c.pendingMu.RLock()
	ch, exists := c.pending[resp.ID]
	c.pendingMu.RUnlock()

	if exists {
		select {
		case ch <- &resp:
		default:
		}
		c.pendingMu.Lock()
		delete(c.pending, resp.ID)
		c.pendingMu.Unlock()
	} else {
		// Handle via callback
		c.handleTradeResponse(&resp)
	}
}

// handleEventResponse handles event-type responses
func (c *TradingWSClient) handleEventResponse(event, code, msg string) {
	switch event {
	case "login":
		if code == "0" {
			c.authMu.Lock()
			c.authenticated = true
			c.authMu.Unlock()
			if c.handler != nil {
				c.handler.OnAuthenticated()
			}
		} else {
			if c.handler != nil {
				c.handler.OnError("login", fmt.Errorf("login failed: %s - %s", code, msg))
			}
		}
	case "error":
		if c.handler != nil {
			c.handler.OnError("", fmt.Errorf("error: %s - %s", code, msg))
		}
	}
}

// handleTradeResponse handles trade operation responses
func (c *TradingWSClient) handleTradeResponse(resp *WSTradeResponse) {
	if c.handler == nil {
		return
	}

	// Check for error
	if resp.Code != "0" {
		c.handler.OnError(resp.ID, &APIError{Code: resp.Code, Message: resp.Msg})
		return
	}

	switch resp.Op {
	case OpOrder:
		var results []OrderResult
		if err := json.Unmarshal(resp.Data, &results); err != nil {
			c.handler.OnError(resp.ID, fmt.Errorf("failed to unmarshal order result: %w", err))
			return
		}
		if len(results) > 0 {
			c.handler.OnOrderResult(resp.ID, &results[0])
		}

	case OpBatchOrders:
		var results []OrderResult
		if err := json.Unmarshal(resp.Data, &results); err != nil {
			c.handler.OnError(resp.ID, fmt.Errorf("failed to unmarshal batch order result: %w", err))
			return
		}
		c.handler.OnBatchOrderResult(resp.ID, results)

	case OpCancelOrder:
		var results []CancelResult
		if err := json.Unmarshal(resp.Data, &results); err != nil {
			c.handler.OnError(resp.ID, fmt.Errorf("failed to unmarshal cancel result: %w", err))
			return
		}
		if len(results) > 0 {
			c.handler.OnCancelResult(resp.ID, &results[0])
		}

	case OpCancelBatchOrders:
		var results []CancelResult
		if err := json.Unmarshal(resp.Data, &results); err != nil {
			c.handler.OnError(resp.ID, fmt.Errorf("failed to unmarshal batch cancel result: %w", err))
			return
		}
		c.handler.OnBatchCancelResult(resp.ID, results)

	case OpAmendOrder:
		var results []AmendResult
		if err := json.Unmarshal(resp.Data, &results); err != nil {
			c.handler.OnError(resp.ID, fmt.Errorf("failed to unmarshal amend result: %w", err))
			return
		}
		if len(results) > 0 {
			c.handler.OnAmendResult(resp.ID, &results[0])
		}
	}
}

// nextRequestID generates a unique request ID
func (c *TradingWSClient) nextRequestID() string {
	return strconv.FormatUint(atomic.AddUint64(&c.requestID, 1), 10)
}

// sendRequest sends a trading request
func (c *TradingWSClient) sendRequest(op string, args []interface{}) (string, error) {
	if !c.IsAuthenticated() {
		return "", fmt.Errorf("not authenticated")
	}

	id := c.nextRequestID()
	req := WSRequestWithID{
		ID:   id,
		Op:   op,
		Args: args,
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}

	return id, nil
}

// sendRequestSync sends a request and waits for response
func (c *TradingWSClient) sendRequestSync(ctx context.Context, op string, args []interface{}) (*WSTradeResponse, error) {
	if !c.IsAuthenticated() {
		return nil, fmt.Errorf("not authenticated")
	}

	id := c.nextRequestID()

	// Create response channel
	respChan := make(chan *WSTradeResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = respChan
	c.pendingMu.Unlock()

	// Clean up on exit
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	req := WSRequestWithID{
		ID:   id,
		Op:   op,
		Args: args,
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respChan:
		if resp == nil {
			return nil, fmt.Errorf("connection closed")
		}
		return resp, nil
	}
}

// =============================================================================
// Trading Operations
// =============================================================================

// PlaceOrder places a single order via WebSocket (async)
func (c *TradingWSClient) PlaceOrder(req *PlaceOrderRequest) (string, error) {
	return c.sendRequest(OpOrder, []interface{}{req})
}

// PlaceOrderSync places a single order and waits for response
func (c *TradingWSClient) PlaceOrderSync(ctx context.Context, req *PlaceOrderRequest) (*OrderResult, error) {
	resp, err := c.sendRequestSync(ctx, OpOrder, []interface{}{req})
	if err != nil {
		return nil, err
	}

	if resp.Code != "0" {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	var results []OrderResult
	if err := json.Unmarshal(resp.Data, &results); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no order result returned")
	}

	result := &results[0]
	if !IsSuccess(result.SCode) {
		return nil, &APIError{Code: result.SCode, Message: result.SMsg}
	}

	return result, nil
}

// PlaceBatchOrders places multiple orders via WebSocket (async)
func (c *TradingWSClient) PlaceBatchOrders(orders []*PlaceOrderRequest) (string, error) {
	if len(orders) > 20 {
		return "", fmt.Errorf("max 20 orders per batch")
	}

	args := make([]interface{}, len(orders))
	for i, order := range orders {
		args[i] = order
	}

	return c.sendRequest(OpBatchOrders, args)
}

// PlaceBatchOrdersSync places multiple orders and waits for response
func (c *TradingWSClient) PlaceBatchOrdersSync(ctx context.Context, orders []*PlaceOrderRequest) ([]OrderResult, error) {
	if len(orders) > 20 {
		return nil, fmt.Errorf("max 20 orders per batch")
	}

	args := make([]interface{}, len(orders))
	for i, order := range orders {
		args[i] = order
	}

	resp, err := c.sendRequestSync(ctx, OpBatchOrders, args)
	if err != nil {
		return nil, err
	}

	if resp.Code != "0" {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	var results []OrderResult
	if err := json.Unmarshal(resp.Data, &results); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return results, nil
}

// CancelOrder cancels an order via WebSocket (async)
func (c *TradingWSClient) CancelOrder(req *CancelOrderRequest) (string, error) {
	return c.sendRequest(OpCancelOrder, []interface{}{req})
}

// CancelOrderSync cancels an order and waits for response
func (c *TradingWSClient) CancelOrderSync(ctx context.Context, req *CancelOrderRequest) (*CancelResult, error) {
	resp, err := c.sendRequestSync(ctx, OpCancelOrder, []interface{}{req})
	if err != nil {
		return nil, err
	}

	if resp.Code != "0" {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	var results []CancelResult
	if err := json.Unmarshal(resp.Data, &results); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no cancel result returned")
	}

	result := &results[0]
	if !IsSuccess(result.SCode) {
		return nil, &APIError{Code: result.SCode, Message: result.SMsg}
	}

	return result, nil
}

// CancelBatchOrders cancels multiple orders via WebSocket (async)
func (c *TradingWSClient) CancelBatchOrders(orders []*CancelOrderRequest) (string, error) {
	if len(orders) > 20 {
		return "", fmt.Errorf("max 20 orders per batch")
	}

	args := make([]interface{}, len(orders))
	for i, order := range orders {
		args[i] = order
	}

	return c.sendRequest(OpCancelBatchOrders, args)
}

// CancelBatchOrdersSync cancels multiple orders and waits for response
func (c *TradingWSClient) CancelBatchOrdersSync(ctx context.Context, orders []*CancelOrderRequest) ([]CancelResult, error) {
	if len(orders) > 20 {
		return nil, fmt.Errorf("max 20 orders per batch")
	}

	args := make([]interface{}, len(orders))
	for i, order := range orders {
		args[i] = order
	}

	resp, err := c.sendRequestSync(ctx, OpCancelBatchOrders, args)
	if err != nil {
		return nil, err
	}

	if resp.Code != "0" {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	var results []CancelResult
	if err := json.Unmarshal(resp.Data, &results); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return results, nil
}

// AmendOrder modifies an order via WebSocket (async)
func (c *TradingWSClient) AmendOrder(req *AmendOrderRequest) (string, error) {
	return c.sendRequest(OpAmendOrder, []interface{}{req})
}

// AmendOrderSync modifies an order and waits for response
func (c *TradingWSClient) AmendOrderSync(ctx context.Context, req *AmendOrderRequest) (*AmendResult, error) {
	resp, err := c.sendRequestSync(ctx, OpAmendOrder, []interface{}{req})
	if err != nil {
		return nil, err
	}

	if resp.Code != "0" {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	var results []AmendResult
	if err := json.Unmarshal(resp.Data, &results); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no amend result returned")
	}

	result := &results[0]
	if !IsSuccess(result.SCode) {
		return nil, &APIError{Code: result.SCode, Message: result.SMsg}
	}

	return result, nil
}

// =============================================================================
// Helper Methods for Quick Order Placement
// =============================================================================

// PlaceLimitOrderSync places a limit order synchronously
func (c *TradingWSClient) PlaceLimitOrderSync(ctx context.Context, instID, side, sz, px, tdMode, posSide string) (*OrderResult, error) {
	req := &PlaceOrderRequest{
		InstID:  instID,
		TdMode:  tdMode,
		Side:    side,
		OrdType: OrdTypeLimit,
		Sz:      sz,
		Px:      px,
		PosSide: posSide,
	}
	return c.PlaceOrderSync(ctx, req)
}

// PlaceMarketOrderSync places a market order synchronously
func (c *TradingWSClient) PlaceMarketOrderSync(ctx context.Context, instID, side, sz, tdMode, posSide string) (*OrderResult, error) {
	req := &PlaceOrderRequest{
		InstID:  instID,
		TdMode:  tdMode,
		Side:    side,
		OrdType: OrdTypeMarket,
		Sz:      sz,
		PosSide: posSide,
	}
	return c.PlaceOrderSync(ctx, req)
}

// CancelOrderByIDSync cancels an order by order ID synchronously
func (c *TradingWSClient) CancelOrderByIDSync(ctx context.Context, instID, orderID string) (*CancelResult, error) {
	req := &CancelOrderRequest{
		InstID: instID,
		OrdID:  orderID,
	}
	return c.CancelOrderSync(ctx, req)
}

// CancelOrderByClientIDSync cancels an order by client order ID synchronously
func (c *TradingWSClient) CancelOrderByClientIDSync(ctx context.Context, instID, clOrdID string) (*CancelResult, error) {
	req := &CancelOrderRequest{
		InstID:  instID,
		ClOrdID: clOrdID,
	}
	return c.CancelOrderSync(ctx, req)
}

// =============================================================================
// Emergency Exit Helper
// =============================================================================

// EmergencyExitPosition places an aggressive market order to exit position
// For arbitrage emergency exit: use market orders to close quickly
func (c *TradingWSClient) EmergencyExitPosition(ctx context.Context, instID, side, sz, tdMode, posSide string) (*OrderResult, error) {
	req := &PlaceOrderRequest{
		InstID:     instID,
		TdMode:     tdMode,
		Side:       side,
		OrdType:    OrdTypeMarket,
		Sz:         sz,
		PosSide:    posSide,
		ReduceOnly: true, // Only reduce position, don't increase
	}
	return c.PlaceOrderSync(ctx, req)
}

// CancelAllOrders cancels all pending orders for an instrument
func (c *TradingWSClient) CancelAllOrders(ctx context.Context, instID string, orders []string) ([]CancelResult, error) {
	if len(orders) == 0 {
		return nil, nil
	}

	// Split into batches of 20
	var allResults []CancelResult
	for i := 0; i < len(orders); i += 20 {
		end := i + 20
		if end > len(orders) {
			end = len(orders)
		}

		batch := make([]*CancelOrderRequest, end-i)
		for j, orderID := range orders[i:end] {
			batch[j] = &CancelOrderRequest{
				InstID: instID,
				OrdID:  orderID,
			}
		}

		results, err := c.CancelBatchOrdersSync(ctx, batch)
		if err != nil {
			return allResults, err
		}
		allResults = append(allResults, results...)
	}

	return allResults, nil
}
