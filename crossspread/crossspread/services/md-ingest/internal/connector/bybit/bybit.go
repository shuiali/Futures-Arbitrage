package bybit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"crossspread-md-ingest/internal/connector"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	bybitWsURL   = "wss://stream.bybit.com/v5/public/linear"
	bybitRestURL = "https://api.bybit.com"
)

// BybitConnector implements the Connector interface for Bybit
type BybitConnector struct {
	*connector.BaseConnector
	conn       *websocket.Conn
	symbols    []string
	depth      int
	mu         sync.RWMutex
	orderbooks map[string]*connector.Orderbook
	done       chan struct{}
}

// NewBybitConnector creates a new Bybit connector
func NewBybitConnector(symbols []string, depth int) *BybitConnector {
	config := connector.ConnectorConfig{
		ExchangeID:     connector.Bybit,
		WsURL:          bybitWsURL,
		RestURL:        bybitRestURL,
		Symbols:        symbols,
		DepthLevels:    depth,
		ReconnectDelay: 5 * time.Second,
		PingInterval:   20 * time.Second,
	}

	return &BybitConnector{
		BaseConnector: connector.NewBaseConnector(config),
		symbols:       symbols,
		depth:         depth,
		orderbooks:    make(map[string]*connector.Orderbook),
		done:          make(chan struct{}),
	}
}

// Connect establishes WebSocket connection to Bybit
func (c *BybitConnector) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, bybitWsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Bybit WebSocket: %w", err)
	}

	c.conn = conn
	c.SetConnected(true)

	// Subscribe to orderbook streams
	if err := c.Subscribe(c.symbols); err != nil {
		return err
	}

	// Start message handler
	go c.readMessages()

	// Start ping handler
	go c.pingLoop()

	return nil
}

// ConnectForSymbols establishes WebSocket connection for specific symbols only
// Used for Phase 2 selective subscription after spread discovery
func (c *BybitConnector) ConnectForSymbols(ctx context.Context, symbols []string) error {
	c.mu.Lock()
	c.symbols = symbols
	c.mu.Unlock()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, bybitWsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Bybit WebSocket: %w", err)
	}

	c.conn = conn
	c.SetConnected(true)

	// Subscribe only to specified symbols
	if err := c.Subscribe(symbols); err != nil {
		return err
	}

	// Start message handler
	go c.readMessages()

	// Start ping handler
	go c.pingLoop()

	log.Info().
		Int("symbols", len(symbols)).
		Msg("Connected to Bybit WebSocket (selective)")

	return nil
}

// Disconnect closes the WebSocket connection
func (c *BybitConnector) Disconnect() error {
	close(c.done)
	c.SetConnected(false)
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Subscribe subscribes to orderbook updates for symbols
func (c *BybitConnector) Subscribe(symbols []string) error {
	args := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		// Bybit uses format: orderbook.50.BTCUSDT
		args = append(args, fmt.Sprintf("orderbook.%d.%s", c.depth, symbol))
	}

	msg := map[string]interface{}{
		"op":   "subscribe",
		"args": args,
	}

	return c.conn.WriteJSON(msg)
}

// Unsubscribe removes subscriptions
func (c *BybitConnector) Unsubscribe(symbols []string) error {
	args := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		args = append(args, fmt.Sprintf("orderbook.%d.%s", c.depth, symbol))
	}

	msg := map[string]interface{}{
		"op":   "unsubscribe",
		"args": args,
	}

	return c.conn.WriteJSON(msg)
}

