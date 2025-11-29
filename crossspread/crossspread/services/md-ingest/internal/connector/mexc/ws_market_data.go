// Package mexc provides WebSocket market data client for MEXC exchange.
package mexc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocket URLs
const (
	WSPublicURL = "wss://contract.mexc.com/edge"
)

// WebSocket channels
const (
	WSChannelTicker    = "push.ticker"
	WSChannelDepth     = "push.depth"
	WSChannelDepthFull = "push.depth.full"
	WSChannelDeal      = "push.deal"
	WSChannelKline     = "push.kline"
	WSChannelFunding   = "push.funding"
)

// MarketDataHandler handles market data updates from WebSocket
type MarketDataHandler interface {
	OnTicker(ticker *WSTickerData)
	OnOrderBook(symbol string, book *WSDepthData, isFull bool)
	OnTrade(symbol string, trade *WSTradeData)
	OnKline(symbol string, interval string, kline *WSKlineData)
	OnError(err error)
	OnConnected()
	OnDisconnected()
}

// MarketDataWSClient handles WebSocket connections for public market data
type MarketDataWSClient struct {
	url     string
	conn    *websocket.Conn
	handler MarketDataHandler

	subscriptions map[string]bool // channel+symbol -> bool
	subMu         sync.RWMutex

	writeMu sync.Mutex
	done    chan struct{}
	wg      sync.WaitGroup

	reconnect      bool
	reconnectWait  time.Duration
	maxReconnect   int
	reconnectCount int

	pingInterval time.Duration

	ctx    context.Context
	cancel context.CancelFunc
}

// MarketDataWSConfig holds configuration for market data WebSocket client
type MarketDataWSConfig struct {
	Handler       MarketDataHandler
	PingInterval  time.Duration
	ReconnectWait time.Duration
	MaxReconnect  int
}

