package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	wsFapiBaseURL = "wss://ws-fapi.binance.com/ws-fapi/v1"
)

// TradingHandler handles trading-related WebSocket events
type TradingHandler struct {
	OnOrderResult func(id string, result *OrderResult, err error)
	OnError       func(err error)
}

// TradingClient manages the WebSocket API connection for low-latency trading
type TradingClient struct {
	conn      *websocket.Conn
	apiKey    string
	secretKey string
	handler   *TradingHandler

	requestID   uint64
	pendingReqs map[string]chan *WSAPIResponse
	mu          sync.RWMutex

	done      chan struct{}
	connected bool
}

// NewTradingClient creates a new trading WebSocket client
func NewTradingClient(apiKey, secretKey string, handler *TradingHandler) *TradingClient {
	return &TradingClient{
		apiKey:      apiKey,
		secretKey:   secretKey,
		handler:     handler,
		pendingReqs: make(map[string]chan *WSAPIResponse),
		done:        make(chan struct{}),
	}
}

// Connect connects to the WebSocket API
func (c *TradingClient) Connect(ctx context.Context) error {
	log.Info().Str("url", wsFapiBaseURL).Msg("Connecting to Binance trading WebSocket API")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, wsFapiBaseURL, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.conn = conn
	c.connected = true
	log.Info().Msg("Connected to Binance trading WebSocket API")

	go c.readLoop()

	return nil
}

// Disconnect closes the connection
func (c *TradingClient) Disconnect() error {
	close(c.done)
	c.connected = false
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// IsConnected returns connection status
func (c *TradingClient) IsConnected() bool {
	return c.connected
}

// =============================================================================
// Order Operations
// =============================================================================

// OrderParams represents parameters for placing an order
type OrderParams struct {
	Symbol           string
	Side             string // BUY or SELL
	Type             string // LIMIT, MARKET, STOP, etc.
	Quantity         float64
	Price            float64 // Required for LIMIT orders
	PositionSide     string  // LONG or SHORT (for hedge mode)
	TimeInForce      string  // GTC, IOC, FOK, GTX
	ReduceOnly       bool
	NewClientOrderId string
	StopPrice        float64 // Required for STOP orders
	WorkingType      string  // MARK_PRICE or CONTRACT_PRICE
}

// PlaceOrder places a new order via WebSocket API (low latency)
func (c *TradingClient) PlaceOrder(ctx context.Context, params *OrderParams) (*OrderResult, error) {
	reqParams := map[string]interface{}{
		"symbol":    params.Symbol,
		"side":      params.Side,
		"type":      params.Type,
		"quantity":  strconv.FormatFloat(params.Quantity, 'f', -1, 64),
		"timestamp": time.Now().UnixMilli(),
	}

	if params.Price > 0 {
		reqParams["price"] = strconv.FormatFloat(params.Price, 'f', -1, 64)
	}
	if params.PositionSide != "" {
		reqParams["positionSide"] = params.PositionSide
	}
	if params.TimeInForce != "" {
		reqParams["timeInForce"] = params.TimeInForce
	}
	if params.ReduceOnly {
		reqParams["reduceOnly"] = "true"
	}
	if params.NewClientOrderId != "" {
		reqParams["newClientOrderId"] = params.NewClientOrderId
	}
	if params.StopPrice > 0 {
		reqParams["stopPrice"] = strconv.FormatFloat(params.StopPrice, 'f', -1, 64)
	}
	if params.WorkingType != "" {
		reqParams["workingType"] = params.WorkingType
	}

	// Add signature
	c.signParams(reqParams)

	resp, err := c.sendRequest(ctx, "order.place", reqParams)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("order error [%d]: %s", resp.Error.Code, resp.Error.Message)
	}

	var result OrderResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("decode order result: %w", err)
	}

	return &result, nil
}

// CancelOrder cancels an existing order
func (c *TradingClient) CancelOrder(ctx context.Context, symbol string, orderId int64, clientOrderId string) (*OrderResult, error) {
	params := map[string]interface{}{
		"symbol":    symbol,
		"timestamp": time.Now().UnixMilli(),
	}

	if orderId > 0 {
		params["orderId"] = orderId
	}
	if clientOrderId != "" {
		params["origClientOrderId"] = clientOrderId
	}

	c.signParams(params)

	resp, err := c.sendRequest(ctx, "order.cancel", params)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("cancel error [%d]: %s", resp.Error.Code, resp.Error.Message)
	}

	var result OrderResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("decode cancel result: %w", err)
	}

	return &result, nil
}

// ModifyOrder modifies an existing order (price and/or quantity)
func (c *TradingClient) ModifyOrder(ctx context.Context, symbol string, orderId int64, quantity, price float64, side string) (*OrderResult, error) {
	params := map[string]interface{}{
		"symbol":    symbol,
		"orderId":   orderId,
		"side":      side,
		"quantity":  strconv.FormatFloat(quantity, 'f', -1, 64),
		"price":     strconv.FormatFloat(price, 'f', -1, 64),
		"timestamp": time.Now().UnixMilli(),
	}

	c.signParams(params)

	resp, err := c.sendRequest(ctx, "order.modify", params)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("modify error [%d]: %s", resp.Error.Code, resp.Error.Message)
	}

	var result OrderResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("decode modify result: %w", err)
	}

	return &result, nil
}

// CancelAllOrders cancels all open orders for a symbol
func (c *TradingClient) CancelAllOrders(ctx context.Context, symbol string) error {
	params := map[string]interface{}{
		"symbol":    symbol,
		"timestamp": time.Now().UnixMilli(),
	}

	c.signParams(params)

	resp, err := c.sendRequest(ctx, "openOrders.cancelAll", params)
	if err != nil {
		return err
	}

	if resp.Error != nil {
		return fmt.Errorf("cancel all error [%d]: %s", resp.Error.Code, resp.Error.Message)
	}

	return nil
}

