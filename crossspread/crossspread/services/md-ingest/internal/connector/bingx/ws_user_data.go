// Package bingx provides WebSocket user data client for BingX Perpetual Futures.
package bingx

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

// WSUserDataHandler handles user data stream callbacks
type WSUserDataHandler struct {
	OnAccountUpdate    func(update *WSAccountUpdate)
	OnOrderTradeUpdate func(update *WSOrderTradeUpdate)
	OnListenKeyExpired func(listenKey string)
	OnError            func(err error)
	OnConnect          func()
	OnDisconnect       func(err error)
}

// WSUserDataClient handles WebSocket user data connections for BingX
type WSUserDataClient struct {
	restClient        *RESTClient
	handler           *WSUserDataHandler
	conn              *websocket.Conn
	listenKey         string
	mu                sync.RWMutex
	writeMu           sync.Mutex
	reconnectDelay    time.Duration
	maxRetries        int
	ctx               context.Context
	cancel            context.CancelFunc
	isConnected       atomic.Bool
	pingInterval      time.Duration
	keepAliveInterval time.Duration
	stopPing          chan struct{}
	stopKeepAlive     chan struct{}
	done              chan struct{}
}

// NewWSUserDataClient creates a new WebSocket user data client
func NewWSUserDataClient(restClient *RESTClient, handler *WSUserDataHandler) *WSUserDataClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &WSUserDataClient{
		restClient:        restClient,
		handler:           handler,
		reconnectDelay:    5 * time.Second,
		maxRetries:        10,
		ctx:               ctx,
		cancel:            cancel,
		pingInterval:      20 * time.Second,
		keepAliveInterval: 30 * time.Minute, // Extend listen key every 30 minutes
	}
}

// Connect establishes WebSocket connection for user data stream
func (c *WSUserDataClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isConnected.Load() {
		return nil // Already connected
	}

	return c.connectInternal()
}

func (c *WSUserDataClient) connectInternal() error {
	// Create listen key
	listenKeyResp, err := c.restClient.CreateListenKey(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to create listen key: %w", err)
	}

	c.listenKey = listenKeyResp.ListenKey

	// Build WebSocket URL with listen key
	wsURL := fmt.Sprintf(WSUserDataURLFormat, c.listenKey)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(c.ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to BingX WS: %w", err)
	}

	c.conn = conn
	c.isConnected.Store(true)
	c.stopPing = make(chan struct{})
	c.stopKeepAlive = make(chan struct{})
	c.done = make(chan struct{})

	// Start message handler
	go c.readLoop()

	// Start ping loop
	go c.pingLoop()

	// Start keep alive loop (extend listen key)
	go c.keepAliveLoop()

	if c.handler != nil && c.handler.OnConnect != nil {
		c.handler.OnConnect()
	}

	log.Printf("[BingX WS] Connected to user data stream")
	return nil
}

// readLoop reads messages from the WebSocket connection
func (c *WSUserDataClient) readLoop() {
	defer func() {
		c.isConnected.Store(false)
		close(c.stopPing)
		close(c.stopKeepAlive)

		if c.handler != nil && c.handler.OnDisconnect != nil {
			c.handler.OnDisconnect(nil)
		}
	}()

	for {
		select {
		case <-c.done:
			return
		case <-c.ctx.Done():
			return
		default:
		}

		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[BingX WS] Read error: %v", err)
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
		case <-c.done:
			return
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if err := c.sendPing(); err != nil {
				log.Printf("[BingX WS] Ping error: %v", err)
				return
			}
		}
	}
}

// keepAliveLoop extends listen key periodically
func (c *WSUserDataClient) keepAliveLoop() {
	ticker := time.NewTicker(c.keepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopKeepAlive:
			return
		case <-c.done:
			return
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if err := c.extendListenKey(); err != nil {
				log.Printf("[BingX WS] Failed to extend listen key: %v", err)
				// Try to reconnect with new listen key
				c.handleReconnect()
				return
			}
			log.Printf("[BingX WS] Listen key extended successfully")
		}
	}
}

// sendPing sends a ping message
func (c *WSUserDataClient) sendPing() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("connection not established")
	}

	// BingX uses text "Ping" message
	return c.conn.WriteMessage(websocket.TextMessage, []byte("Ping"))
}

