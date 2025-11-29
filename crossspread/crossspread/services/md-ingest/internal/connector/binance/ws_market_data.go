package binance

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
	wsStreamBaseURL  = "wss://fstream.binance.com"
	wsStreamEndpoint = "/stream"
)

// MarketDataHandler handles different types of market data events
type MarketDataHandler struct {
	OnTrade      func(event *WSTradeEvent)
	OnDepth      func(event *WSDepthEvent)
	OnMarkPrice  func(event *WSMarkPriceEvent)
	OnKline      func(event *WSKlineEvent)
	OnMiniTicker func(event *WSMiniTickerEvent)
	OnError      func(err error)
}

// MarketDataStream manages WebSocket connections for market data
type MarketDataStream struct {
	conn          *websocket.Conn
	handler       *MarketDataHandler
	subscriptions map[string]bool
	mu            sync.RWMutex
	done          chan struct{}
	reconnect     bool
	connected     bool
}

// NewMarketDataStream creates a new market data stream
func NewMarketDataStream(handler *MarketDataHandler) *MarketDataStream {
	return &MarketDataStream{
		handler:       handler,
		subscriptions: make(map[string]bool),
		done:          make(chan struct{}),
		reconnect:     true,
	}
}

// Connect connects to the WebSocket stream with specified subscriptions
func (s *MarketDataStream) Connect(ctx context.Context, streams []string) error {
	if len(streams) == 0 {
		return fmt.Errorf("no streams specified")
	}

	// Store subscriptions
	s.mu.Lock()
	for _, stream := range streams {
		s.subscriptions[stream] = true
	}
	s.mu.Unlock()

	// Build URL with combined streams
	streamParam := strings.Join(streams, "/")
	url := fmt.Sprintf("%s%s?streams=%s", wsStreamBaseURL, wsStreamEndpoint, streamParam)

	log.Info().
		Str("url", url).
		Int("streams", len(streams)).
		Msg("Connecting to Binance market data stream")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	s.conn = conn
	s.connected = true
	log.Info().Msg("Connected to Binance market data stream")

	// Start reading messages
	go s.readLoop(ctx)

	return nil
}

// ConnectForSymbols creates stream subscriptions for given symbols
func (s *MarketDataStream) ConnectForSymbols(ctx context.Context, symbols []string, streamTypes []string) error {
	streams := make([]string, 0, len(symbols)*len(streamTypes))

	for _, symbol := range symbols {
		symbolLower := strings.ToLower(symbol)
		for _, streamType := range streamTypes {
			streams = append(streams, fmt.Sprintf("%s%s", symbolLower, streamType))
		}
	}

	return s.Connect(ctx, streams)
}

