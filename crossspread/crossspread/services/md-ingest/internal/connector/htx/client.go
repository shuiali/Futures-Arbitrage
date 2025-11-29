package htx

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Client is the unified HTX client combining REST and WebSocket functionality
type Client struct {
	config       *ClientConfig
	rest         *RestClient
	wsMarket     *WSMarketDataClient
	wsUser       *WSUserDataClient
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	onConnect    func()
	onDisconnect func()
	onError      func(error)
}

// ClientConfig holds HTX client configuration
type ClientConfig struct {
	APIKey            string
	SecretKey         string
	BaseURL           string
	WSMarketURL       string
	WSUserURL         string
	HTTPTimeout       time.Duration
	EnableRateLimit   bool
	ReconnectDelay    time.Duration
	MaxReconnectDelay time.Duration
}

// DefaultClientConfig returns default configuration
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		BaseURL:           RestBaseURL,
		WSMarketURL:       WSMarketURL,
		WSUserURL:         WSOrderURL,
		HTTPTimeout:       10 * time.Second,
		EnableRateLimit:   true,
		ReconnectDelay:    1 * time.Second,
		MaxReconnectDelay: 30 * time.Second,
	}
}

// NewClient creates a new HTX client
func NewClient(config *ClientConfig) *Client {
	if config == nil {
		config = DefaultClientConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create credentials
	var creds *Credentials
	if config.APIKey != "" && config.SecretKey != "" {
		creds = &Credentials{
			APIKey:    config.APIKey,
			SecretKey: config.SecretKey,
		}
	}

	// Create REST client
	var rest *RestClient
	if config.BaseURL != "" && config.BaseURL != RestBaseURL {
		rest = NewRestClientWithURL(config.BaseURL, creds)
	} else {
		rest = NewRestClient(creds)
	}

	client := &Client{
		config: config,
		rest:   rest,
		ctx:    ctx,
		cancel: cancel,
	}

	// Create WebSocket clients
	client.wsMarket = NewWSMarketDataClient(config.WSMarketURL)
	if config.APIKey != "" && config.SecretKey != "" {
		client.wsUser = NewWSUserDataClient(config.WSUserURL, config.APIKey, config.SecretKey)
	}

	return client
}

// SetCallbacks sets connection callbacks
func (c *Client) SetCallbacks(onConnect, onDisconnect func(), onError func(error)) {
	c.onConnect = onConnect
	c.onDisconnect = onDisconnect
	c.onError = onError

	// Set callbacks on WebSocket clients
	if c.wsMarket != nil {
		c.wsMarket.SetCallbacks(onConnect, onDisconnect, onError)
	}
	if c.wsUser != nil {
		c.wsUser.SetCallbacks(onConnect, onDisconnect, onError)
	}
}

// Connect establishes all connections
func (c *Client) Connect() error {
	// Connect to market data WebSocket
	if c.wsMarket != nil {
		if err := c.wsMarket.Connect(); err != nil {
			return fmt.Errorf("market websocket connect: %w", err)
		}
		log.Printf("[HTX] Connected to market data WebSocket")
	}

	// Connect to user data WebSocket (if credentials provided)
	if c.wsUser != nil {
		if err := c.wsUser.Connect(); err != nil {
			log.Printf("[HTX] Warning: user data websocket connect failed: %v", err)
			// Don't fail overall connection, user data is optional
		} else {
			log.Printf("[HTX] Connected to user data WebSocket")
		}
	}

	return nil
}

// Disconnect closes all connections
func (c *Client) Disconnect() {
	c.cancel()

	if c.wsMarket != nil {
		c.wsMarket.Disconnect()
	}
	if c.wsUser != nil {
		c.wsUser.Disconnect()
	}

	c.wg.Wait()
}

// REST returns the REST client
func (c *Client) REST() *RestClient {
	return c.rest
}

// WSMarket returns the market data WebSocket client
func (c *Client) WSMarket() *WSMarketDataClient {
	return c.wsMarket
}

// WSUser returns the user data WebSocket client
func (c *Client) WSUser() *WSUserDataClient {
	return c.wsUser
}

// GetConfig returns the client configuration
func (c *Client) GetConfig() *ClientConfig {
	return c.config
}

// ========== Convenience Methods (WebSocket Market Data) ==========

// SubscribeKline subscribes to kline data
func (c *Client) SubscribeKline(symbol, period string, callback func(data []byte)) error {
	if c.wsMarket == nil {
		return fmt.Errorf("market websocket not initialized")
	}
	return c.wsMarket.SubscribeKline(symbol, period, callback)
}

// SubscribeDepth subscribes to depth data
func (c *Client) SubscribeDepth(symbol, depthType string, callback func(data []byte)) error {
	if c.wsMarket == nil {
		return fmt.Errorf("market websocket not initialized")
	}
	return c.wsMarket.SubscribeDepth(symbol, depthType, callback)
}

// SubscribeIncrementalDepth subscribes to incremental depth data
func (c *Client) SubscribeIncrementalDepth(symbol string, size int, callback func(data []byte)) error {
	if c.wsMarket == nil {
		return fmt.Errorf("market websocket not initialized")
	}
	return c.wsMarket.SubscribeIncrementalDepth(symbol, size, callback)
}

// SubscribeBBO subscribes to BBO data
func (c *Client) SubscribeBBO(symbol string, callback func(data []byte)) error {
	if c.wsMarket == nil {
		return fmt.Errorf("market websocket not initialized")
	}
	return c.wsMarket.SubscribeBBO(symbol, callback)
}

// SubscribeTrade subscribes to trade data
func (c *Client) SubscribeTrade(symbol string, callback func(data []byte)) error {
	if c.wsMarket == nil {
		return fmt.Errorf("market websocket not initialized")
	}
	return c.wsMarket.SubscribeTrade(symbol, callback)
}

// SubscribeTicker subscribes to ticker data
func (c *Client) SubscribeTicker(symbol string, callback func(data []byte)) error {
	if c.wsMarket == nil {
		return fmt.Errorf("market websocket not initialized")
	}
	return c.wsMarket.SubscribeTicker(symbol, callback)
}

// GetOrderBook returns the local order book for a symbol
func (c *Client) GetOrderBook(symbol string) (*OrderBook, bool) {
	if c.wsMarket == nil {
		return nil, false
	}
	return c.wsMarket.GetOrderBook(symbol)
}

// ========== Convenience Methods (WebSocket User Data) ==========

// SetOrderCallback sets order update callback
func (c *Client) SetOrderCallback(callback func(order *WSOrderNotify)) {
	if c.wsUser != nil {
		c.wsUser.SetOrderCallback(callback)
	}
}

// SetPositionCallback sets position update callback
func (c *Client) SetPositionCallback(callback func(position *WSPositionNotify)) {
	if c.wsUser != nil {
		c.wsUser.SetPositionCallback(callback)
	}
}

// SetAccountCallback sets account update callback
func (c *Client) SetAccountCallback(callback func(account *WSAccountNotify)) {
	if c.wsUser != nil {
		c.wsUser.SetAccountCallback(callback)
	}
}

// SubscribeOrders subscribes to order updates
func (c *Client) SubscribeOrders(symbol string, callback func(data []byte)) error {
	if c.wsUser == nil {
		return fmt.Errorf("user websocket not initialized")
	}
	return c.wsUser.SubscribeOrders(symbol, callback)
}

// SubscribePositions subscribes to position updates
func (c *Client) SubscribePositions(symbol string, callback func(data []byte)) error {
	if c.wsUser == nil {
		return fmt.Errorf("user websocket not initialized")
	}
	return c.wsUser.SubscribePositions(symbol, callback)
}

// SubscribeAccounts subscribes to account updates
func (c *Client) SubscribeAccounts(marginAccount string, callback func(data []byte)) error {
	if c.wsUser == nil {
		return fmt.Errorf("user websocket not initialized")
	}
	return c.wsUser.SubscribeAccounts(marginAccount, callback)
}

// SubscribeMatchOrders subscribes to match order updates
func (c *Client) SubscribeMatchOrders(symbol string, callback func(data []byte)) error {
	if c.wsUser == nil {
		return fmt.Errorf("user websocket not initialized")
	}
	return c.wsUser.SubscribeMatchOrders(symbol, callback)
}

// SubscribeLiquidationOrders subscribes to liquidation order updates
func (c *Client) SubscribeLiquidationOrders(symbol string, callback func(data []byte)) error {
	if c.wsUser == nil {
		return fmt.Errorf("user websocket not initialized")
	}
	return c.wsUser.SubscribeLiquidationOrders(symbol, callback)
}

// SubscribeTriggerOrders subscribes to trigger order updates
func (c *Client) SubscribeTriggerOrders(symbol string, callback func(data []byte)) error {
	if c.wsUser == nil {
		return fmt.Errorf("user websocket not initialized")
	}
	return c.wsUser.SubscribeTriggerOrders(symbol, callback)
}

// IsUserAuthenticated returns whether the user WebSocket is authenticated
func (c *Client) IsUserAuthenticated() bool {
	if c.wsUser == nil {
		return false
	}
	return c.wsUser.IsAuthenticated()
}

// GetMarketState returns the market WebSocket connection state
func (c *Client) GetMarketState() ConnectionState {
	if c.wsMarket == nil {
		return StateDisconnected
	}
	return c.wsMarket.GetState()
}

// GetUserState returns the user WebSocket connection state
func (c *Client) GetUserState() ConnectionState {
	if c.wsUser == nil {
		return StateDisconnected
	}
	return c.wsUser.GetState()
}
