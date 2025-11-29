package gate

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// WSTradingHandler handles trading callbacks
type WSTradingHandler struct {
	OnOrderPlaced   func(reqID string, order *Order, err error)
	OnOrderCanceled func(reqID string, order *Order, err error)
	OnOrderAmended  func(reqID string, order *Order, err error)
	OnOrderBatch    func(reqID string, orders []*Order, err error)
	OnError         func(err error)
	OnConnect       func(settle string)
	OnDisconnect    func(settle string, err error)
	OnLogin         func(settle string, success bool, err error)
}

// WSTradingClient handles WebSocket trading operations
type WSTradingClient struct {
	baseURL         string
	apiKey          string
	apiSecret       string
	handler         *WSTradingHandler
	connections     map[string]*wsTradingConnection // settle -> connection
	pendingRequests map[string]*pendingRequest      // reqID -> pending
	mu              sync.RWMutex
	reqMu           sync.Mutex
	reconnectDelay  time.Duration
	maxRetries      int
	requestTimeout  time.Duration
	reqIDCounter    uint64
	ctx             context.Context
	cancel          context.CancelFunc
}

// wsTradingConnection represents a trading WebSocket connection
type wsTradingConnection struct {
	conn        *websocket.Conn
	settle      string
	mu          sync.Mutex
	writeMu     sync.Mutex
	isConnected bool
	isLoggedIn  bool
	stopPing    chan struct{}
}

// pendingRequest tracks a pending trading request
type pendingRequest struct {
	reqID    string
	channel  string
	response chan *WSMessage
	created  time.Time
}

// NewWSTradingClient creates a new WebSocket trading client
func NewWSTradingClient(baseURL, apiKey, apiSecret string, handler *WSTradingHandler) *WSTradingClient {
	if baseURL == "" {
		baseURL = "wss://fx-ws.gateio.ws/v4/ws"
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &WSTradingClient{
		baseURL:         baseURL,
		apiKey:          apiKey,
		apiSecret:       apiSecret,
		handler:         handler,
		connections:     make(map[string]*wsTradingConnection),
		pendingRequests: make(map[string]*pendingRequest),
		reconnectDelay:  3 * time.Second,
		maxRetries:      10,
		requestTimeout:  10 * time.Second,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Connect establishes WebSocket connection and authenticates
func (c *WSTradingClient) Connect(settle string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if conn, exists := c.connections[settle]; exists && conn.isConnected {
		return nil
	}

	return c.connectInternal(settle)
}

func (c *WSTradingClient) connectInternal(settle string) error {
	url := fmt.Sprintf("%s/%s", c.baseURL, settle)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(c.ctx, url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Gate.io trading WS (%s): %w", settle, err)
	}

	wsConn := &wsTradingConnection{
		conn:        conn,
		settle:      settle,
		isConnected: true,
		stopPing:    make(chan struct{}),
	}
	c.connections[settle] = wsConn

	// Start message handler
	go c.readLoop(wsConn)

	// Start ping loop
	go c.pingLoop(wsConn)

	if c.handler != nil && c.handler.OnConnect != nil {
		c.handler.OnConnect(settle)
	}

	log.Printf("[Gate.io Trading WS] Connected to %s", settle)

	// Authenticate if credentials are provided
	if c.apiKey != "" && c.apiSecret != "" {
		if err := c.login(settle); err != nil {
			log.Printf("[Gate.io Trading WS] Login failed for %s: %v", settle, err)
			return err
		}
	}

	return nil
}

// login authenticates the WebSocket connection
func (c *WSTradingClient) login(settle string) error {
	timestamp := time.Now().Unix()
	timestampStr := strconv.FormatInt(timestamp, 10)

	// Sign: channel + event + timestamp
	signPayload := fmt.Sprintf("channel=%s&event=%s&time=%s", "futures.login", "api", timestampStr)
	signature := c.sign(signPayload)

	loginReq := WSLoginRequest{
		Time:    timestamp,
		Channel: "futures.login",
		Event:   "api",
		Payload: WSLoginPayload{
			APIKey:    c.apiKey,
			Signature: signature,
			Timestamp: timestampStr,
			ReqID:     c.generateReqID(),
		},
	}

	c.mu.RLock()
	wsConn, ok := c.connections[settle]
	c.mu.RUnlock()

	if !ok || !wsConn.isConnected {
		return fmt.Errorf("not connected to %s", settle)
	}

	data, err := json.Marshal(loginReq)
	if err != nil {
		return err
	}

	wsConn.writeMu.Lock()
	err = wsConn.conn.WriteMessage(websocket.TextMessage, data)
	wsConn.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to send login: %w", err)
	}

	// Wait for login response (handled in readLoop)
	// For now, we'll assume login succeeds if no immediate error
	time.Sleep(500 * time.Millisecond)

	wsConn.mu.Lock()
	wsConn.isLoggedIn = true
	wsConn.mu.Unlock()

	log.Printf("[Gate.io Trading WS] Logged in to %s", settle)

	if c.handler != nil && c.handler.OnLogin != nil {
		c.handler.OnLogin(settle, true, nil)
	}

	return nil
}

// sign generates HMAC-SHA512 signature
func (c *WSTradingClient) sign(payload string) string {
	h := hmac.New(sha512.New, []byte(c.apiSecret))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

// generateReqID generates unique request ID
func (c *WSTradingClient) generateReqID() string {
	id := atomic.AddUint64(&c.reqIDCounter, 1)
	return fmt.Sprintf("t-%d-%d", time.Now().UnixNano(), id)
}

// readLoop reads messages from WebSocket
func (c *WSTradingClient) readLoop(wsConn *wsTradingConnection) {
	defer func() {
		wsConn.mu.Lock()
		wsConn.isConnected = false
		wsConn.isLoggedIn = false
		wsConn.mu.Unlock()
		close(wsConn.stopPing)

		if c.handler != nil && c.handler.OnDisconnect != nil {
			c.handler.OnDisconnect(wsConn.settle, nil)
		}
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		_, message, err := wsConn.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[Gate.io Trading WS] Read error (%s): %v", wsConn.settle, err)
			}
			c.handleReconnect(wsConn.settle)
			return
		}

		c.handleMessage(wsConn.settle, message)
	}
}

// pingLoop sends periodic pings
func (c *WSTradingClient) pingLoop(wsConn *wsTradingConnection) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wsConn.stopPing:
			return
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			wsConn.writeMu.Lock()
			err := wsConn.conn.WriteMessage(websocket.PingMessage, nil)
			wsConn.writeMu.Unlock()
			if err != nil {
				log.Printf("[Gate.io Trading WS] Ping error (%s): %v", wsConn.settle, err)
				return
			}
		}
	}
}

