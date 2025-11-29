// Package bitget provides WebSocket market data client for Bitget exchange.
package bitget

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocket endpoints
const (
	WSPublicURL  = "wss://ws.bitget.com/v2/ws/public"
	WSPrivateURL = "wss://ws.bitget.com/v2/ws/private"
)

// WebSocket operations
const (
	WSOpSubscribe   = "subscribe"
	WSOpUnsubscribe = "unsubscribe"
	WSOpLogin       = "login"
)

// MarketDataHandler handles market data updates
type MarketDataHandler interface {
	OnTicker(ticker *WSTickerData)
	OnOrderBook(instID string, action string, book *WSOrderBookData)
	OnTrade(trade *WSTradeData)
	OnCandle(instID string, channel string, candle *WSCandleData)
	OnError(err error)
	OnConnected()
	OnDisconnected()
}

// MarketDataWSClient handles WebSocket connections for market data
type MarketDataWSClient struct {
	url      string
	conn     *websocket.Conn
	handler  MarketDataHandler
	instType string // USDT-FUTURES, USDC-FUTURES, COIN-FUTURES

	subscriptions map[string]WSSubscribeArg // channel+instID -> arg
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
}

// MarketDataWSConfig holds configuration for market data WebSocket client
type MarketDataWSConfig struct {
	InstType      string // Required: USDT-FUTURES, USDC-FUTURES, COIN-FUTURES
	Handler       MarketDataHandler
	PingInterval  time.Duration
	PongWait      time.Duration
	ReconnectWait time.Duration
	MaxReconnect  int
}

