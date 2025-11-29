package htx

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// WSUserDataClient handles private WebSocket user data for HTX
type WSUserDataClient struct {
	url               string
	apiKey            string
	secretKey         string
	conn              *websocket.Conn
	connMu            sync.Mutex
	state             atomic.Int32
	subscriptions     *SubscriptionManager
	reconnectDelay    time.Duration
	maxReconnectDelay time.Duration
	pingInterval      time.Duration
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	authenticated     atomic.Bool
	onConnect         func()
	onDisconnect      func()
	onError           func(error)
	onOrderUpdate     func(order *WSOrderNotify)
	onPositionUpdate  func(position *WSPositionNotify)
	onAccountUpdate   func(account *WSAccountNotify)
	lastPing          atomic.Int64
}

// NewWSUserDataClient creates a new WebSocket user data client
func NewWSUserDataClient(url, apiKey, secretKey string) *WSUserDataClient {
	ctx, cancel := context.WithCancel(context.Background())
	client := &WSUserDataClient{
		url:               url,
		apiKey:            apiKey,
		secretKey:         secretKey,
		subscriptions:     NewSubscriptionManager(),
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
func (c *WSUserDataClient) SetCallbacks(onConnect, onDisconnect func(), onError func(error)) {
	c.onConnect = onConnect
	c.onDisconnect = onDisconnect
	c.onError = onError
}

// SetOrderCallback sets order update callback
func (c *WSUserDataClient) SetOrderCallback(callback func(order *WSOrderNotify)) {
	c.onOrderUpdate = callback
}

// SetPositionCallback sets position update callback
func (c *WSUserDataClient) SetPositionCallback(callback func(position *WSPositionNotify)) {
	c.onPositionUpdate = callback
}

// SetAccountCallback sets account update callback
func (c *WSUserDataClient) SetAccountCallback(callback func(account *WSAccountNotify)) {
	c.onAccountUpdate = callback
}

// Connect establishes WebSocket connection
func (c *WSUserDataClient) Connect() error {
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

	// Authenticate
	if err := c.authenticate(); err != nil {
		log.Printf("[HTX WS User] authentication failed: %v", err)
		return err
	}

	return nil
}

// Disconnect closes the WebSocket connection
func (c *WSUserDataClient) Disconnect() {
	c.cancel()
	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.connMu.Unlock()
	c.state.Store(int32(StateDisconnected))
	c.authenticated.Store(false)
	c.wg.Wait()
}

// GetState returns the current connection state
func (c *WSUserDataClient) GetState() ConnectionState {
	return ConnectionState(c.state.Load())
}

// IsAuthenticated returns whether the connection is authenticated
func (c *WSUserDataClient) IsAuthenticated() bool {
	return c.authenticated.Load()
}

// authenticate sends authentication request
func (c *WSUserDataClient) authenticate() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	// Generate signature for authentication
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05")

	// Parse URL to get host
	parsedURL, err := url.Parse(c.url)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}

	// Build signature string
	params := map[string]string{
		"AccessKeyId":      c.apiKey,
		"SignatureMethod":  "HmacSHA256",
		"SignatureVersion": "2.1",
		"Timestamp":        timestamp,
	}

	// Sort keys
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build query string
	var queryParts []string
	for _, k := range keys {
		queryParts = append(queryParts, fmt.Sprintf("%s=%s", k, url.QueryEscape(params[k])))
	}
	queryString := strings.Join(queryParts, "&")

	// Build sign string
	signString := fmt.Sprintf("GET\n%s\n%s\n%s", parsedURL.Host, parsedURL.Path, queryString)

	// Generate HMAC-SHA256 signature
	h := hmac.New(sha256.New, []byte(c.secretKey))
	h.Write([]byte(signString))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// Send auth request
	authReq := WSAuthRequest{
		Op:               "auth",
		Type:             "api",
		AccessKeyID:      c.apiKey,
		SignatureMethod:  "HmacSHA256",
		SignatureVersion: "2.1",
		Timestamp:        timestamp,
		Signature:        signature,
	}

	data, err := json.Marshal(authReq)
	if err != nil {
		return fmt.Errorf("marshal auth request: %w", err)
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("send auth request: %w", err)
	}

	return nil
}

