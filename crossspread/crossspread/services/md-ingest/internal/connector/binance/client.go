package binance

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Client is the main Binance client that integrates all functionality
type Client struct {
	// REST client for one-time requests
	Rest *RestClient

	// WebSocket clients
	MarketData *MarketDataStream
	Trading    *TradingClient
	UserData   *UserDataStream

	// State trackers
	Positions *PositionTracker
	Balances  *BalanceTracker
	Orders    *OrderTracker

	// Orderbook managers
	orderbooks map[string]*OrderbookManager
	obMu       sync.RWMutex

	// Configuration
	apiKey    string
	secretKey string
	isLive    bool

	// Internal state
	connected bool
	mu        sync.RWMutex
}

// ClientConfig holds configuration for the Binance client
type ClientConfig struct {
	APIKey    string
	SecretKey string
	IsLive    bool // If false, runs in paper trading mode
}

// NewClient creates a new unified Binance client
func NewClient(config *ClientConfig) *Client {
	apiKey := ""
	secretKey := ""
	if config != nil {
		apiKey = config.APIKey
		secretKey = config.SecretKey
	}

	return &Client{
		Rest:       NewRestClient(apiKey, secretKey),
		Positions:  NewPositionTracker(),
		Balances:   NewBalanceTracker(),
		Orders:     NewOrderTracker(),
		orderbooks: make(map[string]*OrderbookManager),
		apiKey:     apiKey,
		secretKey:  secretKey,
		isLive:     config != nil && config.IsLive,
	}
}

// =============================================================================
// Connection Management
// =============================================================================

// ConnectMarketData connects to market data streams for specified symbols
func (c *Client) ConnectMarketData(ctx context.Context, symbols []string, streamTypes []string) error {
	handler := &MarketDataHandler{
		OnTrade: func(event *WSTradeEvent) {
			log.Trace().
				Str("symbol", event.Symbol).
				Str("price", event.Price).
				Str("qty", event.Quantity).
				Msg("Trade received")
		},
		OnDepth: func(event *WSDepthEvent) {
			c.handleDepthUpdate(event)
		},
		OnMarkPrice: func(event *WSMarkPriceEvent) {
			log.Trace().
				Str("symbol", event.Symbol).
				Str("mark", event.MarkPrice).
				Str("index", event.IndexPrice).
				Str("funding", event.FundingRate).
				Msg("Mark price update")
		},
		OnKline: func(event *WSKlineEvent) {
			log.Trace().
				Str("symbol", event.Symbol).
				Str("interval", event.Kline.Interval).
				Str("close", event.Kline.Close).
				Msg("Kline update")
		},
		OnMiniTicker: func(event *WSMiniTickerEvent) {
			log.Trace().
				Str("symbol", event.Symbol).
				Str("close", event.Close).
				Msg("Mini ticker update")
		},
		OnError: func(err error) {
			log.Error().Err(err).Msg("Market data stream error")
		},
	}

	c.MarketData = NewMarketDataStream(handler)
	return c.MarketData.ConnectForSymbols(ctx, symbols, streamTypes)
}

// ConnectTrading connects to the WebSocket API for trading
func (c *Client) ConnectTrading(ctx context.Context) error {
	if c.apiKey == "" || c.secretKey == "" {
		return fmt.Errorf("API key and secret key required for trading")
	}

	handler := &TradingHandler{
		OnOrderResult: func(id string, result *OrderResult, err error) {
			if err != nil {
				log.Error().Err(err).Str("id", id).Msg("Order operation failed")
			} else {
				log.Info().
					Str("id", id).
					Int64("orderId", result.OrderId).
					Str("status", result.Status).
					Msg("Order operation completed")
			}
		},
		OnError: func(err error) {
			log.Error().Err(err).Msg("Trading WebSocket error")
		},
	}

	c.Trading = NewTradingClient(c.apiKey, c.secretKey, handler)
	return c.Trading.Connect(ctx)
}

