package htx

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// WSMarketDataClient handles public WebSocket market data for HTX
type WSMarketDataClient struct {
	url               string
	conn              *websocket.Conn
	connMu            sync.Mutex
	state             atomic.Int32
	subscriptions     *SubscriptionManager
	orderBooks        map[string]*OrderBook
	orderBooksMu      sync.RWMutex
	reconnectDelay    time.Duration
	maxReconnectDelay time.Duration
	pingInterval      time.Duration
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	onConnect         func()
	onDisconnect      func()
	onError           func(error)
	lastPing          atomic.Int64
}

// NewWSMarketDataClient creates a new WebSocket market data client
func NewWSMarketDataClient(url string) *WSMarketDataClient {
	ctx, cancel := context.WithCancel(context.Background())
	client := &WSMarketDataClient{
		url:               url,
		subscriptions:     NewSubscriptionManager(),
		orderBooks:        make(map[string]*OrderBook),
		reconnectDelay:    1 * time.Second,
		maxReconnectDelay: 30 * time.Second,
		pingInterval:      20 * time.Second,
		ctx:               ctx,
		cancel:            cancel,
	}
	client.state.Store(int32(StateDisconnected))
	return client
}

// SetCallbacks sets connection callbacks
func (c *WSMarketDataClient) SetCallbacks(onConnect, onDisconnect func(), onError func(error)) {
	c.onConnect = onConnect
	c.onDisconnect = onDisconnect
	c.onError = onError
}

// Connect establishes WebSocket connection
func (c *WSMarketDataClient) Connect() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if ConnectionState(c.state.Load()) == StateConnected {
		return nil
	}

	c.state.Store(int32(StateConnecting))

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(c.url, nil)
	if err != nil {
		c.state.Store(int32(StateDisconnected))
		return fmt.Errorf("websocket dial: %w", err)
	}

	c.conn = conn
	c.state.Store(int32(StateConnected))

	// Start message handler
	c.wg.Add(1)
	go c.readMessages()

	// Start ping handler
	c.wg.Add(1)
	go c.pingHandler()

	if c.onConnect != nil {
		c.onConnect()
	}

	// Resubscribe existing subscriptions
	c.resubscribe()

	return nil
}

// Disconnect closes the WebSocket connection
func (c *WSMarketDataClient) Disconnect() {
	c.cancel()
	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.connMu.Unlock()
	c.state.Store(int32(StateDisconnected))
	c.wg.Wait()
}

// GetState returns the current connection state
func (c *WSMarketDataClient) GetState() ConnectionState {
	return ConnectionState(c.state.Load())
}

// decompressGzip decompresses gzip data
func decompressGzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create gzip reader: %w", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read gzip: %w", err)
	}

	return decompressed, nil
}

// readMessages handles incoming WebSocket messages
func (c *WSMarketDataClient) readMessages() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.connMu.Lock()
		conn := c.conn
		c.connMu.Unlock()

		if conn == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			log.Printf("[HTX WS] read error: %v", err)
			if c.onError != nil {
				c.onError(err)
			}
			c.handleDisconnect()
			continue
		}

		// Decompress GZIP data
		decompressed, err := decompressGzip(message)
		if err != nil {
			log.Printf("[HTX WS] decompress error: %v", err)
			continue
		}

		c.handleMessage(decompressed)
	}
}

// handleMessage processes a single message
func (c *WSMarketDataClient) handleMessage(data []byte) {
	// Parse base response
	var resp WSResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Printf("[HTX WS] unmarshal error: %v", err)
		return
	}

	// Handle ping
	if resp.Ping > 0 {
		c.sendPong(resp.Ping)
		c.lastPing.Store(time.Now().UnixMilli())
		return
	}

	// Handle subscription confirmation
	if resp.Subbed != "" {
		log.Printf("[HTX WS] subscribed to: %s", resp.Subbed)
		return
	}

	// Handle channel data
	if resp.Ch != "" {
		c.handleChannelData(resp.Ch, data)
		return
	}

	// Handle request response
	if resp.Rep != "" {
		c.handleRequestData(resp.Rep, data)
		return
	}
}

