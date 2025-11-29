package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	wsUserDataBaseURL = "wss://fstream.binance.com/ws"
)

// UserDataHandler handles user data stream events
type UserDataHandler struct {
	OnAccountUpdate func(event *AccountUpdateEvent)
	OnOrderUpdate   func(event *OrderUpdateEvent)
	OnMarginCall    func(event *MarginCallEvent)
	OnError         func(err error)
}

// UserDataStream manages the user data WebSocket connection
type UserDataStream struct {
	restClient      *RestClient
	conn            *websocket.Conn
	handler         *UserDataHandler
	listenKey       string
	mu              sync.RWMutex
	done            chan struct{}
	connected       bool
	keepAliveTicker *time.Ticker
}

// NewUserDataStream creates a new user data stream
func NewUserDataStream(restClient *RestClient, handler *UserDataHandler) *UserDataStream {
	return &UserDataStream{
		restClient: restClient,
		handler:    handler,
		done:       make(chan struct{}),
	}
}

// Connect creates a listen key and connects to the user data stream
func (s *UserDataStream) Connect(ctx context.Context) error {
	// Create listen key via REST API
	listenKey, err := s.restClient.CreateListenKey(ctx)
	if err != nil {
		return fmt.Errorf("create listen key: %w", err)
	}
	s.listenKey = listenKey

	// Connect to user data stream
	url := fmt.Sprintf("%s/%s", wsUserDataBaseURL, listenKey)
	log.Info().Str("url", url).Msg("Connecting to Binance user data stream")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	s.conn = conn
	s.connected = true
	log.Info().Msg("Connected to Binance user data stream")

	// Start read loop
	go s.readLoop(ctx)

	// Start keepalive loop (must send keepalive every 30 minutes)
	s.keepAliveTicker = time.NewTicker(25 * time.Minute)
	go s.keepAliveLoop(ctx)

	return nil
}

