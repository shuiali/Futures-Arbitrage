package bitget

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
	wsBaseURL   = "wss://ws.bitget.com/v2/ws/public"
	restBaseURL = "https://api.bitget.com"
)

// BitgetConnector implements the Connector interface for Bitget Futures
type BitgetConnector struct {
	*connector.BaseConnector
	conn          *websocket.Conn
	subscriptions map[string]bool
	mu            sync.RWMutex
	done          chan struct{}
}

// NewBitgetConnector creates a new Bitget connector
func NewBitgetConnector(symbols []string, depthLevels int) *BitgetConnector {
	config := connector.ConnectorConfig{
		ExchangeID:     connector.Bitget,
		WsURL:          wsBaseURL,
		RestURL:        restBaseURL,
		Symbols:        symbols,
		DepthLevels:    depthLevels,
		ReconnectDelay: 5 * time.Second,
		PingInterval:   25 * time.Second,
	}

	c := &BitgetConnector{
		BaseConnector: connector.NewBaseConnector(config),
		subscriptions: make(map[string]bool),
		done:          make(chan struct{}),
	}

	for _, s := range symbols {
		c.subscriptions[s] = true
	}

	return c
}

// Connect establishes WebSocket connection to Bitget
func (c *BitgetConnector) Connect(ctx context.Context) error {
	log.Info().Str("url", wsBaseURL).Msg("Connecting to Bitget WebSocket")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, wsBaseURL, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.conn = conn
	c.SetConnected(true)
	log.Info().Msg("Connected to Bitget WebSocket")

	// Subscribe to orderbook updates
	if err := c.subscribeAll(); err != nil {
		log.Error().Err(err).Msg("Failed to subscribe")
	}

	go c.readLoop()
	go c.pingLoop()

	return nil
}

func (c *BitgetConnector) subscribeAll() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var args []map[string]string
	for symbol := range c.subscriptions {
		args = append(args, map[string]string{
			"instType": "USDT-FUTURES",
			"channel":  "books15",
			"instId":   symbol,
		})
	}

	msg := map[string]interface{}{
		"op":   "subscribe",
		"args": args,
	}

	return c.conn.WriteJSON(msg)
}

