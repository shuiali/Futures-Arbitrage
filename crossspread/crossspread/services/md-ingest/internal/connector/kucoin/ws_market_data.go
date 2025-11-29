// Package kucoin provides WebSocket market data client for KuCoin Futures.
package kucoin

import (
	"context"
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
	OnTicker       func(symbol string, ticker *WSTickerData)
	OnOrderBook    func(symbol string, orderbook *WSLevel2Data)
	OnLevel2Change func(symbol string, change *WSLevel2Change)
	OnExecution    func(symbol string, trade *WSExecutionData)
	OnMarkPrice    func(symbol string, data *WSInstrumentMarkPrice)
	OnFundingRate  func(symbol string, data *WSInstrumentFundingRate)
	OnError        func(err error)
	OnConnect      func()
	OnDisconnect   func(err error)
}

// WSMarketDataClient handles WebSocket market data connections for KuCoin
type WSMarketDataClient struct {
	restClient     *RESTClient
	handler        *WSMarketDataHandler
	conn           *websocket.Conn
	subscriptions  map[string]bool // topic -> subscribed
	mu             sync.RWMutex
	writeMu        sync.Mutex
	reconnectDelay time.Duration
	maxRetries     int
	ctx            context.Context
	cancel         context.CancelFunc
	isConnected    atomic.Bool
	pingInterval   time.Duration
	pingTimeout    time.Duration
	msgID          atomic.Int64
	stopPing       chan struct{}
	token          *WSToken
}

// NewWSMarketDataClient creates a new WebSocket market data client
func NewWSMarketDataClient(restClient *RESTClient, handler *WSMarketDataHandler) *WSMarketDataClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &WSMarketDataClient{
		restClient:     restClient,
		handler:        handler,
		subscriptions:  make(map[string]bool),
		reconnectDelay: 5 * time.Second,
		maxRetries:     10,
		ctx:            ctx,
		cancel:         cancel,
		pingInterval:   18 * time.Second, // Default, will be updated from token
		pingTimeout:    10 * time.Second,
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
	// Get connection token from REST API
	token, err := c.restClient.GetPublicToken(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to get WebSocket token: %w", err)
	}
	c.token = token

	if len(token.InstanceServers) == 0 {
		return fmt.Errorf("no WebSocket servers available")
	}

	server := token.InstanceServers[0]

	// Update ping settings from server config
	if server.PingInterval > 0 {
		c.pingInterval = time.Duration(server.PingInterval) * time.Millisecond
	}
	if server.PingTimeout > 0 {
		c.pingTimeout = time.Duration(server.PingTimeout) * time.Millisecond
	}

	// Build WebSocket URL with token
	connectID := GenerateClientOid()
	wsURL := fmt.Sprintf("%s?token=%s&connectId=%s", server.Endpoint, token.Token, connectID)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(c.ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to KuCoin WS: %w", err)
	}

	c.conn = conn
	c.isConnected.Store(true)
	c.stopPing = make(chan struct{})

	// Wait for welcome message
	if err := c.waitForWelcome(); err != nil {
		c.conn.Close()
		c.isConnected.Store(false)
		return fmt.Errorf("failed to receive welcome message: %w", err)
	}

	// Start message handler
	go c.readLoop()

	// Start ping loop
	go c.pingLoop()

	if c.handler != nil && c.handler.OnConnect != nil {
		c.handler.OnConnect()
	}

	log.Printf("[KuCoin WS] Connected to public market data")
	return nil
}

// waitForWelcome waits for the welcome message from KuCoin
func (c *WSMarketDataClient) waitForWelcome() error {
	c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer c.conn.SetReadDeadline(time.Time{})

	_, message, err := c.conn.ReadMessage()
	if err != nil {
		return err
	}

	var msg WSMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		return err
	}

	if msg.Type != WSTypeWelcome {
		return fmt.Errorf("expected welcome message, got: %s", msg.Type)
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
		case <-c.ctx.Done():
			return
		default:
		}

		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[KuCoin WS] Read error: %v", err)
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
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if err := c.sendPing(); err != nil {
				log.Printf("[KuCoin WS] Ping error: %v", err)
				return
			}
		}
	}
}

// sendPing sends a ping message
func (c *WSMarketDataClient) sendPing() error {
	msg := WSPingMessage{
		ID:   c.nextMsgID(),
		Type: WSTypePing,
	}
	return c.sendMessage(msg)
}

