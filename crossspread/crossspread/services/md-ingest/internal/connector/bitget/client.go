// Package bitget provides a unified client for Bitget exchange.
// Combines REST and WebSocket clients for complete exchange integration.
package bitget

import (
	"context"
	"fmt"
	"time"
)

// Client provides unified access to Bitget REST and WebSocket APIs
type Client struct {
	rest       *RESTClient
	marketWS   *MarketDataWSClient
	tradingWS  *TradingWSClient
	userDataWS *UserDataWSClient

	config   *ClientConfig
	instType string
}

// ClientConfig holds configuration for the unified client
type ClientConfig struct {
	// API credentials
	APIKey     string
	SecretKey  string
	Passphrase string

	// Product type: USDT-FUTURES, USDC-FUTURES, COIN-FUTURES
	InstType string

	// Timeouts
	RESTTimeout time.Duration

	// WebSocket settings
	WSPingInterval  time.Duration
	WSPongWait      time.Duration
	WSReconnectWait time.Duration
	WSMaxReconnect  int
}

// DefaultClientConfig returns default client configuration
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		InstType:        ProductTypeUSDTFutures,
		RESTTimeout:     10 * time.Second,
		WSPingInterval:  25 * time.Second,
		WSPongWait:      30 * time.Second,
		WSReconnectWait: 5 * time.Second,
		WSMaxReconnect:  3,
	}
}

// NewClient creates a new unified Bitget client
func NewClient(cfg *ClientConfig) *Client {
	if cfg == nil {
		cfg = DefaultClientConfig()
	}
	if cfg.InstType == "" {
		cfg.InstType = ProductTypeUSDTFutures
	}

	// Create REST client
	restClient := NewRESTClient(RESTClientConfig{
		APIKey:     cfg.APIKey,
		SecretKey:  cfg.SecretKey,
		Passphrase: cfg.Passphrase,
		Timeout:    cfg.RESTTimeout,
	})

	return &Client{
		rest:     restClient,
		config:   cfg,
		instType: cfg.InstType,
	}
}

// REST returns the REST client
func (c *Client) REST() *RESTClient {
	return c.rest
}

// =============================================================================
// Market Data WebSocket
// =============================================================================

// ConnectMarketData connects to the public market data WebSocket
func (c *Client) ConnectMarketData(handler MarketDataHandler) error {
	if c.marketWS != nil {
		return fmt.Errorf("market data WebSocket already connected")
	}

	c.marketWS = NewMarketDataWSClient(MarketDataWSConfig{
		InstType:      c.instType,
		Handler:       handler,
		PingInterval:  c.config.WSPingInterval,
		PongWait:      c.config.WSPongWait,
		ReconnectWait: c.config.WSReconnectWait,
		MaxReconnect:  c.config.WSMaxReconnect,
	})

	return c.marketWS.Connect()
}

// MarketData returns the market data WebSocket client
func (c *Client) MarketData() *MarketDataWSClient {
	return c.marketWS
}

// DisconnectMarketData disconnects from the market data WebSocket
func (c *Client) DisconnectMarketData() error {
	if c.marketWS == nil {
		return nil
	}
	err := c.marketWS.Close()
	c.marketWS = nil
	return err
}

// =============================================================================
// Trading WebSocket
// =============================================================================

// ConnectTrading connects to the private trading WebSocket
func (c *Client) ConnectTrading(handler TradingHandler) error {
	if c.tradingWS != nil {
		return fmt.Errorf("trading WebSocket already connected")
	}

	c.tradingWS = NewTradingWSClient(TradingWSConfig{
		InstType:      c.instType,
		APIKey:        c.config.APIKey,
		SecretKey:     c.config.SecretKey,
		Passphrase:    c.config.Passphrase,
		Handler:       handler,
		PingInterval:  c.config.WSPingInterval,
		PongWait:      c.config.WSPongWait,
		ReconnectWait: c.config.WSReconnectWait,
		MaxReconnect:  c.config.WSMaxReconnect,
	})

	return c.tradingWS.Connect()
}

// Trading returns the trading WebSocket client
func (c *Client) Trading() *TradingWSClient {
	return c.tradingWS
}

