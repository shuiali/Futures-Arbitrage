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
	"time"

	"github.com/gorilla/websocket"
)

// WSUserDataHandler handles user data callbacks
type WSUserDataHandler struct {
	OnOrder           func(settle string, order *WSOrderData)
	OnPosition        func(settle string, position *WSPositionData)
	OnBalance         func(settle string, balance *WSBalanceData)
	OnUserTrade       func(settle string, trade *WSUserTradeData)
	OnLiquidation     func(settle string, data json.RawMessage)
	OnAutoOrder       func(settle string, data json.RawMessage)
	OnReduceRiskLimit func(settle string, data json.RawMessage)
	OnError           func(err error)
	OnConnect         func(settle string)
	OnDisconnect      func(settle string, err error)
	OnLogin           func(settle string, success bool, err error)
}

// WSUserDataClient handles WebSocket user data streams
type WSUserDataClient struct {
	baseURL        string
	apiKey         string
	apiSecret      string
	handler        *WSUserDataHandler
	connections    map[string]*wsUserConnection // settle -> connection
	subscriptions  map[string]map[string]bool   // settle -> channel -> subscribed
	mu             sync.RWMutex
	reconnectDelay time.Duration
	maxRetries     int
	ctx            context.Context
	cancel         context.CancelFunc
}

// wsUserConnection represents a user data WebSocket connection
type wsUserConnection struct {
	conn        *websocket.Conn
	settle      string
	mu          sync.Mutex
	writeMu     sync.Mutex
	isConnected bool
	isLoggedIn  bool
	stopPing    chan struct{}
}

// NewWSUserDataClient creates a new WebSocket user data client
func NewWSUserDataClient(baseURL, apiKey, apiSecret string, handler *WSUserDataHandler) *WSUserDataClient {
	if baseURL == "" {
		baseURL = "wss://fx-ws.gateio.ws/v4/ws"
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &WSUserDataClient{
		baseURL:        baseURL,
		apiKey:         apiKey,
		apiSecret:      apiSecret,
		handler:        handler,
		connections:    make(map[string]*wsUserConnection),
		subscriptions:  make(map[string]map[string]bool),
		reconnectDelay: 5 * time.Second,
		maxRetries:     10,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Connect establishes WebSocket connection for user data
func (c *WSUserDataClient) Connect(settle string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if conn, exists := c.connections[settle]; exists && conn.isConnected {
		return nil
	}

	return c.connectInternal(settle)
}

func (c *WSUserDataClient) connectInternal(settle string) error {
	url := fmt.Sprintf("%s/%s", c.baseURL, settle)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(c.ctx, url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Gate.io user WS (%s): %w", settle, err)
	}

	wsConn := &wsUserConnection{
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

	log.Printf("[Gate.io User WS] Connected to %s", settle)

	// Authenticate
	if c.apiKey != "" && c.apiSecret != "" {
		if err := c.login(settle); err != nil {
			log.Printf("[Gate.io User WS] Login failed for %s: %v", settle, err)
			return err
		}
	}

	return nil
}

// login authenticates the WebSocket connection
func (c *WSUserDataClient) login(settle string) error {
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

	// Wait for login response
	time.Sleep(500 * time.Millisecond)

	wsConn.mu.Lock()
	wsConn.isLoggedIn = true
	wsConn.mu.Unlock()

	log.Printf("[Gate.io User WS] Logged in to %s", settle)

	if c.handler != nil && c.handler.OnLogin != nil {
		c.handler.OnLogin(settle, true, nil)
	}

	return nil
}

// sign generates HMAC-SHA512 signature
func (c *WSUserDataClient) sign(payload string) string {
	h := hmac.New(sha512.New, []byte(c.apiSecret))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

// readLoop reads messages from WebSocket
func (c *WSUserDataClient) readLoop(wsConn *wsUserConnection) {
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
				log.Printf("[Gate.io User WS] Read error (%s): %v", wsConn.settle, err)
			}
			c.handleReconnect(wsConn.settle)
			return
		}

		c.handleMessage(wsConn.settle, message)
	}
}

// pingLoop sends periodic pings
func (c *WSUserDataClient) pingLoop(wsConn *wsUserConnection) {
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
				log.Printf("[Gate.io User WS] Ping error (%s): %v", wsConn.settle, err)
				return
			}
		}
	}
}

// handleMessage processes incoming messages
func (c *WSUserDataClient) handleMessage(settle string, data []byte) {
	var msg WSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[Gate.io User WS] Failed to parse message: %v", err)
		return
	}

	switch msg.Event {
	case "subscribe":
		c.handleSubscribeResponse(settle, &msg)
	case "unsubscribe":
		c.handleUnsubscribeResponse(settle, &msg)
	case "update":
		c.handleUpdateMessage(settle, &msg)
	default:
		// May be login response
		if msg.Channel == "futures.login" {
			c.handleLoginResponse(settle, &msg)
		}
	}
}

