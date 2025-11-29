// Package kucoin provides REST API client for KuCoin Futures exchange.
package kucoin

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
	"strings"
	"sync"
	"time"
)

// REST API endpoints
const (
	// Public endpoints - Market Data
	PathContractsActive    = "/api/v1/contracts/active"
	PathContract           = "/api/v1/contracts/{symbol}"
	PathTicker             = "/api/v1/ticker"
	PathAllTickers         = "/api/v1/allTickers"
	PathLevel2Snapshot     = "/api/v1/level2/snapshot"
	PathLevel2Depth20      = "/api/v1/level2/depth20"
	PathLevel2Depth100     = "/api/v1/level2/depth100"
	PathKline              = "/api/v1/kline/query"
	PathTradeHistory       = "/api/v1/trade/history"
	PathFundingRateCurrent = "/api/v1/funding-rate/{symbol}/current"
	PathFundingRates       = "/api/v1/contract/funding-rates"
	PathMarkPrice          = "/api/v1/mark-price/{symbol}/current"
	PathServiceStatus      = "/api/v1/status"

	// Private endpoints - Orders
	PathOrders           = "/api/v1/orders"
	PathOrder            = "/api/v1/orders/{orderId}"
	PathOrderByClientOid = "/api/v1/orders/byClientOid"
	PathBatchOrders      = "/api/v1/orders/multi"
	PathCancelAllOrders  = "/api/v1/orders"
	PathStopOrders       = "/api/v1/stopOrders"
	PathStopOrder        = "/api/v1/stopOrders/{orderId}"

	// Private endpoints - Fills
	PathFills          = "/api/v1/fills"
	PathRecentFills    = "/api/v1/recentFills"
	PathFillsTimerange = "/api/v1/fills-timerange"

	// Private endpoints - Position
	PathPositions             = "/api/v1/positions"
	PathPosition              = "/api/v1/position"
	PathPositionMarginAuto    = "/api/v1/position/margin/auto-deposit-status"
	PathPositionMarginDeposit = "/api/v1/position/margin/deposit-margin"
	PathPositionRiskLimit     = "/api/v1/position/risk-limit-level/change"

	// Private endpoints - Leverage & Margin
	PathLeverageGet    = "/api/v2/getMaxOpenSize"
	PathLeverageChange = "/api/v2/changeCrossUserLeverage"
	PathMarginMode     = "/api/v2/position/changeMarginMode"

	// Private endpoints - Account
	PathAccountOverview = "/api/v1/account-overview"
	PathTransactionLog  = "/api/v1/transaction-history"

	// Private endpoints - Funding
	PathFundingHistory = "/api/v1/funding-history"

	// WebSocket token endpoints
	PathBulletPublic  = "/api/v1/bullet-public"
	PathBulletPrivate = "/api/v1/bullet-private"
)

// RESTClient provides methods to interact with KuCoin REST API
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
	BaseURL    string
	APIKey     string
	SecretKey  string
	Passphrase string
	Timeout    time.Duration
}