// handleMessage processes incoming messages
func (c *WSTradingClient) handleMessage(settle string, data []byte) {
	var msg WSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[Gate.io Trading WS] Failed to parse message: %v", err)
		return
	}

	// Check if this is a response to a pending request
	if msg.ReqID != "" {
		c.handleRequestResponse(&msg)
		return
	}

	// Handle channel-specific messages
	switch msg.Channel {
	case "futures.login":
		c.handleLoginResponse(settle, &msg)
	case "futures.order_place":
		c.handleOrderPlaceResponse(&msg)
	case "futures.order_cancel":
		c.handleOrderCancelResponse(&msg)
	case "futures.order_amend":
		c.handleOrderAmendResponse(&msg)
	case "futures.order_batch_place":
		c.handleOrderBatchResponse(&msg)
	default:
		log.Printf("[Gate.io Trading WS] Unhandled channel: %s", msg.Channel)
	}
}

func (c *WSTradingClient) handleLoginResponse(settle string, msg *WSMessage) {
	c.mu.RLock()
	wsConn, ok := c.connections[settle]
	c.mu.RUnlock()

	if !ok {
		return
	}

	if msg.Error != nil {
		log.Printf("[Gate.io Trading WS] Login failed: %s", msg.Error.Message)
		if c.handler != nil && c.handler.OnLogin != nil {
			c.handler.OnLogin(settle, false, fmt.Errorf(msg.Error.Message))
		}
		return
	}

	wsConn.mu.Lock()
	wsConn.isLoggedIn = true
	wsConn.mu.Unlock()

	log.Printf("[Gate.io Trading WS] Login successful for %s", settle)
	if c.handler != nil && c.handler.OnLogin != nil {
		c.handler.OnLogin(settle, true, nil)
	}
}

