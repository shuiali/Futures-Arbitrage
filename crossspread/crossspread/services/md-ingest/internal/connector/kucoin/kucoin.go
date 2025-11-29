package kucoin

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
	restBaseURL = "https://api-futures.kucoin.com"
)

// KuCoinConnector implements the Connector interface for KuCoin Futures
type KuCoinConnector struct {
	*connector.BaseConnector
	conn          *websocket.Conn
	subscriptions map[string]bool
	mu            sync.RWMutex
	done          chan struct{}
	wsEndpoint    string
	pingInterval  time.Duration
	token         string
}

// NewKuCoinConnector creates a new KuCoin connector
func NewKuCoinConnector(symbols []string, depthLevels int) *KuCoinConnector {
	config := connector.ConnectorConfig{
		ExchangeID:     connector.KuCoin,
		RestURL:        restBaseURL,
		Symbols:        symbols,
		DepthLevels:    depthLevels,
		ReconnectDelay: 5 * time.Second,
		PingInterval:   30 * time.Second,
	}

	return &KuCoinConnector{
		BaseConnector: connector.NewBaseConnector(config),
		subscriptions: make(map[string]bool),
		done:          make(chan struct{}),
		pingInterval:  30 * time.Second,
	}
}

// Connect establishes WebSocket connection to KuCoin
func (c *KuCoinConnector) Connect(ctx context.Context) error {
	// First get WebSocket token
	if err := c.getWsToken(ctx); err != nil {
		return fmt.Errorf("failed to get WS token: %w", err)
	}

	log.Info().Str("endpoint", c.wsEndpoint).Msg("Connecting to KuCoin WebSocket")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	url := fmt.Sprintf("%s?token=%s", c.wsEndpoint, c.token)
	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.conn = conn
	c.SetConnected(true)
	log.Info().Msg("Connected to KuCoin WebSocket")

	// Subscribe to orderbook updates
	for symbol := range c.subscriptions {
		if err := c.subscribeSymbol(symbol); err != nil {
			log.Error().Err(err).Str("symbol", symbol).Msg("Failed to subscribe")
		}
	}

	// Start reading messages
	go c.readLoop()
	go c.pingLoop()

	return nil
}

