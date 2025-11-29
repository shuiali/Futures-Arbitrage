package bybit

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	// WebSocket Trade URLs for low-latency order entry
	WSTradeURLMainnet = "wss://stream.bybit.com/v5/trade"
	WSTradeURLTestnet = "wss://stream-testnet.bybit.com/v5/trade"

	// Rate limit configuration
	WSTradeRateLimitPerSecond = 10
)

// generateReqID generates a unique request ID
func generateReqID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// TradingWSOrderResponse represents the response for order operations
type TradingWSOrderResponse struct {
	Success     bool
	OrderID     string
	OrderLinkId string
	RetCode     int
	RetMsg      string
	ReqId       string
	Latency     time.Duration
}

// TradingWSCallback types
type TradingOrderCallback func(resp *TradingWSOrderResponse)
type TradingErrorCallback func(reqId string, err error)

// TradingWS handles the WebSocket trade connection for low-latency order operations
type TradingWS struct {
	url           string
	apiKey        string
	apiSecret     string
	recvWindow    int64
	conn          *websocket.Conn
	mu            sync.RWMutex
	connected     bool
	authenticated bool

	// Pending requests
	pendingRequests map[string]chan *WSTradeResponse
	pendingMu       sync.RWMutex

	// Callbacks
	onOrder TradingOrderCallback
	onError TradingErrorCallback

	// Control
	done   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
}

// TradingWSConfig holds configuration for the trading WebSocket client
type TradingWSConfig struct {
	APIKey     string
	APISecret  string
	UseTestnet bool
	RecvWindow int64 // milliseconds, default 5000
}

// NewTradingWS creates a new trading WebSocket client
func NewTradingWS(config TradingWSConfig) *TradingWS {
	url := WSTradeURLMainnet
	if config.UseTestnet {
		url = WSTradeURLTestnet
	}

	if config.RecvWindow == 0 {
		config.RecvWindow = 5000
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TradingWS{
		url:             url,
		apiKey:          config.APIKey,
		apiSecret:       config.APISecret,
		recvWindow:      config.RecvWindow,
		pendingRequests: make(map[string]chan *WSTradeResponse),
		done:            make(chan struct{}),
		ctx:             ctx,
		cancel:          cancel,
	}
}

// SetOrderCallback sets the callback for order responses
func (ws *TradingWS) SetOrderCallback(cb TradingOrderCallback) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.onOrder = cb
}

// SetErrorCallback sets the callback for errors
func (ws *TradingWS) SetErrorCallback(cb TradingErrorCallback) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.onError = cb
}

// Connect establishes WebSocket connection and authenticates
func (ws *TradingWS) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, ws.url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Bybit trade WebSocket: %w", err)
	}

	ws.mu.Lock()
	ws.conn = conn
	ws.connected = true
	ws.mu.Unlock()

	log.Info().Str("url", ws.url).Msg("Connected to Bybit trade WebSocket")

	// Start message reader
	go ws.readMessages()

	// Authenticate
	if err := ws.authenticate(); err != nil {
		ws.Disconnect()
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Start ping loop
	go ws.pingLoop()

	return nil
}

// authenticate sends authentication message
func (ws *TradingWS) authenticate() error {
	expires := time.Now().UnixMilli() + 10000 // 10 seconds from now

	// Generate signature: HMAC SHA256(api_secret, "GET/realtime" + expires)
	signData := fmt.Sprintf("GET/realtime%d", expires)
	h := hmac.New(sha256.New, []byte(ws.apiSecret))
	h.Write([]byte(signData))
	signature := hex.EncodeToString(h.Sum(nil))

	authMsg := WSAuthOperation{
		Op: "auth",
		Args: []string{
			ws.apiKey,
			strconv.FormatInt(expires, 10),
			signature,
		},
	}

	if err := ws.sendJSON(authMsg); err != nil {
		return fmt.Errorf("failed to send auth message: %w", err)
	}

	// Wait for auth response with timeout
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("authentication timeout")
		case <-ticker.C:
			ws.mu.RLock()
			authenticated := ws.authenticated
			ws.mu.RUnlock()
			if authenticated {
				log.Info().Msg("Authenticated to Bybit trade WebSocket")
				return nil
			}
		}
	}
}

