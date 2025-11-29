// Package mexc provides REST API client for MEXC exchange.
package mexc

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
	"sync"
	"time"
)

// REST API endpoints
const (
	BaseURLProduction = "https://contract.mexc.com"

	// Public endpoints - Market Data
	PathContractDetail = "/api/v1/contract/detail"
	PathTicker         = "/api/v1/contract/ticker"
	PathDepth          = "/api/v1/contract/depth"        // /{symbol}
	PathDeals          = "/api/v1/contract/deals"        // /{symbol}
	PathKline          = "/api/v1/contract/kline"        // /{symbol}
	PathIndexPrice     = "/api/v1/contract/index_price"  // /{symbol}
	PathFairPrice      = "/api/v1/contract/fair_price"   // /{symbol}
	PathFundingRate    = "/api/v1/contract/funding_rate" // /{symbol}

	// Private endpoints - Account
	PathAccountAssets = "/api/v1/private/account/assets"
	PathAccountAsset  = "/api/v1/private/account/asset" // /{currency}

	// Private endpoints - Position
	PathOpenPositions      = "/api/v1/private/position/open_positions"
	PathHistoryPositions   = "/api/v1/private/position/list/history_positions"
	PathChangeLeverage     = "/api/v1/private/position/change_leverage"
	PathChangeMargin       = "/api/v1/private/position/change_margin"
	PathChangePositionMode = "/api/v1/private/position/change_position_mode"

	// Private endpoints - Order
	PathOrderSubmit      = "/api/v1/private/order/submit"
	PathOrderSubmitBatch = "/api/v1/private/order/submit_batch"
	PathOrderCancel      = "/api/v1/private/order/cancel"
	PathOrderCancelAll   = "/api/v1/private/order/cancel_all"
	PathOpenOrders       = "/api/v1/private/order/list/open_orders" // /{symbol}
	PathHistoryOrders    = "/api/v1/private/order/list/history_orders"
	PathOrderGet         = "/api/v1/private/order/get"      // /{orderId}
	PathOrderExternal    = "/api/v1/private/order/external" // /{symbol}/{externalOid}
)

// RESTClient provides methods to interact with MEXC REST API
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

	// Refill tokens
	now := time.Now()
	elapsed := now.Sub(r.lastFill)
	if elapsed >= r.interval {
		r.tokens = r.maxTokens
		r.lastFill = now
	}

	// Wait if no tokens available
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

