// Package bitget provides WebSocket user data client for Bitget exchange.
// Supports private streams for account, positions, orders, and fills.
package bitget

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"context"

	"github.com/gorilla/websocket"
)

// UserDataHandler handles user data updates
type UserDataHandler interface {
	OnAccount(data *WSAccountData)
	OnEquity(data *WSEquityData)
	OnPosition(data *WSPositionData)
	OnOrder(data *WSOrderData)
	OnFill(data *WSFillData)
	OnError(err error)
	OnConnected()
	OnDisconnected()
}

// UserDataWSClient handles WebSocket connections for user data
type UserDataWSClient struct {
	url        string
	conn       *websocket.Conn
	handler    UserDataHandler
	instType   string
	apiKey     string
	secretKey  string
	passphrase string

	subscriptions map[string]WSSubscribeArg // channel -> arg
	subMu         sync.RWMutex

	writeMu sync.Mutex
	done    chan struct{}
	wg      sync.WaitGroup

	reconnect     bool
	reconnectWait time.Duration
	maxReconnect  int

	pingInterval time.Duration
	pongWait     time.Duration

	ctx    context.Context
	cancel context.CancelFunc

	isAuthenticated bool
	authMu          sync.Mutex
}

// UserDataWSConfig holds configuration for user data WebSocket client
type UserDataWSConfig struct {
	InstType      string // Required: USDT-FUTURES, USDC-FUTURES, COIN-FUTURES
	APIKey        string
	SecretKey     string
	Passphrase    string
	Handler       UserDataHandler
	PingInterval  time.Duration
	PongWait      time.Duration
	ReconnectWait time.Duration
	MaxReconnect  int
}

// NewUserDataWSClient creates a new user data WebSocket client
func NewUserDataWSClient(cfg UserDataWSConfig) *UserDataWSClient {
	if cfg.PingInterval == 0 {
		cfg.PingInterval = 25 * time.Second
	}
	if cfg.PongWait == 0 {
		cfg.PongWait = 30 * time.Second
	}
	if cfg.ReconnectWait == 0 {
		cfg.ReconnectWait = 5 * time.Second
	}
	if cfg.MaxReconnect == 0 {
		cfg.MaxReconnect = 3
	}
	if cfg.InstType == "" {
		cfg.InstType = ProductTypeUSDTFutures
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &UserDataWSClient{
		url:           WSPrivateURL,
		handler:       cfg.Handler,
		instType:      cfg.InstType,
		apiKey:        cfg.APIKey,
		secretKey:     cfg.SecretKey,
		passphrase:    cfg.Passphrase,
		subscriptions: make(map[string]WSSubscribeArg),
		done:          make(chan struct{}),
		reconnect:     true,
		reconnectWait: cfg.ReconnectWait,
		maxReconnect:  cfg.MaxReconnect,
		pingInterval:  cfg.PingInterval,
		pongWait:      cfg.PongWait,
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
	c.isAuthenticated = false

	// Start goroutines
	c.wg.Add(2)
	go c.readLoop()
	go c.pingLoop()

	// Authenticate
	if err := c.authenticate(); err != nil {
		c.Close()
		return fmt.Errorf("authentication failed: %w", err)
	}

	if c.handler != nil {
		c.handler.OnConnected()
	}

	return nil
}

// authenticate sends login request
func (c *UserDataWSClient) authenticate() error {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	sign := c.sign(timestamp)

	req := WSLoginRequest{
		Op: WSOpLogin,
		Args: []WSLoginArg{{
			APIKey:     c.apiKey,
			Passphrase: c.passphrase,
			Timestamp:  timestamp,
			Sign:       sign,
		}},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return err
	}

	// Wait for authentication response
	time.Sleep(500 * time.Millisecond)

	c.authMu.Lock()
	c.isAuthenticated = true
	c.authMu.Unlock()

	return nil
}

// sign generates HMAC-SHA256 signature for WebSocket authentication
func (c *UserDataWSClient) sign(timestamp string) string {
	message := timestamp + "GET" + "/user/verify"
	h := hmac.New(sha256.New, []byte(c.secretKey))
	h.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
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
	c.isAuthenticated = false
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

	args := make([]WSSubscribeArg, 0, len(c.subscriptions))
	for _, arg := range c.subscriptions {
		args = append(args, arg)
	}

	if len(args) > 0 {
		if err := c.sendSubscribeMultiple(args); err != nil {
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

	var resp WSResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		if c.handler != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal message: %w", err))
		}
		return
	}

	// Handle event responses (subscribe/unsubscribe/login confirmations)
	if resp.Event != "" {
		if resp.Event == "error" && c.handler != nil {
			c.handler.OnError(fmt.Errorf("WebSocket error %s: %s", resp.Code, resp.Msg))
		}
		return
	}

	// Skip if no data
	if len(resp.Data) == 0 {
		return
	}

	// Parse channel argument
	var arg WSSubscribeArg
	if err := json.Unmarshal(resp.Arg, &arg); err != nil {
		if c.handler != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal arg: %w", err))
		}
		return
	}

	// Handle data based on channel
	c.processChannelData(arg, resp.Data)
}

// processChannelData processes data based on channel type
func (c *UserDataWSClient) processChannelData(arg WSSubscribeArg, data json.RawMessage) {
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
		for i := range accounts {
			c.handler.OnAccount(&accounts[i])
		}

	case ChannelEquity:
		var equities []WSEquityData
		if err := json.Unmarshal(data, &equities); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal equity: %w", err))
			return
		}
		for i := range equities {
			c.handler.OnEquity(&equities[i])
		}

	case ChannelPositions:
		var positions []WSPositionData
		if err := json.Unmarshal(data, &positions); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal position: %w", err))
			return
		}
		for i := range positions {
			c.handler.OnPosition(&positions[i])
		}

	case ChannelOrders:
		var orders []WSOrderData
		if err := json.Unmarshal(data, &orders); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal order: %w", err))
			return
		}
		for i := range orders {
			c.handler.OnOrder(&orders[i])
		}

	case ChannelFill:
		var fills []WSFillData
		if err := json.Unmarshal(data, &fills); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal fill: %w", err))
			return
		}
		for i := range fills {
			c.handler.OnFill(&fills[i])
		}
	}
}

