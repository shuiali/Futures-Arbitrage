package spread

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"crossspread-md-ingest/internal/connector"
	"crossspread-md-ingest/internal/normalizer"
	"crossspread-md-ingest/internal/publisher"

	"github.com/rs/zerolog/log"
)

// SpreadOpportunity represents an arbitrage spread opportunity
type SpreadOpportunity struct {
	ID            string               `json:"id"`
	Canonical     string               `json:"canonical"`      // e.g., "BTC"
	LongExchange  connector.ExchangeID `json:"long_exchange"`  // Exchange to buy
	ShortExchange connector.ExchangeID `json:"short_exchange"` // Exchange to sell
	LongSymbol    string               `json:"long_symbol"`
	ShortSymbol   string               `json:"short_symbol"`
	LongPrice     float64              `json:"long_price"`      // Best ask on long exchange
	ShortPrice    float64              `json:"short_price"`     // Best bid on short exchange
	SpreadPercent float64              `json:"spread_percent"`  // (short - long) / long * 100
	SpreadBps     float64              `json:"spread_bps"`      // Spread in basis points
	LongFunding   float64              `json:"long_funding"`    // Funding rate on long
	ShortFunding  float64              `json:"short_funding"`   // Funding rate on short
	NetFunding    float64              `json:"net_funding"`     // short_funding - long_funding
	LongDepthUSD  float64              `json:"long_depth_usd"`  // Top 5 levels depth
	ShortDepthUSD float64              `json:"short_depth_usd"` // Top 5 levels depth
	MinDepthUSD   float64              `json:"min_depth_usd"`   // Min of both sides
	Volume24h     float64              `json:"volume_24h"`      // Combined volume
	Score         float64              `json:"score"`           // Opportunity score
	UpdatedAt     time.Time            `json:"updated_at"`
}

// SpreadDiscovery discovers and tracks arbitrage opportunities
type SpreadDiscovery struct {
	mu sync.RWMutex

	normalizer *normalizer.InstrumentNormalizer
	publisher  *publisher.RedisPublisher

	// Current orderbooks per exchange per canonical symbol
	orderbooks map[string]map[connector.ExchangeID]*connector.Orderbook

	// Current funding rates per exchange per canonical symbol
	fundingRates map[string]map[connector.ExchangeID]float64

	// Current spread opportunities
	spreads map[string]*SpreadOpportunity // key: "canonical:longExchange:shortExchange"

	// Configuration
	minSpreadBps    float64 // Minimum spread in bps to consider
	minDepthUSD     float64 // Minimum depth in USD
	updateInterval  time.Duration
	publishInterval time.Duration

	done chan struct{}
}

// NewSpreadDiscovery creates a new spread discovery service
func NewSpreadDiscovery(
	normalizer *normalizer.InstrumentNormalizer,
	publisher *publisher.RedisPublisher,
) *SpreadDiscovery {
	return &SpreadDiscovery{
		normalizer:      normalizer,
		publisher:       publisher,
		orderbooks:      make(map[string]map[connector.ExchangeID]*connector.Orderbook),
		fundingRates:    make(map[string]map[connector.ExchangeID]float64),
		spreads:         make(map[string]*SpreadOpportunity),
		minSpreadBps:    5.0,  // Minimum 0.05% spread
		minDepthUSD:     5000, // Minimum $5k depth
		updateInterval:  100 * time.Millisecond,
		publishInterval: 500 * time.Millisecond,
		done:            make(chan struct{}),
	}
}

// Start starts the spread discovery service
func (s *SpreadDiscovery) Start(ctx context.Context) {
	publishTicker := time.NewTicker(s.publishInterval)
	defer publishTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-publishTicker.C:
			s.publishSpreads()
		}
	}
}

// Stop stops the spread discovery service
func (s *SpreadDiscovery) Stop() {
	close(s.done)
}

// HandleOrderbook processes an orderbook update
func (s *SpreadDiscovery) HandleOrderbook(ob *connector.Orderbook) {
	s.mu.Lock()
	defer s.mu.Unlock()

	canonical := ob.Canonical
	exchangeID := ob.ExchangeID

	// Store orderbook
	if s.orderbooks[canonical] == nil {
		s.orderbooks[canonical] = make(map[connector.ExchangeID]*connector.Orderbook)
	}
	s.orderbooks[canonical][exchangeID] = ob

	// Recalculate spreads for this canonical symbol
	s.recalculateSpreads(canonical)
}

// HandleFundingRate processes a funding rate update
func (s *SpreadDiscovery) HandleFundingRate(fr *connector.FundingRate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	canonical := fr.Canonical
	exchangeID := fr.ExchangeID

	if s.fundingRates[canonical] == nil {
		s.fundingRates[canonical] = make(map[connector.ExchangeID]float64)
	}
	s.fundingRates[canonical][exchangeID] = fr.FundingRate
}

// recalculateSpreads recalculates all spreads for a canonical symbol
func (s *SpreadDiscovery) recalculateSpreads(canonical string) {
	exchanges, ok := s.orderbooks[canonical]
	if !ok || len(exchanges) < 2 {
		return
	}

	// Get all exchange orderbooks
	var exchangeIDs []connector.ExchangeID
	for id := range exchanges {
		exchangeIDs = append(exchangeIDs, id)
	}

	// Check all pairs of exchanges
	for i := 0; i < len(exchangeIDs); i++ {
		for j := i + 1; j < len(exchangeIDs); j++ {
			ob1 := exchanges[exchangeIDs[i]]
			ob2 := exchanges[exchangeIDs[j]]

			// Check both directions
			s.checkSpread(canonical, ob1, ob2)
			s.checkSpread(canonical, ob2, ob1)
		}
	}
}