// nextMsgID generates a unique message ID
func (c *WSMarketDataClient) nextMsgID() string {
	return fmt.Sprintf("%d", c.msgID.Add(1))
}

// handleMessage processes incoming WebSocket messages
func (c *WSMarketDataClient) handleMessage(data []byte) {
	var msg WSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[KuCoin WS] Failed to parse message: %v", err)
		return
	}

	switch msg.Type {
	case WSTypePong:
		// Pong received, connection is alive
	case WSTypeAck:
		c.handleAckResponse(&msg)
	case WSTypeMessage:
		c.handleDataMessage(&msg)
	case WSTypeError:
		log.Printf("[KuCoin WS] Error message: %s", string(msg.Data))
		if c.handler != nil && c.handler.OnError != nil {
			c.handler.OnError(fmt.Errorf("WebSocket error: %s", string(msg.Data)))
		}
	default:
		log.Printf("[KuCoin WS] Unknown message type: %s", msg.Type)
	}
}

func (c *WSMarketDataClient) handleAckResponse(msg *WSMessage) {
	log.Printf("[KuCoin WS] Subscription acknowledged: %s", msg.ID)
}

func (c *WSMarketDataClient) handleDataMessage(msg *WSMessage) {
	if c.handler == nil {
		return
	}

	// Extract symbol from topic (e.g., /contractMarket/ticker:XBTUSDTM)
	symbol := extractSymbolFromTopic(msg.Topic)

	switch {
	case containsTopic(msg.Topic, WSTopicTicker):
		c.handleTickerUpdate(symbol, msg.Data)
	case containsTopic(msg.Topic, WSTopicLevel2Depth5), containsTopic(msg.Topic, WSTopicLevel2Depth50):
		c.handleOrderBookUpdate(symbol, msg.Data)
	case containsTopic(msg.Topic, WSTopicLevel2):
		c.handleLevel2Change(symbol, msg.Data)
	case containsTopic(msg.Topic, WSTopicExecution):
		c.handleExecutionUpdate(symbol, msg.Data)
	case containsTopic(msg.Topic, WSTopicInstrument):
		c.handleInstrumentUpdate(symbol, msg.Subject, msg.Data)
	default:
		log.Printf("[KuCoin WS] Unhandled topic: %s", msg.Topic)
	}
}

func (c *WSMarketDataClient) handleTickerUpdate(symbol string, data json.RawMessage) {
	if c.handler.OnTicker == nil {
		return
	}

	var ticker WSTickerData
	if err := json.Unmarshal(data, &ticker); err != nil {
		log.Printf("[KuCoin WS] Failed to parse ticker: %v", err)
		return
	}

	c.handler.OnTicker(symbol, &ticker)
}

func (c *WSMarketDataClient) handleOrderBookUpdate(symbol string, data json.RawMessage) {
	if c.handler.OnOrderBook == nil {
		return
	}

	var orderbook WSLevel2Data
	if err := json.Unmarshal(data, &orderbook); err != nil {
		log.Printf("[KuCoin WS] Failed to parse orderbook: %v", err)
		return
	}

	c.handler.OnOrderBook(symbol, &orderbook)
}

func (c *WSMarketDataClient) handleLevel2Change(symbol string, data json.RawMessage) {
	if c.handler.OnLevel2Change == nil {
		return
	}

	var change WSLevel2Change
	if err := json.Unmarshal(data, &change); err != nil {
		log.Printf("[KuCoin WS] Failed to parse level2 change: %v", err)
		return
	}

	c.handler.OnLevel2Change(symbol, &change)
}

func (c *WSMarketDataClient) handleExecutionUpdate(symbol string, data json.RawMessage) {
	if c.handler.OnExecution == nil {
		return
	}

	var execution WSExecutionData
	if err := json.Unmarshal(data, &execution); err != nil {
		log.Printf("[KuCoin WS] Failed to parse execution: %v", err)
		return
	}

	c.handler.OnExecution(symbol, &execution)
}

