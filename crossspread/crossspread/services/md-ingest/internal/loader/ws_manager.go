package loader

import (
	"context"
	"sync"
	"time"

	"crossspread-md-ingest/internal/connector"

	"github.com/rs/zerolog/log"
)

// WebSocketManager manages selective WebSocket connections based on discovered spreads
// This is Phase 2 of the two-phase approach
type WebSocketManager struct {
	connectors map[connector.ExchangeID]connector.Connector
	mu         sync.RWMutex

	// Currently subscribed symbols per exchange
	activeSymbols map[connector.ExchangeID]map[string]bool

	// Handlers
	orderbookHandler connector.OrderbookHandler
	tradeHandler     connector.TradeHandler
	fundingHandler   connector.FundingHandler
	errorHandler     connector.ErrorHandler

	done chan struct{}
}

// NewWebSocketManager creates a new selective WebSocket manager
func NewWebSocketManager(connectorList []connector.Connector) *WebSocketManager {
	connectors := make(map[connector.ExchangeID]connector.Connector)
	for _, c := range connectorList {
		connectors[c.ID()] = c
	}

	return &WebSocketManager{
		connectors:    connectors,
		activeSymbols: make(map[connector.ExchangeID]map[string]bool),
		done:          make(chan struct{}),
	}
}

// SetOrderbookHandler sets the callback for orderbook updates
func (m *WebSocketManager) SetOrderbookHandler(handler connector.OrderbookHandler) {
	m.orderbookHandler = handler
}

// SetTradeHandler sets the callback for trade updates
func (m *WebSocketManager) SetTradeHandler(handler connector.TradeHandler) {
	m.tradeHandler = handler
}

// SetFundingHandler sets the callback for funding rate updates
func (m *WebSocketManager) SetFundingHandler(handler connector.FundingHandler) {
	m.fundingHandler = handler
}

// SetErrorHandler sets the callback for errors
func (m *WebSocketManager) SetErrorHandler(handler connector.ErrorHandler) {
	m.errorHandler = handler
}

// ConnectForSpreads establishes WebSocket connections only for the symbols in discovered spreads
// symbolsByExchange: map of exchange ID to list of symbols to subscribe
func (m *WebSocketManager) ConnectForSpreads(ctx context.Context, symbolsByExchange map[connector.ExchangeID][]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Info().
		Int("exchanges", len(symbolsByExchange)).
		Msg("Phase 2: Connecting WebSockets for discovered spreads")

	var wg sync.WaitGroup
	errCh := make(chan error, len(symbolsByExchange))

	for exchID, symbols := range symbolsByExchange {
		conn, ok := m.connectors[exchID]
		if !ok {
			log.Warn().
				Str("exchange", string(exchID)).
				Msg("Connector not found for exchange")
			continue
		}

		if len(symbols) == 0 {
			continue
		}

		// Set up handlers
		m.setupHandlers(conn)

		wg.Add(1)
		go func(c connector.Connector, syms []string, eid connector.ExchangeID) {
			defer wg.Done()

			log.Info().
				Str("exchange", string(eid)).
				Int("symbols", len(syms)).
				Msg("Connecting to exchange for selected symbols")

			// Use ConnectForSymbols for selective subscription
			if err := c.ConnectForSymbols(ctx, syms); err != nil {
				log.Error().
					Err(err).
					Str("exchange", string(eid)).
					Msg("Failed to connect to exchange")
				errCh <- err
				return
			}

			// Update active symbols
			m.mu.Lock()
			m.activeSymbols[eid] = make(map[string]bool)
			for _, s := range syms {
				m.activeSymbols[eid][s] = true
			}
			m.mu.Unlock()

			log.Info().
				Str("exchange", string(eid)).
				Int("symbols", len(syms)).
				Msg("WebSocket connected successfully")
		}(conn, symbols, exchID)
	}

	wg.Wait()
	close(errCh)

	// Count connected exchanges
	connectedCount := 0
	for _, conn := range m.connectors {
		if conn.IsConnected() {
			connectedCount++
		}
	}

	log.Info().
		Int("connected", connectedCount).
		Int("requested", len(symbolsByExchange)).
		Msg("Phase 2: WebSocket connections established")

	// Return first error if any (non-fatal, some exchanges may fail)
	for err := range errCh {
		if err != nil {
			log.Warn().Err(err).Msg("Some WebSocket connections failed (non-fatal)")
		}
	}

	return nil
}

// setupHandlers configures handlers for a connector
func (m *WebSocketManager) setupHandlers(conn connector.Connector) {
	conn.SetOrderbookHandler(func(ob *connector.Orderbook) {
		if m.orderbookHandler != nil {
			m.orderbookHandler(ob)
		}
	})

	conn.SetTradeHandler(func(trade *connector.Trade) {
		if m.tradeHandler != nil {
			m.tradeHandler(trade)
		}
	})

	conn.SetFundingHandler(func(fr *connector.FundingRate) {
		if m.fundingHandler != nil {
			m.fundingHandler(fr)
		}
	})

	conn.SetErrorHandler(func(err error) {
		if m.errorHandler != nil {
			m.errorHandler(err)
		}
	})
}

