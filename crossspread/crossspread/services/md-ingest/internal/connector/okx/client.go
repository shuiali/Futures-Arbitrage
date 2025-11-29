// Package okx provides a unified client for OKX exchange API.
package okx

import (
	"context"
	"fmt"
	"sync"
)

// Client provides a unified interface to OKX APIs
type Client struct {
	// REST client for API calls
	REST *RESTClient

	// WebSocket clients
	MarketData *MarketDataWSClient
	Trading    *TradingWSClient
	UserData   *UserDataWSClient

	// Configuration
	config ClientConfig

	// State tracking
	orderBook       *OrderBookManager
	positionTracker *PositionTracker
	orderTracker    *OrderTracker

	mu sync.RWMutex
}

// ClientConfig holds configuration for the unified client
type ClientConfig struct {
	// API credentials
	APIKey     string
	SecretKey  string
	Passphrase string

	// Demo mode (paper trading)
	DemoMode bool

	// WebSocket handlers
	MarketDataHandler MarketDataHandler
	TradingHandler    TradingHandler
	UserDataHandler   UserDataHandler

	// Timeouts and retry settings
	HTTPTimeout   int // seconds
	PingInterval  int // seconds
	ReconnectWait int // seconds
	MaxReconnect  int
}

// NewClient creates a new unified OKX client
func NewClient(cfg ClientConfig) *Client {
	client := &Client{
		config:          cfg,
		orderBook:       NewOrderBookManager(),
		positionTracker: NewPositionTracker(),
		orderTracker:    NewOrderTracker(),
	}

	// Create REST client
	client.REST = NewRESTClient(RESTClientConfig{
		APIKey:     cfg.APIKey,
		SecretKey:  cfg.SecretKey,
		Passphrase: cfg.Passphrase,
		DemoMode:   cfg.DemoMode,
	})

	// Create market data WebSocket client if handler provided
	if cfg.MarketDataHandler != nil {
		client.MarketData = NewMarketDataWSClient(MarketDataWSConfig{
			DemoMode: cfg.DemoMode,
			Handler:  cfg.MarketDataHandler,
		})
	}

	// Create trading WebSocket client if credentials provided
	if cfg.APIKey != "" {
		if cfg.TradingHandler != nil {
			client.Trading = NewTradingWSClient(TradingWSConfig{
				APIKey:     cfg.APIKey,
				SecretKey:  cfg.SecretKey,
				Passphrase: cfg.Passphrase,
				DemoMode:   cfg.DemoMode,
				Handler:    cfg.TradingHandler,
			})
		}

		if cfg.UserDataHandler != nil {
			client.UserData = NewUserDataWSClient(UserDataWSConfig{
				APIKey:     cfg.APIKey,
				SecretKey:  cfg.SecretKey,
				Passphrase: cfg.Passphrase,
				DemoMode:   cfg.DemoMode,
				Handler:    cfg.UserDataHandler,
			})
		}
	}

	return client
}

// Connect connects all WebSocket clients
func (c *Client) Connect() error {
	if c.MarketData != nil {
		if err := c.MarketData.Connect(); err != nil {
			return fmt.Errorf("failed to connect market data: %w", err)
		}
	}

	if c.Trading != nil {
		if err := c.Trading.Connect(); err != nil {
			return fmt.Errorf("failed to connect trading: %w", err)
		}
	}

	if c.UserData != nil {
		if err := c.UserData.Connect(); err != nil {
			return fmt.Errorf("failed to connect user data: %w", err)
		}
	}

	return nil
}

