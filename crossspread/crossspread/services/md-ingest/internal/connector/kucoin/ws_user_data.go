// Package kucoin provides WebSocket user data client for KuCoin Futures.
// Handles private channels for order updates, position changes, and balance updates.
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

// WSUserDataHandler handles user data callbacks
type WSUserDataHandler struct {
	OnOrderChange    func(order *WSOrderChange)
	OnPositionChange func(symbol string, position *WSPositionChange)
	OnBalanceChange  func(balance *WSBalanceChange)
	OnError          func(err error)
	OnConnect        func()
	OnDisconnect     func(err error)
}

// WSUserDataClient handles WebSocket user data streams for KuCoin
type WSUserDataClient struct {
	restClient     *RESTClient
	handler        *WSUserDataHandler
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

// NewWSUserDataClient creates a new WebSocket user data client
func NewWSUserDataClient(restClient *RESTClient, handler *WSUserDataHandler) *WSUserDataClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &WSUserDataClient{
		restClient:     restClient,
		handler:        handler,
		subscriptions:  make(map[string]bool),
		reconnectDelay: 5 * time.Second,
		maxRetries:     10,
		ctx:            ctx,
		cancel:         cancel,
		pingInterval:   18 * time.Second,
		pingTimeout:    10 * time.Second,
	}
}

// Connect establishes WebSocket connection for private user data
func (c *WSUserDataClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isConnected.Load() {
		return nil
	}

	return c.connectInternal()
}

func (c *WSUserDataClient) connectInternal() error {
	// Get private connection token (requires authentication)
	token, err := c.restClient.GetPrivateToken(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to get private WebSocket token: %w", err)
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
		return fmt.Errorf("failed to connect to KuCoin private WS: %w", err)
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

	log.Printf("[KuCoin WS] Connected to private user data")
	return nil
}

// waitForWelcome waits for the welcome message from KuCoin
func (c *WSUserDataClient) waitForWelcome() error {
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
func (c *WSUserDataClient) readLoop() {
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
				log.Printf("[KuCoin User WS] Read error: %v", err)
			}
			c.handleReconnect()
			return
		}

		c.handleMessage(message)
	}
}

// pingLoop sends periodic pings to keep connection alive
func (c *WSUserDataClient) pingLoop() {
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
				log.Printf("[KuCoin User WS] Ping error: %v", err)
				return
			}
		}
	}
}

// sendPing sends a ping message
func (c *WSUserDataClient) sendPing() error {
	msg := WSPingMessage{
		ID:   c.nextMsgID(),
		Type: WSTypePing,
	}
	return c.sendMessage(msg)
}

// nextMsgID generates a unique message ID
func (c *WSUserDataClient) nextMsgID() string {
	return fmt.Sprintf("%d", c.msgID.Add(1))
}

// handleMessage processes incoming WebSocket messages
func (c *WSUserDataClient) handleMessage(data []byte) {
	var msg WSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[KuCoin User WS] Failed to parse message: %v", err)
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
		log.Printf("[KuCoin User WS] Error message: %s", string(msg.Data))
		if c.handler != nil && c.handler.OnError != nil {
			c.handler.OnError(fmt.Errorf("WebSocket error: %s", string(msg.Data)))
		}
	default:
		log.Printf("[KuCoin User WS] Unknown message type: %s", msg.Type)
	}
}

func (c *WSUserDataClient) handleAckResponse(msg *WSMessage) {
	log.Printf("[KuCoin User WS] Subscription acknowledged: %s", msg.ID)
}

func (c *WSUserDataClient) handleDataMessage(msg *WSMessage) {
	if c.handler == nil {
		return
	}

	switch {
	case containsTopic(msg.Topic, WSTopicTradeOrders):
		c.handleOrderChange(msg.Data)
	case containsTopic(msg.Topic, WSTopicPosition):
		symbol := extractSymbolFromTopic(msg.Topic)
		c.handlePositionChange(symbol, msg.Data)
	case containsTopic(msg.Topic, WSTopicWallet):
		c.handleBalanceChange(msg.Data)
	default:
		log.Printf("[KuCoin User WS] Unhandled topic: %s", msg.Topic)
	}
}

func (c *WSUserDataClient) handleOrderChange(data json.RawMessage) {
	if c.handler.OnOrderChange == nil {
		return
	}

	var order WSOrderChange
	if err := json.Unmarshal(data, &order); err != nil {
		log.Printf("[KuCoin User WS] Failed to parse order change: %v", err)
		return
	}

	c.handler.OnOrderChange(&order)
}

func (c *WSUserDataClient) handlePositionChange(symbol string, data json.RawMessage) {
	if c.handler.OnPositionChange == nil {
		return
	}

	var position WSPositionChange
	if err := json.Unmarshal(data, &position); err != nil {
		log.Printf("[KuCoin User WS] Failed to parse position change: %v", err)
		return
	}

	c.handler.OnPositionChange(symbol, &position)
}