// NewMarketDataWSClient creates a new market data WebSocket client
func NewMarketDataWSClient(cfg MarketDataWSConfig) *MarketDataWSClient {
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

	return &MarketDataWSClient{
		url:           WSPublicURL,
		handler:       cfg.Handler,
		subscriptions: make(map[string]bool),
		done:          make(chan struct{}),
		reconnect:     true,
		reconnectWait: cfg.ReconnectWait,
		maxReconnect:  cfg.MaxReconnect,
		pingInterval:  cfg.PingInterval,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Connect establishes WebSocket connection
func (c *MarketDataWSClient) Connect() error {
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

	// Start goroutines
	c.wg.Add(2)
	go c.readLoop()
	go c.pingLoop()

	if c.handler != nil {
		c.handler.OnConnected()
	}

	return nil
}

// Close closes the WebSocket connection
func (c *MarketDataWSClient) Close() error {
	c.reconnect = false
	c.cancel()

	if c.conn != nil {
		close(c.done)
		c.writeMu.Lock()
		err := c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.writeMu.Unlock()
		if err != nil {
			// Ignore write errors on close
		}
		c.conn.Close()
	}

	c.wg.Wait()
	return nil
}

// readLoop reads messages from WebSocket
func (c *MarketDataWSClient) readLoop() {
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
// MEXC uses {"method": "ping"} for keep-alive
func (c *MarketDataWSClient) pingLoop() {
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
func (c *MarketDataWSClient) handleDisconnect() {
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
			// Re-subscribe to all channels
			c.resubscribeAll()
		}
	}
}

// resubscribeAll re-subscribes to all previously subscribed channels
func (c *MarketDataWSClient) resubscribeAll() {
	c.subMu.RLock()
	subs := make(map[string]bool)
	for k, v := range c.subscriptions {
		subs[k] = v
	}
	c.subMu.RUnlock()

	for key := range subs {
		// Parse key back to channel and symbol
		// Format: "method:symbol"
		parts := splitSubscriptionKey(key)
		if len(parts) < 2 {
			continue
		}
		method := parts[0]
		symbol := parts[1]

		req := WSSubscribeRequest{
			Method: method,
			Param: map[string]interface{}{
				"symbol": symbol,
			},
		}

		c.writeMu.Lock()
		_ = c.conn.WriteJSON(req) // Ignore errors on resubscribe
		c.writeMu.Unlock()
	}
}

// splitSubscriptionKey splits a subscription key into parts
func splitSubscriptionKey(key string) []string {
	var parts []string
	current := ""
	for _, ch := range key {
		if ch == ':' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// handleMessage processes incoming WebSocket messages
func (c *MarketDataWSClient) handleMessage(data []byte) {
	// Check for pong response
	var pong struct {
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(data, &pong); err == nil && pong.Channel == "pong" {
		return // Pong received, ignore
	}

	// Parse generic message
	var msg WSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		if c.handler != nil {
			c.handler.OnError(fmt.Errorf("failed to parse message: %w", err))
		}
		return
	}

	// Route by channel
	switch msg.Channel {
	case WSChannelTicker:
		c.handleTicker(msg.Symbol, msg.Data)
	case WSChannelDepth:
		c.handleDepth(msg.Symbol, msg.Data, false)
	case WSChannelDepthFull:
		c.handleDepth(msg.Symbol, msg.Data, true)
	case WSChannelDeal:
		c.handleDeal(msg.Symbol, msg.Data)
	case WSChannelKline:
		c.handleKline(msg.Symbol, msg.Data)
	}
}

// handleTicker processes ticker updates
func (c *MarketDataWSClient) handleTicker(symbol string, data json.RawMessage) {
	if c.handler == nil {
		return
	}

	var ticker WSTickerData
	if err := json.Unmarshal(data, &ticker); err != nil {
		c.handler.OnError(fmt.Errorf("failed to parse ticker: %w", err))
		return
	}

	if ticker.Symbol == "" {
		ticker.Symbol = symbol
	}

	c.handler.OnTicker(&ticker)
}

// handleDepth processes orderbook updates
func (c *MarketDataWSClient) handleDepth(symbol string, data json.RawMessage, isFull bool) {
	if c.handler == nil {
		return
	}

	var depth WSDepthData
	if err := json.Unmarshal(data, &depth); err != nil {
		c.handler.OnError(fmt.Errorf("failed to parse depth: %w", err))
		return
	}

	c.handler.OnOrderBook(symbol, &depth, isFull)
}

// handleDeal processes trade updates
func (c *MarketDataWSClient) handleDeal(symbol string, data json.RawMessage) {
	if c.handler == nil {
		return
	}

	var trade WSTradeData
	if err := json.Unmarshal(data, &trade); err != nil {
		c.handler.OnError(fmt.Errorf("failed to parse trade: %w", err))
		return
	}

	c.handler.OnTrade(symbol, &trade)
}

// handleKline processes kline updates
func (c *MarketDataWSClient) handleKline(symbol string, data json.RawMessage) {
	if c.handler == nil {
		return
	}

	var kline WSKlineData
	if err := json.Unmarshal(data, &kline); err != nil {
		c.handler.OnError(fmt.Errorf("failed to parse kline: %w", err))
		return
	}

	c.handler.OnKline(symbol, "", &kline)
}

// =============================================================================
// Subscription Methods
// =============================================================================

// SubscribeTicker subscribes to ticker updates for a symbol
func (c *MarketDataWSClient) SubscribeTicker(symbol string) error {
	return c.subscribe("sub.ticker", symbol, nil)
}

// UnsubscribeTicker unsubscribes from ticker updates
func (c *MarketDataWSClient) UnsubscribeTicker(symbol string) error {
	return c.unsubscribe("unsub.ticker", symbol)
}

// SubscribeDepth subscribes to orderbook depth updates for a symbol
func (c *MarketDataWSClient) SubscribeDepth(symbol string) error {
	return c.subscribe("sub.depth", symbol, nil)
}

// UnsubscribeDepth unsubscribes from orderbook depth updates
func (c *MarketDataWSClient) UnsubscribeDepth(symbol string) error {
	return c.unsubscribe("unsub.depth", symbol)
}

// SubscribeDepthFull subscribes to full orderbook snapshots
func (c *MarketDataWSClient) SubscribeDepthFull(symbol string, limit int) error {
	if limit == 0 {
		limit = 20
	}
	return c.subscribe("sub.depth.full", symbol, map[string]interface{}{
		"limit": limit,
	})
}

// UnsubscribeDepthFull unsubscribes from full orderbook snapshots
func (c *MarketDataWSClient) UnsubscribeDepthFull(symbol string) error {
	return c.unsubscribe("unsub.depth.full", symbol)
}

// SubscribeDeal subscribes to trade updates for a symbol
func (c *MarketDataWSClient) SubscribeDeal(symbol string) error {
	return c.subscribe("sub.deal", symbol, nil)
}

// UnsubscribeDeal unsubscribes from trade updates
func (c *MarketDataWSClient) UnsubscribeDeal(symbol string) error {
	return c.unsubscribe("unsub.deal", symbol)
}

// SubscribeKline subscribes to kline updates for a symbol
func (c *MarketDataWSClient) SubscribeKline(symbol string, interval KlineInterval) error {
	return c.subscribe("sub.kline", symbol, map[string]interface{}{
		"interval": string(interval),
	})
}

// UnsubscribeKline unsubscribes from kline updates
func (c *MarketDataWSClient) UnsubscribeKline(symbol string, interval KlineInterval) error {
	return c.unsubscribe("unsub.kline", symbol)
}

// subscribe sends a subscription request
func (c *MarketDataWSClient) subscribe(method, symbol string, extraParams map[string]interface{}) error {
	param := map[string]interface{}{
		"symbol": symbol,
	}
	for k, v := range extraParams {
		param[k] = v
	}

	req := WSSubscribeRequest{
		Method: method,
		Param:  param,
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	// Track subscription
	c.subMu.Lock()
	c.subscriptions[method+":"+symbol] = true
	c.subMu.Unlock()

	return nil
}

// unsubscribe sends an unsubscription request
func (c *MarketDataWSClient) unsubscribe(method, symbol string) error {
	req := WSSubscribeRequest{
		Method: method,
		Param: map[string]interface{}{
			"symbol": symbol,
		},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(req)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to unsubscribe: %w", err)
	}

	// Remove subscription tracking
	c.subMu.Lock()
	delete(c.subscriptions, method+":"+symbol)
	c.subMu.Unlock()

	return nil
}

// SubscribeMultiple subscribes to multiple symbols at once
func (c *MarketDataWSClient) SubscribeMultiple(symbols []string, depth, ticker, deals bool) error {
	for _, symbol := range symbols {
		if depth {
			if err := c.SubscribeDepth(symbol); err != nil {
				return err
			}
		}
		if ticker {
			if err := c.SubscribeTicker(symbol); err != nil {
				return err
			}
		}
		if deals {
			if err := c.SubscribeDeal(symbol); err != nil {
				return err
			}
		}
	}
	return nil
}

// IsConnected returns true if WebSocket is connected
func (c *MarketDataWSClient) IsConnected() bool {
	return c.conn != nil
}
