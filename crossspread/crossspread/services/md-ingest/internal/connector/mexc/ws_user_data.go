// Package mexc provides WebSocket user data client for MEXC exchange.
package mexc

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// UserDataHandler handles user data updates from WebSocket
type UserDataHandler interface {
	OnPositionUpdate(position *WSPositionUpdate)
	OnAccountUpdate(asset *WSAssetUpdate)
	OnOrderUpdate(order *WSOrderUpdate)
	OnPlanOrderUpdate(order *WSPlanOrderUpdate)
	OnError(err error)
	OnConnected()
	OnDisconnected()
}

// WSAssetUpdate represents account asset update
type WSAssetUpdate struct {
	Currency         string  `json:"currency"`
	AvailableBalance float64 `json:"availableBalance"`
	FrozenBalance    float64 `json:"frozenBalance"`
	PositionMargin   float64 `json:"positionMargin"`
	OrderMargin      float64 `json:"orderMargin"`
	CashBalance      float64 `json:"cashBalance"`
	Equity           float64 `json:"equity"`
	UnrealizedPnl    float64 `json:"unrealisedPnl"`
	Timestamp        int64   `json:"timestamp"`
}

// WSPlanOrderUpdate represents plan/trigger order update
type WSPlanOrderUpdate struct {
	Symbol       string  `json:"symbol"`
	OrderID      string  `json:"orderId"`
	TriggerPrice float64 `json:"triggerPrice"`
	ExecutePrice float64 `json:"executePrice"`
	Volume       float64 `json:"vol"`
	Leverage     int     `json:"leverage"`
	Side         int     `json:"side"`
	OrderType    int     `json:"orderType"`
	TriggerType  int     `json:"triggerType"`
	State        int     `json:"state"`
	CreateTime   int64   `json:"createTime"`
	UpdateTime   int64   `json:"updateTime"`
	ErrorCode    int     `json:"errorCode"`
	ErrorMsg     string  `json:"errorMsg"`
}

// UserDataWSClient handles WebSocket connections for user data
type UserDataWSClient struct {
	url       string
	conn      *websocket.Conn
	handler   UserDataHandler
	apiKey    string
	secretKey string

	writeMu sync.Mutex
	done    chan struct{}
	wg      sync.WaitGroup

	reconnect      bool
	reconnectWait  time.Duration
	maxReconnect   int
	reconnectCount int

	authenticated int32 // atomic
	authChan      chan bool

	pingInterval time.Duration

	ctx    context.Context
	cancel context.CancelFunc
}

// UserDataWSConfig holds configuration for user data WebSocket client
type UserDataWSConfig struct {
	APIKey        string
	SecretKey     string
	Handler       UserDataHandler
	PingInterval  time.Duration
	ReconnectWait time.Duration
	MaxReconnect  int
}

