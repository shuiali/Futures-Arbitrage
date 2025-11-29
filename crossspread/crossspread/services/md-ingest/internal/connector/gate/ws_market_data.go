package gate

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSMarketDataHandler handles market data callbacks
type WSMarketDataHandler struct {
	OnTicker     func(settle string, ticker *WSTickerData)
	OnOrderBook  func(settle string, orderbook *WSOrderBookData)
	OnTrade      func(settle string, trade *WSTradeData)
	OnBookTicker func(settle string, bookTicker *WSBookTickerData)
	OnKline      func(settle string, kline *WSKlineData)
	OnError      func(err error)
	OnConnect    func(settle string)
	OnDisconnect func(settle string, err error)
}

// WSMarketDataClient handles WebSocket market data connections
type WSMarketDataClient struct {
	baseURL        string
	handler        *WSMarketDataHandler
	connections    map[string]*wsConnection   // settle -> connection
	subscriptions  map[string]map[string]bool // settle -> channel:payload -> subscribed
	mu             sync.RWMutex
	reconnectDelay time.Duration
	maxRetries     int
	ctx            context.Context
	cancel         context.CancelFunc
}

// wsConnection represents a single WebSocket connection
type wsConnection struct {
	conn        *websocket.Conn
	settle      string
	mu          sync.Mutex
	writeMu     sync.Mutex
	isConnected bool
	lastPing    time.Time
	stopPing    chan struct{}
}

// NewWSMarketDataClient creates a new WebSocket market data client
func NewWSMarketDataClient(baseURL string, handler *WSMarketDataHandler) *WSMarketDataClient {
	if baseURL == "" {
		baseURL = "wss://fx-ws.gateio.ws/v4/ws"
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &WSMarketDataClient{
		baseURL:        baseURL,
		handler:        handler,
		connections:    make(map[string]*wsConnection),
		subscriptions:  make(map[string]map[string]bool),
		reconnectDelay: 5 * time.Second,
		maxRetries:     10,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Connect establishes WebSocket connection for a specific settle currency
func (c *WSMarketDataClient) Connect(settle string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if conn, exists := c.connections[settle]; exists && conn.isConnected {
		return nil // Already connected
	}

	return c.connectInternal(settle)
}

func (c *WSMarketDataClient) connectInternal(settle string) error {
	url := fmt.Sprintf("%s/%s", c.baseURL, settle)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(c.ctx, url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Gate.io WS (%s): %w", settle, err)
	}

	wsConn := &wsConnection{
		conn:        conn,
		settle:      settle,
		isConnected: true,
		stopPing:    make(chan struct{}),
	}
	c.connections[settle] = wsConn

	if c.subscriptions[settle] == nil {
		c.subscriptions[settle] = make(map[string]bool)
	}

	// Start message handler
	go c.readLoop(wsConn)

	// Start ping loop
	go c.pingLoop(wsConn)

	if c.handler != nil && c.handler.OnConnect != nil {
		c.handler.OnConnect(settle)
	}

	log.Printf("[Gate.io WS] Connected to %s", settle)
	return nil
}

// readLoop reads messages from the WebSocket connection
func (c *WSMarketDataClient) readLoop(wsConn *wsConnection) {
	defer func() {
		wsConn.mu.Lock()
		wsConn.isConnected = false
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
				log.Printf("[Gate.io WS] Read error (%s): %v", wsConn.settle, err)
			}
			c.handleReconnect(wsConn.settle)
			return
		}

		c.handleMessage(wsConn.settle, message)
	}
}

// pingLoop sends periodic pings to keep connection alive
func (c *WSMarketDataClient) pingLoop(wsConn *wsConnection) {
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
				log.Printf("[Gate.io WS] Ping error (%s): %v", wsConn.settle, err)
				return
			}
			wsConn.lastPing = time.Now()
		}
	}
}

// handleMessage processes incoming WebSocket messages
func (c *WSMarketDataClient) handleMessage(settle string, data []byte) {
	var msg WSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[Gate.io WS] Failed to parse message: %v", err)
		return
	}

	// Handle different message events
	switch msg.Event {
	case "subscribe":
		c.handleSubscribeResponse(settle, &msg)
	case "unsubscribe":
		c.handleUnsubscribeResponse(settle, &msg)
	case "update", "all":
		// "all" is sent for full orderbook snapshots, "update" for incremental
		c.handleUpdateMessage(settle, &msg)
	default:
		// Only log truly unknown events, not common ones
		if msg.Event != "" {
			log.Printf("[Gate.io WS] Unknown event: %s", msg.Event)
		}
	}
}

