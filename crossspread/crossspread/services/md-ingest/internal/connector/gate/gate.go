package gate

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"crossspread-md-ingest/internal/connector"

	"github.com/rs/zerolog/log"
)

// GateConnector implements the Connector interface for Gate.io Futures
type GateConnector struct {
	*connector.BaseConnector
	client        *Client
	settle        string // btc or usdt
	subscriptions map[string]bool
	mu            sync.RWMutex
	done          chan struct{}
}

// NewGateConnector creates a new Gate.io connector
func NewGateConnector(symbols []string, depthLevels int, settle string) *GateConnector {
	if settle == "" {
		settle = SettleUSDT
	}

	config := connector.ConnectorConfig{
		ExchangeID:     connector.GateIO,
		WsURL:          "wss://fx-ws.gateio.ws/v4/ws/" + settle,
		RestURL:        "https://api.gateio.ws",
		Symbols:        symbols,
		DepthLevels:    depthLevels,
		ReconnectDelay: 5 * time.Second,
		PingInterval:   15 * time.Second,
	}

	c := &GateConnector{
		BaseConnector: connector.NewBaseConnector(config),
		settle:        settle,
		subscriptions: make(map[string]bool),
		done:          make(chan struct{}),
	}

	for _, s := range symbols {
		c.subscriptions[s] = true
	}

	return c
}

// NewGateConnectorWithCredentials creates a new Gate.io connector with API credentials
func NewGateConnectorWithCredentials(symbols []string, depthLevels int, settle, apiKey, apiSecret string) *GateConnector {
	c := NewGateConnector(symbols, depthLevels, settle)

	// Create client with credentials
	clientConfig := DefaultConfig()
	clientConfig.APIKey = apiKey
	clientConfig.APISecret = apiSecret
	clientConfig.DefaultSettle = settle

	c.client = NewClient(clientConfig)

	return c
}

// marketDataHandlerAdapter adapts connector handlers to WSMarketDataHandler interface
type marketDataHandlerAdapter struct {
	connector *GateConnector
}

func (a *marketDataHandlerAdapter) OnTicker(settle string, ticker *WSTickerData) {
	log.Debug().Str("contract", ticker.Contract).Str("last", ticker.Last).Msg("Gate.io ticker update")
}