// Disconnect closes the WebSocket connection
func (ws *TradingWS) Disconnect() error {
	ws.cancel()
	close(ws.done)

	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.connected = false
	ws.authenticated = false
	if ws.conn != nil {
		return ws.conn.Close()
	}
	return nil
}

// IsConnected returns connection status
func (ws *TradingWS) IsConnected() bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return ws.connected && ws.authenticated
}

// CreateOrder places a new order via WebSocket
func (ws *TradingWS) CreateOrder(ctx context.Context, req *CreateOrderRequest) (*TradingWSOrderResponse, error) {
	reqId := generateReqID()
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	startTime := time.Now()

	wsReq := WSTradeRequest{
		ReqId: reqId,
		Header: map[string]string{
			"X-BAPI-TIMESTAMP":   timestamp,
			"X-BAPI-RECV-WINDOW": strconv.FormatInt(ws.recvWindow, 10),
		},
		Op: "order.create",
		Args: []map[string]interface{}{
			{
				"category":    req.Category,
				"symbol":      req.Symbol,
				"side":        req.Side,
				"orderType":   req.OrderType,
				"qty":         req.Qty,
				"price":       req.Price,
				"timeInForce": req.TimeInForce,
				"positionIdx": req.PositionIdx,
				"orderLinkId": req.OrderLinkId,
			},
		},
	}

	// Clean up empty fields
	cleanArgs(wsReq.Args[0])

	resp, err := ws.sendRequest(ctx, reqId, wsReq)
	if err != nil {
		return nil, err
	}

	return &TradingWSOrderResponse{
		Success:     resp.RetCode == 0,
		OrderID:     resp.Data.OrderID,
		OrderLinkId: resp.Data.OrderLinkId,
		RetCode:     resp.RetCode,
		RetMsg:      resp.RetMsg,
		ReqId:       reqId,
		Latency:     time.Since(startTime),
	}, nil
}

// AmendOrder modifies an existing order via WebSocket
func (ws *TradingWS) AmendOrder(ctx context.Context, req *AmendOrderRequest) (*TradingWSOrderResponse, error) {
	reqId := generateReqID()
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	startTime := time.Now()

	wsReq := WSTradeRequest{
		ReqId: reqId,
		Header: map[string]string{
			"X-BAPI-TIMESTAMP": timestamp,
		},
		Op: "order.amend",
		Args: []map[string]interface{}{
			{
				"category":    req.Category,
				"symbol":      req.Symbol,
				"orderId":     req.OrderID,
				"orderLinkId": req.OrderLinkId,
				"qty":         req.Qty,
				"price":       req.Price,
			},
		},
	}

	cleanArgs(wsReq.Args[0])

	resp, err := ws.sendRequest(ctx, reqId, wsReq)
	if err != nil {
		return nil, err
	}

	return &TradingWSOrderResponse{
		Success:     resp.RetCode == 0,
		OrderID:     resp.Data.OrderID,
		OrderLinkId: resp.Data.OrderLinkId,
		RetCode:     resp.RetCode,
		RetMsg:      resp.RetMsg,
		ReqId:       reqId,
		Latency:     time.Since(startTime),
	}, nil
}

// CancelOrder cancels an order via WebSocket
func (ws *TradingWS) CancelOrder(ctx context.Context, req *CancelOrderRequest) (*TradingWSOrderResponse, error) {
	reqId := generateReqID()
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	startTime := time.Now()

	wsReq := WSTradeRequest{
		ReqId: reqId,
		Header: map[string]string{
			"X-BAPI-TIMESTAMP": timestamp,
		},
		Op: "order.cancel",
		Args: []map[string]interface{}{
			{
				"category":    req.Category,
				"symbol":      req.Symbol,
				"orderId":     req.OrderID,
				"orderLinkId": req.OrderLinkId,
			},
		},
	}

	cleanArgs(wsReq.Args[0])

	resp, err := ws.sendRequest(ctx, reqId, wsReq)
	if err != nil {
		return nil, err
	}

	return &TradingWSOrderResponse{
		Success:     resp.RetCode == 0,
		OrderID:     resp.Data.OrderID,
		OrderLinkId: resp.Data.OrderLinkId,
		RetCode:     resp.RetCode,
		RetMsg:      resp.RetMsg,
		ReqId:       reqId,
		Latency:     time.Since(startTime),
	}, nil
}

