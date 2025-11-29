// Package okx provides REST API client for OKX exchange.
package okx

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
	BaseURLProduction = "https://www.okx.com"
	BaseURLDemo       = "https://www.okx.com" // Use demo header instead

	// Public endpoints
	PathInstruments         = "/api/v5/public/instruments"
	PathTickers             = "/api/v5/market/tickers"
	PathTicker              = "/api/v5/market/ticker"
	PathOrderBook           = "/api/v5/market/books"
	PathCandles             = "/api/v5/market/candles"
	PathHistoryCandles      = "/api/v5/market/history-candles"
	PathIndexCandles        = "/api/v5/market/index-candles"
	PathHistoryIndexCandles = "/api/v5/market/history-index-candles"
	PathTrades              = "/api/v5/market/trades"
	PathIndexTickers        = "/api/v5/market/index-tickers"
	PathFundingRate         = "/api/v5/public/funding-rate"
	PathFundingRateHistory  = "/api/v5/public/funding-rate-history"

	// Private endpoints - Account
	PathBalance         = "/api/v5/account/balance"
	PathPositions       = "/api/v5/account/positions"
	PathAccountConfig   = "/api/v5/account/config"
	PathSetPositionMode = "/api/v5/account/set-position-mode"
	PathSetLeverage     = "/api/v5/account/set-leverage"
	PathTradeFee        = "/api/v5/account/trade-fee"
	PathMaxWithdrawal   = "/api/v5/account/max-withdrawal"

	// Private endpoints - Trade
	PathOrder                = "/api/v5/trade/order"
	PathBatchOrders          = "/api/v5/trade/batch-orders"
	PathCancelOrder          = "/api/v5/trade/cancel-order"
	PathCancelBatchOrders    = "/api/v5/trade/cancel-batch-orders"
	PathAmendOrder           = "/api/v5/trade/amend-order"
	PathAmendBatchOrders     = "/api/v5/trade/amend-batch-orders"
	PathOrdersPending        = "/api/v5/trade/orders-pending"
	PathOrdersHistory        = "/api/v5/trade/orders-history"
	PathOrdersHistoryArchive = "/api/v5/trade/orders-history-archive"
	PathOrderDetails         = "/api/v5/trade/order"
	PathFills                = "/api/v5/trade/fills"
	PathFillsHistory         = "/api/v5/trade/fills-history"

	// Private endpoints - Asset
	PathCurrencies    = "/api/v5/asset/currencies"
	PathAssetBalances = "/api/v5/asset/balances"
)

// RESTClient provides methods to interact with OKX REST API
type RESTClient struct {
	baseURL    string
	apiKey     string
	secretKey  string
	passphrase string
	httpClient *http.Client
	demoMode   bool

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
	DemoMode   bool
	Timeout    time.Duration
}

// NewRESTClient creates a new OKX REST client
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
		demoMode:   cfg.DemoMode,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// sign generates HMAC-SHA256 signature for request
func (c *RESTClient) sign(timestamp, method, requestPath, body string) string {
	message := timestamp + method + requestPath + body
	h := hmac.New(sha256.New, []byte(c.secretKey))
	h.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// getTimestamp returns current timestamp in ISO format
func (c *RESTClient) getTimestamp() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.999Z")
}

