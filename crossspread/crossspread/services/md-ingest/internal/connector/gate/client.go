package gate

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// ClientConfig holds configuration for the Gate.io client
type ClientConfig struct {
	// API credentials
	APIKey    string
	APISecret string

	// REST API settings
	RESTBaseURL string // Default: https://api.gateio.ws

	// WebSocket settings
	WSBaseURL string // Default: wss://fx-ws.gateio.ws/v4/ws

	// Optional: Use testnet
	UseTestnet bool

	// Default settle currency (btc or usdt)
	DefaultSettle string // Default: usdt
}

// DefaultConfig returns default configuration
func DefaultConfig() *ClientConfig {
	return &ClientConfig{
		RESTBaseURL:   "https://api.gateio.ws",
		WSBaseURL:     "wss://fx-ws.gateio.ws/v4/ws",
		DefaultSettle: SettleUSDT,
		UseTestnet:    false,
	}
}

// TestnetConfig returns testnet configuration
func TestnetConfig() *ClientConfig {
	return &ClientConfig{
		RESTBaseURL:   "https://api.gateio.ws",
		WSBaseURL:     "wss://fx-ws-testnet.gateio.ws/v4/ws",
		DefaultSettle: SettleUSDT,
		UseTestnet:    true,
	}
}

// Client is the unified Gate.io client
type Client struct {
	config *ClientConfig

	// REST client
	REST *RESTClient

	// WebSocket clients
	MarketData *WSMarketDataClient
	Trading    *WSTradingClient
	UserData   *WSUserDataClient

	// Handlers (set before connecting)
	marketDataHandler *WSMarketDataHandler
	tradingHandler    *WSTradingHandler
	userDataHandler   *WSUserDataHandler

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewClient creates a new unified Gate.io client
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

// NewTestnetClient creates a testnet client
func NewTestnetClient(apiKey, apiSecret string) *Client {
	config := TestnetConfig()
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

// SetTradingHandler sets the trading callback handler
func (c *Client) SetTradingHandler(handler *WSTradingHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tradingHandler = handler
}

// SetUserDataHandler sets the user data callback handler
func (c *Client) SetUserDataHandler(handler *WSUserDataHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userDataHandler = handler
}

// ConnectMarketData connects the market data WebSocket
func (c *Client) ConnectMarketData(settle string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.MarketData == nil {
		c.MarketData = NewWSMarketDataClient(c.config.WSBaseURL, c.marketDataHandler)
	}

	return c.MarketData.Connect(settle)
}

// ConnectTrading connects the trading WebSocket
func (c *Client) ConnectTrading(settle string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.config.APIKey == "" || c.config.APISecret == "" {
		return fmt.Errorf("API credentials required for trading connection")
	}

	if c.Trading == nil {
		c.Trading = NewWSTradingClient(c.config.WSBaseURL, c.config.APIKey, c.config.APISecret, c.tradingHandler)
	}

	return c.Trading.Connect(settle)
}

// ConnectUserData connects the user data WebSocket
func (c *Client) ConnectUserData(settle string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.config.APIKey == "" || c.config.APISecret == "" {
		return fmt.Errorf("API credentials required for user data connection")
	}

	if c.UserData == nil {
		c.UserData = NewWSUserDataClient(c.config.WSBaseURL, c.config.APIKey, c.config.APISecret, c.userDataHandler)
	}

	return c.UserData.Connect(settle)
}

// ConnectAll connects all WebSocket clients for a settle currency
func (c *Client) ConnectAll(settle string) error {
	if settle == "" {
		settle = c.config.DefaultSettle
	}

	// Connect market data (no auth required)
	if err := c.ConnectMarketData(settle); err != nil {
		return fmt.Errorf("failed to connect market data: %w", err)
	}

	// Connect trading and user data (auth required)
	if c.config.APIKey != "" && c.config.APISecret != "" {
		if err := c.ConnectTrading(settle); err != nil {
			return fmt.Errorf("failed to connect trading: %w", err)
		}

		if err := c.ConnectUserData(settle); err != nil {
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

	if c.Trading != nil {
		if err := c.Trading.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if c.UserData != nil {
		if err := c.UserData.Close(); err != nil {
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

// DefaultSettle returns the default settle currency
func (c *Client) DefaultSettle() string {
	return c.config.DefaultSettle
}

// IsTestnet returns true if using testnet
func (c *Client) IsTestnet() bool {
	return c.config.UseTestnet
}

// =============================================================================
// Convenience Methods - REST API
// =============================================================================

// GetContracts returns all available futures contracts
func (c *Client) GetContracts(ctx context.Context, settle string) ([]Contract, error) {
	return c.REST.GetContracts(ctx, settle)
}

// GetContract returns a specific contract
func (c *Client) GetContract(ctx context.Context, settle, contract string) (*Contract, error) {
	return c.REST.GetContract(ctx, settle, contract)
}

// GetTickers returns all tickers for a settle currency
func (c *Client) GetTickers(ctx context.Context, settle string) ([]Ticker, error) {
	return c.REST.GetTickers(ctx, settle, "")
}

// GetTicker returns ticker for a specific contract
func (c *Client) GetTicker(ctx context.Context, settle, contract string) (*Ticker, error) {
	tickers, err := c.REST.GetTickers(ctx, settle, contract)
	if err != nil {
		return nil, err
	}
	if len(tickers) == 0 {
		return nil, fmt.Errorf("no ticker found for %s", contract)
	}
	return &tickers[0], nil
}

// GetOrderBook returns orderbook for a contract
func (c *Client) GetOrderBook(ctx context.Context, settle, contract string, depth int) (*OrderBook, error) {
	return c.REST.GetOrderBook(ctx, settle, contract, "", depth, false)
}

// GetTrades returns recent trades
func (c *Client) GetTrades(ctx context.Context, settle, contract string, limit int) ([]Trade, error) {
	return c.REST.GetTrades(ctx, settle, contract, limit, 0, 0)
}

// GetCandlesticks returns candlestick/kline data
func (c *Client) GetCandlesticks(ctx context.Context, settle, contract, interval string, limit int) ([]Candlestick, error) {
	return c.REST.GetCandlesticks(ctx, settle, contract, interval, 0, 0, limit)
}

// GetFundingRateHistory returns historical funding rates
func (c *Client) GetFundingRateHistory(ctx context.Context, settle, contract string, limit int) ([]FundingRateHistory, error) {
	return c.REST.GetFundingRateHistory(ctx, settle, contract, limit, 0, 0)
}

// GetAccount returns futures account info
func (c *Client) GetAccount(ctx context.Context, settle string) (*FuturesAccount, error) {
	return c.REST.GetAccount(ctx, settle)
}

// GetPositions returns all positions
func (c *Client) GetPositions(ctx context.Context, settle string) ([]Position, error) {
	return c.REST.GetPositions(ctx, settle, false)
}

// GetPosition returns position for a specific contract
func (c *Client) GetPosition(ctx context.Context, settle, contract string) (*Position, error) {
	return c.REST.GetPosition(ctx, settle, contract)
}

// PlaceOrder places a new order
func (c *Client) PlaceOrder(ctx context.Context, settle string, req *OrderRequest) (*Order, error) {
	return c.REST.PlaceOrder(ctx, settle, req)
}

// CancelOrder cancels an order
func (c *Client) CancelOrder(ctx context.Context, settle string, orderID string) (*Order, error) {
	return c.REST.CancelOrder(ctx, settle, orderID)
}

// GetOrders returns open orders
func (c *Client) GetOrders(ctx context.Context, settle, contract, status string, limit int) ([]Order, error) {
	return c.REST.GetOrders(ctx, settle, contract, status, limit, 0, "")
}

// GetOrder returns a specific order
func (c *Client) GetOrder(ctx context.Context, settle string, orderID string) (*Order, error) {
	return c.REST.GetOrder(ctx, settle, orderID)
}

// =============================================================================
// Convenience Methods - WebSocket Market Data
// =============================================================================

// SubscribeTickers subscribes to ticker updates
func (c *Client) SubscribeTickers(settle string, contracts []string) error {
	if c.MarketData == nil {
		if err := c.ConnectMarketData(settle); err != nil {
			return err
		}
	}
	return c.MarketData.SubscribeTickers(settle, contracts)
}

// SubscribeAllTickers subscribes to all ticker updates
func (c *Client) SubscribeAllTickers(settle string) error {
	return c.SubscribeTickers(settle, []string{"!all"})
}

// SubscribeOrderBook subscribes to orderbook updates
func (c *Client) SubscribeOrderBook(settle, contract string, level, interval string) error {
	if c.MarketData == nil {
		if err := c.ConnectMarketData(settle); err != nil {
			return err
		}
	}
	return c.MarketData.SubscribeOrderBook(settle, contract, level, interval)
}

// SubscribeTrades subscribes to trade updates
func (c *Client) SubscribeTrades(settle string, contracts []string) error {
	if c.MarketData == nil {
		if err := c.ConnectMarketData(settle); err != nil {
			return err
		}
	}
	return c.MarketData.SubscribeTrades(settle, contracts)
}

// SubscribeBookTicker subscribes to best bid/ask updates
func (c *Client) SubscribeBookTicker(settle string, contracts []string) error {
	if c.MarketData == nil {
		if err := c.ConnectMarketData(settle); err != nil {
			return err
		}
	}
	return c.MarketData.SubscribeBookTicker(settle, contracts)
}

// =============================================================================
// Convenience Methods - WebSocket Trading
// =============================================================================

// PlaceOrderWS places order via WebSocket (low latency)
func (c *Client) PlaceOrderWS(settle string, req *OrderRequest) (*Order, error) {
	if c.Trading == nil {
		if err := c.ConnectTrading(settle); err != nil {
			return nil, err
		}
	}
	return c.Trading.PlaceOrder(settle, req)
}

// PlaceOrderAsync places order asynchronously (lowest latency)
func (c *Client) PlaceOrderAsync(settle string, req *OrderRequest) (string, error) {
	if c.Trading == nil {
		if err := c.ConnectTrading(settle); err != nil {
			return "", err
		}
	}
	return c.Trading.PlaceOrderAsync(settle, req)
}

// CancelOrderWS cancels order via WebSocket
func (c *Client) CancelOrderWS(settle string, orderID string) (*Order, error) {
	if c.Trading == nil {
		if err := c.ConnectTrading(settle); err != nil {
			return nil, err
		}
	}
	return c.Trading.CancelOrder(settle, orderID)
}

// CancelOrderAsync cancels order asynchronously
func (c *Client) CancelOrderAsync(settle string, orderID string) (string, error) {
	if c.Trading == nil {
		if err := c.ConnectTrading(settle); err != nil {
			return "", err
		}
	}
	return c.Trading.CancelOrderAsync(settle, orderID)
}

// AmendOrderWS amends order via WebSocket
func (c *Client) AmendOrderWS(settle string, orderID string, price string, size int64) (*Order, error) {
	if c.Trading == nil {
		if err := c.ConnectTrading(settle); err != nil {
			return nil, err
		}
	}
	return c.Trading.AmendOrder(settle, orderID, price, size)
}

// =============================================================================
// Convenience Methods - WebSocket User Data
// =============================================================================

// SubscribeOrders subscribes to order updates
func (c *Client) SubscribeOrders(settle string, contracts []string) error {
	if c.UserData == nil {
		if err := c.ConnectUserData(settle); err != nil {
			return err
		}
	}
	return c.UserData.SubscribeOrders(settle, contracts)
}

// SubscribePositions subscribes to position updates
func (c *Client) SubscribePositions(settle string, contracts []string) error {
	if c.UserData == nil {
		if err := c.ConnectUserData(settle); err != nil {
			return err
		}
	}
	return c.UserData.SubscribePositions(settle, contracts)
}

// SubscribeBalances subscribes to balance updates
func (c *Client) SubscribeBalances(settle string) error {
	if c.UserData == nil {
		if err := c.ConnectUserData(settle); err != nil {
			return err
		}
	}
	return c.UserData.SubscribeBalances(settle)
}

// SubscribeUserTrades subscribes to user trade updates
func (c *Client) SubscribeUserTrades(settle string, contracts []string) error {
	if c.UserData == nil {
		if err := c.ConnectUserData(settle); err != nil {
			return err
		}
	}
	return c.UserData.SubscribeUserTrades(settle, contracts)
}

// SubscribeAllUserData subscribes to all user data channels
func (c *Client) SubscribeAllUserData(settle string) error {
	if c.UserData == nil {
		if err := c.ConnectUserData(settle); err != nil {
			return err
		}
	}
	return c.UserData.SubscribeAll(settle)
}

// =============================================================================
// Helper Methods
// =============================================================================

// NewLimitOrder creates a limit order request
func NewLimitOrder(contract string, size int64, price string, tif string) *OrderRequest {
	if tif == "" {
		tif = TIFGoodTillCancel
	}
	return &OrderRequest{
		Contract: contract,
		Size:     size,
		Price:    price,
		TIF:      tif,
	}
}

// NewMarketOrder creates a market order request (IOC with price 0)
func NewMarketOrder(contract string, size int64) *OrderRequest {
	return &OrderRequest{
		Contract: contract,
		Size:     size,
		Price:    "0",
		TIF:      TIFImmediateOrCancel,
	}
}

// NewReduceOnlyOrder creates a reduce-only order
func NewReduceOnlyOrder(contract string, size int64, price string) *OrderRequest {
	return &OrderRequest{
		Contract:   contract,
		Size:       size,
		Price:      price,
		TIF:        TIFGoodTillCancel,
		ReduceOnly: true,
	}
}

// NewCloseOrder creates a close position order
func NewCloseOrder(contract string, size int64, price string) *OrderRequest {
	return &OrderRequest{
		Contract: contract,
		Size:     size,
		Price:    price,
		TIF:      TIFGoodTillCancel,
		Close:    true,
	}
}

// PrintConnectionStatus prints current connection status
func (c *Client) PrintConnectionStatus(settle string) {
	log.Printf("[Gate.io Client] Connection Status for %s:", settle)

	if c.MarketData != nil {
		log.Printf("  Market Data: connected=%v", c.MarketData.IsConnected(settle))
	} else {
		log.Printf("  Market Data: not initialized")
	}

	if c.Trading != nil {
		log.Printf("  Trading: connected=%v, logged_in=%v",
			c.Trading.IsConnected(settle), c.Trading.IsLoggedIn(settle))
	} else {
		log.Printf("  Trading: not initialized")
	}

	if c.UserData != nil {
		log.Printf("  User Data: connected=%v, logged_in=%v",
			c.UserData.IsConnected(settle), c.UserData.IsLoggedIn(settle))
	} else {
		log.Printf("  User Data: not initialized")
	}
}
