package gateio

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
	wsBaseURL   = "wss://fx-ws.gateio.ws/v4/ws/usdt"
	restBaseURL = "https://api.gateio.ws"
)

// GateIOConnector implements the Connector interface for Gate.io Futures
type GateIOConnector struct {
	*connector.BaseConnector
	conn          *websocket.Conn
	subscriptions map[string]bool
	mu            sync.RWMutex
	done          chan struct{}
}

// NewGateIOConnector creates a new Gate.io connector
func NewGateIOConnector(symbols []string, depthLevels int) *GateIOConnector {
	config := connector.ConnectorConfig{
		ExchangeID:     connector.GateIO,
		WsURL:          wsBaseURL,
		RestURL:        restBaseURL,
		Symbols:        symbols,
		DepthLevels:    depthLevels,
		ReconnectDelay: 5 * time.Second,
		PingInterval:   20 * time.Second,
	}

	c := &GateIOConnector{
		BaseConnector: connector.NewBaseConnector(config),
		subscriptions: make(map[string]bool),
		done:          make(chan struct{}),
	}

	for _, s := range symbols {
		c.subscriptions[s] = true
	}

	return c
}

// Connect establishes WebSocket connection to Gate.io
func (c *GateIOConnector) Connect(ctx context.Context) error {
	log.Info().Str("url", wsBaseURL).Msg("Connecting to Gate.io WebSocket")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, wsBaseURL, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.conn = conn
	c.SetConnected(true)
	log.Info().Msg("Connected to Gate.io WebSocket")

	// Subscribe to orderbook updates
	for symbol := range c.subscriptions {
		if err := c.subscribeSymbol(symbol); err != nil {
			log.Error().Err(err).Str("symbol", symbol).Msg("Failed to subscribe")
		}
	}

	go c.readLoop()
	go c.pingLoop()

	return nil
}

func (c *GateIOConnector) subscribeSymbol(symbol string) error {
	// Gate.io uses order_book_update for incremental updates
	// Payload: [contract, update_frequency, depth]
	msg := map[string]interface{}{
		"time":    time.Now().Unix(),
		"channel": "futures.order_book_update",
		"event":   "subscribe",
		"payload": []string{symbol, "100ms", "20"},
	}
	return c.conn.WriteJSON(msg)
}