// readMessages handles incoming WebSocket messages
func (c *WSUserDataClient) readMessages() {
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
			log.Printf("[HTX WS User] read error: %v", err)
			if c.onError != nil {
				c.onError(err)
			}
			c.handleDisconnect()
			continue
		}

		// Decompress GZIP data
		decompressed, err := c.decompressGzip(message)
		if err != nil {
			log.Printf("[HTX WS User] decompress error: %v", err)
			continue
		}

		c.handleMessage(decompressed)
	}
}

// decompressGzip decompresses gzip data
func (c *WSUserDataClient) decompressGzip(data []byte) ([]byte, error) {
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

// handleMessage processes a single message
func (c *WSUserDataClient) handleMessage(data []byte) {
	// Parse base response
	var resp struct {
		Op      string      `json:"op"`
		Type    string      `json:"type"`
		Topic   string      `json:"topic"`
		Ts      int64       `json:"ts"`
		Cid     string      `json:"cid"`
		ErrCode int         `json:"err-code"`
		ErrMsg  string      `json:"err-msg"`
		Data    interface{} `json:"data"`
		Event   string      `json:"event"`
		Ping    int64       `json:"ping"`
		Pong    int64       `json:"pong"`
		Code    int         `json:"code"`
		Msg     string      `json:"msg"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Printf("[HTX WS User] unmarshal error: %v", err)
		return
	}

	// Handle ping
	if resp.Ping > 0 {
		c.sendPong(resp.Ping)
		c.lastPing.Store(time.Now().UnixMilli())
		return
	}

	// Handle op responses
	if resp.Op != "" {
		c.handleOpResponse(resp.Op, resp.Type, resp.ErrCode, resp.ErrMsg, resp.Topic, data)
		return
	}

	// Handle push data by topic
	if resp.Topic != "" {
		c.handleTopicData(resp.Topic, data)
		return
	}
}

// handleOpResponse handles operation responses
func (c *WSUserDataClient) handleOpResponse(op, _ string, errCode int, errMsg, topic string, data []byte) {
	switch op {
	case "auth":
		if errCode == 0 {
			log.Printf("[HTX WS User] authenticated successfully")
			c.authenticated.Store(true)
			// Resubscribe after authentication
			c.resubscribe()
		} else {
			log.Printf("[HTX WS User] authentication failed: %d - %s", errCode, errMsg)
			c.authenticated.Store(false)
		}
	case "sub":
		if errCode == 0 {
			log.Printf("[HTX WS User] subscribed to: %s", topic)
		} else {
			log.Printf("[HTX WS User] subscription failed for %s: %d - %s", topic, errCode, errMsg)
		}
	case "unsub":
		if errCode == 0 {
			log.Printf("[HTX WS User] unsubscribed from: %s", topic)
		} else {
			log.Printf("[HTX WS User] unsubscription failed for %s: %d - %s", topic, errCode, errMsg)
		}
	case "notify":
		c.handleTopicData(topic, data)
	}
}

// handleTopicData handles topic data
func (c *WSUserDataClient) handleTopicData(topic string, data []byte) {
	// Check subscriptions first
	sub, ok := c.subscriptions.Get(topic)
	if ok && sub.Callback != nil {
		sub.Callback(data)
		return
	}

	// Handle known topics
	if strings.Contains(topic, "orders_cross") {
		c.handleOrderUpdate(data)
	} else if strings.Contains(topic, "positions_cross") {
		c.handlePositionUpdate(data)
	} else if strings.Contains(topic, "accounts_cross") {
		c.handleAccountUpdate(data)
	}
}

// handleOrderUpdate handles order update notifications
func (c *WSUserDataClient) handleOrderUpdate(data []byte) {
	var resp struct {
		Op    string          `json:"op"`
		Topic string          `json:"topic"`
		Ts    int64           `json:"ts"`
		Data  []WSOrderNotify `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Printf("[HTX WS User] order update unmarshal error: %v", err)
		return
	}

	if c.onOrderUpdate != nil {
		for i := range resp.Data {
			c.onOrderUpdate(&resp.Data[i])
		}
	}
}

// handlePositionUpdate handles position update notifications
func (c *WSUserDataClient) handlePositionUpdate(data []byte) {
	var resp struct {
		Op    string             `json:"op"`
		Topic string             `json:"topic"`
		Ts    int64              `json:"ts"`
		Data  []WSPositionNotify `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Printf("[HTX WS User] position update unmarshal error: %v", err)
		return
	}

	if c.onPositionUpdate != nil {
		for i := range resp.Data {
			c.onPositionUpdate(&resp.Data[i])
		}
	}
}

// handleAccountUpdate handles account update notifications
func (c *WSUserDataClient) handleAccountUpdate(data []byte) {
	var resp struct {
		Op    string            `json:"op"`
		Topic string            `json:"topic"`
		Ts    int64             `json:"ts"`
		Data  []WSAccountNotify `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Printf("[HTX WS User] account update unmarshal error: %v", err)
		return
	}

	if c.onAccountUpdate != nil {
		for i := range resp.Data {
			c.onAccountUpdate(&resp.Data[i])
		}
	}
}

// sendPong sends pong response
func (c *WSUserDataClient) sendPong(pingTs int64) {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return
	}

	pong := map[string]interface{}{
		"op": "pong",
		"ts": pingTs,
	}
	data, err := json.Marshal(pong)
	if err != nil {
		log.Printf("[HTX WS User] marshal pong error: %v", err)
		return
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("[HTX WS User] send pong error: %v", err)
	}
}