// NewMarketDataWSClient creates a new market data WebSocket client
func NewMarketDataWSClient(cfg MarketDataWSConfig) *MarketDataWSClient {
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

	return &MarketDataWSClient{
		url:           WSPublicURL,
		handler:       cfg.Handler,
		instType:      cfg.InstType,
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
			return err
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
// Bitget uses string "ping" for keep-alive
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
func (c *MarketDataWSClient) handleDisconnect() {
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
func (c *MarketDataWSClient) resubscribe() {
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
func (c *MarketDataWSClient) handleMessage(data []byte) {
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

	// Handle event responses (subscribe/unsubscribe confirmations)
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
	c.processChannelData(arg, resp.Action, resp.Data)
}

// processChannelData processes data based on channel type
func (c *MarketDataWSClient) processChannelData(arg WSSubscribeArg, action string, data json.RawMessage) {
	if c.handler == nil {
		return
	}

	switch arg.Channel {
	case ChannelTicker:
		var tickers []WSTickerData
		if err := json.Unmarshal(data, &tickers); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal ticker: %w", err))
			return
		}
		for i := range tickers {
			c.handler.OnTicker(&tickers[i])
		}

	case ChannelBooks, ChannelBooks1, ChannelBooks5, ChannelBooks15:
		var books []WSOrderBookData
		if err := json.Unmarshal(data, &books); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal order book: %w", err))
			return
		}
		for i := range books {
			c.handler.OnOrderBook(arg.InstID, action, &books[i])
		}

	case ChannelTrade:
		var trades []WSTradeData
		if err := json.Unmarshal(data, &trades); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal trade: %w", err))
			return
		}
		for i := range trades {
			c.handler.OnTrade(&trades[i])
		}

	default:
		// Check for candlestick channels
		if isCandleChannel(arg.Channel) {
			var candles [][]string
			if err := json.Unmarshal(data, &candles); err != nil {
				c.handler.OnError(fmt.Errorf("failed to unmarshal candle: %w", err))
				return
			}
			for _, candleArr := range candles {
				if len(candleArr) >= 7 {
					candle := &WSCandleData{}
					candle.Ts = Timestamp(parseTimestamp(candleArr[0]))
					candle.Open = candleArr[1]
					candle.High = candleArr[2]
					candle.Low = candleArr[3]
					candle.Close = candleArr[4]
					candle.BaseVolume = candleArr[5]
					candle.QuoteVolume = candleArr[6]
					c.handler.OnCandle(arg.InstID, arg.Channel, candle)
				}
			}
		}
	}
}

// isCandleChannel checks if channel is a candlestick channel
func isCandleChannel(channel string) bool {
	switch channel {
	case ChannelCandle1m, ChannelCandle5m, ChannelCandle15m, ChannelCandle30m,
		ChannelCandle1H, ChannelCandle4H, ChannelCandle12H, ChannelCandle1D, ChannelCandle1W:
		return true
	}
	return false
}

// parseTimestamp parses timestamp string to int64
func parseTimestamp(s string) int64 {
	var ts int64
	json.Unmarshal([]byte(s), &ts)
	return ts
}

// sendSubscribe sends a subscription request for a single channel
func (c *MarketDataWSClient) sendSubscribe(arg WSSubscribeArg) error {
	return c.sendSubscribeMultiple([]WSSubscribeArg{arg})
}

// sendSubscribeMultiple sends a subscription request for multiple channels
func (c *MarketDataWSClient) sendSubscribeMultiple(args []WSSubscribeArg) error {
	req := WSSubscribeRequest{
		Op:   WSOpSubscribe,
		Args: args,
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return c.conn.WriteJSON(req)
}

// sendUnsubscribe sends an unsubscription request
func (c *MarketDataWSClient) sendUnsubscribe(arg WSSubscribeArg) error {
	req := WSSubscribeRequest{
		Op:   WSOpUnsubscribe,
		Args: []WSSubscribeArg{arg},
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return c.conn.WriteJSON(req)
}

// subscriptionKey generates a unique key for subscription
func subscriptionKey(channel, instID string) string {
	return channel + ":" + instID
}

// =============================================================================
// Subscription Methods
// =============================================================================

// SubscribeTicker subscribes to ticker updates for an instrument
func (c *MarketDataWSClient) SubscribeTicker(instID string) error {
	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  ChannelTicker,
		InstID:   instID,
	}

	key := subscriptionKey(ChannelTicker, instID)
	c.subMu.Lock()
	c.subscriptions[key] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeTicker unsubscribes from ticker updates
func (c *MarketDataWSClient) UnsubscribeTicker(instID string) error {
	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  ChannelTicker,
		InstID:   instID,
	}

	key := subscriptionKey(ChannelTicker, instID)
	c.subMu.Lock()
	delete(c.subscriptions, key)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribeOrderBook subscribes to order book updates
// channel: "books" (full), "books1" (top 1), "books5" (top 5), "books15" (top 15)
func (c *MarketDataWSClient) SubscribeOrderBook(instID string, channel string) error {
	if channel == "" {
		channel = ChannelBooks5
	}

	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  channel,
		InstID:   instID,
	}

	key := subscriptionKey(channel, instID)
	c.subMu.Lock()
	c.subscriptions[key] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeOrderBook unsubscribes from order book updates
func (c *MarketDataWSClient) UnsubscribeOrderBook(instID string, channel string) error {
	if channel == "" {
		channel = ChannelBooks5
	}

	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  channel,
		InstID:   instID,
	}

	key := subscriptionKey(channel, instID)
	c.subMu.Lock()
	delete(c.subscriptions, key)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribeTrades subscribes to public trade updates
func (c *MarketDataWSClient) SubscribeTrades(instID string) error {
	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  ChannelTrade,
		InstID:   instID,
	}

	key := subscriptionKey(ChannelTrade, instID)
	c.subMu.Lock()
	c.subscriptions[key] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeTrades unsubscribes from public trade updates
func (c *MarketDataWSClient) UnsubscribeTrades(instID string) error {
	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  ChannelTrade,
		InstID:   instID,
	}

	key := subscriptionKey(ChannelTrade, instID)
	c.subMu.Lock()
	delete(c.subscriptions, key)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribeCandles subscribes to candlestick updates
// channel: "candle1m", "candle5m", "candle15m", "candle30m", "candle1H", etc.
func (c *MarketDataWSClient) SubscribeCandles(instID string, channel string) error {
	if channel == "" {
		channel = ChannelCandle1m
	}

	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  channel,
		InstID:   instID,
	}

	key := subscriptionKey(channel, instID)
	c.subMu.Lock()
	c.subscriptions[key] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeCandles unsubscribes from candlestick updates
func (c *MarketDataWSClient) UnsubscribeCandles(instID string, channel string) error {
	if channel == "" {
		channel = ChannelCandle1m
	}

	arg := WSSubscribeArg{
		InstType: c.instType,
		Channel:  channel,
		InstID:   instID,
	}

	key := subscriptionKey(channel, instID)
	c.subMu.Lock()
	delete(c.subscriptions, key)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribeMultiple subscribes to multiple channels at once
func (c *MarketDataWSClient) SubscribeMultiple(args []WSSubscribeArg) error {
	c.subMu.Lock()
	for _, arg := range args {
		key := subscriptionKey(arg.Channel, arg.InstID)
		c.subscriptions[key] = arg
	}
	c.subMu.Unlock()

	return c.sendSubscribeMultiple(args)
}

// GetInstType returns the configured instrument type
func (c *MarketDataWSClient) GetInstType() string {
	return c.instType
}

// SetInstType sets the instrument type (must be done before connecting)
func (c *MarketDataWSClient) SetInstType(instType string) {
	c.instType = instType
}

// IsConnected returns whether the client is connected
func (c *MarketDataWSClient) IsConnected() bool {
	return c.conn != nil
}

// GetSubscriptionCount returns the number of active subscriptions
func (c *MarketDataWSClient) GetSubscriptionCount() int {
	c.subMu.RLock()
	defer c.subMu.RUnlock()
	return len(c.subscriptions)
}