// QueryOrder queries the status of an order
func (c *TradingClient) QueryOrder(ctx context.Context, symbol string, orderId int64, clientOrderId string) (*OrderResult, error) {
	params := map[string]interface{}{
		"symbol":    symbol,
		"timestamp": time.Now().UnixMilli(),
	}

	if orderId > 0 {
		params["orderId"] = orderId
	}
	if clientOrderId != "" {
		params["origClientOrderId"] = clientOrderId
	}

	c.signParams(params)

	resp, err := c.sendRequest(ctx, "order.status", params)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("query error [%d]: %s", resp.Error.Code, resp.Error.Message)
	}

	var result OrderResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("decode query result: %w", err)
	}

	return &result, nil
}

// =============================================================================
// Batch Operations
// =============================================================================

// PlaceBatchOrders places multiple orders in a single request
func (c *TradingClient) PlaceBatchOrders(ctx context.Context, orders []*OrderParams) ([]*OrderResult, error) {
	batchOrders := make([]map[string]interface{}, 0, len(orders))

	for _, params := range orders {
		order := map[string]interface{}{
			"symbol":   params.Symbol,
			"side":     params.Side,
			"type":     params.Type,
			"quantity": strconv.FormatFloat(params.Quantity, 'f', -1, 64),
		}

		if params.Price > 0 {
			order["price"] = strconv.FormatFloat(params.Price, 'f', -1, 64)
		}
		if params.PositionSide != "" {
			order["positionSide"] = params.PositionSide
		}
		if params.TimeInForce != "" {
			order["timeInForce"] = params.TimeInForce
		}
		if params.ReduceOnly {
			order["reduceOnly"] = "true"
		}
		if params.NewClientOrderId != "" {
			order["newClientOrderId"] = params.NewClientOrderId
		}

		batchOrders = append(batchOrders, order)
	}

	batchJSON, _ := json.Marshal(batchOrders)

	params := map[string]interface{}{
		"batchOrders": string(batchJSON),
		"timestamp":   time.Now().UnixMilli(),
	}

	c.signParams(params)

	resp, err := c.sendRequest(ctx, "order.place.batch", params)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("batch order error [%d]: %s", resp.Error.Code, resp.Error.Message)
	}

	var results []*OrderResult
	if err := json.Unmarshal(resp.Result, &results); err != nil {
		return nil, fmt.Errorf("decode batch results: %w", err)
	}

	return results, nil
}

// =============================================================================
// Internal Methods
// =============================================================================

func (c *TradingClient) sendRequest(ctx context.Context, method string, params map[string]interface{}) (*WSAPIResponse, error) {
	id := fmt.Sprintf("%d", atomic.AddUint64(&c.requestID, 1))

	// Add API key to params
	params["apiKey"] = c.apiKey

	req := WSAPIRequest{
		ID:     id,
		Method: method,
		Params: params,
	}

	// Create response channel
	respChan := make(chan *WSAPIResponse, 1)
	c.mu.Lock()
	c.pendingReqs[id] = respChan
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pendingReqs, id)
		c.mu.Unlock()
	}()

	// Send request
	if err := c.conn.WriteJSON(req); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
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

func (c *TradingClient) readLoop() {
	defer func() {
		c.connected = false
	}()

	for {
		select {
		case <-c.done:
			return
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				if c.handler != nil && c.handler.OnError != nil {
					c.handler.OnError(fmt.Errorf("read error: %w", err))
				}
				return
			}
			c.handleMessage(message)
		}
	}
}

func (c *TradingClient) handleMessage(message []byte) {
	var resp WSAPIResponse
	if err := json.Unmarshal(message, &resp); err != nil {
		log.Warn().Err(err).Msg("Failed to parse WebSocket API response")
		return
	}

	// Route response to waiting request
	c.mu.RLock()
	respChan, ok := c.pendingReqs[resp.ID]
	c.mu.RUnlock()

	if ok {
		select {
		case respChan <- &resp:
		default:
		}
	}
}

func (c *TradingClient) signParams(params map[string]interface{}) {
	// Build query string for signing
	var queryString string
	for k, v := range params {
		if queryString != "" {
			queryString += "&"
		}
		queryString += fmt.Sprintf("%s=%v", k, v)
	}

	// Generate HMAC SHA256 signature
	mac := hmac.New(sha256.New, []byte(c.secretKey))
	mac.Write([]byte(queryString))
	signature := hex.EncodeToString(mac.Sum(nil))

	params["signature"] = signature
}

// =============================================================================
// Order Side and Type Constants
// =============================================================================

const (
	SideBuy  = "BUY"
	SideSell = "SELL"

	OrderTypeLimit              = "LIMIT"
	OrderTypeMarket             = "MARKET"
	OrderTypeStop               = "STOP"
	OrderTypeStopMarket         = "STOP_MARKET"
	OrderTypeTakeProfit         = "TAKE_PROFIT"
	OrderTypeTakeProfitMarket   = "TAKE_PROFIT_MARKET"
	OrderTypeTrailingStopMarket = "TRAILING_STOP_MARKET"

	TimeInForceGTC = "GTC" // Good Til Canceled
	TimeInForceIOC = "IOC" // Immediate Or Cancel
	TimeInForceFOK = "FOK" // Fill Or Kill
	TimeInForceGTX = "GTX" // Good Til Crossing (Post Only)

	PositionSideLong  = "LONG"
	PositionSideShort = "SHORT"
	PositionSideBoth  = "BOTH"

	WorkingTypeMarkPrice     = "MARK_PRICE"
	WorkingTypeContractPrice = "CONTRACT_PRICE"
)