// NewRESTClient creates a new MEXC REST client
func NewRESTClient(cfg RESTClientConfig) *RESTClient {
	if cfg.BaseURL == "" {
		cfg.BaseURL = BaseURLProduction
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

// sign generates HMAC-SHA256 signature for request
// MEXC signature: HMAC_SHA256(accessKey + timestamp + parameterString, secretKey)
func (c *RESTClient) sign(timestamp int64, params string) string {
	message := c.apiKey + strconv.FormatInt(timestamp, 10) + params
	h := hmac.New(sha256.New, []byte(c.secretKey))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

// getTimestamp returns current timestamp in milliseconds
func (c *RESTClient) getTimestamp() int64 {
	return time.Now().UnixMilli()
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

	// Build URL
	fullURL := c.baseURL + path
	if len(params) > 0 {
		fullURL += "?" + params.Encode()
	}

	// Build request body
	var bodyBytes []byte
	var bodyReader io.Reader
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Add authentication headers for private endpoints
	if authenticated {
		timestamp := c.getTimestamp()

		// Build params string for signature
		var paramStr string
		if body != nil {
			paramStr = string(bodyBytes)
		} else if len(params) > 0 {
			paramStr = params.Encode()
		}

		signature := c.sign(timestamp, paramStr)

		req.Header.Set("ApiKey", c.apiKey)
		req.Header.Set("Request-Time", strconv.FormatInt(timestamp, 10))
		req.Header.Set("Signature", signature)
		req.Header.Set("Recv-Window", "5000")
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

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// =============================================================================
// Public Market Data APIs
// =============================================================================

// GetContracts fetches all available contracts
func (c *RESTClient) GetContracts(ctx context.Context) ([]Contract, error) {
	body, err := c.doRequest(ctx, http.MethodGet, PathContractDetail, nil, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Contract]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return resp.Data, nil
}

// GetTickers fetches all ticker data
func (c *RESTClient) GetTickers(ctx context.Context) ([]Ticker, error) {
	body, err := c.doRequest(ctx, http.MethodGet, PathTicker, nil, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Ticker]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return resp.Data, nil
}

// GetDepth fetches orderbook depth for a symbol
func (c *RESTClient) GetDepth(ctx context.Context, symbol string, limit int) (*OrderBook, error) {
	if limit == 0 {
		limit = 20
	}

	path := fmt.Sprintf("%s/%s", PathDepth, symbol)
	params := url.Values{}
	params.Set("limit", strconv.Itoa(limit))

	body, err := c.doRequest(ctx, http.MethodGet, path, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[OrderBook]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return &resp.Data, nil
}

// GetDeals fetches recent trades for a symbol
func (c *RESTClient) GetDeals(ctx context.Context, symbol string, limit int) ([]Trade, error) {
	if limit == 0 {
		limit = 20
	}

	path := fmt.Sprintf("%s/%s", PathDeals, symbol)
	params := url.Values{}
	params.Set("limit", strconv.Itoa(limit))

	body, err := c.doRequest(ctx, http.MethodGet, path, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Trade]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return resp.Data, nil
}

// GetKline fetches candlestick data for a symbol
func (c *RESTClient) GetKline(ctx context.Context, symbol string, interval KlineInterval, start, end int64) (*Kline, error) {
	path := fmt.Sprintf("%s/%s", PathKline, symbol)
	params := url.Values{}

	if interval != "" {
		params.Set("interval", string(interval))
	}
	if start > 0 {
		params.Set("start", strconv.FormatInt(start, 10))
	}
	if end > 0 {
		params.Set("end", strconv.FormatInt(end, 10))
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[Kline]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return &resp.Data, nil
}

// GetIndexPrice fetches index price for a symbol
func (c *RESTClient) GetIndexPrice(ctx context.Context, symbol string) (*IndexPrice, error) {
	path := fmt.Sprintf("%s/%s", PathIndexPrice, symbol)

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[IndexPrice]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return &resp.Data, nil
}

// GetFairPrice fetches fair/mark price for a symbol
func (c *RESTClient) GetFairPrice(ctx context.Context, symbol string) (*FairPrice, error) {
	path := fmt.Sprintf("%s/%s", PathFairPrice, symbol)

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[FairPrice]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return &resp.Data, nil
}

// GetFundingRate fetches funding rate for a symbol
func (c *RESTClient) GetFundingRate(ctx context.Context, symbol string) (*FundingRate, error) {
	path := fmt.Sprintf("%s/%s", PathFundingRate, symbol)

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[FundingRate]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return &resp.Data, nil
}

// GetAllFundingRates fetches funding rates for all symbols
func (c *RESTClient) GetAllFundingRates(ctx context.Context) ([]FundingRate, error) {
	body, err := c.doRequest(ctx, http.MethodGet, PathFundingRate, nil, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]FundingRate]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return resp.Data, nil
}

// =============================================================================
// Private Account APIs
// =============================================================================

// GetAccountAssets fetches all account balances
func (c *RESTClient) GetAccountAssets(ctx context.Context) ([]AccountAsset, error) {
	body, err := c.doRequest(ctx, http.MethodGet, PathAccountAssets, nil, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]AccountAsset]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return resp.Data, nil
}

// GetAccountAsset fetches balance for a specific currency
func (c *RESTClient) GetAccountAsset(ctx context.Context, currency string) (*AccountAsset, error) {
	path := fmt.Sprintf("%s/%s", PathAccountAsset, currency)

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[AccountAsset]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return &resp.Data, nil
}

// =============================================================================
// Private Position APIs
// =============================================================================

// GetOpenPositions fetches all open positions
func (c *RESTClient) GetOpenPositions(ctx context.Context, symbol string) ([]Position, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathOpenPositions, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Position]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return resp.Data, nil
}

// GetHistoryPositions fetches position history
func (c *RESTClient) GetHistoryPositions(ctx context.Context, symbol string, pageNum, pageSize int) ([]Position, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}
	if pageNum > 0 {
		params.Set("page_num", strconv.Itoa(pageNum))
	}
	if pageSize > 0 {
		params.Set("page_size", strconv.Itoa(pageSize))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathHistoryPositions, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Position]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return resp.Data, nil
}

// ChangeLeverage changes leverage for a symbol
func (c *RESTClient) ChangeLeverage(ctx context.Context, req *LeverageRequest) error {
	body, err := c.doRequest(ctx, http.MethodPost, PathChangeLeverage, nil, req, true, 5)
	if err != nil {
		return err
	}

	var resp APIResponse[interface{}]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return &APIError{Code: resp.Code, Message: resp.Message}
	}

	return nil
}

// =============================================================================
// Private Order APIs
// =============================================================================

// PlaceOrder submits a new order
func (c *RESTClient) PlaceOrder(ctx context.Context, req *OrderRequest) (*OrderResponse, error) {
	body, err := c.doRequest(ctx, http.MethodPost, PathOrderSubmit, nil, req, true, 5)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[OrderResponse]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return &resp.Data, nil
}

// PlaceBatchOrders submits multiple orders
func (c *RESTClient) PlaceBatchOrders(ctx context.Context, orders []OrderRequest) ([]OrderResponse, error) {
	body, err := c.doRequest(ctx, http.MethodPost, PathOrderSubmitBatch, nil, orders, true, 5)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]OrderResponse]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return resp.Data, nil
}

