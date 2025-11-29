package lbank

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Client provides a unified interface to LBank APIs
type Client struct {
	config       *ClientConfig
	restClient   *RestClient
	marketDataWs *WsMarketDataClient
	userDataWs   *WsUserDataClient

	mu sync.RWMutex
}

// NewClient creates a new LBank client
func NewClient(config *ClientConfig) *Client {
	if config == nil {
		config = DefaultClientConfig()
	}

	restClient := NewRestClient(config)

	return &Client{
		config:     config,
		restClient: restClient,
	}
}

// SetCredentials sets API credentials
func (c *Client) SetCredentials(apiKey, secretKey, signatureMethod string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config.Credentials = &Credentials{
		APIKey:          apiKey,
		SecretKey:       secretKey,
		SignatureMethod: signatureMethod,
	}

	// Update rest client credentials
	c.restClient.credentials = c.config.Credentials
}

// GetRestClient returns the REST client
func (c *Client) GetRestClient() *RestClient {
	return c.restClient
}

// ConnectMarketData establishes WebSocket connection for market data
func (c *Client) ConnectMarketData(ctx context.Context, handler *MarketDataHandler) error {
	c.mu.Lock()
	if c.marketDataWs != nil {
		c.mu.Unlock()
		return fmt.Errorf("market data WebSocket already connected")
	}

	c.marketDataWs = NewWsMarketDataClient(c.config, handler)
	c.mu.Unlock()

	return c.marketDataWs.Connect(ctx)
}

// DisconnectMarketData closes the market data WebSocket connection
func (c *Client) DisconnectMarketData() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.marketDataWs == nil {
		return nil
	}

	err := c.marketDataWs.Disconnect()
	c.marketDataWs = nil
	return err
}

// ConnectUserData establishes WebSocket connection for user data
func (c *Client) ConnectUserData(ctx context.Context, handler *UserDataHandler) error {
	if c.config.Credentials == nil {
		return fmt.Errorf("credentials required for user data WebSocket")
	}

	c.mu.Lock()
	if c.userDataWs != nil {
		c.mu.Unlock()
		return fmt.Errorf("user data WebSocket already connected")
	}

	c.userDataWs = NewWsUserDataClient(c.restClient, c.config, handler)
	c.mu.Unlock()

	return c.userDataWs.Connect(ctx)
}

// DisconnectUserData closes the user data WebSocket connection
func (c *Client) DisconnectUserData() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.userDataWs == nil {
		return nil
	}

	err := c.userDataWs.Disconnect()
	c.userDataWs = nil
	return err
}