// BatchCreateOrders places multiple orders via WebSocket
func (ws *TradingWS) BatchCreateOrders(ctx context.Context, category string, orders []CreateOrderRequest) (*TradingWSOrderResponse, error) {
	reqId := generateReqID()
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	startTime := time.Now()

	// Build request array
	requestArr := make([]map[string]interface{}, 0, len(orders))
	for _, order := range orders {
		orderMap := map[string]interface{}{
			"symbol":      order.Symbol,
			"side":        order.Side,
			"orderType":   order.OrderType,
			"qty":         order.Qty,
			"price":       order.Price,
			"timeInForce": order.TimeInForce,
			"positionIdx": order.PositionIdx,
		}
		cleanArgs(orderMap)
		requestArr = append(requestArr, orderMap)
	}

	wsReq := WSTradeRequest{
		ReqId: reqId,
		Header: map[string]string{
			"X-BAPI-TIMESTAMP": timestamp,
		},
		Op: "order.create-batch",
		Args: []map[string]interface{}{
			{
				"category": category,
				"request":  requestArr,
			},
		},
	}

	resp, err := ws.sendRequest(ctx, reqId, wsReq)
	if err != nil {
		return nil, err
	}

	return &TradingWSOrderResponse{
		Success: resp.RetCode == 0,
		RetCode: resp.RetCode,
		RetMsg:  resp.RetMsg,
		ReqId:   reqId,
		Latency: time.Since(startTime),
	}, nil
}

// sendRequest sends a request and waits for response
func (ws *TradingWS) sendRequest(ctx context.Context, reqId string, req WSTradeRequest) (*WSTradeResponse, error) {
	// Create response channel
	respChan := make(chan *WSTradeResponse, 1)

	ws.pendingMu.Lock()
	ws.pendingRequests[reqId] = respChan
	ws.pendingMu.Unlock()

	defer func() {
		ws.pendingMu.Lock()
		delete(ws.pendingRequests, reqId)
		ws.pendingMu.Unlock()
	}()

	// Send request
	if err := ws.sendJSON(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response with timeout
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respChan:
		return resp, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("request timeout")
	}
}

// sendJSON sends a JSON message
func (ws *TradingWS) sendJSON(v interface{}) error {
	ws.mu.RLock()
	conn := ws.conn
	ws.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	return conn.WriteJSON(v)
}

// readMessages reads and processes WebSocket messages
func (ws *TradingWS) readMessages() {
	defer func() {
		ws.mu.Lock()
		ws.connected = false
		ws.authenticated = false
		ws.mu.Unlock()
	}()

	for {
		select {
		case <-ws.done:
			return
		case <-ws.ctx.Done():
			return
		default:
			ws.mu.RLock()
			conn := ws.conn
			ws.mu.RUnlock()

			if conn == nil {
				return
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				ws.emitError("", fmt.Errorf("read error: %w", err))
				return
			}

			ws.processMessage(message)
		}
	}
}

