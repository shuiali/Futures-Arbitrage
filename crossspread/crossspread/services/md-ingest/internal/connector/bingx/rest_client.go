// Package bingx provides REST API client for BingX Perpetual Futures exchange.
package bingx

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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// REST API endpoints
const (
	// Public endpoints - Market Data
	PathContracts       = "/openApi/swap/v2/quote/contracts"
	PathPrice           = "/openApi/swap/v2/quote/price"
	PathDepth           = "/openApi/swap/v2/quote/depth"
	PathTrades          = "/openApi/swap/v2/quote/trades"
	PathKlines          = "/openApi/swap/v2/quote/klines"
	PathPremiumIndex    = "/openApi/swap/v2/quote/premiumIndex"
	PathFundingRateHist = "/openApi/swap/v2/quote/fundingRate"
	PathTicker          = "/openApi/swap/v2/quote/ticker"
	PathOpenInterest    = "/openApi/swap/v2/quote/openInterest"

	// Private endpoints - Account
	PathBalance        = "/openApi/swap/v2/user/balance"
	PathPositions      = "/openApi/swap/v2/user/positions"
	PathIncome         = "/openApi/swap/v2/user/income"
	PathCommissionRate = "/openApi/swap/v2/user/commissionRate"

	// Private endpoints - Trading
	PathPlaceOrder        = "/openApi/swap/v2/trade/order"
	PathBatchOrders       = "/openApi/swap/v2/trade/batchOrders"
	PathCancelOrder       = "/openApi/swap/v2/trade/order"
	PathCancelBatchOrders = "/openApi/swap/v2/trade/batchOrders"
	PathCancelAllOrders   = "/openApi/swap/v2/trade/allOpenOrders"
	PathCloseAllPositions = "/openApi/swap/v2/trade/closeAllPositions"
	PathQueryOrder        = "/openApi/swap/v2/trade/order"
	PathOpenOrders        = "/openApi/swap/v2/trade/openOrders"
	PathAllOrders         = "/openApi/swap/v2/trade/allOrders"
	PathAllFillOrders     = "/openApi/swap/v2/trade/allFillOrders"
	PathLeverage          = "/openApi/swap/v2/trade/leverage"
	PathMarginType        = "/openApi/swap/v2/trade/marginType"
	PathPositionMargin    = "/openApi/swap/v2/trade/positionMargin"

	// Listen Key endpoints
	PathListenKey = "/openApi/user/auth/userDataStream"

	// Wallet endpoints (for deposit/withdraw status)
	PathAssetConfig = "/openApi/wallets/v1/capital/config/getall"
)

// RESTClient provides methods to interact with BingX REST API
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

// NewRESTClient creates a new BingX REST client
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

