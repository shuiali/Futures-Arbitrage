package bybit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	// WebSocket Private URLs for user data streams
	WSPrivateURLMainnet = "wss://stream.bybit.com/v5/private"
	WSPrivateURLTestnet = "wss://stream-testnet.bybit.com/v5/private"
)

// UserDataWS callback types
type OrderUpdateCallback func(update *WSOrderUpdate)
type PositionUpdateCallback func(update *WSPositionUpdate)
type ExecutionUpdateCallback func(update *WSExecutionUpdate)
type WalletUpdateCallback func(update *WSWalletUpdate)
type UserDataErrorCallback func(err error)

// UserDataWS handles private WebSocket connections for user data streams
type UserDataWS struct {
	url           string
	apiKey        string
	apiSecret     string
	conn          *websocket.Conn
	mu            sync.RWMutex
	connected     bool
	authenticated bool
	subscriptions map[string]bool

	// Callbacks
	onOrder     OrderUpdateCallback
	onPosition  PositionUpdateCallback
	onExecution ExecutionUpdateCallback
	onWallet    WalletUpdateCallback
	onError     UserDataErrorCallback
	onReconnect ReconnectCallback

	// Control
	done           chan struct{}
	ctx            context.Context
	cancel         context.CancelFunc
	reconnectCount int
}

// UserDataWSConfig holds configuration for the user data WebSocket client
type UserDataWSConfig struct {
	APIKey     string
	APISecret  string
	UseTestnet bool
}

// NewUserDataWS creates a new user data WebSocket client
func NewUserDataWS(config UserDataWSConfig) *UserDataWS {
	url := WSPrivateURLMainnet
	if config.UseTestnet {
		url = WSPrivateURLTestnet
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &UserDataWS{
		url:           url,
		apiKey:        config.APIKey,
		apiSecret:     config.APISecret,
		subscriptions: make(map[string]bool),
		done:          make(chan struct{}),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// SetOrderUpdateCallback sets the callback for order updates
func (ws *UserDataWS) SetOrderUpdateCallback(cb OrderUpdateCallback) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.onOrder = cb
}

// SetPositionUpdateCallback sets the callback for position updates
func (ws *UserDataWS) SetPositionUpdateCallback(cb PositionUpdateCallback) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.onPosition = cb
}

// SetExecutionUpdateCallback sets the callback for execution updates
func (ws *UserDataWS) SetExecutionUpdateCallback(cb ExecutionUpdateCallback) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.onExecution = cb
}

// SetWalletUpdateCallback sets the callback for wallet updates
func (ws *UserDataWS) SetWalletUpdateCallback(cb WalletUpdateCallback) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.onWallet = cb
}

// SetErrorCallback sets the callback for errors
func (ws *UserDataWS) SetErrorCallback(cb UserDataErrorCallback) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.onError = cb
}

// SetReconnectCallback sets the callback for reconnection events
func (ws *UserDataWS) SetReconnectCallback(cb ReconnectCallback) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.onReconnect = cb
}

// Connect establishes WebSocket connection and authenticates
func (ws *UserDataWS) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, ws.url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Bybit private WebSocket: %w", err)
	}

	ws.mu.Lock()
	ws.conn = conn
	ws.connected = true
	ws.reconnectCount = 0
	ws.mu.Unlock()

	log.Info().Str("url", ws.url).Msg("Connected to Bybit private WebSocket")

	// Start message reader
	go ws.readMessages()

	// Authenticate
	if err := ws.authenticate(); err != nil {
		ws.Disconnect()
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Start ping loop
	go ws.pingLoop()

	return nil
}

// authenticate sends authentication message
func (ws *UserDataWS) authenticate() error {
	expires := time.Now().UnixMilli() + 10000 // 10 seconds from now

	// Generate signature: HMAC SHA256(api_secret, "GET/realtime" + expires)
	signData := fmt.Sprintf("GET/realtime%d", expires)
	h := hmac.New(sha256.New, []byte(ws.apiSecret))
	h.Write([]byte(signData))
	signature := hex.EncodeToString(h.Sum(nil))

	authMsg := WSAuthOperation{
		Op: "auth",
		Args: []string{
			ws.apiKey,
			strconv.FormatInt(expires, 10),
			signature,
		},
	}

	if err := ws.sendJSON(authMsg); err != nil {
		return fmt.Errorf("failed to send auth message: %w", err)
	}

	// Wait for auth response with timeout
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("authentication timeout")
		case <-ticker.C:
			ws.mu.RLock()
			authenticated := ws.authenticated
			ws.mu.RUnlock()
			if authenticated {
				log.Info().Msg("Authenticated to Bybit private WebSocket")
				return nil
			}
		}
	}
}

// Disconnect closes the WebSocket connection
func (ws *UserDataWS) Disconnect() error {
	ws.cancel()
	close(ws.done)

	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.connected = false
	ws.authenticated = false
	if ws.conn != nil {
		return ws.conn.Close()
	}
	return nil
}