// processMessage handles incoming WebSocket messages
func (ws *TradingWS) processMessage(data []byte) {
	// Check for auth response
	var authResp struct {
		Success bool   `json:"success"`
		Op      string `json:"op"`
		RetMsg  string `json:"ret_msg"`
		ConnId  string `json:"conn_id"`
	}
	if err := json.Unmarshal(data, &authResp); err == nil && authResp.Op == "auth" {
		if authResp.Success {
			ws.mu.Lock()
			ws.authenticated = true
			ws.mu.Unlock()
		} else {
			ws.emitError("", fmt.Errorf("auth failed: %s", authResp.RetMsg))
		}
		return
	}

	// Check for pong response
	var pongResp struct {
		Op string `json:"op"`
	}
	if err := json.Unmarshal(data, &pongResp); err == nil && pongResp.Op == "pong" {
		return
	}

	// Parse trade response
	var resp WSTradeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Debug().Err(err).Str("data", string(data)).Msg("Failed to parse trade response")
		return
	}

	// Route response to pending request
	if resp.ReqId != "" {
		ws.pendingMu.RLock()
		ch, exists := ws.pendingRequests[resp.ReqId]
		ws.pendingMu.RUnlock()

		if exists {
			select {
			case ch <- &resp:
			default:
			}
		}

		// Also call callback if set
		ws.mu.RLock()
		callback := ws.onOrder
		ws.mu.RUnlock()

		if callback != nil {
			callback(&TradingWSOrderResponse{
				Success:     resp.RetCode == 0,
				OrderID:     resp.Data.OrderID,
				OrderLinkId: resp.Data.OrderLinkId,
				RetCode:     resp.RetCode,
				RetMsg:      resp.RetMsg,
				ReqId:       resp.ReqId,
			})
		}
	}
}

// pingLoop sends periodic ping messages
func (ws *TradingWS) pingLoop() {
	ticker := time.NewTicker(WSPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ws.done:
			return
		case <-ws.ctx.Done():
			return
		case <-ticker.C:
			ping := map[string]string{"op": "ping"}
			if err := ws.sendJSON(ping); err != nil {
				ws.emitError("", fmt.Errorf("ping error: %w", err))
			}
		}
	}
}

// emitError calls the error callback if set
func (ws *TradingWS) emitError(reqId string, err error) {
	ws.mu.RLock()
	callback := ws.onError
	ws.mu.RUnlock()

	if callback != nil {
		callback(reqId, err)
	} else {
		log.Error().Err(err).Str("reqId", reqId).Msg("Bybit trade WebSocket error")
	}
}

// cleanArgs removes empty string values from a map
func cleanArgs(m map[string]interface{}) {
	for k, v := range m {
		if str, ok := v.(string); ok && str == "" {
			delete(m, k)
		}
		if num, ok := v.(int); ok && num == 0 {
			delete(m, k)
		}
	}
}

// =============================================================================
// Convenience Methods for Linear Perpetuals
// =============================================================================

// PlaceLinearLimitOrder places a limit order for linear perpetuals
func (ws *TradingWS) PlaceLinearLimitOrder(ctx context.Context, symbol string, side OrderSide, qty, price string) (*TradingWSOrderResponse, error) {
	req := &CreateOrderRequest{
		Category:    string(CategoryLinear),
		Symbol:      symbol,
		Side:        string(side),
		OrderType:   string(OrderTypeLimit),
		Qty:         qty,
		Price:       price,
		TimeInForce: string(TimeInForceGTC),
		PositionIdx: 0,
	}
	return ws.CreateOrder(ctx, req)
}

// PlaceLinearMarketOrder places a market order for linear perpetuals
func (ws *TradingWS) PlaceLinearMarketOrder(ctx context.Context, symbol string, side OrderSide, qty string) (*TradingWSOrderResponse, error) {
	req := &CreateOrderRequest{
		Category:    string(CategoryLinear),
		Symbol:      symbol,
		Side:        string(side),
		OrderType:   string(OrderTypeMarket),
		Qty:         qty,
		PositionIdx: 0,
	}
	return ws.CreateOrder(ctx, req)
}

// CancelLinearOrder cancels a linear perpetual order by order ID
func (ws *TradingWS) CancelLinearOrder(ctx context.Context, symbol, orderId string) (*TradingWSOrderResponse, error) {
	req := &CancelOrderRequest{
		Category: string(CategoryLinear),
		Symbol:   symbol,
		OrderID:  orderId,
	}
	return ws.CancelOrder(ctx, req)
}

// AmendLinearOrder amends a linear perpetual order
func (ws *TradingWS) AmendLinearOrder(ctx context.Context, symbol, orderId, newQty, newPrice string) (*TradingWSOrderResponse, error) {
	req := &AmendOrderRequest{
		Category: string(CategoryLinear),
		Symbol:   symbol,
		OrderID:  orderId,
		Qty:      newQty,
		Price:    newPrice,
	}
	return ws.AmendOrder(ctx, req)
}