// pingHandler monitors connection health
func (c *WSUserDataClient) pingHandler() {
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
				log.Printf("[HTX WS User] no ping received in 60s, reconnecting")
				c.handleDisconnect()
			}
		}
	}
}

// handleDisconnect handles WebSocket disconnection
func (c *WSUserDataClient) handleDisconnect() {
	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.connMu.Unlock()

	c.state.Store(int32(StateReconnecting))
	c.authenticated.Store(false)

	if c.onDisconnect != nil {
		c.onDisconnect()
	}

	// Attempt reconnection
	go c.reconnect()
}

// reconnect attempts to reconnect with exponential backoff
func (c *WSUserDataClient) reconnect() {
	delay := c.reconnectDelay

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		log.Printf("[HTX WS User] reconnecting in %v", delay)
		time.Sleep(delay)

		if err := c.Connect(); err != nil {
			log.Printf("[HTX WS User] reconnect failed: %v", err)
			delay *= 2
			if delay > c.maxReconnectDelay {
				delay = c.maxReconnectDelay
			}
			continue
		}

		log.Printf("[HTX WS User] reconnected successfully")
		return
	}
}

// resubscribe resubscribes to all subscriptions after reconnection
func (c *WSUserDataClient) resubscribe() {
	if !c.authenticated.Load() {
		return
	}

	subs := c.subscriptions.GetAll()
	for _, sub := range subs {
		if err := c.sendSubscription(sub.Topic); err != nil {
			log.Printf("[HTX WS User] resubscribe error for %s: %v", sub.Topic, err)
		}
	}
}

