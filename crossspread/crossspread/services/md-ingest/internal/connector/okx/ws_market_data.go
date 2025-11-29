// Package okx provides WebSocket market data client for OKX exchange.
package okx

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
	WSPublicURL   = "wss://ws.okx.com:8443/ws/v5/public"
	WSBusinessURL = "wss://ws.okx.com:8443/ws/v5/business"

	// Demo endpoints
	WSPublicDemoURL   = "wss://wspap.okx.com:8443/ws/v5/public"
	WSBusinessDemoURL = "wss://wspap.okx.com:8443/ws/v5/business"
)

// Channel names
const (
	ChannelTickers      = "tickers"
	ChannelBooks        = "books"          // 400 levels, 100ms
	ChannelBooks5       = "books5"         // 5 levels, 200ms
	ChannelBooksBBO     = "bbo-tbt"        // Best bid/offer, tick-by-tick
	ChannelBooks50TBT   = "books50-l2-tbt" // 50 levels, tick-by-tick
	ChannelBooks400TBT  = "books-l2-tbt"   // 400 levels, tick-by-tick
	ChannelTrades       = "trades"
	ChannelTradesAll    = "trades-all" // All trades (includes block trades)
	ChannelFundingRate  = "funding-rate"
	ChannelIndexTickers = "index-tickers"
	ChannelMarkPrice    = "mark-price"
	ChannelOpenInterest = "open-interest"
)

// Candlestick channel prefixes (append to get full channel name e.g., "candle1m")
const (
	ChannelCandle1s  = "candle1s"
	ChannelCandle1m  = "candle1m"
	ChannelCandle3m  = "candle3m"
	ChannelCandle5m  = "candle5m"
	ChannelCandle15m = "candle15m"
	ChannelCandle30m = "candle30m"
	ChannelCandle1H  = "candle1H"
	ChannelCandle2H  = "candle2H"
	ChannelCandle4H  = "candle4H"
	ChannelCandle6H  = "candle6H"
	ChannelCandle12H = "candle12H"
	ChannelCandle1D  = "candle1D"
	ChannelCandle1W  = "candle1W"
	ChannelCandle1M  = "candle1M"
)

// MarketDataHandler handles market data updates
type MarketDataHandler interface {
	OnTicker(ticker *WSTickerData)
	OnOrderBook(instID string, action string, book *WSOrderBookData)
	OnTrade(trade *WSTradeData)
	OnCandle(instID string, channel string, candle []string)
	OnFundingRate(data *WSFundingRateData)
	OnIndexTicker(ticker *IndexTicker)
	OnError(err error)
	OnConnected()
	OnDisconnected()
}

// MarketDataWSClient handles WebSocket connections for market data
type MarketDataWSClient struct {
	url      string
	conn     *websocket.Conn
	handler  MarketDataHandler
	demoMode bool

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
	DemoMode      bool
	Handler       MarketDataHandler
	PingInterval  time.Duration
	PongWait      time.Duration
	ReconnectWait time.Duration
	MaxReconnect  int
}

