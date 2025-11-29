package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"crossspread-md-ingest/internal/connector"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	wsBaseURL   = "wss://fstream.binance.com"
	restBaseURL = "https://fapi.binance.com"
)

// BinanceConnector implements the Connector interface for Binance Futures
type BinanceConnector struct {
	*connector.BaseConnector
	conn          *websocket.Conn
	subscriptions map[string]bool
	mu            sync.RWMutex
	done          chan struct{}
	depthLevels   int
	symbols       []string
}

// NewBinanceConnector creates a new Binance connector
func NewBinanceConnector(symbols []string, depthLevels int) *BinanceConnector {
	config := connector.ConnectorConfig{
		ExchangeID:     connector.Binance,
		WsURL:          wsBaseURL,
		RestURL:        restBaseURL,
		Symbols:        symbols,
		DepthLevels:    depthLevels,
		ReconnectDelay: 5 * time.Second,
		PingInterval:   30 * time.Second,
	}

	bc := &BinanceConnector{
		BaseConnector: connector.NewBaseConnector(config),
		subscriptions: make(map[string]bool),
		done:          make(chan struct{}),
		depthLevels:   depthLevels,
		symbols:       symbols,
	}

	// Pre-populate subscriptions
	for _, s := range symbols {
		bc.subscriptions[s] = true
	}

	return bc
}

// Connect establishes WebSocket connection to Binance
func (c *BinanceConnector) Connect(ctx context.Context) error {
	// Build stream URL for depth updates
	streams := c.buildStreamNames()
	if len(streams) == 0 {
		return fmt.Errorf("no symbols to subscribe")
	}

	url := fmt.Sprintf("%s/stream?streams=%s", wsBaseURL, streams)
	log.Info().Str("url", url).Msg("Connecting to Binance WebSocket")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.conn = conn
	c.SetConnected(true)
	log.Info().Msg("Connected to Binance WebSocket")

	// Start reading messages
	go c.readLoop()

	return nil
}

// ConnectForSymbols establishes WebSocket connection for specific symbols only
// Used for Phase 2 selective subscription after spread discovery
func (c *BinanceConnector) ConnectForSymbols(ctx context.Context, symbols []string) error {
	if len(symbols) == 0 {
		return fmt.Errorf("no symbols to subscribe")
	}

	// Update subscriptions
	c.mu.Lock()
	c.subscriptions = make(map[string]bool)
	for _, s := range symbols {
		c.subscriptions[s] = true
	}
	c.mu.Unlock()

	// Build stream URL only for requested symbols
	streams := c.buildStreamNames()
	url := fmt.Sprintf("%s/stream?streams=%s", wsBaseURL, streams)
	log.Info().
		Str("url", url).
		Int("symbols", len(symbols)).
		Msg("Connecting to Binance WebSocket for selected symbols")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.conn = conn
	c.SetConnected(true)
	log.Info().Int("symbols", len(symbols)).Msg("Connected to Binance WebSocket (selective)")

	// Start reading messages
	go c.readLoop()

	return nil
}

