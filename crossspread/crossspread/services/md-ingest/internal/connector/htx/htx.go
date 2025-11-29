package htx

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	wsBaseURL   = "wss://api.hbdm.com/linear-swap-ws"
	restBaseURL = "https://api.hbdm.com"
)

// HTXConnector implements the Connector interface for HTX (Huobi) Futures
type HTXConnector struct {
	*connector.BaseConnector
	conn          *websocket.Conn
	subscriptions map[string]bool
	mu            sync.RWMutex
	done          chan struct{}
}

// NewHTXConnector creates a new HTX connector
func NewHTXConnector(symbols []string, depthLevels int) *HTXConnector {
	config := connector.ConnectorConfig{
		ExchangeID:     connector.HTX,
		WsURL:          wsBaseURL,
		RestURL:        restBaseURL,
		Symbols:        symbols,
		DepthLevels:    depthLevels,
		ReconnectDelay: 5 * time.Second,
		PingInterval:   20 * time.Second,
	}

	c := &HTXConnector{
		BaseConnector: connector.NewBaseConnector(config),
		subscriptions: make(map[string]bool),
		done:          make(chan struct{}),
	}

	for _, s := range symbols {
		c.subscriptions[s] = true
	}

	return c
}

// Connect establishes WebSocket connection to HTX
func (c *HTXConnector) Connect(ctx context.Context) error {
	log.Info().Str("url", wsBaseURL).Msg("Connecting to HTX WebSocket")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, wsBaseURL, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.conn = conn
	c.SetConnected(true)
	log.Info().Msg("Connected to HTX WebSocket")

	// Subscribe to orderbook updates
	for symbol := range c.subscriptions {
		if err := c.subscribeSymbol(symbol); err != nil {
			log.Error().Err(err).Str("symbol", symbol).Msg("Failed to subscribe")
		}
	}

	go c.readLoop()

	return nil
}

func (c *HTXConnector) subscribeSymbol(symbol string) error {
	msg := map[string]interface{}{
		"sub": fmt.Sprintf("market.%s.depth.step0", symbol),
		"id":  fmt.Sprintf("depth_%s", symbol),
	}
	return c.conn.WriteJSON(msg)
}