// Disconnect closes the WebSocket connection
func (s *MarketDataStream) Disconnect() error {
	s.reconnect = false
	close(s.done)
	s.connected = false
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// IsConnected returns true if the stream is connected
func (s *MarketDataStream) IsConnected() bool {
	return s.connected
}

// readLoop reads messages from the WebSocket
func (s *MarketDataStream) readLoop(ctx context.Context) {
	defer func() {
		s.connected = false
		if s.reconnect {
			log.Warn().Msg("Binance market data stream disconnected, will attempt reconnect")
			// Implement exponential backoff reconnection here if needed
		}
	}()

	for {
		select {
		case <-s.done:
			return
		case <-ctx.Done():
			return
		default:
			_, message, err := s.conn.ReadMessage()
			if err != nil {
				if s.handler != nil && s.handler.OnError != nil {
					s.handler.OnError(fmt.Errorf("read error: %w", err))
				}
				return
			}
			s.handleMessage(message)
		}
	}
}

// handleMessage parses and routes incoming messages
func (s *MarketDataStream) handleMessage(message []byte) {
	// Combined stream messages have a wrapper
	var wrapper struct {
		Stream string          `json:"stream"`
		Data   json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(message, &wrapper); err != nil {
		// Try parsing as direct message
		s.parseDirectMessage(message)
		return
	}

	// Route based on stream type
	if strings.HasSuffix(wrapper.Stream, "@trade") {
		var event WSTradeEvent
		if err := json.Unmarshal(wrapper.Data, &event); err == nil && s.handler != nil && s.handler.OnTrade != nil {
			s.handler.OnTrade(&event)
		}
	} else if strings.Contains(wrapper.Stream, "@depth") {
		var event WSDepthEvent
		if err := json.Unmarshal(wrapper.Data, &event); err == nil && s.handler != nil && s.handler.OnDepth != nil {
			s.handler.OnDepth(&event)
		}
	} else if strings.Contains(wrapper.Stream, "@markPrice") {
		var event WSMarkPriceEvent
		if err := json.Unmarshal(wrapper.Data, &event); err == nil && s.handler != nil && s.handler.OnMarkPrice != nil {
			s.handler.OnMarkPrice(&event)
		}
	} else if strings.Contains(wrapper.Stream, "@kline") {
		var event WSKlineEvent
		if err := json.Unmarshal(wrapper.Data, &event); err == nil && s.handler != nil && s.handler.OnKline != nil {
			s.handler.OnKline(&event)
		}
	} else if strings.Contains(wrapper.Stream, "miniTicker") {
		var event WSMiniTickerEvent
		if err := json.Unmarshal(wrapper.Data, &event); err == nil && s.handler != nil && s.handler.OnMiniTicker != nil {
			s.handler.OnMiniTicker(&event)
		}
	}
}

// parseDirectMessage parses messages without stream wrapper
func (s *MarketDataStream) parseDirectMessage(message []byte) {
	// Try to determine message type from the event field
	var eventType struct {
		E string `json:"e"`
	}
	if err := json.Unmarshal(message, &eventType); err != nil {
		return
	}

	switch eventType.E {
	case "trade":
		var event WSTradeEvent
		if err := json.Unmarshal(message, &event); err == nil && s.handler != nil && s.handler.OnTrade != nil {
			s.handler.OnTrade(&event)
		}
	case "depthUpdate":
		var event WSDepthEvent
		if err := json.Unmarshal(message, &event); err == nil && s.handler != nil && s.handler.OnDepth != nil {
			s.handler.OnDepth(&event)
		}
	case "markPriceUpdate":
		var event WSMarkPriceEvent
		if err := json.Unmarshal(message, &event); err == nil && s.handler != nil && s.handler.OnMarkPrice != nil {
			s.handler.OnMarkPrice(&event)
		}
	case "kline":
		var event WSKlineEvent
		if err := json.Unmarshal(message, &event); err == nil && s.handler != nil && s.handler.OnKline != nil {
			s.handler.OnKline(&event)
		}
	case "24hrMiniTicker":
		var event WSMiniTickerEvent
		if err := json.Unmarshal(message, &event); err == nil && s.handler != nil && s.handler.OnMiniTicker != nil {
			s.handler.OnMiniTicker(&event)
		}
	}
}

// =============================================================================
// Stream Type Constants
// =============================================================================

const (
	StreamTypeTrade          = "@trade"
	StreamTypeDepth          = "@depth"
	StreamTypeDepth100ms     = "@depth@100ms"
	StreamTypeDepth500ms     = "@depth@500ms"
	StreamTypeMarkPrice      = "@markPrice"
	StreamTypeMarkPrice1s    = "@markPrice@1s"
	StreamTypeKline1m        = "@kline_1m"
	StreamTypeKline5m        = "@kline_5m"
	StreamTypeKline15m       = "@kline_15m"
	StreamTypeKline1h        = "@kline_1h"
	StreamTypeKline4h        = "@kline_4h"
	StreamTypeKline1d        = "@kline_1d"
	StreamTypeMiniTicker     = "@miniTicker"
	StreamTypeAllMiniTickers = "!miniTicker@arr"
)

// =============================================================================
// Helper Functions for Stream Data Conversion
// =============================================================================

// ParseDepthLevels converts WebSocket depth data to price levels
func ParseDepthLevels(data [][]string) []PriceLevel {
	levels := make([]PriceLevel, 0, len(data))
	for _, item := range data {
		if len(item) < 2 {
			continue
		}
		price, _ := strconv.ParseFloat(item[0], 64)
		qty, _ := strconv.ParseFloat(item[1], 64)
		if qty > 0 {
			levels = append(levels, PriceLevel{
				Price:    price,
				Quantity: qty,
			})
		}
	}
	return levels
}

// PriceLevel represents a single price level in the orderbook
type PriceLevel struct {
	Price    float64
	Quantity float64
}

// =============================================================================
// Orderbook Manager (for maintaining local orderbook)
// =============================================================================

// OrderbookManager maintains a local copy of the orderbook
type OrderbookManager struct {
	symbol       string
	lastUpdateId int64
	bids         map[string]float64 // price -> quantity
	asks         map[string]float64 // price -> quantity
	mu           sync.RWMutex
	initialized  bool
}

// NewOrderbookManager creates a new orderbook manager
func NewOrderbookManager(symbol string) *OrderbookManager {
	return &OrderbookManager{
		symbol: symbol,
		bids:   make(map[string]float64),
		asks:   make(map[string]float64),
	}
}

// InitializeFromSnapshot initializes the orderbook from a REST snapshot
func (o *OrderbookManager) InitializeFromSnapshot(snapshot *DepthResponse) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.bids = make(map[string]float64)
	o.asks = make(map[string]float64)

	for _, bid := range snapshot.Bids {
		if len(bid) >= 2 {
			qty, _ := strconv.ParseFloat(bid[1], 64)
			if qty > 0 {
				o.bids[bid[0]] = qty
			}
		}
	}

	for _, ask := range snapshot.Asks {
		if len(ask) >= 2 {
			qty, _ := strconv.ParseFloat(ask[1], 64)
			if qty > 0 {
				o.asks[ask[0]] = qty
			}
		}
	}

	o.lastUpdateId = snapshot.LastUpdateId
	o.initialized = true
}

// ApplyUpdate applies a depth update to the orderbook
func (o *OrderbookManager) ApplyUpdate(event *WSDepthEvent) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Validate update sequence
	if !o.initialized {
		return false
	}

	// First update after snapshot
	if event.PrevFinalId != o.lastUpdateId && event.FirstUpdateId > o.lastUpdateId+1 {
		return false // Need to resync
	}

	// Apply bid updates
	for _, bid := range event.Bids {
		if len(bid) >= 2 {
			qty, _ := strconv.ParseFloat(bid[1], 64)
			if qty == 0 {
				delete(o.bids, bid[0])
			} else {
				o.bids[bid[0]] = qty
			}
		}
	}

	// Apply ask updates
	for _, ask := range event.Asks {
		if len(ask) >= 2 {
			qty, _ := strconv.ParseFloat(ask[1], 64)
			if qty == 0 {
				delete(o.asks, ask[0])
			} else {
				o.asks[ask[0]] = qty
			}
		}
	}

	o.lastUpdateId = event.FinalUpdateId
	return true
}