// ConnectUserData connects to the user data stream
func (c *Client) ConnectUserData(ctx context.Context) error {
	if c.apiKey == "" || c.secretKey == "" {
		return fmt.Errorf("API key and secret key required for user data stream")
	}

	handler := &UserDataHandler{
		OnAccountUpdate: func(event *AccountUpdateEvent) {
			c.Positions.UpdateFromEvent(event)
			c.Balances.UpdateFromEvent(event)

			log.Info().
				Str("reason", event.AccountUpdate.Reason).
				Int("positions", len(event.AccountUpdate.Positions)).
				Int("balances", len(event.AccountUpdate.Balances)).
				Msg("Account update received")
		},
		OnOrderUpdate: func(event *OrderUpdateEvent) {
			state := c.Orders.UpdateFromEvent(event)

			log.Info().
				Int64("orderId", state.OrderId).
				Str("symbol", state.Symbol).
				Str("status", state.Status).
				Float64("executed", state.ExecutedQty).
				Msg("Order update received")
		},
		OnMarginCall: func(event *MarginCallEvent) {
			log.Warn().
				Str("crossWalletBal", event.CrossWalletBal).
				Int("positions", len(event.MarginPositions)).
				Msg("⚠️ MARGIN CALL received")
		},
		OnError: func(err error) {
			log.Error().Err(err).Msg("User data stream error")
		},
	}

	c.UserData = NewUserDataStream(c.Rest, handler)
	return c.UserData.Connect(ctx)
}

// ConnectAll connects all WebSocket streams
func (c *Client) ConnectAll(ctx context.Context, symbols []string) error {
	// Connect market data
	streamTypes := []string{StreamTypeDepth100ms, StreamTypeMarkPrice1s}
	if err := c.ConnectMarketData(ctx, symbols, streamTypes); err != nil {
		return fmt.Errorf("connect market data: %w", err)
	}

	// Connect trading WebSocket API (if authenticated)
	if c.apiKey != "" && c.secretKey != "" {
		if err := c.ConnectTrading(ctx); err != nil {
			log.Warn().Err(err).Msg("Failed to connect trading API")
		}

		// Connect user data stream
		if err := c.ConnectUserData(ctx); err != nil {
			log.Warn().Err(err).Msg("Failed to connect user data stream")
		}
	}

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	return nil
}

// DisconnectAll closes all WebSocket connections
func (c *Client) DisconnectAll() {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()

	if c.MarketData != nil {
		c.MarketData.Disconnect()
	}
	if c.Trading != nil {
		c.Trading.Disconnect()
	}
	if c.UserData != nil {
		c.UserData.Disconnect()
	}
}

// IsConnected returns true if all required streams are connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// =============================================================================
// Orderbook Management
// =============================================================================

func (c *Client) handleDepthUpdate(event *WSDepthEvent) {
	c.obMu.Lock()
	ob, exists := c.orderbooks[event.Symbol]
	if !exists {
		ob = NewOrderbookManager(event.Symbol)
		c.orderbooks[event.Symbol] = ob
	}
	c.obMu.Unlock()

	if !ob.initialized {
		// Need to fetch snapshot first
		go c.initializeOrderbook(context.Background(), event.Symbol)
		return
	}

	if !ob.ApplyUpdate(event) {
		// Sequence gap, need to resync
		log.Warn().Str("symbol", event.Symbol).Msg("Orderbook sequence gap, resyncing")
		go c.initializeOrderbook(context.Background(), event.Symbol)
	}
}

func (c *Client) initializeOrderbook(ctx context.Context, symbol string) {
	snapshot, err := c.Rest.FetchDepth(ctx, symbol, 100)
	if err != nil {
		log.Error().Err(err).Str("symbol", symbol).Msg("Failed to fetch orderbook snapshot")
		return
	}

	c.obMu.Lock()
	ob, exists := c.orderbooks[symbol]
	if !exists {
		ob = NewOrderbookManager(symbol)
		c.orderbooks[symbol] = ob
	}
	c.obMu.Unlock()

	ob.InitializeFromSnapshot(snapshot)
	log.Debug().Str("symbol", symbol).Msg("Orderbook initialized from snapshot")
}

