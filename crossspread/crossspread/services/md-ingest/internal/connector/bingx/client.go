// Package bingx provides a unified client for BingX Perpetual Futures exchange.
package bingx

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ClientConfig holds configuration for the BingX client
type ClientConfig struct {
	// API credentials
	APIKey    string
	APISecret string

	// REST API settings
	RESTBaseURL string // Default: https://open-api.bingx.com

	// Request timeout
	Timeout time.Duration
}

// DefaultConfig returns default configuration
func DefaultConfig() *ClientConfig {
	return &ClientConfig{
		RESTBaseURL: RESTBaseURL,
		Timeout:     10 * time.Second,
	}
}

// Client is the unified BingX client
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

// NewClient creates a new unified BingX client
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
		BaseURL:   config.RESTBaseURL,
		APIKey:    config.APIKey,
		SecretKey: config.APISecret,
		Timeout:   config.Timeout,
	})

	return c
}

// NewClientWithCredentials creates a client with API credentials
func NewClientWithCredentials(apiKey, apiSecret string) *Client {
	config := DefaultConfig()
	config.APIKey = apiKey
	config.APISecret = apiSecret
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
		c.MarketData = NewWSMarketDataClient(c.marketDataHandler)
	}

	return c.MarketData.Connect()
}

// ConnectUserData connects the user data WebSocket (requires authentication)
func (c *Client) ConnectUserData() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.config.APIKey == "" || c.config.APISecret == "" {
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
	if c.config.APIKey != "" && c.config.APISecret != "" {
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

// =============================================================================
// Convenience Methods - REST API (Market Data)
// =============================================================================

// GetContracts returns all available perpetual contracts
func (c *Client) GetContracts(ctx context.Context) ([]*Contract, error) {
	return c.REST.GetContracts(ctx)
}

// GetPrice returns current price for a symbol
func (c *Client) GetPrice(ctx context.Context, symbol string) ([]*Price, error) {
	return c.REST.GetPrice(ctx, symbol)
}

// GetTicker returns 24hr ticker for a symbol
func (c *Client) GetTicker(ctx context.Context, symbol string) ([]*Ticker, error) {
	return c.REST.GetTicker(ctx, symbol)
}

// GetOrderBook returns orderbook for a symbol
func (c *Client) GetOrderBook(ctx context.Context, symbol string, limit int) (*OrderBook, error) {
	return c.REST.GetOrderBook(ctx, symbol, limit)
}

// GetTrades returns recent trades for a symbol
func (c *Client) GetTrades(ctx context.Context, symbol string, limit int) ([]*Trade, error) {
	return c.REST.GetTrades(ctx, symbol, limit)
}

// GetKlines returns candlestick data
func (c *Client) GetKlines(ctx context.Context, symbol, interval string, startTime, endTime int64, limit int) ([]*Kline, error) {
	return c.REST.GetKlines(ctx, symbol, interval, startTime, endTime, limit)
}

// GetPremiumIndex returns mark price and funding rate
func (c *Client) GetPremiumIndex(ctx context.Context, symbol string) ([]*PremiumIndex, error) {
	return c.REST.GetPremiumIndex(ctx, symbol)
}

// GetFundingRateHistory returns historical funding rates
func (c *Client) GetFundingRateHistory(ctx context.Context, symbol string, startTime, endTime int64, limit int) ([]*FundingRateHistory, error) {
	return c.REST.GetFundingRateHistory(ctx, symbol, startTime, endTime, limit)
}

// GetOpenInterest returns open interest for a symbol
func (c *Client) GetOpenInterest(ctx context.Context, symbol string) (*OpenInterest, error) {
	return c.REST.GetOpenInterest(ctx, symbol)
}

// =============================================================================
// Convenience Methods - REST API (Account)
// =============================================================================

// GetBalance returns account balance
func (c *Client) GetBalance(ctx context.Context) (*AccountBalance, error) {
	return c.REST.GetBalance(ctx)
}

// GetPositions returns all positions
func (c *Client) GetPositions(ctx context.Context, symbol string) ([]*Position, error) {
	return c.REST.GetPositions(ctx, symbol)
}

// GetCommissionRate returns commission rate for a symbol
func (c *Client) GetCommissionRate(ctx context.Context, symbol string) (*CommissionRate, error) {
	return c.REST.GetCommissionRate(ctx, symbol)
}

// =============================================================================
// Convenience Methods - Trading
// =============================================================================

// PlaceOrder places a new order
func (c *Client) PlaceOrder(ctx context.Context, req *OrderRequest) (*OrderResponse, error) {
	if c.Trading != nil {
		return c.Trading.PlaceOrder(ctx, req)
	}
	return c.REST.PlaceOrder(ctx, req)
}

// CancelOrder cancels an order
func (c *Client) CancelOrder(ctx context.Context, symbol string, orderID int64) (*CancelResponse, error) {
	if c.Trading != nil {
		return c.Trading.CancelOrder(ctx, symbol, orderID)
	}
	return c.REST.CancelOrder(ctx, symbol, orderID, "")
}

// GetOpenOrders returns all open orders
func (c *Client) GetOpenOrders(ctx context.Context, symbol string) ([]*Order, error) {
	return c.REST.GetOpenOrders(ctx, symbol)
}

// QueryOrder queries a specific order
func (c *Client) QueryOrder(ctx context.Context, symbol string, orderID int64, clientOrderID string) (*Order, error) {
	return c.REST.QueryOrder(ctx, symbol, orderID, clientOrderID)
}

// SetLeverage sets leverage for a symbol
func (c *Client) SetLeverage(ctx context.Context, symbol, side string, leverage int) error {
	return c.REST.SetLeverage(ctx, symbol, side, leverage)
}

// SetMarginType sets margin type for a symbol
func (c *Client) SetMarginType(ctx context.Context, symbol, marginType string) error {
	return c.REST.SetMarginType(ctx, symbol, marginType)
}

// =============================================================================
// Convenience Methods - WebSocket Subscriptions
// =============================================================================

// SubscribeDepth subscribes to orderbook depth for a symbol
func (c *Client) SubscribeDepth(symbol string, levels int) error {
	if c.MarketData == nil {
		return fmt.Errorf("market data client not initialized")
	}
	return c.MarketData.SubscribeDepth(symbol, levels)
}

// SubscribeTrade subscribes to trades for a symbol
func (c *Client) SubscribeTrade(symbol string) error {
	if c.MarketData == nil {
		return fmt.Errorf("market data client not initialized")
	}
	return c.MarketData.SubscribeTrade(symbol)
}

// SubscribeKline subscribes to klines for a symbol
func (c *Client) SubscribeKline(symbol, interval string) error {
	if c.MarketData == nil {
		return fmt.Errorf("market data client not initialized")
	}
	return c.MarketData.SubscribeKline(symbol, interval)
}

// SubscribeMultipleDepth subscribes to depth for multiple symbols
func (c *Client) SubscribeMultipleDepth(symbols []string, levels int) error {
	if c.MarketData == nil {
		return fmt.Errorf("market data client not initialized")
	}
	return c.MarketData.SubscribeMultipleDepth(symbols, levels)
}

// =============================================================================
// Status Methods
// =============================================================================

// IsMarketDataConnected returns whether market data WebSocket is connected
func (c *Client) IsMarketDataConnected() bool {
	if c.MarketData == nil {
		return false
	}
	return c.MarketData.IsConnected()
}

// IsUserDataConnected returns whether user data WebSocket is connected
func (c *Client) IsUserDataConnected() bool {
	if c.UserData == nil {
		return false
	}
	return c.UserData.IsConnected()
}

// =============================================================================
// Example Usage
// =============================================================================

// ExampleUsage demonstrates how to use the BingX client
func ExampleUsage() {
	// Create client with credentials
	client := NewClientWithCredentials("your-api-key", "your-api-secret")
	defer client.Close()

	// Set up market data handler
	client.SetMarketDataHandler(&WSMarketDataHandler{
		OnDepth: func(symbol string, depth *WSDepthData) {
			log.Printf("Depth update: %s, bids=%d, asks=%d", symbol, len(depth.Bids), len(depth.Asks))
		},
		OnTrade: func(symbol string, trades *WSTradeData) {
			log.Printf("Trade update: %s, trades=%d", symbol, len(trades.Trades))
		},
		OnConnect: func() {
			log.Println("Market data connected")
		},
		OnError: func(err error) {
			log.Printf("Market data error: %v", err)
		},
	})

	// Set up user data handler
	client.SetUserDataHandler(&WSUserDataHandler{
		OnAccountUpdate: func(update *WSAccountUpdate) {
			log.Printf("Account update: balances=%d, positions=%d", len(update.A.B), len(update.A.P))
		},
		OnOrderTradeUpdate: func(update *WSOrderTradeUpdate) {
			log.Printf("Order update: %s %s %s status=%s",
				update.O.S, update.O.SD, update.O.OT, update.O.XS)
		},
		OnConnect: func() {
			log.Println("User data connected")
		},
		OnError: func(err error) {
			log.Printf("User data error: %v", err)
		},
	})

	// Set up trading handler
	client.SetTradingHandler(&TradingHandler{
		OnOrderPlaced: func(order *OrderResponse) {
			log.Printf("Order placed: %d", order.OrderID)
		},
		OnOrderCanceled: func(cancel *CancelResponse) {
			log.Printf("Order canceled: %d", cancel.OrderID)
		},
		OnError: func(err error) {
			log.Printf("Trading error: %v", err)
		},
	})

	// Connect all
	if err := client.ConnectAll(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	// Subscribe to market data
	client.SubscribeDepth("BTC-USDT", 20)
	client.SubscribeTrade("BTC-USDT")

	ctx := context.Background()

	// Get contracts
	contracts, err := client.GetContracts(ctx)
	if err != nil {
		log.Printf("Failed to get contracts: %v", err)
	} else {
		log.Printf("Got %d contracts", len(contracts))
	}

	// Get balance
	balance, err := client.GetBalance(ctx)
	if err != nil {
		log.Printf("Failed to get balance: %v", err)
	} else {
		log.Printf("Balance: %s USDT", balance.Balance)
	}

	// Place order example (commented out for safety)
	/*
		order, err := client.PlaceOrder(ctx, &OrderRequest{
			Symbol:       "BTC-USDT",
			Type:         OrderTypeLimit,
			Side:         OrderSideBuy,
			PositionSide: PositionSideLong,
			Price:        50000,
			Quantity:     0.001,
			TimeInForce:  TIFGoodTillCancel,
		})
		if err != nil {
			log.Printf("Failed to place order: %v", err)
		} else {
			log.Printf("Order placed: %d", order.OrderID)
		}
	*/
}
