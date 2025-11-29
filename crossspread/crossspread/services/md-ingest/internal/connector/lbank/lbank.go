package lbank

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"crossspread-md-ingest/internal/connector"

	"github.com/rs/zerolog/log"
)

// LBankConnector implements the Connector interface for LBank Futures
type LBankConnector struct {
	*connector.BaseConnector
	client        *Client
	subscriptions map[string]bool
	mu            sync.RWMutex
	done          chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
	depthLevels   int
}

// NewLBankConnector creates a new LBank connector
func NewLBankConnector(symbols []string, depthLevels int) *LBankConnector {
	config := connector.ConnectorConfig{
		ExchangeID:     connector.LBank,
		WsURL:          ContractWsBaseURL,
		RestURL:        ContractRestBaseURL,
		Symbols:        symbols,
		DepthLevels:    depthLevels,
		ReconnectDelay: 5 * time.Second,
		PingInterval:   30 * time.Second,
	}

	c := &LBankConnector{
		BaseConnector: connector.NewBaseConnector(config),
		client:        NewClient(DefaultClientConfig()),
		subscriptions: make(map[string]bool),
		done:          make(chan struct{}),
		depthLevels:   depthLevels,
	}

	for _, s := range symbols {
		c.subscriptions[s] = true
	}

	return c
}

// Connect establishes WebSocket connection to LBank
func (c *LBankConnector) Connect(ctx context.Context) error {
	log.Info().Str("url", ContractWsBaseURL).Msg("Connecting to LBank WebSocket")

	c.ctx, c.cancel = context.WithCancel(ctx)

	// Create market data handler that emits orderbook updates
	handler := &MarketDataHandler{
		OnDepth: func(symbol string, asks, bids [][]float64, timestamp time.Time) {
			c.handleDepthUpdate(symbol, asks, bids, timestamp)
		},
		OnTrade: func(symbol string, price, volume float64, side string, timestamp time.Time) {
			c.handleTradeUpdate(symbol, price, volume, side, timestamp)
		},
		OnTicker: func(symbol string, ticker *WsTickResponse) {
			c.handleTickerUpdate(symbol, ticker)
		},
		OnError: func(err error) {
			c.EmitError(err)
		},
		OnConnect: func() {
			log.Info().Msg("LBank market data WebSocket connected")
		},
		OnDisconnect: func() {
			log.Warn().Msg("LBank market data WebSocket disconnected")
			c.SetConnected(false)
		},
	}

	// Connect market data WebSocket
	if err := c.client.ConnectMarketData(c.ctx, handler); err != nil {
		return fmt.Errorf("failed to connect market data WebSocket: %w", err)
	}

	c.SetConnected(true)

	// Subscribe to orderbook depth for all symbols
	symbols := make([]string, 0, len(c.subscriptions))
	for symbol := range c.subscriptions {
		symbols = append(symbols, symbol)
	}

	if len(symbols) > 0 {
		if err := c.client.SubscribeDepth(symbols, c.depthLevels); err != nil {
			log.Error().Err(err).Msg("Failed to subscribe to depth")
		}
	}

	// Start auto-reconnect
	c.client.StartAutoReconnect(c.ctx)

	return nil
}

// Disconnect closes the WebSocket connection
func (c *LBankConnector) Disconnect() error {
	if c.cancel != nil {
		c.cancel()
	}
	close(c.done)
	c.SetConnected(false)
	return c.client.Close()
}

// Subscribe adds symbol subscriptions
func (c *LBankConnector) Subscribe(symbols []string) error {
	c.mu.Lock()
	for _, s := range symbols {
		c.subscriptions[s] = true
	}
	c.mu.Unlock()

	if c.client.IsMarketDataConnected() {
		return c.client.SubscribeDepth(symbols, c.depthLevels)
	}
	return nil
}

// Unsubscribe removes symbol subscriptions
func (c *LBankConnector) Unsubscribe(symbols []string) error {
	c.mu.Lock()
	for _, s := range symbols {
		delete(c.subscriptions, s)
	}
	c.mu.Unlock()

	if c.client.IsMarketDataConnected() {
		return c.client.UnsubscribeDepth(symbols, c.depthLevels)
	}
	return nil
}