// IsConnected returns connection status
func (ws *UserDataWS) IsConnected() bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return ws.connected && ws.authenticated
}

// SubscribeOrders subscribes to order updates
// category: "linear", "inverse", "spot", "option" or empty for all
func (ws *UserDataWS) SubscribeOrders(category string) error {
	topic := "order"
	if category != "" {
		topic = fmt.Sprintf("order.%s", category)
	}

	ws.mu.Lock()
	ws.subscriptions[topic] = true
	ws.mu.Unlock()

	return ws.subscribe([]string{topic})
}

// SubscribePositions subscribes to position updates
// category: "linear", "inverse", "option" or empty for all
func (ws *UserDataWS) SubscribePositions(category string) error {
	topic := "position"
	if category != "" {
		topic = fmt.Sprintf("position.%s", category)
	}

	ws.mu.Lock()
	ws.subscriptions[topic] = true
	ws.mu.Unlock()

	return ws.subscribe([]string{topic})
}

// SubscribeExecutions subscribes to execution/fill updates
// category: "linear", "inverse", "spot", "option" or empty for all
func (ws *UserDataWS) SubscribeExecutions(category string) error {
	topic := "execution"
	if category != "" {
		topic = fmt.Sprintf("execution.%s", category)
	}

	ws.mu.Lock()
	ws.subscriptions[topic] = true
	ws.mu.Unlock()

	return ws.subscribe([]string{topic})
}

// SubscribeWallet subscribes to wallet balance updates
func (ws *UserDataWS) SubscribeWallet() error {
	topic := "wallet"

	ws.mu.Lock()
	ws.subscriptions[topic] = true
	ws.mu.Unlock()

	return ws.subscribe([]string{topic})
}

// SubscribeAll subscribes to all user data streams for a category
func (ws *UserDataWS) SubscribeAll(category string) error {
	topics := []string{
		"order",
		"position",
		"execution",
		"wallet",
	}

	if category != "" {
		for i, t := range topics {
			if t != "wallet" {
				topics[i] = fmt.Sprintf("%s.%s", t, category)
			}
		}
	}

	ws.mu.Lock()
	for _, topic := range topics {
		ws.subscriptions[topic] = true
	}
	ws.mu.Unlock()

	return ws.subscribe(topics)
}

// Unsubscribe removes subscriptions
func (ws *UserDataWS) Unsubscribe(topics []string) error {
	ws.mu.Lock()
	for _, topic := range topics {
		delete(ws.subscriptions, topic)
	}
	ws.mu.Unlock()

	msg := WSOperation{
		Op:   "unsubscribe",
		Args: topics,
	}

	return ws.sendJSON(msg)
}

// subscribe sends subscription request
func (ws *UserDataWS) subscribe(args []string) error {
	msg := WSOperation{
		Op:   "subscribe",
		Args: args,
	}

	log.Debug().Strs("topics", args).Msg("Subscribing to private topics")
	return ws.sendJSON(msg)
}

// sendJSON sends a JSON message
func (ws *UserDataWS) sendJSON(v interface{}) error {
	ws.mu.RLock()
	conn := ws.conn
	ws.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	return conn.WriteJSON(v)
}

// readMessages reads and processes WebSocket messages
func (ws *UserDataWS) readMessages() {
	defer func() {
		ws.mu.Lock()
		ws.connected = false
		ws.authenticated = false
		ws.mu.Unlock()
		ws.tryReconnect()
	}()

	for {
		select {
		case <-ws.done:
			return
		case <-ws.ctx.Done():
			return
		default:
			ws.mu.RLock()
			conn := ws.conn
			ws.mu.RUnlock()

			if conn == nil {
				return
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				ws.emitError(fmt.Errorf("read error: %w", err))
				return
			}

			ws.processMessage(message)
		}
	}
}

// processMessage handles incoming WebSocket messages
func (ws *UserDataWS) processMessage(data []byte) {
	// Check for auth response
	var authResp struct {
		Success bool   `json:"success"`
		Op      string `json:"op"`
		RetMsg  string `json:"ret_msg"`
		ConnId  string `json:"conn_id"`
	}
	if err := json.Unmarshal(data, &authResp); err == nil && authResp.Op == "auth" {
		if authResp.Success {
			ws.mu.Lock()
			ws.authenticated = true
			ws.mu.Unlock()
		} else {
			ws.emitError(fmt.Errorf("auth failed: %s", authResp.RetMsg))
		}
		return
	}

	// Check for pong response
	var pongResp struct {
		Op string `json:"op"`
	}
	if err := json.Unmarshal(data, &pongResp); err == nil && pongResp.Op == "pong" {
		return
	}

	// Check for subscription response
	var subResp struct {
		Success bool   `json:"success"`
		Op      string `json:"op"`
		RetMsg  string `json:"ret_msg"`
	}
	if err := json.Unmarshal(data, &subResp); err == nil && subResp.Op == "subscribe" {
		if !subResp.Success {
			ws.emitError(fmt.Errorf("subscription failed: %s", subResp.RetMsg))
		}
		return
	}

	// Parse message with topic
	var msg struct {
		Topic        string          `json:"topic"`
		CreationTime int64           `json:"creationTime"`
		Data         json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		log.Debug().Err(err).Str("data", string(data)).Msg("Failed to parse private WebSocket message")
		return
	}

	if msg.Topic == "" {
		return
	}

	// Route message based on topic
	switch {
	case strings.HasPrefix(msg.Topic, "order"):
		ws.handleOrderUpdate(msg.Data)
	case strings.HasPrefix(msg.Topic, "position"):
		ws.handlePositionUpdate(msg.Data)
	case strings.HasPrefix(msg.Topic, "execution"):
		ws.handleExecutionUpdate(msg.Data)
	case msg.Topic == "wallet":
		ws.handleWalletUpdate(msg.Data)
	}
}