func (c *WSUserDataClient) handleLoginResponse(settle string, msg *WSMessage) {
	c.mu.RLock()
	wsConn, ok := c.connections[settle]
	c.mu.RUnlock()

	if !ok {
		return
	}

	if msg.Error != nil {
		log.Printf("[Gate.io User WS] Login failed: %s", msg.Error.Message)
		if c.handler != nil && c.handler.OnLogin != nil {
			c.handler.OnLogin(settle, false, fmt.Errorf(msg.Error.Message))
		}
		return
	}

	wsConn.mu.Lock()
	wsConn.isLoggedIn = true
	wsConn.mu.Unlock()

	log.Printf("[Gate.io User WS] Login successful for %s", settle)
}

func (c *WSUserDataClient) handleSubscribeResponse(settle string, msg *WSMessage) {
	if msg.Error != nil {
		log.Printf("[Gate.io User WS] Subscribe error (%s): code=%d, msg=%s",
			settle, msg.Error.Code, msg.Error.Message)
		if c.handler != nil && c.handler.OnError != nil {
			c.handler.OnError(fmt.Errorf("subscribe error: %s", msg.Error.Message))
		}
		return
	}
	log.Printf("[Gate.io User WS] Subscribed to %s on %s", msg.Channel, settle)
}

func (c *WSUserDataClient) handleUnsubscribeResponse(settle string, msg *WSMessage) {
	log.Printf("[Gate.io User WS] Unsubscribed from %s on %s", msg.Channel, settle)
}

func (c *WSUserDataClient) handleUpdateMessage(settle string, msg *WSMessage) {
	if c.handler == nil {
		return
	}

	switch msg.Channel {
	case "futures.orders":
		c.handleOrderUpdate(settle, msg.Result)
	case "futures.positions":
		c.handlePositionUpdate(settle, msg.Result)
	case "futures.balances":
		c.handleBalanceUpdate(settle, msg.Result)
	case "futures.usertrades":
		c.handleUserTradeUpdate(settle, msg.Result)
	case "futures.liquidates":
		c.handleLiquidationUpdate(settle, msg.Result)
	case "futures.auto_orders":
		c.handleAutoOrderUpdate(settle, msg.Result)
	case "futures.reduce_risk_limits":
		c.handleReduceRiskLimitUpdate(settle, msg.Result)
	default:
		log.Printf("[Gate.io User WS] Unhandled channel: %s", msg.Channel)
	}
}

func (c *WSUserDataClient) handleOrderUpdate(settle string, data json.RawMessage) {
	if c.handler.OnOrder == nil {
		return
	}

	var orders []WSOrderData
	if err := json.Unmarshal(data, &orders); err != nil {
		log.Printf("[Gate.io User WS] Failed to parse order: %v", err)
		return
	}

	for _, order := range orders {
		c.handler.OnOrder(settle, &order)
	}
}

func (c *WSUserDataClient) handlePositionUpdate(settle string, data json.RawMessage) {
	if c.handler.OnPosition == nil {
		return
	}

	var positions []WSPositionData
	if err := json.Unmarshal(data, &positions); err != nil {
		log.Printf("[Gate.io User WS] Failed to parse position: %v", err)
		return
	}

	for _, pos := range positions {
		c.handler.OnPosition(settle, &pos)
	}
}

func (c *WSUserDataClient) handleBalanceUpdate(settle string, data json.RawMessage) {
	if c.handler.OnBalance == nil {
		return
	}

	var balances []WSBalanceData
	if err := json.Unmarshal(data, &balances); err != nil {
		log.Printf("[Gate.io User WS] Failed to parse balance: %v", err)
		return
	}

	for _, bal := range balances {
		c.handler.OnBalance(settle, &bal)
	}
}

func (c *WSUserDataClient) handleUserTradeUpdate(settle string, data json.RawMessage) {
	if c.handler.OnUserTrade == nil {
		return
	}

	var trades []WSUserTradeData
	if err := json.Unmarshal(data, &trades); err != nil {
		log.Printf("[Gate.io User WS] Failed to parse user trade: %v", err)
		return
	}

	for _, trade := range trades {
		c.handler.OnUserTrade(settle, &trade)
	}
}

func (c *WSUserDataClient) handleLiquidationUpdate(settle string, data json.RawMessage) {
	if c.handler.OnLiquidation == nil {
		return
	}
	c.handler.OnLiquidation(settle, data)
}

func (c *WSUserDataClient) handleAutoOrderUpdate(settle string, data json.RawMessage) {
	if c.handler.OnAutoOrder == nil {
		return
	}
	c.handler.OnAutoOrder(settle, data)
}

