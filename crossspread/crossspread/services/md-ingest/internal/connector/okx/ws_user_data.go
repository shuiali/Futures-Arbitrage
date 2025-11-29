// Package okx provides WebSocket client for private user data streams.
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
	"time"

	"github.com/gorilla/websocket"
)

// Private channel names
const (
	ChannelAccount            = "account"
	ChannelPositions          = "positions"
	ChannelBalanceAndPosition = "balance_and_position"
	ChannelOrders             = "orders"
	ChannelOrdersAlgo         = "orders-algo"
	ChannelLiquidationWarning = "liquidation-warning"
	ChannelAccountGreeks      = "account-greeks"
)

// UserDataHandler handles user data updates
type UserDataHandler interface {
	OnAccount(data *WSAccountData)
	OnPosition(data *WSPositionData)
	OnBalanceAndPosition(data *WSBalanceAndPositionData)
	OnOrder(data *WSOrderData)
	OnError(err error)
	OnConnected()
	OnDisconnected()
	OnAuthenticated()
}

// UserDataWSClient handles WebSocket connections for private user data
type UserDataWSClient struct {
	url        string
	apiKey     string
	secretKey  string
	passphrase string
	demoMode   bool

	conn    *websocket.Conn
	handler UserDataHandler

	subscriptions map[string]WSSubscribeArg
	subMu         sync.RWMutex

	writeMu sync.Mutex
	done    chan struct{}
	wg      sync.WaitGroup

	reconnect     bool
	reconnectWait time.Duration
	maxReconnect  int

	pingInterval  time.Duration
	authenticated bool
	authMu        sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

// UserDataWSConfig holds configuration for user data WebSocket client
type UserDataWSConfig struct {
	APIKey        string
	SecretKey     string
	Passphrase    string
	DemoMode      bool
	Handler       UserDataHandler
	PingInterval  time.Duration
	ReconnectWait time.Duration
	MaxReconnect  int
}

// NewUserDataWSClient creates a new user data WebSocket client
func NewUserDataWSClient(cfg UserDataWSConfig) *UserDataWSClient {
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

	return &UserDataWSClient{
		url:           url,
		apiKey:        cfg.APIKey,
		secretKey:     cfg.SecretKey,
		passphrase:    cfg.Passphrase,
		demoMode:      cfg.DemoMode,
		handler:       cfg.Handler,
		subscriptions: make(map[string]WSSubscribeArg),
		done:          make(chan struct{}),
		reconnect:     true,
		reconnectWait: cfg.ReconnectWait,
		maxReconnect:  cfg.MaxReconnect,
		pingInterval:  cfg.PingInterval,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Connect establishes WebSocket connection and authenticates
func (c *UserDataWSClient) Connect() error {
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

	// Start goroutines
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
func (c *UserDataWSClient) authenticate() error {
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

	return nil
}

// Close closes the WebSocket connection
func (c *UserDataWSClient) Close() error {
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
func (c *UserDataWSClient) IsAuthenticated() bool {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return c.authenticated
}

// readLoop reads messages from WebSocket
func (c *UserDataWSClient) readLoop() {
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
						c.handler.OnError(fmt.Errorf("WebSocket read error: %w", err))
					}
				}
				return
			}

			c.handleMessage(message)
		}
	}
}

// pingLoop sends periodic ping messages
func (c *UserDataWSClient) pingLoop() {
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
					c.handler.OnError(fmt.Errorf("ping error: %w", err))
				}
				return
			}
		}
	}
}

// handleDisconnect handles disconnection and reconnection
func (c *UserDataWSClient) handleDisconnect() {
	c.authMu.Lock()
	c.authenticated = false
	c.authMu.Unlock()

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
				c.handler.OnError(fmt.Errorf("reconnect attempt %d failed: %w", i+1, err))
			}
			continue
		}

		// Resubscribe to all channels
		c.resubscribe()
		return
	}

	if c.handler != nil {
		c.handler.OnError(fmt.Errorf("max reconnection attempts reached"))
	}
}