// Close closes all connections
func (c *Client) Close() error {
	var errs []error

	if err := c.DisconnectMarketData(); err != nil {
		errs = append(errs, fmt.Errorf("market data disconnect: %w", err))
	}

	if err := c.DisconnectUserData(); err != nil {
		errs = append(errs, fmt.Errorf("user data disconnect: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// ==================== Market Data Subscription Methods ====================

// SubscribeDepth subscribes to orderbook depth updates
func (c *Client) SubscribeDepth(symbols []string, depth int) error {
	c.mu.RLock()
	ws := c.marketDataWs
	c.mu.RUnlock()

	if ws == nil {
		return fmt.Errorf("market data WebSocket not connected")
	}

	return ws.SubscribeDepth(symbols, depth)
}

// SubscribeTrades subscribes to trade updates
func (c *Client) SubscribeTrades(symbols []string) error {
	c.mu.RLock()
	ws := c.marketDataWs
	c.mu.RUnlock()

	if ws == nil {
		return fmt.Errorf("market data WebSocket not connected")
	}

	return ws.SubscribeTrades(symbols)
}

// SubscribeTicker subscribes to ticker updates
func (c *Client) SubscribeTicker(symbols []string) error {
	c.mu.RLock()
	ws := c.marketDataWs
	c.mu.RUnlock()

	if ws == nil {
		return fmt.Errorf("market data WebSocket not connected")
	}

	return ws.SubscribeTicker(symbols)
}

// SubscribeKline subscribes to kline updates
func (c *Client) SubscribeKline(symbols []string, interval string) error {
	c.mu.RLock()
	ws := c.marketDataWs
	c.mu.RUnlock()

	if ws == nil {
		return fmt.Errorf("market data WebSocket not connected")
	}

	return ws.SubscribeKline(symbols, interval)
}

// UnsubscribeDepth unsubscribes from orderbook depth
func (c *Client) UnsubscribeDepth(symbols []string, depth int) error {
	c.mu.RLock()
	ws := c.marketDataWs
	c.mu.RUnlock()

	if ws == nil {
		return nil
	}

	return ws.UnsubscribeDepth(symbols, depth)
}

// ==================== User Data Subscription Methods ====================

// SubscribeOrderUpdates subscribes to order updates
func (c *Client) SubscribeOrderUpdates(pairs []string) error {
	c.mu.RLock()
	ws := c.userDataWs
	c.mu.RUnlock()

	if ws == nil {
		return fmt.Errorf("user data WebSocket not connected")
	}

	return ws.SubscribeOrderUpdates(pairs)
}

// SubscribeAssetUpdates subscribes to asset/balance updates
func (c *Client) SubscribeAssetUpdates() error {
	c.mu.RLock()
	ws := c.userDataWs
	c.mu.RUnlock()

	if ws == nil {
		return fmt.Errorf("user data WebSocket not connected")
	}

	return ws.SubscribeAssetUpdates()
}

// ==================== REST API Convenience Methods ====================

// GetContractInstruments fetches all perpetual contracts
func (c *Client) GetContractInstruments(ctx context.Context) ([]ContractInstrument, error) {
	return c.restClient.GetContractInstruments(ctx)
}

// GetContractMarketData fetches market data for all contracts
func (c *Client) GetContractMarketData(ctx context.Context) ([]ContractMarketData, error) {
	return c.restClient.GetContractMarketData(ctx)
}

// GetContractOrderbook fetches orderbook for a symbol
func (c *Client) GetContractOrderbook(ctx context.Context, symbol string, depth int) (*ContractOrderbook, error) {
	return c.restClient.GetContractOrderbook(ctx, symbol, depth)
}

// GetContractAccount fetches account info
func (c *Client) GetContractAccount(ctx context.Context, asset string) (*ContractAccount, error) {
	return c.restClient.GetContractAccount(ctx, asset)
}

// GetContractPositions fetches all positions
func (c *Client) GetContractPositions(ctx context.Context) ([]ContractPosition, error) {
	return c.restClient.GetContractPositions(ctx)
}

// PlaceContractOrder places a contract order
func (c *Client) PlaceContractOrder(ctx context.Context, symbol, side, orderType string, price, volume float64) (*ContractOrder, error) {
	return c.restClient.PlaceContractOrder(ctx, symbol, side, orderType, price, volume)
}

// CancelContractOrder cancels a contract order
func (c *Client) CancelContractOrder(ctx context.Context, symbol, orderID string) error {
	return c.restClient.CancelContractOrder(ctx, symbol, orderID)
}

// GetSpotTickers fetches all spot tickers
func (c *Client) GetSpotTickers(ctx context.Context) ([]SpotTicker, error) {
	return c.restClient.GetSpotTickers(ctx)
}

// GetSpotOrderbook fetches spot orderbook
func (c *Client) GetSpotOrderbook(ctx context.Context, symbol string, size int) (*SpotOrderbook, error) {
	return c.restClient.GetSpotOrderbook(ctx, symbol, size)
}

// GetSpotAssetConfigs fetches deposit/withdrawal configs
func (c *Client) GetSpotAssetConfigs(ctx context.Context) ([]SpotAssetConfig, error) {
	return c.restClient.GetSpotAssetConfigs(ctx)
}

// GetSpotUserInfo fetches user account info
func (c *Client) GetSpotUserInfo(ctx context.Context) (*SpotUserInfo, error) {
	return c.restClient.GetSpotUserInfo(ctx)
}

// PlaceSpotOrder places a spot order
func (c *Client) PlaceSpotOrder(ctx context.Context, symbol, orderType string, price, amount float64) (string, error) {
	return c.restClient.PlaceSpotOrder(ctx, symbol, orderType, price, amount)
}

// CancelSpotOrder cancels a spot order
func (c *Client) CancelSpotOrder(ctx context.Context, symbol, orderID string) error {
	return c.restClient.CancelSpotOrder(ctx, symbol, orderID)
}

// ==================== Connection Status Methods ====================

// IsMarketDataConnected returns market data WebSocket connection status
func (c *Client) IsMarketDataConnected() bool {
	c.mu.RLock()
	ws := c.marketDataWs
	c.mu.RUnlock()

	if ws == nil {
		return false
	}
	return ws.IsConnected()
}

// IsUserDataConnected returns user data WebSocket connection status
func (c *Client) IsUserDataConnected() bool {
	c.mu.RLock()
	ws := c.userDataWs
	c.mu.RUnlock()

	if ws == nil {
		return false
	}
	return ws.IsConnected()
}

// ReconnectMarketData reconnects the market data WebSocket
func (c *Client) ReconnectMarketData(ctx context.Context) error {
	c.mu.RLock()
	ws := c.marketDataWs
	c.mu.RUnlock()

	if ws == nil {
		return fmt.Errorf("market data WebSocket not initialized")
	}

	return ws.Reconnect(ctx)
}

// ReconnectUserData reconnects the user data WebSocket
func (c *Client) ReconnectUserData(ctx context.Context) error {
	c.mu.RLock()
	ws := c.userDataWs
	c.mu.RUnlock()

	if ws == nil {
		return fmt.Errorf("user data WebSocket not initialized")
	}

	return ws.Reconnect(ctx)
}

// StartAutoReconnect starts automatic reconnection for WebSocket connections
func (c *Client) StartAutoReconnect(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Check and reconnect market data
				c.mu.RLock()
				marketWs := c.marketDataWs
				userWs := c.userDataWs
				c.mu.RUnlock()

				if marketWs != nil && !marketWs.IsConnected() {
					log.Warn().Msg("Market data WebSocket disconnected, reconnecting...")
					if err := marketWs.Reconnect(ctx); err != nil {
						log.Error().Err(err).Msg("Failed to reconnect market data WebSocket")
					}
				}

				if userWs != nil && !userWs.IsConnected() {
					log.Warn().Msg("User data WebSocket disconnected, reconnecting...")
					if err := userWs.Reconnect(ctx); err != nil {
						log.Error().Err(err).Msg("Failed to reconnect user data WebSocket")
					}
				}
			}
		}
	}()
}
