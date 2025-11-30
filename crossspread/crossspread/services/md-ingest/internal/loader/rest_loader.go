package loader

import (
	"context"
	"fmt"
	"sync"
	"time"

	"crossspread-md-ingest/internal/connector"
	"crossspread-md-ingest/internal/metrics"

	"github.com/rs/zerolog/log"
)

// ExchangeData holds all data fetched from a single exchange via REST
type ExchangeData struct {
	ExchangeID   connector.ExchangeID    `json:"exchange_id"`
	Instruments  []connector.Instrument  `json:"instruments"`
	Tickers      []connector.PriceTicker `json:"tickers"`
	FundingRates []connector.FundingRate `json:"funding_rates"`
	AssetInfo    []connector.AssetInfo   `json:"asset_info"`
	FetchedAt    time.Time               `json:"fetched_at"`
}

// TokenData aggregates data for a single token across all exchanges
type TokenData struct {
	Canonical string                                      `json:"canonical"`
	Exchanges map[connector.ExchangeID]*ExchangeTokenData `json:"exchanges"`
}

// ExchangeTokenData holds token-specific data for a single exchange
type ExchangeTokenData struct {
	ExchangeID      connector.ExchangeID `json:"exchange_id"`
	Symbol          string               `json:"symbol"`
	Price           float64              `json:"price"`
	BidPrice        float64              `json:"bid_price,omitempty"`
	AskPrice        float64              `json:"ask_price,omitempty"`
	FundingRate     float64              `json:"funding_rate,omitempty"`
	MakerFee        float64              `json:"maker_fee"`
	TakerFee        float64              `json:"taker_fee"`
	DepositEnabled  bool                 `json:"deposit_enabled"`
	WithdrawEnabled bool                 `json:"withdraw_enabled"`
	TickSize        float64              `json:"tick_size"`
	LotSize         float64              `json:"lot_size"`
	MinNotional     float64              `json:"min_notional"`
	Volume24h       float64              `json:"volume_24h,omitempty"`
}

// RestPreliminarySpread represents a preliminary spread discovered from REST data
type RestPreliminarySpread struct {
	Canonical     string               `json:"canonical"`
	LongExchange  connector.ExchangeID `json:"long_exchange"`
	ShortExchange connector.ExchangeID `json:"short_exchange"`
	LongSymbol    string               `json:"long_symbol"`
	ShortSymbol   string               `json:"short_symbol"`
	LongPrice     float64              `json:"long_price"`
	ShortPrice    float64              `json:"short_price"`
	SpreadPercent float64              `json:"spread_percent"`
	SpreadBps     float64              `json:"spread_bps"`
	LongFunding   float64              `json:"long_funding"`
	ShortFunding  float64              `json:"short_funding"`
	NetFunding    float64              `json:"net_funding"`
	LongDeposit   bool                 `json:"long_deposit_enabled"`
	ShortWithdraw bool                 `json:"short_withdraw_enabled"`
	EstimatedPnL  float64              `json:"estimated_pnl_bps"` // After fees
	DiscoveredAt  time.Time            `json:"discovered_at"`
}

// RestDataLoader handles Phase 1: loading all data from REST APIs
type RestDataLoader struct {
	connectors []connector.Connector
	mu         sync.RWMutex

	// Cached data
	exchangeData map[connector.ExchangeID]*ExchangeData
	tokenData    map[string]*TokenData // canonical -> data
	spreads      []*RestPreliminarySpread

	// Config
	minSpreadBps    float64
	refreshInterval time.Duration
	parallelFetch   bool
}

// NewRestDataLoader creates a new REST data loader
func NewRestDataLoader(connectors []connector.Connector) *RestDataLoader {
	return &RestDataLoader{
		connectors:      connectors,
		exchangeData:    make(map[connector.ExchangeID]*ExchangeData),
		tokenData:       make(map[string]*TokenData),
		spreads:         make([]*RestPreliminarySpread, 0),
		minSpreadBps:    1.0, // Minimum 0.01% spread to consider (lowered from 5.0)
		refreshInterval: 30 * time.Second,
		parallelFetch:   true,
	}
}