// Close closes all connections
func (c *Client) Close() error {
	var errs []error

	if c.MarketData != nil {
		if err := c.MarketData.Close(); err != nil {
			errs = append(errs, fmt.Errorf("market data close: %w", err))
		}
	}

	if c.Trading != nil {
		if err := c.Trading.Close(); err != nil {
			errs = append(errs, fmt.Errorf("trading close: %w", err))
		}
	}

	if c.UserData != nil {
		if err := c.UserData.Close(); err != nil {
			errs = append(errs, fmt.Errorf("user data close: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}

	return nil
}

// =============================================================================
// State Accessors
// =============================================================================

// GetOrderBook returns the order book manager for accessing maintained order books
func (c *Client) GetOrderBook() *OrderBookManager {
	return c.orderBook
}

// GetPositionTracker returns the position tracker
func (c *Client) GetPositionTracker() *PositionTracker {
	return c.positionTracker
}

// GetOrderTracker returns the order tracker
func (c *Client) GetOrderTracker() *OrderTracker {
	return c.orderTracker
}

// =============================================================================
// Market Data Convenience Methods
// =============================================================================

// SubscribeToInstrument subscribes to all market data for an instrument
func (c *Client) SubscribeToInstrument(instID string) error {
	if c.MarketData == nil {
		return fmt.Errorf("market data client not initialized")
	}

	// Subscribe to ticker
	if err := c.MarketData.SubscribeTicker(instID); err != nil {
		return fmt.Errorf("failed to subscribe ticker: %w", err)
	}

	// Subscribe to order book
	if err := c.MarketData.SubscribeOrderBook(instID, ChannelBooks); err != nil {
		return fmt.Errorf("failed to subscribe order book: %w", err)
	}

	// Subscribe to trades
	if err := c.MarketData.SubscribeTrades(instID); err != nil {
		return fmt.Errorf("failed to subscribe trades: %w", err)
	}

	return nil
}

// SubscribeToInstrumentWithDepth subscribes to market data with tick-by-tick depth
func (c *Client) SubscribeToInstrumentWithDepth(instID string, levels int) error {
	if c.MarketData == nil {
		return fmt.Errorf("market data client not initialized")
	}

	// Subscribe to ticker
	if err := c.MarketData.SubscribeTicker(instID); err != nil {
		return fmt.Errorf("failed to subscribe ticker: %w", err)
	}

	// Subscribe to appropriate order book channel
	channel := ChannelBooks
	if levels <= 5 {
		channel = ChannelBooks5
	} else if levels <= 50 {
		channel = ChannelBooks50TBT
	} else {
		channel = ChannelBooks400TBT
	}

	if err := c.MarketData.SubscribeOrderBook(instID, channel); err != nil {
		return fmt.Errorf("failed to subscribe order book: %w", err)
	}

	// Subscribe to trades
	if err := c.MarketData.SubscribeTrades(instID); err != nil {
		return fmt.Errorf("failed to subscribe trades: %w", err)
	}

	return nil
}

// SubscribeToFundingRate subscribes to funding rate updates for a perpetual
func (c *Client) SubscribeToFundingRate(instID string) error {
	if c.MarketData == nil {
		return fmt.Errorf("market data client not initialized")
	}
	return c.MarketData.SubscribeFundingRate(instID)
}

// =============================================================================
// Trading Convenience Methods
// =============================================================================

// PlaceLimitOrder places a limit order using WebSocket for low latency
func (c *Client) PlaceLimitOrder(ctx context.Context, instID, side, sz, px, tdMode, posSide string) (*OrderResult, error) {
	if c.Trading != nil && c.Trading.IsAuthenticated() {
		return c.Trading.PlaceLimitOrderSync(ctx, instID, side, sz, px, tdMode, posSide)
	}

	// Fallback to REST
	return c.REST.PlaceLimitOrder(ctx, instID, side, sz, px, tdMode, posSide, false)
}

// PlaceMarketOrder places a market order using WebSocket for low latency
func (c *Client) PlaceMarketOrder(ctx context.Context, instID, side, sz, tdMode, posSide string) (*OrderResult, error) {
	if c.Trading != nil && c.Trading.IsAuthenticated() {
		return c.Trading.PlaceMarketOrderSync(ctx, instID, side, sz, tdMode, posSide)
	}

	// Fallback to REST
	return c.REST.PlaceMarketOrder(ctx, instID, side, sz, tdMode, posSide, false)
}

// CancelOrder cancels an order using WebSocket for low latency
func (c *Client) CancelOrder(ctx context.Context, instID, orderID string) (*CancelResult, error) {
	if c.Trading != nil && c.Trading.IsAuthenticated() {
		return c.Trading.CancelOrderByIDSync(ctx, instID, orderID)
	}

	// Fallback to REST
	return c.REST.CancelOrder(ctx, &CancelOrderRequest{
		InstID: instID,
		OrdID:  orderID,
	})
}

// EmergencyExit performs emergency exit from a position
func (c *Client) EmergencyExit(ctx context.Context, instID, side, sz, tdMode, posSide string) (*OrderResult, error) {
	if c.Trading != nil && c.Trading.IsAuthenticated() {
		return c.Trading.EmergencyExitPosition(ctx, instID, side, sz, tdMode, posSide)
	}

	// Fallback to REST with market order
	req := &PlaceOrderRequest{
		InstID:     instID,
		TdMode:     tdMode,
		Side:       side,
		OrdType:    OrdTypeMarket,
		Sz:         sz,
		PosSide:    posSide,
		ReduceOnly: true,
	}
	return c.REST.PlaceOrder(ctx, req)
}

// =============================================================================
// Slicing Order Execution
// =============================================================================

// SliceConfig defines parameters for order slicing
type SliceConfig struct {
	InstID      string
	Side        string
	TotalSz     float64 // Total size to execute
	SliceCount  int     // Number of slices
	SliceSize   float64 // Size per slice (alternative to SliceCount)
	PriceOffset float64 // Offset from mid price (in %)
	TdMode      string
	PosSide     string
	IntervalMs  int // Interval between slices in milliseconds
}

// SliceResult contains results of sliced execution
type SliceResult struct {
	Orders      []*OrderResult
	TotalFilled float64
	AvgPrice    float64
	Errors      []error
}

// ExecuteSlicedOrder executes an order in slices
func (c *Client) ExecuteSlicedOrder(ctx context.Context, cfg SliceConfig) (*SliceResult, error) {
	result := &SliceResult{
		Orders: make([]*OrderResult, 0),
		Errors: make([]error, 0),
	}

	// Calculate slice parameters
	var slices int
	var sizePerSlice float64

	if cfg.SliceSize > 0 {
		slices = int(cfg.TotalSz / cfg.SliceSize)
		if float64(slices)*cfg.SliceSize < cfg.TotalSz {
			slices++
		}
		sizePerSlice = cfg.SliceSize
	} else if cfg.SliceCount > 0 {
		slices = cfg.SliceCount
		sizePerSlice = cfg.TotalSz / float64(slices)
	} else {
		return nil, fmt.Errorf("either SliceCount or SliceSize must be specified")
	}

	// Get current price for limit orders
	ticker, err := c.REST.GetTicker(ctx, cfg.InstID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ticker: %w", err)
	}

	// Calculate limit price with offset
	// For buys: price slightly above current ask
	// For sells: price slightly below current bid
	var basePrice string
	if cfg.Side == SideBuy {
		basePrice = ticker.AskPx
	} else {
		basePrice = ticker.BidPx
	}

	remaining := cfg.TotalSz
	for i := 0; i < slices && remaining > 0; i++ {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Calculate this slice's size
		sz := sizePerSlice
		if sz > remaining {
			sz = remaining
		}

		szStr := fmt.Sprintf("%.8f", sz)

		// Place limit order (use post-only to ensure maker)
		orderResult, err := c.PlaceLimitOrder(ctx, cfg.InstID, cfg.Side, szStr, basePrice, cfg.TdMode, cfg.PosSide)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("slice %d: %w", i+1, err))
			continue
		}

		result.Orders = append(result.Orders, orderResult)
		remaining -= sz

		// Wait between slices if configured
		if cfg.IntervalMs > 0 && i < slices-1 {
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-ctx.Done():
			}
		}
	}

	return result, nil
}

// =============================================================================
// Default Handlers
// =============================================================================

// DefaultMarketDataHandler provides a default implementation of MarketDataHandler
type DefaultMarketDataHandler struct {
	OnTickerFunc       func(ticker *WSTickerData)
	OnOrderBookFunc    func(instID string, action string, book *WSOrderBookData)
	OnTradeFunc        func(trade *WSTradeData)
	OnCandleFunc       func(instID string, channel string, candle []string)
	OnFundingRateFunc  func(data *WSFundingRateData)
	OnIndexTickerFunc  func(ticker *IndexTicker)
	OnErrorFunc        func(err error)
	OnConnectedFunc    func()
	OnDisconnectedFunc func()
}

func (h *DefaultMarketDataHandler) OnTicker(ticker *WSTickerData) {
	if h.OnTickerFunc != nil {
		h.OnTickerFunc(ticker)
	}
}

func (h *DefaultMarketDataHandler) OnOrderBook(instID string, action string, book *WSOrderBookData) {
	if h.OnOrderBookFunc != nil {
		h.OnOrderBookFunc(instID, action, book)
	}
}

func (h *DefaultMarketDataHandler) OnTrade(trade *WSTradeData) {
	if h.OnTradeFunc != nil {
		h.OnTradeFunc(trade)
	}
}

func (h *DefaultMarketDataHandler) OnCandle(instID string, channel string, candle []string) {
	if h.OnCandleFunc != nil {
		h.OnCandleFunc(instID, channel, candle)
	}
}

func (h *DefaultMarketDataHandler) OnFundingRate(data *WSFundingRateData) {
	if h.OnFundingRateFunc != nil {
		h.OnFundingRateFunc(data)
	}
}

func (h *DefaultMarketDataHandler) OnIndexTicker(ticker *IndexTicker) {
	if h.OnIndexTickerFunc != nil {
		h.OnIndexTickerFunc(ticker)
	}
}

func (h *DefaultMarketDataHandler) OnError(err error) {
	if h.OnErrorFunc != nil {
		h.OnErrorFunc(err)
	}
}

func (h *DefaultMarketDataHandler) OnConnected() {
	if h.OnConnectedFunc != nil {
		h.OnConnectedFunc()
	}
}

func (h *DefaultMarketDataHandler) OnDisconnected() {
	if h.OnDisconnectedFunc != nil {
		h.OnDisconnectedFunc()
	}
}

// DefaultTradingHandler provides a default implementation of TradingHandler
type DefaultTradingHandler struct {
	OnOrderResultFunc       func(id string, result *OrderResult)
	OnBatchOrderResultFunc  func(id string, results []OrderResult)
	OnCancelResultFunc      func(id string, result *CancelResult)
	OnBatchCancelResultFunc func(id string, results []CancelResult)
	OnAmendResultFunc       func(id string, result *AmendResult)
	OnErrorFunc             func(id string, err error)
	OnConnectedFunc         func()
	OnDisconnectedFunc      func()
	OnAuthenticatedFunc     func()
}

func (h *DefaultTradingHandler) OnOrderResult(id string, result *OrderResult) {
	if h.OnOrderResultFunc != nil {
		h.OnOrderResultFunc(id, result)
	}
}

func (h *DefaultTradingHandler) OnBatchOrderResult(id string, results []OrderResult) {
	if h.OnBatchOrderResultFunc != nil {
		h.OnBatchOrderResultFunc(id, results)
	}
}

func (h *DefaultTradingHandler) OnCancelResult(id string, result *CancelResult) {
	if h.OnCancelResultFunc != nil {
		h.OnCancelResultFunc(id, result)
	}
}

func (h *DefaultTradingHandler) OnBatchCancelResult(id string, results []CancelResult) {
	if h.OnBatchCancelResultFunc != nil {
		h.OnBatchCancelResultFunc(id, results)
	}
}

func (h *DefaultTradingHandler) OnAmendResult(id string, result *AmendResult) {
	if h.OnAmendResultFunc != nil {
		h.OnAmendResultFunc(id, result)
	}
}

func (h *DefaultTradingHandler) OnError(id string, err error) {
	if h.OnErrorFunc != nil {
		h.OnErrorFunc(id, err)
	}
}

func (h *DefaultTradingHandler) OnConnected() {
	if h.OnConnectedFunc != nil {
		h.OnConnectedFunc()
	}
}

func (h *DefaultTradingHandler) OnDisconnected() {
	if h.OnDisconnectedFunc != nil {
		h.OnDisconnectedFunc()
	}
}

func (h *DefaultTradingHandler) OnAuthenticated() {
	if h.OnAuthenticatedFunc != nil {
		h.OnAuthenticatedFunc()
	}
}

// DefaultUserDataHandler provides a default implementation of UserDataHandler
type DefaultUserDataHandler struct {
	OnAccountFunc            func(data *WSAccountData)
	OnPositionFunc           func(data *WSPositionData)
	OnBalanceAndPositionFunc func(data *WSBalanceAndPositionData)
	OnOrderFunc              func(data *WSOrderData)
	OnErrorFunc              func(err error)
	OnConnectedFunc          func()
	OnDisconnectedFunc       func()
	OnAuthenticatedFunc      func()
}

func (h *DefaultUserDataHandler) OnAccount(data *WSAccountData) {
	if h.OnAccountFunc != nil {
		h.OnAccountFunc(data)
	}
}

func (h *DefaultUserDataHandler) OnPosition(data *WSPositionData) {
	if h.OnPositionFunc != nil {
		h.OnPositionFunc(data)
	}
}

func (h *DefaultUserDataHandler) OnBalanceAndPosition(data *WSBalanceAndPositionData) {
	if h.OnBalanceAndPositionFunc != nil {
		h.OnBalanceAndPositionFunc(data)
	}
}

func (h *DefaultUserDataHandler) OnOrder(data *WSOrderData) {
	if h.OnOrderFunc != nil {
		h.OnOrderFunc(data)
	}
}

func (h *DefaultUserDataHandler) OnError(err error) {
	if h.OnErrorFunc != nil {
		h.OnErrorFunc(err)
	}
}

func (h *DefaultUserDataHandler) OnConnected() {
	if h.OnConnectedFunc != nil {
		h.OnConnectedFunc()
	}
}

func (h *DefaultUserDataHandler) OnDisconnected() {
	if h.OnDisconnectedFunc != nil {
		h.OnDisconnectedFunc()
	}
}

func (h *DefaultUserDataHandler) OnAuthenticated() {
	if h.OnAuthenticatedFunc != nil {
		h.OnAuthenticatedFunc()
	}
}
