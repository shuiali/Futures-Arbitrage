package mexc

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"crossspread-md-ingest/internal/connector"

	"github.com/rs/zerolog/log"
)

// MEXCConnector implements the Connector interface for MEXC Futures
type MEXCConnector struct {
	*connector.BaseConnector
	client        *Client
	subscriptions map[string]bool
	mu            sync.RWMutex
	done          chan struct{}
}

// NewMEXCConnector creates a new MEXC connector
func NewMEXCConnector(symbols []string, depthLevels int) *MEXCConnector {
	config := connector.ConnectorConfig{
		ExchangeID:     connector.MEXC,
		WsURL:          WSPublicURL,
		RestURL:        BaseURLProduction,
		Symbols:        symbols,
		DepthLevels:    depthLevels,
		ReconnectDelay: 5 * time.Second,
		PingInterval:   20 * time.Second,
	}

	c := &MEXCConnector{
		BaseConnector: connector.NewBaseConnector(config),
		subscriptions: make(map[string]bool),
		done:          make(chan struct{}),
	}

	for _, s := range symbols {
		c.subscriptions[s] = true
	}

	return c
}

// NewMEXCConnectorWithCredentials creates a new MEXC connector with API credentials
func NewMEXCConnectorWithCredentials(symbols []string, depthLevels int, apiKey, secretKey string) *MEXCConnector {
	c := NewMEXCConnector(symbols, depthLevels)

	// Create client with credentials
	client, err := NewClient(&ClientConfig{
		APIKey:          apiKey,
		SecretKey:       secretKey,
		RESTBaseURL:     BaseURLProduction,
		RESTTimeout:     30,
		WSReconnect:     true,
		WSReconnectWait: 5,
		WSMaxReconnect:  3,
		WSPingInterval:  20,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to create MEXC client")
		return c
	}

	c.client = client
	return c
}

// marketDataHandlerAdapter adapts connector handlers to MarketDataHandler interface
type marketDataHandlerAdapter struct {
	connector *MEXCConnector
}

func (a *marketDataHandlerAdapter) OnTicker(ticker *WSTickerData) {
	// Convert to connector.PriceTicker if needed
	log.Debug().Str("symbol", ticker.Symbol).Float64("last", ticker.LastPrice).Msg("Ticker update")
}

func (a *marketDataHandlerAdapter) OnOrderBook(symbol string, book *WSDepthData, isFull bool) {
	ob := &connector.Orderbook{
		ExchangeID: connector.MEXC,
		Symbol:     symbol,
		Canonical:  extractCanonical(symbol),
		Timestamp:  time.Now(),
		SequenceID: book.Version,
		IsSnapshot: isFull,
	}

	ob.Bids = convertFloatLevels(book.Bids)
	ob.Asks = convertFloatLevels(book.Asks)

	if len(ob.Bids) > 0 {
		ob.BestBid = ob.Bids[0].Price
	}
	if len(ob.Asks) > 0 {
		ob.BestAsk = ob.Asks[0].Price
	}
	if ob.BestBid > 0 && ob.BestAsk > 0 {
		ob.SpreadBps = (ob.BestAsk - ob.BestBid) / ob.BestBid * 10000
	}

	a.connector.EmitOrderbook(ob)
}

func (a *marketDataHandlerAdapter) OnTrade(symbol string, trade *WSTradeData) {
	t := &connector.Trade{
		ExchangeID: connector.MEXC,
		Symbol:     symbol,
		Canonical:  extractCanonical(symbol),
		TradeID:    fmt.Sprintf("%d", trade.Timestamp), // Use timestamp as trade ID since no ID provided
		Price:      trade.Price,
		Quantity:   trade.Volume,
		Side:       getSideString(trade.TradeType),
		Timestamp:  time.UnixMilli(trade.Timestamp),
	}
	a.connector.EmitTrade(t)
}

func (a *marketDataHandlerAdapter) OnKline(symbol string, interval string, kline *WSKlineData) {
	log.Debug().Str("symbol", symbol).Str("interval", interval).Msg("Kline update")
}

func (a *marketDataHandlerAdapter) OnError(err error) {
	a.connector.EmitError(err)
}

func (a *marketDataHandlerAdapter) OnConnected() {
	a.connector.SetConnected(true)
	log.Info().Msg("Connected to MEXC WebSocket")
}

func (a *marketDataHandlerAdapter) OnDisconnected() {
	a.connector.SetConnected(false)
	log.Info().Msg("Disconnected from MEXC WebSocket")
}

// Connect establishes WebSocket connection to MEXC
func (c *MEXCConnector) Connect(ctx context.Context) error {
	log.Info().Str("url", WSPublicURL).Msg("Connecting to MEXC WebSocket")

	// Create client if not exists
	if c.client == nil {
		client, err := NewClient(DefaultClientConfig())
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		c.client = client
	}

	// Set handler
	c.client.SetMarketDataHandler(&marketDataHandlerAdapter{connector: c})

	// Connect market data WebSocket
	if err := c.client.ConnectMarketData(); err != nil {
		return fmt.Errorf("failed to connect market data: %w", err)
	}

	// Subscribe to symbols
	c.mu.RLock()
	symbols := make([]string, 0, len(c.subscriptions))
	for s := range c.subscriptions {
		symbols = append(symbols, s)
	}
	c.mu.RUnlock()

	for _, symbol := range symbols {
		if err := c.client.SubscribeDepth(symbol); err != nil {
			log.Error().Err(err).Str("symbol", symbol).Msg("Failed to subscribe to depth")
		}
	}

	return nil
}

// Disconnect closes the WebSocket connection
func (c *MEXCConnector) Disconnect() error {
	c.SetConnected(false)
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// Subscribe adds symbol subscriptions
func (c *MEXCConnector) Subscribe(symbols []string) error {
	c.mu.Lock()
	for _, s := range symbols {
		c.subscriptions[s] = true
	}
	c.mu.Unlock()

	// If connected, subscribe immediately
	if c.client != nil && c.client.IsMarketDataConnected() {
		for _, s := range symbols {
			if err := c.client.SubscribeDepth(s); err != nil {
				log.Error().Err(err).Str("symbol", s).Msg("Failed to subscribe")
			}
		}
	}

	return nil
}

// Unsubscribe removes symbol subscriptions
func (c *MEXCConnector) Unsubscribe(symbols []string) error {
	c.mu.Lock()
	for _, s := range symbols {
		delete(c.subscriptions, s)
	}
	c.mu.Unlock()

	// If connected, unsubscribe immediately
	if c.client != nil && c.client.IsMarketDataConnected() {
		md := c.client.MarketData()
		if md != nil {
			for _, s := range symbols {
				if err := md.UnsubscribeDepth(s); err != nil {
					log.Error().Err(err).Str("symbol", s).Msg("Failed to unsubscribe")
				}
			}
		}
	}

	return nil
}

// FetchInstruments fetches all perpetual futures
func (c *MEXCConnector) FetchInstruments(ctx context.Context) ([]connector.Instrument, error) {
	// Use client if available
	if c.client != nil {
		contracts, err := c.client.GetContracts(ctx)
		if err != nil {
			return nil, err
		}

		var instruments []connector.Instrument
		for _, contract := range contracts {
			if contract.State != 0 {
				continue
			}

			inst := connector.Instrument{
				ExchangeID:     connector.MEXC,
				Symbol:         contract.Symbol,
				Canonical:      fmt.Sprintf("%s-%s-PERP", contract.BaseCoin, contract.QuoteCoin),
				BaseAsset:      contract.BaseCoin,
				QuoteAsset:     contract.QuoteCoin,
				InstrumentType: "perpetual",
				TickSize:       contract.PriceUnit,
				LotSize:        contract.VolUnit,
				ContractSize:   contract.ContractSize,
				TakerFee:       contract.TakerFeeRate,
				MakerFee:       contract.MakerFeeRate,
			}
			instruments = append(instruments, inst)
		}

		return instruments, nil
	}

	// Fallback to direct REST call
	rest := NewRESTClient(RESTClientConfig{
		BaseURL: BaseURLProduction,
	})

	contracts, err := rest.GetContracts(ctx)
	if err != nil {
		return nil, err
	}

	var instruments []connector.Instrument
	for _, contract := range contracts {
		if contract.State != 0 {
			continue
		}

		inst := connector.Instrument{
			ExchangeID:     connector.MEXC,
			Symbol:         contract.Symbol,
			Canonical:      fmt.Sprintf("%s-%s-PERP", contract.BaseCoin, contract.QuoteCoin),
			BaseAsset:      contract.BaseCoin,
			QuoteAsset:     contract.QuoteCoin,
			InstrumentType: "perpetual",
			TickSize:       contract.PriceUnit,
			LotSize:        contract.VolUnit,
			ContractSize:   contract.ContractSize,
			TakerFee:       contract.TakerFeeRate,
			MakerFee:       contract.MakerFeeRate,
		}
		instruments = append(instruments, inst)
	}

	return instruments, nil
}

// FetchOrderbookSnapshot fetches orderbook via REST API
func (c *MEXCConnector) FetchOrderbookSnapshot(ctx context.Context, symbol string, depth int) (*connector.Orderbook, error) {
	var ob *OrderBook
	var err error

	if c.client != nil {
		ob, err = c.client.GetDepth(ctx, symbol, depth)
	} else {
		rest := NewRESTClient(RESTClientConfig{
			BaseURL: BaseURLProduction,
		})
		ob, err = rest.GetDepth(ctx, symbol, depth)
	}

	if err != nil {
		return nil, err
	}

	result := &connector.Orderbook{
		ExchangeID: connector.MEXC,
		Symbol:     symbol,
		Timestamp:  time.UnixMilli(ob.Timestamp),
		SequenceID: ob.Version,
		IsSnapshot: true,
	}

	result.Bids = convertOrderBookLevels(ob.Bids)
	result.Asks = convertOrderBookLevels(ob.Asks)

	if len(result.Bids) > 0 {
		result.BestBid = result.Bids[0].Price
	}
	if len(result.Asks) > 0 {
		result.BestAsk = result.Asks[0].Price
	}
	if result.BestBid > 0 && result.BestAsk > 0 {
		result.SpreadBps = (result.BestAsk - result.BestBid) / result.BestBid * 10000
	}

	return result, nil
}

// FetchFundingRates fetches current funding rates
func (c *MEXCConnector) FetchFundingRates(ctx context.Context) ([]connector.FundingRate, error) {
	var fundingRates []FundingRate
	var err error

	if c.client != nil {
		fundingRates, err = c.client.GetAllFundingRates(ctx)
	} else {
		rest := NewRESTClient(RESTClientConfig{
			BaseURL: BaseURLProduction,
		})
		fundingRates, err = rest.GetAllFundingRates(ctx)
	}

	if err != nil {
		return nil, err
	}

	var rates []connector.FundingRate
	for _, fr := range fundingRates {
		rates = append(rates, connector.FundingRate{
			ExchangeID:           connector.MEXC,
			Symbol:               fr.Symbol,
			Canonical:            extractCanonical(fr.Symbol),
			FundingRate:          fr.FundingRate,
			NextFundingTime:      time.UnixMilli(fr.NextSettleTime),
			FundingIntervalHours: 8,
			Timestamp:            time.Now(),
		})
	}

	return rates, nil
}

// FetchPriceTickers fetches current prices for all symbols via REST API
func (c *MEXCConnector) FetchPriceTickers(ctx context.Context) ([]connector.PriceTicker, error) {
	var tickers []Ticker
	var err error

	if c.client != nil {
		tickers, err = c.client.GetTickers(ctx)
	} else {
		rest := NewRESTClient(RESTClientConfig{
			BaseURL: BaseURLProduction,
		})
		tickers, err = rest.GetTickers(ctx)
	}

	if err != nil {
		return nil, err
	}

	var result []connector.PriceTicker
	for _, t := range tickers {
		result = append(result, connector.PriceTicker{
			ExchangeID: connector.MEXC,
			Symbol:     t.Symbol,
			Canonical:  extractCanonical(t.Symbol),
			Price:      t.LastPrice,
			BidPrice:   t.Bid1,
			AskPrice:   t.Ask1,
			Volume24h:  t.Volume24,
			Timestamp:  time.Now(),
		})
	}

	return result, nil
}

// FetchAssetInfo fetches deposit/withdrawal status for assets
// Uses contract info to derive available assets since detailed deposit/withdraw info requires authentication
func (c *MEXCConnector) FetchAssetInfo(ctx context.Context) ([]connector.AssetInfo, error) {
	var contracts []Contract
	var err error

	if c.client != nil {
		contracts, err = c.client.GetContracts(ctx)
	} else {
		rest := NewRESTClient(RESTClientConfig{
			BaseURL: BaseURLProduction,
		})
		contracts, err = rest.GetContracts(ctx)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to fetch contracts: %w", err)
	}

	// Extract unique base assets from active contracts
	assetMap := make(map[string]*connector.AssetInfo)
	for _, contract := range contracts {
		asset := contract.BaseCoin
		if asset == "" {
			continue
		}

		if _, exists := assetMap[asset]; !exists {
			// State 0 = active, APIAllowed indicates trading is available
			isActive := contract.State == 0 && contract.APIAllowed

			assetMap[asset] = &connector.AssetInfo{
				ExchangeID:      connector.MEXC,
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
	result := make([]connector.AssetInfo, 0, len(assetMap))
	for _, info := range assetMap {
		result = append(result, *info)
	}

	log.Debug().Int("count", len(result)).Msg("Fetched MEXC asset info from contracts")
	return result, nil
}

// ConnectForSymbols establishes WebSocket connection for specific symbols only
func (c *MEXCConnector) ConnectForSymbols(ctx context.Context, symbols []string) error {
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

// Client returns the underlying MEXC client for advanced operations
func (c *MEXCConnector) Client() *Client {
	return c.client
}

// =============================================================================
// Helper Functions
// =============================================================================

// convertFloatLevels converts float array levels to PriceLevel
func convertFloatLevels(data [][]float64) []connector.PriceLevel {
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

// convertOrderBookLevels converts OrderBookLevel to connector.PriceLevel
func convertOrderBookLevels(data [][]float64) []connector.PriceLevel {
	return convertFloatLevels(data)
}

func getSideString(tradeType int) string {
	if tradeType == 1 {
		return "buy"
	}
	return "sell"
}

// extractCanonical extracts base asset from MEXC symbol (BTC_USDT -> BTC)
func extractCanonical(symbol string) string {
	for i, ch := range symbol {
		if ch == '_' {
			return symbol[:i]
		}
	}
	// Fallback for symbols without underscore
	quotes := []string{"USDT", "USDC", "USD"}
	for _, quote := range quotes {
		if len(symbol) > len(quote) && symbol[len(symbol)-len(quote):] == quote {
			return symbol[:len(symbol)-len(quote)]
		}
	}
	return symbol
}
