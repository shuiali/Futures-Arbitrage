// Package kucoin provides a unified client for KuCoin Futures exchange.
package kucoin

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ClientConfig holds configuration for the KuCoin client
type ClientConfig struct {
	// API credentials
	APIKey     string
	APISecret  string
	Passphrase string

	// REST API settings
	RESTBaseURL string // Default: https://api-futures.kucoin.com

	// Optional: Use testnet (sandbox)
	UseTestnet bool

	// Default currency for account operations
	DefaultCurrency string // Default: USDT

	// Request timeout
	Timeout time.Duration
}

// DefaultConfig returns default configuration
func DefaultConfig() *ClientConfig {
	return &ClientConfig{
		RESTBaseURL:     FuturesRESTBaseURL,
		DefaultCurrency: SettleUSDT,
		UseTestnet:      false,
		Timeout:         10 * time.Second,
	}
}

// TestnetConfig returns testnet (sandbox) configuration
func TestnetConfig() *ClientConfig {
	return &ClientConfig{
		RESTBaseURL:     "https://api-sandbox-futures.kucoin.com",
		DefaultCurrency: SettleUSDT,
		UseTestnet:      true,
		Timeout:         10 * time.Second,
	}
}

// Client is the unified KuCoin client
type Client struct {
	config *ClientConfig

	// REST client
	REST *RESTClient

	// WebSocket clients
	MarketData *WSMarketDataClient
	UserData   *WSUserDataClient

	// Trading client (REST-based wrapper)
	Trading *TradingClient

	// Handlers (set before connecting)
	marketDataHandler *WSMarketDataHandler
	userDataHandler   *WSUserDataHandler
	tradingHandler    *TradingHandler

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewClient creates a new unified KuCoin client
func NewClient(config *ClientConfig) *Client {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &Client{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize REST client
	c.REST = NewRESTClient(RESTClientConfig{
		BaseURL:    config.RESTBaseURL,
		APIKey:     config.APIKey,
		SecretKey:  config.APISecret,
		Passphrase: config.Passphrase,
		Timeout:    config.Timeout,
	})

	return c
}

// NewClientWithCredentials creates a client with API credentials
func NewClientWithCredentials(apiKey, apiSecret, passphrase string) *Client {
	config := DefaultConfig()
	config.APIKey = apiKey
	config.APISecret = apiSecret
	config.Passphrase = passphrase
	return NewClient(config)
}

// NewTestnetClient creates a testnet client
func NewTestnetClient(apiKey, apiSecret, passphrase string) *Client {
	config := TestnetConfig()
	config.APIKey = apiKey
	config.APISecret = apiSecret
	config.Passphrase = passphrase
	return NewClient(config)
}

// SetMarketDataHandler sets the market data callback handler
func (c *Client) SetMarketDataHandler(handler *WSMarketDataHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.marketDataHandler = handler
}

// SetUserDataHandler sets the user data callback handler
func (c *Client) SetUserDataHandler(handler *WSUserDataHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userDataHandler = handler
}

// SetTradingHandler sets the trading callback handler
func (c *Client) SetTradingHandler(handler *TradingHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tradingHandler = handler
}

// ConnectMarketData connects the market data WebSocket
func (c *Client) ConnectMarketData() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.MarketData == nil {
		c.MarketData = NewWSMarketDataClient(c.REST, c.marketDataHandler)
	}

	return c.MarketData.Connect()
}

// ConnectUserData connects the user data WebSocket (requires authentication)
func (c *Client) ConnectUserData() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.config.APIKey == "" || c.config.APISecret == "" || c.config.Passphrase == "" {
		return fmt.Errorf("API credentials required for user data connection")
	}

	if c.UserData == nil {
		c.UserData = NewWSUserDataClient(c.REST, c.userDataHandler)
	}

	return c.UserData.Connect()
}