// sign generates HMAC-SHA256 signature for BingX API
// Signature is generated from sorted query parameters
func (c *RESTClient) sign(params url.Values) string {
	// Sort parameters alphabetically
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build query string
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(params.Get(k))
	}
	queryString := sb.String()

	// Generate HMAC-SHA256 signature
	h := hmac.New(sha256.New, []byte(c.secretKey))
	h.Write([]byte(queryString))
	return hex.EncodeToString(h.Sum(nil))
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

	// Ensure params is initialized
	if params == nil {
		params = url.Values{}
	}

	// Add authentication parameters
	if authenticated {
		params.Set("timestamp", c.getTimestamp())
		signature := c.sign(params)
		params.Set("signature", signature)
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

	// Add API key header for authenticated requests
	if authenticated {
		req.Header.Set("X-BX-APIKEY", c.apiKey)
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

// GetContracts fetches all perpetual swap contracts
func (c *RESTClient) GetContracts(ctx context.Context) ([]*Contract, error) {
	body, err := c.doRequest(ctx, http.MethodGet, PathContracts, nil, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var contracts []*Contract
	if err := json.Unmarshal(body, &contracts); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return contracts, nil
}

// GetPrice fetches latest price for symbol(s)
// If symbol is empty, returns prices for all symbols
func (c *RESTClient) GetPrice(ctx context.Context, symbol string) ([]*Price, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathPrice, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var prices []*Price
	if err := json.Unmarshal(body, &prices); err != nil {
		// Try single price
		var price Price
		if err := json.Unmarshal(body, &price); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}
		return []*Price{&price}, nil
	}

	return prices, nil
}

// GetOrderBook fetches order book depth for a symbol
func (c *RESTClient) GetOrderBook(ctx context.Context, symbol string, limit int) (*OrderBook, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathDepth, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var orderBook OrderBook
	if err := json.Unmarshal(body, &orderBook); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	orderBook.Symbol = symbol

	return &orderBook, nil
}

// GetTrades fetches recent trades for a symbol
func (c *RESTClient) GetTrades(ctx context.Context, symbol string, limit int) ([]*Trade, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathTrades, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var trades []*Trade
	if err := json.Unmarshal(body, &trades); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return trades, nil
}

// GetKlines fetches candlestick data
func (c *RESTClient) GetKlines(ctx context.Context, symbol, interval string, startTime, endTime int64, limit int) ([]*Kline, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("interval", interval)
	if startTime > 0 {
		params.Set("startTime", strconv.FormatInt(startTime, 10))
	}
	if endTime > 0 {
		params.Set("endTime", strconv.FormatInt(endTime, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathKlines, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var klines []*Kline
	if err := json.Unmarshal(body, &klines); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return klines, nil
}

// GetPremiumIndex fetches mark price and funding rate
// If symbol is empty, returns for all symbols
func (c *RESTClient) GetPremiumIndex(ctx context.Context, symbol string) ([]*PremiumIndex, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathPremiumIndex, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var indices []*PremiumIndex
	if err := json.Unmarshal(body, &indices); err != nil {
		// Try single
		var index PremiumIndex
		if err := json.Unmarshal(body, &index); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}
		return []*PremiumIndex{&index}, nil
	}

	return indices, nil
}

// GetFundingRateHistory fetches historical funding rates
func (c *RESTClient) GetFundingRateHistory(ctx context.Context, symbol string, startTime, endTime int64, limit int) ([]*FundingRateHistory, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	if startTime > 0 {
		params.Set("startTime", strconv.FormatInt(startTime, 10))
	}
	if endTime > 0 {
		params.Set("endTime", strconv.FormatInt(endTime, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathFundingRateHist, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var rates []*FundingRateHistory
	if err := json.Unmarshal(body, &rates); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return rates, nil
}

// GetTicker fetches 24hr price ticker
// If symbol is empty, returns for all symbols
func (c *RESTClient) GetTicker(ctx context.Context, symbol string) ([]*Ticker, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathTicker, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var tickers []*Ticker
	if err := json.Unmarshal(body, &tickers); err != nil {
		// Try single
		var ticker Ticker
		if err := json.Unmarshal(body, &ticker); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}
		return []*Ticker{&ticker}, nil
	}

	return tickers, nil
}

// GetOpenInterest fetches open interest for a symbol
func (c *RESTClient) GetOpenInterest(ctx context.Context, symbol string) (*OpenInterest, error) {
	params := url.Values{}
	params.Set("symbol", symbol)

	body, err := c.doRequest(ctx, http.MethodGet, PathOpenInterest, params, nil, false, 20)
	if err != nil {
		return nil, err
	}

	var oi OpenInterest
	if err := json.Unmarshal(body, &oi); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &oi, nil
}

// =============================================================================
// Private Account APIs
// =============================================================================

// GetBalance fetches account balance
func (c *RESTClient) GetBalance(ctx context.Context) (*AccountBalance, error) {
	params := url.Values{}

	body, err := c.doRequest(ctx, http.MethodGet, PathBalance, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var balance AccountBalance
	if err := json.Unmarshal(body, &balance); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &balance, nil
}

// GetPositions fetches all positions or for a specific symbol
func (c *RESTClient) GetPositions(ctx context.Context, symbol string) ([]*Position, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathPositions, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var positions []*Position
	if err := json.Unmarshal(body, &positions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return positions, nil
}

// GetIncome fetches income history
func (c *RESTClient) GetIncome(ctx context.Context, symbol, incomeType string, startTime, endTime int64, limit int) ([]*Income, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}
	if incomeType != "" {
		params.Set("incomeType", incomeType)
	}
	if startTime > 0 {
		params.Set("startTime", strconv.FormatInt(startTime, 10))
	}
	if endTime > 0 {
		params.Set("endTime", strconv.FormatInt(endTime, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathIncome, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var income []*Income
	if err := json.Unmarshal(body, &income); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return income, nil
}

// GetCommissionRate fetches commission rate for a symbol
func (c *RESTClient) GetCommissionRate(ctx context.Context, symbol string) (*CommissionRate, error) {
	params := url.Values{}
	params.Set("symbol", symbol)

	body, err := c.doRequest(ctx, http.MethodGet, PathCommissionRate, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var rate CommissionRate
	if err := json.Unmarshal(body, &rate); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &rate, nil
}

// =============================================================================
// Private Trading APIs
// =============================================================================

// PlaceOrder places a new order
func (c *RESTClient) PlaceOrder(ctx context.Context, req *OrderRequest) (*OrderResponse, error) {
	params := url.Values{}
	params.Set("symbol", req.Symbol)
	params.Set("type", req.Type)
	params.Set("side", req.Side)

	if req.PositionSide != "" {
		params.Set("positionSide", req.PositionSide)
	}
	if req.Price > 0 {
		params.Set("price", strconv.FormatFloat(req.Price, 'f', -1, 64))
	}
	if req.Quantity > 0 {
		params.Set("quantity", strconv.FormatFloat(req.Quantity, 'f', -1, 64))
	}
	if req.StopPrice > 0 {
		params.Set("stopPrice", strconv.FormatFloat(req.StopPrice, 'f', -1, 64))
	}
	if req.ClientOrderID != "" {
		params.Set("clientOrderID", req.ClientOrderID)
	}
	if req.TimeInForce != "" {
		params.Set("timeInForce", req.TimeInForce)
	}
	if req.ReduceOnly {
		params.Set("reduceOnly", "true")
	}

	body, err := c.doRequest(ctx, http.MethodPost, PathPlaceOrder, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp OrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

// PlaceBatchOrders places multiple orders (max 5)
func (c *RESTClient) PlaceBatchOrders(ctx context.Context, orders []*OrderRequest) ([]*OrderResponse, error) {
	if len(orders) > 5 {
		return nil, fmt.Errorf("maximum 5 orders allowed per batch")
	}

	params := url.Values{}
	ordersJSON, err := json.Marshal(orders)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal orders: %w", err)
	}
	params.Set("batchOrders", string(ordersJSON))

	body, err := c.doRequest(ctx, http.MethodPost, PathBatchOrders, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp []*OrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return resp, nil
}

// CancelOrder cancels an existing order
func (c *RESTClient) CancelOrder(ctx context.Context, symbol string, orderID int64, clientOrderID string) (*CancelResponse, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	if orderID > 0 {
		params.Set("orderId", strconv.FormatInt(orderID, 10))
	}
	if clientOrderID != "" {
		params.Set("clientOrderId", clientOrderID)
	}

	body, err := c.doRequest(ctx, http.MethodDelete, PathCancelOrder, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp CancelResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

// CancelBatchOrders cancels multiple orders
func (c *RESTClient) CancelBatchOrders(ctx context.Context, symbol string, orderIDs []int64, clientOrderIDs []string) ([]*CancelResponse, error) {
	params := url.Values{}
	params.Set("symbol", symbol)

	if len(orderIDs) > 0 {
		ids, _ := json.Marshal(orderIDs)
		params.Set("orderIdList", string(ids))
	}
	if len(clientOrderIDs) > 0 {
		ids, _ := json.Marshal(clientOrderIDs)
		params.Set("clientOrderIDList", string(ids))
	}

	body, err := c.doRequest(ctx, http.MethodDelete, PathCancelBatchOrders, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var resp []*CancelResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return resp, nil
}

// CancelAllOrders cancels all open orders for a symbol
func (c *RESTClient) CancelAllOrders(ctx context.Context, symbol string) error {
	params := url.Values{}
	params.Set("symbol", symbol)

	_, err := c.doRequest(ctx, http.MethodDelete, PathCancelAllOrders, params, nil, true, 10)
	return err
}

// CloseAllPositions closes all positions
func (c *RESTClient) CloseAllPositions(ctx context.Context) error {
	params := url.Values{}

	_, err := c.doRequest(ctx, http.MethodPost, PathCloseAllPositions, params, nil, true, 10)
	return err
}

// QueryOrder queries a specific order
func (c *RESTClient) QueryOrder(ctx context.Context, symbol string, orderID int64, clientOrderID string) (*Order, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	if orderID > 0 {
		params.Set("orderId", strconv.FormatInt(orderID, 10))
	}
	if clientOrderID != "" {
		params.Set("clientOrderId", clientOrderID)
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathQueryOrder, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var order Order
	if err := json.Unmarshal(body, &order); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &order, nil
}

// GetOpenOrders fetches all open orders
func (c *RESTClient) GetOpenOrders(ctx context.Context, symbol string) ([]*Order, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathOpenOrders, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var orders []*Order
	if err := json.Unmarshal(body, &orders); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return orders, nil
}

// GetAllOrders fetches order history
func (c *RESTClient) GetAllOrders(ctx context.Context, symbol string, orderID int64, startTime, endTime int64, limit int) ([]*Order, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	if orderID > 0 {
		params.Set("orderId", strconv.FormatInt(orderID, 10))
	}
	if startTime > 0 {
		params.Set("startTime", strconv.FormatInt(startTime, 10))
	}
	if endTime > 0 {
		params.Set("endTime", strconv.FormatInt(endTime, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathAllOrders, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var orders []*Order
	if err := json.Unmarshal(body, &orders); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return orders, nil
}

// GetAllFills fetches trade fill history
func (c *RESTClient) GetAllFills(ctx context.Context, symbol string, orderID int64, startTime, endTime int64) ([]*Fill, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	if orderID > 0 {
		params.Set("orderId", strconv.FormatInt(orderID, 10))
	}
	if startTime > 0 {
		params.Set("startTs", strconv.FormatInt(startTime, 10))
	}
	if endTime > 0 {
		params.Set("endTs", strconv.FormatInt(endTime, 10))
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathAllFillOrders, params, nil, true, 10)
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
// Leverage & Margin APIs
// =============================================================================

// GetLeverage fetches current leverage for a symbol
func (c *RESTClient) GetLeverage(ctx context.Context, symbol string) (*Leverage, error) {
	params := url.Values{}
	params.Set("symbol", symbol)

	body, err := c.doRequest(ctx, http.MethodGet, PathLeverage, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var leverage Leverage
	if err := json.Unmarshal(body, &leverage); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &leverage, nil
}

// SetLeverage sets leverage for a symbol
func (c *RESTClient) SetLeverage(ctx context.Context, symbol, side string, leverage int) error {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", side)
	params.Set("leverage", strconv.Itoa(leverage))

	_, err := c.doRequest(ctx, http.MethodPost, PathLeverage, params, nil, true, 10)
	return err
}

// GetMarginType fetches margin type for a symbol
func (c *RESTClient) GetMarginType(ctx context.Context, symbol string) (*MarginType, error) {
	params := url.Values{}
	params.Set("symbol", symbol)

	body, err := c.doRequest(ctx, http.MethodGet, PathMarginType, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var marginType MarginType
	if err := json.Unmarshal(body, &marginType); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &marginType, nil
}

// SetMarginType sets margin type for a symbol
func (c *RESTClient) SetMarginType(ctx context.Context, symbol, marginType string) error {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("marginType", marginType)

	_, err := c.doRequest(ctx, http.MethodPost, PathMarginType, params, nil, true, 10)
	return err
}

// AdjustPositionMargin adjusts position margin
func (c *RESTClient) AdjustPositionMargin(ctx context.Context, symbol string, amount float64, adjustType int, positionSide string) error {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))
	params.Set("type", strconv.Itoa(adjustType)) // 1=Add, 2=Reduce
	if positionSide != "" {
		params.Set("positionSide", positionSide)
	}

	_, err := c.doRequest(ctx, http.MethodPost, PathPositionMargin, params, nil, true, 10)
	return err
}

// =============================================================================
// Listen Key APIs (for WebSocket User Data Stream)
// =============================================================================

// CreateListenKey creates a new listen key for user data stream
func (c *RESTClient) CreateListenKey(ctx context.Context) (*ListenKey, error) {
	// For listen key, only X-BX-APIKEY header is needed, no signature
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+PathListenKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-BX-APIKEY", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp Response
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !apiResp.IsSuccess() {
		return nil, apiResp.Error()
	}

	var listenKey ListenKey
	if err := json.Unmarshal(apiResp.Data, &listenKey); err != nil {
		return nil, fmt.Errorf("failed to unmarshal listen key: %w", err)
	}

	return &listenKey, nil
}

// ExtendListenKey extends listen key validity (call every 30 minutes)
func (c *RESTClient) ExtendListenKey(ctx context.Context, listenKey string) error {
	params := url.Values{}
	params.Set("listenKey", listenKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+PathListenKey+"?"+params.Encode(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-BX-APIKEY", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to extend listen key: %s", string(respBody))
	}

	return nil
}

// DeleteListenKey deletes a listen key
func (c *RESTClient) DeleteListenKey(ctx context.Context, listenKey string) error {
	params := url.Values{}
	params.Set("listenKey", listenKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+PathListenKey+"?"+params.Encode(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-BX-APIKEY", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete listen key: %s", string(respBody))
	}

	return nil
}

// =============================================================================
// Wallet APIs
// =============================================================================

// GetAssetConfig fetches deposit/withdraw configuration for all assets
func (c *RESTClient) GetAssetConfig(ctx context.Context) ([]*AssetConfig, error) {
	params := url.Values{}

	body, err := c.doRequest(ctx, http.MethodGet, PathAssetConfig, params, nil, true, 10)
	if err != nil {
		return nil, err
	}

	var configs []*AssetConfig
	if err := json.Unmarshal(body, &configs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return configs, nil
}