// extendListenKey extends the listen key validity
func (c *WSUserDataClient) extendListenKey() error {
	c.mu.RLock()
	listenKey := c.listenKey
	c.mu.RUnlock()

	if listenKey == "" {
		return fmt.Errorf("no listen key to extend")
	}

	return c.restClient.ExtendListenKey(c.ctx, listenKey)
}

// handleMessage processes incoming WebSocket messages
func (c *WSUserDataClient) handleMessage(data []byte) {
	// Handle Pong response
	if string(data) == "Pong" {
		return
	}

	// First try to determine event type
	var baseEvent WSUserDataEvent
	if err := json.Unmarshal(data, &baseEvent); err != nil {
		log.Printf("[BingX WS] Failed to parse message: %v", err)
		return
	}

	switch baseEvent.E {
	case WSEventAccountUpdate:
		c.handleAccountUpdate(data)
	case WSEventOrderTradeUpdate:
		c.handleOrderTradeUpdate(data)
	case WSEventListenKeyExpired:
		c.handleListenKeyExpired(data)
	default:
		log.Printf("[BingX WS] Unknown event type: %s", baseEvent.E)
	}
}

func (c *WSUserDataClient) handleAccountUpdate(data []byte) {
	if c.handler == nil || c.handler.OnAccountUpdate == nil {
		return
	}

	var update WSAccountUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		log.Printf("[BingX WS] Failed to parse account update: %v", err)
		return
	}

	c.handler.OnAccountUpdate(&update)
}

func (c *WSUserDataClient) handleOrderTradeUpdate(data []byte) {
	if c.handler == nil || c.handler.OnOrderTradeUpdate == nil {
		return
	}

	var update WSOrderTradeUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		log.Printf("[BingX WS] Failed to parse order trade update: %v", err)
		return
	}

	c.handler.OnOrderTradeUpdate(&update)
}

func (c *WSUserDataClient) handleListenKeyExpired(data []byte) {
	var expired WSListenKeyExpired
	if err := json.Unmarshal(data, &expired); err != nil {
		log.Printf("[BingX WS] Failed to parse listen key expired: %v", err)
		return
	}

	log.Printf("[BingX WS] Listen key expired: %s", expired.ListenKey)

	if c.handler != nil && c.handler.OnListenKeyExpired != nil {
		c.handler.OnListenKeyExpired(expired.ListenKey)
	}

	// Automatically reconnect with new listen key
	c.handleReconnect()
}

// handleReconnect attempts to reconnect to WebSocket
func (c *WSUserDataClient) handleReconnect() {
	for i := 0; i < c.maxRetries; i++ {
		select {
		case <-c.done:
			return
		case <-c.ctx.Done():
			return
		default:
		}

		log.Printf("[BingX WS] Attempting reconnect %d/%d in %v", i+1, c.maxRetries, c.reconnectDelay)
		time.Sleep(c.reconnectDelay)

		// Delete old listen key if exists
		c.mu.RLock()
		oldKey := c.listenKey
		c.mu.RUnlock()

		if oldKey != "" {
			_ = c.restClient.DeleteListenKey(c.ctx, oldKey)
		}

		c.mu.Lock()
		err := c.connectInternal()
		c.mu.Unlock()

		if err == nil {
			log.Printf("[BingX WS] Reconnected successfully")
			return
		}

		log.Printf("[BingX WS] Reconnect failed: %v", err)
	}

	log.Printf("[BingX WS] Max reconnection attempts reached")
	if c.handler != nil && c.handler.OnError != nil {
		c.handler.OnError(fmt.Errorf("max reconnection attempts reached"))
	}
}

// GetListenKey returns the current listen key
func (c *WSUserDataClient) GetListenKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.listenKey
}

// IsConnected returns whether the client is connected
func (c *WSUserDataClient) IsConnected() bool {
	return c.isConnected.Load()
}

// Close closes the WebSocket connection
func (c *WSUserDataClient) Close() error {
	c.cancel()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.done != nil {
		select {
		case <-c.done:
			// Already closed
		default:
			close(c.done)
		}
	}

	c.isConnected.Store(false)

	// Delete listen key
	if c.listenKey != "" {
		_ = c.restClient.DeleteListenKey(context.Background(), c.listenKey)
		c.listenKey = ""
	}

	if c.conn != nil {
		return c.conn.Close()
	}

	return nil
}