// sendSubscription sends a subscription request
func (c *WSUserDataClient) sendSubscription(topic string) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	req := map[string]interface{}{
		"op":    "sub",
		"topic": topic,
		"cid":   fmt.Sprintf("sub_%d", time.Now().UnixNano()),
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
func (c *WSUserDataClient) sendUnsubscription(topic string) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	req := map[string]interface{}{
		"op":    "unsub",
		"topic": topic,
		"cid":   fmt.Sprintf("unsub_%d", time.Now().UnixNano()),
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

// SubscribeOrders subscribes to order updates
// symbol: specific contract code (e.g., "BTC-USDT") or "*" for all
func (c *WSUserDataClient) SubscribeOrders(symbol string, callback func(data []byte)) error {
	topic := fmt.Sprintf("orders_cross.%s", symbol)
	c.subscriptions.Add(topic, callback)

	if c.authenticated.Load() {
		return c.sendSubscription(topic)
	}
	return nil
}

// UnsubscribeOrders unsubscribes from order updates
func (c *WSUserDataClient) UnsubscribeOrders(symbol string) error {
	topic := fmt.Sprintf("orders_cross.%s", symbol)
	c.subscriptions.Remove(topic)
	return c.sendUnsubscription(topic)
}

// SubscribePositions subscribes to position updates
// symbol: specific contract code (e.g., "BTC-USDT") or "*" for all
func (c *WSUserDataClient) SubscribePositions(symbol string, callback func(data []byte)) error {
	topic := fmt.Sprintf("positions_cross.%s", symbol)
	c.subscriptions.Add(topic, callback)

	if c.authenticated.Load() {
		return c.sendSubscription(topic)
	}
	return nil
}

// UnsubscribePositions unsubscribes from position updates
func (c *WSUserDataClient) UnsubscribePositions(symbol string) error {
	topic := fmt.Sprintf("positions_cross.%s", symbol)
	c.subscriptions.Remove(topic)
	return c.sendUnsubscription(topic)
}

// SubscribeAccounts subscribes to account updates
// marginAccount: margin account (e.g., "USDT") or "*" for all
func (c *WSUserDataClient) SubscribeAccounts(marginAccount string, callback func(data []byte)) error {
	topic := fmt.Sprintf("accounts_cross.%s", marginAccount)
	c.subscriptions.Add(topic, callback)

	if c.authenticated.Load() {
		return c.sendSubscription(topic)
	}
	return nil
}

// UnsubscribeAccounts unsubscribes from account updates
func (c *WSUserDataClient) UnsubscribeAccounts(marginAccount string) error {
	topic := fmt.Sprintf("accounts_cross.%s", marginAccount)
	c.subscriptions.Remove(topic)
	return c.sendUnsubscription(topic)
}

// SubscribeMatchOrders subscribes to match order updates (execution only)
// symbol: specific contract code (e.g., "BTC-USDT") or "*" for all
func (c *WSUserDataClient) SubscribeMatchOrders(symbol string, callback func(data []byte)) error {
	topic := fmt.Sprintf("matchOrders_cross.%s", symbol)
	c.subscriptions.Add(topic, callback)

	if c.authenticated.Load() {
		return c.sendSubscription(topic)
	}
	return nil
}

// UnsubscribeMatchOrders unsubscribes from match order updates
func (c *WSUserDataClient) UnsubscribeMatchOrders(symbol string) error {
	topic := fmt.Sprintf("matchOrders_cross.%s", symbol)
	c.subscriptions.Remove(topic)
	return c.sendUnsubscription(topic)
}

// SubscribeLiquidationOrders subscribes to liquidation order updates
// symbol: specific contract code (e.g., "BTC-USDT") or "*" for all
func (c *WSUserDataClient) SubscribeLiquidationOrders(symbol string, callback func(data []byte)) error {
	topic := fmt.Sprintf("liquidation_orders_cross.%s", symbol)
	c.subscriptions.Add(topic, callback)

	if c.authenticated.Load() {
		return c.sendSubscription(topic)
	}
	return nil
}

// UnsubscribeLiquidationOrders unsubscribes from liquidation order updates
func (c *WSUserDataClient) UnsubscribeLiquidationOrders(symbol string) error {
	topic := fmt.Sprintf("liquidation_orders_cross.%s", symbol)
	c.subscriptions.Remove(topic)
	return c.sendUnsubscription(topic)
}

// SubscribeTriggerOrders subscribes to trigger/conditional order updates
// symbol: specific contract code (e.g., "BTC-USDT") or "*" for all
func (c *WSUserDataClient) SubscribeTriggerOrders(symbol string, callback func(data []byte)) error {
	topic := fmt.Sprintf("trigger_order_cross.%s", symbol)
	c.subscriptions.Add(topic, callback)

	if c.authenticated.Load() {
		return c.sendSubscription(topic)
	}
	return nil
}

// UnsubscribeTriggerOrders unsubscribes from trigger order updates
func (c *WSUserDataClient) UnsubscribeTriggerOrders(symbol string) error {
	topic := fmt.Sprintf("trigger_order_cross.%s", symbol)
	c.subscriptions.Remove(topic)
	return c.sendUnsubscription(topic)
}

// SubscribeContractInfo subscribes to contract info updates
// symbol: specific contract code (e.g., "BTC-USDT") or "*" for all
func (c *WSUserDataClient) SubscribeContractInfo(symbol string, callback func(data []byte)) error {
	topic := fmt.Sprintf("contract_info.%s", symbol)
	c.subscriptions.Add(topic, callback)

	if c.authenticated.Load() {
		return c.sendSubscription(topic)
	}
	return nil
}

// UnsubscribeContractInfo unsubscribes from contract info updates
func (c *WSUserDataClient) UnsubscribeContractInfo(symbol string) error {
	topic := fmt.Sprintf("contract_info.%s", symbol)
	c.subscriptions.Remove(topic)
	return c.sendUnsubscription(topic)
}