// handleChannelData routes channel data to appropriate handler
func (c *WSMarketDataClient) handleChannelData(channel string, data []byte) {
	// Find subscription callback
	sub, ok := c.subscriptions.Get(channel)
	if ok && sub.Callback != nil {
		sub.Callback(data)
		return
	}

	// Parse and handle based on channel type
	if containsSubstring(channel, ".kline.") {
		c.handleKlineData(channel, data)
	} else if containsSubstring(channel, ".depth.") {
		c.handleDepthData(channel, data)
	} else if containsSubstring(channel, ".bbo") {
		c.handleBBOData(channel, data)
	} else if containsSubstring(channel, ".trade.detail") {
		c.handleTradeData(channel, data)
	} else if containsSubstring(channel, ".detail") {
		c.handleTickerData(channel, data)
	}
}

// containsSubstring checks if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// handleRequestData handles request response data
func (c *WSMarketDataClient) handleRequestData(rep string, data []byte) {
	sub, ok := c.subscriptions.Get(rep)
	if ok && sub.Callback != nil {
		sub.Callback(data)
	}
}

// handleKlineData handles kline data
func (c *WSMarketDataClient) handleKlineData(channel string, data []byte) {
	var resp struct {
		Ch   string      `json:"ch"`
		Ts   int64       `json:"ts"`
		Tick WSKlineTick `json:"tick"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Printf("[HTX WS] kline unmarshal error: %v", err)
		return
	}

	// Call callback if registered
	sub, ok := c.subscriptions.Get(channel)
	if ok && sub.Callback != nil {
		sub.Callback(data)
	}
}

// handleDepthData handles depth data
func (c *WSMarketDataClient) handleDepthData(channel string, data []byte) {
	var resp struct {
		Ch   string      `json:"ch"`
		Ts   int64       `json:"ts"`
		Tick WSDepthTick `json:"tick"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Printf("[HTX WS] depth unmarshal error: %v", err)
		return
	}

	// Update local order book
	symbol := extractSymbolFromChannel(channel)
	if symbol != "" {
		c.orderBooksMu.Lock()
		if ob, ok := c.orderBooks[symbol]; ok {
			ob.Update(resp.Tick.Asks, resp.Tick.Bids, resp.Tick.Ts, resp.Tick.Version)
		}
		c.orderBooksMu.Unlock()
	}

	// Call callback if registered
	sub, ok := c.subscriptions.Get(channel)
	if ok && sub.Callback != nil {
		sub.Callback(data)
	}
}

// handleBBOData handles BBO data
func (c *WSMarketDataClient) handleBBOData(channel string, data []byte) {
	var resp struct {
		Ch   string    `json:"ch"`
		Ts   int64     `json:"ts"`
		Tick WSBBOTick `json:"tick"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Printf("[HTX WS] bbo unmarshal error: %v", err)
		return
	}

	// Call callback if registered
	sub, ok := c.subscriptions.Get(channel)
	if ok && sub.Callback != nil {
		sub.Callback(data)
	}
}

// handleTradeData handles trade data
func (c *WSMarketDataClient) handleTradeData(channel string, data []byte) {
	var resp struct {
		Ch   string      `json:"ch"`
		Ts   int64       `json:"ts"`
		Tick WSTradeTick `json:"tick"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Printf("[HTX WS] trade unmarshal error: %v", err)
		return
	}

	// Call callback if registered
	sub, ok := c.subscriptions.Get(channel)
	if ok && sub.Callback != nil {
		sub.Callback(data)
	}
}

// handleTickerData handles ticker data
func (c *WSMarketDataClient) handleTickerData(channel string, data []byte) {
	// Call callback if registered
	sub, ok := c.subscriptions.Get(channel)
	if ok && sub.Callback != nil {
		sub.Callback(data)
	}
}

// extractSymbolFromChannel extracts symbol from channel name
func extractSymbolFromChannel(channel string) string {
	// Format: market.BTC-USDT.depth.step0
	parts := strings.Split(channel, ".")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

// sendPong sends pong response
func (c *WSMarketDataClient) sendPong(pingTs int64) {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return
	}

	pong := WSPong{Pong: pingTs}
	data, err := json.Marshal(pong)
	if err != nil {
		log.Printf("[HTX WS] marshal pong error: %v", err)
		return
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("[HTX WS] send pong error: %v", err)
	}
}

