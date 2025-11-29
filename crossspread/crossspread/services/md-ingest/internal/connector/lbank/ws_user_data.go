package lbank

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// UserDataHandler defines callback functions for user data events
type UserDataHandler struct {
	OnOrderUpdate    func(order *SpotOrder)
	OnAssetUpdate    func(asset string, free, frozen float64)
	OnPositionUpdate func(position *ContractPosition)
	OnAccountUpdate  func(account *ContractAccount)
	OnError          func(err error)
	OnConnect        func()
	OnDisconnect     func()
}

// WsUserDataClient handles WebSocket connections for private user data
type WsUserDataClient struct {
	restClient     *RestClient
	wsURL          string
	subscribeKey   string
	conn           *websocket.Conn
	handler        *UserDataHandler
	pingInterval   time.Duration
	reconnectDelay time.Duration
	keyRefreshInt  time.Duration

	mu          sync.RWMutex
	done        chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	isConnected bool

	// Subscriptions
	orderPairs []string
	subAssets  bool
}

// NewWsUserDataClient creates a new user data WebSocket client
func NewWsUserDataClient(restClient *RestClient, config *ClientConfig, handler *UserDataHandler) *WsUserDataClient {
	pingInterval := config.PingInterval
	if pingInterval == 0 {
		pingInterval = 30 * time.Second
	}

	reconnectDelay := config.ReconnectDelay
	if reconnectDelay == 0 {
		reconnectDelay = 5 * time.Second
	}

	return &WsUserDataClient{
		restClient:     restClient,
		wsURL:          SpotWsBaseURL, // User data goes through spot WebSocket
		handler:        handler,
		pingInterval:   pingInterval,
		reconnectDelay: reconnectDelay,
		keyRefreshInt:  45 * time.Minute, // Refresh key before 60 min expiry
		done:           make(chan struct{}),
		orderPairs:     make([]string, 0),
	}
}

// Connect establishes the WebSocket connection for user data
func (c *WsUserDataClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.isConnected {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	// Get subscribe key first
	key, err := c.restClient.GetSubscribeKey(ctx)
	if err != nil {
		return fmt.Errorf("failed to get subscribe key: %w", err)
	}
	c.subscribeKey = key

	c.ctx, c.cancel = context.WithCancel(ctx)

	// Connect with subscribe key
	wsURL := c.wsURL + "?subscribeKey=" + c.subscribeKey

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	log.Info().Str("url", wsURL).Msg("Connecting to LBank user data WebSocket")

	conn, _, err := dialer.DialContext(c.ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.isConnected = true
	c.done = make(chan struct{})
	c.mu.Unlock()

	log.Info().Msg("Connected to LBank user data WebSocket")

	if c.handler != nil && c.handler.OnConnect != nil {
		c.handler.OnConnect()
	}

	// Start background loops
	go c.readLoop()
	go c.pingLoop()
	go c.keyRefreshLoop()

	// Resubscribe
	c.resubscribe()

	return nil
}

// Disconnect closes the WebSocket connection
func (c *WsUserDataClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isConnected {
		return nil
	}

	c.isConnected = false
	close(c.done)

	if c.cancel != nil {
		c.cancel()
	}

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}

	if c.handler != nil && c.handler.OnDisconnect != nil {
		c.handler.OnDisconnect()
	}

	return nil
}

// IsConnected returns connection status
func (c *WsUserDataClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isConnected
}

// SubscribeOrderUpdates subscribes to order updates for given pairs
func (c *WsUserDataClient) SubscribeOrderUpdates(pairs []string) error {
	c.mu.Lock()
	c.orderPairs = append(c.orderPairs, pairs...)
	c.mu.Unlock()

	if !c.IsConnected() {
		return nil
	}

	for _, pair := range pairs {
		msg := map[string]string{
			"action":    ActionSubscribe,
			"subscribe": "orderUpdate",
			"pair":      pair,
		}
		if err := c.sendMessage(msg); err != nil {
			return err
		}
	}

	return nil
}

// SubscribeAssetUpdates subscribes to asset/balance updates
func (c *WsUserDataClient) SubscribeAssetUpdates() error {
	c.mu.Lock()
	c.subAssets = true
	c.mu.Unlock()

	if !c.IsConnected() {
		return nil
	}

	msg := map[string]string{
		"action":    ActionSubscribe,
		"subscribe": "assetUpdate",
	}
	return c.sendMessage(msg)
}

// UnsubscribeOrderUpdates unsubscribes from order updates
func (c *WsUserDataClient) UnsubscribeOrderUpdates(pairs []string) error {
	c.mu.Lock()
	newPairs := make([]string, 0)
	for _, p := range c.orderPairs {
		found := false
		for _, up := range pairs {
			if p == up {
				found = true
				break
			}
		}
		if !found {
			newPairs = append(newPairs, p)
		}
	}
	c.orderPairs = newPairs
	c.mu.Unlock()

	if !c.IsConnected() {
		return nil
	}

	for _, pair := range pairs {
		msg := map[string]string{
			"action":    ActionUnsubscribe,
			"subscribe": "orderUpdate",
			"pair":      pair,
		}
		if err := c.sendMessage(msg); err != nil {
			return err
		}
	}

	return nil
}

// sendMessage sends a message over WebSocket
func (c *WsUserDataClient) sendMessage(msg interface{}) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	log.Debug().Str("msg", string(data)).Msg("Sending user data WS message")
	return conn.WriteMessage(websocket.TextMessage, data)
}