// UpdateSubscriptions adds or removes symbol subscriptions dynamically
// This can be called when new spreads are discovered or old ones become unprofitable
func (m *WebSocketManager) UpdateSubscriptions(ctx context.Context, symbolsByExchange map[connector.ExchangeID][]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for exchID, newSymbols := range symbolsByExchange {
		conn, ok := m.connectors[exchID]
		if !ok {
			continue
		}

		// Get current symbols
		currentSymbols := m.activeSymbols[exchID]
		if currentSymbols == nil {
			currentSymbols = make(map[string]bool)
		}

		// Find symbols to add
		var toAdd []string
		for _, s := range newSymbols {
			if !currentSymbols[s] {
				toAdd = append(toAdd, s)
			}
		}

		// Find symbols to remove
		newSymbolSet := make(map[string]bool)
		for _, s := range newSymbols {
			newSymbolSet[s] = true
		}
		var toRemove []string
		for s := range currentSymbols {
			if !newSymbolSet[s] {
				toRemove = append(toRemove, s)
			}
		}

		// Subscribe to new symbols
		if len(toAdd) > 0 {
			if err := conn.Subscribe(toAdd); err != nil {
				log.Error().
					Err(err).
					Str("exchange", string(exchID)).
					Int("count", len(toAdd)).
					Msg("Failed to subscribe to new symbols")
			} else {
				for _, s := range toAdd {
					m.activeSymbols[exchID][s] = true
				}
				log.Info().
					Str("exchange", string(exchID)).
					Int("count", len(toAdd)).
					Msg("Subscribed to new symbols")
			}
		}

		// Unsubscribe from old symbols
		if len(toRemove) > 0 {
			if err := conn.Unsubscribe(toRemove); err != nil {
				log.Error().
					Err(err).
					Str("exchange", string(exchID)).
					Int("count", len(toRemove)).
					Msg("Failed to unsubscribe from old symbols")
			} else {
				for _, s := range toRemove {
					delete(m.activeSymbols[exchID], s)
				}
				log.Info().
					Str("exchange", string(exchID)).
					Int("count", len(toRemove)).
					Msg("Unsubscribed from old symbols")
			}
		}
	}

	return nil
}

// GetActiveSymbols returns currently subscribed symbols per exchange
func (m *WebSocketManager) GetActiveSymbols() map[connector.ExchangeID][]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[connector.ExchangeID][]string)
	for exchID, symbols := range m.activeSymbols {
		symbolList := make([]string, 0, len(symbols))
		for s := range symbols {
			symbolList = append(symbolList, s)
		}
		result[exchID] = symbolList
	}
	return result
}

// GetConnectedExchanges returns list of connected exchange IDs
func (m *WebSocketManager) GetConnectedExchanges() []connector.ExchangeID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []connector.ExchangeID
	for exchID, conn := range m.connectors {
		if conn.IsConnected() {
			result = append(result, exchID)
		}
	}
	return result
}

// GetTotalSymbolCount returns total number of subscribed symbols across all exchanges
func (m *WebSocketManager) GetTotalSymbolCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := 0
	for _, symbols := range m.activeSymbols {
		total += len(symbols)
	}
	return total
}

// DisconnectAll disconnects all WebSocket connections
func (m *WebSocketManager) DisconnectAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	close(m.done)

	for exchID, conn := range m.connectors {
		if conn.IsConnected() {
			if err := conn.Disconnect(); err != nil {
				log.Error().
					Err(err).
					Str("exchange", string(exchID)).
					Msg("Error disconnecting from exchange")
			}
		}
	}

	m.activeSymbols = make(map[connector.ExchangeID]map[string]bool)

	log.Info().Msg("All WebSocket connections disconnected")
	return nil
}

// MonitorConnections monitors WebSocket connections and reconnects if needed
func (m *WebSocketManager) MonitorConnections(ctx context.Context, checkInterval time.Duration) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.done:
			return
		case <-ticker.C:
			m.checkAndReconnect(ctx)
		}
	}
}

// checkAndReconnect checks for disconnected exchanges and attempts to reconnect
func (m *WebSocketManager) checkAndReconnect(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for exchID, conn := range m.connectors {
		symbols := m.activeSymbols[exchID]
		if len(symbols) == 0 {
			continue // No active subscriptions
		}

		if !conn.IsConnected() {
			log.Warn().
				Str("exchange", string(exchID)).
				Msg("WebSocket disconnected, attempting reconnect")

			// Convert map to slice
			symbolList := make([]string, 0, len(symbols))
			for s := range symbols {
				symbolList = append(symbolList, s)
			}

			if err := conn.ConnectForSymbols(ctx, symbolList); err != nil {
				log.Error().
					Err(err).
					Str("exchange", string(exchID)).
					Msg("Failed to reconnect to exchange")
			} else {
				log.Info().
					Str("exchange", string(exchID)).
					Int("symbols", len(symbolList)).
					Msg("Reconnected to exchange")
			}
		}

		// Check for stale data
		lastMsg := conn.LastMessageTime()
		if !lastMsg.IsZero() && time.Since(lastMsg) > 30*time.Second {
			log.Warn().
				Str("exchange", string(exchID)).
				Dur("stale", time.Since(lastMsg)).
				Msg("WebSocket may be stale, no messages received")
		}
	}
}
