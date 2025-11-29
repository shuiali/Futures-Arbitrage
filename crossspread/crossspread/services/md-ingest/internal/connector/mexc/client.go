// Package mexc provides unified MEXC exchange client.
package mexc

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Client is the unified MEXC exchange client
type Client struct {
	cfg    *ClientConfig
	rest   *RESTClient
	market *MarketDataWSClient
	trade  *TradingWSClient
	user   *UserDataWSClient

	mu sync.RWMutex

	marketHandler MarketDataHandler
	tradeHandler  TradingHandler
	userHandler   UserDataHandler
}

// ClientConfig holds configuration for the unified client
type ClientConfig struct {
	// API credentials
	APIKey    string
	SecretKey string

	// REST configuration
	RESTBaseURL string
	RESTTimeout int // seconds

	// WebSocket configuration
	WSReconnect     bool
	WSReconnectWait int // seconds
	WSMaxReconnect  int
	WSPingInterval  int // seconds

	// Testnet mode
	Testnet bool
}

// DefaultClientConfig returns default client configuration
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		RESTBaseURL:     BaseURLProduction,
		RESTTimeout:     30,
		WSReconnect:     true,
		WSReconnectWait: 5,
		WSMaxReconnect:  3,
		WSPingInterval:  20,
		Testnet:         false,
	}
}

// NewClient creates a new unified MEXC client
func NewClient(cfg *ClientConfig) (*Client, error) {
	if cfg == nil {
		cfg = DefaultClientConfig()
	}

	// Create REST client
	rest := NewRESTClient(RESTClientConfig{
		BaseURL:   cfg.RESTBaseURL,
		APIKey:    cfg.APIKey,
		SecretKey: cfg.SecretKey,
		Timeout:   time.Duration(cfg.RESTTimeout) * time.Second,
	})

	return &Client{
		cfg:  cfg,
		rest: rest,
	}, nil
}

// REST returns the REST client
func (c *Client) REST() *RESTClient {
	return c.rest
}

// SetMarketDataHandler sets the market data handler
func (c *Client) SetMarketDataHandler(handler MarketDataHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.marketHandler = handler
}

// SetTradingHandler sets the trading handler
func (c *Client) SetTradingHandler(handler TradingHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tradeHandler = handler
}

// SetUserDataHandler sets the user data handler
func (c *Client) SetUserDataHandler(handler UserDataHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userHandler = handler
}

// ConnectMarketData connects to market data WebSocket
func (c *Client) ConnectMarketData() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.market != nil {
		return fmt.Errorf("market data already connected")
	}

	c.market = NewMarketDataWSClient(MarketDataWSConfig{
		Handler:       c.marketHandler,
		PingInterval:  secondsToDuration(c.cfg.WSPingInterval),
		ReconnectWait: secondsToDuration(c.cfg.WSReconnectWait),
		MaxReconnect:  c.cfg.WSMaxReconnect,
	})

	if err := c.market.Connect(); err != nil {
		c.market = nil
		return fmt.Errorf("failed to connect market data: %w", err)
	}

	return nil
}

// ConnectTrading connects to trading WebSocket
func (c *Client) ConnectTrading() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cfg.APIKey == "" || c.cfg.SecretKey == "" {
		return fmt.Errorf("API credentials required for trading")
	}

	if c.trade != nil {
		return fmt.Errorf("trading already connected")
	}

	c.trade = NewTradingWSClient(TradingWSConfig{
		APIKey:        c.cfg.APIKey,
		SecretKey:     c.cfg.SecretKey,
		Handler:       c.tradeHandler,
		ReconnectWait: secondsToDuration(c.cfg.WSReconnectWait),
		MaxReconnect:  c.cfg.WSMaxReconnect,
	})

	if err := c.trade.Connect(); err != nil {
		c.trade = nil
		return fmt.Errorf("failed to connect trading: %w", err)
	}

	return nil
}

// ConnectUserData connects to user data WebSocket
func (c *Client) ConnectUserData() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cfg.APIKey == "" || c.cfg.SecretKey == "" {
		return fmt.Errorf("API credentials required for user data")
	}

	if c.user != nil {
		return fmt.Errorf("user data already connected")
	}

	c.user = NewUserDataWSClient(UserDataWSConfig{
		APIKey:        c.cfg.APIKey,
		SecretKey:     c.cfg.SecretKey,
		Handler:       c.userHandler,
		PingInterval:  secondsToDuration(c.cfg.WSPingInterval),
		ReconnectWait: secondsToDuration(c.cfg.WSReconnectWait),
		MaxReconnect:  c.cfg.WSMaxReconnect,
	})

	if err := c.user.Connect(); err != nil {
		c.user = nil
		return fmt.Errorf("failed to connect user data: %w", err)
	}

	return nil
}

// MarketData returns the market data WebSocket client
func (c *Client) MarketData() *MarketDataWSClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.market
}

// Trading returns the trading WebSocket client
func (c *Client) Trading() *TradingWSClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.trade
}

// UserData returns the user data WebSocket client
func (c *Client) UserData() *UserDataWSClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.user
}

// Close closes all connections
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var lastErr error

	if c.market != nil {
		if err := c.market.Close(); err != nil {
			lastErr = err
		}
		c.market = nil
	}

	if c.trade != nil {
		if err := c.trade.Close(); err != nil {
			lastErr = err
		}
		c.trade = nil
	}

	if c.user != nil {
		if err := c.user.Close(); err != nil {
			lastErr = err
		}
		c.user = nil
	}

	return lastErr
}

// =============================================================================
// Convenience Methods (wrapping REST client)
// =============================================================================

// GetContracts returns all available contracts
func (c *Client) GetContracts(ctx context.Context) ([]Contract, error) {
	return c.rest.GetContracts(ctx)
}