// NewUserDataWSClient creates a new user data WebSocket client
func NewUserDataWSClient(cfg UserDataWSConfig) *UserDataWSClient {
	if cfg.PingInterval == 0 {
		cfg.PingInterval = 20 * time.Second
	}
	if cfg.ReconnectWait == 0 {
		cfg.ReconnectWait = 5 * time.Second
	}
	if cfg.MaxReconnect == 0 {
		cfg.MaxReconnect = 3
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &UserDataWSClient{
		url:           WSPrivateURL,
		handler:       cfg.Handler,
		apiKey:        cfg.APIKey,
		secretKey:     cfg.SecretKey,
		done:          make(chan struct{}),
		reconnect:     true,
		reconnectWait: cfg.ReconnectWait,
		maxReconnect:  cfg.MaxReconnect,
		authChan:      make(chan bool, 1),
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
	c.reconnectCount = 0
	atomic.StoreInt32(&c.authenticated, 0)

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

// authenticate sends authentication request
func (c *UserDataWSClient) authenticate() error {
	timestamp := time.Now().UnixMilli()
	signStr := fmt.Sprintf("%s%d", c.apiKey, timestamp)
	signature := c.sign(signStr)

	authReq := map[string]interface{}{
		"method": "login",
		"param": map[string]interface{}{
			"apiKey":    c.apiKey,
			"reqTime":   timestamp,
			"signature": signature,
		},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(authReq)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to send auth request: %w", err)
	}

	// Wait for auth response
	select {
	case success := <-c.authChan:
		if !success {
			return fmt.Errorf("authentication rejected")
		}
	case <-time.After(10 * time.Second):
		return fmt.Errorf("authentication timeout")
	}

	return nil
}

// sign creates HMAC-SHA256 signature
func (c *UserDataWSClient) sign(message string) string {
	mac := hmac.New(sha256.New, []byte(c.secretKey))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// Close closes the WebSocket connection
func (c *UserDataWSClient) Close() error {
	c.reconnect = false
	c.cancel()

	if c.conn != nil {
		close(c.done)
		c.writeMu.Lock()
		_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.writeMu.Unlock()
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
			err := c.conn.WriteJSON(map[string]string{"method": "ping"})
			c.writeMu.Unlock()
			if err != nil {
				if c.handler != nil {
					c.handler.OnError(fmt.Errorf("ping failed: %w", err))
				}
				return
			}
		}
	}
}

// handleDisconnect handles disconnection and reconnection
func (c *UserDataWSClient) handleDisconnect() {
	atomic.StoreInt32(&c.authenticated, 0)

	if c.handler != nil {
		c.handler.OnDisconnected()
	}

	// Attempt reconnection if enabled
	if c.reconnect && c.reconnectCount < c.maxReconnect {
		c.reconnectCount++
		time.Sleep(c.reconnectWait)

		if err := c.Connect(); err != nil {
			if c.handler != nil {
				c.handler.OnError(fmt.Errorf("reconnection failed: %w", err))
			}
		} else {
			// Re-subscribe to all channels after reconnect
			c.resubscribeAll()
		}
	}
}

// resubscribeAll re-subscribes to user data channels after reconnect
func (c *UserDataWSClient) resubscribeAll() {
	// Re-subscribe to all personal channels
	_ = c.SubscribePosition()
	_ = c.SubscribeAsset()
	_ = c.SubscribeOrder()
}

// handleMessage processes incoming WebSocket messages
func (c *UserDataWSClient) handleMessage(data []byte) {
	var msg struct {
		Channel string          `json:"channel"`
		Data    json.RawMessage `json:"data"`
		Code    int             `json:"code,omitempty"`
		Success bool            `json:"success,omitempty"`
		Msg     string          `json:"msg,omitempty"`
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		if c.handler != nil {
			c.handler.OnError(fmt.Errorf("failed to parse message: %w", err))
		}
		return
	}

	// Handle auth response
	if msg.Channel == "rs.login" {
		success := msg.Code == 0 || msg.Success
		atomic.StoreInt32(&c.authenticated, userDataBoolToInt32(success))
		select {
		case c.authChan <- success:
		default:
		}
		return
	}

	// Handle pong
	if msg.Channel == "pong" {
		return
	}

	// Route by channel
	switch msg.Channel {
	case "push.personal.position":
		c.handlePositionUpdate(msg.Data)
	case "push.personal.asset":
		c.handleAssetUpdate(msg.Data)
	case "push.personal.order":
		c.handleOrderUpdate(msg.Data)
	case "push.personal.plan.order":
		c.handlePlanOrderUpdate(msg.Data)
	}
}

// handlePositionUpdate processes position updates
func (c *UserDataWSClient) handlePositionUpdate(data json.RawMessage) {
	if c.handler == nil {
		return
	}

	var update WSPositionUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		c.handler.OnError(fmt.Errorf("failed to parse position update: %w", err))
		return
	}

	c.handler.OnPositionUpdate(&update)
}

// handleAssetUpdate processes asset updates
func (c *UserDataWSClient) handleAssetUpdate(data json.RawMessage) {
	if c.handler == nil {
		return
	}

	var update WSAssetUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		c.handler.OnError(fmt.Errorf("failed to parse asset update: %w", err))
		return
	}

	c.handler.OnAccountUpdate(&update)
}

// handleOrderUpdate processes order updates
func (c *UserDataWSClient) handleOrderUpdate(data json.RawMessage) {
	if c.handler == nil {
		return
	}

	var update WSOrderUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		c.handler.OnError(fmt.Errorf("failed to parse order update: %w", err))
		return
	}

	c.handler.OnOrderUpdate(&update)
}