// FetchInstruments fetches all perpetual futures via REST API
func (c *LBankConnector) FetchInstruments(ctx context.Context) ([]connector.Instrument, error) {
	instruments, err := c.client.GetContractInstruments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch instruments: %w", err)
	}

	var result []connector.Instrument
	for _, inst := range instruments {
		// Parse tick sizes from priceTick and volumeTick
		tickSize := inst.PriceTick
		if tickSize == 0 {
			tickSize = 0.01 // Default
		}
		lotSize := inst.VolumeTick
		if lotSize == 0 {
			lotSize = 0.001 // Default
		}

		// Default fees (LBank typical fees)
		makerFee := 0.0002 // 0.02%
		takerFee := 0.0004 // 0.04%

		contractSize := inst.VolumeMultiple
		if contractSize == 0 {
			contractSize = 1
		}

		// Extract base and quote from symbol (e.g., BTCUSDT -> BTC, USDT)
		base := inst.BaseCurrency
		quote := inst.ClearCurrency
		if base == "" || quote == "" {
			base, quote = parseContractSymbol(inst.Symbol)
		}

		result = append(result, connector.Instrument{
			ExchangeID:     connector.LBank,
			Symbol:         inst.Symbol,
			Canonical:      fmt.Sprintf("%s-%s-PERP", base, quote),
			BaseAsset:      base,
			QuoteAsset:     quote,
			InstrumentType: "perpetual",
			TickSize:       tickSize,
			LotSize:        lotSize,
			ContractSize:   contractSize,
			TakerFee:       takerFee,
			MakerFee:       makerFee,
		})
	}

	return result, nil
}

// FetchOrderbookSnapshot fetches orderbook via REST API
func (c *LBankConnector) FetchOrderbookSnapshot(ctx context.Context, symbol string, depth int) (*connector.Orderbook, error) {
	ob, err := c.client.GetContractOrderbook(ctx, symbol, depth)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch orderbook: %w", err)
	}

	result := &connector.Orderbook{
		ExchangeID: connector.LBank,
		Symbol:     symbol,
		Canonical:  extractCanonical(symbol),
		Timestamp:  time.Now(),
		IsSnapshot: true,
	}

	// Convert bids - ContractOrderbookLevel has Price and Volume fields
	for _, bid := range ob.Bids {
		if bid.Volume > 0 {
			result.Bids = append(result.Bids, connector.PriceLevel{
				Price:    bid.Price,
				Quantity: bid.Volume,
			})
		}
	}

	// Convert asks
	for _, ask := range ob.Asks {
		if ask.Volume > 0 {
			result.Asks = append(result.Asks, connector.PriceLevel{
				Price:    ask.Price,
				Quantity: ask.Volume,
			})
		}
	}

	// Sort bids desc, asks asc
	sort.Slice(result.Bids, func(i, j int) bool {
		return result.Bids[i].Price > result.Bids[j].Price
	})
	sort.Slice(result.Asks, func(i, j int) bool {
		return result.Asks[i].Price < result.Asks[j].Price
	})

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
func (c *LBankConnector) FetchFundingRates(ctx context.Context) ([]connector.FundingRate, error) {
	// Get market data which includes funding rates
	marketData, err := c.client.GetContractMarketData(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch funding rates: %w", err)
	}

	var rates []connector.FundingRate
	for _, data := range marketData {
		rate, _ := strconv.ParseFloat(data.PrePositionFeeRate, 64)

		rates = append(rates, connector.FundingRate{
			ExchangeID:           connector.LBank,
			Symbol:               data.Symbol,
			Canonical:            extractCanonical(data.Symbol),
			FundingRate:          rate,
			NextFundingTime:      time.Now().Add(8 * time.Hour), // Default 8h funding interval
			FundingIntervalHours: 8,
			Timestamp:            time.Now(),
		})
	}

	return rates, nil
}