// NewMarketDataWSClient creates a new market data WebSocket client
func NewMarketDataWSClient(cfg MarketDataWSConfig) *MarketDataWSClient {
	url := WSPublicURL
	if cfg.DemoMode {
		url = WSPublicDemoURL
	}

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

	ctx, cancel := context.WithCancel(context.Background())

	return &MarketDataWSClient{
		url:           url,
		handler:       cfg.Handler,
		demoMode:      cfg.DemoMode,
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

	for _, arg := range c.subscriptions {
		if err := c.sendSubscribe(arg); err != nil {
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

	// Parse channel argument
	var arg WSChannelArg
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
func (c *MarketDataWSClient) processChannelData(arg WSChannelArg, action string, data json.RawMessage) {
	if c.handler == nil {
		return
	}

	switch arg.Channel {
	case ChannelTickers:
		var tickers []WSTickerData
		if err := json.Unmarshal(data, &tickers); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal ticker: %w", err))
			return
		}
		for _, t := range tickers {
			c.handler.OnTicker(&t)
		}

	case ChannelBooks, ChannelBooks5, ChannelBooksBBO, ChannelBooks50TBT, ChannelBooks400TBT:
		var books []WSOrderBookData
		if err := json.Unmarshal(data, &books); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal order book: %w", err))
			return
		}
		for _, b := range books {
			c.handler.OnOrderBook(arg.InstID, action, &b)
		}

	case ChannelTrades, ChannelTradesAll:
		var trades []WSTradeData
		if err := json.Unmarshal(data, &trades); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal trade: %w", err))
			return
		}
		for _, t := range trades {
			c.handler.OnTrade(&t)
		}

	case ChannelFundingRate:
		var rates []WSFundingRateData
		if err := json.Unmarshal(data, &rates); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal funding rate: %w", err))
			return
		}
		for _, r := range rates {
			c.handler.OnFundingRate(&r)
		}

	case ChannelIndexTickers:
		var tickers []IndexTicker
		if err := json.Unmarshal(data, &tickers); err != nil {
			c.handler.OnError(fmt.Errorf("failed to unmarshal index ticker: %w", err))
			return
		}
		for _, t := range tickers {
			c.handler.OnIndexTicker(&t)
		}

	default:
		// Check for candlestick channels
		if len(arg.Channel) >= 6 && arg.Channel[:6] == "candle" {
			var candles [][]string
			if err := json.Unmarshal(data, &candles); err != nil {
				c.handler.OnError(fmt.Errorf("failed to unmarshal candle: %w", err))
				return
			}
			for _, candle := range candles {
				c.handler.OnCandle(arg.InstID, arg.Channel, candle)
			}
		}
	}
}