// sendSubscribe sends a subscription request
func (c *UserDataWSClient) sendSubscribe(arg WSSubscribeArg) error {
	return c.sendSubscribeMultiple([]WSSubscribeArg{arg})
}

// sendSubscribeMultiple sends a subscription request for multiple channels
func (c *UserDataWSClient) sendSubscribeMultiple(args []WSSubscribeArg) error {
	req := WSSubscribeRequest{
		Op:   WSOpSubscribe,
		Args: args,
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return c.conn.WriteJSON(req)
}

// sendUnsubscribe sends an unsubscription request
func (c *UserDataWSClient) sendUnsubscribe(arg WSSubscribeArg) error {
	req := WSSubscribeRequest{
		Op:   WSOpUnsubscribe,
		Args: []WSSubscribeArg{arg},
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return c.conn.WriteJSON(req)
}

// =============================================================================
// Subscription Methods
// =============================================================================

// SubscribeAccount subscribes to account balance updates
func (c *UserDataWSClient) SubscribeAccount(coin string) error {
	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  ChannelAccount,
		Coin:     coin,
	}

	c.subMu.Lock()
	c.subscriptions[ChannelAccount+":"+coin] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeAccount unsubscribes from account balance updates
func (c *UserDataWSClient) UnsubscribeAccount(coin string) error {
	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  ChannelAccount,
		Coin:     coin,
	}

	c.subMu.Lock()
	delete(c.subscriptions, ChannelAccount+":"+coin)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribeEquity subscribes to equity updates
func (c *UserDataWSClient) SubscribeEquity() error {
	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  ChannelEquity,
	}

	c.subMu.Lock()
	c.subscriptions[ChannelEquity] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeEquity unsubscribes from equity updates
func (c *UserDataWSClient) UnsubscribeEquity() error {
	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  ChannelEquity,
	}

	c.subMu.Lock()
	delete(c.subscriptions, ChannelEquity)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribePositions subscribes to position updates
func (c *UserDataWSClient) SubscribePositions() error {
	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  ChannelPositions,
	}

	c.subMu.Lock()
	c.subscriptions[ChannelPositions] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribePositions unsubscribes from position updates
func (c *UserDataWSClient) UnsubscribePositions() error {
	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  ChannelPositions,
	}

	c.subMu.Lock()
	delete(c.subscriptions, ChannelPositions)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribeOrders subscribes to order updates
func (c *UserDataWSClient) SubscribeOrders(instID string) error {
	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  ChannelOrders,
		InstID:   instID,
	}

	c.subMu.Lock()
	c.subscriptions[ChannelOrders+":"+instID] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeOrders unsubscribes from order updates
func (c *UserDataWSClient) UnsubscribeOrders(instID string) error {
	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  ChannelOrders,
		InstID:   instID,
	}

	c.subMu.Lock()
	delete(c.subscriptions, ChannelOrders+":"+instID)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribeFills subscribes to fill/trade updates
func (c *UserDataWSClient) SubscribeFills() error {
	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  ChannelFill,
	}

	c.subMu.Lock()
	c.subscriptions[ChannelFill] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeFills unsubscribes from fill/trade updates
func (c *UserDataWSClient) UnsubscribeFills() error {
	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  ChannelFill,
	}

	c.subMu.Lock()
	delete(c.subscriptions, ChannelFill)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribeAll subscribes to all user data channels
func (c *UserDataWSClient) SubscribeAll(coin string) error {
	args := []WSSubscribeArg{
		{InstType: c.instType, Channel: ChannelAccount, Coin: coin},
		{InstType: c.instType, Channel: ChannelEquity},
		{InstType: c.instType, Channel: ChannelPositions},
		{InstType: c.instType, Channel: ChannelOrders},
		{InstType: c.instType, Channel: ChannelFill},
	}

	c.subMu.Lock()
	for _, arg := range args {
		key := arg.Channel
		if arg.Coin != "" {
			key += ":" + arg.Coin
		}
		if arg.InstID != "" {
			key += ":" + arg.InstID
		}
		c.subscriptions[key] = arg
	}
	c.subMu.Unlock()

	return c.sendSubscribeMultiple(args)
}

// IsAuthenticated returns whether the client is authenticated
func (c *UserDataWSClient) IsAuthenticated() bool {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	return c.isAuthenticated
}

// IsConnected returns whether the client is connected
func (c *UserDataWSClient) IsConnected() bool {
	return c.conn != nil
}

// GetSubscriptionCount returns the number of active subscriptions
func (c *UserDataWSClient) GetSubscriptionCount() int {
	c.subMu.RLock()
	defer c.subMu.RUnlock()
	return len(c.subscriptions)
}
