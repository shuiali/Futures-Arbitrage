package bybit

import (
	"context"
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
	// WebSocket URLs for public market data
	WSURLLinearMainnet  = "wss://stream.bybit.com/v5/public/linear"
	WSURLLinearTestnet  = "wss://stream-testnet.bybit.com/v5/public/linear"
	WSURLInverseMainnet = "wss://stream.bybit.com/v5/public/inverse"
	WSURLInverseTestnet = "wss://stream-testnet.bybit.com/v5/public/inverse"
	WSURLSpotMainnet    = "wss://stream.bybit.com/v5/public/spot"
	WSURLSpotTestnet    = "wss://stream-testnet.bybit.com/v5/public/spot"

	// WebSocket configuration
	WSPingInterval   = 20 * time.Second
	WSReadTimeout    = 30 * time.Second
	WSWriteTimeout   = 10 * time.Second
	WSReconnectDelay = 5 * time.Second
	WSMaxReconnects  = 10
)

// OrderbookDepth represents available orderbook depth levels
type OrderbookDepth int

const (
	Depth1   OrderbookDepth = 1   // 10ms update
	Depth50  OrderbookDepth = 50  // 20ms update
	Depth200 OrderbookDepth = 200 // 100ms update
	Depth500 OrderbookDepth = 500 // 100ms update
)

// MarketDataCallback types
type OrderbookCallback func(symbol string, data *WSOrderbookData, isSnapshot bool, timestamp int64)
type TickerCallback func(symbol string, data *WSTickerData)
type TradeCallback func(symbol string, trades []WSTradeData)
type ErrorCallback func(err error)
type ReconnectCallback func()

// MarketDataWS handles public WebSocket connections for market data
type MarketDataWS struct {
	url           string
	conn          *websocket.Conn
	mu            sync.RWMutex
	connected     bool
	subscriptions map[string]bool
	orderbooks    map[string]*LocalOrderbook

	// Callbacks
	onOrderbook OrderbookCallback
	onTicker    TickerCallback
	onTrade     TradeCallback
	onError     ErrorCallback
	onReconnect ReconnectCallback

	// Control
	done           chan struct{}
	reconnectCount int
	ctx            context.Context
	cancel         context.CancelFunc
}

// LocalOrderbook maintains local orderbook state for delta updates
type LocalOrderbook struct {
	Symbol    string
	Bids      map[string]float64 // price -> size
	Asks      map[string]float64 // price -> size
	UpdateID  int64
	Seq       int64
	Timestamp int64
}

// MarketDataWSConfig holds configuration for the WebSocket client
type MarketDataWSConfig struct {
	URL            string
	Category       string // linear, inverse, spot
	UseTestnet     bool
	PingInterval   time.Duration
	ReconnectDelay time.Duration
}

// NewMarketDataWS creates a new market data WebSocket client
func NewMarketDataWS(config MarketDataWSConfig) *MarketDataWS {
	url := config.URL
	if url == "" {
		// Default to linear mainnet
		switch config.Category {
		case "inverse":
			if config.UseTestnet {
				url = WSURLInverseTestnet
			} else {
				url = WSURLInverseMainnet
			}
		case "spot":
			if config.UseTestnet {
				url = WSURLSpotTestnet
			} else {
				url = WSURLSpotMainnet
			}
		default: // linear
			if config.UseTestnet {
				url = WSURLLinearTestnet
			} else {
				url = WSURLLinearMainnet
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &MarketDataWS{
		url:           url,
		subscriptions: make(map[string]bool),
		orderbooks:    make(map[string]*LocalOrderbook),
		done:          make(chan struct{}),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// SetOrderbookCallback sets the callback for orderbook updates
func (ws *MarketDataWS) SetOrderbookCallback(cb OrderbookCallback) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.onOrderbook = cb
}

// SetTickerCallback sets the callback for ticker updates
func (ws *MarketDataWS) SetTickerCallback(cb TickerCallback) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.onTicker = cb
}

// SetTradeCallback sets the callback for trade updates
func (ws *MarketDataWS) SetTradeCallback(cb TradeCallback) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.onTrade = cb
}

// SetErrorCallback sets the callback for errors
func (ws *MarketDataWS) SetErrorCallback(cb ErrorCallback) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.onError = cb
}

// SetReconnectCallback sets the callback for reconnection events
func (ws *MarketDataWS) SetReconnectCallback(cb ReconnectCallback) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.onReconnect = cb
}

// Connect establishes WebSocket connection
func (ws *MarketDataWS) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, ws.url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Bybit WebSocket: %w", err)
	}

	ws.mu.Lock()
	ws.conn = conn
	ws.connected = true
	ws.reconnectCount = 0
	ws.mu.Unlock()

	log.Info().Str("url", ws.url).Msg("Connected to Bybit market data WebSocket")

	// Start message reader
	go ws.readMessages()

	// Start ping loop
	go ws.pingLoop()

	return nil
}

// Disconnect closes the WebSocket connection
func (ws *MarketDataWS) Disconnect() error {
	ws.cancel()
	close(ws.done)

	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.connected = false
	if ws.conn != nil {
		return ws.conn.Close()
	}
	return nil
}