// handleOrderUpdate processes order update messages
func (ws *UserDataWS) handleOrderUpdate(data json.RawMessage) {
	var updates []WSOrderUpdate
	if err := json.Unmarshal(data, &updates); err != nil {
		log.Error().Err(err).Msg("Failed to parse order update")
		return
	}

	ws.mu.RLock()
	callback := ws.onOrder
	ws.mu.RUnlock()

	if callback != nil {
		for _, update := range updates {
			callback(&update)
		}
	}
}

// handlePositionUpdate processes position update messages
func (ws *UserDataWS) handlePositionUpdate(data json.RawMessage) {
	var updates []WSPositionUpdate
	if err := json.Unmarshal(data, &updates); err != nil {
		log.Error().Err(err).Msg("Failed to parse position update")
		return
	}

	ws.mu.RLock()
	callback := ws.onPosition
	ws.mu.RUnlock()

	if callback != nil {
		for _, update := range updates {
			callback(&update)
		}
	}
}

// handleExecutionUpdate processes execution update messages
func (ws *UserDataWS) handleExecutionUpdate(data json.RawMessage) {
	var updates []WSExecutionUpdate
	if err := json.Unmarshal(data, &updates); err != nil {
		log.Error().Err(err).Msg("Failed to parse execution update")
		return
	}

	ws.mu.RLock()
	callback := ws.onExecution
	ws.mu.RUnlock()

	if callback != nil {
		for _, update := range updates {
			callback(&update)
		}
	}
}

// handleWalletUpdate processes wallet update messages
func (ws *UserDataWS) handleWalletUpdate(data json.RawMessage) {
	var updates []WSWalletUpdate
	if err := json.Unmarshal(data, &updates); err != nil {
		log.Error().Err(err).Msg("Failed to parse wallet update")
		return
	}

	ws.mu.RLock()
	callback := ws.onWallet
	ws.mu.RUnlock()

	if callback != nil {
		for _, update := range updates {
			callback(&update)
		}
	}
}

// pingLoop sends periodic ping messages
func (ws *UserDataWS) pingLoop() {
	ticker := time.NewTicker(WSPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ws.done:
			return
		case <-ws.ctx.Done():
			return
		case <-ticker.C:
			ping := map[string]string{"op": "ping"}
			if err := ws.sendJSON(ping); err != nil {
				ws.emitError(fmt.Errorf("ping error: %w", err))
			}
		}
	}
}

// tryReconnect attempts to reconnect with exponential backoff
func (ws *UserDataWS) tryReconnect() {
	select {
	case <-ws.done:
		return
	case <-ws.ctx.Done():
		return
	default:
	}

	ws.mu.Lock()
	ws.reconnectCount++
	count := ws.reconnectCount
	subs := make([]string, 0, len(ws.subscriptions))
	for topic := range ws.subscriptions {
		subs = append(subs, topic)
	}
	ws.mu.Unlock()

	if count > WSMaxReconnects {
		ws.emitError(fmt.Errorf("max reconnection attempts reached"))
		return
	}

	delay := WSReconnectDelay * time.Duration(count)
	log.Warn().
		Int("attempt", count).
		Dur("delay", delay).
		Msg("Reconnecting to Bybit private WebSocket")

	time.Sleep(delay)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := ws.Connect(ctx); err != nil {
		ws.emitError(fmt.Errorf("reconnection failed: %w", err))
		ws.tryReconnect()
		return
	}

	// Resubscribe to previous topics
	if len(subs) > 0 {
		if err := ws.subscribe(subs); err != nil {
			ws.emitError(fmt.Errorf("resubscription failed: %w", err))
		}
	}

	// Emit reconnect callback
	ws.mu.RLock()
	callback := ws.onReconnect
	ws.mu.RUnlock()

	if callback != nil {
		callback()
	}
}

// emitError calls the error callback if set
func (ws *UserDataWS) emitError(err error) {
	ws.mu.RLock()
	callback := ws.onError
	ws.mu.RUnlock()

	if callback != nil {
		callback(err)
	} else {
		log.Error().Err(err).Msg("Bybit private WebSocket error")
	}
}