// pingHandler monitors connection health
func (c *WSMarketDataClient) pingHandler() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			lastPing := c.lastPing.Load()
			if lastPing > 0 && time.Now().UnixMilli()-lastPing > 60000 {
				log.Printf("[HTX WS] no ping received in 60s, reconnecting")
				c.handleDisconnect()
			}
		}
	}
}

// handleDisconnect handles WebSocket disconnection
func (c *WSMarketDataClient) handleDisconnect() {
	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.connMu.Unlock()

	c.state.Store(int32(StateReconnecting))

	if c.onDisconnect != nil {
		c.onDisconnect()
	}

	// Attempt reconnection
	go c.reconnect()
}

// reconnect attempts to reconnect with exponential backoff
func (c *WSMarketDataClient) reconnect() {
	delay := c.reconnectDelay

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		log.Printf("[HTX WS] reconnecting in %v", delay)
		time.Sleep(delay)

		if err := c.Connect(); err != nil {
			log.Printf("[HTX WS] reconnect failed: %v", err)
			delay *= 2
			if delay > c.maxReconnectDelay {
				delay = c.maxReconnectDelay
			}
			continue
		}

		log.Printf("[HTX WS] reconnected successfully")
		return
	}
}

// resubscribe resubscribes to all subscriptions after reconnection
func (c *WSMarketDataClient) resubscribe() {
	subs := c.subscriptions.GetAll()
	for _, sub := range subs {
		if err := c.sendSubscription(sub.Topic); err != nil {
			log.Printf("[HTX WS] resubscribe error for %s: %v", sub.Topic, err)
		}
	}
}

// sendSubscription sends a subscription request
func (c *WSMarketDataClient) sendSubscription(topic string) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	req := WSRequest{
		Sub: topic,
		ID:  fmt.Sprintf("sub_%d", time.Now().UnixNano()),
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal subscription: %w", err)
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("send subscription: %w", err)
	}

	return nil
}

// sendUnsubscription sends an unsubscription request
func (c *WSMarketDataClient) sendUnsubscription(topic string) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	req := WSRequest{
		Unsub: topic,
		ID:    fmt.Sprintf("unsub_%d", time.Now().UnixNano()),
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal unsubscription: %w", err)
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("send unsubscription: %w", err)
	}

	return nil
}

// ========== Subscription Methods ==========

// SubscribeKline subscribes to kline data
func (c *WSMarketDataClient) SubscribeKline(symbol, period string, callback func(data []byte)) error {
	topic := fmt.Sprintf("market.%s.kline.%s", symbol, period)
	c.subscriptions.Add(topic, callback)

	if ConnectionState(c.state.Load()) == StateConnected {
		return c.sendSubscription(topic)
	}
	return nil
}

// UnsubscribeKline unsubscribes from kline data
func (c *WSMarketDataClient) UnsubscribeKline(symbol, period string) error {
	topic := fmt.Sprintf("market.%s.kline.%s", symbol, period)
	c.subscriptions.Remove(topic)
	return c.sendUnsubscription(topic)
}

// SubscribeDepth subscribes to depth data
func (c *WSMarketDataClient) SubscribeDepth(symbol, depthType string, callback func(data []byte)) error {
	topic := fmt.Sprintf("market.%s.depth.%s", symbol, depthType)
	c.subscriptions.Add(topic, callback)

	// Initialize order book
	c.orderBooksMu.Lock()
	if _, ok := c.orderBooks[symbol]; !ok {
		c.orderBooks[symbol] = NewOrderBook(symbol)
	}
	c.orderBooksMu.Unlock()

	if ConnectionState(c.state.Load()) == StateConnected {
		return c.sendSubscription(topic)
	}
	return nil
}