// DisconnectTrading disconnects from the trading WebSocket
func (c *Client) DisconnectTrading() error {
	if c.tradingWS == nil {
		return nil
	}
	err := c.tradingWS.Close()
	c.tradingWS = nil
	return err
}

// =============================================================================
// User Data WebSocket
// =============================================================================

// ConnectUserData connects to the private user data WebSocket
func (c *Client) ConnectUserData(handler UserDataHandler) error {
	if c.userDataWS != nil {
		return fmt.Errorf("user data WebSocket already connected")
	}

	c.userDataWS = NewUserDataWSClient(UserDataWSConfig{
		InstType:      c.instType,
		APIKey:        c.config.APIKey,
		SecretKey:     c.config.SecretKey,
		Passphrase:    c.config.Passphrase,
		Handler:       handler,
		PingInterval:  c.config.WSPingInterval,
		PongWait:      c.config.WSPongWait,
		ReconnectWait: c.config.WSReconnectWait,
		MaxReconnect:  c.config.WSMaxReconnect,
	})

	return c.userDataWS.Connect()
}

// UserData returns the user data WebSocket client
func (c *Client) UserData() *UserDataWSClient {
	return c.userDataWS
}

// DisconnectUserData disconnects from the user data WebSocket
func (c *Client) DisconnectUserData() error {
	if c.userDataWS == nil {
		return nil
	}
	err := c.userDataWS.Close()
	c.userDataWS = nil
	return err
}

// =============================================================================
// Convenience Methods
// =============================================================================

