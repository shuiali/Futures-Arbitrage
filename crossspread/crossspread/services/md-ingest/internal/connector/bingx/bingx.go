package bingx

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
	wsBaseURL   = "wss://open-api-swap.bingx.com/swap-market"
	restBaseURL = "https://open-api.bingx.com"
)

// BingXConnector implements the Connector interface for BingX Futures
type BingXConnector struct {
	*connector.BaseConnector
	conn          *websocket.Conn
	subscriptions map[string]bool
	mu            sync.RWMutex
	done          chan struct{}

	// New comprehensive client (optional, for advanced usage)
	client *Client
}

// NewBingXConnector creates a new BingX connector
func NewBingXConnector(symbols []string, depthLevels int) *BingXConnector {
	config := connector.ConnectorConfig{
		ExchangeID:     connector.BingX,
		WsURL:          wsBaseURL,
		RestURL:        restBaseURL,
		Symbols:        symbols,
		DepthLevels:    depthLevels,
		ReconnectDelay: 5 * time.Second,
		PingInterval:   20 * time.Second,
	}

	c := &BingXConnector{
		BaseConnector: connector.NewBaseConnector(config),
		subscriptions: make(map[string]bool),
		done:          make(chan struct{}),
	}

	for _, s := range symbols {
		c.subscriptions[s] = true
	}

	return c
}

// NewBingXConnectorWithCredentials creates a connector with API credentials for authenticated operations
func NewBingXConnectorWithCredentials(symbols []string, depthLevels int, apiKey, apiSecret string) *BingXConnector {
	c := NewBingXConnector(symbols, depthLevels)

	// Initialize comprehensive client for trading operations
	c.client = NewClientWithCredentials(apiKey, apiSecret)

	return c
}

// GetClient returns the underlying comprehensive client
func (c *BingXConnector) GetClient() *Client {
	return c.client
}