func (c *WSMarketDataClient) handleInstrumentUpdate(symbol, subject string, data json.RawMessage) {
	switch subject {
	case "mark.index.price":
		if c.handler.OnMarkPrice == nil {
			return
		}
		var markPrice WSInstrumentMarkPrice
		if err := json.Unmarshal(data, &markPrice); err != nil {
			log.Printf("[KuCoin WS] Failed to parse mark price: %v", err)
			return
		}
		c.handler.OnMarkPrice(symbol, &markPrice)
	case "funding.rate":
		if c.handler.OnFundingRate == nil {
			return
		}
		var fundingRate WSInstrumentFundingRate
		if err := json.Unmarshal(data, &fundingRate); err != nil {
			log.Printf("[KuCoin WS] Failed to parse funding rate: %v", err)
			return
		}
		c.handler.OnFundingRate(symbol, &fundingRate)
	default:
		log.Printf("[KuCoin WS] Unhandled instrument subject: %s", subject)
	}
}

// handleReconnect attempts to reconnect after disconnection
func (c *WSMarketDataClient) handleReconnect() {
	c.mu.Lock()
	// Save subscriptions to restore
	subs := make(map[string]bool)
	for k, v := range c.subscriptions {
		subs[k] = v
	}
	c.mu.Unlock()

	for retry := 0; retry < c.maxRetries; retry++ {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(c.reconnectDelay):
		}

		log.Printf("[KuCoin WS] Reconnecting (attempt %d/%d)", retry+1, c.maxRetries)

		c.mu.Lock()
		err := c.connectInternal()
		c.mu.Unlock()

		if err == nil {
			// Resubscribe to previous topics
			c.resubscribe(subs)
			return
		}
		log.Printf("[KuCoin WS] Reconnect failed: %v", err)
	}

	log.Printf("[KuCoin WS] Max reconnect attempts reached")
	if c.handler != nil && c.handler.OnError != nil {
		c.handler.OnError(fmt.Errorf("max reconnect attempts reached"))
	}
}

// resubscribe restores subscriptions after reconnect
func (c *WSMarketDataClient) resubscribe(subs map[string]bool) {
	for topic := range subs {
		if err := c.subscribe(topic, false); err != nil {
			log.Printf("[KuCoin WS] Failed to resubscribe to %s: %v", topic, err)
		}
	}
}

