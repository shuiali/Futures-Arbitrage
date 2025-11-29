package coinex

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"crossspread-md-ingest/internal/connector"

	"github.com/rs/zerolog/log"
)

// CoinExConnector implements the Connector interface for CoinEx Futures
// Uses the new v2 API for better performance and more features
type CoinExConnector struct {
	*connector.BaseConnector
	client        *Client
	subscriptions map[string]bool
	mu            sync.RWMutex
	done          chan struct{}
	depthLevels   int
}

// CoinExConnectorConfig holds configuration for the CoinEx connector
type CoinExConnectorConfig struct {
	Symbols       []string
	DepthLevels   int
	APIKey        string
	APISecret     string
	EnableTrading bool
}

// NewCoinExConnector creates a new CoinEx connector
func NewCoinExConnector(symbols []string, depthLevels int) *CoinExConnector {
	return NewCoinExConnectorWithConfig(CoinExConnectorConfig{
		Symbols:     symbols,
		DepthLevels: depthLevels,
	})
}

// NewCoinExConnectorWithConfig creates a new CoinEx connector with full configuration
func NewCoinExConnectorWithConfig(cfg CoinExConnectorConfig) *CoinExConnector {
	config := connector.ConnectorConfig{
		ExchangeID:     connector.CoinEx,
		WsURL:          WSFuturesURL,
		RestURL:        RESTBaseURL,
		Symbols:        cfg.Symbols,
		DepthLevels:    cfg.DepthLevels,
		ReconnectDelay: 5 * time.Second,
		PingInterval:   20 * time.Second,
	}

	// Create the unified client
	clientCfg := DefaultClientConfig()
	clientCfg.APIKey = cfg.APIKey
	clientCfg.APISecret = cfg.APISecret
	clientCfg.EnableUserData = cfg.EnableTrading && cfg.APIKey != ""

	c := &CoinExConnector{
		BaseConnector: connector.NewBaseConnector(config),
		client:        NewClient(clientCfg),
		subscriptions: make(map[string]bool),
		done:          make(chan struct{}),
		depthLevels:   cfg.DepthLevels,
	}

	if c.depthLevels == 0 {
		c.depthLevels = 20
	}

	for _, s := range cfg.Symbols {
		c.subscriptions[s] = true
	}

	// Setup callbacks
	c.setupCallbacks()

	return c
}

// setupCallbacks configures WebSocket event handlers
func (c *CoinExConnector) setupCallbacks() {
	// Handle depth updates
	c.client.SetDepthHandler(func(update *WSDepthUpdate) {
		c.handleDepthUpdate(update)
	})

	// Handle trades/deals updates
	c.client.SetTradesHandler(func(update *WSDealsUpdate) {
		c.handleDealsUpdate(update)
	})

	// Handle BBO updates
	c.client.SetBBOHandler(func(update *WSBBOUpdate) {
		c.handleBBOUpdate(update)
	})

	// Handle errors
	c.client.SetErrorHandler(func(err error) {
		c.EmitError(err)
	})
}

// Connect establishes WebSocket connection to CoinEx
func (c *CoinExConnector) Connect(ctx context.Context) error {
	log.Info().Str("url", WSFuturesURL).Msg("Connecting to CoinEx WebSocket")

	if err := c.client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect client: %w", err)
	}

	c.SetConnected(true)
	log.Info().Msg("Connected to CoinEx WebSocket")

	// Subscribe to orderbook updates
	c.mu.RLock()
	symbols := make([]string, 0, len(c.subscriptions))
	for symbol := range c.subscriptions {
		symbols = append(symbols, symbol)
	}
	c.mu.RUnlock()

	if len(symbols) > 0 {
		if err := c.client.SubscribeOrderbook(symbols, c.depthLevels, true); err != nil {
			log.Error().Err(err).Msg("Failed to subscribe to orderbook")
		}
		if err := c.client.SubscribeBBO(symbols); err != nil {
			log.Error().Err(err).Msg("Failed to subscribe to BBO")
		}
	}

	return nil
}

// Disconnect closes the WebSocket connection
func (c *CoinExConnector) Disconnect() error {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	c.SetConnected(false)
	return c.client.Disconnect()
}

// Subscribe adds symbol subscriptions
func (c *CoinExConnector) Subscribe(symbols []string) error {
	c.mu.Lock()
	for _, s := range symbols {
		c.subscriptions[s] = true
	}
	c.mu.Unlock()

	if c.IsConnected() {
		if err := c.client.SubscribeOrderbook(symbols, c.depthLevels, true); err != nil {
			return err
		}
		if err := c.client.SubscribeBBO(symbols); err != nil {
			return err
		}
	}
	return nil
}