// Disconnect closes the WebSocket connection
func (c *HTXConnector) Disconnect() error {
	close(c.done)
	c.SetConnected(false)
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Subscribe adds symbol subscriptions
func (c *HTXConnector) Subscribe(symbols []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range symbols {
		c.subscriptions[s] = true
	}
	return nil
}

// Unsubscribe removes symbol subscriptions
func (c *HTXConnector) Unsubscribe(symbols []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range symbols {
		delete(c.subscriptions, s)
	}
	return nil
}

// FetchInstruments fetches all USDT perpetual futures
func (c *HTXConnector) FetchInstruments(ctx context.Context) ([]connector.Instrument, error) {
	url := fmt.Sprintf("%s/linear-swap-api/v1/swap_contract_info", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
		Data   []struct {
			Symbol         string  `json:"symbol"`
			ContractCode   string  `json:"contract_code"`
			PriceTick      float64 `json:"price_tick"`
			ContractSize   float64 `json:"contract_size"`
			ContractStatus int     `json:"contract_status"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var instruments []connector.Instrument
	for _, s := range result.Data {
		if s.ContractStatus != 1 {
			continue
		}

		inst := connector.Instrument{
			ExchangeID:     connector.HTX,
			Symbol:         s.ContractCode,
			Canonical:      fmt.Sprintf("%s-USDT-PERP", s.Symbol),
			BaseAsset:      s.Symbol,
			QuoteAsset:     "USDT",
			InstrumentType: "perpetual",
			TickSize:       s.PriceTick,
			LotSize:        1,
			ContractSize:   s.ContractSize,
			TakerFee:       0.0004,
			MakerFee:       0.0002,
		}
		instruments = append(instruments, inst)
	}

	return instruments, nil
}

// FetchOrderbookSnapshot fetches orderbook via REST API
func (c *HTXConnector) FetchOrderbookSnapshot(ctx context.Context, symbol string, depth int) (*connector.Orderbook, error) {
	url := fmt.Sprintf("%s/linear-swap-ex/market/depth?contract_code=%s&type=step0", restBaseURL, symbol)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
		Tick   struct {
			Bids [][]float64 `json:"bids"`
			Asks [][]float64 `json:"asks"`
			Ts   int64       `json:"ts"`
		} `json:"tick"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	ob := &connector.Orderbook{
		ExchangeID: connector.HTX,
		Symbol:     symbol,
		Timestamp:  time.UnixMilli(result.Tick.Ts),
		IsSnapshot: true,
	}

	ob.Bids = parseFloatLevels(result.Tick.Bids)
	ob.Asks = parseFloatLevels(result.Tick.Asks)

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
func (c *HTXConnector) FetchFundingRates(ctx context.Context) ([]connector.FundingRate, error) {
	url := fmt.Sprintf("%s/linear-swap-api/v1/swap_batch_funding_rate", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
		Data   []struct {
			Symbol          string `json:"symbol"`
			ContractCode    string `json:"contract_code"`
			FundingRate     string `json:"funding_rate"`
			NextFundingTime string `json:"next_funding_time"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var rates []connector.FundingRate
	for _, d := range result.Data {
		rate, _ := strconv.ParseFloat(d.FundingRate, 64)
		nextTime, _ := strconv.ParseInt(d.NextFundingTime, 10, 64)
		rates = append(rates, connector.FundingRate{
			ExchangeID:           connector.HTX,
			Symbol:               d.ContractCode,
			FundingRate:          rate,
			NextFundingTime:      time.UnixMilli(nextTime),
			FundingIntervalHours: 8,
			Timestamp:            time.Now(),
		})
	}

	return rates, nil
}

func (c *HTXConnector) readLoop() {
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

			// HTX sends gzip compressed messages
			decompressed, err := gzipDecompress(message)
			if err != nil {
				c.handleMessage(message) // Try uncompressed
			} else {
				c.handleMessage(decompressed)
			}
		}
	}
}

func gzipDecompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (c *HTXConnector) handleMessage(message []byte) {
	// Handle ping messages
	var ping struct {
		Ping int64 `json:"ping"`
	}
	if err := json.Unmarshal(message, &ping); err == nil && ping.Ping > 0 {
		pong := map[string]int64{"pong": ping.Ping}
		c.conn.WriteJSON(pong)
		return
	}

	var msg struct {
		Ch   string `json:"ch"`
		Ts   int64  `json:"ts"`
		Tick struct {
			Bids [][]float64 `json:"bids"`
			Asks [][]float64 `json:"asks"`
		} `json:"tick"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		return
	}

	if msg.Ch == "" || msg.Tick.Bids == nil {
		return
	}

	// Extract symbol from channel: market.BTC-USDT.depth.step0
	symbol := extractSymbol(msg.Ch)

	ob := &connector.Orderbook{
		ExchangeID: connector.HTX,
		Symbol:     symbol,
		Canonical:  extractCanonical(symbol),
		Timestamp:  time.UnixMilli(msg.Ts),
		IsSnapshot: true,
	}

	ob.Bids = parseFloatLevels(msg.Tick.Bids)
	ob.Asks = parseFloatLevels(msg.Tick.Asks)

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

func extractSymbol(ch string) string {
	// market.BTC-USDT.depth.step0 -> BTC-USDT
	parts := splitString(ch, '.')
	if len(parts) >= 2 {
		return parts[1]
	}
	return ch
}

// extractCanonical extracts base asset from HTX symbol (BTC-USDT -> BTC)
func extractCanonical(symbol string) string {
	parts := splitString(symbol, '-')
	if len(parts) >= 1 {
		return parts[0]
	}
	return symbol
}

func splitString(s string, sep byte) []string {
	var parts []string
	current := ""
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(s[i])
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func parseFloatLevels(data [][]float64) []connector.PriceLevel {
	levels := make([]connector.PriceLevel, 0, len(data))
	for _, item := range data {
		if len(item) < 2 {
			continue
		}
		if item[1] > 0 {
			levels = append(levels, connector.PriceLevel{
				Price:    item[0],
				Quantity: item[1],
			})
		}
	}

	sort.Slice(levels, func(i, j int) bool {
		return levels[i].Price > levels[j].Price
	})

	return levels
}

// ConnectForSymbols establishes WebSocket connection for specific symbols only
func (c *HTXConnector) ConnectForSymbols(ctx context.Context, symbols []string) error {
	if len(symbols) == 0 {
		return fmt.Errorf("no symbols to subscribe")
	}

	c.mu.Lock()
	c.subscriptions = make(map[string]bool)
	for _, s := range symbols {
		c.subscriptions[s] = true
	}
	c.mu.Unlock()

	return c.Connect(ctx)
}

// FetchPriceTickers fetches current prices for all symbols via REST API
func (c *HTXConnector) FetchPriceTickers(ctx context.Context) ([]connector.PriceTicker, error) {
	url := fmt.Sprintf("%s/linear-swap-ex/market/detail/batch_merged", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
		Ticks  []struct {
			ContractCode string      `json:"contract_code"`
			Close        json.Number `json:"close"`
			Bid          []float64   `json:"bid"`
			Ask          []float64   `json:"ask"`
			Vol          json.Number `json:"vol"`
		} `json:"ticks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var tickers []connector.PriceTicker
	for _, t := range result.Ticks {
		var bidPrice, askPrice float64
		if len(t.Bid) > 0 {
			bidPrice = t.Bid[0]
		}
		if len(t.Ask) > 0 {
			askPrice = t.Ask[0]
		}

		closePrice, _ := t.Close.Float64()
		vol, _ := t.Vol.Float64()

		tickers = append(tickers, connector.PriceTicker{
			ExchangeID: connector.HTX,
			Symbol:     t.ContractCode,
			Canonical:  extractCanonical(t.ContractCode),
			Price:      closePrice,
			BidPrice:   bidPrice,
			AskPrice:   askPrice,
			Volume24h:  vol,
			Timestamp:  time.Now(),
		})
	}

	return tickers, nil
}

// FetchAssetInfo fetches deposit/withdrawal status for assets
// Uses contract info to derive available assets since detailed deposit/withdraw info requires authentication
func (c *HTXConnector) FetchAssetInfo(ctx context.Context) ([]connector.AssetInfo, error) {
	url := fmt.Sprintf("%s/linear-swap-api/v1/swap_contract_info", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch contract info: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
		Data   []struct {
			Symbol         string `json:"symbol"`
			ContractCode   string `json:"contract_code"`
			ContractStatus int    `json:"contract_status"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Status != "ok" {
		return nil, fmt.Errorf("HTX API returned status: %s", result.Status)
	}

	// Extract unique base assets from active contracts
	assetMap := make(map[string]*connector.AssetInfo)
	for _, c := range result.Data {
		asset := c.Symbol
		if asset == "" {
			continue
		}

		if _, exists := assetMap[asset]; !exists {
			// Contract status 1 = active
			isActive := c.ContractStatus == 1

			assetMap[asset] = &connector.AssetInfo{
				ExchangeID:      connector.HTX,
				Asset:           asset,
				Networks:        []string{asset},
				DepositEnabled:  isActive,
				WithdrawEnabled: isActive,
				MinWithdraw:     0, // Requires authenticated API
				WithdrawFee:     0, // Requires authenticated API
				Timestamp:       time.Now(),
			}
		}
	}

	// Convert map to slice
	assets := make([]connector.AssetInfo, 0, len(assetMap))
	for _, info := range assetMap {
		assets = append(assets, *info)
	}

	log.Debug().Int("count", len(assets)).Msg("Fetched HTX asset info from contracts")
	return assets, nil
}