// GetBestBidAsk returns the best bid and ask prices and quantities
func (o *OrderbookManager) GetBestBidAsk() (bestBid, bestBidQty, bestAsk, bestAskQty float64) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	// Find best bid (highest price)
	for priceStr, qty := range o.bids {
		price, _ := strconv.ParseFloat(priceStr, 64)
		if price > bestBid {
			bestBid = price
			bestBidQty = qty
		}
	}

	// Find best ask (lowest price)
	bestAsk = -1
	for priceStr, qty := range o.asks {
		price, _ := strconv.ParseFloat(priceStr, 64)
		if bestAsk < 0 || price < bestAsk {
			bestAsk = price
			bestAskQty = qty
		}
	}
	if bestAsk < 0 {
		bestAsk = 0
	}

	return
}

// GetTopLevels returns the top N bid and ask levels
func (o *OrderbookManager) GetTopLevels(n int) (bids, asks []PriceLevel) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	// Convert to slices and sort
	bidSlice := make([]PriceLevel, 0, len(o.bids))
	for priceStr, qty := range o.bids {
		price, _ := strconv.ParseFloat(priceStr, 64)
		bidSlice = append(bidSlice, PriceLevel{Price: price, Quantity: qty})
	}

	askSlice := make([]PriceLevel, 0, len(o.asks))
	for priceStr, qty := range o.asks {
		price, _ := strconv.ParseFloat(priceStr, 64)
		askSlice = append(askSlice, PriceLevel{Price: price, Quantity: qty})
	}

	// Sort bids descending
	for i := 0; i < len(bidSlice)-1; i++ {
		for j := i + 1; j < len(bidSlice); j++ {
			if bidSlice[i].Price < bidSlice[j].Price {
				bidSlice[i], bidSlice[j] = bidSlice[j], bidSlice[i]
			}
		}
	}

	// Sort asks ascending
	for i := 0; i < len(askSlice)-1; i++ {
		for j := i + 1; j < len(askSlice); j++ {
			if askSlice[i].Price > askSlice[j].Price {
				askSlice[i], askSlice[j] = askSlice[j], askSlice[i]
			}
		}
	}

	// Return top N
	if len(bidSlice) > n {
		bids = bidSlice[:n]
	} else {
		bids = bidSlice
	}

	if len(askSlice) > n {
		asks = askSlice[:n]
	} else {
		asks = askSlice
	}

	return
}