func (c *WSUserDataClient) handleReduceRiskLimitUpdate(settle string, data json.RawMessage) {
	if c.handler.OnReduceRiskLimit == nil {
		return
	}
	c.handler.OnReduceRiskLimit(settle, data)
}

// handleReconnect handles reconnection
func (c *WSUserDataClient) handleReconnect(settle string) {
	c.mu.Lock()

	// Get subscriptions to restore
	subs := make(map[string]bool)
	if existing, ok := c.subscriptions[settle]; ok {
		for k, v := range existing {
			subs[k] = v
		}
	}

	delete(c.connections, settle)
	c.mu.Unlock()

	for retry := 0; retry < c.maxRetries; retry++ {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(c.reconnectDelay):
		}

		log.Printf("[Gate.io User WS] Reconnecting to %s (attempt %d/%d)", settle, retry+1, c.maxRetries)

		c.mu.Lock()
		err := c.connectInternal(settle)
		c.mu.Unlock()

		if err == nil {
			// Resubscribe to channels
			c.resubscribe(settle, subs)
			return
		}
		log.Printf("[Gate.io User WS] Reconnect failed: %v", err)
	}

	log.Printf("[Gate.io User WS] Max reconnect attempts reached for %s", settle)
	if c.handler != nil && c.handler.OnError != nil {
		c.handler.OnError(fmt.Errorf("max reconnect attempts reached for %s", settle))
	}
}

// resubscribe restores subscriptions
func (c *WSUserDataClient) resubscribe(settle string, subs map[string]bool) {
	for channel := range subs {
		switch channel {
		case "futures.orders":
			c.SubscribeOrders(settle, nil)
		case "futures.positions":
			c.SubscribePositions(settle, nil)
		case "futures.balances":
			c.SubscribeBalances(settle)
		case "futures.usertrades":
			c.SubscribeUserTrades(settle, nil)
		}
	}
}