func (c *WSMarketDataClient) handleSubscribeResponse(settle string, msg *WSMessage) {
	if msg.Error != nil {
		log.Printf("[Gate.io WS] Subscribe error (%s): code=%d, msg=%s",
			settle, msg.Error.Code, msg.Error.Message)
		if c.handler != nil && c.handler.OnError != nil {
			c.handler.OnError(fmt.Errorf("subscribe error: %s", msg.Error.Message))
		}
		return
	}
	log.Printf("[Gate.io WS] Subscribed to %s on %s", msg.Channel, settle)
}

func (c *WSMarketDataClient) handleUnsubscribeResponse(settle string, msg *WSMessage) {
	log.Printf("[Gate.io WS] Unsubscribed from %s on %s", msg.Channel, settle)
}

func (c *WSMarketDataClient) handleUpdateMessage(settle string, msg *WSMessage) {
	if c.handler == nil {
		return
	}

	switch msg.Channel {
	case "futures.tickers":
		c.handleTickerUpdate(settle, msg.Result)
	case "futures.order_book":
		c.handleOrderBookUpdate(settle, msg.Result)
	case "futures.order_book_update":
		c.handleOrderBookDeltaUpdate(settle, msg.Result)
	case "futures.trades":
		c.handleTradeUpdate(settle, msg.Result)
	case "futures.book_ticker":
		c.handleBookTickerUpdate(settle, msg.Result)
	case "futures.candlesticks":
		c.handleKlineUpdate(settle, msg.Result)
	default:
		log.Printf("[Gate.io WS] Unhandled channel: %s", msg.Channel)
	}
}

func (c *WSMarketDataClient) handleTickerUpdate(settle string, data json.RawMessage) {
	if c.handler.OnTicker == nil {
		return
	}

	var tickers []WSTickerData
	if err := json.Unmarshal(data, &tickers); err != nil {
		log.Printf("[Gate.io WS] Failed to parse ticker: %v", err)
		return
	}

	for _, ticker := range tickers {
		c.handler.OnTicker(settle, &ticker)
	}
}

func (c *WSMarketDataClient) handleOrderBookUpdate(settle string, data json.RawMessage) {
	if c.handler.OnOrderBook == nil {
		return
	}

	var orderbook WSOrderBookData
	if err := json.Unmarshal(data, &orderbook); err != nil {
		log.Printf("[Gate.io WS] Failed to parse orderbook: %v", err)
		return
	}

	c.handler.OnOrderBook(settle, &orderbook)
}

func (c *WSMarketDataClient) handleOrderBookDeltaUpdate(settle string, data json.RawMessage) {
	if c.handler.OnOrderBook == nil {
		return
	}

	// Delta updates have the same structure
	var orderbook WSOrderBookData
	if err := json.Unmarshal(data, &orderbook); err != nil {
		log.Printf("[Gate.io WS] Failed to parse orderbook delta: %v", err)
		return
	}

	c.handler.OnOrderBook(settle, &orderbook)
}

func (c *WSMarketDataClient) handleTradeUpdate(settle string, data json.RawMessage) {
	if c.handler.OnTrade == nil {
		return
	}

	var trades []WSTradeData
	if err := json.Unmarshal(data, &trades); err != nil {
		log.Printf("[Gate.io WS] Failed to parse trade: %v", err)
		return
	}

	for _, trade := range trades {
		c.handler.OnTrade(settle, &trade)
	}
}

func (c *WSMarketDataClient) handleBookTickerUpdate(settle string, data json.RawMessage) {
	if c.handler.OnBookTicker == nil {
		return
	}

	var bookTicker WSBookTickerData
	if err := json.Unmarshal(data, &bookTicker); err != nil {
		log.Printf("[Gate.io WS] Failed to parse book ticker: %v", err)
		return
	}

	c.handler.OnBookTicker(settle, &bookTicker)
}

func (c *WSMarketDataClient) handleKlineUpdate(settle string, data json.RawMessage) {
	if c.handler.OnKline == nil {
		return
	}

	var klines []WSKlineData
	if err := json.Unmarshal(data, &klines); err != nil {
		log.Printf("[Gate.io WS] Failed to parse kline: %v", err)
		return
	}

	for _, kline := range klines {
		c.handler.OnKline(settle, &kline)
	}
}

