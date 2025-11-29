// Package bingx provides WebSocket market data client for BingX Perpetual Futures.
package bingx

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// WSMarketDataHandler handles market data callbacks
type WSMarketDataHandler struct {
	OnDepth      func(symbol string, depth *WSDepthData)
	OnTrade      func(symbol string, trades *WSTradeData)
	OnKline      func(symbol string, interval string, kline *WSKlineData)
	OnError      func(err error)
	OnConnect    func()
	OnDisconnect func(err error)
}

// WSMarketDataClient handles WebSocket market data connections for BingX
type WSMarketDataClient struct {
	handler        *WSMarketDataHandler
	conn           *websocket.Conn
	subscriptions  map[string]bool // dataType -> subscribed
	mu             sync.RWMutex
	writeMu        sync.Mutex
	reconnectDelay time.Duration
	maxRetries     int
	isConnected    atomic.Bool
	pingInterval   time.Duration
	stopPing       chan struct{}
	done           chan struct{}
	msgID          atomic.Int64
}

// NewWSMarketDataClient creates a new WebSocket market data client
func NewWSMarketDataClient(handler *WSMarketDataHandler) *WSMarketDataClient {
	return &WSMarketDataClient{
		handler:        handler,
		subscriptions:  make(map[string]bool),
		reconnectDelay: 5 * time.Second,
		maxRetries:     10,
		pingInterval:   20 * time.Second,
	}
}

// Connect establishes WebSocket connection
func (c *WSMarketDataClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isConnected.Load() {
		return nil // Already connected
	}

	return c.connectInternal()
}

func (c *WSMarketDataClient) connectInternal() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(WSMarketDataURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to BingX WS: %w", err)
	}

	c.conn = conn
	c.isConnected.Store(true)
	c.stopPing = make(chan struct{})
	c.done = make(chan struct{})

	// Start message handler
	go c.readLoop()

	// Start ping loop
	go c.pingLoop()

	if c.handler != nil && c.handler.OnConnect != nil {
		c.handler.OnConnect()
	}

	log.Printf("[BingX WS] Connected to market data stream")

	// Resubscribe to existing subscriptions
	for dataType := range c.subscriptions {
		if err := c.sendSubscribe(dataType); err != nil {
			log.Printf("[BingX WS] Failed to resubscribe to %s: %v", dataType, err)
		}
	}

	return nil
}

// readLoop reads messages from the WebSocket connection
func (c *WSMarketDataClient) readLoop() {
	defer func() {
		c.isConnected.Store(false)
		close(c.stopPing)

		if c.handler != nil && c.handler.OnDisconnect != nil {
			c.handler.OnDisconnect(nil)
		}
	}()

	for {
		select {
		case <-c.done:
			return
		default:
		}

		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[BingX WS] Read error: %v", err)
			}
			c.handleReconnect()
			return
		}

		c.handleMessage(message)
	}
}

// pingLoop sends periodic pings to keep connection alive
func (c *WSMarketDataClient) pingLoop() {
	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopPing:
			return
		case <-c.done:
			return
		case <-ticker.C:
			if err := c.sendPing(); err != nil {
				log.Printf("[BingX WS] Ping error: %v", err)
				return
			}
		}
	}
}

// sendPing sends a ping message
func (c *WSMarketDataClient) sendPing() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("connection not established")
	}

	// BingX uses text "Ping" message
	return c.conn.WriteMessage(websocket.TextMessage, []byte("Ping"))
}

// nextMsgID generates a unique message ID
func (c *WSMarketDataClient) nextMsgID() string {
	return fmt.Sprintf("%d", c.msgID.Add(1))
}

// handleMessage processes incoming WebSocket messages
func (c *WSMarketDataClient) handleMessage(data []byte) {
	// Handle Pong response
	if string(data) == "Pong" {
		return
	}

	var msg WSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[BingX WS] Failed to parse message: %v", err)
		return
	}

	// Handle error codes
	if msg.Code != 0 {
		log.Printf("[BingX WS] Error response: code=%d", msg.Code)
		if c.handler != nil && c.handler.OnError != nil {
			c.handler.OnError(fmt.Errorf("WebSocket error: code=%d", msg.Code))
		}
		return
	}

	// Handle data messages
	if msg.DataType != "" {
		c.handleDataMessage(msg.DataType, msg.Data)
	}
}