// GetTickers returns all tickers
func (c *Client) GetTickers(ctx context.Context) ([]Ticker, error) {
	return c.rest.GetTickers(ctx)
}

// GetDepth returns orderbook depth
func (c *Client) GetDepth(ctx context.Context, symbol string, limit int) (*OrderBook, error) {
	return c.rest.GetDepth(ctx, symbol, limit)
}

// GetFundingRate returns funding rate for a symbol
func (c *Client) GetFundingRate(ctx context.Context, symbol string) (*FundingRate, error) {
	return c.rest.GetFundingRate(ctx, symbol)
}

// GetAllFundingRates returns funding rates for all symbols
func (c *Client) GetAllFundingRates(ctx context.Context) ([]FundingRate, error) {
	return c.rest.GetAllFundingRates(ctx)
}

// GetKline returns kline/candlestick data
func (c *Client) GetKline(ctx context.Context, symbol string, interval KlineInterval, start, end int64) (*Kline, error) {
	return c.rest.GetKline(ctx, symbol, interval, start, end)
}

// GetAccountAssets returns account assets
func (c *Client) GetAccountAssets(ctx context.Context) ([]AccountAsset, error) {
	return c.rest.GetAccountAssets(ctx)
}

// GetOpenPositions returns all open positions
func (c *Client) GetOpenPositions(ctx context.Context, symbol string) ([]Position, error) {
	return c.rest.GetOpenPositions(ctx, symbol)
}

// PlaceOrder places a new order
func (c *Client) PlaceOrder(ctx context.Context, req *OrderRequest) (*OrderResponse, error) {
	return c.rest.PlaceOrder(ctx, req)
}

// CancelOrder cancels an order
func (c *Client) CancelOrder(ctx context.Context, req *CancelOrderRequest) error {
	return c.rest.CancelOrder(ctx, req)
}

// CancelAllOrders cancels all orders for a symbol
func (c *Client) CancelAllOrders(ctx context.Context, symbol string, positionID int64) error {
	return c.rest.CancelAllOrders(ctx, symbol, positionID)
}

// GetOpenOrders returns all open orders
func (c *Client) GetOpenOrders(ctx context.Context, symbol string, pageNum, pageSize int) ([]Order, error) {
	return c.rest.GetOpenOrders(ctx, symbol, pageNum, pageSize)
}

// GetOrder returns an order by ID
func (c *Client) GetOrder(ctx context.Context, orderID int64) (*Order, error) {
	return c.rest.GetOrder(ctx, orderID)
}

// GetOrderByExternalID returns an order by external ID
func (c *Client) GetOrderByExternalID(ctx context.Context, symbol, externalOID string) (*Order, error) {
	return c.rest.GetOrderByExternalID(ctx, symbol, externalOID)
}

// ChangeLeverage changes leverage for a symbol
func (c *Client) ChangeLeverage(ctx context.Context, req *LeverageRequest) error {
	return c.rest.ChangeLeverage(ctx, req)
}

// =============================================================================
// WebSocket Subscription Convenience Methods
// =============================================================================

// SubscribeTicker subscribes to ticker updates
func (c *Client) SubscribeTicker(symbol string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.market == nil {
		return fmt.Errorf("market data not connected")
	}
	return c.market.SubscribeTicker(symbol)
}

// SubscribeDepth subscribes to orderbook depth updates
func (c *Client) SubscribeDepth(symbol string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.market == nil {
		return fmt.Errorf("market data not connected")
	}
	return c.market.SubscribeDepth(symbol)
}

// SubscribeTrades subscribes to trade updates
func (c *Client) SubscribeTrades(symbol string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.market == nil {
		return fmt.Errorf("market data not connected")
	}
	return c.market.SubscribeDeal(symbol)
}

// SubscribeKline subscribes to kline updates
func (c *Client) SubscribeKline(symbol string, interval KlineInterval) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.market == nil {
		return fmt.Errorf("market data not connected")
	}
	return c.market.SubscribeKline(symbol, interval)
}

// SubscribeUserData subscribes to all user data streams
func (c *Client) SubscribeUserData() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.user == nil {
		return fmt.Errorf("user data not connected")
	}
	return c.user.SubscribeAll()
}

// =============================================================================
// WebSocket Trading Methods
// =============================================================================

// PlaceOrderWS places an order via WebSocket
func (c *Client) PlaceOrderWS(req *PlaceOrderRequest) (*WSOrderResponse, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.trade == nil {
		return nil, fmt.Errorf("trading not connected")
	}
	return c.trade.PlaceOrder(req)
}

// CancelOrderWS cancels an order via WebSocket
func (c *Client) CancelOrderWS(symbol, orderID string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.trade == nil {
		return fmt.Errorf("trading not connected")
	}
	return c.trade.CancelOrder(symbol, orderID)
}

// CancelAllOrdersWS cancels all orders via WebSocket
func (c *Client) CancelAllOrdersWS(symbol string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.trade == nil {
		return fmt.Errorf("trading not connected")
	}
	return c.trade.CancelAllOrders(symbol)
}

// =============================================================================
// Status Methods
// =============================================================================

// IsMarketDataConnected returns true if market data WebSocket is connected
func (c *Client) IsMarketDataConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.market != nil && c.market.IsConnected()
}

// IsTradingConnected returns true if trading WebSocket is connected and authenticated
func (c *Client) IsTradingConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.trade != nil && c.trade.IsAuthenticated()
}

// IsUserDataConnected returns true if user data WebSocket is connected and authenticated
func (c *Client) IsUserDataConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.user != nil && c.user.IsAuthenticated()
}

// =============================================================================
// Helper Functions
// =============================================================================

func secondsToDuration(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}