// Close closes all connections
func (c *Client) Close() error {
	var errs []error

	if err := c.DisconnectMarketData(); err != nil {
		errs = append(errs, fmt.Errorf("market data disconnect: %w", err))
	}

	if err := c.DisconnectTrading(); err != nil {
		errs = append(errs, fmt.Errorf("trading disconnect: %w", err))
	}

	if err := c.DisconnectUserData(); err != nil {
		errs = append(errs, fmt.Errorf("user data disconnect: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during close: %v", errs)
	}
	return nil
}

// GetInstType returns the configured instrument type
func (c *Client) GetInstType() string {
	return c.instType
}

// SetInstType sets the instrument type
// Note: This will not affect already-connected WebSocket clients
func (c *Client) SetInstType(instType string) {
	c.instType = instType
}

// =============================================================================
// REST API Shortcuts
// =============================================================================

// GetContracts retrieves all contracts for the configured product type
func (c *Client) GetContracts(ctx context.Context) ([]Contract, error) {
	return c.rest.GetContracts(ctx, c.instType)
}

// GetTickers retrieves all tickers for the configured product type
func (c *Client) GetTickers(ctx context.Context) ([]Ticker, error) {
	return c.rest.GetTickers(ctx, c.instType)
}

// GetTicker retrieves ticker for a specific symbol
func (c *Client) GetTicker(ctx context.Context, symbol string) (*Ticker, error) {
	return c.rest.GetTicker(ctx, symbol, c.instType)
}

// GetOrderBook retrieves order book for a symbol
func (c *Client) GetOrderBook(ctx context.Context, symbol string, limit int) (*OrderBook, error) {
	return c.rest.GetOrderBook(ctx, symbol, c.instType, limit, "")
}

// GetCandles retrieves candlestick data
func (c *Client) GetCandles(ctx context.Context, symbol, granularity string, startTime, endTime int64, limit int) ([]Candlestick, error) {
	return c.rest.GetCandles(ctx, symbol, c.instType, granularity, startTime, endTime, limit)
}

// GetHistoryCandles retrieves historical candlestick data
func (c *Client) GetHistoryCandles(ctx context.Context, symbol, granularity string, startTime, endTime int64, limit int) ([]Candlestick, error) {
	return c.rest.GetHistoryCandles(ctx, symbol, c.instType, granularity, startTime, endTime, limit)
}

// GetCurrentFundingRate retrieves current funding rate
func (c *Client) GetCurrentFundingRate(ctx context.Context, symbol string) (*FundingRate, error) {
	return c.rest.GetCurrentFundingRate(ctx, symbol, c.instType)
}

// GetHistoryFundingRate retrieves historical funding rates
func (c *Client) GetHistoryFundingRate(ctx context.Context, symbol string, pageSize, pageNo int) ([]FundingRateHistory, error) {
	return c.rest.GetHistoryFundingRate(ctx, symbol, c.instType, pageSize, pageNo)
}

// GetAccount retrieves account balance
func (c *Client) GetAccount(ctx context.Context, symbol, marginCoin string) (*Account, error) {
	return c.rest.GetAccount(ctx, symbol, c.instType, marginCoin)
}

// GetAccounts retrieves all account balances
func (c *Client) GetAccounts(ctx context.Context) ([]AccountList, error) {
	return c.rest.GetAccounts(ctx, c.instType)
}

// GetPositions retrieves all positions
func (c *Client) GetPositions(ctx context.Context, marginCoin string) ([]Position, error) {
	return c.rest.GetPositions(ctx, c.instType, marginCoin)
}

// GetSinglePosition retrieves position for a symbol
func (c *Client) GetSinglePosition(ctx context.Context, symbol, marginCoin string) ([]Position, error) {
	return c.rest.GetSinglePosition(ctx, symbol, c.instType, marginCoin)
}

// PlaceOrder places an order
func (c *Client) PlaceOrder(ctx context.Context, req *PlaceOrderRequest) (*OrderResult, error) {
	if req.ProductType == "" {
		req.ProductType = c.instType
	}
	return c.rest.PlaceOrder(ctx, req)
}

// CancelOrder cancels an order
func (c *Client) CancelOrder(ctx context.Context, req *CancelOrderRequest) (*CancelResult, error) {
	if req.ProductType == "" {
		req.ProductType = c.instType
	}
	return c.rest.CancelOrder(ctx, req)
}

// GetPendingOrders retrieves pending orders
func (c *Client) GetPendingOrders(ctx context.Context, symbol string, pageSize int) ([]Order, error) {
	return c.rest.GetPendingOrders(ctx, c.instType, symbol, pageSize, "")
}

// GetHistoryOrders retrieves historical orders
func (c *Client) GetHistoryOrders(ctx context.Context, symbol string, startTime, endTime int64, pageSize int) ([]Order, error) {
	return c.rest.GetHistoryOrders(ctx, c.instType, symbol, startTime, endTime, pageSize, "")
}

// =============================================================================
// WebSocket Trading Shortcuts
// =============================================================================

// WSPlaceOrder places an order via WebSocket (low-latency)
func (c *Client) WSPlaceOrder(ctx context.Context, req *WSPlaceOrderArg) (*WSTradeResponse, error) {
	if c.tradingWS == nil {
		return nil, fmt.Errorf("trading WebSocket not connected")
	}
	return c.tradingWS.PlaceOrder(ctx, req)
}

// WSCancelOrder cancels an order via WebSocket (low-latency)
func (c *Client) WSCancelOrder(ctx context.Context, req *WSCancelOrderArg) (*WSTradeResponse, error) {
	if c.tradingWS == nil {
		return nil, fmt.Errorf("trading WebSocket not connected")
	}
	return c.tradingWS.CancelOrder(ctx, req)
}

// =============================================================================
// Market Data Subscription Shortcuts
// =============================================================================

// SubscribeTicker subscribes to ticker updates
func (c *Client) SubscribeTicker(instID string) error {
	if c.marketWS == nil {
		return fmt.Errorf("market data WebSocket not connected")
	}
	return c.marketWS.SubscribeTicker(instID)
}

// SubscribeOrderBook subscribes to order book updates
func (c *Client) SubscribeOrderBook(instID, channel string) error {
	if c.marketWS == nil {
		return fmt.Errorf("market data WebSocket not connected")
	}
	return c.marketWS.SubscribeOrderBook(instID, channel)
}

// SubscribeTrades subscribes to trade updates
func (c *Client) SubscribeTrades(instID string) error {
	if c.marketWS == nil {
		return fmt.Errorf("market data WebSocket not connected")
	}
	return c.marketWS.SubscribeTrades(instID)
}

// SubscribeCandles subscribes to candlestick updates
func (c *Client) SubscribeCandles(instID, channel string) error {
	if c.marketWS == nil {
		return fmt.Errorf("market data WebSocket not connected")
	}
	return c.marketWS.SubscribeCandles(instID, channel)
}