// resubscribe resubscribes after reconnect
func (c *WsUserDataClient) resubscribe() {
	c.mu.RLock()
	pairs := make([]string, len(c.orderPairs))
	copy(pairs, c.orderPairs)
	subAssets := c.subAssets
	c.mu.RUnlock()

	for _, pair := range pairs {
		msg := map[string]string{
			"action":    ActionSubscribe,
			"subscribe": "orderUpdate",
			"pair":      pair,
		}
		if err := c.sendMessage(msg); err != nil {
			log.Error().Err(err).Str("pair", pair).Msg("Failed to resubscribe to order updates")
		}
	}

	if subAssets {
		msg := map[string]string{
			"action":    ActionSubscribe,
			"subscribe": "assetUpdate",
		}
		if err := c.sendMessage(msg); err != nil {
			log.Error().Err(err).Msg("Failed to resubscribe to asset updates")
		}
	}
}

// readLoop reads messages from WebSocket
func (c *WsUserDataClient) readLoop() {
	defer func() {
		c.mu.Lock()
		c.isConnected = false
		c.mu.Unlock()

		if c.handler != nil && c.handler.OnDisconnect != nil {
			c.handler.OnDisconnect()
		}
	}()

	for {
		select {
		case <-c.done:
			return
		default:
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn == nil {
				return
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Error().Err(err).Msg("User data WebSocket read error")
				if c.handler != nil && c.handler.OnError != nil {
					c.handler.OnError(err)
				}
				return
			}

			c.handleMessage(message)
		}
	}
}

// pingLoop sends periodic ping messages
func (c *WsUserDataClient) pingLoop() {
	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if !c.IsConnected() {
				return
			}

			pingID := fmt.Sprintf("ping_%d", time.Now().UnixNano())
			msg := map[string]string{
				"action": ActionPing,
				"ping":   pingID,
			}

			if err := c.sendMessage(msg); err != nil {
				log.Error().Err(err).Msg("Failed to send user data ping")
			}
		}
	}
}

// keyRefreshLoop refreshes the subscribe key periodically
func (c *WsUserDataClient) keyRefreshLoop() {
	ticker := time.NewTicker(c.keyRefreshInt)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if !c.IsConnected() {
				return
			}

			c.mu.RLock()
			key := c.subscribeKey
			c.mu.RUnlock()

			if key == "" {
				continue
			}

			if err := c.restClient.RefreshSubscribeKey(c.ctx, key); err != nil {
				log.Error().Err(err).Msg("Failed to refresh subscribe key")
				// Try to reconnect with new key
				go func() {
					c.Disconnect()
					time.Sleep(c.reconnectDelay)
					if err := c.Connect(c.ctx); err != nil {
						log.Error().Err(err).Msg("Failed to reconnect after key refresh failure")
					}
				}()
			} else {
				log.Debug().Msg("Successfully refreshed subscribe key")
			}
		}
	}
}

