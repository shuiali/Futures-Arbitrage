package bybit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Client is the unified Bybit client that integrates REST and WebSocket APIs
type Client struct {
	// REST client for API calls
	REST *RESTClient

	// Market data WebSocket for orderbooks, tickers, trades
	MarketData *MarketDataWS

	// Trading WebSocket for low-latency order operations
	Trading *TradingWS

	// User data WebSocket for orders, positions, executions, wallet
	UserData *UserDataWS

	// Configuration
	config ClientConfig

	// State
	mu        sync.RWMutex
	connected bool
}

// ClientConfig holds all configuration for the Bybit client
type ClientConfig struct {
	// API credentials
	APIKey    string
	APISecret string

	// Environment
	UseTestnet bool

	// WebSocket settings
	EnableMarketData bool
	EnableTrading    bool
	EnableUserData   bool

	// Market data settings
	Category       string // linear, inverse, spot
	OrderbookDepth OrderbookDepth

	// Timeouts
	HTTPTimeout time.Duration
	RecvWindow  int64
}

// NewClient creates a new unified Bybit client
func NewClient(config ClientConfig) *Client {
	// Set defaults
	if config.HTTPTimeout == 0 {
		config.HTTPTimeout = 10 * time.Second
	}
	if config.RecvWindow == 0 {
		config.RecvWindow = 5000
	}
	if config.OrderbookDepth == 0 {
		config.OrderbookDepth = Depth50
	}
	if config.Category == "" {
		config.Category = "linear"
	}

	// Initialize REST client
	baseURL := BaseURLMainnet
	if config.UseTestnet {
		baseURL = BaseURLTestnet
	}

	rest := NewRESTClient(RESTClientConfig{
		BaseURL:    baseURL,
		APIKey:     config.APIKey,
		APISecret:  config.APISecret,
		Timeout:    config.HTTPTimeout,
		RecvWindow: config.RecvWindow,
	})

	client := &Client{
		REST:   rest,
		config: config,
	}

	// Initialize market data WebSocket if enabled
	if config.EnableMarketData {
		client.MarketData = NewMarketDataWS(MarketDataWSConfig{
			Category:   config.Category,
			UseTestnet: config.UseTestnet,
		})
	}

	// Initialize trading WebSocket if enabled
	if config.EnableTrading && config.APIKey != "" {
		client.Trading = NewTradingWS(TradingWSConfig{
			APIKey:     config.APIKey,
			APISecret:  config.APISecret,
			UseTestnet: config.UseTestnet,
			RecvWindow: config.RecvWindow,
		})
	}

	// Initialize user data WebSocket if enabled
	if config.EnableUserData && config.APIKey != "" {
		client.UserData = NewUserDataWS(UserDataWSConfig{
			APIKey:     config.APIKey,
			APISecret:  config.APISecret,
			UseTestnet: config.UseTestnet,
		})
	}

	return client
}