// getRateLimiter gets or creates a rate limiter for a path
func (c *RESTClient) getRateLimiter(path string, maxRequests int) *rateLimiter {
	if v, ok := c.rateLimiter.Load(path); ok {
		return v.(*rateLimiter)
	}
	rl := newRateLimiter(maxRequests, 2*time.Second)
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

	// Add demo mode header if needed
	if c.demoMode {
		req.Header.Set("x-simulated-trading", "1")
	}

	// Add authentication headers
	if authenticated && c.apiKey != "" {
		timestamp := c.getTimestamp()
		requestPath := path
		if len(params) > 0 {
			requestPath += "?" + params.Encode()
		}
		sign := c.sign(timestamp, method, requestPath, string(bodyBytes))

		req.Header.Set("OK-ACCESS-KEY", c.apiKey)
		req.Header.Set("OK-ACCESS-SIGN", sign)
		req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
		req.Header.Set("OK-ACCESS-PASSPHRASE", c.passphrase)
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
// Public Data Endpoints
// =============================================================================

// GetInstruments retrieves all trading instruments
func (c *RESTClient) GetInstruments(ctx context.Context, instType string, uly string, instFamily string, instID string) ([]Instrument, error) {
	params := url.Values{}
	params.Set("instType", instType)
	if uly != "" {
		params.Set("uly", uly)
	}
	if instFamily != "" {
		params.Set("instFamily", instFamily)
	}
	if instID != "" {
		params.Set("instId", instID)
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathInstruments, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Instrument]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// GetAllSwapInstruments retrieves all perpetual swap instruments
func (c *RESTClient) GetAllSwapInstruments(ctx context.Context) ([]Instrument, error) {
	return c.GetInstruments(ctx, InstTypeSwap, "", "", "")
}

// GetAllFuturesInstruments retrieves all futures instruments
func (c *RESTClient) GetAllFuturesInstruments(ctx context.Context, instFamily string) ([]Instrument, error) {
	return c.GetInstruments(ctx, InstTypeFutures, "", instFamily, "")
}

// =============================================================================
// Market Data Endpoints
// =============================================================================

// GetTickers retrieves all tickers for an instrument type
func (c *RESTClient) GetTickers(ctx context.Context, instType string, instFamily string) ([]Ticker, error) {
	params := url.Values{}
	params.Set("instType", instType)
	if instFamily != "" {
		params.Set("instFamily", instFamily)
	}

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

// GetTicker retrieves ticker for a specific instrument
func (c *RESTClient) GetTicker(ctx context.Context, instID string) (*Ticker, error) {
	params := url.Values{}
	params.Set("instId", instID)

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
		return nil, fmt.Errorf("no ticker data for %s", instID)
	}

	return &resp.Data[0], nil
}

// GetOrderBook retrieves order book for an instrument
func (c *RESTClient) GetOrderBook(ctx context.Context, instID string, depth int) (*OrderBook, error) {
	params := url.Values{}
	params.Set("instId", instID)
	if depth > 0 {
		params.Set("sz", strconv.Itoa(depth))
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathOrderBook, params, nil, false, 40)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]OrderBook]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no order book data for %s", instID)
	}

	return &resp.Data[0], nil
}