// Unsubscribe removes symbol subscriptions
func (c *CoinExConnector) Unsubscribe(symbols []string) error {
	c.mu.Lock()
	for _, s := range symbols {
		delete(c.subscriptions, s)
	}
	c.mu.Unlock()
	return nil
}

// FetchInstruments fetches all perpetual futures using v2 API
func (c *CoinExConnector) FetchInstruments(ctx context.Context) ([]connector.Instrument, error) {
	markets, err := c.client.GetAllMarkets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch markets: %w", err)
	}

	var instruments []connector.Instrument
	for _, m := range markets {
		takerFee := StringToFloat64(m.TakerFeeRate)
		makerFee := StringToFloat64(m.MakerFeeRate)
		tickSize := StringToFloat64(m.TickSize)
		minAmount := StringToFloat64(m.MinAmount)

		inst := connector.Instrument{
			ExchangeID:     connector.CoinEx,
			Symbol:         m.Market,
			Canonical:      fmt.Sprintf("%s-%s-PERP", m.BaseCcy, m.QuoteCcy),
			BaseAsset:      m.BaseCcy,
			QuoteAsset:     m.QuoteCcy,
			InstrumentType: "perpetual",
			TickSize:       tickSize,
			LotSize:        minAmount,
			ContractSize:   1,
			TakerFee:       takerFee,
			MakerFee:       makerFee,
		}
		instruments = append(instruments, inst)
	}

	return instruments, nil
}

// FetchOrderbookSnapshot fetches orderbook via REST API
func (c *CoinExConnector) FetchOrderbookSnapshot(ctx context.Context, symbol string, depth int) (*connector.Orderbook, error) {
	depthData, err := c.client.GetOrderbook(ctx, symbol, depth)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch orderbook: %w", err)
	}

	ob := &connector.Orderbook{
		ExchangeID: connector.CoinEx,
		Symbol:     symbol,
		Canonical:  extractCanonical(symbol),
		Timestamp:  time.Now(),
		IsSnapshot: true,
	}

	// Parse bids and asks from the Depth.Depth field
	ob.Bids = parseDepthLevelsToConnector(depthData.Depth.Bids)
	ob.Asks = parseDepthLevelsToConnector(depthData.Depth.Asks)

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

// FetchFundingRates fetches current funding rates using v2 API
func (c *CoinExConnector) FetchFundingRates(ctx context.Context) ([]connector.FundingRate, error) {
	// First get all markets to know which ones to query
	markets, err := c.client.GetAllMarkets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch markets: %w", err)
	}

	marketNames := make([]string, 0, len(markets))
	for _, m := range markets {
		marketNames = append(marketNames, m.Market)
	}

	fundingRates, err := c.client.GetFundingRates(ctx, marketNames)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch funding rates: %w", err)
	}

	var rates []connector.FundingRate
	for _, fr := range fundingRates {
		rate := StringToFloat64(fr.LatestFundingRate)

		rates = append(rates, connector.FundingRate{
			ExchangeID:           connector.CoinEx,
			Symbol:               fr.Market,
			Canonical:            extractCanonical(fr.Market),
			FundingRate:          rate,
			NextFundingTime:      time.UnixMilli(fr.NextFundingTime),
			FundingIntervalHours: 8,
			Timestamp:            time.Now(),
		})
	}

	return rates, nil
}

// FetchPriceTickers fetches current prices for all symbols via REST API
func (c *CoinExConnector) FetchPriceTickers(ctx context.Context) ([]connector.PriceTicker, error) {
	// Get all markets first
	markets, err := c.client.GetAllMarkets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch markets: %w", err)
	}

	marketNames := make([]string, 0, len(markets))
	for _, m := range markets {
		marketNames = append(marketNames, m.Market)
	}

	tickers, err := c.client.GetTickers(ctx, marketNames)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tickers: %w", err)
	}

	var result []connector.PriceTicker
	for _, t := range tickers {
		price := StringToFloat64(t.Last)
		bidPrice := StringToFloat64(t.VolumeBuy)
		askPrice := StringToFloat64(t.VolumeSell)
		volume := StringToFloat64(t.Volume)

		result = append(result, connector.PriceTicker{
			ExchangeID: connector.CoinEx,
			Symbol:     t.Market,
			Canonical:  extractCanonical(t.Market),
			Price:      price,
			BidPrice:   bidPrice,
			AskPrice:   askPrice,
			Volume24h:  volume,
			Timestamp:  time.Now(),
		})
	}

	return result, nil
}