// GetOrderbook returns the orderbook for a symbol
func (c *Client) GetOrderbook(symbol string) *OrderbookManager {
	c.obMu.RLock()
	defer c.obMu.RUnlock()
	return c.orderbooks[symbol]
}

// GetBestPrices returns best bid/ask for a symbol
func (c *Client) GetBestPrices(symbol string) (bestBid, bestBidQty, bestAsk, bestAskQty float64) {
	c.obMu.RLock()
	ob := c.orderbooks[symbol]
	c.obMu.RUnlock()

	if ob != nil {
		return ob.GetBestBidAsk()
	}
	return 0, 0, 0, 0
}

// =============================================================================
// Trading Operations (High-Level API)
// =============================================================================

// PlaceLimitOrder places a limit order
func (c *Client) PlaceLimitOrder(ctx context.Context, symbol, side string, price, quantity float64) (*OrderResult, error) {
	if c.Trading == nil || !c.Trading.IsConnected() {
		return nil, fmt.Errorf("trading client not connected")
	}

	return c.Trading.PlaceOrder(ctx, &OrderParams{
		Symbol:      symbol,
		Side:        side,
		Type:        OrderTypeLimit,
		Price:       price,
		Quantity:    quantity,
		TimeInForce: TimeInForceGTC,
	})
}

// PlaceMarketOrder places a market order
func (c *Client) PlaceMarketOrder(ctx context.Context, symbol, side string, quantity float64) (*OrderResult, error) {
	if c.Trading == nil || !c.Trading.IsConnected() {
		return nil, fmt.Errorf("trading client not connected")
	}

	return c.Trading.PlaceOrder(ctx, &OrderParams{
		Symbol:   symbol,
		Side:     side,
		Type:     OrderTypeMarket,
		Quantity: quantity,
	})
}

// PlacePostOnlyOrder places a post-only (maker) order
func (c *Client) PlacePostOnlyOrder(ctx context.Context, symbol, side string, price, quantity float64) (*OrderResult, error) {
	if c.Trading == nil || !c.Trading.IsConnected() {
		return nil, fmt.Errorf("trading client not connected")
	}

	return c.Trading.PlaceOrder(ctx, &OrderParams{
		Symbol:      symbol,
		Side:        side,
		Type:        OrderTypeLimit,
		Price:       price,
		Quantity:    quantity,
		TimeInForce: TimeInForceGTX, // Post-only
	})
}

// CancelOrder cancels an order by ID
func (c *Client) CancelOrder(ctx context.Context, symbol string, orderId int64) (*OrderResult, error) {
	if c.Trading == nil || !c.Trading.IsConnected() {
		return nil, fmt.Errorf("trading client not connected")
	}

	return c.Trading.CancelOrder(ctx, symbol, orderId, "")
}

// CancelAllOrders cancels all open orders for a symbol
func (c *Client) CancelAllOrders(ctx context.Context, symbol string) error {
	if c.Trading == nil || !c.Trading.IsConnected() {
		return fmt.Errorf("trading client not connected")
	}

	return c.Trading.CancelAllOrders(ctx, symbol)
}

// =============================================================================
// Data Fetching (High-Level API)
// =============================================================================

// FetchAllTokenData fetches comprehensive market data for all tokens
func (c *Client) FetchAllTokenData(ctx context.Context) (map[string]*TokenData, error) {
	return c.Rest.FetchAllTokenData(ctx)
}