// sendMessage sends a message to the WebSocket connection
func (c *WSMarketDataClient) sendMessage(msg interface{}) error {
	if !c.isConnected.Load() {
		return fmt.Errorf("not connected")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// subscribe sends a subscription request
func (c *WSMarketDataClient) subscribe(topic string, privateChannel bool) error {
	msg := WSSubscribeRequest{
		ID:             c.nextMsgID(),
		Type:           WSTypeSubscribe,
		Topic:          topic,
		PrivateChannel: privateChannel,
		Response:       true,
	}

	if err := c.sendMessage(msg); err != nil {
		return err
	}

	// Track subscription
	c.mu.Lock()
	c.subscriptions[topic] = true
	c.mu.Unlock()

	return nil
}

// unsubscribe sends an unsubscription request
func (c *WSMarketDataClient) unsubscribe(topic string, privateChannel bool) error {
	msg := WSSubscribeRequest{
		ID:             c.nextMsgID(),
		Type:           WSTypeUnsubscribe,
		Topic:          topic,
		PrivateChannel: privateChannel,
		Response:       true,
	}

	if err := c.sendMessage(msg); err != nil {
		return err
	}

	// Remove from tracked subscriptions
	c.mu.Lock()
	delete(c.subscriptions, topic)
	c.mu.Unlock()

	return nil
}

// SubscribeTicker subscribes to ticker updates for a symbol
func (c *WSMarketDataClient) SubscribeTicker(symbol string) error {
	if err := c.Connect(); err != nil {
		return err
	}
	topic := fmt.Sprintf("%s:%s", WSTopicTicker, symbol)
	return c.subscribe(topic, false)
}

// UnsubscribeTicker unsubscribes from ticker updates
func (c *WSMarketDataClient) UnsubscribeTicker(symbol string) error {
	topic := fmt.Sprintf("%s:%s", WSTopicTicker, symbol)
	return c.unsubscribe(topic, false)
}

// SubscribeTickers subscribes to ticker updates for multiple symbols
func (c *WSMarketDataClient) SubscribeTickers(symbols []string) error {
	if err := c.Connect(); err != nil {
		return err
	}
	for _, symbol := range symbols {
		topic := fmt.Sprintf("%s:%s", WSTopicTicker, symbol)
		if err := c.subscribe(topic, false); err != nil {
			return err
		}
	}
	return nil
}

// SubscribeOrderBook5 subscribes to 5-level orderbook depth
func (c *WSMarketDataClient) SubscribeOrderBook5(symbol string) error {
	if err := c.Connect(); err != nil {
		return err
	}
	topic := fmt.Sprintf("%s:%s", WSTopicLevel2Depth5, symbol)
	return c.subscribe(topic, false)
}

// UnsubscribeOrderBook5 unsubscribes from 5-level orderbook
func (c *WSMarketDataClient) UnsubscribeOrderBook5(symbol string) error {
	topic := fmt.Sprintf("%s:%s", WSTopicLevel2Depth5, symbol)
	return c.unsubscribe(topic, false)
}

// SubscribeOrderBook50 subscribes to 50-level orderbook depth
func (c *WSMarketDataClient) SubscribeOrderBook50(symbol string) error {
	if err := c.Connect(); err != nil {
		return err
	}
	topic := fmt.Sprintf("%s:%s", WSTopicLevel2Depth50, symbol)
	return c.subscribe(topic, false)
}

// UnsubscribeOrderBook50 unsubscribes from 50-level orderbook
func (c *WSMarketDataClient) UnsubscribeOrderBook50(symbol string) error {
	topic := fmt.Sprintf("%s:%s", WSTopicLevel2Depth50, symbol)
	return c.unsubscribe(topic, false)
}

// SubscribeLevel2Increment subscribes to incremental L2 updates
func (c *WSMarketDataClient) SubscribeLevel2Increment(symbol string) error {
	if err := c.Connect(); err != nil {
		return err
	}
	topic := fmt.Sprintf("%s:%s", WSTopicLevel2, symbol)
	return c.subscribe(topic, false)
}

// UnsubscribeLevel2Increment unsubscribes from incremental L2 updates
func (c *WSMarketDataClient) UnsubscribeLevel2Increment(symbol string) error {
	topic := fmt.Sprintf("%s:%s", WSTopicLevel2, symbol)
	return c.unsubscribe(topic, false)
}

// SubscribeExecution subscribes to trade execution updates
func (c *WSMarketDataClient) SubscribeExecution(symbol string) error {
	if err := c.Connect(); err != nil {
		return err
	}
	topic := fmt.Sprintf("%s:%s", WSTopicExecution, symbol)
	return c.subscribe(topic, false)
}

// UnsubscribeExecution unsubscribes from trade execution updates
func (c *WSMarketDataClient) UnsubscribeExecution(symbol string) error {
	topic := fmt.Sprintf("%s:%s", WSTopicExecution, symbol)
	return c.unsubscribe(topic, false)
}

// SubscribeInstrument subscribes to instrument info (mark price, index, funding)
func (c *WSMarketDataClient) SubscribeInstrument(symbol string) error {
	if err := c.Connect(); err != nil {
		return err
	}
	topic := fmt.Sprintf("%s:%s", WSTopicInstrument, symbol)
	return c.subscribe(topic, false)
}

// UnsubscribeInstrument unsubscribes from instrument info
func (c *WSMarketDataClient) UnsubscribeInstrument(symbol string) error {
	topic := fmt.Sprintf("%s:%s", WSTopicInstrument, symbol)
	return c.unsubscribe(topic, false)
}

// Close closes the WebSocket connection
func (c *WSMarketDataClient) Close() error {
	c.cancel()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.writeMu.Lock()
		c.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.conn.Close()
		c.writeMu.Unlock()
		c.conn = nil
	}

	c.isConnected.Store(false)
	return nil
}

// IsConnected checks if connected
func (c *WSMarketDataClient) IsConnected() bool {
	return c.isConnected.Load()
}

// GetSubscriptions returns current subscriptions
func (c *WSMarketDataClient) GetSubscriptions() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var subs []string
	for topic := range c.subscriptions {
		subs = append(subs, topic)
	}
	return subs
}

// =============================================================================
// Helper functions
// =============================================================================

// extractSymbolFromTopic extracts symbol from topic string
// e.g., "/contractMarket/ticker:XBTUSDTM" -> "XBTUSDTM"
func extractSymbolFromTopic(topic string) string {
	idx := len(topic) - 1
	for i := len(topic) - 1; i >= 0; i-- {
		if topic[i] == ':' {
			return topic[i+1:]
		}
		if topic[i] == '/' {
			break
		}
		idx = i
	}
	return topic[idx:]
}

// containsTopic checks if topic contains a specific prefix
func containsTopic(topic, prefix string) bool {
	return len(topic) >= len(prefix) && topic[:len(prefix)] == prefix
}