// handleReconnect attempts to reconnect after disconnection
func (c *WSMarketDataClient) handleReconnect(settle string) {
	c.mu.Lock()

	// Get subscriptions to restore
	subs := make(map[string]bool)
	if existing, ok := c.subscriptions[settle]; ok {
		for k, v := range existing {
			subs[k] = v
		}
	}

	// Remove old connection
	delete(c.connections, settle)
	c.mu.Unlock()

	for retry := 0; retry < c.maxRetries; retry++ {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(c.reconnectDelay):
		}

		log.Printf("[Gate.io WS] Reconnecting to %s (attempt %d/%d)", settle, retry+1, c.maxRetries)

		c.mu.Lock()
		err := c.connectInternal(settle)
		c.mu.Unlock()

		if err == nil {
			// Resubscribe to previous channels
			c.resubscribe(settle, subs)
			return
		}
		log.Printf("[Gate.io WS] Reconnect failed: %v", err)
	}

	log.Printf("[Gate.io WS] Max reconnect attempts reached for %s", settle)
	if c.handler != nil && c.handler.OnError != nil {
		c.handler.OnError(fmt.Errorf("max reconnect attempts reached for %s", settle))
	}
}

// resubscribe restores subscriptions after reconnect
func (c *WSMarketDataClient) resubscribe(settle string, subs map[string]bool) {
	for subKey := range subs {
		// Parse the subscription key to get channel and payload
		var sub struct {
			Channel string   `json:"channel"`
			Payload []string `json:"payload"`
		}
		if err := json.Unmarshal([]byte(subKey), &sub); err != nil {
			continue
		}

		if err := c.subscribe(settle, sub.Channel, sub.Payload); err != nil {
			log.Printf("[Gate.io WS] Failed to resubscribe to %s: %v", sub.Channel, err)
		}
	}
}