// InitTrading initializes the trading client
func (c *Client) InitTrading() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Trading == nil {
		c.Trading = NewTradingClient(c.REST, c.tradingHandler)
	}
}

// ConnectAll connects all WebSocket clients
func (c *Client) ConnectAll() error {
	// Connect market data (no auth required)
	if err := c.ConnectMarketData(); err != nil {
		return fmt.Errorf("failed to connect market data: %w", err)
	}

	// Initialize trading
	c.InitTrading()

	// Connect user data (auth required)
	if c.config.APIKey != "" && c.config.APISecret != "" && c.config.Passphrase != "" {
		if err := c.ConnectUserData(); err != nil {
			return fmt.Errorf("failed to connect user data: %w", err)
		}
	}

	return nil
}

// Close closes all connections
func (c *Client) Close() error {
	c.cancel()

	var errs []error

	if c.MarketData != nil {
		if err := c.MarketData.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if c.UserData != nil {
		if err := c.UserData.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if c.Trading != nil {
		if err := c.Trading.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connections: %v", errs)
	}

	return nil
}

// GetConfig returns the client configuration (read-only)
func (c *Client) GetConfig() ClientConfig {
	return *c.config
}

// DefaultCurrency returns the default currency
func (c *Client) DefaultCurrency() string {
	return c.config.DefaultCurrency
}

// IsTestnet returns true if using testnet
func (c *Client) IsTestnet() bool {
	return c.config.UseTestnet
}

// =============================================================================
// Convenience Methods - REST API (Market Data)
// =============================================================================

// GetContracts returns all available futures contracts
func (c *Client) GetContracts(ctx context.Context) ([]*Contract, error) {
	return c.REST.GetContracts(ctx)
}

// GetContract returns a specific contract
func (c *Client) GetContract(ctx context.Context, symbol string) (*Contract, error) {
	return c.REST.GetContract(ctx, symbol)
}

// GetTicker returns ticker for a specific symbol
func (c *Client) GetTicker(ctx context.Context, symbol string) (*Ticker, error) {
	return c.REST.GetTicker(ctx, symbol)
}

// GetAllTickers returns all tickers
func (c *Client) GetAllTickers(ctx context.Context) ([]*AllTickersItem, error) {
	return c.REST.GetAllTickers(ctx)
}

// GetOrderBook returns orderbook for a symbol
func (c *Client) GetOrderBook(ctx context.Context, symbol string) (*OrderBook, error) {
	return c.REST.GetOrderBook(ctx, symbol)
}

// GetOrderBookDepth returns partial orderbook (20 or 100 levels)
func (c *Client) GetOrderBookDepth(ctx context.Context, symbol string, depth int) (*OrderBook, error) {
	return c.REST.GetOrderBookDepth(ctx, symbol, depth)
}

// GetKlines returns candlestick data
func (c *Client) GetKlines(ctx context.Context, symbol string, granularity int, from, to int64) ([][]interface{}, error) {
	return c.REST.GetKlines(ctx, symbol, granularity, from, to)
}

// GetTradeHistory returns recent trades
func (c *Client) GetTradeHistory(ctx context.Context, symbol string) ([]*Trade, error) {
	return c.REST.GetTradeHistory(ctx, symbol)
}

// GetFundingRate returns current funding rate
func (c *Client) GetFundingRate(ctx context.Context, symbol string) (*FundingRate, error) {
	return c.REST.GetFundingRate(ctx, symbol)
}

// GetFundingRateHistory returns historical funding rates
func (c *Client) GetFundingRateHistory(ctx context.Context, symbol string, from, to int64) ([]*FundingRateHistory, error) {
	return c.REST.GetFundingRateHistory(ctx, symbol, from, to)
}

// GetMarkPrice returns current mark price
func (c *Client) GetMarkPrice(ctx context.Context, symbol string) (*MarkPrice, error) {
	return c.REST.GetMarkPrice(ctx, symbol)
}

// GetServiceStatus returns service status
func (c *Client) GetServiceStatus(ctx context.Context) (*ServiceStatus, error) {
	return c.REST.GetServiceStatus(ctx)
}

// =============================================================================
// Convenience Methods - REST API (Account & Trading)
// =============================================================================

// GetAccount returns futures account info
func (c *Client) GetAccount(ctx context.Context, currency string) (*Account, error) {
	if currency == "" {
		currency = c.config.DefaultCurrency
	}
	return c.REST.GetAccount(ctx, currency)
}

// GetPositions returns all positions
func (c *Client) GetPositions(ctx context.Context, currency string) ([]*Position, error) {
	return c.REST.GetPositions(ctx, currency)
}

// GetPosition returns position for a specific symbol
func (c *Client) GetPosition(ctx context.Context, symbol string) (*Position, error) {
	return c.REST.GetPosition(ctx, symbol)
}

// PlaceOrder places a new order
func (c *Client) PlaceOrder(ctx context.Context, req *OrderRequest) (*OrderResponse, error) {
	if c.Trading == nil {
		c.InitTrading()
	}
	return c.Trading.PlaceOrder(ctx, req)
}

// PlaceLimitOrder places a limit order
func (c *Client) PlaceLimitOrder(ctx context.Context, symbol, side string, size int, price string, opts ...OrderOption) (*OrderResponse, error) {
	if c.Trading == nil {
		c.InitTrading()
	}
	return c.Trading.PlaceLimitOrder(ctx, symbol, side, size, price, opts...)
}

// PlaceMarketOrder places a market order
func (c *Client) PlaceMarketOrder(ctx context.Context, symbol, side string, size int, opts ...OrderOption) (*OrderResponse, error) {
	if c.Trading == nil {
		c.InitTrading()
	}
	return c.Trading.PlaceMarketOrder(ctx, symbol, side, size, opts...)
}

// CancelOrder cancels an order
func (c *Client) CancelOrder(ctx context.Context, orderID string) (*CancelResponse, error) {
	if c.Trading == nil {
		c.InitTrading()
	}
	return c.Trading.CancelOrder(ctx, orderID)
}

// CancelAllOrders cancels all orders for a symbol
func (c *Client) CancelAllOrders(ctx context.Context, symbol string) (*CancelResponse, error) {
	if c.Trading == nil {
		c.InitTrading()
	}
	return c.Trading.CancelAllOrders(ctx, symbol)
}

// GetOrders returns orders
func (c *Client) GetOrders(ctx context.Context, symbol, status string, pageSize, currentPage int) (*OrderList, error) {
	return c.REST.GetOrders(ctx, symbol, status, pageSize, currentPage)
}

// GetOrder returns a specific order
func (c *Client) GetOrder(ctx context.Context, orderID string) (*Order, error) {
	return c.REST.GetOrder(ctx, orderID)
}

// GetFills returns fill history
func (c *Client) GetFills(ctx context.Context, symbol, orderID string, pageSize, currentPage int) (*FillList, error) {
	return c.REST.GetFills(ctx, symbol, orderID, pageSize, currentPage)
}

// =============================================================================
// Convenience Methods - WebSocket Market Data
// =============================================================================

// SubscribeTicker subscribes to ticker updates
func (c *Client) SubscribeTicker(symbol string) error {
	if c.MarketData == nil {
		if err := c.ConnectMarketData(); err != nil {
			return err
		}
	}
	return c.MarketData.SubscribeTicker(symbol)
}

// SubscribeTickers subscribes to multiple tickers
func (c *Client) SubscribeTickers(symbols []string) error {
	if c.MarketData == nil {
		if err := c.ConnectMarketData(); err != nil {
			return err
		}
	}
	return c.MarketData.SubscribeTickers(symbols)
}

// SubscribeOrderBook5 subscribes to 5-level orderbook
func (c *Client) SubscribeOrderBook5(symbol string) error {
	if c.MarketData == nil {
		if err := c.ConnectMarketData(); err != nil {
			return err
		}
	}
	return c.MarketData.SubscribeOrderBook5(symbol)
}

// SubscribeOrderBook50 subscribes to 50-level orderbook
func (c *Client) SubscribeOrderBook50(symbol string) error {
	if c.MarketData == nil {
		if err := c.ConnectMarketData(); err != nil {
			return err
		}
	}
	return c.MarketData.SubscribeOrderBook50(symbol)
}

// SubscribeExecution subscribes to trade execution updates
func (c *Client) SubscribeExecution(symbol string) error {
	if c.MarketData == nil {
		if err := c.ConnectMarketData(); err != nil {
			return err
		}
	}
	return c.MarketData.SubscribeExecution(symbol)
}

// SubscribeInstrument subscribes to instrument info (mark/index price, funding)
func (c *Client) SubscribeInstrument(symbol string) error {
	if c.MarketData == nil {
		if err := c.ConnectMarketData(); err != nil {
			return err
		}
	}
	return c.MarketData.SubscribeInstrument(symbol)
}

// =============================================================================
// Convenience Methods - WebSocket User Data
// =============================================================================

// SubscribeTradeOrders subscribes to order updates
func (c *Client) SubscribeTradeOrders() error {
	if c.UserData == nil {
		if err := c.ConnectUserData(); err != nil {
			return err
		}
	}
	return c.UserData.SubscribeTradeOrders()
}

// SubscribeUserPosition subscribes to position updates
func (c *Client) SubscribeUserPosition(symbol string) error {
	if c.UserData == nil {
		if err := c.ConnectUserData(); err != nil {
			return err
		}
	}
	return c.UserData.SubscribePosition(symbol)
}

// SubscribeWallet subscribes to balance updates
func (c *Client) SubscribeWallet() error {
	if c.UserData == nil {
		if err := c.ConnectUserData(); err != nil {
			return err
		}
	}
	return c.UserData.SubscribeWallet()
}

// SubscribeAllUserData subscribes to all user data channels
func (c *Client) SubscribeAllUserData(symbols ...string) error {
	if c.UserData == nil {
		if err := c.ConnectUserData(); err != nil {
			return err
		}
	}
	return c.UserData.SubscribeAll(symbols...)
}

// =============================================================================
// Utility Methods
// =============================================================================

// Ping checks API connectivity
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.REST.GetServiceStatus(ctx)
	return err
}

// GetServerTime returns server timestamp (approximation via contract query)
func (c *Client) GetServerTime(ctx context.Context) (time.Time, error) {
	// KuCoin doesn't have a direct server time endpoint for futures
	// Use a contract query and extract timestamp
	contracts, err := c.REST.GetContracts(ctx)
	if err != nil {
		return time.Time{}, err
	}
	if len(contracts) > 0 && contracts[0].NextFundingRateTime > 0 {
		// Return approximate server time
		return time.Now(), nil
	}
	return time.Now(), nil
}

// LogStatus logs the current connection status
func (c *Client) LogStatus() {
	log.Printf("[KuCoin Client] Status:")
	log.Printf("  - Testnet: %v", c.config.UseTestnet)
	log.Printf("  - Has credentials: %v", c.config.APIKey != "")

	if c.MarketData != nil {
		log.Printf("  - Market Data WS: connected=%v, subs=%d",
			c.MarketData.IsConnected(), len(c.MarketData.GetSubscriptions()))
	} else {
		log.Printf("  - Market Data WS: not initialized")
	}

	if c.UserData != nil {
		log.Printf("  - User Data WS: connected=%v, subs=%d",
			c.UserData.IsConnected(), len(c.UserData.GetSubscriptions()))
	} else {
		log.Printf("  - User Data WS: not initialized")
	}

	if c.Trading != nil {
		log.Printf("  - Trading: initialized")
	} else {
		log.Printf("  - Trading: not initialized")
	}
}