// FetchAssetInfo fetches deposit/withdrawal status for assets
// Uses market data to derive available assets since detailed deposit/withdraw info requires authentication
func (c *CoinExConnector) FetchAssetInfo(ctx context.Context) ([]connector.AssetInfo, error) {
	markets, err := c.client.GetAllMarkets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch markets: %w", err)
	}

	// Extract unique base assets from available markets
	assetMap := make(map[string]*connector.AssetInfo)
	for _, m := range markets {
		asset := m.BaseCcy
		if asset == "" {
			continue
		}

		if _, exists := assetMap[asset]; !exists {
			// Determine if trading is available (implies deposit/withdraw likely available)
			canTrade := m.IsAPITradingAvailable && m.Status == "available"

			assetMap[asset] = &connector.AssetInfo{
				ExchangeID:      connector.CoinEx,
				Asset:           asset,
				Networks:        []string{asset}, // Default network to asset name
				DepositEnabled:  canTrade,
				WithdrawEnabled: canTrade,
				MinWithdraw:     0, // Requires authenticated API
				WithdrawFee:     0, // Requires authenticated API
				Timestamp:       time.Now(),
			}
		}
	}

	// Convert map to slice
	result := make([]connector.AssetInfo, 0, len(assetMap))
	for _, info := range assetMap {
		result = append(result, *info)
	}

	log.Debug().Int("count", len(result)).Msg("Fetched CoinEx asset info from markets")
	return result, nil
}

// ConnectForSymbols establishes WebSocket connection for specific symbols only
func (c *CoinExConnector) ConnectForSymbols(ctx context.Context, symbols []string) error {
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

// =============================================================================
// WebSocket Event Handlers
// =============================================================================

func (c *CoinExConnector) handleDepthUpdate(update *WSDepthUpdate) {
	ob := &connector.Orderbook{
		ExchangeID: connector.CoinEx,
		Symbol:     update.Market,
		Canonical:  extractCanonical(update.Market),
		Timestamp:  time.UnixMilli(update.Depth.UpdatedAt),
		IsSnapshot: update.IsFull,
	}

	ob.Bids = parseDepthLevelsToConnector(update.Depth.Bids)
	ob.Asks = parseDepthLevelsToConnector(update.Depth.Asks)

	// Sort bids descending, asks ascending
	sort.Slice(ob.Bids, func(i, j int) bool {
		return ob.Bids[i].Price > ob.Bids[j].Price
	})
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

func (c *CoinExConnector) handleDealsUpdate(update *WSDealsUpdate) {
	for _, deal := range update.DealList {
		trade := &connector.Trade{
			ExchangeID: connector.CoinEx,
			Symbol:     update.Market,
			Canonical:  extractCanonical(update.Market),
			Price:      StringToFloat64(deal.Price),
			Quantity:   StringToFloat64(deal.Amount),
			Side:       deal.Side,
			Timestamp:  time.UnixMilli(deal.CreatedAt),
		}
		c.EmitTrade(trade)
	}
}

func (c *CoinExConnector) handleBBOUpdate(update *WSBBOUpdate) {
	ob := &connector.Orderbook{
		ExchangeID: connector.CoinEx,
		Symbol:     update.Market,
		Canonical:  extractCanonical(update.Market),
		Timestamp:  time.UnixMilli(update.UpdatedAt),
		IsSnapshot: false,
		BestBid:    StringToFloat64(update.BestBidPrice),
		BestAsk:    StringToFloat64(update.BestAskPrice),
	}

	bidQty := StringToFloat64(update.BestBidSize)
	askQty := StringToFloat64(update.BestAskSize)

	if ob.BestBid > 0 {
		ob.Bids = []connector.PriceLevel{{Price: ob.BestBid, Quantity: bidQty}}
	}
	if ob.BestAsk > 0 {
		ob.Asks = []connector.PriceLevel{{Price: ob.BestAsk, Quantity: askQty}}
	}
	if ob.BestBid > 0 && ob.BestAsk > 0 {
		ob.SpreadBps = (ob.BestAsk - ob.BestBid) / ob.BestBid * 10000
	}

	c.EmitOrderbook(ob)
}

// =============================================================================
// Helper Functions
// =============================================================================

// parseDepthLevelsToConnector converts string arrays to connector.PriceLevel
func parseDepthLevelsToConnector(levels [][]string) []connector.PriceLevel {
	result := make([]connector.PriceLevel, 0, len(levels))
	for _, item := range levels {
		if len(item) < 2 {
			continue
		}
		price, _ := strconv.ParseFloat(item[0], 64)
		qty, _ := strconv.ParseFloat(item[1], 64)
		if qty > 0 {
			result = append(result, connector.PriceLevel{
				Price:    price,
				Quantity: qty,
			})
		}
	}
	return result
}

// extractCanonical extracts base asset from CoinEx symbol (BTCUSDT -> BTC)
func extractCanonical(symbol string) string {
	quotes := []string{"USDT", "USDC", "USD"}
	for _, quote := range quotes {
		if len(symbol) > len(quote) && symbol[len(symbol)-len(quote):] == quote {
			return symbol[:len(symbol)-len(quote)]
		}
	}
	return symbol
}
