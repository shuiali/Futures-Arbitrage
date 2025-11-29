// Package coinex provides REST API client for CoinEx Perpetual Futures exchange.
package coinex

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// REST API endpoints
const (
	// Public endpoints - Market Data
	PathMarkets         = "/futures/market"
	PathTicker          = "/futures/ticker"
	PathDepth           = "/futures/depth"
	PathDeals           = "/futures/deals"
	PathKline           = "/futures/kline"
	PathFundingRate     = "/futures/funding-rate"
	PathFundingRateHist = "/futures/funding-rate-history"
	PathIndex           = "/futures/index"

	// Private endpoints - Account
	PathFuturesBalance  = "/assets/futures/balance"
	PathSpotBalance     = "/assets/spot/balance"
	PathDepositHistory  = "/assets/deposit-history"
	PathWithdrawHistory = "/assets/withdraw"

	// Private endpoints - Trading
	PathPlaceOrder       = "/futures/order"
	PathCancelOrder      = "/futures/cancel-order"
	PathCancelByClientID = "/futures/cancel-order-by-client-id"
	PathCancelAllOrders  = "/futures/cancel-all-order"
	PathClosePosition    = "/futures/close-position"
	PathPendingOrders    = "/futures/pending-order"
	PathFinishedOrders   = "/futures/finished-order"

	// Private endpoints - Position
	PathPendingPositions = "/futures/pending-position"
	PathAdjustLeverage   = "/futures/adjust-position-leverage"
)

// RESTClient provides methods to interact with CoinEx REST API
type RESTClient struct {
	baseURL    string
	apiKey     string
	secretKey  string
	httpClient *http.Client

	// Rate limiting
	rateLimiter sync.Map // path -> *rateLimiter
}

// rateLimiter implements a simple token bucket rate limiter
type rateLimiter struct {
	tokens    int
	maxTokens int
	interval  time.Duration
	lastFill  time.Time
	mu        sync.Mutex
}

func newRateLimiter(maxTokens int, interval time.Duration) *rateLimiter {
	return &rateLimiter{
		tokens:    maxTokens,
		maxTokens: maxTokens,
		interval:  interval,
		lastFill:  time.Now(),
	}
}

func (r *rateLimiter) wait(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastFill)
	if elapsed >= r.interval {
		r.tokens = r.maxTokens
		r.lastFill = now
	}

	if r.tokens <= 0 {
		waitTime := r.interval - elapsed
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
		}
		r.mu.Lock()
		r.tokens = r.maxTokens
		r.lastFill = time.Now()
	}

	r.tokens--
	return nil
}

// RESTClientConfig holds configuration for REST client
type RESTClientConfig struct {
	BaseURL   string
	APIKey    string
	SecretKey string
	Timeout   time.Duration
}