// SetMinSpreadBps sets the minimum spread in basis points
func (l *RestDataLoader) SetMinSpreadBps(bps float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minSpreadBps = bps
}

// LoadAll fetches data from all exchanges via REST APIs
// This is Phase 1 of the two-phase approach
func (l *RestDataLoader) LoadAll(ctx context.Context) error {
	log.Info().Int("exchanges", len(l.connectors)).Msg("Phase 1: Loading data from REST APIs")
	startTime := time.Now()

	var err error
	if l.parallelFetch {
		err = l.loadAllParallel(ctx)
	} else {
		err = l.loadAllSequential(ctx)
	}

	log.Info().
		Dur("duration", time.Since(startTime)).
		Int("exchanges", len(l.exchangeData)).
		Msg("Phase 1: REST data loading complete")

	return err
}

// loadAllParallel fetches from all exchanges in parallel
func (l *RestDataLoader) loadAllParallel(ctx context.Context) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(l.connectors))
	dataCh := make(chan *ExchangeData, len(l.connectors))

	for _, conn := range l.connectors {
		wg.Add(1)
		go func(c connector.Connector) {
			defer wg.Done()

			data, err := l.fetchExchangeData(ctx, c)
			if err != nil {
				log.Error().
					Err(err).
					Str("exchange", string(c.ID())).
					Msg("Failed to fetch exchange data")
				errCh <- fmt.Errorf("%s: %w", c.ID(), err)
				return
			}
			dataCh <- data
		}(conn)
	}

	// Wait for all goroutines
	wg.Wait()
	close(errCh)
	close(dataCh)

	// Collect results
	for data := range dataCh {
		l.mu.Lock()
		l.exchangeData[data.ExchangeID] = data
		l.mu.Unlock()
	}

	// Aggregate by token
	l.aggregateByToken()

	// Discover preliminary spreads
	l.discoverSpreads()

	// Log any errors (non-fatal, we continue with available data)
	for err := range errCh {
		log.Warn().Err(err).Msg("Exchange fetch error (non-fatal)")
	}

	return nil
}

// loadAllSequential fetches from exchanges one by one
func (l *RestDataLoader) loadAllSequential(ctx context.Context) error {
	for _, conn := range l.connectors {
		data, err := l.fetchExchangeData(ctx, conn)
		if err != nil {
			log.Error().
				Err(err).
				Str("exchange", string(conn.ID())).
				Msg("Failed to fetch exchange data (continuing)")
			continue
		}

		l.mu.Lock()
		l.exchangeData[data.ExchangeID] = data
		l.mu.Unlock()
	}

	l.aggregateByToken()
	l.discoverSpreads()

	return nil
}

// fetchExchangeData fetches all data from a single exchange
func (l *RestDataLoader) fetchExchangeData(ctx context.Context, conn connector.Connector) (*ExchangeData, error) {
	timer := metrics.NewTimer()
	exchangeID := conn.ID()

	log.Debug().Str("exchange", string(exchangeID)).Msg("Fetching exchange data")

	data := &ExchangeData{
		ExchangeID: exchangeID,
		FetchedAt:  time.Now(),
	}

	// Fetch instruments (exchange info)
	instruments, err := conn.FetchInstruments(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch instruments: %w", err)
	}
	data.Instruments = instruments
	log.Debug().
		Str("exchange", string(exchangeID)).
		Int("instruments", len(instruments)).
		Msg("Fetched instruments")

	// Fetch price tickers
	tickers, err := conn.FetchPriceTickers(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch tickers: %w", err)
	}
	data.Tickers = tickers
	log.Debug().
		Str("exchange", string(exchangeID)).
		Int("tickers", len(tickers)).
		Msg("Fetched price tickers")

	// Fetch funding rates
	fundingRates, err := conn.FetchFundingRates(ctx)
	if err != nil {
		log.Warn().Err(err).Str("exchange", string(exchangeID)).Msg("Failed to fetch funding rates")
		// Non-fatal, continue
	} else {
		data.FundingRates = fundingRates
	}

	// Fetch asset info (deposit/withdrawal status)
	assetInfo, err := conn.FetchAssetInfo(ctx)
	if err != nil {
		log.Warn().Err(err).Str("exchange", string(exchangeID)).Msg("Failed to fetch asset info")
		// Non-fatal, continue
	} else {
		data.AssetInfo = assetInfo
	}

	// Record REST fetch duration
	timer.ObserveDuration(metrics.RestFetchDuration, string(exchangeID), "all")

	return data, nil
}