// resubscribe resubscribes to all channels after reconnection
func (c *UserDataWSClient) resubscribe() {
	c.subMu.RLock()
	defer c.subMu.RUnlock()

	for _, arg := range c.subscriptions {
		if err := c.sendSubscribe(arg); err != nil {
			if c.handler != nil {
				c.handler.OnError(fmt.Errorf("resubscribe error: %w", err))
			}
		}
	}
}

// handleMessage processes incoming WebSocket messages
func (c *UserDataWSClient) handleMessage(data []byte) {
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

	var resp WSResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		if c.handler != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal message: %w", err))
		}
		return
	}

	// Parse channel argument
	var arg WSChannelArg
	if err := json.Unmarshal(resp.Arg, &arg); err != nil {
		if c.handler != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal arg: %w", err))
		}
		return
	}

	// Handle data based on channel
	c.processChannelData(arg, resp.Data)
}

// handleEventResponse handles event-type responses
func (c *UserDataWSClient) handleEventResponse(event, code, msg string) {
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
				c.handler.OnError(fmt.Errorf("login failed: %s - %s", code, msg))
			}
		}
	case "subscribe":
		// Subscription confirmed
	case "unsubscribe":
		// Unsubscription confirmed
	case "error":
		if c.handler != nil {
			c.handler.OnError(fmt.Errorf("error: %s - %s", code, msg))
		}
	}
}

// processChannelData processes data based on channel type
func (c *UserDataWSClient) processChannelData(arg WSChannelArg, data json.RawMessage) {
	if c.handler == nil {
		return
	}

	switch arg.Channel {
	case ChannelAccount:
		var accounts []WSAccountData
		if err := json.Unmarshal(data, &accounts); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal account: %w", err))
			return
		}
		for _, a := range accounts {
			c.handler.OnAccount(&a)
		}

	case ChannelPositions:
		var positions []WSPositionData
		if err := json.Unmarshal(data, &positions); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal position: %w", err))
			return
		}
		for _, p := range positions {
			c.handler.OnPosition(&p)
		}

	case ChannelBalanceAndPosition:
		var updates []WSBalanceAndPositionData
		if err := json.Unmarshal(data, &updates); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal balance_and_position: %w", err))
			return
		}
		for _, u := range updates {
			c.handler.OnBalanceAndPosition(&u)
		}

	case ChannelOrders:
		var orders []WSOrderData
		if err := json.Unmarshal(data, &orders); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal order: %w", err))
			return
		}
		for _, o := range orders {
			c.handler.OnOrder(&o)
		}
	}
}