// GetCandles retrieves candlestick data
func (c *RESTClient) GetCandles(ctx context.Context, instID string, bar string, after int64, before int64, limit int) ([]Candlestick, error) {
	params := url.Values{}
	params.Set("instId", instID)
	if bar != "" {
		params.Set("bar", bar)
	}
	if after > 0 {
		params.Set("after", strconv.FormatInt(after, 10))
	}
	if before > 0 {
		params.Set("before", strconv.FormatInt(before, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathCandles, params, nil, false, 40)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Candlestick]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// GetHistoryCandles retrieves historical candlestick data
func (c *RESTClient) GetHistoryCandles(ctx context.Context, instID string, bar string, after int64, before int64, limit int) ([]Candlestick, error) {
	params := url.Values{}
	params.Set("instId", instID)
	if bar != "" {
		params.Set("bar", bar)
	}
	if after > 0 {
		params.Set("after", strconv.FormatInt(after, 10))
	}
	if before > 0 {
		params.Set("before", strconv.FormatInt(before, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathHistoryCandles, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Candlestick]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// GetTrades retrieves recent public trades
func (c *RESTClient) GetTrades(ctx context.Context, instID string, limit int) ([]Trade, error) {
	params := url.Values{}
	params.Set("instId", instID)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathTrades, params, nil, false, 100)
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

// GetIndexTickers retrieves index tickers
func (c *RESTClient) GetIndexTickers(ctx context.Context, quoteCcy string, instID string) ([]IndexTicker, error) {
	params := url.Values{}
	if quoteCcy != "" {
		params.Set("quoteCcy", quoteCcy)
	}
	if instID != "" {
		params.Set("instId", instID)
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathIndexTickers, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]IndexTicker]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// =============================================================================
// Funding Rate Endpoints
// =============================================================================

// GetFundingRate retrieves current funding rate
func (c *RESTClient) GetFundingRate(ctx context.Context, instID string) (*FundingRate, error) {
	params := url.Values{}
	params.Set("instId", instID)

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
		return nil, fmt.Errorf("no funding rate data for %s", instID)
	}

	return &resp.Data[0], nil
}

// GetFundingRateHistory retrieves historical funding rates
func (c *RESTClient) GetFundingRateHistory(ctx context.Context, instID string, before int64, after int64, limit int) ([]FundingRateHistory, error) {
	params := url.Values{}
	params.Set("instId", instID)
	if before > 0 {
		params.Set("before", strconv.FormatInt(before, 10))
	}
	if after > 0 {
		params.Set("after", strconv.FormatInt(after, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathFundingRateHistory, params, nil, false, 10)
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
// Asset Endpoints
// =============================================================================

// GetCurrencies retrieves all currencies and their deposit/withdrawal status
func (c *RESTClient) GetCurrencies(ctx context.Context) ([]Currency, error) {
	data, err := c.doRequest(ctx, http.MethodGet, PathCurrencies, nil, nil, true, 6)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Currency]
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

// GetBalance retrieves account balance
func (c *RESTClient) GetBalance(ctx context.Context, ccy string) (*AccountBalance, error) {
	params := url.Values{}
	if ccy != "" {
		params.Set("ccy", ccy)
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathBalance, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]AccountBalance]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no balance data")
	}

	return &resp.Data[0], nil
}

// GetPositions retrieves all positions
func (c *RESTClient) GetPositions(ctx context.Context, instType string, instID string, posID string) ([]Position, error) {
	params := url.Values{}
	if instType != "" {
		params.Set("instType", instType)
	}
	if instID != "" {
		params.Set("instId", instID)
	}
	if posID != "" {
		params.Set("posId", posID)
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

// GetAccountConfig retrieves account configuration
func (c *RESTClient) GetAccountConfig(ctx context.Context) (*AccountConfig, error) {
	data, err := c.doRequest(ctx, http.MethodGet, PathAccountConfig, nil, nil, true, 5)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]AccountConfig]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no account config data")
	}

	return &resp.Data[0], nil
}

// SetPositionMode sets position mode (hedge or net)
func (c *RESTClient) SetPositionMode(ctx context.Context, posMode string) error {
	body := map[string]string{"posMode": posMode}

	data, err := c.doRequest(ctx, http.MethodPost, PathSetPositionMode, nil, body, true, 5)
	if err != nil {
		return err
	}

	var resp APIResponse[interface{}]
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return nil
}

// SetLeverage sets leverage for an instrument
func (c *RESTClient) SetLeverage(ctx context.Context, instID string, lever string, mgnMode string, posSide string) error {
	body := map[string]string{
		"instId":  instID,
		"lever":   lever,
		"mgnMode": mgnMode,
	}
	if posSide != "" {
		body["posSide"] = posSide
	}

	data, err := c.doRequest(ctx, http.MethodPost, PathSetLeverage, nil, body, true, 20)
	if err != nil {
		return err
	}

	var resp APIResponse[interface{}]
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return nil
}

// GetTradeFee retrieves trading fee rates
func (c *RESTClient) GetTradeFee(ctx context.Context, instType string, instID string, instFamily string) (*TradeFee, error) {
	params := url.Values{}
	params.Set("instType", instType)
	if instID != "" {
		params.Set("instId", instID)
	}
	if instFamily != "" {
		params.Set("instFamily", instFamily)
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathTradeFee, params, nil, true, 5)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]TradeFee]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no fee data")
	}

	return &resp.Data[0], nil
}

// =============================================================================
// Trading Endpoints
// =============================================================================

// PlaceOrder places a single order
func (c *RESTClient) PlaceOrder(ctx context.Context, req *PlaceOrderRequest) (*OrderResult, error) {
	data, err := c.doRequest(ctx, http.MethodPost, PathOrder, nil, req, true, 60)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]OrderResult]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no order result")
	}

	result := &resp.Data[0]
	if !IsSuccess(result.SCode) {
		return nil, &APIError{Code: result.SCode, Message: result.SMsg}
	}

	return result, nil
}

// PlaceBatchOrders places multiple orders (max 20)
func (c *RESTClient) PlaceBatchOrders(ctx context.Context, orders []*PlaceOrderRequest) ([]OrderResult, error) {
	if len(orders) > 20 {
		return nil, fmt.Errorf("max 20 orders per batch")
	}

	data, err := c.doRequest(ctx, http.MethodPost, PathBatchOrders, nil, orders, true, 300)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]OrderResult]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// CancelOrder cancels a single order
func (c *RESTClient) CancelOrder(ctx context.Context, req *CancelOrderRequest) (*CancelResult, error) {
	data, err := c.doRequest(ctx, http.MethodPost, PathCancelOrder, nil, req, true, 60)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]CancelResult]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no cancel result")
	}

	result := &resp.Data[0]
	if !IsSuccess(result.SCode) {
		return nil, &APIError{Code: result.SCode, Message: result.SMsg}
	}

	return result, nil
}

