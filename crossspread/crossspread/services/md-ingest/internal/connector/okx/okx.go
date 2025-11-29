package okx

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
)

const (
	okxWsURL   = "wss://ws.okx.com:8443/ws/v5/public"
	okxRestURL = "https://www.okx.com"
)

// OKXConnector implements the Connector interface for OKX
type OKXConnector struct {
	*connector.BaseConnector
	conn       *websocket.Conn
	symbols    []string
	depth      int
	mu         sync.RWMutex
	orderbooks map[string]*connector.Orderbook
	done       chan struct{}
}

// NewOKXConnector creates a new OKX connector
func NewOKXConnector(symbols []string, depth int) *OKXConnector {
	config := connector.ConnectorConfig{
		ExchangeID:     connector.OKX,
		WsURL:          okxWsURL,
		RestURL:        okxRestURL,
		Symbols:        symbols,
		DepthLevels:    depth,
		ReconnectDelay: 5 * time.Second,
		PingInterval:   25 * time.Second,
	}

	return &OKXConnector{
		BaseConnector: connector.NewBaseConnector(config),
		symbols:       symbols,
		depth:         depth,
		orderbooks:    make(map[string]*connector.Orderbook),
		done:          make(chan struct{}),
	}
}

// Connect establishes WebSocket connection to OKX
func (c *OKXConnector) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, okxWsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to OKX WebSocket: %w", err)
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
func (c *OKXConnector) ConnectForSymbols(ctx context.Context, symbols []string) error {
	c.mu.Lock()
	c.symbols = symbols
	c.mu.Unlock()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, okxWsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to OKX WebSocket: %w", err)
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

	return nil
}

