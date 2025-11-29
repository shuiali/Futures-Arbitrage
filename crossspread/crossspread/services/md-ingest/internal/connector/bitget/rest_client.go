// Package bitget provides REST API client for Bitget exchange.
package bitget

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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
	BaseURLProduction = "https://api.bitget.com"

	// Public endpoints - Market Data
	PathContracts          = "/api/v2/mix/market/contracts"
	PathTickers            = "/api/v2/mix/market/tickers"
	PathTicker             = "/api/v2/mix/market/ticker"
	PathMergeDepth         = "/api/v2/mix/market/merge-depth"
	PathCandles            = "/api/v2/mix/market/candles"
	PathHistoryCandles     = "/api/v2/mix/market/history-candles"
	PathFundingRate        = "/api/v2/mix/market/current-fund-rate"
	PathHistoryFundingRate = "/api/v2/mix/market/history-fund-rate"
	PathTrades             = "/api/v2/mix/market/fills"

	// Private endpoints - Account
	PathAccount         = "/api/v2/mix/account/account"
	PathAccounts        = "/api/v2/mix/account/accounts"
	PathSetLeverage     = "/api/v2/mix/account/set-leverage"
	PathSetMarginMode   = "/api/v2/mix/account/set-margin-mode"
	PathSetPositionMode = "/api/v2/mix/account/set-position-mode"

	// Private endpoints - Position
	PathPositions       = "/api/v2/mix/position/all-position"
	PathSinglePosition  = "/api/v2/mix/position/single-position"
	PathHistoryPosition = "/api/v2/mix/position/history-position"

	// Private endpoints - Trade
	PathPlaceOrder       = "/api/v2/mix/order/place-order"
	PathBatchPlaceOrder  = "/api/v2/mix/order/batch-place-order"
	PathCancelOrder      = "/api/v2/mix/order/cancel-order"
	PathBatchCancelOrder = "/api/v2/mix/order/batch-cancel-orders"
	PathModifyOrder      = "/api/v2/mix/order/modify-order"
	PathPendingOrders    = "/api/v2/mix/order/orders-pending"
	PathHistoryOrders    = "/api/v2/mix/order/orders-history"
	PathOrderDetail      = "/api/v2/mix/order/detail"
	PathFills            = "/api/v2/mix/order/fill-history"
)

// RESTClient provides methods to interact with Bitget REST API
type RESTClient struct {
	baseURL    string
	apiKey     string
	secretKey  string
	passphrase string
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
	BaseURL    string
	APIKey     string
	SecretKey  string
	Passphrase string
	Timeout    time.Duration
}