// aggregateByToken aggregates exchange data by canonical token
func (l *RestDataLoader) aggregateByToken() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.tokenData = make(map[string]*TokenData)

	// Build maps for quick lookup
	for exchID, exchData := range l.exchangeData {
		// Create instrument map
		instrumentMap := make(map[string]*connector.Instrument)
		for i := range exchData.Instruments {
			inst := &exchData.Instruments[i]
			instrumentMap[inst.Symbol] = inst
		}

		// Create funding rate map
		fundingMap := make(map[string]float64)
		for _, fr := range exchData.FundingRates {
			fundingMap[fr.Symbol] = fr.FundingRate
		}

		// Create asset info map
		assetInfoMap := make(map[string]*connector.AssetInfo)
		for i := range exchData.AssetInfo {
			ai := &exchData.AssetInfo[i]
			assetInfoMap[ai.Asset] = ai
		}

		// Process tickers
		for _, ticker := range exchData.Tickers {
			canonical := ticker.Canonical
			if canonical == "" {
				continue
			}

			// Get or create token data
			td, ok := l.tokenData[canonical]
			if !ok {
				td = &TokenData{
					Canonical: canonical,
					Exchanges: make(map[connector.ExchangeID]*ExchangeTokenData),
				}
				l.tokenData[canonical] = td
			}

			// Build exchange token data
			etd := &ExchangeTokenData{
				ExchangeID: exchID,
				Symbol:     ticker.Symbol,
				Price:      ticker.Price,
				BidPrice:   ticker.BidPrice,
				AskPrice:   ticker.AskPrice,
				Volume24h:  ticker.Volume24h,
			}

			// Add instrument info
			if inst, ok := instrumentMap[ticker.Symbol]; ok {
				etd.MakerFee = inst.MakerFee
				etd.TakerFee = inst.TakerFee
				etd.TickSize = inst.TickSize
				etd.LotSize = inst.LotSize
				etd.MinNotional = inst.MinNotional
			}

			// Add funding rate
			if fr, ok := fundingMap[ticker.Symbol]; ok {
				etd.FundingRate = fr
			}

			// Add asset info (deposit/withdrawal)
			if ai, ok := assetInfoMap[canonical]; ok {
				etd.DepositEnabled = ai.DepositEnabled
				etd.WithdrawEnabled = ai.WithdrawEnabled
			} else {
				// Default to enabled if no info
				etd.DepositEnabled = true
				etd.WithdrawEnabled = true
			}

			td.Exchanges[exchID] = etd
		}
	}

	log.Info().
		Int("tokens", len(l.tokenData)).
		Msg("Aggregated token data across exchanges")
}

