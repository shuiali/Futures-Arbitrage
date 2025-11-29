package normalizer

import (
	"strings"
	"sync"

	"crossspread-md-ingest/internal/connector"
)

// InstrumentNormalizer maps exchange-specific symbols to canonical symbols
type InstrumentNormalizer struct {
	mu sync.RWMutex

	// exchangeToCanonical: exchange -> symbol -> canonical
	exchangeToCanonical map[connector.ExchangeID]map[string]string

	// canonicalToExchange: canonical -> exchange -> symbol
	canonicalToExchange map[string]map[connector.ExchangeID]string

	// instruments: canonical -> exchange -> Instrument
	instruments map[string]map[connector.ExchangeID]*connector.Instrument
}

// NewInstrumentNormalizer creates a new normalizer
func NewInstrumentNormalizer() *InstrumentNormalizer {
	return &InstrumentNormalizer{
		exchangeToCanonical: make(map[connector.ExchangeID]map[string]string),
		canonicalToExchange: make(map[string]map[connector.ExchangeID]string),
		instruments:         make(map[string]map[connector.ExchangeID]*connector.Instrument),
	}
}

// RegisterInstruments registers instruments from an exchange
func (n *InstrumentNormalizer) RegisterInstruments(instruments []connector.Instrument) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for i := range instruments {
		inst := &instruments[i]
		exchangeID := inst.ExchangeID
		symbol := inst.Symbol
		canonical := n.normalizeToCanonical(inst.BaseAsset)

		// Update instrument canonical field
		inst.Canonical = canonical

		// exchangeToCanonical mapping
		if n.exchangeToCanonical[exchangeID] == nil {
			n.exchangeToCanonical[exchangeID] = make(map[string]string)
		}
		n.exchangeToCanonical[exchangeID][symbol] = canonical

		// canonicalToExchange mapping
		if n.canonicalToExchange[canonical] == nil {
			n.canonicalToExchange[canonical] = make(map[connector.ExchangeID]string)
		}
		n.canonicalToExchange[canonical][exchangeID] = symbol

		// instruments mapping
		if n.instruments[canonical] == nil {
			n.instruments[canonical] = make(map[connector.ExchangeID]*connector.Instrument)
		}
		n.instruments[canonical][exchangeID] = inst
	}
}

// ToCanonical converts an exchange-specific symbol to canonical
func (n *InstrumentNormalizer) ToCanonical(exchangeID connector.ExchangeID, symbol string) string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if mapping, ok := n.exchangeToCanonical[exchangeID]; ok {
		if canonical, ok := mapping[symbol]; ok {
			return canonical
		}
	}

	// Fallback: extract base asset from common formats
	return n.normalizeToCanonical(n.extractBaseAsset(symbol))
}

// ToExchangeSymbol converts a canonical symbol to exchange-specific
func (n *InstrumentNormalizer) ToExchangeSymbol(canonical string, exchangeID connector.ExchangeID) string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if mapping, ok := n.canonicalToExchange[canonical]; ok {
		if symbol, ok := mapping[exchangeID]; ok {
			return symbol
		}
	}

	// Fallback: construct common format
	return n.constructExchangeSymbol(canonical, exchangeID)
}

// GetInstrument returns the instrument for a canonical symbol on an exchange
func (n *InstrumentNormalizer) GetInstrument(canonical string, exchangeID connector.ExchangeID) *connector.Instrument {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if mapping, ok := n.instruments[canonical]; ok {
		return mapping[exchangeID]
	}
	return nil
}

// GetAllExchangesForCanonical returns all exchanges that have the canonical symbol
func (n *InstrumentNormalizer) GetAllExchangesForCanonical(canonical string) []connector.ExchangeID {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var exchanges []connector.ExchangeID
	if mapping, ok := n.canonicalToExchange[canonical]; ok {
		for exchangeID := range mapping {
			exchanges = append(exchanges, exchangeID)
		}
	}
	return exchanges
}

// GetAllCanonicalSymbols returns all canonical symbols
func (n *InstrumentNormalizer) GetAllCanonicalSymbols() []string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	symbols := make([]string, 0, len(n.canonicalToExchange))
	for canonical := range n.canonicalToExchange {
		symbols = append(symbols, canonical)
	}
	return symbols
}

// GetCommonSymbols returns canonical symbols available on at least N exchanges
func (n *InstrumentNormalizer) GetCommonSymbols(minExchanges int) []string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var symbols []string
	for canonical, mapping := range n.canonicalToExchange {
		if len(mapping) >= minExchanges {
			symbols = append(symbols, canonical)
		}
	}
	return symbols
}

// normalizeToCanonical normalizes a base asset to canonical form
func (n *InstrumentNormalizer) normalizeToCanonical(baseAsset string) string {
	canonical := strings.ToUpper(strings.TrimSpace(baseAsset))

	// Handle common variations
	synonyms := map[string]string{
		"WBTC":      "BTC",
		"WETH":      "ETH",
		"WSOL":      "SOL",
		"STETH":     "ETH",
		"RETH":      "ETH",
		"1000SHIB":  "SHIB",
		"1000PEPE":  "PEPE",
		"1000FLOKI": "FLOKI",
		"1000LUNC":  "LUNC",
		"1000XEC":   "XEC",
		"USDC":      "USDC",
		"USDT":      "USDT",
		"BUSD":      "BUSD",
	}

	if normalized, ok := synonyms[canonical]; ok {
		return normalized
	}

	// Remove common prefixes like "1000"
	if strings.HasPrefix(canonical, "1000") {
		return canonical[4:]
	}

	return canonical
}

// extractBaseAsset extracts base asset from exchange symbol formats
func (n *InstrumentNormalizer) extractBaseAsset(symbol string) string {
	// Handle BTCUSDT format
	for _, quote := range []string{"USDT", "USDC", "BUSD", "USD"} {
		if strings.HasSuffix(symbol, quote) {
			return strings.TrimSuffix(symbol, quote)
		}
	}

	// Handle BTC-USDT-SWAP format (OKX)
	parts := strings.Split(symbol, "-")
	if len(parts) >= 1 {
		return parts[0]
	}

	// Handle BTC/USDT format
	parts = strings.Split(symbol, "/")
	if len(parts) >= 1 {
		return parts[0]
	}

	return symbol
}

// constructExchangeSymbol constructs exchange-specific symbol from canonical
func (n *InstrumentNormalizer) constructExchangeSymbol(canonical string, exchangeID connector.ExchangeID) string {
	switch exchangeID {
	case connector.Binance, connector.Bybit, connector.MEXC, connector.Bitget:
		return canonical + "USDT"
	case connector.OKX:
		return canonical + "-USDT-SWAP"
	case connector.KuCoin:
		return canonical + "-USDT"
	case connector.GateIO:
		return canonical + "_USDT"
	default:
		return canonical + "USDT"
	}
}

// SymbolMapping represents a symbol mapping for serialization
type SymbolMapping struct {
	Canonical string                          `json:"canonical"`
	Exchanges map[connector.ExchangeID]string `json:"exchanges"`
}

// ExportMappings exports all symbol mappings
func (n *InstrumentNormalizer) ExportMappings() []SymbolMapping {
	n.mu.RLock()
	defer n.mu.RUnlock()

	mappings := make([]SymbolMapping, 0, len(n.canonicalToExchange))
	for canonical, exchanges := range n.canonicalToExchange {
		mappings = append(mappings, SymbolMapping{
			Canonical: canonical,
			Exchanges: exchanges,
		})
	}
	return mappings
}