// Disconnect closes the WebSocket connection
func (c *BinanceConnector) Disconnect() error {
	close(c.done)
	c.SetConnected(false)
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Subscribe adds symbol subscriptions
func (c *BinanceConnector) Subscribe(symbols []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, s := range symbols {
		c.subscriptions[s] = true
	}
	return nil
}

// Unsubscribe removes symbol subscriptions
func (c *BinanceConnector) Unsubscribe(symbols []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, s := range symbols {
		delete(c.subscriptions, s)
	}
	return nil
}

// FetchInstruments fetches all USDT perpetual futures
func (c *BinanceConnector) FetchInstruments(ctx context.Context) ([]connector.Instrument, error) {
	url := fmt.Sprintf("%s/fapi/v1/exchangeInfo", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var exchangeInfo struct {
		Symbols []struct {
			Symbol       string `json:"symbol"`
			Status       string `json:"status"`
			BaseAsset    string `json:"baseAsset"`
			QuoteAsset   string `json:"quoteAsset"`
			ContractType string `json:"contractType"`
			Filters      []struct {
				FilterType  string `json:"filterType"`
				TickSize    string `json:"tickSize,omitempty"`
				StepSize    string `json:"stepSize,omitempty"`
				MinNotional string `json:"notional,omitempty"`
			} `json:"filters"`
		} `json:"symbols"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&exchangeInfo); err != nil {
		return nil, err
	}

	var instruments []connector.Instrument
	for _, s := range exchangeInfo.Symbols {
		if s.Status != "TRADING" || s.ContractType != "PERPETUAL" {
			continue
		}

		inst := connector.Instrument{
			ExchangeID:     connector.Binance,
			Symbol:         s.Symbol,
			Canonical:      fmt.Sprintf("%s-%s-PERP", s.BaseAsset, s.QuoteAsset),
			BaseAsset:      s.BaseAsset,
			QuoteAsset:     s.QuoteAsset,
			InstrumentType: "perpetual",
			ContractSize:   1,
			MakerFee:       0.0002,
			TakerFee:       0.0004,
		}

		// Extract filters
		for _, f := range s.Filters {
			switch f.FilterType {
			case "PRICE_FILTER":
				if ts, err := strconv.ParseFloat(f.TickSize, 64); err == nil {
					inst.TickSize = ts
				}
			case "LOT_SIZE":
				if ss, err := strconv.ParseFloat(f.StepSize, 64); err == nil {
					inst.LotSize = ss
				}
			case "MIN_NOTIONAL":
				if mn, err := strconv.ParseFloat(f.MinNotional, 64); err == nil {
					inst.MinNotional = mn
				}
			}
		}

		instruments = append(instruments, inst)
	}

	return instruments, nil
}

// FetchOrderbookSnapshot fetches orderbook via REST API
func (c *BinanceConnector) FetchOrderbookSnapshot(ctx context.Context, symbol string, depth int) (*connector.Orderbook, error) {
	url := fmt.Sprintf("%s/fapi/v1/depth?symbol=%s&limit=%d", restBaseURL, symbol, depth)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		LastUpdateID int64      `json:"lastUpdateId"`
		Bids         [][]string `json:"bids"`
		Asks         [][]string `json:"asks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	ob := &connector.Orderbook{
		ExchangeID: connector.Binance,
		Symbol:     symbol,
		Timestamp:  time.Now(),
		SequenceID: data.LastUpdateID,
		IsSnapshot: true,
	}

	ob.Bids = parseLevels(data.Bids)
	ob.Asks = parseLevels(data.Asks)

	if len(ob.Bids) > 0 {
		ob.BestBid = ob.Bids[0].Price
	}
	if len(ob.Asks) > 0 {
		ob.BestAsk = ob.Asks[0].Price
	}
	if ob.BestBid > 0 && ob.BestAsk > 0 {
		ob.SpreadBps = (ob.BestAsk - ob.BestBid) / ob.BestBid * 10000
	}

	return ob, nil
}

// FetchFundingRates fetches current funding rates
func (c *BinanceConnector) FetchFundingRates(ctx context.Context) ([]connector.FundingRate, error) {
	url := fmt.Sprintf("%s/fapi/v1/premiumIndex", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data []struct {
		Symbol          string `json:"symbol"`
		LastFundingRate string `json:"lastFundingRate"`
		NextFundingTime int64  `json:"nextFundingTime"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var rates []connector.FundingRate
	for _, d := range data {
		rate, _ := strconv.ParseFloat(d.LastFundingRate, 64)
		rates = append(rates, connector.FundingRate{
			ExchangeID:           connector.Binance,
			Symbol:               d.Symbol,
			FundingRate:          rate,
			NextFundingTime:      time.UnixMilli(d.NextFundingTime),
			FundingIntervalHours: 8,
			Timestamp:            time.Now(),
		})
	}

	return rates, nil
}

// readLoop reads messages from WebSocket
func (c *BinanceConnector) readLoop() {
	defer c.SetConnected(false)

	for {
		select {
		case <-c.done:
			return
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				c.EmitError(fmt.Errorf("websocket read error: %w", err))
				return
			}

			c.handleMessage(message)
		}
	}
}

// handleMessage processes incoming WebSocket messages
func (c *BinanceConnector) handleMessage(message []byte) {
	var wrapper struct {
		Stream string          `json:"stream"`
		Data   json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(message, &wrapper); err != nil {
		c.EmitError(fmt.Errorf("unmarshal wrapper failed: %w", err))
		return
	}

	// Depth update
	if len(wrapper.Stream) > 0 && wrapper.Data != nil {
		var depth struct {
			EventType     string     `json:"e"`
			EventTime     int64      `json:"E"`
			Symbol        string     `json:"s"`
			FirstUpdateID int64      `json:"U"`
			FinalUpdateID int64      `json:"u"`
			Bids          [][]string `json:"b"`
			Asks          [][]string `json:"a"`
		}

		if err := json.Unmarshal(wrapper.Data, &depth); err != nil {
			c.EmitError(fmt.Errorf("unmarshal depth failed: %w", err))
			return
		}

		if depth.EventType == "depthUpdate" {
			ob := &connector.Orderbook{
				ExchangeID: connector.Binance,
				Symbol:     depth.Symbol,
				Canonical:  extractCanonical(depth.Symbol),
				Timestamp:  time.UnixMilli(depth.EventTime),
				SequenceID: depth.FinalUpdateID,
				IsSnapshot: false,
				Bids:       parseLevels(depth.Bids),
				Asks:       parseLevels(depth.Asks),
			}

			if len(ob.Bids) > 0 {
				ob.BestBid = ob.Bids[0].Price
			}
			if len(ob.Asks) > 0 {
				ob.BestAsk = ob.Asks[0].Price
			}
			if ob.BestBid > 0 && ob.BestAsk > 0 {
				ob.SpreadBps = (ob.BestAsk - ob.BestBid) / ob.BestBid * 10000
			}

			c.EmitOrderbook(ob)
		}
	}
}

// buildStreamNames builds the combined stream URL parameter
func (c *BinanceConnector) buildStreamNames() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var streams []string
	for symbol := range c.subscriptions {
		// depth@100ms for 100ms updates
		streams = append(streams, fmt.Sprintf("%s@depth@100ms", toLower(symbol)))
	}

	result := ""
	for i, s := range streams {
		if i > 0 {
			result += "/"
		}
		result += s
	}
	return result
}

// parseLevels converts string arrays to PriceLevel slice
func parseLevels(data [][]string) []connector.PriceLevel {
	levels := make([]connector.PriceLevel, 0, len(data))
	for _, item := range data {
		if len(item) < 2 {
			continue
		}
		price, _ := strconv.ParseFloat(item[0], 64)
		qty, _ := strconv.ParseFloat(item[1], 64)
		if qty > 0 {
			levels = append(levels, connector.PriceLevel{
				Price:    price,
				Quantity: qty,
			})
		}
	}

	// Sort bids descending, asks ascending
	sort.Slice(levels, func(i, j int) bool {
		return levels[i].Price > levels[j].Price
	})

	return levels
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}

// FetchPriceTickers fetches current prices for all symbols via REST API
// This is used for Phase 1 spread discovery before WebSocket connection
func (c *BinanceConnector) FetchPriceTickers(ctx context.Context) ([]connector.PriceTicker, error) {
	url := fmt.Sprintf("%s/fapi/v1/ticker/price", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var data []struct {
		Symbol string `json:"symbol"`
		Price  string `json:"price"`
		Time   int64  `json:"time"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	tickers := make([]connector.PriceTicker, 0, len(data))
	for _, d := range data {
		price, _ := strconv.ParseFloat(d.Price, 64)
		if price <= 0 {
			continue
		}

		// Extract base asset from symbol (e.g., BTCUSDT -> BTC)
		canonical := extractCanonical(d.Symbol)

		tickers = append(tickers, connector.PriceTicker{
			ExchangeID: connector.Binance,
			Symbol:     d.Symbol,
			Canonical:  canonical,
			Price:      price,
			Timestamp:  time.UnixMilli(d.Time),
		})
	}

	log.Info().Int("count", len(tickers)).Msg("Fetched Binance price tickers")
	return tickers, nil
}

// FetchBookTickers fetches current best bid/ask for all symbols via REST API
// More detailed than FetchPriceTickers, includes bid/ask spreads
func (c *BinanceConnector) FetchBookTickers(ctx context.Context) ([]connector.PriceTicker, error) {
	url := fmt.Sprintf("%s/fapi/v1/ticker/bookTicker", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var data []struct {
		Symbol   string `json:"symbol"`
		BidPrice string `json:"bidPrice"`
		AskPrice string `json:"askPrice"`
		BidQty   string `json:"bidQty"`
		AskQty   string `json:"askQty"`
		Time     int64  `json:"time"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	tickers := make([]connector.PriceTicker, 0, len(data))
	for _, d := range data {
		bidPrice, _ := strconv.ParseFloat(d.BidPrice, 64)
		askPrice, _ := strconv.ParseFloat(d.AskPrice, 64)
		if bidPrice <= 0 || askPrice <= 0 {
			continue
		}

		canonical := extractCanonical(d.Symbol)
		midPrice := (bidPrice + askPrice) / 2

		tickers = append(tickers, connector.PriceTicker{
			ExchangeID: connector.Binance,
			Symbol:     d.Symbol,
			Canonical:  canonical,
			Price:      midPrice,
			BidPrice:   bidPrice,
			AskPrice:   askPrice,
			Timestamp:  time.UnixMilli(d.Time),
		})
	}

	log.Info().Int("count", len(tickers)).Msg("Fetched Binance book tickers")
	return tickers, nil
}

// FetchAssetInfo fetches deposit/withdrawal status for assets
// Note: This requires API key authentication for Binance
// For unauthenticated access, we return basic asset info from exchangeInfo
func (c *BinanceConnector) FetchAssetInfo(ctx context.Context) ([]connector.AssetInfo, error) {
	// Fetch from exchangeInfo to get list of assets
	url := fmt.Sprintf("%s/fapi/v1/exchangeInfo", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var exchangeInfo struct {
		Assets []struct {
			Asset           string `json:"asset"`
			MarginAvailable bool   `json:"marginAvailable"`
		} `json:"assets"`
		Symbols []struct {
			Symbol     string `json:"symbol"`
			Status     string `json:"status"`
			BaseAsset  string `json:"baseAsset"`
			QuoteAsset string `json:"quoteAsset"`
		} `json:"symbols"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&exchangeInfo); err != nil {
		return nil, err
	}

	// Create a map of unique base assets from trading symbols
	assetMap := make(map[string]bool)
	for _, s := range exchangeInfo.Symbols {
		if s.Status == "TRADING" {
			assetMap[s.BaseAsset] = true
		}
	}

	// Build asset info (futures don't have deposit/withdrawal, using margin available)
	assetInfos := make([]connector.AssetInfo, 0, len(assetMap))
	for asset := range assetMap {
		assetInfos = append(assetInfos, connector.AssetInfo{
			ExchangeID:      connector.Binance,
			Asset:           asset,
			DepositEnabled:  true, // Futures margin deposit always available if trading
			WithdrawEnabled: true, // Futures margin withdrawal always available
			Timestamp:       time.Now(),
		})
	}

	log.Info().Int("count", len(assetInfos)).Msg("Fetched Binance asset info")
	return assetInfos, nil
}

// extractCanonical extracts the canonical symbol from exchange-specific format
// BTCUSDT -> BTC, ETHUSDT -> ETH
func extractCanonical(symbol string) string {
	// Common quote currencies in order of length (longest first)
	quotes := []string{"USDT", "USDC", "BUSD", "TUSD", "USD"}
	for _, quote := range quotes {
		if len(symbol) > len(quote) && symbol[len(symbol)-len(quote):] == quote {
			return symbol[:len(symbol)-len(quote)]
		}
	}
	return symbol
}