// Disconnect closes the WebSocket connection
func (c *OKXConnector) Disconnect() error {
	close(c.done)
	c.SetConnected(false)
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Subscribe subscribes to orderbook updates for symbols
func (c *OKXConnector) Subscribe(symbols []string) error {
	args := make([]map[string]string, 0, len(symbols))
	for _, symbol := range symbols {
		// OKX uses format: BTC-USDT-SWAP for perpetuals
		instId := c.toOKXSymbol(symbol)
		args = append(args, map[string]string{
			"channel": "books5", // Top 5 levels, fast updates
			"instId":  instId,
		})
	}

	msg := map[string]interface{}{
		"op":   "subscribe",
		"args": args,
	}

	return c.conn.WriteJSON(msg)
}

// Unsubscribe removes subscriptions
func (c *OKXConnector) Unsubscribe(symbols []string) error {
	args := make([]map[string]string, 0, len(symbols))
	for _, symbol := range symbols {
		instId := c.toOKXSymbol(symbol)
		args = append(args, map[string]string{
			"channel": "books5",
			"instId":  instId,
		})
	}

	msg := map[string]interface{}{
		"op":   "unsubscribe",
		"args": args,
	}

	return c.conn.WriteJSON(msg)
}

// toOKXSymbol converts BTCUSDT to BTC-USDT-SWAP
func (c *OKXConnector) toOKXSymbol(symbol string) string {
	base := strings.TrimSuffix(symbol, "USDT")
	return fmt.Sprintf("%s-USDT-SWAP", base)
}

// fromOKXSymbol converts BTC-USDT-SWAP to BTCUSDT
func (c *OKXConnector) fromOKXSymbol(instId string) string {
	parts := strings.Split(instId, "-")
	if len(parts) >= 2 {
		return parts[0] + parts[1]
	}
	return instId
}

// FetchInstruments fetches all available instruments
func (c *OKXConnector) FetchInstruments(ctx context.Context) ([]connector.Instrument, error) {
	url := fmt.Sprintf("%s/api/v5/public/instruments?instType=SWAP", okxRestURL)

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
			InstId   string `json:"instId"`
			BaseCcy  string `json:"baseCcy"`
			QuoteCcy string `json:"quoteCcy"`
			CtVal    string `json:"ctVal"`
			TickSz   string `json:"tickSz"`
			LotSz    string `json:"lotSz"`
			MinSz    string `json:"minSz"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	instruments := make([]connector.Instrument, 0, len(result.Data))
	for _, item := range result.Data {
		// Only include USDT-margined perps
		if !strings.HasSuffix(item.InstId, "-USDT-SWAP") {
			continue
		}

		tickSize, _ := strconv.ParseFloat(item.TickSz, 64)
		lotSize, _ := strconv.ParseFloat(item.LotSz, 64)
		ctVal, _ := strconv.ParseFloat(item.CtVal, 64)

		instruments = append(instruments, connector.Instrument{
			ExchangeID:     connector.OKX,
			Symbol:         c.fromOKXSymbol(item.InstId),
			Canonical:      strings.ToUpper(item.BaseCcy),
			BaseAsset:      item.BaseCcy,
			QuoteAsset:     item.QuoteCcy,
			InstrumentType: "perpetual",
			ContractSize:   ctVal,
			TickSize:       tickSize,
			LotSize:        lotSize,
			MakerFee:       0.0002, // 0.02%
			TakerFee:       0.0005, // 0.05%
		})
	}

	return instruments, nil
}

// FetchOrderbookSnapshot fetches current orderbook via REST
func (c *OKXConnector) FetchOrderbookSnapshot(ctx context.Context, symbol string, depth int) (*connector.Orderbook, error) {
	instId := c.toOKXSymbol(symbol)
	url := fmt.Sprintf("%s/api/v5/market/books?instId=%s&sz=%d", okxRestURL, instId, depth)

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
			Bids [][]string `json:"bids"`
			Asks [][]string `json:"asks"`
			Ts   string     `json:"ts"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no orderbook data returned")
	}

	data := result.Data[0]
	ts, _ := strconv.ParseInt(data.Ts, 10, 64)

	ob := &connector.Orderbook{
		ExchangeID: connector.OKX,
		Symbol:     symbol,
		Canonical:  strings.TrimSuffix(symbol, "USDT"),
		Bids:       make([]connector.PriceLevel, 0, len(data.Bids)),
		Asks:       make([]connector.PriceLevel, 0, len(data.Asks)),
		Timestamp:  time.UnixMilli(ts),
		IsSnapshot: true,
	}

	for _, bid := range data.Bids {
		price, _ := strconv.ParseFloat(bid[0], 64)
		qty, _ := strconv.ParseFloat(bid[1], 64)
		ob.Bids = append(ob.Bids, connector.PriceLevel{Price: price, Quantity: qty})
	}

	for _, ask := range data.Asks {
		price, _ := strconv.ParseFloat(ask[0], 64)
		qty, _ := strconv.ParseFloat(ask[1], 64)
		ob.Asks = append(ob.Asks, connector.PriceLevel{Price: price, Quantity: qty})
	}

	c.updateSpread(ob)
	return ob, nil
}

// FetchFundingRates fetches current funding rates
func (c *OKXConnector) FetchFundingRates(ctx context.Context) ([]connector.FundingRate, error) {
	url := fmt.Sprintf("%s/api/v5/public/funding-rate?instType=SWAP", okxRestURL)

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
			InstId          string `json:"instId"`
			FundingRate     string `json:"fundingRate"`
			NextFundingTime string `json:"nextFundingTime"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	rates := make([]connector.FundingRate, 0, len(result.Data))
	for _, item := range result.Data {
		if !strings.HasSuffix(item.InstId, "-USDT-SWAP") {
			continue
		}

		rate, _ := strconv.ParseFloat(item.FundingRate, 64)
		nextTime, _ := strconv.ParseInt(item.NextFundingTime, 10, 64)

		rates = append(rates, connector.FundingRate{
			ExchangeID:           connector.OKX,
			Symbol:               c.fromOKXSymbol(item.InstId),
			Canonical:            strings.Split(item.InstId, "-")[0],
			FundingRate:          rate,
			NextFundingTime:      time.UnixMilli(nextTime),
			FundingIntervalHours: 8,
			Timestamp:            time.Now(),
		})
	}

	return rates, nil
}

func (c *OKXConnector) readMessages() {
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

func (c *OKXConnector) processMessage(data []byte) {
	var msg struct {
		Event string `json:"event"`
		Arg   struct {
			Channel string `json:"channel"`
			InstId  string `json:"instId"`
		} `json:"arg"`
		Data []struct {
			Bids [][]string `json:"bids"`
			Asks [][]string `json:"asks"`
			Ts   string     `json:"ts"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	// Handle orderbook data
	if len(msg.Data) > 0 && msg.Arg.Channel == "books5" {
		c.processOrderbook(msg.Arg.InstId, msg.Data[0])
	}
}