func (c *WSUserDataClient) handleBalanceChange(data json.RawMessage) {
	if c.handler.OnBalanceChange == nil {
		return
	}

	var balance WSBalanceChange
	if err := json.Unmarshal(data, &balance); err != nil {
		log.Printf("[KuCoin User WS] Failed to parse balance change: %v", err)
		return
	}

	c.handler.OnBalanceChange(&balance)
}

// handleReconnect attempts to reconnect after disconnection
func (c *WSUserDataClient) handleReconnect() {
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

		log.Printf("[KuCoin User WS] Reconnecting (attempt %d/%d)", retry+1, c.maxRetries)

		c.mu.Lock()
		err := c.connectInternal()
		c.mu.Unlock()

		if err == nil {
			// Resubscribe to previous topics
			c.resubscribe(subs)
			return
		}
		log.Printf("[KuCoin User WS] Reconnect failed: %v", err)
	}

	log.Printf("[KuCoin User WS] Max reconnect attempts reached")
	if c.handler != nil && c.handler.OnError != nil {
		c.handler.OnError(fmt.Errorf("max reconnect attempts reached"))
	}
}

// resubscribe restores subscriptions after reconnect
func (c *WSUserDataClient) resubscribe(subs map[string]bool) {
	for topic := range subs {
		if err := c.subscribe(topic, true); err != nil {
			log.Printf("[KuCoin User WS] Failed to resubscribe to %s: %v", topic, err)
		}
	}
}

// sendMessage sends a message to the WebSocket connection
func (c *WSUserDataClient) sendMessage(msg interface{}) error {
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

// subscribe sends a subscription request for private channel
func (c *WSUserDataClient) subscribe(topic string, privateChannel bool) error {
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
func (c *WSUserDataClient) unsubscribe(topic string, privateChannel bool) error {
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

// SubscribeTradeOrders subscribes to trade order updates
// This receives all order updates (open, match, filled, canceled)
func (c *WSUserDataClient) SubscribeTradeOrders() error {
	if err := c.Connect(); err != nil {
		return err
	}
	return c.subscribe(WSTopicTradeOrders, true)
}

// UnsubscribeTradeOrders unsubscribes from trade order updates
func (c *WSUserDataClient) UnsubscribeTradeOrders() error {
	return c.unsubscribe(WSTopicTradeOrders, true)
}

// SubscribePosition subscribes to position updates for a symbol
func (c *WSUserDataClient) SubscribePosition(symbol string) error {
	if err := c.Connect(); err != nil {
		return err
	}
	topic := fmt.Sprintf("%s:%s", WSTopicPosition, symbol)
	return c.subscribe(topic, true)
}

// UnsubscribePosition unsubscribes from position updates
func (c *WSUserDataClient) UnsubscribePosition(symbol string) error {
	topic := fmt.Sprintf("%s:%s", WSTopicPosition, symbol)
	return c.unsubscribe(topic, true)
}

// SubscribeAllPositions subscribes to all position updates
func (c *WSUserDataClient) SubscribeAllPositions() error {
	if err := c.Connect(); err != nil {
		return err
	}
	// Subscribe to all positions by using empty symbol
	return c.subscribe(WSTopicPosition, true)
}

// SubscribeWallet subscribes to wallet/balance updates
func (c *WSUserDataClient) SubscribeWallet() error {
	if err := c.Connect(); err != nil {
		return err
	}
	return c.subscribe(WSTopicWallet, true)
}

// UnsubscribeWallet unsubscribes from wallet updates
func (c *WSUserDataClient) UnsubscribeWallet() error {
	return c.unsubscribe(WSTopicWallet, true)
}

// SubscribeAll subscribes to all user data channels
func (c *WSUserDataClient) SubscribeAll(symbols ...string) error {
	if err := c.SubscribeTradeOrders(); err != nil {
		return fmt.Errorf("failed to subscribe to trade orders: %w", err)
	}

	if err := c.SubscribeWallet(); err != nil {
		return fmt.Errorf("failed to subscribe to wallet: %w", err)
	}

	// Subscribe to positions for specified symbols, or all if none specified
	if len(symbols) == 0 {
		if err := c.SubscribeAllPositions(); err != nil {
			return fmt.Errorf("failed to subscribe to all positions: %w", err)
		}
	} else {
		for _, symbol := range symbols {
			if err := c.SubscribePosition(symbol); err != nil {
				return fmt.Errorf("failed to subscribe to position %s: %w", symbol, err)
			}
		}
	}

	return nil
}

// Close closes the WebSocket connection
func (c *WSUserDataClient) Close() error {
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
func (c *WSUserDataClient) IsConnected() bool {
	return c.isConnected.Load()
}

// GetSubscriptions returns current subscriptions
func (c *WSUserDataClient) GetSubscriptions() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var subs []string
	for topic := range c.subscriptions {
		subs = append(subs, topic)
	}
	return subs
}