// checkSpread checks if there's a profitable spread between two orderbooks
// longOb is where we buy (use ask price), shortOb is where we sell (use bid price)
func (s *SpreadDiscovery) checkSpread(canonical string, longOb, shortOb *connector.Orderbook) {
	if len(longOb.Asks) == 0 || len(shortOb.Bids) == 0 {
		return
	}

	longPrice := longOb.Asks[0].Price   // Buy at ask
	shortPrice := shortOb.Bids[0].Price // Sell at bid

	if longPrice <= 0 || shortPrice <= 0 {
		return
	}

	spreadPercent := (shortPrice - longPrice) / longPrice * 100
	spreadBps := spreadPercent * 100

	// Skip if spread is too small
	if spreadBps < s.minSpreadBps {
		return
	}

	// Calculate depth
	longDepth := s.calculateDepthUSD(longOb.Asks)
	shortDepth := s.calculateDepthUSD(shortOb.Bids)
	minDepth := math.Min(longDepth, shortDepth)

	// Skip if depth is too small
	if minDepth < s.minDepthUSD {
		return
	}

	// Get funding rates
	var longFunding, shortFunding float64
	if rates, ok := s.fundingRates[canonical]; ok {
		longFunding = rates[longOb.ExchangeID]
		shortFunding = rates[shortOb.ExchangeID]
	}

	// Calculate opportunity score
	// Higher spread, better funding, more depth = higher score
	score := spreadBps * math.Log10(minDepth+1) * (1 + (shortFunding-longFunding)*100)

	spreadID := fmt.Sprintf("%s:%s:%s", canonical, longOb.ExchangeID, shortOb.ExchangeID)

	opportunity := &SpreadOpportunity{
		ID:            spreadID,
		Canonical:     canonical,
		LongExchange:  longOb.ExchangeID,
		ShortExchange: shortOb.ExchangeID,
		LongSymbol:    longOb.Symbol,
		ShortSymbol:   shortOb.Symbol,
		LongPrice:     longPrice,
		ShortPrice:    shortPrice,
		SpreadPercent: spreadPercent,
		SpreadBps:     spreadBps,
		LongFunding:   longFunding,
		ShortFunding:  shortFunding,
		NetFunding:    shortFunding - longFunding,
		LongDepthUSD:  longDepth,
		ShortDepthUSD: shortDepth,
		MinDepthUSD:   minDepth,
		Score:         score,
		UpdatedAt:     time.Now(),
	}

	s.spreads[spreadID] = opportunity
}

// calculateDepthUSD calculates depth in USD for top N levels
func (s *SpreadDiscovery) calculateDepthUSD(levels []connector.PriceLevel) float64 {
	var total float64
	for i, level := range levels {
		if i >= 5 { // Top 5 levels
			break
		}
		total += level.Price * level.Quantity
	}
	return total
}

// GetTopSpreads returns top N spreads sorted by score
func (s *SpreadDiscovery) GetTopSpreads(n int) []*SpreadOpportunity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	spreads := make([]*SpreadOpportunity, 0, len(s.spreads))
	for _, spread := range s.spreads {
		spreads = append(spreads, spread)
	}

	// Sort by score descending
	sort.Slice(spreads, func(i, j int) bool {
		return spreads[i].Score > spreads[j].Score
	})

	if n > len(spreads) {
		n = len(spreads)
	}
	return spreads[:n]
}

// GetSpreadsByCanonical returns all spreads for a canonical symbol
func (s *SpreadDiscovery) GetSpreadsByCanonical(canonical string) []*SpreadOpportunity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var spreads []*SpreadOpportunity
	for _, spread := range s.spreads {
		if spread.Canonical == canonical {
			spreads = append(spreads, spread)
		}
	}

	sort.Slice(spreads, func(i, j int) bool {
		return spreads[i].Score > spreads[j].Score
	})

	return spreads
}

// publishSpreads publishes current spreads to Redis
func (s *SpreadDiscovery) publishSpreads() {
	topSpreads := s.GetTopSpreads(100)

	for _, spread := range topSpreads {
		data, err := json.Marshal(spread)
		if err != nil {
			log.Error().Err(err).Str("spread", spread.ID).Msg("Failed to marshal spread")
			continue
		}

		// Store spread as a Redis key (for backend API to read)
		if err := s.publisher.SetSpread(spread.ID, data); err != nil {
			log.Error().Err(err).Str("spread", spread.ID).Msg("Failed to store spread")
		}

		// Publish to Redis channel (for real-time WebSocket updates)
		channel := fmt.Sprintf("spread:%s", spread.Canonical)
		if err := s.publisher.Publish(channel, string(data)); err != nil {
			log.Error().Err(err).Str("channel", channel).Msg("Failed to publish spread")
		}

		// Also publish to spread-specific channel
		spreadChannel := fmt.Sprintf("spread:%s", spread.ID)
		if err := s.publisher.Publish(spreadChannel, string(data)); err != nil {
			log.Error().Err(err).Str("channel", spreadChannel).Msg("Failed to publish spread detail")
		}
	}

	// Publish summary of top spreads and store as a list
	summary := struct {
		Timestamp time.Time            `json:"timestamp"`
		Count     int                  `json:"count"`
		Top10     []*SpreadOpportunity `json:"top_10"`
		Spreads   []*SpreadOpportunity `json:"spreads"`
	}{
		Timestamp: time.Now(),
		Count:     len(topSpreads),
		Top10:     topSpreads[:min(10, len(topSpreads))],
		Spreads:   topSpreads,
	}

	data, _ := json.Marshal(summary)
	s.publisher.Publish("spreads:summary", string(data))
	s.publisher.SetSpreadsList(data)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