// SubscribeIncrementalDepth subscribes to incremental depth data
func (c *WSMarketDataClient) SubscribeIncrementalDepth(symbol string, size int, callback func(data []byte)) error {
	topic := fmt.Sprintf("market.%s.depth.size_%d.high_freq", symbol, size)
	c.subscriptions.Add(topic, callback)

	// Initialize order book
	c.orderBooksMu.Lock()
	if _, ok := c.orderBooks[symbol]; !ok {
		c.orderBooks[symbol] = NewOrderBook(symbol)
	}
	c.orderBooksMu.Unlock()

	if ConnectionState(c.state.Load()) == StateConnected {
		// Send subscription with data_type
		c.connMu.Lock()
		defer c.connMu.Unlock()

		if c.conn == nil {
			return fmt.Errorf("not connected")
		}

		req := WSRequest{
			Sub:      topic,
			DataType: "incremental",
			ID:       fmt.Sprintf("sub_%d", time.Now().UnixNano()),
		}

		data, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("marshal subscription: %w", err)
		}

		if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return fmt.Errorf("send subscription: %w", err)
		}
	}
	return nil
}

// UnsubscribeDepth unsubscribes from depth data
func (c *WSMarketDataClient) UnsubscribeDepth(symbol, depthType string) error {
	topic := fmt.Sprintf("market.%s.depth.%s", symbol, depthType)
	c.subscriptions.Remove(topic)

	c.orderBooksMu.Lock()
	delete(c.orderBooks, symbol)
	c.orderBooksMu.Unlock()

	return c.sendUnsubscription(topic)
}

// SubscribeBBO subscribes to BBO data
func (c *WSMarketDataClient) SubscribeBBO(symbol string, callback func(data []byte)) error {
	topic := fmt.Sprintf("market.%s.bbo", symbol)
	c.subscriptions.Add(topic, callback)

	if ConnectionState(c.state.Load()) == StateConnected {
		return c.sendSubscription(topic)
	}
	return nil
}

// UnsubscribeBBO unsubscribes from BBO data
func (c *WSMarketDataClient) UnsubscribeBBO(symbol string) error {
	topic := fmt.Sprintf("market.%s.bbo", symbol)
	c.subscriptions.Remove(topic)
	return c.sendUnsubscription(topic)
}

// SubscribeTrade subscribes to trade data
func (c *WSMarketDataClient) SubscribeTrade(symbol string, callback func(data []byte)) error {
	topic := fmt.Sprintf("market.%s.trade.detail", symbol)
	c.subscriptions.Add(topic, callback)

	if ConnectionState(c.state.Load()) == StateConnected {
		return c.sendSubscription(topic)
	}
	return nil
}

// UnsubscribeTrade unsubscribes from trade data
func (c *WSMarketDataClient) UnsubscribeTrade(symbol string) error {
	topic := fmt.Sprintf("market.%s.trade.detail", symbol)
	c.subscriptions.Remove(topic)
	return c.sendUnsubscription(topic)
}

// SubscribeTicker subscribes to ticker data
func (c *WSMarketDataClient) SubscribeTicker(symbol string, callback func(data []byte)) error {
	topic := fmt.Sprintf("market.%s.detail", symbol)
	c.subscriptions.Add(topic, callback)

	if ConnectionState(c.state.Load()) == StateConnected {
		return c.sendSubscription(topic)
	}
	return nil
}

// UnsubscribeTicker unsubscribes from ticker data
func (c *WSMarketDataClient) UnsubscribeTicker(symbol string) error {
	topic := fmt.Sprintf("market.%s.detail", symbol)
	c.subscriptions.Remove(topic)
	return c.sendUnsubscription(topic)
}

// GetOrderBook returns the local order book for a symbol
func (c *WSMarketDataClient) GetOrderBook(symbol string) (*OrderBook, bool) {
	c.orderBooksMu.RLock()
	defer c.orderBooksMu.RUnlock()
	ob, ok := c.orderBooks[symbol]
	return ob, ok
}

// RequestKline requests historical kline data via WebSocket
func (c *WSMarketDataClient) RequestKline(symbol, period string, from, to int64, callback func(data []byte)) error {
	topic := fmt.Sprintf("market.%s.kline.%s", symbol, period)
	c.subscriptions.Add(topic, callback)

	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	req := map[string]interface{}{
		"req":  topic,
		"id":   fmt.Sprintf("req_%d", time.Now().UnixNano()),
		"from": from,
		"to":   to,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	return nil
}