// NewRESTClient creates a new KuCoin REST client
func NewRESTClient(cfg RESTClientConfig) *RESTClient {
	if cfg.BaseURL == "" {
		cfg.BaseURL = FuturesRESTBaseURL
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

// sign generates HMAC-SHA256 signature for KuCoin API
// signature = base64(hmac_sha256(secret, timestamp + method + endpoint + body))
func (c *RESTClient) sign(timestamp, method, endpoint, body string) string {
	message := timestamp + method + endpoint + body
	h := hmac.New(sha256.New, []byte(c.secretKey))
	h.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// signPassphrase encrypts passphrase for API Key Version 2
// encrypted = base64(hmac_sha256(secret, passphrase))
func (c *RESTClient) signPassphrase() string {
	h := hmac.New(sha256.New, []byte(c.secretKey))
	h.Write([]byte(c.passphrase))
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

// buildPath replaces path parameters
func buildPath(template string, params map[string]string) string {
	result := template
	for k, v := range params {
		result = strings.Replace(result, "{"+k+"}", v, -1)
	}
	return result
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

	// Build query string
	queryString := ""
	if len(params) > 0 {
		queryString = "?" + params.Encode()
		fullURL += queryString
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
	req.Header.Set("Accept", "application/json")

	// Add authentication headers for private endpoints
	if authenticated {
		timestamp := c.getTimestamp()
		bodyStr := ""
		if len(bodyBytes) > 0 {
			bodyStr = string(bodyBytes)
		}
		// For signature, path includes query string
		signPath := path + queryString
		signature := c.sign(timestamp, method, signPath, bodyStr)
		encryptedPassphrase := c.signPassphrase()

		req.Header.Set("KC-API-KEY", c.apiKey)
		req.Header.Set("KC-API-SIGN", signature)
		req.Header.Set("KC-API-TIMESTAMP", timestamp)
		req.Header.Set("KC-API-PASSPHRASE", encryptedPassphrase)
		req.Header.Set("KC-API-KEY-VERSION", APIKeyVersion)
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

	// Parse response
	var apiResp Response
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w, body: %s", err, string(respBody))
	}

	// Check for API errors
	if !apiResp.IsSuccess() {
		return nil, apiResp.Error()
	}

	return apiResp.Data, nil
}

// =============================================================================
// Public Market Data APIs
// =============================================================================

// GetContracts fetches all active futures contracts
func (c *RESTClient) GetContracts(ctx context.Context) ([]*Contract, error) {
	body, err := c.doRequest(ctx, http.MethodGet, PathContractsActive, nil, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var contracts []*Contract
	if err := json.Unmarshal(body, &contracts); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return contracts, nil
}

// GetContract fetches a single futures contract
func (c *RESTClient) GetContract(ctx context.Context, symbol string) (*Contract, error) {
	path := buildPath(PathContract, map[string]string{"symbol": symbol})

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var contract Contract
	if err := json.Unmarshal(body, &contract); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &contract, nil
}

// GetTicker fetches ticker for a symbol
func (c *RESTClient) GetTicker(ctx context.Context, symbol string) (*Ticker, error) {
	params := url.Values{}
	params.Set("symbol", symbol)

	body, err := c.doRequest(ctx, http.MethodGet, PathTicker, params, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var ticker Ticker
	if err := json.Unmarshal(body, &ticker); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &ticker, nil
}

// GetAllTickers fetches all tickers
func (c *RESTClient) GetAllTickers(ctx context.Context) ([]*AllTickersItem, error) {
	body, err := c.doRequest(ctx, http.MethodGet, PathAllTickers, nil, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var tickers []*AllTickersItem
	if err := json.Unmarshal(body, &tickers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return tickers, nil
}

// GetOrderBook fetches full order book (Level 2) snapshot
func (c *RESTClient) GetOrderBook(ctx context.Context, symbol string) (*OrderBook, error) {
	params := url.Values{}
	params.Set("symbol", symbol)

	body, err := c.doRequest(ctx, http.MethodGet, PathLevel2Snapshot, params, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var orderBook OrderBook
	if err := json.Unmarshal(body, &orderBook); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &orderBook, nil
}

// GetOrderBookDepth fetches partial order book (depth 20 or 100)
func (c *RESTClient) GetOrderBookDepth(ctx context.Context, symbol string, depth int) (*OrderBook, error) {
	params := url.Values{}
	params.Set("symbol", symbol)

	path := PathLevel2Depth20
	if depth >= 100 {
		path = PathLevel2Depth100
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, params, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var orderBook OrderBook
	if err := json.Unmarshal(body, &orderBook); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &orderBook, nil
}

// GetKlines fetches candlestick data
// granularity: 1, 5, 15, 30, 60, 120, 240, 480, 720, 1440, 10080 (minutes)
func (c *RESTClient) GetKlines(ctx context.Context, symbol string, granularity int, from, to int64) ([][]interface{}, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("granularity", strconv.Itoa(granularity))
	if from > 0 {
		params.Set("from", strconv.FormatInt(from, 10))
	}
	if to > 0 {
		params.Set("to", strconv.FormatInt(to, 10))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathKline, params, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var klines [][]interface{}
	if err := json.Unmarshal(body, &klines); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return klines, nil
}

// GetTradeHistory fetches recent trades
func (c *RESTClient) GetTradeHistory(ctx context.Context, symbol string) ([]*Trade, error) {
	params := url.Values{}
	params.Set("symbol", symbol)

	body, err := c.doRequest(ctx, http.MethodGet, PathTradeHistory, params, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var trades []*Trade
	if err := json.Unmarshal(body, &trades); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return trades, nil
}

// GetFundingRate fetches current funding rate
func (c *RESTClient) GetFundingRate(ctx context.Context, symbol string) (*FundingRate, error) {
	path := buildPath(PathFundingRateCurrent, map[string]string{"symbol": symbol})

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var fundingRate FundingRate
	if err := json.Unmarshal(body, &fundingRate); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &fundingRate, nil
}

// GetFundingRateHistory fetches historical funding rates
func (c *RESTClient) GetFundingRateHistory(ctx context.Context, symbol string, from, to int64) ([]*FundingRateHistory, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	if from > 0 {
		params.Set("from", strconv.FormatInt(from, 10))
	}
	if to > 0 {
		params.Set("to", strconv.FormatInt(to, 10))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathFundingRates, params, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var rates []*FundingRateHistory
	if err := json.Unmarshal(body, &rates); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return rates, nil
}

// GetMarkPrice fetches current mark price
func (c *RESTClient) GetMarkPrice(ctx context.Context, symbol string) (*MarkPrice, error) {
	path := buildPath(PathMarkPrice, map[string]string{"symbol": symbol})

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var markPrice MarkPrice
	if err := json.Unmarshal(body, &markPrice); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &markPrice, nil
}

// GetServiceStatus fetches service status
func (c *RESTClient) GetServiceStatus(ctx context.Context) (*ServiceStatus, error) {
	body, err := c.doRequest(ctx, http.MethodGet, PathServiceStatus, nil, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var status ServiceStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &status, nil
}

// =============================================================================
// WebSocket Token APIs
// =============================================================================

// GetPublicToken fetches WebSocket connection token for public channels
func (c *RESTClient) GetPublicToken(ctx context.Context) (*WSToken, error) {
	body, err := c.doRequest(ctx, http.MethodPost, PathBulletPublic, nil, nil, false, 50)
	if err != nil {
		return nil, err
	}

	var token WSToken
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &token, nil
}

// GetPrivateToken fetches WebSocket connection token for private channels
func (c *RESTClient) GetPrivateToken(ctx context.Context) (*WSToken, error) {
	body, err := c.doRequest(ctx, http.MethodPost, PathBulletPrivate, nil, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var token WSToken
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &token, nil
}

// =============================================================================
// Private Order APIs
// =============================================================================

// PlaceOrder places a futures order
func (c *RESTClient) PlaceOrder(ctx context.Context, req *OrderRequest) (*OrderResponse, error) {
	body, err := c.doRequest(ctx, http.MethodPost, PathOrders, nil, req, true, 100)
	if err != nil {
		return nil, err
	}

	var resp OrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

// PlaceBatchOrders places multiple orders at once (max 10)
func (c *RESTClient) PlaceBatchOrders(ctx context.Context, orders []*OrderRequest) ([]*BatchOrderResponse, error) {
	body, err := c.doRequest(ctx, http.MethodPost, PathBatchOrders, nil, orders, true, 100)
	if err != nil {
		return nil, err
	}

	var result []*BatchOrderResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// GetOrder fetches order details by order ID
func (c *RESTClient) GetOrder(ctx context.Context, orderID string) (*Order, error) {
	path := buildPath(PathOrder, map[string]string{"orderId": orderID})

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var order Order
	if err := json.Unmarshal(body, &order); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &order, nil
}

// GetOrderByClientOid fetches order by client order ID
func (c *RESTClient) GetOrderByClientOid(ctx context.Context, clientOid string) (*Order, error) {
	params := url.Values{}
	params.Set("clientOid", clientOid)

	body, err := c.doRequest(ctx, http.MethodGet, PathOrderByClientOid, params, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var order Order
	if err := json.Unmarshal(body, &order); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &order, nil
}

// GetOrders fetches order list
func (c *RESTClient) GetOrders(ctx context.Context, symbol, status string, pageSize, currentPage int) (*OrderList, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}
	if status != "" {
		params.Set("status", status)
	}
	if pageSize > 0 {
		params.Set("pageSize", strconv.Itoa(pageSize))
	}
	if currentPage > 0 {
		params.Set("currentPage", strconv.Itoa(currentPage))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathOrders, params, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var result OrderList
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// CancelOrder cancels a single order by order ID
func (c *RESTClient) CancelOrder(ctx context.Context, orderID string) (*CancelResponse, error) {
	path := buildPath(PathOrder, map[string]string{"orderId": orderID})

	body, err := c.doRequest(ctx, http.MethodDelete, path, nil, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var resp CancelResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

// CancelOrderByClientOid cancels order by client order ID
func (c *RESTClient) CancelOrderByClientOid(ctx context.Context, clientOid, symbol string) (*CancelResponse, error) {
	params := url.Values{}
	params.Set("clientOid", clientOid)
	if symbol != "" {
		params.Set("symbol", symbol)
	}

	body, err := c.doRequest(ctx, http.MethodDelete, PathOrderByClientOid, params, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var resp CancelResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

// CancelAllOrders cancels all orders, optionally filtered by symbol
func (c *RESTClient) CancelAllOrders(ctx context.Context, symbol string) (*CancelResponse, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}

	body, err := c.doRequest(ctx, http.MethodDelete, PathCancelAllOrders, params, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var resp CancelResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

// =============================================================================
// Private Fill APIs
// =============================================================================

// GetFills fetches fill/trade history
func (c *RESTClient) GetFills(ctx context.Context, symbol, orderID string, pageSize, currentPage int) (*FillList, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}
	if orderID != "" {
		params.Set("orderId", orderID)
	}
	if pageSize > 0 {
		params.Set("pageSize", strconv.Itoa(pageSize))
	}
	if currentPage > 0 {
		params.Set("currentPage", strconv.Itoa(currentPage))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathFills, params, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var result FillList
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// GetRecentFills fetches recent fills (last 24 hours)
func (c *RESTClient) GetRecentFills(ctx context.Context, symbol string) ([]*Fill, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathRecentFills, params, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var fills []*Fill
	if err := json.Unmarshal(body, &fills); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return fills, nil
}

// =============================================================================
// Private Position APIs
// =============================================================================

// GetPositions fetches all positions
func (c *RESTClient) GetPositions(ctx context.Context, currency string) ([]*Position, error) {
	params := url.Values{}
	if currency != "" {
		params.Set("currency", currency)
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathPositions, params, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var positions []*Position
	if err := json.Unmarshal(body, &positions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return positions, nil
}

// GetPosition fetches a single position
func (c *RESTClient) GetPosition(ctx context.Context, symbol string) (*Position, error) {
	params := url.Values{}
	params.Set("symbol", symbol)

	body, err := c.doRequest(ctx, http.MethodGet, PathPosition, params, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var position Position
	if err := json.Unmarshal(body, &position); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &position, nil
}

// SetAutoDepositMargin enables/disables auto-deposit margin for a position
func (c *RESTClient) SetAutoDepositMargin(ctx context.Context, symbol string, status bool) (*Position, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("status", strconv.FormatBool(status))

	body, err := c.doRequest(ctx, http.MethodPost, PathPositionMarginAuto, params, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var position Position
	if err := json.Unmarshal(body, &position); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &position, nil
}

// AddPositionMargin adds margin to a position
func (c *RESTClient) AddPositionMargin(ctx context.Context, symbol string, margin float64, bizNo string) (*Position, error) {
	reqBody := map[string]interface{}{
		"symbol": symbol,
		"margin": margin,
		"bizNo":  bizNo,
	}

	body, err := c.doRequest(ctx, http.MethodPost, PathPositionMarginDeposit, nil, reqBody, true, 50)
	if err != nil {
		return nil, err
	}

	var position Position
	if err := json.Unmarshal(body, &position); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &position, nil
}

// =============================================================================
// Private Leverage & Margin APIs
// =============================================================================

// GetMaxOpenSize fetches maximum open size for a symbol
func (c *RESTClient) GetMaxOpenSize(ctx context.Context, symbol string, price float64, leverage int) (map[string]interface{}, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	if price > 0 {
		params.Set("price", strconv.FormatFloat(price, 'f', -1, 64))
	}
	if leverage > 0 {
		params.Set("leverage", strconv.Itoa(leverage))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathLeverageGet, params, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// ChangeLeverage changes leverage for cross margin
func (c *RESTClient) ChangeLeverage(ctx context.Context, symbol string, leverage int) error {
	reqBody := map[string]interface{}{
		"symbol":   symbol,
		"leverage": strconv.Itoa(leverage),
	}

	_, err := c.doRequest(ctx, http.MethodPost, PathLeverageChange, nil, reqBody, true, 50)
	return err
}

// ChangeMarginMode changes margin mode (isolated/cross)
func (c *RESTClient) ChangeMarginMode(ctx context.Context, symbol, marginMode string) error {
	reqBody := map[string]interface{}{
		"symbol":     symbol,
		"marginMode": marginMode,
	}

	_, err := c.doRequest(ctx, http.MethodPost, PathMarginMode, nil, reqBody, true, 50)
	return err
}

// =============================================================================
// Private Account APIs
// =============================================================================

// GetAccount fetches account overview
func (c *RESTClient) GetAccount(ctx context.Context, currency string) (*Account, error) {
	params := url.Values{}
	if currency != "" {
		params.Set("currency", currency)
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathAccountOverview, params, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var account Account
	if err := json.Unmarshal(body, &account); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &account, nil
}

// =============================================================================
// Private Funding APIs
// =============================================================================

// GetFundingHistory fetches funding fee history
func (c *RESTClient) GetFundingHistory(ctx context.Context, symbol string, startAt, endAt int64, pageSize, currentPage int) (map[string]interface{}, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}
	if startAt > 0 {
		params.Set("startAt", strconv.FormatInt(startAt, 10))
	}
	if endAt > 0 {
		params.Set("endAt", strconv.FormatInt(endAt, 10))
	}
	if pageSize > 0 {
		params.Set("pageSize", strconv.Itoa(pageSize))
	}
	if currentPage > 0 {
		params.Set("currentPage", strconv.Itoa(currentPage))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathFundingHistory, params, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// =============================================================================
// Stop Order APIs
// =============================================================================

// PlaceStopOrder places a stop order
func (c *RESTClient) PlaceStopOrder(ctx context.Context, req *OrderRequest) (*OrderResponse, error) {
	body, err := c.doRequest(ctx, http.MethodPost, PathStopOrders, nil, req, true, 100)
	if err != nil {
		return nil, err
	}

	var resp OrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

// GetStopOrders fetches stop order list
func (c *RESTClient) GetStopOrders(ctx context.Context, symbol string, pageSize, currentPage int) (*OrderList, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}
	if pageSize > 0 {
		params.Set("pageSize", strconv.Itoa(pageSize))
	}
	if currentPage > 0 {
		params.Set("currentPage", strconv.Itoa(currentPage))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathStopOrders, params, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var result OrderList
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// CancelStopOrder cancels a stop order
func (c *RESTClient) CancelStopOrder(ctx context.Context, orderID string) (*CancelResponse, error) {
	path := buildPath(PathStopOrder, map[string]string{"orderId": orderID})

	body, err := c.doRequest(ctx, http.MethodDelete, path, nil, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var resp CancelResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

// CancelAllStopOrders cancels all stop orders
func (c *RESTClient) CancelAllStopOrders(ctx context.Context, symbol string) (*CancelResponse, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}

	body, err := c.doRequest(ctx, http.MethodDelete, PathStopOrders, params, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var resp CancelResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}