func (a *marketDataHandlerAdapter) OnOrderBook(settle string, book *WSOrderBookData) {
	ob := &connector.Orderbook{
		ExchangeID: connector.GateIO,
		Symbol:     book.Contract,
		Canonical:  extractCanonical(book.Contract),
		Timestamp:  time.UnixMilli(book.T),
		SequenceID: book.ID,
		IsSnapshot: true,
	}

	ob.Bids = convertOrderBookEntries(book.Bids)
	ob.Asks = convertOrderBookEntries(book.Asks)

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

func (a *marketDataHandlerAdapter) OnTrade(settle string, trade *WSTradeData) {
	price, _ := strconv.ParseFloat(trade.Price, 64)
	side := "buy"
	if trade.Size < 0 {
		side = "sell"
	}

	t := &connector.Trade{
		ExchangeID: connector.GateIO,
		Symbol:     trade.Contract,
		Canonical:  extractCanonical(trade.Contract),
		TradeID:    fmt.Sprintf("%d", trade.ID),
		Price:      price,
		Quantity:   float64(abs(trade.Size)),
		Side:       side,
		Timestamp:  time.UnixMilli(trade.CreateTimeMs),
	}
	a.connector.EmitTrade(t)
}

func (a *marketDataHandlerAdapter) OnBookTicker(settle string, bookTicker *WSBookTickerData) {
	log.Debug().Str("contract", bookTicker.S).Str("bid", bookTicker.B).Str("ask", bookTicker.A).Msg("Gate.io book ticker update")
}

func (a *marketDataHandlerAdapter) OnKline(settle string, kline *WSKlineData) {
	log.Debug().Int64("t", kline.T).Str("close", kline.C).Msg("Gate.io kline update")
}

func (a *marketDataHandlerAdapter) OnError(err error) {
	a.connector.EmitError(err)
}

func (a *marketDataHandlerAdapter) OnConnect(settle string) {
	a.connector.SetConnected(true)
	log.Info().Str("settle", settle).Msg("Connected to Gate.io WebSocket")
}

func (a *marketDataHandlerAdapter) OnDisconnect(settle string, err error) {
	a.connector.SetConnected(false)
	log.Info().Str("settle", settle).Err(err).Msg("Disconnected from Gate.io WebSocket")
}

// Connect establishes WebSocket connection to Gate.io
func (c *GateConnector) Connect(ctx context.Context) error {
	log.Info().Str("settle", c.settle).Msg("Connecting to Gate.io WebSocket")

	// Create client if not exists
	if c.client == nil {
		c.client = NewClient(DefaultConfig())
	}

	// Set handler
	c.client.SetMarketDataHandler(&WSMarketDataHandler{
		OnTicker:     (&marketDataHandlerAdapter{connector: c}).OnTicker,
		OnOrderBook:  (&marketDataHandlerAdapter{connector: c}).OnOrderBook,
		OnTrade:      (&marketDataHandlerAdapter{connector: c}).OnTrade,
		OnBookTicker: (&marketDataHandlerAdapter{connector: c}).OnBookTicker,
		OnKline:      (&marketDataHandlerAdapter{connector: c}).OnKline,
		OnError:      (&marketDataHandlerAdapter{connector: c}).OnError,
		OnConnect:    (&marketDataHandlerAdapter{connector: c}).OnConnect,
		OnDisconnect: (&marketDataHandlerAdapter{connector: c}).OnDisconnect,
	})

	// Connect market data WebSocket
	if err := c.client.ConnectMarketData(c.settle); err != nil {
		return fmt.Errorf("failed to connect market data: %w", err)
	}

	// Subscribe to symbols
	c.mu.RLock()
	symbols := make([]string, 0, len(c.subscriptions))
	for s := range c.subscriptions {
		symbols = append(symbols, s)
	}
	c.mu.RUnlock()

	// Subscribe to orderbook for each symbol
	for _, symbol := range symbols {
		if err := c.client.SubscribeOrderBook(c.settle, symbol, "20", "0"); err != nil {
			log.Error().Err(err).Str("symbol", symbol).Msg("Failed to subscribe to depth")
		}
	}

	return nil
}

// ConnectForSymbols establishes WebSocket connection for specific symbols only
func (c *GateConnector) ConnectForSymbols(ctx context.Context, symbols []string) error {
	c.mu.Lock()
	c.subscriptions = make(map[string]bool)
	for _, s := range symbols {
		c.subscriptions[s] = true
	}
	c.mu.Unlock()

	return c.Connect(ctx)
}

// Disconnect closes the WebSocket connection
func (c *GateConnector) Disconnect() error {
	c.SetConnected(false)
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// Subscribe adds symbol subscriptions
func (c *GateConnector) Subscribe(symbols []string) error {
	c.mu.Lock()
	for _, s := range symbols {
		c.subscriptions[s] = true
	}
	c.mu.Unlock()

	// If connected, subscribe immediately
	if c.client != nil && c.client.MarketData != nil && c.client.MarketData.IsConnected(c.settle) {
		for _, s := range symbols {
			if err := c.client.SubscribeOrderBook(c.settle, s, "20", "0"); err != nil {
				log.Error().Err(err).Str("symbol", s).Msg("Failed to subscribe")
			}
		}
	}

	return nil
}

// Unsubscribe removes symbol subscriptions
func (c *GateConnector) Unsubscribe(symbols []string) error {
	c.mu.Lock()
	for _, s := range symbols {
		delete(c.subscriptions, s)
	}
	c.mu.Unlock()

	// If connected, unsubscribe immediately
	if c.client != nil && c.client.MarketData != nil && c.client.MarketData.IsConnected(c.settle) {
		for _, s := range symbols {
			if err := c.client.MarketData.UnsubscribeOrderBook(c.settle, s, "20", "0"); err != nil {
				log.Error().Err(err).Str("symbol", s).Msg("Failed to unsubscribe")
			}
		}
	}

	return nil
}

// FetchInstruments fetches all perpetual futures
func (c *GateConnector) FetchInstruments(ctx context.Context) ([]connector.Instrument, error) {
	rest := c.getRESTClient()

	contracts, err := rest.GetContracts(ctx, c.settle)
	if err != nil {
		return nil, err
	}

	var instruments []connector.Instrument
	for _, contract := range contracts {
		// Skip delisting contracts
		if contract.InDelisting {
			continue
		}

		// Parse tick size
		tickSize, _ := strconv.ParseFloat(contract.OrderPriceRound, 64)

		// Parse fees
		makerFee, _ := strconv.ParseFloat(contract.MakerFeeRate, 64)
		takerFee, _ := strconv.ParseFloat(contract.TakerFeeRate, 64)

		// Parse canonical: BTC_USDT -> BTC-USDT-PERP
		parts := strings.Split(contract.Name, "_")
		base := parts[0]
		quote := c.settle
		if len(parts) > 1 {
			quote = parts[1]
		}

		inst := connector.Instrument{
			ExchangeID:     connector.GateIO,
			Symbol:         contract.Name,
			Canonical:      fmt.Sprintf("%s-%s-PERP", base, quote),
			BaseAsset:      base,
			QuoteAsset:     quote,
			InstrumentType: "perpetual",
			TickSize:       tickSize,
			LotSize:        1, // Gate uses contracts
			ContractSize:   1,
			TakerFee:       takerFee,
			MakerFee:       makerFee,
		}
		instruments = append(instruments, inst)
	}

	return instruments, nil
}

// FetchOrderbookSnapshot fetches orderbook via REST API
func (c *GateConnector) FetchOrderbookSnapshot(ctx context.Context, symbol string, depth int) (*connector.Orderbook, error) {
	rest := c.getRESTClient()

	ob, err := rest.GetOrderBook(ctx, c.settle, symbol, "", depth, true)
	if err != nil {
		return nil, err
	}

	result := &connector.Orderbook{
		ExchangeID: connector.GateIO,
		Symbol:     symbol,
		Canonical:  extractCanonical(symbol),
		Timestamp:  time.UnixMilli(ob.Current),
		SequenceID: ob.ID,
		IsSnapshot: true,
	}

	result.Bids = convertStringLevels(ob.Bids)
	result.Asks = convertStringLevels(ob.Asks)

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
func (c *GateConnector) FetchFundingRates(ctx context.Context) ([]connector.FundingRate, error) {
	rest := c.getRESTClient()

	contracts, err := rest.GetContracts(ctx, c.settle)
	if err != nil {
		return nil, err
	}

	var rates []connector.FundingRate
	for _, contract := range contracts {
		if contract.InDelisting {
			continue
		}

		fundingRate, _ := strconv.ParseFloat(contract.FundingRate, 64)

		fr := connector.FundingRate{
			ExchangeID:           connector.GateIO,
			Symbol:               contract.Name,
			Canonical:            extractCanonical(contract.Name),
			FundingRate:          fundingRate,
			NextFundingTime:      time.Unix(contract.FundingNextApply, 0),
			FundingIntervalHours: contract.FundingInterval / 3600, // Convert seconds to hours
			Timestamp:            time.Now(),
		}
		rates = append(rates, fr)
	}

	return rates, nil
}

// FetchPriceTickers fetches current prices for all symbols (Phase 1 REST)
func (c *GateConnector) FetchPriceTickers(ctx context.Context) ([]connector.PriceTicker, error) {
	rest := c.getRESTClient()

	tickers, err := rest.GetTickers(ctx, c.settle, "")
	if err != nil {
		return nil, err
	}

	var result []connector.PriceTicker
	for _, t := range tickers {
		lastPrice, _ := strconv.ParseFloat(t.Last, 64)
		bidPrice, _ := strconv.ParseFloat(t.HighestBid, 64)
		askPrice, _ := strconv.ParseFloat(t.LowestAsk, 64)
		volume, _ := strconv.ParseFloat(t.Volume24hQuote, 64)

		pt := connector.PriceTicker{
			ExchangeID: connector.GateIO,
			Symbol:     t.Contract,
			Canonical:  extractCanonical(t.Contract),
			Price:      lastPrice,
			BidPrice:   bidPrice,
			AskPrice:   askPrice,
			Volume24h:  volume,
			Timestamp:  time.Now(),
		}
		result = append(result, pt)
	}

	return result, nil
}

// FetchAssetInfo fetches deposit/withdrawal status for assets (Phase 1 REST)
func (c *GateConnector) FetchAssetInfo(ctx context.Context) ([]connector.AssetInfo, error) {
	rest := c.getRESTClient()

	currencies, err := rest.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}

	var result []connector.AssetInfo
	for _, cur := range currencies {
		ai := connector.AssetInfo{
			ExchangeID:      connector.GateIO,
			Asset:           cur.Currency,
			DepositEnabled:  !cur.DepositDisabled,
			WithdrawEnabled: !cur.WithdrawDisabled,
			Timestamp:       time.Now(),
		}

		// Extract networks
		for _, chain := range cur.Chains {
			ai.Networks = append(ai.Networks, chain.Chain)
		}

		result = append(result, ai)
	}

	return result, nil
}

// getRESTClient returns the REST client, creating one if necessary
func (c *GateConnector) getRESTClient() *RESTClient {
	if c.client != nil {
		return c.client.REST
	}
	return NewRESTClient(RESTClientConfig{
		BaseURL: "https://api.gateio.ws",
	})
}

// Helper functions

// extractCanonical converts Gate.io symbol to canonical format
// BTC_USDT -> BTC-USDT-PERP
func extractCanonical(symbol string) string {
	parts := strings.Split(symbol, "_")
	if len(parts) < 2 {
		return symbol + "-PERP"
	}
	return fmt.Sprintf("%s-%s-PERP", parts[0], parts[1])
}

// convertOrderBookEntries converts [{p, s}, ...] from WebSocket to []PriceLevel
func convertOrderBookEntries(entries []WSOrderBookEntry) []connector.PriceLevel {
	result := make([]connector.PriceLevel, 0, len(entries))
	for _, entry := range entries {
		price, _ := strconv.ParseFloat(entry.P, 64)
		result = append(result, connector.PriceLevel{
			Price:    price,
			Quantity: entry.S,
		})
	}
	return result
}

// convertStringLevels converts [[price, size], ...] from REST API to []PriceLevel
func convertStringLevels(levels [][]string) []connector.PriceLevel {
	result := make([]connector.PriceLevel, 0, len(levels))
	for _, level := range levels {
		if len(level) < 2 {
			continue
		}
		price, _ := strconv.ParseFloat(level[0], 64)
		qty, _ := strconv.ParseFloat(level[1], 64)
		result = append(result, connector.PriceLevel{
			Price:    price,
			Quantity: qty,
		})
	}
	return result
}

// abs returns absolute value of int64
func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