// sendSubscribe sends a subscription request
func (c *MarketDataWSClient) sendSubscribe(arg WSSubscribeArg) error {
	req := WSRequest{
		Op:   "subscribe",
		Args: []interface{}{arg},
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return c.conn.WriteJSON(req)
}

// sendUnsubscribe sends an unsubscription request
func (c *MarketDataWSClient) sendUnsubscribe(arg WSSubscribeArg) error {
	req := WSRequest{
		Op:   "unsubscribe",
		Args: []interface{}{arg},
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
		Channel: ChannelTickers,
		InstID:  instID,
	}

	key := subscriptionKey(ChannelTickers, instID)
	c.subMu.Lock()
	c.subscriptions[key] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeTicker unsubscribes from ticker updates
func (c *MarketDataWSClient) UnsubscribeTicker(instID string) error {
	arg := WSSubscribeArg{
		Channel: ChannelTickers,
		InstID:  instID,
	}

	key := subscriptionKey(ChannelTickers, instID)
	c.subMu.Lock()
	delete(c.subscriptions, key)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribeOrderBook subscribes to order book updates
// channel: "books", "books5", "bbo-tbt", "books50-l2-tbt", "books-l2-tbt"
func (c *MarketDataWSClient) SubscribeOrderBook(instID string, channel string) error {
	if channel == "" {
		channel = ChannelBooks
	}

	arg := WSSubscribeArg{
		Channel: channel,
		InstID:  instID,
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
		channel = ChannelBooks
	}

	arg := WSSubscribeArg{
		Channel: channel,
		InstID:  instID,
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
		Channel: ChannelTrades,
		InstID:  instID,
	}

	key := subscriptionKey(ChannelTrades, instID)
	c.subMu.Lock()
	c.subscriptions[key] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeTrades unsubscribes from public trade updates
func (c *MarketDataWSClient) UnsubscribeTrades(instID string) error {
	arg := WSSubscribeArg{
		Channel: ChannelTrades,
		InstID:  instID,
	}

	key := subscriptionKey(ChannelTrades, instID)
	c.subMu.Lock()
	delete(c.subscriptions, key)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribeCandles subscribes to candlestick updates
// channel: "candle1m", "candle5m", "candle15m", "candle1H", etc.
func (c *MarketDataWSClient) SubscribeCandles(instID string, channel string) error {
	if channel == "" {
		channel = ChannelCandle1m
	}

	arg := WSSubscribeArg{
		Channel: channel,
		InstID:  instID,
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
		Channel: channel,
		InstID:  instID,
	}

	key := subscriptionKey(channel, instID)
	c.subMu.Lock()
	delete(c.subscriptions, key)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribeFundingRate subscribes to funding rate updates
func (c *MarketDataWSClient) SubscribeFundingRate(instID string) error {
	arg := WSSubscribeArg{
		Channel: ChannelFundingRate,
		InstID:  instID,
	}

	key := subscriptionKey(ChannelFundingRate, instID)
	c.subMu.Lock()
	c.subscriptions[key] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeFundingRate unsubscribes from funding rate updates
func (c *MarketDataWSClient) UnsubscribeFundingRate(instID string) error {
	arg := WSSubscribeArg{
		Channel: ChannelFundingRate,
		InstID:  instID,
	}

	key := subscriptionKey(ChannelFundingRate, instID)
	c.subMu.Lock()
	delete(c.subscriptions, key)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// SubscribeIndexTicker subscribes to index ticker updates
func (c *MarketDataWSClient) SubscribeIndexTicker(instID string) error {
	arg := WSSubscribeArg{
		Channel: ChannelIndexTickers,
		InstID:  instID,
	}

	key := subscriptionKey(ChannelIndexTickers, instID)
	c.subMu.Lock()
	c.subscriptions[key] = arg
	c.subMu.Unlock()

	return c.sendSubscribe(arg)
}

// UnsubscribeIndexTicker unsubscribes from index ticker updates
func (c *MarketDataWSClient) UnsubscribeIndexTicker(instID string) error {
	arg := WSSubscribeArg{
		Channel: ChannelIndexTickers,
		InstID:  instID,
	}

	key := subscriptionKey(ChannelIndexTickers, instID)
	c.subMu.Lock()
	delete(c.subscriptions, key)
	c.subMu.Unlock()

	return c.sendUnsubscribe(arg)
}

// =============================================================================
// Batch Subscription Methods
// =============================================================================

// SubscribeMultipleTickers subscribes to tickers for multiple instruments
func (c *MarketDataWSClient) SubscribeMultipleTickers(instIDs []string) error {
	args := make([]interface{}, len(instIDs))
	for i, instID := range instIDs {
		arg := WSSubscribeArg{
			Channel: ChannelTickers,
			InstID:  instID,
		}
		args[i] = arg

		key := subscriptionKey(ChannelTickers, instID)
		c.subMu.Lock()
		c.subscriptions[key] = arg
		c.subMu.Unlock()
	}

	req := WSRequest{
		Op:   "subscribe",
		Args: args,
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return c.conn.WriteJSON(req)
}

// SubscribeMultipleOrderBooks subscribes to order books for multiple instruments
func (c *MarketDataWSClient) SubscribeMultipleOrderBooks(instIDs []string, channel string) error {
	if channel == "" {
		channel = ChannelBooks
	}

	args := make([]interface{}, len(instIDs))
	for i, instID := range instIDs {
		arg := WSSubscribeArg{
			Channel: channel,
			InstID:  instID,
		}
		args[i] = arg

		key := subscriptionKey(channel, instID)
		c.subMu.Lock()
		c.subscriptions[key] = arg
		c.subMu.Unlock()
	}

	req := WSRequest{
		Op:   "subscribe",
		Args: args,
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return c.conn.WriteJSON(req)
}

// =============================================================================
// Order Book Management Helper
// =============================================================================

// OrderBookManager maintains a local order book from WebSocket updates
type OrderBookManager struct {
	books map[string]*LocalOrderBook
	mu    sync.RWMutex
}

// LocalOrderBook represents a locally maintained order book
type LocalOrderBook struct {
	InstID   string
	Asks     map[string]OrderBookLevel // price -> level
	Bids     map[string]OrderBookLevel // price -> level
	SeqID    int64
	Checksum int64
	Ts       Timestamp
	mu       sync.RWMutex
}

// NewOrderBookManager creates a new order book manager
func NewOrderBookManager() *OrderBookManager {
	return &OrderBookManager{
		books: make(map[string]*LocalOrderBook),
	}
}

// ProcessUpdate processes an order book update
func (m *OrderBookManager) ProcessUpdate(instID string, action string, data *WSOrderBookData) *LocalOrderBook {
	m.mu.Lock()
	defer m.mu.Unlock()

	if action == "snapshot" {
		// Create new order book from snapshot
		book := &LocalOrderBook{
			InstID:   instID,
			Asks:     make(map[string]OrderBookLevel),
			Bids:     make(map[string]OrderBookLevel),
			SeqID:    data.SeqID,
			Checksum: data.Checksum,
			Ts:       data.Ts,
		}

		for _, ask := range data.Asks {
			book.Asks[ask.Price] = ask
		}
		for _, bid := range data.Bids {
			book.Bids[bid.Price] = bid
		}

		m.books[instID] = book
		return book
	}

	// Process incremental update
	book, exists := m.books[instID]
	if !exists {
		return nil // Need snapshot first
	}

	book.mu.Lock()
	defer book.mu.Unlock()

	// Verify sequence
	if data.PrevSeqID != 0 && data.PrevSeqID != book.SeqID {
		// Sequence mismatch - need resync
		delete(m.books, instID)
		return nil
	}

	// Apply updates
	for _, ask := range data.Asks {
		if ask.Size == "0" {
			delete(book.Asks, ask.Price)
		} else {
			book.Asks[ask.Price] = ask
		}
	}

	for _, bid := range data.Bids {
		if bid.Size == "0" {
			delete(book.Bids, bid.Price)
		} else {
			book.Bids[bid.Price] = bid
		}
	}

	book.SeqID = data.SeqID
	book.Checksum = data.Checksum
	book.Ts = data.Ts

	return book
}

// GetOrderBook returns the current order book for an instrument
func (m *OrderBookManager) GetOrderBook(instID string) *LocalOrderBook {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.books[instID]
}

// GetSortedAsks returns asks sorted by price (ascending)
func (b *LocalOrderBook) GetSortedAsks(limit int) []OrderBookLevel {
	b.mu.RLock()
	defer b.mu.RUnlock()

	asks := make([]OrderBookLevel, 0, len(b.Asks))
	for _, level := range b.Asks {
		asks = append(asks, level)
	}

	// Sort by price ascending
	for i := 0; i < len(asks)-1; i++ {
		for j := i + 1; j < len(asks); j++ {
			if asks[i].Price > asks[j].Price {
				asks[i], asks[j] = asks[j], asks[i]
			}
		}
	}

	if limit > 0 && limit < len(asks) {
		return asks[:limit]
	}
	return asks
}

// GetSortedBids returns bids sorted by price (descending)
func (b *LocalOrderBook) GetSortedBids(limit int) []OrderBookLevel {
	b.mu.RLock()
	defer b.mu.RUnlock()

	bids := make([]OrderBookLevel, 0, len(b.Bids))
	for _, level := range b.Bids {
		bids = append(bids, level)
	}

	// Sort by price descending
	for i := 0; i < len(bids)-1; i++ {
		for j := i + 1; j < len(bids); j++ {
			if bids[i].Price < bids[j].Price {
				bids[i], bids[j] = bids[j], bids[i]
			}
		}
	}

	if limit > 0 && limit < len(bids) {
		return bids[:limit]
	}
	return bids
}

// GetBestAsk returns the best ask price and size
func (b *LocalOrderBook) GetBestAsk() (string, string) {
	asks := b.GetSortedAsks(1)
	if len(asks) == 0 {
		return "", ""
	}
	return asks[0].Price, asks[0].Size
}

// GetBestBid returns the best bid price and size
func (b *LocalOrderBook) GetBestBid() (string, string) {
	bids := b.GetSortedBids(1)
	if len(bids) == 0 {
		return "", ""
	}
	return bids[0].Price, bids[0].Size
}

// GetSpread returns the spread between best bid and ask
func (b *LocalOrderBook) GetSpread() (bestBid, bestAsk, spread string) {
	bestBid, _ = b.GetBestBid()
	bestAsk, _ = b.GetBestAsk()
	// Note: Caller should parse and calculate spread
	return bestBid, bestAsk, ""
}