// NewRESTClient creates a new Bitget REST client
func NewRESTClient(cfg RESTClientConfig) *RESTClient {
	if cfg.BaseURL == "" {
		cfg.BaseURL = BaseURLProduction
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	return &RESTClient{
		baseURL:    cfg.BaseURL,
		apiKey:     cfg.APIKey,
		secretKey:  cfg.SecretKey,
		passphrase: cfg.Passphrase,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// sign generates HMAC-SHA256 signature for request
// Bitget signature: Base64(HMAC_SHA256(timestamp + method + requestPath + body, secretKey))
func (c *RESTClient) sign(timestamp, method, requestPath, body string) string {
	message := timestamp + method + requestPath + body
	h := hmac.New(sha256.New, []byte(c.secretKey))
	h.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
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

// doRequest performs HTTP request with authentication
func (c *RESTClient) doRequest(ctx context.Context, method, path string, params url.Values, body interface{}, authenticated bool, rateLimit int) ([]byte, error) {
	// Apply rate limiting
	rl := c.getRateLimiter(path, rateLimit)
	if err := rl.wait(ctx); err != nil {
		return nil, err
	}

	// Build URL
	fullURL := c.baseURL + path
	requestPath := path
	if len(params) > 0 {
		queryStr := params.Encode()
		fullURL += "?" + queryStr
		requestPath += "?" + queryStr
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

	// Set common headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("locale", "en-US")

	// Add authentication headers
	if authenticated && c.apiKey != "" {
		timestamp := c.getTimestamp()
		sign := c.sign(timestamp, method, requestPath, string(bodyBytes))

		req.Header.Set("ACCESS-KEY", c.apiKey)
		req.Header.Set("ACCESS-SIGN", sign)
		req.Header.Set("ACCESS-TIMESTAMP", timestamp)
		req.Header.Set("ACCESS-PASSPHRASE", c.passphrase)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
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
// Public Data Endpoints - Contracts
// =============================================================================

// GetContracts retrieves all contracts for a product type
func (c *RESTClient) GetContracts(ctx context.Context, productType string) ([]Contract, error) {
	params := url.Values{}
	params.Set("productType", productType)

	data, err := c.doRequest(ctx, http.MethodGet, PathContracts, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Contract]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// GetAllUSDTFuturesContracts retrieves all USDT futures contracts
func (c *RESTClient) GetAllUSDTFuturesContracts(ctx context.Context) ([]Contract, error) {
	return c.GetContracts(ctx, ProductTypeUSDTFutures)
}

// GetAllUSDCFuturesContracts retrieves all USDC futures contracts
func (c *RESTClient) GetAllUSDCFuturesContracts(ctx context.Context) ([]Contract, error) {
	return c.GetContracts(ctx, ProductTypeUSDCFutures)
}

// GetAllCoinFuturesContracts retrieves all coin-margined futures contracts
func (c *RESTClient) GetAllCoinFuturesContracts(ctx context.Context) ([]Contract, error) {
	return c.GetContracts(ctx, ProductTypeCoinFutures)
}

// =============================================================================
// Market Data Endpoints - Tickers
// =============================================================================

// GetTickers retrieves all tickers for a product type
func (c *RESTClient) GetTickers(ctx context.Context, productType string) ([]Ticker, error) {
	params := url.Values{}
	params.Set("productType", productType)

	data, err := c.doRequest(ctx, http.MethodGet, PathTickers, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Ticker]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// GetTicker retrieves ticker for a specific symbol
func (c *RESTClient) GetTicker(ctx context.Context, symbol, productType string) (*Ticker, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("productType", productType)

	data, err := c.doRequest(ctx, http.MethodGet, PathTicker, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Ticker]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no ticker data returned for %s", symbol)
	}

	return &resp.Data[0], nil
}

// =============================================================================
// Market Data Endpoints - Order Book
// =============================================================================

// GetOrderBook retrieves order book for a symbol (merge-depth endpoint)
// limit: 1, 5, 15, 50, 100 (default)
// precision: price precision scale (optional)
func (c *RESTClient) GetOrderBook(ctx context.Context, symbol, productType string, limit int, precision string) (*OrderBook, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("productType", productType)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if precision != "" {
		params.Set("precision", precision)
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathMergeDepth, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[OrderBook]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return &resp.Data, nil
}

// =============================================================================
// Market Data Endpoints - Candlesticks
// =============================================================================

// GetCandles retrieves candlestick data
// granularity: 1m, 3m, 5m, 15m, 30m, 1H, 2H, 4H, 6H, 12H, 1D, 3D, 1W, 1M
// limit: max 1000 (default 100)
func (c *RESTClient) GetCandles(ctx context.Context, symbol, productType, granularity string, startTime, endTime int64, limit int) ([]Candlestick, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("productType", productType)
	params.Set("granularity", granularity)
	if startTime > 0 {
		params.Set("startTime", strconv.FormatInt(startTime, 10))
	}
	if endTime > 0 {
		params.Set("endTime", strconv.FormatInt(endTime, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathCandles, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[][]string]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	// Convert to Candlestick structs
	candles := make([]Candlestick, len(resp.Data))
	for i, arr := range resp.Data {
		if len(arr) >= 7 {
			ts, _ := strconv.ParseInt(arr[0], 10, 64)
			candles[i] = Candlestick{
				Ts:          Timestamp(ts),
				Open:        arr[1],
				High:        arr[2],
				Low:         arr[3],
				Close:       arr[4],
				BaseVolume:  arr[5],
				QuoteVolume: arr[6],
			}
		}
	}

	return candles, nil
}

// GetHistoryCandles retrieves historical candlestick data (beyond recent limit)
func (c *RESTClient) GetHistoryCandles(ctx context.Context, symbol, productType, granularity string, startTime, endTime int64, limit int) ([]Candlestick, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("productType", productType)
	params.Set("granularity", granularity)
	if startTime > 0 {
		params.Set("startTime", strconv.FormatInt(startTime, 10))
	}
	if endTime > 0 {
		params.Set("endTime", strconv.FormatInt(endTime, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathHistoryCandles, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[][]string]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	// Convert to Candlestick structs
	candles := make([]Candlestick, len(resp.Data))
	for i, arr := range resp.Data {
		if len(arr) >= 7 {
			ts, _ := strconv.ParseInt(arr[0], 10, 64)
			candles[i] = Candlestick{
				Ts:          Timestamp(ts),
				Open:        arr[1],
				High:        arr[2],
				Low:         arr[3],
				Close:       arr[4],
				BaseVolume:  arr[5],
				QuoteVolume: arr[6],
			}
		}
	}

	return candles, nil
}

// =============================================================================
// Market Data Endpoints - Funding Rate
// =============================================================================

// GetCurrentFundingRate retrieves current funding rate for a symbol
func (c *RESTClient) GetCurrentFundingRate(ctx context.Context, symbol, productType string) (*FundingRate, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("productType", productType)

	data, err := c.doRequest(ctx, http.MethodGet, PathFundingRate, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]FundingRate]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no funding rate data returned for %s", symbol)
	}

	return &resp.Data[0], nil
}

// GetHistoryFundingRate retrieves historical funding rates
func (c *RESTClient) GetHistoryFundingRate(ctx context.Context, symbol, productType string, pageSize int, pageNo int) ([]FundingRateHistory, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("productType", productType)
	if pageSize > 0 {
		params.Set("pageSize", strconv.Itoa(pageSize))
	}
	if pageNo > 0 {
		params.Set("pageNo", strconv.Itoa(pageNo))
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathHistoryFundingRate, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]FundingRateHistory]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// =============================================================================
// Market Data Endpoints - Trades
// =============================================================================

// GetTrades retrieves recent public trades
func (c *RESTClient) GetTrades(ctx context.Context, symbol, productType string, limit int) ([]Trade, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("productType", productType)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathTrades, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Trade]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// =============================================================================
// Account Endpoints
// =============================================================================

// GetAccount retrieves single account balance
func (c *RESTClient) GetAccount(ctx context.Context, symbol, productType, marginCoin string) (*Account, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("productType", productType)
	params.Set("marginCoin", marginCoin)

	data, err := c.doRequest(ctx, http.MethodGet, PathAccount, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[Account]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return &resp.Data, nil
}

// GetAccounts retrieves all account balances for a product type
func (c *RESTClient) GetAccounts(ctx context.Context, productType string) ([]AccountList, error) {
	params := url.Values{}
	params.Set("productType", productType)

	data, err := c.doRequest(ctx, http.MethodGet, PathAccounts, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]AccountList]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// SetLeverage sets leverage for a symbol
func (c *RESTClient) SetLeverage(ctx context.Context, symbol, productType, marginCoin string, leverage int, holdSide string) error {
	body := map[string]interface{}{
		"symbol":      symbol,
		"productType": productType,
		"marginCoin":  marginCoin,
		"leverage":    strconv.Itoa(leverage),
	}
	if holdSide != "" {
		body["holdSide"] = holdSide
	}

	data, err := c.doRequest(ctx, http.MethodPost, PathSetLeverage, nil, body, true, 10)
	if err != nil {
		return err
	}

	var resp APIResponse[json.RawMessage]
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return nil
}

// SetMarginMode sets margin mode (crossed or isolated)
func (c *RESTClient) SetMarginMode(ctx context.Context, symbol, productType, marginCoin, marginMode string) error {
	body := map[string]interface{}{
		"symbol":      symbol,
		"productType": productType,
		"marginCoin":  marginCoin,
		"marginMode":  marginMode,
	}

	data, err := c.doRequest(ctx, http.MethodPost, PathSetMarginMode, nil, body, true, 10)
	if err != nil {
		return err
	}

	var resp APIResponse[json.RawMessage]
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return nil
}

// SetPositionMode sets position mode (single_hold or double_hold)
func (c *RESTClient) SetPositionMode(ctx context.Context, productType, posMode string) error {
	body := map[string]interface{}{
		"productType": productType,
		"posMode":     posMode,
	}

	data, err := c.doRequest(ctx, http.MethodPost, PathSetPositionMode, nil, body, true, 10)
	if err != nil {
		return err
	}

	var resp APIResponse[json.RawMessage]
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return nil
}

// =============================================================================
// Position Endpoints
// =============================================================================

// GetPositions retrieves all positions for a product type
func (c *RESTClient) GetPositions(ctx context.Context, productType, marginCoin string) ([]Position, error) {
	params := url.Values{}
	params.Set("productType", productType)
	if marginCoin != "" {
		params.Set("marginCoin", marginCoin)
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathPositions, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Position]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// GetSinglePosition retrieves position for a specific symbol
func (c *RESTClient) GetSinglePosition(ctx context.Context, symbol, productType, marginCoin string) ([]Position, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("productType", productType)
	params.Set("marginCoin", marginCoin)

	data, err := c.doRequest(ctx, http.MethodGet, PathSinglePosition, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Position]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// GetHistoryPositions retrieves historical positions
func (c *RESTClient) GetHistoryPositions(ctx context.Context, productType string, startTime, endTime int64, pageSize int, lastEndId string) ([]PositionHistory, error) {
	params := url.Values{}
	params.Set("productType", productType)
	if startTime > 0 {
		params.Set("startTime", strconv.FormatInt(startTime, 10))
	}
	if endTime > 0 {
		params.Set("endTime", strconv.FormatInt(endTime, 10))
	}
	if pageSize > 0 {
		params.Set("pageSize", strconv.Itoa(pageSize))
	}
	if lastEndId != "" {
		params.Set("lastEndId", lastEndId)
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathHistoryPosition, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[struct {
		List      []PositionHistory `json:"list"`
		LastEndId string            `json:"lastEndId"`
	}]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data.List, nil
}

// =============================================================================
// Trade Endpoints - Order Placement
// =============================================================================

// PlaceOrder places a single order
func (c *RESTClient) PlaceOrder(ctx context.Context, req *PlaceOrderRequest) (*OrderResult, error) {
	data, err := c.doRequest(ctx, http.MethodPost, PathPlaceOrder, nil, req, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[OrderResult]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return &resp.Data, nil
}

// BatchPlaceOrder places multiple orders at once
func (c *RESTClient) BatchPlaceOrder(ctx context.Context, req *BatchPlaceOrderRequest) (*BatchOrderResult, error) {
	data, err := c.doRequest(ctx, http.MethodPost, PathBatchPlaceOrder, nil, req, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[BatchOrderResult]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return &resp.Data, nil
}

// =============================================================================
// Trade Endpoints - Order Cancellation
// =============================================================================

// CancelOrder cancels a single order
func (c *RESTClient) CancelOrder(ctx context.Context, req *CancelOrderRequest) (*CancelResult, error) {
	data, err := c.doRequest(ctx, http.MethodPost, PathCancelOrder, nil, req, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[CancelResult]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return &resp.Data, nil
}

// BatchCancelOrder cancels multiple orders
func (c *RESTClient) BatchCancelOrder(ctx context.Context, req *BatchCancelOrderRequest) ([]CancelResult, error) {
	data, err := c.doRequest(ctx, http.MethodPost, PathBatchCancelOrder, nil, req, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[struct {
		SuccessList []CancelResult `json:"successList"`
		FailureList []FailedOrder  `json:"failureList"`
	}]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data.SuccessList, nil
}

// =============================================================================
// Trade Endpoints - Order Modification
// =============================================================================

// ModifyOrder modifies an existing order
func (c *RESTClient) ModifyOrder(ctx context.Context, req *ModifyOrderRequest) (*OrderResult, error) {
	data, err := c.doRequest(ctx, http.MethodPost, PathModifyOrder, nil, req, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[OrderResult]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return &resp.Data, nil
}

// =============================================================================
// Trade Endpoints - Order Queries
// =============================================================================

// GetPendingOrders retrieves all pending orders
func (c *RESTClient) GetPendingOrders(ctx context.Context, productType string, symbol string, pageSize int, idLessThan string) ([]Order, error) {
	params := url.Values{}
	params.Set("productType", productType)
	if symbol != "" {
		params.Set("symbol", symbol)
	}
	if pageSize > 0 {
		params.Set("pageSize", strconv.Itoa(pageSize))
	}
	if idLessThan != "" {
		params.Set("idLessThan", idLessThan)
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathPendingOrders, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[struct {
		EntrustedList []Order `json:"entrustedList"`
		EndId         string  `json:"endId"`
	}]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data.EntrustedList, nil
}

// GetHistoryOrders retrieves historical orders
func (c *RESTClient) GetHistoryOrders(ctx context.Context, productType string, symbol string, startTime, endTime int64, pageSize int, idLessThan string) ([]Order, error) {
	params := url.Values{}
	params.Set("productType", productType)
	if symbol != "" {
		params.Set("symbol", symbol)
	}
	if startTime > 0 {
		params.Set("startTime", strconv.FormatInt(startTime, 10))
	}
	if endTime > 0 {
		params.Set("endTime", strconv.FormatInt(endTime, 10))
	}
	if pageSize > 0 {
		params.Set("pageSize", strconv.Itoa(pageSize))
	}
	if idLessThan != "" {
		params.Set("idLessThan", idLessThan)
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathHistoryOrders, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[struct {
		EntrustedList []Order `json:"entrustedList"`
		EndId         string  `json:"endId"`
	}]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data.EntrustedList, nil
}

// GetOrderDetail retrieves details of a specific order
func (c *RESTClient) GetOrderDetail(ctx context.Context, productType, symbol, orderId, clientOid string) (*Order, error) {
	params := url.Values{}
	params.Set("productType", productType)
	params.Set("symbol", symbol)
	if orderId != "" {
		params.Set("orderId", orderId)
	}
	if clientOid != "" {
		params.Set("clientOid", clientOid)
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathOrderDetail, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[Order]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return &resp.Data, nil
}

// GetFillHistory retrieves fill history
func (c *RESTClient) GetFillHistory(ctx context.Context, productType string, symbol string, startTime, endTime int64, pageSize int, idLessThan string) ([]Trade, error) {
	params := url.Values{}
	params.Set("productType", productType)
	if symbol != "" {
		params.Set("symbol", symbol)
	}
	if startTime > 0 {
		params.Set("startTime", strconv.FormatInt(startTime, 10))
	}
	if endTime > 0 {
		params.Set("endTime", strconv.FormatInt(endTime, 10))
	}
	if pageSize > 0 {
		params.Set("pageSize", strconv.Itoa(pageSize))
	}
	if idLessThan != "" {
		params.Set("idLessThan", idLessThan)
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathFills, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[struct {
		FillList []Trade `json:"fillList"`
		EndId    string  `json:"endId"`
	}]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data.FillList, nil
}