func (c *OKXConnector) processOrderbook(instId string, data struct {
	Bids [][]string `json:"bids"`
	Asks [][]string `json:"asks"`
	Ts   string     `json:"ts"`
}) {
	symbol := c.fromOKXSymbol(instId)
	ts, _ := strconv.ParseInt(data.Ts, 10, 64)

	ob := &connector.Orderbook{
		ExchangeID: connector.OKX,
		Symbol:     symbol,
		Canonical:  strings.Split(instId, "-")[0],
		Bids:       make([]connector.PriceLevel, 0, len(data.Bids)),
		Asks:       make([]connector.PriceLevel, 0, len(data.Asks)),
		Timestamp:  time.UnixMilli(ts),
		IsSnapshot: true, // OKX books5 sends full snapshots
	}

	for _, bid := range data.Bids {
		price, _ := strconv.ParseFloat(bid[0], 64)
		qty, _ := strconv.ParseFloat(bid[1], 64)
		ob.Bids = append(ob.Bids, connector.PriceLevel{Price: price, Quantity: qty})
	}

	for _, ask := range data.Asks {
		price, _ := strconv.ParseFloat(ask[0], 64)
		qty, _ := strconv.ParseFloat(ask[1], 64)
		ob.Asks = append(ob.Asks, connector.PriceLevel{Price: price, Quantity: qty})
	}

	c.updateSpread(ob)

	c.mu.Lock()
	c.orderbooks[symbol] = ob
	c.mu.Unlock()

	c.EmitOrderbook(ob)
}

func (c *OKXConnector) updateSpread(ob *connector.Orderbook) {
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

func (c *OKXConnector) pingLoop() {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if err := c.conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
				c.EmitError(fmt.Errorf("ping error: %w", err))
			}
		}
	}
}

// FetchPriceTickers fetches current prices for all symbols via REST API
func (c *OKXConnector) FetchPriceTickers(ctx context.Context) ([]connector.PriceTicker, error) {
	url := fmt.Sprintf("%s/api/v5/market/tickers?instType=SWAP", okxRestURL)

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
			InstId string `json:"instId"`
			Last   string `json:"last"`
			BidPx  string `json:"bidPx"`
			AskPx  string `json:"askPx"`
			Vol24h string `json:"vol24h"`
			Ts     string `json:"ts"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Code != "0" {
		return nil, fmt.Errorf("API error: code %s", result.Code)
	}

	tickers := make([]connector.PriceTicker, 0, len(result.Data))
	for _, t := range result.Data {
		// Only include USDT perpetuals
		if !strings.Contains(t.InstId, "-USDT-SWAP") {
			continue
		}

		price, _ := strconv.ParseFloat(t.Last, 64)
		bidPrice, _ := strconv.ParseFloat(t.BidPx, 64)
		askPrice, _ := strconv.ParseFloat(t.AskPx, 64)
		volume, _ := strconv.ParseFloat(t.Vol24h, 64)
		ts, _ := strconv.ParseInt(t.Ts, 10, 64)

		if price <= 0 {
			continue
		}

		// Extract canonical from BTC-USDT-SWAP -> BTC
		parts := strings.Split(t.InstId, "-")
		canonical := parts[0]

		tickers = append(tickers, connector.PriceTicker{
			ExchangeID: connector.OKX,
			Symbol:     c.fromOKXSymbol(t.InstId),
			Canonical:  canonical,
			Price:      price,
			BidPrice:   bidPrice,
			AskPrice:   askPrice,
			Volume24h:  volume,
			Timestamp:  time.UnixMilli(ts),
		})
	}

	return tickers, nil
}

// FetchAssetInfo fetches deposit/withdrawal status for assets
func (c *OKXConnector) FetchAssetInfo(ctx context.Context) ([]connector.AssetInfo, error) {
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
			ExchangeID:      connector.OKX,
			Asset:           asset,
			DepositEnabled:  true,
			WithdrawEnabled: true,
			Timestamp:       time.Now(),
		})
	}

	return assetInfos, nil
}