func (c *WSTradingClient) handleRequestResponse(msg *WSMessage) {
	c.reqMu.Lock()
	pending, ok := c.pendingRequests[msg.ReqID]
	if ok {
		delete(c.pendingRequests, msg.ReqID)
	}
	c.reqMu.Unlock()

	if ok && pending.response != nil {
		select {
		case pending.response <- msg:
		default:
		}
	}
}

func (c *WSTradingClient) handleOrderPlaceResponse(msg *WSMessage) {
	if c.handler == nil || c.handler.OnOrderPlaced == nil {
		return
	}

	if msg.Error != nil {
		c.handler.OnOrderPlaced(msg.ReqID, nil, fmt.Errorf(msg.Error.Message))
		return
	}

	var order Order
	if err := json.Unmarshal(msg.Result, &order); err != nil {
		c.handler.OnOrderPlaced(msg.ReqID, nil, err)
		return
	}

	c.handler.OnOrderPlaced(msg.ReqID, &order, nil)
}

func (c *WSTradingClient) handleOrderCancelResponse(msg *WSMessage) {
	if c.handler == nil || c.handler.OnOrderCanceled == nil {
		return
	}

	if msg.Error != nil {
		c.handler.OnOrderCanceled(msg.ReqID, nil, fmt.Errorf(msg.Error.Message))
		return
	}

	var order Order
	if err := json.Unmarshal(msg.Result, &order); err != nil {
		c.handler.OnOrderCanceled(msg.ReqID, nil, err)
		return
	}

	c.handler.OnOrderCanceled(msg.ReqID, &order, nil)
}

func (c *WSTradingClient) handleOrderAmendResponse(msg *WSMessage) {
	if c.handler == nil || c.handler.OnOrderAmended == nil {
		return
	}

	if msg.Error != nil {
		c.handler.OnOrderAmended(msg.ReqID, nil, fmt.Errorf(msg.Error.Message))
		return
	}

	var order Order
	if err := json.Unmarshal(msg.Result, &order); err != nil {
		c.handler.OnOrderAmended(msg.ReqID, nil, err)
		return
	}

	c.handler.OnOrderAmended(msg.ReqID, &order, nil)
}

func (c *WSTradingClient) handleOrderBatchResponse(msg *WSMessage) {
	if c.handler == nil || c.handler.OnOrderBatch == nil {
		return
	}

	if msg.Error != nil {
		c.handler.OnOrderBatch(msg.ReqID, nil, fmt.Errorf(msg.Error.Message))
		return
	}

	var orders []*Order
	if err := json.Unmarshal(msg.Result, &orders); err != nil {
		c.handler.OnOrderBatch(msg.ReqID, nil, err)
		return
	}

	c.handler.OnOrderBatch(msg.ReqID, orders, nil)
}

// handleReconnect handles reconnection after disconnection
func (c *WSTradingClient) handleReconnect(settle string) {
	c.mu.Lock()
	delete(c.connections, settle)
	c.mu.Unlock()

	for retry := 0; retry < c.maxRetries; retry++ {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(c.reconnectDelay):
		}

		log.Printf("[Gate.io Trading WS] Reconnecting to %s (attempt %d/%d)", settle, retry+1, c.maxRetries)

		c.mu.Lock()
		err := c.connectInternal(settle)
		c.mu.Unlock()

		if err == nil {
			return
		}
		log.Printf("[Gate.io Trading WS] Reconnect failed: %v", err)
	}

	log.Printf("[Gate.io Trading WS] Max reconnect attempts reached for %s", settle)
	if c.handler != nil && c.handler.OnError != nil {
		c.handler.OnError(fmt.Errorf("max reconnect attempts reached for %s", settle))
	}
}