// Connect establishes WebSocket connection to BingX
func (c *BingXConnector) Connect(ctx context.Context) error {
	log.Info().Str("url", wsBaseURL).Msg("Connecting to BingX WebSocket")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, wsBaseURL, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.conn = conn
	c.SetConnected(true)
	log.Info().Msg("Connected to BingX WebSocket")

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

func (c *BingXConnector) subscribeSymbol(symbol string) error {
	msg := map[string]interface{}{
		"id":       fmt.Sprintf("depth_%s", symbol),
		"reqType":  "sub",
		"dataType": fmt.Sprintf("%s@depth20", symbol),
	}
	return c.conn.WriteJSON(msg)
}

// Disconnect closes the WebSocket connection
func (c *BingXConnector) Disconnect() error {
	close(c.done)
	c.SetConnected(false)
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Subscribe adds symbol subscriptions
func (c *BingXConnector) Subscribe(symbols []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range symbols {
		c.subscriptions[s] = true
	}
	return nil
}

// Unsubscribe removes symbol subscriptions
func (c *BingXConnector) Unsubscribe(symbols []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range symbols {
		delete(c.subscriptions, s)
	}
	return nil
}

// FetchInstruments fetches all USDT perpetual futures
func (c *BingXConnector) FetchInstruments(ctx context.Context) ([]connector.Instrument, error) {
	url := fmt.Sprintf("%s/openApi/swap/v2/quote/contracts", restBaseURL)

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
		Code int `json:"code"`
		Data []struct {
			Symbol            string      `json:"symbol"`
			Asset             string      `json:"asset"`
			Currency          string      `json:"currency"`
			PricePrecision    int         `json:"pricePrecision"`
			QuantityPrecision int         `json:"quantityPrecision"`
			TakerFeeRate      json.Number `json:"takerFeeRate"`
			MakerFeeRate      json.Number `json:"makerFeeRate"`
			ContractId        string      `json:"contractId"`
			Size              string      `json:"size"`
			Status            int         `json:"status"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var instruments []connector.Instrument
	for _, s := range result.Data {
		if s.Status != 1 {
			continue
		}

		takerFee, _ := s.TakerFeeRate.Float64()
		makerFee, _ := s.MakerFeeRate.Float64()
		contractSize, _ := strconv.ParseFloat(s.Size, 64)

		tickSize := 1.0
		for i := 0; i < s.PricePrecision; i++ {
			tickSize /= 10
		}
		lotSize := 1.0
		for i := 0; i < s.QuantityPrecision; i++ {
			lotSize /= 10
		}

		inst := connector.Instrument{
			ExchangeID:     connector.BingX,
			Symbol:         s.Symbol,
			Canonical:      fmt.Sprintf("%s-%s-PERP", s.Asset, s.Currency),
			BaseAsset:      s.Asset,
			QuoteAsset:     s.Currency,
			InstrumentType: "perpetual",
			TickSize:       tickSize,
			LotSize:        lotSize,
			ContractSize:   contractSize,
			TakerFee:       takerFee,
			MakerFee:       makerFee,
		}
		instruments = append(instruments, inst)
	}

	return instruments, nil
}

// FetchOrderbookSnapshot fetches orderbook via REST API
func (c *BingXConnector) FetchOrderbookSnapshot(ctx context.Context, symbol string, depth int) (*connector.Orderbook, error) {
	url := fmt.Sprintf("%s/openApi/swap/v2/quote/depth?symbol=%s&limit=%d", restBaseURL, symbol, depth)

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
		Code int `json:"code"`
		Data struct {
			Bids [][]string `json:"bids"`
			Asks [][]string `json:"asks"`
			T    int64      `json:"T"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	ob := &connector.Orderbook{
		ExchangeID: connector.BingX,
		Symbol:     symbol,
		Timestamp:  time.UnixMilli(result.Data.T),
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
func (c *BingXConnector) FetchFundingRates(ctx context.Context) ([]connector.FundingRate, error) {
	url := fmt.Sprintf("%s/openApi/swap/v2/quote/premiumIndex", restBaseURL)

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
		Code int `json:"code"`
		Data []struct {
			Symbol          string `json:"symbol"`
			LastFundingRate string `json:"lastFundingRate"`
			NextFundingTime int64  `json:"nextFundingTime"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var rates []connector.FundingRate
	for _, d := range result.Data {
		rate, _ := strconv.ParseFloat(d.LastFundingRate, 64)
		rates = append(rates, connector.FundingRate{
			ExchangeID:           connector.BingX,
			Symbol:               d.Symbol,
			FundingRate:          rate,
			NextFundingTime:      time.UnixMilli(d.NextFundingTime),
			FundingIntervalHours: 8,
			Timestamp:            time.Now(),
		})
	}

	return rates, nil
}

func (c *BingXConnector) readLoop() {
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

func (c *BingXConnector) pingLoop() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if err := c.conn.WriteMessage(websocket.TextMessage, []byte("Ping")); err != nil {
				log.Error().Err(err).Msg("Failed to send ping")
			}
		}
	}
}

func (c *BingXConnector) handleMessage(message []byte) {
	// Skip pong messages
	if string(message) == "Pong" {
		return
	}

	var msg struct {
		Code     int    `json:"code"`
		DataType string `json:"dataType"`
		Data     struct {
			Bids [][]string `json:"bids"`
			Asks [][]string `json:"asks"`
			T    int64      `json:"T"`
		} `json:"data"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		return
	}

	if msg.DataType == "" {
		return
	}

	// Extract symbol from dataType: BTC-USDT@depth20
	symbol := ""
	for i, ch := range msg.DataType {
		if ch == '@' {
			symbol = msg.DataType[:i]
			break
		}
	}

	ob := &connector.Orderbook{
		ExchangeID: connector.BingX,
		Symbol:     symbol,
		Canonical:  extractCanonical(symbol),
		Timestamp:  time.UnixMilli(msg.Data.T),
		IsSnapshot: true,
	}

	ob.Bids = parseStringLevels(msg.Data.Bids)
	ob.Asks = parseStringLevels(msg.Data.Asks)

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

// extractCanonical extracts base asset from BingX symbol (BTC-USDT -> BTC)
func extractCanonical(symbol string) string {
	for i, ch := range symbol {
		if ch == '-' {
			return symbol[:i]
		}
	}
	return symbol
}

// ConnectForSymbols establishes WebSocket connection for specific symbols only
func (c *BingXConnector) ConnectForSymbols(ctx context.Context, symbols []string) error {
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
func (c *BingXConnector) FetchPriceTickers(ctx context.Context) ([]connector.PriceTicker, error) {
	url := fmt.Sprintf("%s/openApi/swap/v2/quote/ticker", restBaseURL)

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
		Code int `json:"code"` // BingX returns numeric code
		Data []struct {
			Symbol    string `json:"symbol"`
			LastPrice string `json:"lastPrice"`
			BidPrice  string `json:"bidPrice"`
			AskPrice  string `json:"askPrice"`
			Volume    string `json:"volume"`
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
		volume, _ := strconv.ParseFloat(t.Volume, 64)

		tickers = append(tickers, connector.PriceTicker{
			ExchangeID: connector.BingX,
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
func (c *BingXConnector) FetchAssetInfo(ctx context.Context) ([]connector.AssetInfo, error) {
	if c.client != nil {
		configs, err := c.client.REST.GetAssetConfig(ctx)
		if err != nil {
			return []connector.AssetInfo{}, nil // Return empty on error to maintain backward compatibility
		}

		var assets []connector.AssetInfo
		for _, cfg := range configs {
			depositEnabled := false
			withdrawEnabled := false

			for _, net := range cfg.NetworkList {
				if net.DepositEnable {
					depositEnabled = true
				}
				if net.WithdrawEnable {
					withdrawEnabled = true
				}
			}

			assets = append(assets, connector.AssetInfo{
				ExchangeID:      connector.BingX,
				Asset:           cfg.Coin,
				DepositEnabled:  depositEnabled,
				WithdrawEnabled: withdrawEnabled,
			})
		}
		return assets, nil
	}
	return []connector.AssetInfo{}, nil
}

// =============================================================================
// Trading Methods (require credentials)
// =============================================================================

// PlaceOrder places a new order (requires credentials)
func (c *BingXConnector) PlaceOrder(ctx context.Context, symbol, side, positionSide, orderType string, price, quantity float64) (*OrderResponse, error) {
	if c.client == nil {
		return nil, fmt.Errorf("client not initialized with credentials")
	}

	return c.client.PlaceOrder(ctx, &OrderRequest{
		Symbol:       symbol,
		Side:         side,
		PositionSide: positionSide,
		Type:         orderType,
		Price:        price,
		Quantity:     quantity,
	})
}

// PlaceLimitOrder places a limit order (requires credentials)
func (c *BingXConnector) PlaceLimitOrder(ctx context.Context, symbol, side, positionSide string, price, quantity float64) (*OrderResponse, error) {
	if c.client == nil || c.client.Trading == nil {
		return nil, fmt.Errorf("trading client not initialized")
	}
	return c.client.Trading.PlaceLimitOrder(ctx, symbol, side, positionSide, price, quantity, TIFGoodTillCancel)
}

// PlaceMarketOrder places a market order (requires credentials)
func (c *BingXConnector) PlaceMarketOrder(ctx context.Context, symbol, side, positionSide string, quantity float64) (*OrderResponse, error) {
	if c.client == nil || c.client.Trading == nil {
		return nil, fmt.Errorf("trading client not initialized")
	}
	return c.client.Trading.PlaceMarketOrder(ctx, symbol, side, positionSide, quantity)
}

// CancelOrder cancels an order (requires credentials)
func (c *BingXConnector) CancelOrder(ctx context.Context, symbol string, orderID int64) (*CancelResponse, error) {
	if c.client == nil || c.client.Trading == nil {
		return nil, fmt.Errorf("trading client not initialized")
	}
	return c.client.Trading.CancelOrder(ctx, symbol, orderID)
}

// CancelAllOrders cancels all orders for a symbol (requires credentials)
func (c *BingXConnector) CancelAllOrders(ctx context.Context, symbol string) error {
	if c.client == nil || c.client.Trading == nil {
		return fmt.Errorf("trading client not initialized")
	}
	return c.client.Trading.CancelAllOrders(ctx, symbol)
}

// GetBalance returns account balance (requires credentials)
func (c *BingXConnector) GetBalance(ctx context.Context) (*AccountBalance, error) {
	if c.client == nil {
		return nil, fmt.Errorf("client not initialized with credentials")
	}
	return c.client.GetBalance(ctx)
}

// GetPositions returns positions (requires credentials)
func (c *BingXConnector) GetPositions(ctx context.Context, symbol string) ([]*Position, error) {
	if c.client == nil {
		return nil, fmt.Errorf("client not initialized with credentials")
	}
	return c.client.GetPositions(ctx, symbol)
}

// GetOpenOrders returns open orders (requires credentials)
func (c *BingXConnector) GetOpenOrders(ctx context.Context, symbol string) ([]*Order, error) {
	if c.client == nil {
		return nil, fmt.Errorf("client not initialized with credentials")
	}
	return c.client.GetOpenOrders(ctx, symbol)
}

// SetLeverage sets leverage for a symbol (requires credentials)
func (c *BingXConnector) SetLeverage(ctx context.Context, symbol string, leverage int) error {
	if c.client == nil || c.client.Trading == nil {
		return fmt.Errorf("trading client not initialized")
	}
	return c.client.Trading.SetLeverage(ctx, symbol, leverage)
}

// SetMarginMode sets margin mode for a symbol (requires credentials)
func (c *BingXConnector) SetMarginMode(ctx context.Context, symbol, marginMode string) error {
	if c.client == nil || c.client.Trading == nil {
		return fmt.Errorf("trading client not initialized")
	}
	return c.client.Trading.SetMarginMode(ctx, symbol, marginMode)
}

// ExecuteSlicedOrder executes an order in multiple slices (requires credentials)
func (c *BingXConnector) ExecuteSlicedOrder(ctx context.Context, config *SlicedOrderConfig) (*SlicedOrderResult, error) {
	if c.client == nil || c.client.Trading == nil {
		return nil, fmt.Errorf("trading client not initialized")
	}
	return c.client.Trading.ExecuteSlicedOrder(ctx, config)
}

// ConnectUserData connects to user data stream for real-time account/order updates (requires credentials)
func (c *BingXConnector) ConnectUserData(handler *WSUserDataHandler) error {
	if c.client == nil {
		return fmt.Errorf("client not initialized with credentials")
	}
	c.client.SetUserDataHandler(handler)
	return c.client.ConnectUserData()
}

// InitTrading initializes the trading client (requires credentials)
func (c *BingXConnector) InitTrading(handler *TradingHandler) error {
	if c.client == nil {
		return fmt.Errorf("client not initialized with credentials")
	}
	c.client.SetTradingHandler(handler)
	c.client.InitTrading()
	return nil
}