// FetchHistoricalSpread fetches historical price data for calculating spread
func (c *Client) FetchHistoricalSpread(ctx context.Context, symbol1, symbol2, interval string, lookback time.Duration) ([]SpreadPoint, error) {
	endTime := time.Now()
	startTime := endTime.Add(-lookback)

	// Fetch historical prices for both symbols in parallel
	type result struct {
		symbol string
		prices []HistoricalPrice
		err    error
	}

	results := make(chan result, 2)

	go func() {
		prices, err := c.Rest.FetchHistoricalPrices(ctx, symbol1, interval, startTime, endTime)
		results <- result{symbol1, prices, err}
	}()

	go func() {
		prices, err := c.Rest.FetchHistoricalPrices(ctx, symbol2, interval, startTime, endTime)
		results <- result{symbol2, prices, err}
	}()

	// Collect results
	var prices1, prices2 []HistoricalPrice
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			return nil, fmt.Errorf("fetch %s prices: %w", r.symbol, r.err)
		}
		if r.symbol == symbol1 {
			prices1 = r.prices
		} else {
			prices2 = r.prices
		}
	}

	// Calculate spread for each time point
	spreads := make([]SpreadPoint, 0)

	// Create a map for quick lookup of prices2 by timestamp
	prices2Map := make(map[int64]float64)
	for _, p := range prices2 {
		prices2Map[p.Timestamp.Unix()] = p.Close
	}

	for _, p1 := range prices1 {
		if p2, ok := prices2Map[p1.Timestamp.Unix()]; ok && p2 > 0 {
			spreadPct := ((p1.Close - p2) / p2) * 100
			spreadBps := spreadPct * 100

			spreads = append(spreads, SpreadPoint{
				Timestamp: p1.Timestamp,
				Price1:    p1.Close,
				Price2:    p2,
				SpreadPct: spreadPct,
				SpreadBps: spreadBps,
			})
		}
	}

	return spreads, nil
}

// SpreadPoint represents a single historical spread data point
type SpreadPoint struct {
	Timestamp time.Time
	Price1    float64
	Price2    float64
	SpreadPct float64
	SpreadBps float64
}

// =============================================================================
// Account Information
// =============================================================================

// GetPositions returns all current positions
func (c *Client) GetPositions() []*Position {
	return c.Positions.GetAllPositions()
}

// GetPosition returns a specific position
func (c *Client) GetPosition(symbol, side string) *Position {
	return c.Positions.GetPosition(symbol, side)
}

// GetBalance returns balance for an asset
func (c *Client) GetBalance(asset string) *Balance {
	return c.Balances.GetBalance(asset)
}

// GetOpenOrders returns all open orders
func (c *Client) GetOpenOrders() []*OrderState {
	return c.Orders.GetOpenOrders()
}

// GetTotalUnrealizedPnL returns total unrealized PnL
func (c *Client) GetTotalUnrealizedPnL() float64 {
	return c.Positions.GetTotalUnrealizedPnL()
}

// RefreshAccountData fetches latest account data via REST
func (c *Client) RefreshAccountData(ctx context.Context) error {
	if c.apiKey == "" || c.secretKey == "" {
		return fmt.Errorf("API key required")
	}

	account, err := c.Rest.FetchFuturesAccount(ctx)
	if err != nil {
		return err
	}

	// Update positions
	for _, p := range account.Positions {
		posAmt := parseFloat(p.PositionAmt)
		if posAmt != 0 {
			pos := &Position{
				Symbol:        p.Symbol,
				PositionSide:  p.PositionSide,
				PositionAmt:   posAmt,
				EntryPrice:    parseFloat(p.EntryPrice),
				UnrealizedPnL: parseFloat(p.UnrealizedProfit),
				UpdateTime:    time.UnixMilli(p.UpdateTime),
			}
			c.Positions.mu.Lock()
			c.Positions.positions[fmt.Sprintf("%s:%s", p.Symbol, p.PositionSide)] = pos
			c.Positions.mu.Unlock()
		}
	}

	// Update balances
	for _, a := range account.Assets {
		walletBal := parseFloat(a.WalletBalance)
		if walletBal > 0 {
			bal := &Balance{
				Asset:              a.Asset,
				WalletBalance:      walletBal,
				CrossWalletBalance: parseFloat(a.CrossWalletBalance),
				AvailableBalance:   parseFloat(a.AvailableBalance),
				UpdateTime:         time.UnixMilli(a.UpdateTime),
			}
			c.Balances.mu.Lock()
			c.Balances.balances[a.Asset] = bal
			c.Balances.mu.Unlock()
		}
	}

	return nil
}