// Disconnect closes the WebSocket connection
func (c *GateIOConnector) Disconnect() error {
	close(c.done)
	c.SetConnected(false)
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Subscribe adds symbol subscriptions
func (c *GateIOConnector) Subscribe(symbols []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range symbols {
		c.subscriptions[s] = true
	}
	return nil
}

// Unsubscribe removes symbol subscriptions
func (c *GateIOConnector) Unsubscribe(symbols []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range symbols {
		delete(c.subscriptions, s)
	}
	return nil
}

// FetchInstruments fetches all USDT perpetual futures
func (c *GateIOConnector) FetchInstruments(ctx context.Context) ([]connector.Instrument, error) {
	url := fmt.Sprintf("%s/api/v4/futures/usdt/contracts", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var contracts []struct {
		Name            string  `json:"name"`
		Underlying      string  `json:"underlying"`
		Quanto          string  `json:"quanto_multiplier"`
		MarkPrice       string  `json:"mark_price"`
		TakerFeeRate    string  `json:"taker_fee_rate"`
		MakerFeeRate    string  `json:"maker_fee_rate"`
		OrderPriceRound string  `json:"order_price_round"`
		OrderSizeMin    float64 `json:"order_size_min"`
		InDelisting     bool    `json:"in_delisting"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&contracts); err != nil {
		return nil, err
	}

	var instruments []connector.Instrument
	for _, s := range contracts {
		if s.InDelisting {
			continue
		}

		takerFee, _ := strconv.ParseFloat(s.TakerFeeRate, 64)
		makerFee, _ := strconv.ParseFloat(s.MakerFeeRate, 64)
		tickSize, _ := strconv.ParseFloat(s.OrderPriceRound, 64)
		multiplier, _ := strconv.ParseFloat(s.Quanto, 64)

		// Parse base/quote from name (e.g., BTC_USDT)
		base := s.Underlying
		quote := "USDT"

		inst := connector.Instrument{
			ExchangeID:     connector.GateIO,
			Symbol:         s.Name,
			Canonical:      fmt.Sprintf("%s-%s-PERP", base, quote),
			BaseAsset:      base,
			QuoteAsset:     quote,
			InstrumentType: "perpetual",
			TickSize:       tickSize,
			LotSize:        1,
			ContractSize:   multiplier,
			TakerFee:       takerFee,
			MakerFee:       makerFee,
		}
		instruments = append(instruments, inst)
	}

	return instruments, nil
}

// FetchOrderbookSnapshot fetches orderbook via REST API
func (c *GateIOConnector) FetchOrderbookSnapshot(ctx context.Context, symbol string, depth int) (*connector.Orderbook, error) {
	url := fmt.Sprintf("%s/api/v4/futures/usdt/order_book?contract=%s&limit=%d", restBaseURL, symbol, depth)

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
		Current float64 `json:"current"`
		Bids    []struct {
			P string `json:"p"`
			S int64  `json:"s"`
		} `json:"bids"`
		Asks []struct {
			P string `json:"p"`
			S int64  `json:"s"`
		} `json:"asks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	ob := &connector.Orderbook{
		ExchangeID: connector.GateIO,
		Symbol:     symbol,
		Timestamp:  time.Now(),
		IsSnapshot: true,
	}

	for _, b := range result.Bids {
		price, _ := strconv.ParseFloat(b.P, 64)
		ob.Bids = append(ob.Bids, connector.PriceLevel{
			Price:    price,
			Quantity: float64(b.S),
		})
	}

	for _, a := range result.Asks {
		price, _ := strconv.ParseFloat(a.P, 64)
		ob.Asks = append(ob.Asks, connector.PriceLevel{
			Price:    price,
			Quantity: float64(a.S),
		})
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

	return ob, nil
}

// FetchFundingRates fetches current funding rates
func (c *GateIOConnector) FetchFundingRates(ctx context.Context) ([]connector.FundingRate, error) {
	url := fmt.Sprintf("%s/api/v4/futures/usdt/contracts", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var contracts []struct {
		Name             string `json:"name"`
		FundingRate      string `json:"funding_rate"`
		FundingNextApply int64  `json:"funding_next_apply"`
		FundingInterval  int    `json:"funding_interval"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&contracts); err != nil {
		return nil, err
	}

	var rates []connector.FundingRate
	for _, d := range contracts {
		rate, _ := strconv.ParseFloat(d.FundingRate, 64)
		rates = append(rates, connector.FundingRate{
			ExchangeID:           connector.GateIO,
			Symbol:               d.Name,
			FundingRate:          rate,
			NextFundingTime:      time.Unix(d.FundingNextApply, 0),
			FundingIntervalHours: d.FundingInterval / 3600,
			Timestamp:            time.Now(),
		})
	}

	return rates, nil
}

func (c *GateIOConnector) readLoop() {
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

func (c *GateIOConnector) pingLoop() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			msg := map[string]interface{}{
				"time":    time.Now().Unix(),
				"channel": "futures.ping",
			}
			if err := c.conn.WriteJSON(msg); err != nil {
				log.Error().Err(err).Msg("Failed to send ping")
			}
		}
	}
}

func (c *GateIOConnector) handleMessage(message []byte) {
	var msg struct {
		Channel string `json:"channel"`
		Event   string `json:"event"`
		Result  struct {
			T int64  `json:"t"` // timestamp
			S string `json:"s"` // contract name
			U int64  `json:"u"` // update ID
			B []struct {
				P string `json:"p"`
				S int64  `json:"s"`
			} `json:"b"` // bids
			A []struct {
				P string `json:"p"`
				S int64  `json:"s"`
			} `json:"a"` // asks
		} `json:"result"`
		Time int64 `json:"time"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		return
	}

	// Handle both order_book and order_book_update channels
	if msg.Channel != "futures.order_book_update" && msg.Channel != "futures.order_book" {
		return
	}
	if msg.Event != "update" && msg.Event != "all" {
		return
	}

	symbol := msg.Result.S
	if symbol == "" {
		return
	}

	ob := &connector.Orderbook{
		ExchangeID: connector.GateIO,
		Symbol:     symbol,
		Canonical:  extractCanonical(symbol),
		Timestamp:  time.UnixMilli(msg.Result.T),
		SequenceID: msg.Result.U,
		IsSnapshot: msg.Event == "all",
	}

	for _, b := range msg.Result.B {
		price, _ := strconv.ParseFloat(b.P, 64)
		if b.S > 0 { // Only add non-zero sizes
			ob.Bids = append(ob.Bids, connector.PriceLevel{
				Price:    price,
				Quantity: float64(b.S),
			})
		}
	}

	for _, a := range msg.Result.A {
		price, _ := strconv.ParseFloat(a.P, 64)
		if a.S > 0 { // Only add non-zero sizes
			ob.Asks = append(ob.Asks, connector.PriceLevel{
				Price:    price,
				Quantity: float64(a.S),
			})
		}
	}

	// Sort bids descending
	sort.Slice(ob.Bids, func(i, j int) bool {
		return ob.Bids[i].Price > ob.Bids[j].Price
	})
	// Sort asks ascending
	sort.Slice(ob.Asks, func(i, j int) bool {
		return ob.Asks[i].Price < ob.Asks[j].Price
	})

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

// extractCanonical extracts base asset from symbol (BTC_USDT -> BTC)
func extractCanonical(symbol string) string {
	// Gate.io uses underscore separator
	for i, ch := range symbol {
		if ch == '_' {
			return symbol[:i]
		}
	}
	return symbol
}

// ConnectForSymbols establishes WebSocket connection for specific symbols only
func (c *GateIOConnector) ConnectForSymbols(ctx context.Context, symbols []string) error {
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
func (c *GateIOConnector) FetchPriceTickers(ctx context.Context) ([]connector.PriceTicker, error) {
	url := fmt.Sprintf("%s/api/v4/futures/usdt/tickers", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result []struct {
		Contract   string `json:"contract"`
		Last       string `json:"last"`
		HighestBid string `json:"highest_bid"`
		LowestAsk  string `json:"lowest_ask"`
		Volume24h  string `json:"volume_24h_base"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var tickers []connector.PriceTicker
	for _, t := range result {
		price, _ := strconv.ParseFloat(t.Last, 64)
		bidPrice, _ := strconv.ParseFloat(t.HighestBid, 64)
		askPrice, _ := strconv.ParseFloat(t.LowestAsk, 64)
		volume, _ := strconv.ParseFloat(t.Volume24h, 64)

		tickers = append(tickers, connector.PriceTicker{
			ExchangeID: connector.GateIO,
			Symbol:     t.Contract,
			Canonical:  extractCanonical(t.Contract),
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
func (c *GateIOConnector) FetchAssetInfo(ctx context.Context) ([]connector.AssetInfo, error) {
	url := fmt.Sprintf("%s/api/v4/futures/usdt/contracts", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch contracts: %w", err)
	}
	defer resp.Body.Close()

	var contracts []struct {
		Name        string `json:"name"`
		Underlying  string `json:"underlying"`
		InDelisting bool   `json:"in_delisting"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&contracts); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract unique base assets from active contracts
	assetMap := make(map[string]*connector.AssetInfo)
	for _, s := range contracts {
		asset := s.Underlying
		if asset == "" {
			continue
		}

		if _, exists := assetMap[asset]; !exists {
			// InDelisting false means contract is active
			isActive := !s.InDelisting

			assetMap[asset] = &connector.AssetInfo{
				ExchangeID:      connector.GateIO,
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

	log.Debug().Int("count", len(assets)).Msg("Fetched GateIO asset info from contracts")
	return assets, nil
}