// sendSubscribe sends a subscription request
func (c *UserDataWSClient) sendSubscribe(arg WSSubscribeArg) error {
	if !c.IsAuthenticated() {
		return fmt.Errorf("not authenticated")
	}

	req := WSRequest{
		Op:   "subscribe",
		Args: []interface{}{arg},
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return c.conn.WriteJSON(req)
}

// sendUnsubscribe sends an unsubscription request
func (c *UserDataWSClient) sendUnsubscribe(arg WSSubscribeArg) error {
	req := WSRequest{
		Op:   "unsubscribe",
		Args: []interface{}{arg},
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return c.conn.WriteJSON(req)
}

// userDataSubscriptionKey generates a unique key for subscription
func userDataSubscriptionKey(channel, instType string) string {
	return channel + ":" + instType
}

// =============================================================================
// Subscription Methods
// =============================================================================

// SubscribeAccount subscribes to account updates
func (c *UserDataWSClient) SubscribeAccount() error {
	arg := WSSubscribeArg{
		Channel: ChannelAccount,
	}

	key := userDataSubscriptionKey(ChannelAccount, "")
	c.subMu.Lock()
	c.subscriptions[key] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeAccount unsubscribes from account updates
func (c *UserDataWSClient) UnsubscribeAccount() error {
	arg := WSSubscribeArg{
		Channel: ChannelAccount,
	}

	key := userDataSubscriptionKey(ChannelAccount, "")
	c.subMu.Lock()
	delete(c.subscriptions, key)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribePositions subscribes to position updates
// instType: MARGIN, SWAP, FUTURES, OPTION (leave empty for all)
// instFamily: optional filter for instrument family
// instID: optional filter for specific instrument
func (c *UserDataWSClient) SubscribePositions(instType, instFamily, instID string) error {
	arg := WSSubscribeArg{
		Channel:    ChannelPositions,
		InstType:   instType,
		InstFamily: instFamily,
		InstID:     instID,
	}

	key := userDataSubscriptionKey(ChannelPositions, instType)
	c.subMu.Lock()
	c.subscriptions[key] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribePositions unsubscribes from position updates
func (c *UserDataWSClient) UnsubscribePositions(instType, instFamily, instID string) error {
	arg := WSSubscribeArg{
		Channel:    ChannelPositions,
		InstType:   instType,
		InstFamily: instFamily,
		InstID:     instID,
	}

	key := userDataSubscriptionKey(ChannelPositions, instType)
	c.subMu.Lock()
	delete(c.subscriptions, key)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribeBalanceAndPosition subscribes to combined balance and position updates
func (c *UserDataWSClient) SubscribeBalanceAndPosition() error {
	arg := WSSubscribeArg{
		Channel: ChannelBalanceAndPosition,
	}

	key := userDataSubscriptionKey(ChannelBalanceAndPosition, "")
	c.subMu.Lock()
	c.subscriptions[key] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeBalanceAndPosition unsubscribes from balance and position updates
func (c *UserDataWSClient) UnsubscribeBalanceAndPosition() error {
	arg := WSSubscribeArg{
		Channel: ChannelBalanceAndPosition,
	}

	key := userDataSubscriptionKey(ChannelBalanceAndPosition, "")
	c.subMu.Lock()
	delete(c.subscriptions, key)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribeOrders subscribes to order updates
// instType: SPOT, MARGIN, SWAP, FUTURES, OPTION, ANY
// instFamily: optional filter for instrument family
// instID: optional filter for specific instrument
func (c *UserDataWSClient) SubscribeOrders(instType, instFamily, instID string) error {
	arg := WSSubscribeArg{
		Channel:    ChannelOrders,
		InstType:   instType,
		InstFamily: instFamily,
		InstID:     instID,
	}

	key := userDataSubscriptionKey(ChannelOrders, instType)
	c.subMu.Lock()
	c.subscriptions[key] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeOrders unsubscribes from order updates
func (c *UserDataWSClient) UnsubscribeOrders(instType, instFamily, instID string) error {
	arg := WSSubscribeArg{
		Channel:    ChannelOrders,
		InstType:   instType,
		InstFamily: instFamily,
		InstID:     instID,
	}

	key := userDataSubscriptionKey(ChannelOrders, instType)
	c.subMu.Lock()
	delete(c.subscriptions, key)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// =============================================================================
// Convenience Methods
// =============================================================================

// SubscribeAllSwapOrders subscribes to all swap (perpetual) order updates
func (c *UserDataWSClient) SubscribeAllSwapOrders() error {
	return c.SubscribeOrders(InstTypeSwap, "", "")
}

// SubscribeAllSwapPositions subscribes to all swap (perpetual) position updates
func (c *UserDataWSClient) SubscribeAllSwapPositions() error {
	return c.SubscribePositions(InstTypeSwap, "", "")
}

// SubscribeAllFuturesOrders subscribes to all futures order updates
func (c *UserDataWSClient) SubscribeAllFuturesOrders() error {
	return c.SubscribeOrders(InstTypeFutures, "", "")
}

// SubscribeAllFuturesPositions subscribes to all futures position updates
func (c *UserDataWSClient) SubscribeAllFuturesPositions() error {
	return c.SubscribePositions(InstTypeFutures, "", "")
}

// SubscribeAll subscribes to all relevant channels for arbitrage trading
func (c *UserDataWSClient) SubscribeAll() error {
	// Subscribe to account changes
	if err := c.SubscribeAccount(); err != nil {
		return fmt.Errorf("failed to subscribe account: %w", err)
	}

	// Subscribe to combined balance and position updates
	if err := c.SubscribeBalanceAndPosition(); err != nil {
		return fmt.Errorf("failed to subscribe balance_and_position: %w", err)
	}

	// Subscribe to all order types
	if err := c.SubscribeOrders("ANY", "", ""); err != nil {
		return fmt.Errorf("failed to subscribe orders: %w", err)
	}

	return nil
}

// =============================================================================
// Position Tracker
// =============================================================================

// PositionTracker tracks positions received via WebSocket
type PositionTracker struct {
	positions map[string]*WSPositionData // instID + posSide -> position
	mu        sync.RWMutex
}

// NewPositionTracker creates a new position tracker
func NewPositionTracker() *PositionTracker {
	return &PositionTracker{
		positions: make(map[string]*WSPositionData),
	}
}

// positionKey generates a unique key for a position
func positionKey(instID, posSide string) string {
	return instID + ":" + posSide
}

// Update updates a position from WebSocket data
func (t *PositionTracker) Update(data *WSPositionData) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := positionKey(data.InstID, data.PosSide)

	// Remove position if size is 0
	if data.Pos == "0" {
		delete(t.positions, key)
		return
	}

	t.positions[key] = data
}

// Get returns a position by instrument ID and side
func (t *PositionTracker) Get(instID, posSide string) *WSPositionData {
	t.mu.RLock()
	defer t.mu.RUnlock()

	key := positionKey(instID, posSide)
	return t.positions[key]
}

// GetByInstrument returns all positions for an instrument
func (t *PositionTracker) GetByInstrument(instID string) []*WSPositionData {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []*WSPositionData
	for key, pos := range t.positions {
		if len(key) > len(instID) && key[:len(instID)] == instID {
			result = append(result, pos)
		}
	}
	return result
}

// GetAll returns all tracked positions
func (t *PositionTracker) GetAll() []*WSPositionData {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*WSPositionData, 0, len(t.positions))
	for _, pos := range t.positions {
		result = append(result, pos)
	}
	return result
}

// Clear removes all tracked positions
func (t *PositionTracker) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.positions = make(map[string]*WSPositionData)
}

// =============================================================================
// Order Tracker
// =============================================================================

// OrderTracker tracks orders received via WebSocket
type OrderTracker struct {
	orders map[string]*WSOrderData // orderId -> order
	mu     sync.RWMutex
}

// NewOrderTracker creates a new order tracker
func NewOrderTracker() *OrderTracker {
	return &OrderTracker{
		orders: make(map[string]*WSOrderData),
	}
}

// Update updates an order from WebSocket data
func (t *OrderTracker) Update(data *WSOrderData) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Remove completed orders
	if data.State == OrderStateFilled || data.State == OrderStateCanceled || data.State == OrderStateMMPCanceled {
		delete(t.orders, data.OrderID)
		return
	}

	t.orders[data.OrderID] = data
}

// Get returns an order by order ID
func (t *OrderTracker) Get(orderID string) *WSOrderData {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.orders[orderID]
}

// GetByInstrument returns all orders for an instrument
func (t *OrderTracker) GetByInstrument(instID string) []*WSOrderData {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []*WSOrderData
	for _, order := range t.orders {
		if order.InstID == instID {
			result = append(result, order)
		}
	}
	return result
}

// GetPending returns all pending orders
func (t *OrderTracker) GetPending() []*WSOrderData {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []*WSOrderData
	for _, order := range t.orders {
		if order.State == OrderStateLive || order.State == OrderStatePartiallyFilled {
			result = append(result, order)
		}
	}
	return result
}

// GetAll returns all tracked orders
func (t *OrderTracker) GetAll() []*WSOrderData {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*WSOrderData, 0, len(t.orders))
	for _, order := range t.orders {
		result = append(result, order)
	}
	return result
}

// Clear removes all tracked orders
func (t *OrderTracker) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.orders = make(map[string]*WSOrderData)
}