// sendMessage sends a message to the WebSocket connection
func (c *WSMarketDataClient) sendMessage(settle string, msg interface{}) error {
	c.mu.RLock()
	wsConn, ok := c.connections[settle]
	c.mu.RUnlock()

	if !ok || !wsConn.isConnected {
		return fmt.Errorf("not connected to %s", settle)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	wsConn.writeMu.Lock()
	defer wsConn.writeMu.Unlock()
	return wsConn.conn.WriteMessage(websocket.TextMessage, data)
}

// subscribe sends a subscription request
func (c *WSMarketDataClient) subscribe(settle string, channel string, payload []string) error {
	msg := WSSubscription{
		Time:    time.Now().Unix(),
		Channel: channel,
		Event:   "subscribe",
		Payload: payload,
	}

	if err := c.sendMessage(settle, msg); err != nil {
		return err
	}

	// Track subscription
	c.mu.Lock()
	if c.subscriptions[settle] == nil {
		c.subscriptions[settle] = make(map[string]bool)
	}
	subKey, _ := json.Marshal(struct {
		Channel string   `json:"channel"`
		Payload []string `json:"payload"`
	}{channel, payload})
	c.subscriptions[settle][string(subKey)] = true
	c.mu.Unlock()

	return nil
}

// unsubscribe sends an unsubscription request
func (c *WSMarketDataClient) unsubscribe(settle string, channel string, payload []string) error {
	msg := WSSubscription{
		Time:    time.Now().Unix(),
		Channel: channel,
		Event:   "unsubscribe",
		Payload: payload,
	}

	if err := c.sendMessage(settle, msg); err != nil {
		return err
	}

	// Remove from tracked subscriptions
	c.mu.Lock()
	subKey, _ := json.Marshal(struct {
		Channel string   `json:"channel"`
		Payload []string `json:"payload"`
	}{channel, payload})
	delete(c.subscriptions[settle], string(subKey))
	c.mu.Unlock()

	return nil
}

// SubscribeTickers subscribes to ticker updates for specific contracts
func (c *WSMarketDataClient) SubscribeTickers(settle string, contracts []string) error {
	if err := c.Connect(settle); err != nil {
		return err
	}
	return c.subscribe(settle, "futures.tickers", contracts)
}

// UnsubscribeTickers unsubscribes from ticker updates
func (c *WSMarketDataClient) UnsubscribeTickers(settle string, contracts []string) error {
	return c.unsubscribe(settle, "futures.tickers", contracts)
}

// SubscribeOrderBook subscribes to orderbook snapshots
// level: depth levels, e.g., "20", "100"
// interval: "0" for legacy order_book channel
func (c *WSMarketDataClient) SubscribeOrderBook(settle string, contract string, level string, interval string) error {
	if err := c.Connect(settle); err != nil {
		return err
	}
	// Gate.io API expects: contract, level, interval (per official docs)
	// Example: ["BTC_USD", "20", "0"]
	payload := []string{contract, level, interval}
	return c.subscribe(settle, "futures.order_book", payload)
}

// UnsubscribeOrderBook unsubscribes from orderbook snapshots
func (c *WSMarketDataClient) UnsubscribeOrderBook(settle string, contract string, level string, interval string) error {
	// Gate.io API expects: contract, level, interval (per official docs)
	payload := []string{contract, level, interval}
	return c.unsubscribe(settle, "futures.order_book", payload)
}

// SubscribeOrderBookUpdate subscribes to incremental orderbook updates
// interval: "100ms", "1000ms"
func (c *WSMarketDataClient) SubscribeOrderBookUpdate(settle string, contract string, interval string) error {
	if err := c.Connect(settle); err != nil {
		return err
	}
	payload := []string{contract, interval}
	return c.subscribe(settle, "futures.order_book_update", payload)
}

// UnsubscribeOrderBookUpdate unsubscribes from incremental updates
func (c *WSMarketDataClient) UnsubscribeOrderBookUpdate(settle string, contract string, interval string) error {
	payload := []string{contract, interval}
	return c.unsubscribe(settle, "futures.order_book_update", payload)
}

// SubscribeTrades subscribes to trade updates
func (c *WSMarketDataClient) SubscribeTrades(settle string, contracts []string) error {
	if err := c.Connect(settle); err != nil {
		return err
	}
	return c.subscribe(settle, "futures.trades", contracts)
}

// UnsubscribeTrades unsubscribes from trade updates
func (c *WSMarketDataClient) UnsubscribeTrades(settle string, contracts []string) error {
	return c.unsubscribe(settle, "futures.trades", contracts)
}

// SubscribeBookTicker subscribes to best bid/ask updates (high frequency)
func (c *WSMarketDataClient) SubscribeBookTicker(settle string, contracts []string) error {
	if err := c.Connect(settle); err != nil {
		return err
	}
	return c.subscribe(settle, "futures.book_ticker", contracts)
}

// UnsubscribeBookTicker unsubscribes from best bid/ask updates
func (c *WSMarketDataClient) UnsubscribeBookTicker(settle string, contracts []string) error {
	return c.unsubscribe(settle, "futures.book_ticker", contracts)
}

// SubscribeCandlesticks subscribes to candlestick updates
// interval: "10s", "1m", "5m", "15m", "30m", "1h", "4h", "8h", "1d", "7d", "30d"
func (c *WSMarketDataClient) SubscribeCandlesticks(settle string, contract string, interval string) error {
	if err := c.Connect(settle); err != nil {
		return err
	}
	payload := []string{interval, contract}
	return c.subscribe(settle, "futures.candlesticks", payload)
}

// UnsubscribeCandlesticks unsubscribes from candlestick updates
func (c *WSMarketDataClient) UnsubscribeCandlesticks(settle string, contract string, interval string) error {
	payload := []string{interval, contract}
	return c.unsubscribe(settle, "futures.candlesticks", payload)
}

// SubscribeAllTickers subscribes to all ticker updates for a settle currency
func (c *WSMarketDataClient) SubscribeAllTickers(settle string) error {
	return c.SubscribeTickers(settle, []string{"!all"})
}

// Close closes all WebSocket connections
func (c *WSMarketDataClient) Close() error {
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

	return nil
}

// IsConnected checks if connected to a specific settle currency
func (c *WSMarketDataClient) IsConnected(settle string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if wsConn, ok := c.connections[settle]; ok {
		return wsConn.isConnected
	}
	return false
}

// GetSubscriptions returns current subscriptions for a settle currency
func (c *WSMarketDataClient) GetSubscriptions(settle string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var subs []string
	if settleSubs, ok := c.subscriptions[settle]; ok {
		for sub := range settleSubs {
			subs = append(subs, sub)
		}
	}
	return subs
}
