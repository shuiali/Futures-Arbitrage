// Package coinex provides a unified client for CoinEx Futures exchange.
package coinex

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Client provides a unified interface to CoinEx Futures exchange
type Client struct {
	// REST client
	REST *RESTClient

	// WebSocket clients
	WSMarketData *WSMarketDataClient
	WSUserData   *WSUserDataClient

	// Configuration
	cfg ClientConfig

	// State
	mu        sync.RWMutex
	connected bool
	ctx       context.Context
	cancel    context.CancelFunc
}

// ClientConfig holds configuration for the unified client
type ClientConfig struct {
	// API credentials
	APIKey    string
	APISecret string

	// REST configuration
	RESTURL      string
	RateLimitRPS float64

	// WebSocket configuration
	WSURL          string
	ReconnectDelay time.Duration
	PingInterval   time.Duration

	// Feature flags
	EnableMarketData bool
	EnableUserData   bool
}

// DefaultClientConfig returns a default client configuration
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		RESTURL:          RESTBaseURL,
		WSURL:            WSFuturesURL,
		RateLimitRPS:     50,
		ReconnectDelay:   5 * time.Second,
		PingInterval:     20 * time.Second,
		EnableMarketData: true,
		EnableUserData:   true,
	}
}

// NewClient creates a new unified CoinEx client
func NewClient(cfg ClientConfig) *Client {
	// Create REST client
	restClient := NewRESTClient(RESTClientConfig{
		BaseURL:   cfg.RESTURL,
		APIKey:    cfg.APIKey,
		SecretKey: cfg.APISecret,
		Timeout:   10 * time.Second,
	})

	// Create WebSocket clients
	var wsMarketData *WSMarketDataClient
	var wsUserData *WSUserDataClient

	if cfg.EnableMarketData {
		wsMarketData = NewWSMarketDataClient(WSMarketDataConfig{
			URL:            cfg.WSURL,
			ReconnectDelay: cfg.ReconnectDelay,
			PingInterval:   cfg.PingInterval,
		})
	}

	if cfg.EnableUserData && cfg.APIKey != "" {
		wsUserData = NewWSUserDataClient(WSUserDataConfig{
			URL:       cfg.WSURL,
			APIKey:    cfg.APIKey,
			APISecret: cfg.APISecret,
		})
	}

	return &Client{
		REST:         restClient,
		WSMarketData: wsMarketData,
		WSUserData:   wsUserData,
		cfg:          cfg,
	}
}

// Connect establishes all connections
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ctx, c.cancel = context.WithCancel(ctx)

	// Connect market data WebSocket
	if c.WSMarketData != nil {
		if err := c.WSMarketData.Connect(c.ctx); err != nil {
			log.Error().Err(err).Msg("Failed to connect market data WebSocket")
			return fmt.Errorf("market data websocket connect failed: %w", err)
		}
	}

	// Connect user data WebSocket
	if c.WSUserData != nil {
		if err := c.WSUserData.Connect(c.ctx); err != nil {
			log.Error().Err(err).Msg("Failed to connect user data WebSocket")
			// User data is optional, don't fail completely
		}
	}

	c.connected = true
	log.Info().Msg("CoinEx client connected")
	return nil
}

// Disconnect closes all connections
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
	}

	var lastErr error

	if c.WSMarketData != nil {
		if err := c.WSMarketData.Disconnect(); err != nil {
			lastErr = err
		}
	}

	if c.WSUserData != nil {
		if err := c.WSUserData.Disconnect(); err != nil {
			lastErr = err
		}
	}

	c.connected = false
	log.Info().Msg("CoinEx client disconnected")
	return lastErr
}

// IsConnected returns whether all clients are connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// =============================================================================
// Convenience Methods - Market Data
// =============================================================================

// GetAllMarkets retrieves all available futures markets
func (c *Client) GetAllMarkets(ctx context.Context) ([]Market, error) {
	return c.REST.GetMarkets(ctx)
}

// GetTickers retrieves ticker data for all markets
func (c *Client) GetTickers(ctx context.Context, markets []string) ([]Ticker, error) {
	return c.REST.GetTickers(ctx, markets...)
}

// GetOrderbook retrieves orderbook for a market
func (c *Client) GetOrderbook(ctx context.Context, market string, limit int) (*Depth, error) {
	return c.REST.GetDepth(ctx, market, limit, "0")
}

// GetFundingRates retrieves funding rates for markets
func (c *Client) GetFundingRates(ctx context.Context, markets []string) ([]FundingRate, error) {
	return c.REST.GetFundingRates(ctx, markets...)
}

// GetKlines retrieves kline/candlestick data
func (c *Client) GetKlines(ctx context.Context, market, period string, limit int) ([]Kline, error) {
	return c.REST.GetKlines(ctx, market, period, limit)
}

// GetDeals retrieves recent trades
func (c *Client) GetDeals(ctx context.Context, market string, limit int) ([]Deal, error) {
	return c.REST.GetDeals(ctx, market, limit, 0)
}

// =============================================================================
// Convenience Methods - Account & Trading
// =============================================================================

// GetBalance retrieves futures account balance
func (c *Client) GetBalance(ctx context.Context) ([]FuturesBalance, error) {
	return c.REST.GetFuturesBalance(ctx)
}

// GetPositions retrieves all open positions
func (c *Client) GetPositions(ctx context.Context, market string, page, pageSize int) ([]Position, error) {
	return c.REST.GetPositions(ctx, market, page, pageSize)
}