// Disconnect closes the WebSocket connection
func (c *BitgetConnector) Disconnect() error {
	close(c.done)
	c.SetConnected(false)
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Subscribe adds symbol subscriptions
func (c *BitgetConnector) Subscribe(symbols []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range symbols {
		c.subscriptions[s] = true
	}
	return nil
}

// Unsubscribe removes symbol subscriptions
func (c *BitgetConnector) Unsubscribe(symbols []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range symbols {
		delete(c.subscriptions, s)
	}
	return nil
}

// FetchInstruments fetches all USDT perpetual futures
func (c *BitgetConnector) FetchInstruments(ctx context.Context) ([]connector.Instrument, error) {
	url := fmt.Sprintf("%s/api/v2/mix/market/contracts?productType=USDT-FUTURES", restBaseURL)

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
		Code string `json:"code"`
		Data []struct {
			Symbol         string `json:"symbol"`
			BaseCoin       string `json:"baseCoin"`
			QuoteCoin      string `json:"quoteCoin"`
			PricePlace     string `json:"pricePlace"`
			VolumePlace    string `json:"volumePlace"`
			SizeMultiplier string `json:"sizeMultiplier"`
			TakerFeeRate   string `json:"takerFeeRate"`
			MakerFeeRate   string `json:"makerFeeRate"`
			SymbolStatus   string `json:"symbolStatus"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var instruments []connector.Instrument
	for _, s := range result.Data {
		if s.SymbolStatus != "normal" {
			continue
		}

		pricePlace, _ := strconv.Atoi(s.PricePlace)
		volumePlace, _ := strconv.Atoi(s.VolumePlace)
		takerFee, _ := strconv.ParseFloat(s.TakerFeeRate, 64)
		makerFee, _ := strconv.ParseFloat(s.MakerFeeRate, 64)
		multiplier, _ := strconv.ParseFloat(s.SizeMultiplier, 64)

		tickSize := 1.0
		for i := 0; i < pricePlace; i++ {
			tickSize /= 10
		}
		lotSize := 1.0
		for i := 0; i < volumePlace; i++ {
			lotSize /= 10
		}

		inst := connector.Instrument{
			ExchangeID:     connector.Bitget,
			Symbol:         s.Symbol,
			Canonical:      fmt.Sprintf("%s-%s-PERP", s.BaseCoin, s.QuoteCoin),
			BaseAsset:      s.BaseCoin,
			QuoteAsset:     s.QuoteCoin,
			InstrumentType: "perpetual",
			TickSize:       tickSize,
			LotSize:        lotSize,
			ContractSize:   multiplier,
			TakerFee:       takerFee,
			MakerFee:       makerFee,
		}
		instruments = append(instruments, inst)
	}

	return instruments, nil
}

// FetchOrderbookSnapshot fetches orderbook via REST API
func (c *BitgetConnector) FetchOrderbookSnapshot(ctx context.Context, symbol string, depth int) (*connector.Orderbook, error) {
	url := fmt.Sprintf("%s/api/v2/mix/market/depth?symbol=%s&productType=USDT-FUTURES&limit=%d", restBaseURL, symbol, depth)

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
		Code string `json:"code"`
		Data struct {
			Bids [][]string `json:"bids"`
			Asks [][]string `json:"asks"`
			Ts   string     `json:"ts"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	ts, _ := strconv.ParseInt(result.Data.Ts, 10, 64)

	ob := &connector.Orderbook{
		ExchangeID: connector.Bitget,
		Symbol:     symbol,
		Timestamp:  time.UnixMilli(ts),
		IsSnapshot: true,
	}

	ob.Bids = parseStringLevels(result.Data.Bids)
	ob.Asks = parseStringLevels(result.Data.Asks)

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
func (c *BitgetConnector) FetchFundingRates(ctx context.Context) ([]connector.FundingRate, error) {
	url := fmt.Sprintf("%s/api/v2/mix/market/tickers?productType=USDT-FUTURES", restBaseURL)

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
		Code string `json:"code"`
		Data []struct {
			Symbol      string `json:"symbol"`
			FundingRate string `json:"fundingRate"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var rates []connector.FundingRate
	for _, d := range result.Data {
		rate, _ := strconv.ParseFloat(d.FundingRate, 64)
		
		// Extract canonical from symbol (e.g., BTCUSDT -> BTC)
		canonical := extractCanonical(d.Symbol)
		
		rates = append(rates, connector.FundingRate{
			ExchangeID:           connector.Bitget,
			Symbol:               d.Symbol,
			Canonical:            canonical,
			FundingRate:          rate,
			FundingIntervalHours: 8,
			Timestamp:            time.Now(),
		})
	}

	return rates, nil
}

func (c *BitgetConnector) readLoop() {
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

func (c *BitgetConnector) pingLoop() {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if err := c.conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
				log.Error().Err(err).Msg("Failed to send ping")
			}
		}
	}
}

func (c *BitgetConnector) handleMessage(message []byte) {
	// Skip pong messages
	if string(message) == "pong" {
		return
	}

	var msg struct {
		Action string `json:"action"`
		Arg    struct {
			InstType string `json:"instType"`
			Channel  string `json:"channel"`
			InstId   string `json:"instId"`
		} `json:"arg"`
		Data []struct {
			Bids [][]string `json:"bids"`
			Asks [][]string `json:"asks"`
			Ts   string     `json:"ts"`
		} `json:"data"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		return
	}

	if msg.Arg.Channel != "books15" || len(msg.Data) == 0 {
		return
	}

	ts, _ := strconv.ParseInt(msg.Data[0].Ts, 10, 64)

	ob := &connector.Orderbook{
		ExchangeID: connector.Bitget,
		Symbol:     msg.Arg.InstId,
		Canonical:  extractCanonical(msg.Arg.InstId),
		Timestamp:  time.UnixMilli(ts),
		IsSnapshot: msg.Action == "snapshot",
	}

	ob.Bids = parseStringLevels(msg.Data[0].Bids)
	ob.Asks = parseStringLevels(msg.Data[0].Asks)

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

func parseStringLevels(data [][]string) []connector.PriceLevel {
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

	sort.Slice(levels, func(i, j int) bool {
		return levels[i].Price > levels[j].Price
	})

	return levels
}

// extractCanonical extracts base asset from symbol (BTCUSDT -> BTC)
func extractCanonical(symbol string) string {
	quotes := []string{"USDT", "USDC", "USD"}
	for _, quote := range quotes {
		if len(symbol) > len(quote) && symbol[len(symbol)-len(quote):] == quote {
			return symbol[:len(symbol)-len(quote)]
		}
	}
	return symbol
}

// ConnectForSymbols establishes WebSocket connection for specific symbols only
func (c *BitgetConnector) ConnectForSymbols(ctx context.Context, symbols []string) error {
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
func (c *BitgetConnector) FetchPriceTickers(ctx context.Context) ([]connector.PriceTicker, error) {
	url := fmt.Sprintf("%s/api/v2/mix/market/tickers?productType=USDT-FUTURES", restBaseURL)

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
		Code string `json:"code"`
		Data []struct {
			Symbol    string `json:"symbol"`
			LastPrice string `json:"lastPr"`
			BidPrice  string `json:"bidPr"`
			AskPrice  string `json:"askPr"`
			Volume24h string `json:"baseVolume"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var tickers []connector.PriceTicker
	for _, t := range result.Data {
		price, _ := strconv.ParseFloat(t.LastPrice, 64)
		bidPrice, _ := strconv.ParseFloat(t.BidPrice, 64)
		askPrice, _ := strconv.ParseFloat(t.AskPrice, 64)
		volume, _ := strconv.ParseFloat(t.Volume24h, 64)

		tickers = append(tickers, connector.PriceTicker{
			ExchangeID: connector.Bitget,
			Symbol:     t.Symbol,
			Canonical:  extractCanonical(t.Symbol),
			Price:      price,
			BidPrice:   bidPrice,
			AskPrice:   askPrice,
			Volume24h:  volume,
			Timestamp:  time.Now(),
		})
	}

	return tickers, nil
}

// FetchAssetInfo fetches deposit/withdrawal status for assets
// Uses contract info to derive available assets since detailed deposit/withdraw info requires authentication
func (c *BitgetConnector) FetchAssetInfo(ctx context.Context) ([]connector.AssetInfo, error) {
	url := fmt.Sprintf("%s/api/v2/mix/market/contracts?productType=USDT-FUTURES", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch contracts: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code string `json:"code"`
		Data []struct {
			Symbol       string `json:"symbol"`
			BaseCoin     string `json:"baseCoin"`
			SymbolStatus string `json:"symbolStatus"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Code != "00000" {
		return nil, fmt.Errorf("Bitget API returned code: %s", result.Code)
	}

	// Extract unique base assets from active contracts
	assetMap := make(map[string]*connector.AssetInfo)
	for _, s := range result.Data {
		asset := s.BaseCoin
		if asset == "" {
			continue
		}

		if _, exists := assetMap[asset]; !exists {
			// SymbolStatus "normal" indicates active trading
			isActive := s.SymbolStatus == "normal"

			assetMap[asset] = &connector.AssetInfo{
				ExchangeID:      connector.Bitget,
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

	log.Debug().Int("count", len(assets)).Msg("Fetched Bitget asset info from contracts")
	return assets, nil
}