// getWsToken gets the WebSocket connection token from REST API
func (c *KuCoinConnector) getWsToken(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v1/bullet-public", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Code string `json:"code"`
		Data struct {
			Token           string `json:"token"`
			InstanceServers []struct {
				Endpoint     string `json:"endpoint"`
				PingInterval int    `json:"pingInterval"`
			} `json:"instanceServers"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if result.Code != "200000" || len(result.Data.InstanceServers) == 0 {
		return fmt.Errorf("invalid response: %s", result.Code)
	}

	c.token = result.Data.Token
	c.wsEndpoint = result.Data.InstanceServers[0].Endpoint
	c.pingInterval = time.Duration(result.Data.InstanceServers[0].PingInterval) * time.Millisecond

	return nil
}

// subscribeSymbol sends subscription message for a symbol
func (c *KuCoinConnector) subscribeSymbol(symbol string) error {
	msg := map[string]interface{}{
		"id":             time.Now().UnixNano(),
		"type":           "subscribe",
		"topic":          fmt.Sprintf("/contractMarket/level2Depth50:%s", symbol),
		"privateChannel": false,
		"response":       true,
	}

	return c.conn.WriteJSON(msg)
}

// Disconnect closes the WebSocket connection
func (c *KuCoinConnector) Disconnect() error {
	close(c.done)
	c.SetConnected(false)
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Subscribe adds symbol subscriptions
func (c *KuCoinConnector) Subscribe(symbols []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, s := range symbols {
		c.subscriptions[s] = true
	}
	return nil
}

// Unsubscribe removes symbol subscriptions
func (c *KuCoinConnector) Unsubscribe(symbols []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, s := range symbols {
		delete(c.subscriptions, s)
	}
	return nil
}

// FetchInstruments fetches all USDT perpetual futures
func (c *KuCoinConnector) FetchInstruments(ctx context.Context) ([]connector.Instrument, error) {
	url := fmt.Sprintf("%s/api/v1/contracts/active", restBaseURL)

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
			Symbol         string      `json:"symbol"`
			BaseCurrency   string      `json:"baseCurrency"`
			QuoteCurrency  string      `json:"quoteCurrency"`
			TickSize       float64     `json:"tickSize"`
			LotSize        float64     `json:"lotSize"`
			Multiplier     float64     `json:"multiplier"`
			IsInverse      bool        `json:"isInverse"`
			Status         string      `json:"status"`
			TakerFeeRate   json.Number `json:"takerFeeRate"`
			MakerFeeRate   json.Number `json:"makerFeeRate"`
			FundingFeeRate json.Number `json:"fundingFeeRate"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var instruments []connector.Instrument
	for _, s := range result.Data {
		if s.Status != "Open" || s.IsInverse {
			continue
		}

		takerFee, _ := s.TakerFeeRate.Float64()
		makerFee, _ := s.MakerFeeRate.Float64()

		inst := connector.Instrument{
			ExchangeID:     connector.KuCoin,
			Symbol:         s.Symbol,
			Canonical:      fmt.Sprintf("%s-%s-PERP", s.BaseCurrency, s.QuoteCurrency),
			BaseAsset:      s.BaseCurrency,
			QuoteAsset:     s.QuoteCurrency,
			InstrumentType: "perpetual",
			TickSize:       s.TickSize,
			LotSize:        s.LotSize,
			ContractSize:   s.Multiplier,
			TakerFee:       takerFee,
			MakerFee:       makerFee,
		}

		instruments = append(instruments, inst)
	}

	return instruments, nil
}

// FetchOrderbookSnapshot fetches orderbook via REST API
func (c *KuCoinConnector) FetchOrderbookSnapshot(ctx context.Context, symbol string, depth int) (*connector.Orderbook, error) {
	url := fmt.Sprintf("%s/api/v1/level2/depth%d?symbol=%s", restBaseURL, depth, symbol)

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
			Sequence int64      `json:"sequence"`
			Bids     [][]string `json:"bids"`
			Asks     [][]string `json:"asks"`
			Ts       int64      `json:"ts"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	ob := &connector.Orderbook{
		ExchangeID: connector.KuCoin,
		Symbol:     symbol,
		Timestamp:  time.UnixMilli(result.Data.Ts),
		SequenceID: result.Data.Sequence,
		IsSnapshot: true,
	}

	ob.Bids = parseLevels(result.Data.Bids)
	ob.Asks = parseLevels(result.Data.Asks)

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
func (c *KuCoinConnector) FetchFundingRates(ctx context.Context) ([]connector.FundingRate, error) {
	url := fmt.Sprintf("%s/api/v1/contracts/active", restBaseURL)

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
			Symbol              string      `json:"symbol"`
			FundingFeeRate      json.Number `json:"fundingFeeRate"` // Can be string or number
			NextFundingRateTime int64       `json:"nextFundingRateTime"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var rates []connector.FundingRate
	for _, d := range result.Data {
		rate, _ := d.FundingFeeRate.Float64()
		rates = append(rates, connector.FundingRate{
			ExchangeID:           connector.KuCoin,
			Symbol:               d.Symbol,
			FundingRate:          rate,
			NextFundingTime:      time.UnixMilli(d.NextFundingRateTime),
			FundingIntervalHours: 8,
			Timestamp:            time.Now(),
		})
	}

	return rates, nil
}

// readLoop reads messages from WebSocket
func (c *KuCoinConnector) readLoop() {
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

// pingLoop sends ping messages periodically
func (c *KuCoinConnector) pingLoop() {
	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			msg := map[string]interface{}{
				"id":   time.Now().UnixNano(),
				"type": "ping",
			}
			if err := c.conn.WriteJSON(msg); err != nil {
				log.Error().Err(err).Msg("Failed to send ping")
			}
		}
	}
}