// PlaceOrder places a new order
func (c *Client) PlaceOrder(ctx context.Context, order *OrderRequest) (*Order, error) {
	return c.REST.PlaceOrder(ctx, order)
}

// CancelOrder cancels an order by ID
func (c *Client) CancelOrder(ctx context.Context, market string, orderID int64) (*Order, error) {
	return c.REST.CancelOrder(ctx, market, orderID)
}

// ClosePosition closes a position
func (c *Client) ClosePosition(ctx context.Context, req *ClosePositionRequest) (*Order, error) {
	return c.REST.ClosePosition(ctx, req)
}

// GetOpenOrders retrieves open orders
func (c *Client) GetOpenOrders(ctx context.Context, market string, side string, page, pageSize int) ([]Order, error) {
	return c.REST.GetPendingOrders(ctx, market, side, "", page, pageSize)
}

// GetFinishedOrders retrieves finished orders
func (c *Client) GetFinishedOrders(ctx context.Context, market, side string, page, pageSize int) ([]Order, error) {
	return c.REST.GetFinishedOrders(ctx, market, side, page, pageSize)
}

// =============================================================================
// Convenience Methods - WebSocket Subscriptions
// =============================================================================

// SubscribeOrderbook subscribes to orderbook updates for given markets
func (c *Client) SubscribeOrderbook(markets []string, depth int, isFull bool) error {
	if c.WSMarketData == nil {
		return fmt.Errorf("market data websocket not enabled")
	}
	return c.WSMarketData.SubscribeDepth(markets, depth, "0", isFull)
}

// SubscribeTrades subscribes to trade updates for given markets
func (c *Client) SubscribeTrades(markets []string) error {
	if c.WSMarketData == nil {
		return fmt.Errorf("market data websocket not enabled")
	}
	return c.WSMarketData.SubscribeDeals(markets)
}

// SubscribeBBO subscribes to best bid/offer updates for given markets
func (c *Client) SubscribeBBO(markets []string) error {
	if c.WSMarketData == nil {
		return fmt.Errorf("market data websocket not enabled")
	}
	return c.WSMarketData.SubscribeBBO(markets)
}

// SubscribeTicker subscribes to ticker updates for given markets
func (c *Client) SubscribeTicker(markets []string) error {
	if c.WSMarketData == nil {
		return fmt.Errorf("market data websocket not enabled")
	}
	return c.WSMarketData.SubscribeState(markets)
}

// SubscribeOrders subscribes to order updates for given markets
func (c *Client) SubscribeOrders(markets []string) error {
	if c.WSUserData == nil {
		return fmt.Errorf("user data websocket not enabled")
	}
	return c.WSUserData.SubscribeOrders(markets)
}

// SubscribePositions subscribes to position updates for given markets
func (c *Client) SubscribePositions(markets []string) error {
	if c.WSUserData == nil {
		return fmt.Errorf("user data websocket not enabled")
	}
	return c.WSUserData.SubscribePositions(markets)
}

// SubscribeAccountBalance subscribes to account balance updates
func (c *Client) SubscribeAccountBalance() error {
	if c.WSUserData == nil {
		return fmt.Errorf("user data websocket not enabled")
	}
	return c.WSUserData.SubscribeBalance()
}

// =============================================================================
// Callback Setters
// =============================================================================

// SetDepthHandler sets the handler for orderbook depth updates
func (c *Client) SetDepthHandler(handler func(*WSDepthUpdate)) {
	if c.WSMarketData != nil {
		c.WSMarketData.SetDepthHandler(handler)
	}
}

// SetTradesHandler sets the handler for trade updates
func (c *Client) SetTradesHandler(handler func(*WSDealsUpdate)) {
	if c.WSMarketData != nil {
		c.WSMarketData.SetDealsHandler(handler)
	}
}

// SetBBOHandler sets the handler for BBO updates
func (c *Client) SetBBOHandler(handler func(*WSBBOUpdate)) {
	if c.WSMarketData != nil {
		c.WSMarketData.SetBBOHandler(handler)
	}
}

// SetTickerHandler sets the handler for ticker/state updates
func (c *Client) SetTickerHandler(handler func(*WSStateUpdate)) {
	if c.WSMarketData != nil {
		c.WSMarketData.SetStateHandler(handler)
	}
}

// SetOrderHandler sets the handler for order updates
func (c *Client) SetOrderHandler(handler func(*WSOrderUpdate)) {
	if c.WSUserData != nil {
		c.WSUserData.SetOrderHandler(handler)
	}
}

// SetPositionHandler sets the handler for position updates
func (c *Client) SetPositionHandler(handler func(*WSPositionUpdate)) {
	if c.WSUserData != nil {
		c.WSUserData.SetPositionHandler(handler)
	}
}

// SetBalanceHandler sets the handler for balance updates
func (c *Client) SetBalanceHandler(handler func(*WSBalanceUpdate)) {
	if c.WSUserData != nil {
		c.WSUserData.SetBalanceHandler(handler)
	}
}

// SetErrorHandler sets the handler for errors
func (c *Client) SetErrorHandler(handler func(error)) {
	if c.WSMarketData != nil {
		c.WSMarketData.SetErrorHandler(handler)
	}
	if c.WSUserData != nil {
		c.WSUserData.SetErrorHandler(handler)
	}
}