// FetchPriceTickers fetches current prices for all symbols via REST API
func (c *LBankConnector) FetchPriceTickers(ctx context.Context) ([]connector.PriceTicker, error) {
	marketData, err := c.client.GetContractMarketData(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch price tickers: %w", err)
	}

	var tickers []connector.PriceTicker
	for _, data := range marketData {
		price, _ := strconv.ParseFloat(data.LastPrice, 64)
		volume, _ := strconv.ParseFloat(data.Volume, 64)

		// Calculate bid/ask from last price if not available
		bidPrice := price * 0.9999
		askPrice := price * 1.0001

		tickers = append(tickers, connector.PriceTicker{
			ExchangeID: connector.LBank,
			Symbol:     data.Symbol,
			Canonical:  extractCanonical(data.Symbol),
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
func (c *LBankConnector) FetchAssetInfo(ctx context.Context) ([]connector.AssetInfo, error) {
	// LBank spot API provides asset info
	configs, err := c.client.GetSpotAssetConfigs(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to fetch spot asset configs")
		return []connector.AssetInfo{}, nil
	}

	var result []connector.AssetInfo
	for _, cfg := range configs {
		fee, _ := cfg.Fee.Float64()
		minWithdraw, _ := cfg.MinWithdraw.Float64()

		result = append(result, connector.AssetInfo{
			ExchangeID:      connector.LBank,
			Asset:           cfg.AssetCode,
			DepositEnabled:  cfg.CanDeposit,
			WithdrawEnabled: cfg.CanWithdraw,
			WithdrawFee:     fee,
			MinWithdraw:     minWithdraw,
			Networks:        []string{cfg.Chain},
			Timestamp:       time.Now(),
		})
	}

	return result, nil
}

// ConnectForSymbols establishes WebSocket connection for specific symbols only
func (c *LBankConnector) ConnectForSymbols(ctx context.Context, symbols []string) error {
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

// handleDepthUpdate processes depth updates from WebSocket
func (c *LBankConnector) handleDepthUpdate(symbol string, asks, bids [][]float64, timestamp time.Time) {
	ob := &connector.Orderbook{
		ExchangeID: connector.LBank,
		Symbol:     symbol,
		Canonical:  extractCanonical(symbol),
		Timestamp:  timestamp,
		IsSnapshot: true,
	}

	// Parse bids
	for _, bid := range bids {
		if len(bid) >= 2 && bid[1] > 0 {
			ob.Bids = append(ob.Bids, connector.PriceLevel{
				Price:    bid[0],
				Quantity: bid[1],
			})
		}
	}

	// Parse asks
	for _, ask := range asks {
		if len(ask) >= 2 && ask[1] > 0 {
			ob.Asks = append(ob.Asks, connector.PriceLevel{
				Price:    ask[0],
				Quantity: ask[1],
			})
		}
	}

	// Sort bids desc, asks asc
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

// handleTradeUpdate processes trade updates from WebSocket
func (c *LBankConnector) handleTradeUpdate(symbol string, price, volume float64, side string, timestamp time.Time) {
	trade := &connector.Trade{
		ExchangeID: connector.LBank,
		Symbol:     symbol,
		Canonical:  extractCanonical(symbol),
		TradeID:    fmt.Sprintf("%d", timestamp.UnixNano()),
		Price:      price,
		Quantity:   volume,
		Side:       strings.ToLower(side),
		Timestamp:  timestamp,
	}

	c.EmitTrade(trade)
}

// handleTickerUpdate processes ticker updates from WebSocket
func (c *LBankConnector) handleTickerUpdate(symbol string, ticker *WsTickResponse) {
	// Ticker updates can be used to update funding handler if needed
	if ticker == nil {
		return
	}

	// We could emit a funding rate update here if the data contains funding info
	// For now, just log it
	log.Debug().
		Str("pair", symbol).
		Float64("latest_price", ticker.Tick.Latest).
		Msg("LBank ticker update")
}

// parseContractSymbol parses contract symbol like BTCUSDT into base and quote
func parseContractSymbol(symbol string) (base, quote string) {
	// Common quote currencies
	quotes := []string{"USDT", "USDC", "USD", "BTC", "ETH"}
	symbol = strings.ToUpper(symbol)

	for _, q := range quotes {
		if strings.HasSuffix(symbol, q) {
			return symbol[:len(symbol)-len(q)], q
		}
	}

	// Default fallback
	if len(symbol) > 4 {
		return symbol[:len(symbol)-4], symbol[len(symbol)-4:]
	}
	return symbol, "USDT"
}

// extractCanonical extracts canonical name from LBank symbol
func extractCanonical(symbol string) string {
	// For contract symbols like BTCUSDT
	base, quote := parseContractSymbol(symbol)
	return fmt.Sprintf("%s-%s-PERP", base, quote)
}