// CancelBatchOrders cancels multiple orders (max 20)
func (c *RESTClient) CancelBatchOrders(ctx context.Context, orders []*CancelOrderRequest) ([]CancelResult, error) {
	if len(orders) > 20 {
		return nil, fmt.Errorf("max 20 orders per batch")
	}

	data, err := c.doRequest(ctx, http.MethodPost, PathCancelBatchOrders, nil, orders, true, 300)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]CancelResult]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// AmendOrder modifies an existing order
func (c *RESTClient) AmendOrder(ctx context.Context, req *AmendOrderRequest) (*AmendResult, error) {
	data, err := c.doRequest(ctx, http.MethodPost, PathAmendOrder, nil, req, true, 60)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]AmendResult]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no amend result")
	}

	result := &resp.Data[0]
	if !IsSuccess(result.SCode) {
		return nil, &APIError{Code: result.SCode, Message: result.SMsg}
	}

	return result, nil
}

// GetOrder retrieves order details
func (c *RESTClient) GetOrder(ctx context.Context, instID string, orderID string, clOrdID string) (*Order, error) {
	params := url.Values{}
	params.Set("instId", instID)
	if orderID != "" {
		params.Set("ordId", orderID)
	}
	if clOrdID != "" {
		params.Set("clOrdId", clOrdID)
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathOrderDetails, params, nil, true, 60)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Order]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("order not found")
	}

	return &resp.Data[0], nil
}

// GetPendingOrders retrieves all active orders
func (c *RESTClient) GetPendingOrders(ctx context.Context, instType string, instID string, ordType string, state string, limit int) ([]Order, error) {
	params := url.Values{}
	if instType != "" {
		params.Set("instType", instType)
	}
	if instID != "" {
		params.Set("instId", instID)
	}
	if ordType != "" {
		params.Set("ordType", ordType)
	}
	if state != "" {
		params.Set("state", state)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathOrdersPending, params, nil, true, 60)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Order]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// GetOrdersHistory retrieves order history (last 7 days)
func (c *RESTClient) GetOrdersHistory(ctx context.Context, instType string, instID string, ordType string, state string, before int64, after int64, limit int) ([]Order, error) {
	params := url.Values{}
	params.Set("instType", instType)
	if instID != "" {
		params.Set("instId", instID)
	}
	if ordType != "" {
		params.Set("ordType", ordType)
	}
	if state != "" {
		params.Set("state", state)
	}
	if before > 0 {
		params.Set("before", strconv.FormatInt(before, 10))
	}
	if after > 0 {
		params.Set("after", strconv.FormatInt(after, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doRequest(ctx, http.MethodGet, PathOrdersHistory, params, nil, true, 40)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Order]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !IsSuccess(resp.Code) {
		return nil, &APIError{Code: resp.Code, Message: resp.Msg}
	}

	return resp.Data, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// PlaceMarketOrder is a convenience method for placing a market order
func (c *RESTClient) PlaceMarketOrder(ctx context.Context, instID string, side string, sz string, tdMode string, posSide string, reduceOnly bool) (*OrderResult, error) {
	req := &PlaceOrderRequest{
		InstID:     instID,
		TdMode:     tdMode,
		Side:       side,
		OrdType:    OrdTypeMarket,
		Sz:         sz,
		PosSide:    posSide,
		ReduceOnly: reduceOnly,
	}
	return c.PlaceOrder(ctx, req)
}

// PlaceLimitOrder is a convenience method for placing a limit order
func (c *RESTClient) PlaceLimitOrder(ctx context.Context, instID string, side string, sz string, px string, tdMode string, posSide string, reduceOnly bool) (*OrderResult, error) {
	req := &PlaceOrderRequest{
		InstID:     instID,
		TdMode:     tdMode,
		Side:       side,
		OrdType:    OrdTypeLimit,
		Sz:         sz,
		Px:         px,
		PosSide:    posSide,
		ReduceOnly: reduceOnly,
	}
	return c.PlaceOrder(ctx, req)
}

// PlacePostOnlyOrder is a convenience method for placing a post-only limit order
func (c *RESTClient) PlacePostOnlyOrder(ctx context.Context, instID string, side string, sz string, px string, tdMode string, posSide string) (*OrderResult, error) {
	req := &PlaceOrderRequest{
		InstID:  instID,
		TdMode:  tdMode,
		Side:    side,
		OrdType: OrdTypePostOnly,
		Sz:      sz,
		Px:      px,
		PosSide: posSide,
	}
	return c.PlaceOrder(ctx, req)
}