// sendTradingRequest sends a trading request and waits for response
func (c *WSTradingClient) sendTradingRequest(settle string, channel string, payload interface{}) (*WSMessage, error) {
	c.mu.RLock()
	wsConn, ok := c.connections[settle]
	c.mu.RUnlock()

	if !ok || !wsConn.isConnected {
		return nil, fmt.Errorf("not connected to %s", settle)
	}

	if !wsConn.isLoggedIn {
		return nil, fmt.Errorf("not logged in to %s", settle)
	}

	reqID := c.generateReqID()

	// Create pending request
	pending := &pendingRequest{
		reqID:    reqID,
		channel:  channel,
		response: make(chan *WSMessage, 1),
		created:  time.Now(),
	}

	c.reqMu.Lock()
	c.pendingRequests[reqID] = pending
	c.reqMu.Unlock()

	defer func() {
		c.reqMu.Lock()
		delete(c.pendingRequests, reqID)
		c.reqMu.Unlock()
	}()

	// Build request message
	msg := map[string]interface{}{
		"time":    time.Now().Unix(),
		"channel": channel,
		"event":   "api",
		"payload": payload,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	wsConn.writeMu.Lock()
	err = wsConn.conn.WriteMessage(websocket.TextMessage, data)
	wsConn.writeMu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response
	select {
	case resp := <-pending.response:
		if resp.Error != nil {
			return nil, fmt.Errorf("API error [%d]: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	case <-time.After(c.requestTimeout):
		return nil, fmt.Errorf("request timeout")
	case <-c.ctx.Done():
		return nil, c.ctx.Err()
	}
}

// PlaceOrder places a new order via WebSocket
func (c *WSTradingClient) PlaceOrder(settle string, req *OrderRequest) (*Order, error) {
	payload := WSOrderPlacePayload{
		ReqID:    c.generateReqID(),
		ReqParam: *req,
	}

	resp, err := c.sendTradingRequest(settle, "futures.order_place", payload)
	if err != nil {
		return nil, err
	}

	var order Order
	if err := json.Unmarshal(resp.Result, &order); err != nil {
		return nil, err
	}

	return &order, nil
}

// PlaceOrderAsync places order asynchronously
func (c *WSTradingClient) PlaceOrderAsync(settle string, req *OrderRequest) (string, error) {
	c.mu.RLock()
	wsConn, ok := c.connections[settle]
	c.mu.RUnlock()

	if !ok || !wsConn.isConnected {
		return "", fmt.Errorf("not connected to %s", settle)
	}

	if !wsConn.isLoggedIn {
		return "", fmt.Errorf("not logged in to %s", settle)
	}

	reqID := c.generateReqID()

	msg := WSOrderPlaceRequest{
		Time:    time.Now().Unix(),
		Channel: "futures.order_place",
		Event:   "api",
		Payload: WSOrderPlacePayload{
			ReqID:    reqID,
			ReqParam: *req,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}

	wsConn.writeMu.Lock()
	err = wsConn.conn.WriteMessage(websocket.TextMessage, data)
	wsConn.writeMu.Unlock()

	if err != nil {
		return "", fmt.Errorf("failed to send order: %w", err)
	}

	return reqID, nil
}

// CancelOrder cancels an order via WebSocket
func (c *WSTradingClient) CancelOrder(settle string, orderID string) (*Order, error) {
	payload := WSOrderCancelPayload{
		ReqID: c.generateReqID(),
		ReqParam: WSOrderCancelReqParam{
			OrderID: orderID,
		},
	}

	resp, err := c.sendTradingRequest(settle, "futures.order_cancel", payload)
	if err != nil {
		return nil, err
	}

	var order Order
	if err := json.Unmarshal(resp.Result, &order); err != nil {
		return nil, err
	}

	return &order, nil
}

// CancelOrderAsync cancels order asynchronously
func (c *WSTradingClient) CancelOrderAsync(settle string, orderID string) (string, error) {
	c.mu.RLock()
	wsConn, ok := c.connections[settle]
	c.mu.RUnlock()

	if !ok || !wsConn.isConnected {
		return "", fmt.Errorf("not connected to %s", settle)
	}

	if !wsConn.isLoggedIn {
		return "", fmt.Errorf("not logged in to %s", settle)
	}

	reqID := c.generateReqID()

	msg := WSOrderCancelRequest{
		Time:    time.Now().Unix(),
		Channel: "futures.order_cancel",
		Event:   "api",
		Payload: WSOrderCancelPayload{
			ReqID: reqID,
			ReqParam: WSOrderCancelReqParam{
				OrderID: orderID,
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}

	wsConn.writeMu.Lock()
	err = wsConn.conn.WriteMessage(websocket.TextMessage, data)
	wsConn.writeMu.Unlock()

	if err != nil {
		return "", fmt.Errorf("failed to send cancel: %w", err)
	}

	return reqID, nil
}

// AmendOrder amends an existing order via WebSocket
func (c *WSTradingClient) AmendOrder(settle string, orderID string, price string, size int64) (*Order, error) {
	payload := WSOrderAmendPayload{
		ReqID: c.generateReqID(),
		ReqParam: WSOrderAmendReqParam{
			OrderID: orderID,
			Price:   price,
			Size:    size,
		},
	}

	resp, err := c.sendTradingRequest(settle, "futures.order_amend", payload)
	if err != nil {
		return nil, err
	}

	var order Order
	if err := json.Unmarshal(resp.Result, &order); err != nil {
		return nil, err
	}

	return &order, nil
}

// AmendOrderAsync amends order asynchronously
func (c *WSTradingClient) AmendOrderAsync(settle string, orderID string, price string, size int64) (string, error) {
	c.mu.RLock()
	wsConn, ok := c.connections[settle]
	c.mu.RUnlock()

	if !ok || !wsConn.isConnected {
		return "", fmt.Errorf("not connected to %s", settle)
	}

	if !wsConn.isLoggedIn {
		return "", fmt.Errorf("not logged in to %s", settle)
	}

	reqID := c.generateReqID()

	msg := WSOrderAmendRequest{
		Time:    time.Now().Unix(),
		Channel: "futures.order_amend",
		Event:   "api",
		Payload: WSOrderAmendPayload{
			ReqID: reqID,
			ReqParam: WSOrderAmendReqParam{
				OrderID: orderID,
				Price:   price,
				Size:    size,
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}

	wsConn.writeMu.Lock()
	err = wsConn.conn.WriteMessage(websocket.TextMessage, data)
	wsConn.writeMu.Unlock()

	if err != nil {
		return "", fmt.Errorf("failed to send amend: %w", err)
	}

	return reqID, nil
}

// PlaceBatchOrders places multiple orders in a single request
func (c *WSTradingClient) PlaceBatchOrders(settle string, orders []*OrderRequest) ([]*Order, error) {
	payload := map[string]interface{}{
		"req_id":    c.generateReqID(),
		"req_param": orders,
	}

	resp, err := c.sendTradingRequest(settle, "futures.order_batch_place", payload)
	if err != nil {
		return nil, err
	}

	var result []*Order
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// CancelBatchOrders cancels multiple orders
func (c *WSTradingClient) CancelBatchOrders(settle string, orderIDs []string) error {
	payload := map[string]interface{}{
		"req_id":    c.generateReqID(),
		"req_param": orderIDs,
	}

	_, err := c.sendTradingRequest(settle, "futures.order_batch_cancel", payload)
	return err
}

// IsConnected checks if connected to a settle currency
func (c *WSTradingClient) IsConnected(settle string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if wsConn, ok := c.connections[settle]; ok {
		return wsConn.isConnected
	}
	return false
}

// IsLoggedIn checks if logged in to a settle currency
func (c *WSTradingClient) IsLoggedIn(settle string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if wsConn, ok := c.connections[settle]; ok {
		return wsConn.isLoggedIn
	}
	return false
}

// Close closes all connections
func (c *WSTradingClient) Close() error {
	c.cancel()

	c.mu.Lock()
	defer c.mu.Unlock()

	for settle, wsConn := range c.connections {
		if wsConn.conn != nil {
			wsConn.writeMu.Lock()
			wsConn.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			wsConn.conn.Close()
			wsConn.writeMu.Unlock()
		}
		delete(c.connections, settle)
	}

	// Clear pending requests
	c.reqMu.Lock()
	for reqID := range c.pendingRequests {
		delete(c.pendingRequests, reqID)
	}
	c.reqMu.Unlock()

	return nil
}

// SetRequestTimeout sets the request timeout
func (c *WSTradingClient) SetRequestTimeout(timeout time.Duration) {
	c.requestTimeout = timeout
}

// SetReconnectDelay sets the reconnection delay
func (c *WSTradingClient) SetReconnectDelay(delay time.Duration) {
	c.reconnectDelay = delay
}