func (c *WSMarketDataClient) handleDataMessage(dataType string, data json.RawMessage) {
	if c.handler == nil {
		return
	}

	// Parse dataType: e.g., "BTC-USDT@depth20", "BTC-USDT@trade", "BTC-USDT@kline_1m"
	symbol, subscriptionType := parseDataType(dataType)

	switch {
	case isDepthType(subscriptionType):
		c.handleDepthUpdate(symbol, data)
	case isTradeType(subscriptionType):
		c.handleTradeUpdate(symbol, data)
	case isKlineType(subscriptionType):
		interval := parseKlineInterval(subscriptionType)
		c.handleKlineUpdate(symbol, interval, data)
	default:
		log.Printf("[BingX WS] Unhandled data type: %s", dataType)
	}
}

func (c *WSMarketDataClient) handleDepthUpdate(symbol string, data json.RawMessage) {
	if c.handler.OnDepth == nil {
		return
	}

	var depth WSDepthData
	if err := json.Unmarshal(data, &depth); err != nil {
		log.Printf("[BingX WS] Failed to parse depth: %v", err)
		return
	}

	c.handler.OnDepth(symbol, &depth)
}

func (c *WSMarketDataClient) handleTradeUpdate(symbol string, data json.RawMessage) {
	if c.handler.OnTrade == nil {
		return
	}

	var trades WSTradeData
	if err := json.Unmarshal(data, &trades); err != nil {
		log.Printf("[BingX WS] Failed to parse trades: %v", err)
		return
	}

	c.handler.OnTrade(symbol, &trades)
}

func (c *WSMarketDataClient) handleKlineUpdate(symbol, interval string, data json.RawMessage) {
	if c.handler.OnKline == nil {
		return
	}

	var kline WSKlineData
	if err := json.Unmarshal(data, &kline); err != nil {
		log.Printf("[BingX WS] Failed to parse kline: %v", err)
		return
	}

	c.handler.OnKline(symbol, interval, &kline)
}

// handleReconnect attempts to reconnect to WebSocket
func (c *WSMarketDataClient) handleReconnect() {
	for i := 0; i < c.maxRetries; i++ {
		select {
		case <-c.done:
			return
		default:
		}

		log.Printf("[BingX WS] Attempting reconnect %d/%d in %v", i+1, c.maxRetries, c.reconnectDelay)
		time.Sleep(c.reconnectDelay)

		c.mu.Lock()
		err := c.connectInternal()
		c.mu.Unlock()

		if err == nil {
			log.Printf("[BingX WS] Reconnected successfully")
			return
		}

		log.Printf("[BingX WS] Reconnect failed: %v", err)
	}

	log.Printf("[BingX WS] Max reconnection attempts reached")
	if c.handler != nil && c.handler.OnError != nil {
		c.handler.OnError(fmt.Errorf("max reconnection attempts reached"))
	}
}

// sendSubscribe sends subscription message
func (c *WSMarketDataClient) sendSubscribe(dataType string) error {
	msg := WSSubscribeRequest{
		ID:       c.nextMsgID(),
		ReqType:  "sub",
		DataType: dataType,
	}
	return c.sendMessage(msg)
}

// sendUnsubscribe sends unsubscription message
func (c *WSMarketDataClient) sendUnsubscribe(dataType string) error {
	msg := WSSubscribeRequest{
		ID:       c.nextMsgID(),
		ReqType:  "unsub",
		DataType: dataType,
	}
	return c.sendMessage(msg)
}

// sendMessage sends a JSON message
func (c *WSMarketDataClient) sendMessage(msg interface{}) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("connection not established")
	}

	return c.conn.WriteJSON(msg)
}

// =============================================================================
// Subscription Methods
// =============================================================================

// SubscribeDepth subscribes to orderbook depth for a symbol
// levels: 5, 10, 20, 50, 100
func (c *WSMarketDataClient) SubscribeDepth(symbol string, levels int) error {
	dataType := fmt.Sprintf("%s@depth%d", symbol, levels)

	c.mu.Lock()
	c.subscriptions[dataType] = true
	c.mu.Unlock()

	if c.isConnected.Load() {
		return c.sendSubscribe(dataType)
	}
	return nil
}

// UnsubscribeDepth unsubscribes from orderbook depth
func (c *WSMarketDataClient) UnsubscribeDepth(symbol string, levels int) error {
	dataType := fmt.Sprintf("%s@depth%d", symbol, levels)

	c.mu.Lock()
	delete(c.subscriptions, dataType)
	c.mu.Unlock()

	if c.isConnected.Load() {
		return c.sendUnsubscribe(dataType)
	}
	return nil
}