// CancelOrder cancels an order
func (c *RESTClient) CancelOrder(ctx context.Context, req *CancelOrderRequest) error {
	body, err := c.doRequest(ctx, http.MethodPost, PathOrderCancel, nil, req, true, 5)
	if err != nil {
		return err
	}

	var resp APIResponse[interface{}]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return &APIError{Code: resp.Code, Message: resp.Message}
	}

	return nil
}

// CancelAllOrders cancels all orders for a symbol
func (c *RESTClient) CancelAllOrders(ctx context.Context, symbol string, positionID int64) error {
	reqBody := map[string]interface{}{}
	if symbol != "" {
		reqBody["symbol"] = symbol
	}
	if positionID > 0 {
		reqBody["positionId"] = positionID
	}

	body, err := c.doRequest(ctx, http.MethodPost, PathOrderCancelAll, nil, reqBody, true, 5)
	if err != nil {
		return err
	}

	var resp APIResponse[interface{}]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return &APIError{Code: resp.Code, Message: resp.Message}
	}

	return nil
}

// GetOpenOrders fetches open orders for a symbol
func (c *RESTClient) GetOpenOrders(ctx context.Context, symbol string, pageNum, pageSize int) ([]Order, error) {
	path := fmt.Sprintf("%s/%s", PathOpenOrders, symbol)
	params := url.Values{}
	if pageNum > 0 {
		params.Set("page_num", strconv.Itoa(pageNum))
	}
	if pageSize > 0 {
		params.Set("page_size", strconv.Itoa(pageSize))
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Order]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return resp.Data, nil
}

// GetHistoryOrders fetches order history
func (c *RESTClient) GetHistoryOrders(ctx context.Context, symbol string, states string, startTime, endTime int64, pageNum, pageSize int) ([]Order, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}
	if states != "" {
		params.Set("states", states)
	}
	if startTime > 0 {
		params.Set("start_time", strconv.FormatInt(startTime, 10))
	}
	if endTime > 0 {
		params.Set("end_time", strconv.FormatInt(endTime, 10))
	}
	if pageNum > 0 {
		params.Set("page_num", strconv.Itoa(pageNum))
	}
	if pageSize > 0 {
		params.Set("page_size", strconv.Itoa(pageSize))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathHistoryOrders, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Order]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return resp.Data, nil
}

// GetOrder fetches order by ID
func (c *RESTClient) GetOrder(ctx context.Context, orderID int64) (*Order, error) {
	path := fmt.Sprintf("%s/%d", PathOrderGet, orderID)

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[Order]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return &resp.Data, nil
}

// GetOrderByExternalID fetches order by external (client) order ID
func (c *RESTClient) GetOrderByExternalID(ctx context.Context, symbol, externalOID string) (*Order, error) {
	path := fmt.Sprintf("%s/%s/%s", PathOrderExternal, symbol, externalOID)

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[Order]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !resp.Success {
		return nil, &APIError{Code: resp.Code, Message: resp.Message}
	}

	return &resp.Data, nil
}