// discoverSpreads finds preliminary spread opportunities from REST data
func (l *RestDataLoader) discoverSpreads() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.spreads = make([]*RestPreliminarySpread, 0)

	for canonical, td := range l.tokenData {
		// Need at least 2 exchanges
		if len(td.Exchanges) < 2 {
			continue
		}

		// Get list of exchanges
		exchanges := make([]connector.ExchangeID, 0, len(td.Exchanges))
		for exchID := range td.Exchanges {
			exchanges = append(exchanges, exchID)
		}

		// Check all pairs
		for i := 0; i < len(exchanges); i++ {
			for j := 0; j < len(exchanges); j++ {
				if i == j {
					continue
				}

				longExch := exchanges[i]
				shortExch := exchanges[j]
				longData := td.Exchanges[longExch]
				shortData := td.Exchanges[shortExch]

				// Use ask price for long (buy), bid price for short (sell)
				// If bid/ask not available, use mid price
				longPrice := longData.AskPrice
				if longPrice == 0 {
					longPrice = longData.Price
				}
				shortPrice := shortData.BidPrice
				if shortPrice == 0 {
					shortPrice = shortData.Price
				}

				if longPrice <= 0 || shortPrice <= 0 {
					continue
				}

				// Calculate spread
				spreadPercent := (shortPrice - longPrice) / longPrice * 100
				spreadBps := spreadPercent * 100

				// Skip negative or too small spreads
				if spreadBps < l.minSpreadBps {
					continue
				}

				// Calculate fees impact
				totalFees := longData.TakerFee + shortData.TakerFee
				estimatedPnL := spreadBps - (totalFees * 10000) // Convert fees to bps

				spread := &RestPreliminarySpread{
					Canonical:     canonical,
					LongExchange:  longExch,
					ShortExchange: shortExch,
					LongSymbol:    longData.Symbol,
					ShortSymbol:   shortData.Symbol,
					LongPrice:     longPrice,
					ShortPrice:    shortPrice,
					SpreadPercent: spreadPercent,
					SpreadBps:     spreadBps,
					LongFunding:   longData.FundingRate,
					ShortFunding:  shortData.FundingRate,
					NetFunding:    shortData.FundingRate - longData.FundingRate,
					LongDeposit:   longData.DepositEnabled,
					ShortWithdraw: shortData.WithdrawEnabled,
					EstimatedPnL:  estimatedPnL,
					DiscoveredAt:  time.Now(),
				}

				l.spreads = append(l.spreads, spread)
			}
		}
	}

	log.Info().
		Int("spreads", len(l.spreads)).
		Float64("min_bps", l.minSpreadBps).
		Msg("Discovered preliminary spreads from REST data")
}

// GetDiscoveredSpreads returns the preliminary spreads found
func (l *RestDataLoader) GetDiscoveredSpreads() []*RestPreliminarySpread {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*RestPreliminarySpread, len(l.spreads))
	copy(result, l.spreads)
	return result
}

// GetSymbolsForWebSocket returns the unique symbols that need WebSocket subscription
// This is used for Phase 2: selective WebSocket connection
func (l *RestDataLoader) GetSymbolsForWebSocket() map[connector.ExchangeID][]string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make(map[connector.ExchangeID][]string)
	symbolSets := make(map[connector.ExchangeID]map[string]bool)

	for _, spread := range l.spreads {
		// Long exchange symbol
		if symbolSets[spread.LongExchange] == nil {
			symbolSets[spread.LongExchange] = make(map[string]bool)
		}
		symbolSets[spread.LongExchange][spread.LongSymbol] = true

		// Short exchange symbol
		if symbolSets[spread.ShortExchange] == nil {
			symbolSets[spread.ShortExchange] = make(map[string]bool)
		}
		symbolSets[spread.ShortExchange][spread.ShortSymbol] = true
	}

	// Convert sets to slices
	for exchID, symbols := range symbolSets {
		symbolList := make([]string, 0, len(symbols))
		for s := range symbols {
			symbolList = append(symbolList, s)
		}
		result[exchID] = symbolList
	}

	return result
}

// GetTokenData returns aggregated token data
func (l *RestDataLoader) GetTokenData() map[string]*TokenData {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make(map[string]*TokenData, len(l.tokenData))
	for k, v := range l.tokenData {
		result[k] = v
	}
	return result
}

// GetExchangeData returns raw exchange data
func (l *RestDataLoader) GetExchangeData() map[connector.ExchangeID]*ExchangeData {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make(map[connector.ExchangeID]*ExchangeData, len(l.exchangeData))
	for k, v := range l.exchangeData {
		result[k] = v
	}
	return result
}

// Refresh reloads all data from REST APIs
func (l *RestDataLoader) Refresh(ctx context.Context) error {
	return l.LoadAll(ctx)
}

// StartPeriodicRefresh starts a background goroutine to refresh data periodically
func (l *RestDataLoader) StartPeriodicRefresh(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(l.refreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := l.Refresh(ctx); err != nil {
					log.Error().Err(err).Msg("Periodic refresh failed")
				}
			}
		}
	}()
}