// SubscribeTrade subscribes to trade updates for a symbol
func (c *WSMarketDataClient) SubscribeTrade(symbol string) error {
	dataType := fmt.Sprintf("%s@trade", symbol)

	c.mu.Lock()
	c.subscriptions[dataType] = true
	c.mu.Unlock()

	if c.isConnected.Load() {
		return c.sendSubscribe(dataType)
	}
	return nil
}

// UnsubscribeTrade unsubscribes from trade updates
func (c *WSMarketDataClient) UnsubscribeTrade(symbol string) error {
	dataType := fmt.Sprintf("%s@trade", symbol)

	c.mu.Lock()
	delete(c.subscriptions, dataType)
	c.mu.Unlock()

	if c.isConnected.Load() {
		return c.sendUnsubscribe(dataType)
	}
	return nil
}

// SubscribeKline subscribes to kline/candlestick updates
// interval: 1m, 3m, 5m, 15m, 30m, 1h, 2h, 4h, 6h, 8h, 12h, 1d, 3d, 1w, 1M
func (c *WSMarketDataClient) SubscribeKline(symbol, interval string) error {
	dataType := fmt.Sprintf("%s@kline_%s", symbol, interval)

	c.mu.Lock()
	c.subscriptions[dataType] = true
	c.mu.Unlock()

	if c.isConnected.Load() {
		return c.sendSubscribe(dataType)
	}
	return nil
}

// UnsubscribeKline unsubscribes from kline updates
func (c *WSMarketDataClient) UnsubscribeKline(symbol, interval string) error {
	dataType := fmt.Sprintf("%s@kline_%s", symbol, interval)

	c.mu.Lock()
	delete(c.subscriptions, dataType)
	c.mu.Unlock()

	if c.isConnected.Load() {
		return c.sendUnsubscribe(dataType)
	}
	return nil
}

// SubscribeMultipleDepth subscribes to depth for multiple symbols
func (c *WSMarketDataClient) SubscribeMultipleDepth(symbols []string, levels int) error {
	for _, symbol := range symbols {
		if err := c.SubscribeDepth(symbol, levels); err != nil {
			return fmt.Errorf("failed to subscribe %s: %w", symbol, err)
		}
	}
	return nil
}

// UnsubscribeAll unsubscribes from all subscriptions
func (c *WSMarketDataClient) UnsubscribeAll() error {
	c.mu.Lock()
	subs := make([]string, 0, len(c.subscriptions))
	for dataType := range c.subscriptions {
		subs = append(subs, dataType)
	}
	c.subscriptions = make(map[string]bool)
	c.mu.Unlock()

	for _, dataType := range subs {
		if err := c.sendUnsubscribe(dataType); err != nil {
			log.Printf("[BingX WS] Failed to unsubscribe %s: %v", dataType, err)
		}
	}
	return nil
}

// GetSubscriptions returns list of current subscriptions
func (c *WSMarketDataClient) GetSubscriptions() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	subs := make([]string, 0, len(c.subscriptions))
	for dataType := range c.subscriptions {
		subs = append(subs, dataType)
	}
	return subs
}

// IsConnected returns whether the client is connected
func (c *WSMarketDataClient) IsConnected() bool {
	return c.isConnected.Load()
}

// Close closes the WebSocket connection
func (c *WSMarketDataClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.done != nil {
		close(c.done)
	}

	c.isConnected.Store(false)
	c.subscriptions = make(map[string]bool)

	if c.conn != nil {
		return c.conn.Close()
	}

	return nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// parseDataType parses dataType into symbol and subscription type
// e.g., "BTC-USDT@depth20" -> "BTC-USDT", "depth20"
func parseDataType(dataType string) (symbol, subType string) {
	for i, ch := range dataType {
		if ch == '@' {
			return dataType[:i], dataType[i+1:]
		}
	}
	return dataType, ""
}

// isDepthType checks if subscription type is depth
func isDepthType(subType string) bool {
	return len(subType) >= 5 && subType[:5] == "depth"
}

// isTradeType checks if subscription type is trade
func isTradeType(subType string) bool {
	return subType == "trade"
}

// isKlineType checks if subscription type is kline
func isKlineType(subType string) bool {
	return len(subType) >= 6 && subType[:6] == "kline_"
}

// parseKlineInterval extracts interval from kline subscription type
// e.g., "kline_1m" -> "1m"
func parseKlineInterval(subType string) string {
	if len(subType) > 6 {
		return subType[6:]
	}
	return ""
}