// Disconnect closes the connection
func (s *UserDataStream) Disconnect() error {
	close(s.done)
	s.connected = false

	if s.keepAliveTicker != nil {
		s.keepAliveTicker.Stop()
	}

	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// IsConnected returns connection status
func (s *UserDataStream) IsConnected() bool {
	return s.connected
}

// GetListenKey returns the current listen key
func (s *UserDataStream) GetListenKey() string {
	return s.listenKey
}

func (s *UserDataStream) readLoop(ctx context.Context) {
	defer func() {
		s.connected = false
	}()

	for {
		select {
		case <-s.done:
			return
		case <-ctx.Done():
			return
		default:
			_, message, err := s.conn.ReadMessage()
			if err != nil {
				if s.handler != nil && s.handler.OnError != nil {
					s.handler.OnError(fmt.Errorf("read error: %w", err))
				}
				return
			}
			s.handleMessage(message)
		}
	}
}

func (s *UserDataStream) keepAliveLoop(ctx context.Context) {
	for {
		select {
		case <-s.done:
			return
		case <-ctx.Done():
			return
		case <-s.keepAliveTicker.C:
			if err := s.restClient.KeepAliveListenKey(ctx); err != nil {
				log.Warn().Err(err).Msg("Failed to keepalive listen key")
				if s.handler != nil && s.handler.OnError != nil {
					s.handler.OnError(fmt.Errorf("keepalive failed: %w", err))
				}
			} else {
				log.Debug().Msg("User data stream listen key kept alive")
			}
		}
	}
}

func (s *UserDataStream) handleMessage(message []byte) {
	// First, determine the event type
	var baseEvent UserDataEvent
	if err := json.Unmarshal(message, &baseEvent); err != nil {
		log.Warn().Err(err).Msg("Failed to parse user data event type")
		return
	}

	switch baseEvent.EventType {
	case "ACCOUNT_UPDATE":
		var event AccountUpdateEvent
		if err := json.Unmarshal(message, &event); err == nil && s.handler != nil && s.handler.OnAccountUpdate != nil {
			s.handler.OnAccountUpdate(&event)
		}

	case "ORDER_TRADE_UPDATE":
		var event OrderUpdateEvent
		if err := json.Unmarshal(message, &event); err == nil && s.handler != nil && s.handler.OnOrderUpdate != nil {
			s.handler.OnOrderUpdate(&event)
		}

	case "MARGIN_CALL":
		var event MarginCallEvent
		if err := json.Unmarshal(message, &event); err == nil && s.handler != nil && s.handler.OnMarginCall != nil {
			s.handler.OnMarginCall(&event)
		}

	case "listenKeyExpired":
		log.Warn().Msg("Listen key expired, need to reconnect")
		if s.handler != nil && s.handler.OnError != nil {
			s.handler.OnError(fmt.Errorf("listen key expired"))
		}

	case "ACCOUNT_CONFIG_UPDATE":
		// Account configuration update (leverage, margin type, etc.)
		log.Debug().Str("event", baseEvent.EventType).Msg("Account config update received")

	default:
		log.Debug().Str("event", baseEvent.EventType).Msg("Unknown user data event type")
	}
}

// =============================================================================
// Helper Types for Position and Balance Tracking
// =============================================================================

// PositionTracker tracks all positions for an account
type PositionTracker struct {
	positions map[string]*Position
	mu        sync.RWMutex
}

// Position represents a single position
type Position struct {
	Symbol         string
	PositionSide   string
	PositionAmt    float64
	EntryPrice     float64
	BreakEvenPrice float64
	UnrealizedPnL  float64
	RealizedPnL    float64
	MarginType     string
	IsolatedWallet float64
	UpdateTime     time.Time
}

// NewPositionTracker creates a new position tracker
func NewPositionTracker() *PositionTracker {
	return &PositionTracker{
		positions: make(map[string]*Position),
	}
}

// UpdateFromEvent updates positions from an account update event
func (t *PositionTracker) UpdateFromEvent(event *AccountUpdateEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, p := range event.AccountUpdate.Positions {
		key := fmt.Sprintf("%s:%s", p.Symbol, p.PositionSide)

		posAmt := parseFloat(p.PositionAmt)

		if posAmt == 0 {
			// Position closed
			delete(t.positions, key)
		} else {
			t.positions[key] = &Position{
				Symbol:         p.Symbol,
				PositionSide:   p.PositionSide,
				PositionAmt:    posAmt,
				EntryPrice:     parseFloat(p.EntryPrice),
				BreakEvenPrice: parseFloat(p.BreakEvenPrice),
				UnrealizedPnL:  parseFloat(p.UnrealizedPnL),
				RealizedPnL:    parseFloat(p.AccumulatedRealized),
				MarginType:     p.MarginType,
				IsolatedWallet: parseFloat(p.IsolatedWallet),
				UpdateTime:     time.UnixMilli(event.TransactTime),
			}
		}
	}
}

// GetPosition returns a position by symbol and side
func (t *PositionTracker) GetPosition(symbol, positionSide string) *Position {
	t.mu.RLock()
	defer t.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", symbol, positionSide)
	if pos, ok := t.positions[key]; ok {
		// Return a copy
		copy := *pos
		return &copy
	}
	return nil
}

// GetAllPositions returns all open positions
func (t *PositionTracker) GetAllPositions() []*Position {
	t.mu.RLock()
	defer t.mu.RUnlock()

	positions := make([]*Position, 0, len(t.positions))
	for _, p := range t.positions {
		copy := *p
		positions = append(positions, &copy)
	}
	return positions
}

// GetTotalUnrealizedPnL returns the total unrealized PnL across all positions
func (t *PositionTracker) GetTotalUnrealizedPnL() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var total float64
	for _, p := range t.positions {
		total += p.UnrealizedPnL
	}
	return total
}

// BalanceTracker tracks account balances
type BalanceTracker struct {
	balances map[string]*Balance
	mu       sync.RWMutex
}