// FetchInstruments fetches all available instruments
func (c *BybitConnector) FetchInstruments(ctx context.Context) ([]connector.Instrument, error) {
	url := fmt.Sprintf("%s/v5/market/instruments-info?category=linear", bybitRestURL)

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
		RetCode int `json:"retCode"`
		Result  struct {
			List []struct {
				Symbol       string `json:"symbol"`
				BaseCoin     string `json:"baseCoin"`
				QuoteCoin    string `json:"quoteCoin"`
				ContractType string `json:"contractType"`
				PriceFilter  struct {
					TickSize string `json:"tickSize"`
				} `json:"priceFilter"`
				LotSizeFilter struct {
					QtyStep     string `json:"qtyStep"`
					MinOrderQty string `json:"minOrderQty"`
				} `json:"lotSizeFilter"`
			} `json:"list"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	instruments := make([]connector.Instrument, 0, len(result.Result.List))
	for _, item := range result.Result.List {
		tickSize, _ := strconv.ParseFloat(item.PriceFilter.TickSize, 64)
		lotSize, _ := strconv.ParseFloat(item.LotSizeFilter.QtyStep, 64)
		minQty, _ := strconv.ParseFloat(item.LotSizeFilter.MinOrderQty, 64)

		instruments = append(instruments, connector.Instrument{
			ExchangeID:     connector.Bybit,
			Symbol:         item.Symbol,
			Canonical:      normalizeSymbol(item.BaseCoin),
			BaseAsset:      item.BaseCoin,
			QuoteAsset:     item.QuoteCoin,
			InstrumentType: "perpetual",
			TickSize:       tickSize,
			LotSize:        lotSize,
			MinNotional:    minQty * tickSize,
			MakerFee:       0.0001, // 0.01%
			TakerFee:       0.0006, // 0.06%
		})
	}

	return instruments, nil
}

// FetchOrderbookSnapshot fetches current orderbook via REST
func (c *BybitConnector) FetchOrderbookSnapshot(ctx context.Context, symbol string, depth int) (*connector.Orderbook, error) {
	url := fmt.Sprintf("%s/v5/market/orderbook?category=linear&symbol=%s&limit=%d", bybitRestURL, symbol, depth)

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
		RetCode int `json:"retCode"`
		Result  struct {
			Symbol string     `json:"s"`
			Bids   [][]string `json:"b"`
			Asks   [][]string `json:"a"`
			Ts     int64      `json:"ts"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	ob := &connector.Orderbook{
		ExchangeID: connector.Bybit,
		Symbol:     symbol,
		Canonical:  normalizeSymbol(strings.TrimSuffix(symbol, "USDT")),
		Bids:       make([]connector.PriceLevel, 0, len(result.Result.Bids)),
		Asks:       make([]connector.PriceLevel, 0, len(result.Result.Asks)),
		Timestamp:  time.UnixMilli(result.Result.Ts),
		IsSnapshot: true,
	}

	for _, bid := range result.Result.Bids {
		price, _ := strconv.ParseFloat(bid[0], 64)
		qty, _ := strconv.ParseFloat(bid[1], 64)
		ob.Bids = append(ob.Bids, connector.PriceLevel{Price: price, Quantity: qty})
	}

	for _, ask := range result.Result.Asks {
		price, _ := strconv.ParseFloat(ask[0], 64)
		qty, _ := strconv.ParseFloat(ask[1], 64)
		ob.Asks = append(ob.Asks, connector.PriceLevel{Price: price, Quantity: qty})
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
func (c *BybitConnector) FetchFundingRates(ctx context.Context) ([]connector.FundingRate, error) {
	url := fmt.Sprintf("%s/v5/market/tickers?category=linear", bybitRestURL)

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
		RetCode int `json:"retCode"`
		Result  struct {
			List []struct {
				Symbol          string `json:"symbol"`
				FundingRate     string `json:"fundingRate"`
				NextFundingTime string `json:"nextFundingTime"`
			} `json:"list"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	rates := make([]connector.FundingRate, 0, len(result.Result.List))
	for _, item := range result.Result.List {
		rate, _ := strconv.ParseFloat(item.FundingRate, 64)
		nextTime, _ := strconv.ParseInt(item.NextFundingTime, 10, 64)

		rates = append(rates, connector.FundingRate{
			ExchangeID:           connector.Bybit,
			Symbol:               item.Symbol,
			Canonical:            normalizeSymbol(strings.TrimSuffix(item.Symbol, "USDT")),
			FundingRate:          rate,
			NextFundingTime:      time.UnixMilli(nextTime),
			FundingIntervalHours: 8,
			Timestamp:            time.Now(),
		})
	}

	return rates, nil
}

func (c *BybitConnector) readMessages() {
	for {
		select {
		case <-c.done:
			return
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				c.EmitError(fmt.Errorf("read error: %w", err))
				c.SetConnected(false)
				return
			}

			c.processMessage(message)
		}
	}
}

func (c *BybitConnector) processMessage(data []byte) {
	var msg struct {
		Topic string          `json:"topic"`
		Type  string          `json:"type"`
		Data  json.RawMessage `json:"data"`
		Ts    int64           `json:"ts"`
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	// Handle orderbook messages
	if strings.HasPrefix(msg.Topic, "orderbook.") {
		c.processOrderbook(msg.Topic, msg.Type, msg.Data, msg.Ts)
	}
}

func (c *BybitConnector) processOrderbook(topic, msgType string, data json.RawMessage, ts int64) {
	// Extract symbol from topic: orderbook.50.BTCUSDT
	parts := strings.Split(topic, ".")
	if len(parts) < 3 {
		return
	}
	symbol := parts[2]

	var obData struct {
		Symbol string     `json:"s"`
		Bids   [][]string `json:"b"`
		Asks   [][]string `json:"a"`
		Seq    int64      `json:"seq"`
	}

	if err := json.Unmarshal(data, &obData); err != nil {
		log.Error().Err(err).Msg("Failed to parse orderbook data")
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	isSnapshot := msgType == "snapshot"

	if isSnapshot {
		// Create new orderbook
		ob := &connector.Orderbook{
			ExchangeID: connector.Bybit,
			Symbol:     symbol,
			Canonical:  normalizeSymbol(strings.TrimSuffix(symbol, "USDT")),
			Bids:       make([]connector.PriceLevel, 0, len(obData.Bids)),
			Asks:       make([]connector.PriceLevel, 0, len(obData.Asks)),
			Timestamp:  time.UnixMilli(ts),
			SequenceID: obData.Seq,
			IsSnapshot: true,
		}

		for _, bid := range obData.Bids {
			price, _ := strconv.ParseFloat(bid[0], 64)
			qty, _ := strconv.ParseFloat(bid[1], 64)
			ob.Bids = append(ob.Bids, connector.PriceLevel{Price: price, Quantity: qty})
		}

		for _, ask := range obData.Asks {
			price, _ := strconv.ParseFloat(ask[0], 64)
			qty, _ := strconv.ParseFloat(ask[1], 64)
			ob.Asks = append(ob.Asks, connector.PriceLevel{Price: price, Quantity: qty})
		}

		c.orderbooks[symbol] = ob
		c.updateSpread(ob)
		c.EmitOrderbook(ob)
	} else {
		// Apply delta update
		ob, exists := c.orderbooks[symbol]
		if !exists {
			return
		}

		// Update bids
		for _, bid := range obData.Bids {
			price, _ := strconv.ParseFloat(bid[0], 64)
			qty, _ := strconv.ParseFloat(bid[1], 64)
			c.updateLevel(&ob.Bids, price, qty, true)
		}

		// Update asks
		for _, ask := range obData.Asks {
			price, _ := strconv.ParseFloat(ask[0], 64)
			qty, _ := strconv.ParseFloat(ask[1], 64)
			c.updateLevel(&ob.Asks, price, qty, false)
		}

		ob.Timestamp = time.UnixMilli(ts)
		ob.SequenceID = obData.Seq
		ob.IsSnapshot = false

		c.updateSpread(ob)
		c.EmitOrderbook(ob)
	}
}

func (c *BybitConnector) updateLevel(levels *[]connector.PriceLevel, price, qty float64, isBid bool) {
	if qty == 0 {
		// Remove level
		for i, level := range *levels {
			if level.Price == price {
				*levels = append((*levels)[:i], (*levels)[i+1:]...)
				return
			}
		}
		return
	}

	// Update or insert level
	for i, level := range *levels {
		if level.Price == price {
			(*levels)[i].Quantity = qty
			return
		}
	}

	// Insert at correct position
	newLevel := connector.PriceLevel{Price: price, Quantity: qty}
	inserted := false
	for i, level := range *levels {
		if (isBid && price > level.Price) || (!isBid && price < level.Price) {
			*levels = append((*levels)[:i], append([]connector.PriceLevel{newLevel}, (*levels)[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		*levels = append(*levels, newLevel)
	}
}

func (c *BybitConnector) updateSpread(ob *connector.Orderbook) {
	if len(ob.Bids) > 0 {
		ob.BestBid = ob.Bids[0].Price
	}
	if len(ob.Asks) > 0 {
		ob.BestAsk = ob.Asks[0].Price
	}
	if ob.BestBid > 0 && ob.BestAsk > 0 {
		ob.SpreadBps = (ob.BestAsk - ob.BestBid) / ob.BestBid * 10000
	}
}

func (c *BybitConnector) pingLoop() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			ping := map[string]interface{}{
				"op": "ping",
			}
			if err := c.conn.WriteJSON(ping); err != nil {
				c.EmitError(fmt.Errorf("ping error: %w", err))
			}
		}
	}
}

func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSuffix(symbol, "USDT"))
}

// FetchPriceTickers fetches current prices for all symbols via REST API
func (c *BybitConnector) FetchPriceTickers(ctx context.Context) ([]connector.PriceTicker, error) {
	url := fmt.Sprintf("%s/v5/market/tickers?category=linear", bybitRestURL)

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
		RetCode int `json:"retCode"`
		Result  struct {
			List []struct {
				Symbol    string `json:"symbol"`
				LastPrice string `json:"lastPrice"`
				Bid1Price string `json:"bid1Price"`
				Ask1Price string `json:"ask1Price"`
				Volume24h string `json:"volume24h"`
				UpdatedAt string `json:"updatedTime"`
			} `json:"list"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.RetCode != 0 {
		return nil, fmt.Errorf("API error: code %d", result.RetCode)
	}

	tickers := make([]connector.PriceTicker, 0, len(result.Result.List))
	for _, t := range result.Result.List {
		// Only include USDT perpetuals
		if !strings.HasSuffix(t.Symbol, "USDT") {
			continue
		}

		price, _ := strconv.ParseFloat(t.LastPrice, 64)
		bidPrice, _ := strconv.ParseFloat(t.Bid1Price, 64)
		askPrice, _ := strconv.ParseFloat(t.Ask1Price, 64)
		volume, _ := strconv.ParseFloat(t.Volume24h, 64)
		updatedAt, _ := strconv.ParseInt(t.UpdatedAt, 10, 64)

		if price <= 0 {
			continue
		}

		canonical := normalizeSymbol(t.Symbol)
		tickers = append(tickers, connector.PriceTicker{
			ExchangeID: connector.Bybit,
			Symbol:     t.Symbol,
			Canonical:  canonical,
			Price:      price,
			BidPrice:   bidPrice,
			AskPrice:   askPrice,
			Volume24h:  volume,
			Timestamp:  time.UnixMilli(updatedAt),
		})
	}

	log.Info().Int("count", len(tickers)).Msg("Fetched Bybit price tickers")
	return tickers, nil
}

// FetchAssetInfo fetches deposit/withdrawal status for assets
func (c *BybitConnector) FetchAssetInfo(ctx context.Context) ([]connector.AssetInfo, error) {
	// For futures, we derive asset info from instruments
	instruments, err := c.FetchInstruments(ctx)
	if err != nil {
		return nil, err
	}

	assetMap := make(map[string]bool)
	for _, inst := range instruments {
		assetMap[inst.BaseAsset] = true
	}

	assetInfos := make([]connector.AssetInfo, 0, len(assetMap))
	for asset := range assetMap {
		assetInfos = append(assetInfos, connector.AssetInfo{
			ExchangeID:      connector.Bybit,
			Asset:           asset,
			DepositEnabled:  true,
			WithdrawEnabled: true,
			Timestamp:       time.Now(),
		})
	}

	log.Info().Int("count", len(assetInfos)).Msg("Fetched Bybit asset info")
	return assetInfos, nil
}