// Connect establishes all configured WebSocket connections
func (c *Client) Connect(ctx context.Context) error {
	var errs []error

	// Connect market data WebSocket
	if c.MarketData != nil {
		if err := c.MarketData.Connect(ctx); err != nil {
			errs = append(errs, fmt.Errorf("market data WS: %w", err))
		} else {
			log.Info().Msg("Connected to Bybit market data WebSocket")
		}
	}

	// Connect trading WebSocket
	if c.Trading != nil {
		if err := c.Trading.Connect(ctx); err != nil {
			errs = append(errs, fmt.Errorf("trading WS: %w", err))
		} else {
			log.Info().Msg("Connected to Bybit trading WebSocket")
		}
	}

	// Connect user data WebSocket
	if c.UserData != nil {
		if err := c.UserData.Connect(ctx); err != nil {
			errs = append(errs, fmt.Errorf("user data WS: %w", err))
		} else {
			log.Info().Msg("Connected to Bybit user data WebSocket")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("connection errors: %v", errs)
	}

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	return nil
}

// Disconnect closes all WebSocket connections
func (c *Client) Disconnect() error {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()

	var errs []error

	if c.MarketData != nil {
		if err := c.MarketData.Disconnect(); err != nil {
			errs = append(errs, err)
		}
	}

	if c.Trading != nil {
		if err := c.Trading.Disconnect(); err != nil {
			errs = append(errs, err)
		}
	}

	if c.UserData != nil {
		if err := c.UserData.Disconnect(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("disconnect errors: %v", errs)
	}

	return nil
}

// IsConnected returns the overall connection status
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// =============================================================================
// Market Data Methods
// =============================================================================

// SubscribeOrderbook subscribes to orderbook updates
func (c *Client) SubscribeOrderbook(symbols []string, callbacks ...OrderbookCallback) error {
	if c.MarketData == nil {
		return fmt.Errorf("market data WebSocket not enabled")
	}

	if len(callbacks) > 0 {
		c.MarketData.SetOrderbookCallback(callbacks[0])
	}

	return c.MarketData.SubscribeOrderbook(symbols, c.config.OrderbookDepth)
}

// SubscribeTickers subscribes to ticker updates
func (c *Client) SubscribeTickers(symbols []string, callbacks ...TickerCallback) error {
	if c.MarketData == nil {
		return fmt.Errorf("market data WebSocket not enabled")
	}

	if len(callbacks) > 0 {
		c.MarketData.SetTickerCallback(callbacks[0])
	}

	return c.MarketData.SubscribeTicker(symbols)
}

// SubscribeTrades subscribes to public trade updates
func (c *Client) SubscribeTrades(symbols []string, callbacks ...TradeCallback) error {
	if c.MarketData == nil {
		return fmt.Errorf("market data WebSocket not enabled")
	}

	if len(callbacks) > 0 {
		c.MarketData.SetTradeCallback(callbacks[0])
	}

	return c.MarketData.SubscribeTrades(symbols)
}

// =============================================================================
// Trading Methods (via WebSocket for low latency)
// =============================================================================

// PlaceLimitOrder places a limit order via WebSocket
func (c *Client) PlaceLimitOrder(ctx context.Context, symbol string, side OrderSide, qty, price string) (*TradingWSOrderResponse, error) {
	if c.Trading == nil {
		return nil, fmt.Errorf("trading WebSocket not enabled")
	}
	return c.Trading.PlaceLinearLimitOrder(ctx, symbol, side, qty, price)
}

// PlaceMarketOrder places a market order via WebSocket
func (c *Client) PlaceMarketOrder(ctx context.Context, symbol string, side OrderSide, qty string) (*TradingWSOrderResponse, error) {
	if c.Trading == nil {
		return nil, fmt.Errorf("trading WebSocket not enabled")
	}
	return c.Trading.PlaceLinearMarketOrder(ctx, symbol, side, qty)
}

// CancelOrderWS cancels an order via WebSocket
func (c *Client) CancelOrderWS(ctx context.Context, symbol, orderId string) (*TradingWSOrderResponse, error) {
	if c.Trading == nil {
		return nil, fmt.Errorf("trading WebSocket not enabled")
	}
	return c.Trading.CancelLinearOrder(ctx, symbol, orderId)
}

// AmendOrderWS amends an order via WebSocket
func (c *Client) AmendOrderWS(ctx context.Context, symbol, orderId, newQty, newPrice string) (*TradingWSOrderResponse, error) {
	if c.Trading == nil {
		return nil, fmt.Errorf("trading WebSocket not enabled")
	}
	return c.Trading.AmendLinearOrder(ctx, symbol, orderId, newQty, newPrice)
}

// =============================================================================
// User Data Methods
// =============================================================================

// SubscribeOrderUpdates subscribes to order update stream
func (c *Client) SubscribeOrderUpdates(category string, callback OrderUpdateCallback) error {
	if c.UserData == nil {
		return fmt.Errorf("user data WebSocket not enabled")
	}

	c.UserData.SetOrderUpdateCallback(callback)
	return c.UserData.SubscribeOrders(category)
}

// SubscribePositionUpdates subscribes to position update stream
func (c *Client) SubscribePositionUpdates(category string, callback PositionUpdateCallback) error {
	if c.UserData == nil {
		return fmt.Errorf("user data WebSocket not enabled")
	}

	c.UserData.SetPositionUpdateCallback(callback)
	return c.UserData.SubscribePositions(category)
}

// SubscribeExecutionUpdates subscribes to execution/fill update stream
func (c *Client) SubscribeExecutionUpdates(category string, callback ExecutionUpdateCallback) error {
	if c.UserData == nil {
		return fmt.Errorf("user data WebSocket not enabled")
	}

	c.UserData.SetExecutionUpdateCallback(callback)
	return c.UserData.SubscribeExecutions(category)
}

// SubscribeWalletUpdates subscribes to wallet balance update stream
func (c *Client) SubscribeWalletUpdates(callback WalletUpdateCallback) error {
	if c.UserData == nil {
		return fmt.Errorf("user data WebSocket not enabled")
	}

	c.UserData.SetWalletUpdateCallback(callback)
	return c.UserData.SubscribeWallet()
}

// SubscribeAllUserData subscribes to all user data streams for a category
func (c *Client) SubscribeAllUserData(category string) error {
	if c.UserData == nil {
		return fmt.Errorf("user data WebSocket not enabled")
	}
	return c.UserData.SubscribeAll(category)
}

// =============================================================================
// REST API Convenience Methods
// =============================================================================

// GetInstruments fetches all instruments for the configured category
func (c *Client) GetInstruments(ctx context.Context) (*InstrumentsInfoResponse, error) {
	return c.REST.GetInstruments(ctx, c.config.Category)
}

// GetTickers fetches all tickers for the configured category
func (c *Client) GetTickers(ctx context.Context) (*TickersResponse, error) {
	return c.REST.GetTickers(ctx, c.config.Category, "")
}

// GetTicker fetches ticker for a specific symbol
func (c *Client) GetTicker(ctx context.Context, symbol string) (*TickersResponse, error) {
	return c.REST.GetTickers(ctx, c.config.Category, symbol)
}

// GetOrderbook fetches orderbook for a symbol
func (c *Client) GetOrderbook(ctx context.Context, symbol string, limit int) (*OrderbookResponse, error) {
	return c.REST.GetOrderbook(ctx, c.config.Category, symbol, limit)
}

// GetKlines fetches kline/candlestick data
func (c *Client) GetKlines(ctx context.Context, symbol, interval string, startTime, endTime int64, limit int) (*KlineResponse, error) {
	return c.REST.GetKline(ctx, c.config.Category, symbol, interval, startTime, endTime, limit)
}

// GetFundingHistory fetches historical funding rates
func (c *Client) GetFundingHistory(ctx context.Context, symbol string, startTime, endTime int64, limit int) (*FundingHistoryResponse, error) {
	return c.REST.GetFundingHistory(ctx, c.config.Category, symbol, startTime, endTime, limit)
}

// GetPositions fetches current positions
func (c *Client) GetPositions(ctx context.Context, symbol string) (*GetPositionsResponse, error) {
	return c.REST.GetPositions(ctx, c.config.Category, symbol, 200)
}

// GetOpenOrders fetches open orders
func (c *Client) GetOpenOrders(ctx context.Context, symbol string) (*GetOrdersResponse, error) {
	return c.REST.GetOpenOrders(ctx, c.config.Category, symbol, 50)
}

// GetWalletBalance fetches wallet balance
func (c *Client) GetWalletBalance(ctx context.Context, coin string) (*GetWalletBalanceResponse, error) {
	return c.REST.GetWalletBalance(ctx, "UNIFIED", coin)
}

// GetFeeRate fetches fee rates
func (c *Client) GetFeeRate(ctx context.Context, symbol string) (*GetFeeRateResponse, error) {
	return c.REST.GetFeeRate(ctx, c.config.Category, symbol)
}

// GetCoinInfo fetches coin deposit/withdrawal info
func (c *Client) GetCoinInfo(ctx context.Context, coin string) (*GetCoinInfoResponse, error) {
	return c.REST.GetCoinInfo(ctx, coin)
}

// PlaceLimitOrderREST places a limit order via REST API
func (c *Client) PlaceLimitOrderREST(ctx context.Context, symbol string, side OrderSide, qty, price string) (*CreateOrderResponse, error) {
	return c.REST.PlaceLimitOrder(ctx, symbol, side, qty, price)
}

// PlaceMarketOrderREST places a market order via REST API
func (c *Client) PlaceMarketOrderREST(ctx context.Context, symbol string, side OrderSide, qty string) (*CreateOrderResponse, error) {
	return c.REST.PlaceMarketOrder(ctx, symbol, side, qty)
}

// CancelOrderREST cancels an order via REST API
func (c *Client) CancelOrderREST(ctx context.Context, symbol, orderId string) (*CancelOrderResponse, error) {
	req := &CancelOrderRequest{
		Category: c.config.Category,
		Symbol:   symbol,
		OrderID:  orderId,
	}
	return c.REST.CancelOrder(ctx, req)
}

// =============================================================================
// Slippage Calculator
// =============================================================================

// SlippageResult contains the result of a slippage calculation
type SlippageResult struct {
	Symbol          string
	Side            string
	RequestedQty    float64
	AvgPrice        float64
	TotalCost       float64
	SlippageBps     float64
	FilledLevels    int
	RemainingQty    float64
	InsufficientLiq bool
}

// CalculateSlippage calculates expected slippage for a given order size by walking the book
func (c *Client) CalculateSlippage(ctx context.Context, symbol string, side OrderSide, qty float64) (*SlippageResult, error) {
	// Get current orderbook
	obResp, err := c.GetOrderbook(ctx, symbol, 200)
	if err != nil {
		return nil, fmt.Errorf("failed to get orderbook: %w", err)
	}

	result := &SlippageResult{
		Symbol:       symbol,
		Side:         string(side),
		RequestedQty: qty,
	}

	var levels [][]string
	if side == OrderSideBuy {
		levels = obResp.Result.Asks // We buy from asks
	} else {
		levels = obResp.Result.Bids // We sell to bids
	}

	if len(levels) == 0 {
		result.InsufficientLiq = true
		return result, nil
	}

	remainingQty := qty
	totalValue := 0.0
	filledLevels := 0
	bestPrice := 0.0

	for i, level := range levels {
		if len(level) < 2 {
			continue
		}

		price := parseFloat(level[0])
		size := parseFloat(level[1])

		if i == 0 {
			bestPrice = price
		}

		fillQty := min(remainingQty, size)
		totalValue += fillQty * price
		remainingQty -= fillQty
		filledLevels++

		if remainingQty <= 0 {
			break
		}
	}

	result.FilledLevels = filledLevels
	result.RemainingQty = remainingQty
	result.InsufficientLiq = remainingQty > 0

	if qty-remainingQty > 0 {
		result.AvgPrice = totalValue / (qty - remainingQty)
		result.TotalCost = totalValue
		if bestPrice > 0 {
			result.SlippageBps = (result.AvgPrice - bestPrice) / bestPrice * 10000
			if side == OrderSideSell {
				result.SlippageBps = -result.SlippageBps // Selling at lower price is positive slippage
			}
		}
	}

	return result, nil
}

// Helper function to parse float
func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// min returns the minimum of two floats
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