// handleMessage processes incoming WebSocket messages
func (c *WsUserDataClient) handleMessage(message []byte) {
	var baseMsg map[string]interface{}
	if err := json.Unmarshal(message, &baseMsg); err != nil {
		log.Error().Err(err).Str("msg", string(message)).Msg("Failed to parse user data message")
		return
	}

	// Check for pong response
	if action, ok := baseMsg["action"].(string); ok && action == ActionPong {
		log.Debug().Msg("Received user data pong")
		return
	}

	// Determine message type
	msgType, _ := baseMsg["type"].(string)

	switch msgType {
	case "orderUpdate":
		c.handleOrderUpdate(message)
	case "assetUpdate":
		c.handleAssetUpdate(message)
	default:
		log.Debug().Str("type", msgType).Str("msg", string(message)).Msg("Unhandled user data message type")
	}
}

// OrderUpdateMessage represents an order update from WebSocket
type OrderUpdateMessage struct {
	Type   string `json:"type"`
	Pair   string `json:"pair"`
	Server string `json:"SERVER"`
	TS     string `json:"TS"`
	Order  struct {
		OrderID    string  `json:"orderId"`
		Symbol     string  `json:"symbol"`
		Type       string  `json:"type"` // buy/sell
		Price      float64 `json:"price"`
		Amount     float64 `json:"amount"`
		DealAmount float64 `json:"dealAmount"`
		AvgPrice   float64 `json:"avgPrice"`
		Status     int     `json:"status"`
		CreateTime int64   `json:"createTime"`
	} `json:"order"`
}

// AssetUpdateMessage represents an asset update from WebSocket
type AssetUpdateMessage struct {
	Type   string `json:"type"`
	Server string `json:"SERVER"`
	TS     string `json:"TS"`
	Asset  struct {
		AssetCode string  `json:"assetCode"`
		Free      float64 `json:"free"`
		Frozen    float64 `json:"frozen"`
	} `json:"asset"`
}

// handleOrderUpdate processes order update messages
func (c *WsUserDataClient) handleOrderUpdate(message []byte) {
	if c.handler == nil || c.handler.OnOrderUpdate == nil {
		return
	}

	var msg OrderUpdateMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Error().Err(err).Msg("Failed to parse order update message")
		return
	}

	order := &SpotOrder{
		OrderID:    msg.Order.OrderID,
		Symbol:     msg.Order.Symbol,
		Type:       msg.Order.Type,
		Price:      msg.Order.Price,
		Amount:     msg.Order.Amount,
		DealAmount: msg.Order.DealAmount,
		AvgPrice:   msg.Order.AvgPrice,
		Status:     msg.Order.Status,
		CreateTime: msg.Order.CreateTime,
	}

	c.handler.OnOrderUpdate(order)
}

// handleAssetUpdate processes asset update messages
func (c *WsUserDataClient) handleAssetUpdate(message []byte) {
	if c.handler == nil || c.handler.OnAssetUpdate == nil {
		return
	}

	var msg AssetUpdateMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Error().Err(err).Msg("Failed to parse asset update message")
		return
	}

	c.handler.OnAssetUpdate(msg.Asset.AssetCode, msg.Asset.Free, msg.Asset.Frozen)
}

// Reconnect attempts to reconnect the WebSocket
func (c *WsUserDataClient) Reconnect(ctx context.Context) error {
	c.Disconnect()
	time.Sleep(c.reconnectDelay)
	return c.Connect(ctx)
}

// GetSubscribeKey returns the current subscribe key
func (c *WsUserDataClient) GetSubscribeKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.subscribeKey
}