// handlePlanOrderUpdate processes plan order updates
func (c *UserDataWSClient) handlePlanOrderUpdate(data json.RawMessage) {
	if c.handler == nil {
		return
	}

	var update WSPlanOrderUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		c.handler.OnError(fmt.Errorf("failed to parse plan order update: %w", err))
		return
	}

	c.handler.OnPlanOrderUpdate(&update)
}

// =============================================================================
// Subscription Methods
// =============================================================================

// SubscribePosition subscribes to position updates
func (c *UserDataWSClient) SubscribePosition() error {
	if atomic.LoadInt32(&c.authenticated) != 1 {
		return fmt.Errorf("not authenticated")
	}

	req := map[string]interface{}{
		"method": "sub.personal.position",
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to subscribe position: %w", err)
	}

	return nil
}

// UnsubscribePosition unsubscribes from position updates
func (c *UserDataWSClient) UnsubscribePosition() error {
	if atomic.LoadInt32(&c.authenticated) != 1 {
		return fmt.Errorf("not authenticated")
	}

	req := map[string]interface{}{
		"method": "unsub.personal.position",
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to unsubscribe position: %w", err)
	}

	return nil
}

// SubscribeAsset subscribes to asset/account updates
func (c *UserDataWSClient) SubscribeAsset() error {
	if atomic.LoadInt32(&c.authenticated) != 1 {
		return fmt.Errorf("not authenticated")
	}

	req := map[string]interface{}{
		"method": "sub.personal.asset",
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to subscribe asset: %w", err)
	}

	return nil
}

// UnsubscribeAsset unsubscribes from asset updates
func (c *UserDataWSClient) UnsubscribeAsset() error {
	if atomic.LoadInt32(&c.authenticated) != 1 {
		return fmt.Errorf("not authenticated")
	}

	req := map[string]interface{}{
		"method": "unsub.personal.asset",
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to unsubscribe asset: %w", err)
	}

	return nil
}

// SubscribeOrder subscribes to order updates
func (c *UserDataWSClient) SubscribeOrder() error {
	if atomic.LoadInt32(&c.authenticated) != 1 {
		return fmt.Errorf("not authenticated")
	}

	req := map[string]interface{}{
		"method": "sub.personal.order",
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to subscribe order: %w", err)
	}

	return nil
}

// UnsubscribeOrder unsubscribes from order updates
func (c *UserDataWSClient) UnsubscribeOrder() error {
	if atomic.LoadInt32(&c.authenticated) != 1 {
		return fmt.Errorf("not authenticated")
	}

	req := map[string]interface{}{
		"method": "unsub.personal.order",
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to unsubscribe order: %w", err)
	}

	return nil
}

// SubscribePlanOrder subscribes to plan/trigger order updates
func (c *UserDataWSClient) SubscribePlanOrder() error {
	if atomic.LoadInt32(&c.authenticated) != 1 {
		return fmt.Errorf("not authenticated")
	}

	req := map[string]interface{}{
		"method": "sub.personal.plan.order",
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to subscribe plan order: %w", err)
	}

	return nil
}

// SubscribeAll subscribes to all user data channels
func (c *UserDataWSClient) SubscribeAll() error {
	if err := c.SubscribePosition(); err != nil {
		return err
	}
	if err := c.SubscribeAsset(); err != nil {
		return err
	}
	if err := c.SubscribeOrder(); err != nil {
		return err
	}
	return nil
}

// IsAuthenticated returns true if WebSocket is authenticated
func (c *UserDataWSClient) IsAuthenticated() bool {
	return atomic.LoadInt32(&c.authenticated) == 1
}

// IsConnected returns true if WebSocket is connected
func (c *UserDataWSClient) IsConnected() bool {
	return c.conn != nil
}

// Helper function
func userDataBoolToInt32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}