// NewRESTClient creates a new CoinEx REST client
func NewRESTClient(cfg RESTClientConfig) *RESTClient {
	if cfg.BaseURL == "" {
		cfg.BaseURL = RESTBaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	return &RESTClient{
		baseURL:   cfg.BaseURL,
		apiKey:    cfg.APIKey,
		secretKey: cfg.SecretKey,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// sign generates HMAC-SHA256 signature for CoinEx API
// Signature format: METHOD + request_path + body(optional) + timestamp
func (c *RESTClient) sign(method, path string, body []byte, timestamp string) string {
	// Build the prepared string
	var sb strings.Builder
	sb.WriteString(method)
	sb.WriteString(path)
	if len(body) > 0 {
		sb.Write(body)
	}
	sb.WriteString(timestamp)
	preparedStr := sb.String()

	// Generate HMAC-SHA256 signature
	h := hmac.New(sha256.New, []byte(c.secretKey))
	h.Write([]byte(preparedStr))
	return strings.ToLower(hex.EncodeToString(h.Sum(nil)))
}

// getTimestamp returns current timestamp in milliseconds as string
func (c *RESTClient) getTimestamp() string {
	return strconv.FormatInt(time.Now().UnixMilli(), 10)
}

// getRateLimiter gets or creates a rate limiter for a path
func (c *RESTClient) getRateLimiter(path string, maxRequests int) *rateLimiter {
	if v, ok := c.rateLimiter.Load(path); ok {
		return v.(*rateLimiter)
	}
	rl := newRateLimiter(maxRequests, time.Second)
	actual, _ := c.rateLimiter.LoadOrStore(path, rl)
	return actual.(*rateLimiter)
}

// doRequest performs HTTP request with optional authentication
func (c *RESTClient) doRequest(ctx context.Context, method, path string, params url.Values, body interface{}, authenticated bool, rateLimit int) ([]byte, error) {
	// Apply rate limiting
	rl := c.getRateLimiter(path, rateLimit)
	if err := rl.wait(ctx); err != nil {
		return nil, err
	}

	// Build full URL
	fullURL := c.baseURL + path
	if len(params) > 0 {
		fullURL += "?" + params.Encode()
	}

	// Prepare request body
	var bodyBytes []byte
	var err error
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
	}

	// Create request
	var reqBody io.Reader
	if len(bodyBytes) > 0 {
		reqBody = bytes.NewReader(bodyBytes)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CrossSpread/1.0")

	// Add authentication headers if required
	if authenticated {
		timestamp := c.getTimestamp()

		// Build path with query string for signature
		signPath := path
		if len(params) > 0 {
			signPath += "?" + params.Encode()
		}

		signature := c.sign(method, signPath, bodyBytes, timestamp)

		req.Header.Set("X-COINEX-KEY", c.apiKey)
		req.Header.Set("X-COINEX-SIGN", signature)
		req.Header.Set("X-COINEX-TIMESTAMP", timestamp)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// parseResponse parses API response and checks for errors
func (c *RESTClient) parseResponse(data []byte, result interface{}) error {
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !resp.IsSuccess() {
		return resp.Error()
	}

	if result != nil && len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, result); err != nil {
			return fmt.Errorf("failed to parse data: %w", err)
		}
	}

	return nil
}

// =============================================================================
// Public Market Data API
// =============================================================================

// GetMarkets fetches all available futures markets
func (c *RESTClient) GetMarkets(ctx context.Context, markets ...string) ([]Market, error) {
	params := url.Values{}
	if len(markets) > 0 {
		params.Set("market", strings.Join(markets, ","))
	}

	data, err := c.doRequest(ctx, "GET", PathMarkets, params, nil, false, 50)
	if err != nil {
		return nil, err
	}

	var result []Market
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetTickers fetches 24h ticker data
// If no markets specified, fetches all tickers
func (c *RESTClient) GetTickers(ctx context.Context, markets ...string) ([]Ticker, error) {
	params := url.Values{}
	// Don't pass market parameter if too many markets - fetch all instead
	// CoinEx returns all tickers by default when no market param is provided
	if len(markets) > 0 && len(markets) <= 50 {
		params.Set("market", strings.Join(markets, ","))
	}

	data, err := c.doRequest(ctx, "GET", PathTicker, params, nil, false, 50)
	if err != nil {
		return nil, err
	}

	var result []Ticker
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetDepth fetches orderbook depth
func (c *RESTClient) GetDepth(ctx context.Context, market string, limit int, interval string) (*Depth, error) {
	params := url.Values{
		"market":   {market},
		"limit":    {strconv.Itoa(limit)},
		"interval": {interval},
	}

	data, err := c.doRequest(ctx, "GET", PathDepth, params, nil, false, 50)
	if err != nil {
		return nil, err
	}

	var result Depth
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetDeals fetches recent trades
func (c *RESTClient) GetDeals(ctx context.Context, market string, limit int, lastID int64) ([]Deal, error) {
	params := url.Values{
		"market": {market},
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if lastID > 0 {
		params.Set("last_id", strconv.FormatInt(lastID, 10))
	}

	data, err := c.doRequest(ctx, "GET", PathDeals, params, nil, false, 50)
	if err != nil {
		return nil, err
	}

	var result []Deal
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetKlines fetches candlestick data
func (c *RESTClient) GetKlines(ctx context.Context, market, period string, limit int) ([]Kline, error) {
	params := url.Values{
		"market": {market},
		"period": {period},
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, "GET", PathKline, params, nil, false, 50)
	if err != nil {
		return nil, err
	}

	var result []Kline
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetFundingRates fetches current funding rates
// If too many markets are passed, fetch all without filter to avoid URL length issues
func (c *RESTClient) GetFundingRates(ctx context.Context, markets ...string) ([]FundingRate, error) {
	params := url.Values{}
	// Don't pass market parameter if too many markets - fetch all instead
	// CoinEx CloudFront blocks requests with very long URLs
	if len(markets) > 0 && len(markets) <= 50 {
		params.Set("market", strings.Join(markets, ","))
	}

	data, err := c.doRequest(ctx, "GET", PathFundingRate, params, nil, false, 50)
	if err != nil {
		return nil, err
	}

	var result []FundingRate
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetFundingRateHistory fetches historical funding rates
func (c *RESTClient) GetFundingRateHistory(ctx context.Context, market string, startTime, endTime int64, page, limit int) ([]FundingRateHistory, error) {
	params := url.Values{
		"market": {market},
	}
	if startTime > 0 {
		params.Set("start_time", strconv.FormatInt(startTime, 10))
	}
	if endTime > 0 {
		params.Set("end_time", strconv.FormatInt(endTime, 10))
	}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, "GET", PathFundingRateHist, params, nil, false, 10)
	if err != nil {
		return nil, err
	}

	var result []FundingRateHistory
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetIndex fetches market index data
func (c *RESTClient) GetIndex(ctx context.Context, markets ...string) ([]Index, error) {
	params := url.Values{}
	if len(markets) > 0 {
		params.Set("market", strings.Join(markets, ","))
	}

	data, err := c.doRequest(ctx, "GET", PathIndex, params, nil, false, 50)
	if err != nil {
		return nil, err
	}

	var result []Index
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// =============================================================================
// Private Account API
// =============================================================================

// GetFuturesBalance fetches futures account balance
func (c *RESTClient) GetFuturesBalance(ctx context.Context) ([]FuturesBalance, error) {
	data, err := c.doRequest(ctx, "GET", PathFuturesBalance, nil, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var result []FuturesBalance
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetSpotBalance fetches spot account balance
func (c *RESTClient) GetSpotBalance(ctx context.Context) ([]SpotBalance, error) {
	data, err := c.doRequest(ctx, "GET", PathSpotBalance, nil, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var result []SpotBalance
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetDepositHistory fetches deposit records
func (c *RESTClient) GetDepositHistory(ctx context.Context, ccy, status string, page, limit int) ([]DepositRecord, error) {
	params := url.Values{}
	if ccy != "" {
		params.Set("ccy", ccy)
	}
	if status != "" {
		params.Set("status", status)
	}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, "GET", PathDepositHistory, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var result []DepositRecord
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetWithdrawHistory fetches withdrawal records
func (c *RESTClient) GetWithdrawHistory(ctx context.Context, ccy, status string, page, limit int) ([]WithdrawRecord, error) {
	params := url.Values{}
	if ccy != "" {
		params.Set("ccy", ccy)
	}
	if status != "" {
		params.Set("status", status)
	}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, "GET", PathWithdrawHistory, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var result []WithdrawRecord
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// =============================================================================
// Private Trading API
// =============================================================================

// PlaceOrder places a new order
func (c *RESTClient) PlaceOrder(ctx context.Context, req *OrderRequest) (*Order, error) {
	data, err := c.doRequest(ctx, "POST", PathPlaceOrder, nil, req, true, 20)
	if err != nil {
		return nil, err
	}

	var result Order
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// PlaceLimitOrder is a convenience method to place a limit order
func (c *RESTClient) PlaceLimitOrder(ctx context.Context, market, side string, amount, price float64, clientID string) (*Order, error) {
	req := &OrderRequest{
		Market:     market,
		MarketType: MarketTypeFutures,
		Side:       side,
		Type:       OrderTypeLimit,
		Amount:     Float64ToString(amount),
		Price:      Float64ToString(price),
		ClientID:   clientID,
	}
	return c.PlaceOrder(ctx, req)
}

// PlaceMarketOrder is a convenience method to place a market order
func (c *RESTClient) PlaceMarketOrder(ctx context.Context, market, side string, amount float64, clientID string) (*Order, error) {
	req := &OrderRequest{
		Market:     market,
		MarketType: MarketTypeFutures,
		Side:       side,
		Type:       OrderTypeMarket,
		Amount:     Float64ToString(amount),
		ClientID:   clientID,
	}
	return c.PlaceOrder(ctx, req)
}

// CancelOrder cancels an order by order ID
func (c *RESTClient) CancelOrder(ctx context.Context, market string, orderID int64) (*Order, error) {
	req := &CancelOrderRequest{
		Market:     market,
		MarketType: MarketTypeFutures,
		OrderID:    orderID,
	}

	data, err := c.doRequest(ctx, "POST", PathCancelOrder, nil, req, true, 40)
	if err != nil {
		return nil, err
	}

	var result Order
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// CancelOrderByClientID cancels orders by client ID
func (c *RESTClient) CancelOrderByClientID(ctx context.Context, market, clientID string) ([]Order, error) {
	req := &CancelByClientIDRequest{
		Market:     market,
		MarketType: MarketTypeFutures,
		ClientID:   clientID,
	}

	data, err := c.doRequest(ctx, "POST", PathCancelByClientID, nil, req, true, 20)
	if err != nil {
		return nil, err
	}

	var result []Order
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// CancelAllOrders cancels all orders for a market
func (c *RESTClient) CancelAllOrders(ctx context.Context, market string, side string) error {
	req := &CancelAllOrdersRequest{
		Market:     market,
		MarketType: MarketTypeFutures,
		Side:       side,
	}

	data, err := c.doRequest(ctx, "POST", PathCancelAllOrders, nil, req, true, 20)
	if err != nil {
		return err
	}

	return c.parseResponse(data, nil)
}

// ClosePosition closes a position
func (c *RESTClient) ClosePosition(ctx context.Context, req *ClosePositionRequest) (*Order, error) {
	data, err := c.doRequest(ctx, "POST", PathClosePosition, nil, req, true, 20)
	if err != nil {
		return nil, err
	}

	var result Order
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ClosePositionMarket closes a position at market price
func (c *RESTClient) ClosePositionMarket(ctx context.Context, market string, amount float64) (*Order, error) {
	req := &ClosePositionRequest{
		Market:     market,
		MarketType: MarketTypeFutures,
		Type:       OrderTypeMarket,
	}
	if amount > 0 {
		req.Amount = Float64ToString(amount)
	}
	return c.ClosePosition(ctx, req)
}

// GetPendingOrders fetches unfilled orders
func (c *RESTClient) GetPendingOrders(ctx context.Context, market, side, clientID string, page, limit int) ([]Order, error) {
	params := url.Values{
		"market_type": {MarketTypeFutures},
	}
	if market != "" {
		params.Set("market", market)
	}
	if side != "" {
		params.Set("side", side)
	}
	if clientID != "" {
		params.Set("client_id", clientID)
	}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, "GET", PathPendingOrders, params, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var result []Order
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetFinishedOrders fetches filled orders
func (c *RESTClient) GetFinishedOrders(ctx context.Context, market, side string, page, limit int) ([]Order, error) {
	params := url.Values{
		"market_type": {MarketTypeFutures},
	}
	if market != "" {
		params.Set("market", market)
	}
	if side != "" {
		params.Set("side", side)
	}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, "GET", PathFinishedOrders, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var result []Order
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// =============================================================================
// Private Position API
// =============================================================================

// GetPositions fetches current positions
func (c *RESTClient) GetPositions(ctx context.Context, market string, page, limit int) ([]Position, error) {
	params := url.Values{
		"market_type": {MarketTypeFutures},
	}
	if market != "" {
		params.Set("market", market)
	}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, "GET", PathPendingPositions, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var result []Position
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// AdjustLeverage adjusts position leverage
func (c *RESTClient) AdjustLeverage(ctx context.Context, market, marginMode string, leverage int) (*AdjustLeverageResponse, error) {
	req := &AdjustLeverageRequest{
		Market:     market,
		MarketType: MarketTypeFutures,
		MarginMode: marginMode,
		Leverage:   leverage,
	}

	data, err := c.doRequest(ctx, "POST", PathAdjustLeverage, nil, req, true, 20)
	if err != nil {
		return nil, err
	}

	var result AdjustLeverageResponse
	if err := c.parseResponse(data, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