// sendMessage sends a WebSocket message
func (c *WSUserDataClient) sendMessage(settle string, msg interface{}) error {
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

// subscribeAuth sends an authenticated subscription
func (c *WSUserDataClient) subscribeAuth(settle string, channel string, payload []string) error {
	c.mu.RLock()
	wsConn, ok := c.connections[settle]
	c.mu.RUnlock()

	if !ok || !wsConn.isConnected {
		return fmt.Errorf("not connected to %s", settle)
	}

	if !wsConn.isLoggedIn {
		return fmt.Errorf("not logged in to %s", settle)
	}

	timestamp := time.Now().Unix()

	// Sign the subscription
	signPayload := fmt.Sprintf("channel=%s&event=%s&time=%d", channel, "subscribe", timestamp)
	signature := c.sign(signPayload)

	msg := map[string]interface{}{
		"time":    timestamp,
		"channel": channel,
		"event":   "subscribe",
		"payload": payload,
		"auth": map[string]string{
			"method": "api_key",
			"KEY":    c.apiKey,
			"SIGN":   signature,
		},
	}

	if err := c.sendMessage(settle, msg); err != nil {
		return err
	}

	// Track subscription
	c.mu.Lock()
	if c.subscriptions[settle] == nil {
		c.subscriptions[settle] = make(map[string]bool)
	}
	c.subscriptions[settle][channel] = true
	c.mu.Unlock()

	return nil
}

// unsubscribeAuth sends an authenticated unsubscription
func (c *WSUserDataClient) unsubscribeAuth(settle string, channel string, payload []string) error {
	c.mu.RLock()
	wsConn, ok := c.connections[settle]
	c.mu.RUnlock()

	if !ok || !wsConn.isConnected {
		return fmt.Errorf("not connected to %s", settle)
	}

	timestamp := time.Now().Unix()
	signPayload := fmt.Sprintf("channel=%s&event=%s&time=%d", channel, "unsubscribe", timestamp)
	signature := c.sign(signPayload)

	msg := map[string]interface{}{
		"time":    timestamp,
		"channel": channel,
		"event":   "unsubscribe",
		"payload": payload,
		"auth": map[string]string{
			"method": "api_key",
			"KEY":    c.apiKey,
			"SIGN":   signature,
		},
	}

	if err := c.sendMessage(settle, msg); err != nil {
		return err
	}

	// Remove from subscriptions
	c.mu.Lock()
	delete(c.subscriptions[settle], channel)
	c.mu.Unlock()

	return nil
}

// SubscribeOrders subscribes to order updates
// contracts: specific contracts or nil for all
func (c *WSUserDataClient) SubscribeOrders(settle string, contracts []string) error {
	if err := c.Connect(settle); err != nil {
		return err
	}

	// User ID is extracted from API key by the server
	payload := []string{"!all"}
	if len(contracts) > 0 {
		payload = contracts
	}

	return c.subscribeAuth(settle, "futures.orders", payload)
}

// UnsubscribeOrders unsubscribes from order updates
func (c *WSUserDataClient) UnsubscribeOrders(settle string, contracts []string) error {
	payload := []string{"!all"}
	if len(contracts) > 0 {
		payload = contracts
	}
	return c.unsubscribeAuth(settle, "futures.orders", payload)
}

// SubscribePositions subscribes to position updates
func (c *WSUserDataClient) SubscribePositions(settle string, contracts []string) error {
	if err := c.Connect(settle); err != nil {
		return err
	}

	payload := []string{"!all"}
	if len(contracts) > 0 {
		payload = contracts
	}

	return c.subscribeAuth(settle, "futures.positions", payload)
}

// UnsubscribePositions unsubscribes from position updates
func (c *WSUserDataClient) UnsubscribePositions(settle string, contracts []string) error {
	payload := []string{"!all"}
	if len(contracts) > 0 {
		payload = contracts
	}
	return c.unsubscribeAuth(settle, "futures.positions", payload)
}

// SubscribeBalances subscribes to balance updates
func (c *WSUserDataClient) SubscribeBalances(settle string) error {
	if err := c.Connect(settle); err != nil {
		return err
	}
	return c.subscribeAuth(settle, "futures.balances", nil)
}

// UnsubscribeBalances unsubscribes from balance updates
func (c *WSUserDataClient) UnsubscribeBalances(settle string) error {
	return c.unsubscribeAuth(settle, "futures.balances", nil)
}

// SubscribeUserTrades subscribes to user trade updates
func (c *WSUserDataClient) SubscribeUserTrades(settle string, contracts []string) error {
	if err := c.Connect(settle); err != nil {
		return err
	}

	payload := []string{"!all"}
	if len(contracts) > 0 {
		payload = contracts
	}

	return c.subscribeAuth(settle, "futures.usertrades", payload)
}

// UnsubscribeUserTrades unsubscribes from user trade updates
func (c *WSUserDataClient) UnsubscribeUserTrades(settle string, contracts []string) error {
	payload := []string{"!all"}
	if len(contracts) > 0 {
		payload = contracts
	}
	return c.unsubscribeAuth(settle, "futures.usertrades", payload)
}

// SubscribeLiquidations subscribes to liquidation notifications
func (c *WSUserDataClient) SubscribeLiquidations(settle string) error {
	if err := c.Connect(settle); err != nil {
		return err
	}
	return c.subscribeAuth(settle, "futures.liquidates", nil)
}

// UnsubscribeLiquidations unsubscribes from liquidation notifications
func (c *WSUserDataClient) UnsubscribeLiquidations(settle string) error {
	return c.unsubscribeAuth(settle, "futures.liquidates", nil)
}

// SubscribeAutoOrders subscribes to auto-order updates (TP/SL, etc.)
func (c *WSUserDataClient) SubscribeAutoOrders(settle string) error {
	if err := c.Connect(settle); err != nil {
		return err
	}
	return c.subscribeAuth(settle, "futures.auto_orders", nil)
}

// UnsubscribeAutoOrders unsubscribes from auto-order updates
func (c *WSUserDataClient) UnsubscribeAutoOrders(settle string) error {
	return c.unsubscribeAuth(settle, "futures.auto_orders", nil)
}

// SubscribeAll subscribes to all user data channels
func (c *WSUserDataClient) SubscribeAll(settle string) error {
	if err := c.SubscribeOrders(settle, nil); err != nil {
		return err
	}
	if err := c.SubscribePositions(settle, nil); err != nil {
		return err
	}
	if err := c.SubscribeBalances(settle); err != nil {
		return err
	}
	if err := c.SubscribeUserTrades(settle, nil); err != nil {
		return err
	}
	return nil
}

// UnsubscribeAll unsubscribes from all user data channels
func (c *WSUserDataClient) UnsubscribeAll(settle string) error {
	c.UnsubscribeOrders(settle, nil)
	c.UnsubscribePositions(settle, nil)
	c.UnsubscribeBalances(settle)
	c.UnsubscribeUserTrades(settle, nil)
	return nil
}

// IsConnected checks if connected
func (c *WSUserDataClient) IsConnected(settle string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if wsConn, ok := c.connections[settle]; ok {
		return wsConn.isConnected
	}
	return false
}

// IsLoggedIn checks if logged in
func (c *WSUserDataClient) IsLoggedIn(settle string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if wsConn, ok := c.connections[settle]; ok {
		return wsConn.isLoggedIn
	}
	return false
}

// Close closes all connections
func (c *WSUserDataClient) Close() error {
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

// GetSubscriptions returns current subscriptions
func (c *WSUserDataClient) GetSubscriptions(settle string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var subs []string
	if settleSubs, ok := c.subscriptions[settle]; ok {
		for channel := range settleSubs {
			subs = append(subs, channel)
		}
	}
	return subs
}