// IsConnected returns connection status
func (ws *MarketDataWS) IsConnected() bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return ws.connected
}

// SubscribeOrderbook subscribes to orderbook updates for symbols
func (ws *MarketDataWS) SubscribeOrderbook(symbols []string, depth OrderbookDepth) error {
	args := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		topic := fmt.Sprintf("orderbook.%d.%s", depth, symbol)
		args = append(args, topic)

		ws.mu.Lock()
		ws.subscriptions[topic] = true
		// Initialize local orderbook
		ws.orderbooks[symbol] = &LocalOrderbook{
			Symbol: symbol,
			Bids:   make(map[string]float64),
			Asks:   make(map[string]float64),
		}
		ws.mu.Unlock()
	}

	return ws.subscribe(args)
}

// SubscribeTicker subscribes to ticker updates for symbols
func (ws *MarketDataWS) SubscribeTicker(symbols []string) error {
	args := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		topic := fmt.Sprintf("tickers.%s", symbol)
		args = append(args, topic)

		ws.mu.Lock()
		ws.subscriptions[topic] = true
		ws.mu.Unlock()
	}

	return ws.subscribe(args)
}

// SubscribeTrades subscribes to public trade stream for symbols
func (ws *MarketDataWS) SubscribeTrades(symbols []string) error {
	args := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		topic := fmt.Sprintf("publicTrade.%s", symbol)
		args = append(args, topic)

		ws.mu.Lock()
		ws.subscriptions[topic] = true
		ws.mu.Unlock()
	}

	return ws.subscribe(args)
}