// handleMessage processes incoming WebSocket messages
func (c *KuCoinConnector) handleMessage(message []byte) {
	var msg struct {
		Type    string `json:"type"`
		Topic   string `json:"topic"`
		Subject string `json:"subject"`
		Data    struct {
			Sequence  int64   `json:"sequence"`
			Bids      [][]any `json:"bids"`
			Asks      [][]any `json:"asks"`
			Timestamp int64   `json:"timestamp"`
		} `json:"data"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		return
	}

	if msg.Type != "message" || msg.Subject != "level2" {
		return
	}

	// Extract symbol from topic: /contractMarket/level2Depth50:XBTUSDTM
	symbol := ""
	if len(msg.Topic) > 0 {
		parts := splitTopic(msg.Topic)
		if len(parts) > 0 {
			symbol = parts[len(parts)-1]
		}
	}

	ob := &connector.Orderbook{
		ExchangeID: connector.KuCoin,
		Symbol:     symbol,
		Canonical:  extractCanonical(symbol),
		Timestamp:  time.UnixMilli(msg.Data.Timestamp),
		SequenceID: msg.Data.Sequence,
		IsSnapshot: true,
	}

	ob.Bids = parseLevelsAny(msg.Data.Bids)
	ob.Asks = parseLevelsAny(msg.Data.Asks)

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

func splitTopic(topic string) []string {
	var parts []string
	current := ""
	for _, ch := range topic {
		if ch == ':' || ch == '/' {
			if current != "" {
				parts = append(parts, current)
			}
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

	sort.Slice(levels, func(i, j int) bool {
		return levels[i].Price > levels[j].Price
	})

	return levels
}

func parseLevelsAny(data [][]any) []connector.PriceLevel {
	levels := make([]connector.PriceLevel, 0, len(data))
	for _, item := range data {
		if len(item) < 2 {
			continue
		}
		price := toFloat64(item[0])
		qty := toFloat64(item[1])
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

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

// extractCanonical extracts base asset from KuCoin symbol (XBTUSDTM -> XBT)
func extractCanonical(symbol string) string {
	// KuCoin uses format like XBTUSDTM, ETHUSDTM
	suffixes := []string{"USDTM", "USDCM", "USDM"}
	for _, suffix := range suffixes {
		if len(symbol) > len(suffix) && symbol[len(symbol)-len(suffix):] == suffix {
			base := symbol[:len(symbol)-len(suffix)]
			// Convert XBT to BTC
			if base == "XBT" {
				return "BTC"
			}
			return base
		}
	}
	return symbol
}

// ConnectForSymbols establishes WebSocket connection for specific symbols only
func (c *KuCoinConnector) ConnectForSymbols(ctx context.Context, symbols []string) error {
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
func (c *KuCoinConnector) FetchPriceTickers(ctx context.Context) ([]connector.PriceTicker, error) {
	url := fmt.Sprintf("%s/api/v1/allTickers", restBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// KuCoin futures returns data as direct array: {"code":"200000","data":[...]}
	var result struct {
		Code string `json:"code"`
		Data []struct {
			Symbol       string `json:"symbol"`
			Price        string `json:"price"`
			BestBidPrice string `json:"bestBidPrice"`
			BestAskPrice string `json:"bestAskPrice"`
			Size         int    `json:"size"`
			Ts           int64  `json:"ts"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var tickers []connector.PriceTicker
	for _, t := range result.Data {
		price, _ := strconv.ParseFloat(t.Price, 64)
		bidPrice, _ := strconv.ParseFloat(t.BestBidPrice, 64)
		askPrice, _ := strconv.ParseFloat(t.BestAskPrice, 64)

		tickers = append(tickers, connector.PriceTicker{
			ExchangeID: connector.KuCoin,
			Symbol:     t.Symbol,
			Canonical:  extractCanonical(t.Symbol),
			Price:      price,
			BidPrice:   bidPrice,
			AskPrice:   askPrice,
			Volume24h:  float64(t.Size),
			Timestamp:  time.Now(),
		})
	}

	return tickers, nil
}

// FetchAssetInfo fetches deposit/withdrawal status for assets
// Derives asset info from active futures contracts since detailed deposit/withdraw info requires auth
func (c *KuCoinConnector) FetchAssetInfo(ctx context.Context) ([]connector.AssetInfo, error) {
	url := fmt.Sprintf("%s/api/v1/contracts/active", restBaseURL)

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

	var result struct {
		Code string `json:"code"`
		Data []struct {
			Symbol         string `json:"symbol"`
			BaseCurrency   string `json:"baseCurrency"`
			QuoteCurrency  string `json:"quoteCurrency"`
			SettleCurrency string `json:"settleCurrency"`
			Status         string `json:"status"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Create map of unique base assets from active contracts
	assetMap := make(map[string]bool)
	for _, contract := range result.Data {
		if contract.BaseCurrency != "" {
			assetMap[contract.BaseCurrency] = true
		}
	}

	// Build asset info - futures contracts imply deposit/withdrawal available
	var assets []connector.AssetInfo
	for asset := range assetMap {
		assets = append(assets, connector.AssetInfo{
			ExchangeID:      connector.KuCoin,
			Asset:           asset,
			DepositEnabled:  true, // Assume enabled if contract is active
			WithdrawEnabled: true,
			Timestamp:       time.Now(),
		})
	}

	log.Info().Int("count", len(assets)).Msg("Fetched KuCoin asset info")
	return assets, nil
}