// Balance represents a single asset balance
type Balance struct {
	Asset              string
	WalletBalance      float64
	CrossWalletBalance float64
	AvailableBalance   float64
	UpdateTime         time.Time
}

// NewBalanceTracker creates a new balance tracker
func NewBalanceTracker() *BalanceTracker {
	return &BalanceTracker{
		balances: make(map[string]*Balance),
	}
}

// UpdateFromEvent updates balances from an account update event
func (t *BalanceTracker) UpdateFromEvent(event *AccountUpdateEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, b := range event.AccountUpdate.Balances {
		t.balances[b.Asset] = &Balance{
			Asset:              b.Asset,
			WalletBalance:      parseFloat(b.WalletBalance),
			CrossWalletBalance: parseFloat(b.CrossWalletBalance),
			UpdateTime:         time.UnixMilli(event.TransactTime),
		}
	}
}

// GetBalance returns balance for an asset
func (t *BalanceTracker) GetBalance(asset string) *Balance {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if bal, ok := t.balances[asset]; ok {
		copy := *bal
		return &copy
	}
	return nil
}

// OrderTracker tracks order updates
type OrderTracker struct {
	orders map[int64]*OrderState
	mu     sync.RWMutex
}

// OrderState represents the current state of an order
type OrderState struct {
	OrderId         int64
	Symbol          string
	ClientOrderId   string
	Side            string
	OrderType       string
	Status          string
	OriginalQty     float64
	ExecutedQty     float64
	AveragePrice    float64
	LastFilledPrice float64
	LastFilledQty   float64
	Commission      float64
	CommissionAsset string
	RealizedProfit  float64
	IsMaker         bool
	UpdateTime      time.Time
}

// NewOrderTracker creates a new order tracker
func NewOrderTracker() *OrderTracker {
	return &OrderTracker{
		orders: make(map[int64]*OrderState),
	}
}

// UpdateFromEvent updates order state from an order update event
func (t *OrderTracker) UpdateFromEvent(event *OrderUpdateEvent) *OrderState {
	t.mu.Lock()
	defer t.mu.Unlock()

	o := event.Order

	state := &OrderState{
		OrderId:         o.OrderId,
		Symbol:          o.Symbol,
		ClientOrderId:   o.ClientOrderId,
		Side:            o.Side,
		OrderType:       o.OrderType,
		Status:          o.OrderStatus,
		OriginalQty:     parseFloat(o.OriginalQty),
		ExecutedQty:     parseFloat(o.CumulativeFilledQty),
		AveragePrice:    parseFloat(o.AveragePrice),
		LastFilledPrice: parseFloat(o.LastFilledPrice),
		LastFilledQty:   parseFloat(o.LastFilledQty),
		Commission:      parseFloat(o.Commission),
		CommissionAsset: o.CommissionAsset,
		RealizedProfit:  parseFloat(o.RealizedProfit),
		IsMaker:         o.IsMaker,
		UpdateTime:      time.UnixMilli(o.TradeTime),
	}

	// Remove completed orders after a delay, or keep in map
	if o.OrderStatus == "FILLED" || o.OrderStatus == "CANCELED" || o.OrderStatus == "EXPIRED" {
		// Could optionally move to a "completed" map
		t.orders[o.OrderId] = state
	} else {
		t.orders[o.OrderId] = state
	}

	return state
}

// GetOrder returns an order by ID
func (t *OrderTracker) GetOrder(orderId int64) *OrderState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if order, ok := t.orders[orderId]; ok {
		copy := *order
		return &copy
	}
	return nil
}

// GetOpenOrders returns all open orders
func (t *OrderTracker) GetOpenOrders() []*OrderState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	orders := make([]*OrderState, 0)
	for _, o := range t.orders {
		if o.Status == "NEW" || o.Status == "PARTIALLY_FILLED" {
			copy := *o
			orders = append(orders, &copy)
		}
	}
	return orders
}

// Helper function to parse floats
func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