// Unsubscribe removes subscriptions
func (ws *MarketDataWS) Unsubscribe(topics []string) error {
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
func (ws *MarketDataWS) subscribe(args []string) error {
	msg := WSOperation{
		Op:   "subscribe",
		Args: args,
	}

	log.Debug().Strs("topics", args).Msg("Subscribing to topics")
	return ws.sendJSON(msg)
}

// sendJSON sends a JSON message
func (ws *MarketDataWS) sendJSON(v interface{}) error {
	ws.mu.RLock()
	conn := ws.conn
	ws.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	return conn.WriteJSON(v)
}

// readMessages reads and processes WebSocket messages
func (ws *MarketDataWS) readMessages() {
	defer func() {
		ws.mu.Lock()
		ws.connected = false
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
func (ws *MarketDataWS) processMessage(data []byte) {
	// Check for pong response
	var pongResp struct {
		Success bool   `json:"success"`
		Op      string `json:"op"`
	}
	if err := json.Unmarshal(data, &pongResp); err == nil && pongResp.Op == "pong" {
		return
	}

	// Check for subscription response
	var subResp struct {
		Success bool   `json:"success"`
		Op      string `json:"op"`
		RetMsg  string `json:"ret_msg"`
		ConnId  string `json:"conn_id"`
	}
	if err := json.Unmarshal(data, &subResp); err == nil && subResp.Op == "subscribe" {
		if !subResp.Success {
			ws.emitError(fmt.Errorf("subscription failed: %s", subResp.RetMsg))
		}
		return
	}

	// Parse message with topic
	var msg struct {
		Topic string          `json:"topic"`
		Type  string          `json:"type"`
		Ts    int64           `json:"ts"`
		Data  json.RawMessage `json:"data"`
		Cts   int64           `json:"cts"` // Matching engine timestamp
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		log.Debug().Err(err).Str("data", string(data)).Msg("Failed to parse WebSocket message")
		return
	}

	if msg.Topic == "" {
		return
	}

	// Route message based on topic
	switch {
	case strings.HasPrefix(msg.Topic, "orderbook."):
		ws.handleOrderbook(msg.Topic, msg.Type, msg.Data, msg.Ts, msg.Cts)
	case strings.HasPrefix(msg.Topic, "tickers."):
		ws.handleTicker(msg.Topic, msg.Data)
	case strings.HasPrefix(msg.Topic, "publicTrade."):
		ws.handleTrade(msg.Topic, msg.Data)
	}
}

// handleOrderbook processes orderbook messages
func (ws *MarketDataWS) handleOrderbook(topic, msgType string, data json.RawMessage, ts, _ int64) {
	// Extract symbol from topic: orderbook.50.BTCUSDT
	parts := strings.Split(topic, ".")
	if len(parts) < 3 {
		return
	}
	symbol := parts[2]

	var obData WSOrderbookData
	if err := json.Unmarshal(data, &obData); err != nil {
		log.Error().Err(err).Msg("Failed to parse orderbook data")
		return
	}

	isSnapshot := msgType == "snapshot"

	ws.mu.Lock()
	ob, exists := ws.orderbooks[symbol]
	if !exists {
		ob = &LocalOrderbook{
			Symbol: symbol,
			Bids:   make(map[string]float64),
			Asks:   make(map[string]float64),
		}
		ws.orderbooks[symbol] = ob
	}

	if isSnapshot {
		// Clear and rebuild orderbook
		ob.Bids = make(map[string]float64)
		ob.Asks = make(map[string]float64)
	}

	// Apply updates
	for _, bid := range obData.Bids {
		if len(bid) >= 2 {
			size, _ := strconv.ParseFloat(bid[1], 64)
			if size == 0 {
				delete(ob.Bids, bid[0])
			} else {
				ob.Bids[bid[0]] = size
			}
		}
	}

	for _, ask := range obData.Asks {
		if len(ask) >= 2 {
			size, _ := strconv.ParseFloat(ask[1], 64)
			if size == 0 {
				delete(ob.Asks, ask[0])
			} else {
				ob.Asks[ask[0]] = size
			}
		}
	}

	ob.UpdateID = obData.UpdateID
	ob.Seq = obData.Seq
	ob.Timestamp = ts
	ws.mu.Unlock()

	// Emit callback
	ws.mu.RLock()
	callback := ws.onOrderbook
	ws.mu.RUnlock()

	if callback != nil {
		callback(symbol, &obData, isSnapshot, ts)
	}
}

// handleTicker processes ticker messages
func (ws *MarketDataWS) handleTicker(topic string, data json.RawMessage) {
	// Extract symbol from topic: tickers.BTCUSDT
	parts := strings.Split(topic, ".")
	if len(parts) < 2 {
		return
	}
	symbol := parts[1]

	var tickerData WSTickerData
	if err := json.Unmarshal(data, &tickerData); err != nil {
		log.Error().Err(err).Msg("Failed to parse ticker data")
		return
	}

	ws.mu.RLock()
	callback := ws.onTicker
	ws.mu.RUnlock()

	if callback != nil {
		callback(symbol, &tickerData)
	}
}

// handleTrade processes trade messages
func (ws *MarketDataWS) handleTrade(topic string, data json.RawMessage) {
	// Extract symbol from topic: publicTrade.BTCUSDT
	parts := strings.Split(topic, ".")
	if len(parts) < 2 {
		return
	}
	symbol := parts[1]

	var trades []WSTradeData
	if err := json.Unmarshal(data, &trades); err != nil {
		log.Error().Err(err).Msg("Failed to parse trade data")
		return
	}

	ws.mu.RLock()
	callback := ws.onTrade
	ws.mu.RUnlock()

	if callback != nil {
		callback(symbol, trades)
	}
}

// pingLoop sends periodic ping messages
func (ws *MarketDataWS) pingLoop() {
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
func (ws *MarketDataWS) tryReconnect() {
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
		Msg("Reconnecting to Bybit WebSocket")

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
func (ws *MarketDataWS) emitError(err error) {
	ws.mu.RLock()
	callback := ws.onError
	ws.mu.RUnlock()

	if callback != nil {
		callback(err)
	} else {
		log.Error().Err(err).Msg("Bybit WebSocket error")
	}
}

// GetLocalOrderbook returns the current local orderbook state
func (ws *MarketDataWS) GetLocalOrderbook(symbol string) *LocalOrderbook {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return ws.orderbooks[symbol]
}

// GetSortedOrderbook returns sorted bids and asks from local orderbook
func (ws *MarketDataWS) GetSortedOrderbook(symbol string) (bids, asks [][2]float64) {
	ws.mu.RLock()
	ob := ws.orderbooks[symbol]
	ws.mu.RUnlock()

	if ob == nil {
		return nil, nil
	}

	ws.mu.RLock()
	defer ws.mu.RUnlock()

	// Convert and sort bids (descending by price)
	bidPrices := make([]float64, 0, len(ob.Bids))
	for priceStr := range ob.Bids {
		price, _ := strconv.ParseFloat(priceStr, 64)
		bidPrices = append(bidPrices, price)
	}
	sortDescending(bidPrices)

	bids = make([][2]float64, len(bidPrices))
	for i, price := range bidPrices {
		priceStr := fmt.Sprintf("%v", price)
		bids[i] = [2]float64{price, ob.Bids[priceStr]}
	}

	// Convert and sort asks (ascending by price)
	askPrices := make([]float64, 0, len(ob.Asks))
	for priceStr := range ob.Asks {
		price, _ := strconv.ParseFloat(priceStr, 64)
		askPrices = append(askPrices, price)
	}
	sortAscending(askPrices)

	asks = make([][2]float64, len(askPrices))
	for i, price := range askPrices {
		priceStr := fmt.Sprintf("%v", price)
		asks[i] = [2]float64{price, ob.Asks[priceStr]}
	}

	return bids, asks
}

// Helper functions for sorting
func sortDescending(prices []float64) {
	for i := 0; i < len(prices)-1; i++ {
		for j := i + 1; j < len(prices); j++ {
			if prices[i] < prices[j] {
				prices[i], prices[j] = prices[j], prices[i]
			}
		}
	}
}

func sortAscending(prices []float64) {
	for i := 0; i < len(prices)-1; i++ {
		for j := i + 1; j < len(prices); j++ {
			if prices[i] > prices[j] {
				prices[i], prices[j] = prices[j], prices[i]
			}
		}
	}
}
