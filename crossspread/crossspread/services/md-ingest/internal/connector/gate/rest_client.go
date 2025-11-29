// Package gate provides REST API client for Gate.io exchange.
package gate

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
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
	BaseURLProduction = "https://api.gateio.ws"
	BaseURLFutures    = "https://api.gateio.ws"
	APIVersion        = "/api/v4"

	// Public endpoints - Futures Market Data
	PathFuturesContracts     = "/futures/{settle}/contracts"
	PathFuturesContract      = "/futures/{settle}/contracts/{contract}"
	PathFuturesTickers       = "/futures/{settle}/tickers"
	PathFuturesOrderBook     = "/futures/{settle}/order_book"
	PathFuturesTrades        = "/futures/{settle}/trades"
	PathFuturesCandlesticks  = "/futures/{settle}/candlesticks"
	PathFuturesFundingRate   = "/futures/{settle}/funding_rate"
	PathFuturesInsurance     = "/futures/{settle}/insurance"
	PathFuturesContractStats = "/futures/{settle}/contract_stats"
	PathFuturesLiqOrders     = "/futures/{settle}/liq_orders"
	PathFuturesRiskLimit     = "/futures/{settle}/risk_limit_tiers"
	PathFuturesIndexConst    = "/futures/{settle}/index_constituents/{index}"

	// Private endpoints - Futures Account
	PathFuturesAccounts    = "/futures/{settle}/accounts"
	PathFuturesAccountBook = "/futures/{settle}/account_book"

	// Private endpoints - Futures Position
	PathFuturesPositions         = "/futures/{settle}/positions"
	PathFuturesPosition          = "/futures/{settle}/positions/{contract}"
	PathFuturesPositionMargin    = "/futures/{settle}/positions/{contract}/margin"
	PathFuturesPositionLeverage  = "/futures/{settle}/positions/{contract}/leverage"
	PathFuturesPositionRiskLimit = "/futures/{settle}/positions/{contract}/risk_limit"
	PathFuturesDualMode          = "/futures/{settle}/dual_mode"
	PathFuturesDualPositions     = "/futures/{settle}/dual_comp/positions/{contract}"

	// Private endpoints - Futures Orders
	PathFuturesOrders          = "/futures/{settle}/orders"
	PathFuturesOrder           = "/futures/{settle}/orders/{order_id}"
	PathFuturesBatchOrders     = "/futures/{settle}/batch_orders"
	PathFuturesOrdersTimeRange = "/futures/{settle}/orders_timerange"
	PathFuturesMyTrades        = "/futures/{settle}/my_trades"
	PathFuturesMyTradesTime    = "/futures/{settle}/my_trades_timerange"
	PathFuturesPositionClose   = "/futures/{settle}/position_close"
	PathFuturesLiquidates      = "/futures/{settle}/liquidates"
	PathFuturesAutoDeleverages = "/futures/{settle}/auto_deleverages"
	PathFuturesCountdownCancel = "/futures/{settle}/countdown_cancel_all"
	PathFuturesPriceOrders     = "/futures/{settle}/price_orders"
	PathFuturesFee             = "/futures/{settle}/fee"

	// Wallet endpoints
	PathWalletFee            = "/wallet/fee"
	PathWalletCurrencyChains = "/wallet/currency_chains"
	PathWalletWithdrawStatus = "/wallet/withdraw_status"
	PathWalletDepositAddress = "/wallet/deposit_address"
	PathWalletDeposits       = "/wallet/deposits"
	PathWalletWithdrawals    = "/wallet/withdrawals"
	PathWalletTotalBalance   = "/wallet/total_balance"

	// Spot endpoints (for currency info)
	PathSpotCurrencies = "/spot/currencies"
	PathSpotCurrency   = "/spot/currencies/{currency}"
)

// RESTClient provides methods to interact with Gate.io REST API
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

// NewRESTClient creates a new Gate.io REST client
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

// sign generates HMAC-SHA512 signature for Gate.io API
// signature = HMAC-SHA512(sign_string, secret_key)
// sign_string = request_method + "\n" + request_path + "\n" + query_string + "\n" + body_hash + "\n" + timestamp
func (c *RESTClient) sign(method, path, queryString, bodyHash string, timestamp int64) string {
	signString := fmt.Sprintf("%s\n%s\n%s\n%s\n%d", method, path, queryString, bodyHash, timestamp)
	h := hmac.New(sha512.New, []byte(c.secretKey))
	h.Write([]byte(signString))
	return hex.EncodeToString(h.Sum(nil))
}

// hashBody creates SHA512 hash of request body
func (c *RESTClient) hashBody(body []byte) string {
	if len(body) == 0 {
		body = []byte("")
	}
	h := sha512.New()
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// getTimestamp returns current timestamp in seconds
func (c *RESTClient) getTimestamp() int64 {
	return time.Now().Unix()
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

	// Build URL with API version
	fullPath := APIVersion + path
	fullURL := c.baseURL + fullPath

	// Build query string
	queryString := ""
	if len(params) > 0 {
		queryString = params.Encode()
		fullURL += "?" + queryString
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
		bodyHash := c.hashBody(bodyBytes)
		signature := c.sign(method, fullPath, queryString, bodyHash, timestamp)

		req.Header.Set("KEY", c.apiKey)
		req.Header.Set("Timestamp", strconv.FormatInt(timestamp, 10))
		req.Header.Set("SIGN", signature)
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
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var apiErr APIError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Label != "" {
			return nil, &apiErr
		}
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// =============================================================================
// Public Market Data APIs - Futures
// =============================================================================

// GetContracts fetches all futures contracts for a settlement currency
func (c *RESTClient) GetContracts(ctx context.Context, settle string) ([]Contract, error) {
	path := buildPath(PathFuturesContracts, map[string]string{"settle": settle})

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var contracts []Contract
	if err := json.Unmarshal(body, &contracts); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return contracts, nil
}

// GetContract fetches a single futures contract
func (c *RESTClient) GetContract(ctx context.Context, settle, contract string) (*Contract, error) {
	path := buildPath(PathFuturesContract, map[string]string{
		"settle":   settle,
		"contract": contract,
	})

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var result Contract
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// GetTickers fetches all ticker data for a settlement currency
func (c *RESTClient) GetTickers(ctx context.Context, settle string, contract string) ([]Ticker, error) {
	path := buildPath(PathFuturesTickers, map[string]string{"settle": settle})

	params := url.Values{}
	if contract != "" {
		params.Set("contract", contract)
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, params, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var tickers []Ticker
	if err := json.Unmarshal(body, &tickers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return tickers, nil
}

// GetOrderBook fetches order book depth
func (c *RESTClient) GetOrderBook(ctx context.Context, settle, contract string, interval string, limit int, withID bool) (*OrderBook, error) {
	path := buildPath(PathFuturesOrderBook, map[string]string{"settle": settle})

	params := url.Values{}
	params.Set("contract", contract)
	if interval != "" {
		params.Set("interval", interval)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if withID {
		params.Set("with_id", "true")
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, params, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var orderbook OrderBook
	if err := json.Unmarshal(body, &orderbook); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &orderbook, nil
}

// GetTrades fetches recent trades
func (c *RESTClient) GetTrades(ctx context.Context, settle, contract string, limit int, from, to int64) ([]Trade, error) {
	path := buildPath(PathFuturesTrades, map[string]string{"settle": settle})

	params := url.Values{}
	params.Set("contract", contract)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if from > 0 {
		params.Set("from", strconv.FormatInt(from, 10))
	}
	if to > 0 {
		params.Set("to", strconv.FormatInt(to, 10))
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, params, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var trades []Trade
	if err := json.Unmarshal(body, &trades); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return trades, nil
}

// GetCandlesticks fetches candlestick data
// Use prefix "mark_" or "index_" for mark/index price candlesticks
func (c *RESTClient) GetCandlesticks(ctx context.Context, settle, contract, interval string, from, to int64, limit int) ([]Candlestick, error) {
	path := buildPath(PathFuturesCandlesticks, map[string]string{"settle": settle})

	params := url.Values{}
	params.Set("contract", contract)
	if interval != "" {
		params.Set("interval", interval)
	}
	if from > 0 {
		params.Set("from", strconv.FormatInt(from, 10))
	}
	if to > 0 {
		params.Set("to", strconv.FormatInt(to, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, params, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var candlesticks []Candlestick
	if err := json.Unmarshal(body, &candlesticks); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return candlesticks, nil
}

// GetFundingRateHistory fetches historical funding rates
func (c *RESTClient) GetFundingRateHistory(ctx context.Context, settle, contract string, limit int, from, to int64) ([]FundingRateHistory, error) {
	path := buildPath(PathFuturesFundingRate, map[string]string{"settle": settle})

	params := url.Values{}
	params.Set("contract", contract)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if from > 0 {
		params.Set("from", strconv.FormatInt(from, 10))
	}
	if to > 0 {
		params.Set("to", strconv.FormatInt(to, 10))
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, params, nil, false, 100)
	if err != nil {
		return nil, err
	}

	var fundingRates []FundingRateHistory
	if err := json.Unmarshal(body, &fundingRates); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return fundingRates, nil
}

// =============================================================================
// Private Account APIs - Futures
// =============================================================================

// GetAccount fetches futures account balance
func (c *RESTClient) GetAccount(ctx context.Context, settle string) (*FuturesAccount, error) {
	path := buildPath(PathFuturesAccounts, map[string]string{"settle": settle})

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var account FuturesAccount
	if err := json.Unmarshal(body, &account); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &account, nil
}

// GetPositions fetches all positions
func (c *RESTClient) GetPositions(ctx context.Context, settle string, holding bool) ([]Position, error) {
	path := buildPath(PathFuturesPositions, map[string]string{"settle": settle})

	params := url.Values{}
	if holding {
		params.Set("holding", "true")
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, params, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var positions []Position
	if err := json.Unmarshal(body, &positions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return positions, nil
}

// GetPosition fetches a single position
func (c *RESTClient) GetPosition(ctx context.Context, settle, contract string) (*Position, error) {
	path := buildPath(PathFuturesPosition, map[string]string{
		"settle":   settle,
		"contract": contract,
	})

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var position Position
	if err := json.Unmarshal(body, &position); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &position, nil
}

// UpdatePositionMargin updates position margin
func (c *RESTClient) UpdatePositionMargin(ctx context.Context, settle, contract, change string) (*Position, error) {
	path := buildPath(PathFuturesPositionMargin, map[string]string{
		"settle":   settle,
		"contract": contract,
	})

	params := url.Values{}
	params.Set("change", change)

	body, err := c.doRequest(ctx, http.MethodPost, path, params, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var position Position
	if err := json.Unmarshal(body, &position); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &position, nil
}

// UpdatePositionLeverage updates position leverage
func (c *RESTClient) UpdatePositionLeverage(ctx context.Context, settle, contract, leverage string, crossLeverageLimit string) (*Position, error) {
	path := buildPath(PathFuturesPositionLeverage, map[string]string{
		"settle":   settle,
		"contract": contract,
	})

	params := url.Values{}
	params.Set("leverage", leverage)
	if crossLeverageLimit != "" {
		params.Set("cross_leverage_limit", crossLeverageLimit)
	}

	body, err := c.doRequest(ctx, http.MethodPost, path, params, nil, true, 50)
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
// Private Order APIs - Futures
// =============================================================================

// PlaceOrder places a futures order
func (c *RESTClient) PlaceOrder(ctx context.Context, settle string, req *OrderRequest) (*Order, error) {
	path := buildPath(PathFuturesOrders, map[string]string{"settle": settle})

	body, err := c.doRequest(ctx, http.MethodPost, path, nil, req, true, 100)
	if err != nil {
		return nil, err
	}

	var order Order
	if err := json.Unmarshal(body, &order); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &order, nil
}

// PlaceBatchOrders places multiple orders at once
func (c *RESTClient) PlaceBatchOrders(ctx context.Context, settle string, orders []OrderRequest) ([]Order, error) {
	path := buildPath(PathFuturesBatchOrders, map[string]string{"settle": settle})

	body, err := c.doRequest(ctx, http.MethodPost, path, nil, orders, true, 100)
	if err != nil {
		return nil, err
	}

	var result []Order
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// GetOrders fetches order list
func (c *RESTClient) GetOrders(ctx context.Context, settle, contract, status string, limit, offset int, lastID string) ([]Order, error) {
	path := buildPath(PathFuturesOrders, map[string]string{"settle": settle})

	params := url.Values{}
	params.Set("status", status)
	if contract != "" {
		params.Set("contract", contract)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}
	if lastID != "" {
		params.Set("last_id", lastID)
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, params, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var orders []Order
	if err := json.Unmarshal(body, &orders); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return orders, nil
}

// GetOrder fetches a single order
func (c *RESTClient) GetOrder(ctx context.Context, settle, orderID string) (*Order, error) {
	path := buildPath(PathFuturesOrder, map[string]string{
		"settle":   settle,
		"order_id": orderID,
	})

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

// CancelOrder cancels a single order
func (c *RESTClient) CancelOrder(ctx context.Context, settle, orderID string) (*Order, error) {
	path := buildPath(PathFuturesOrder, map[string]string{
		"settle":   settle,
		"order_id": orderID,
	})

	body, err := c.doRequest(ctx, http.MethodDelete, path, nil, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var order Order
	if err := json.Unmarshal(body, &order); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &order, nil
}

// CancelAllOrders cancels all orders for a contract
func (c *RESTClient) CancelAllOrders(ctx context.Context, settle, contract, side string) ([]Order, error) {
	path := buildPath(PathFuturesOrders, map[string]string{"settle": settle})

	params := url.Values{}
	params.Set("contract", contract)
	if side != "" {
		params.Set("side", side)
	}

	body, err := c.doRequest(ctx, http.MethodDelete, path, params, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var orders []Order
	if err := json.Unmarshal(body, &orders); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return orders, nil
}

// AmendOrder amends an existing order
func (c *RESTClient) AmendOrder(ctx context.Context, settle, orderID string, req *OrderAmendRequest) (*Order, error) {
	path := buildPath(PathFuturesOrder, map[string]string{
		"settle":   settle,
		"order_id": orderID,
	})

	body, err := c.doRequest(ctx, http.MethodPut, path, nil, req, true, 100)
	if err != nil {
		return nil, err
	}

	var order Order
	if err := json.Unmarshal(body, &order); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &order, nil
}

// GetMyTrades fetches user's trade history
func (c *RESTClient) GetMyTrades(ctx context.Context, settle, contract string, orderID string, limit, offset int, lastID string) ([]UserTrade, error) {
	path := buildPath(PathFuturesMyTrades, map[string]string{"settle": settle})

	params := url.Values{}
	if contract != "" {
		params.Set("contract", contract)
	}
	if orderID != "" {
		params.Set("order", orderID)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}
	if lastID != "" {
		params.Set("last_id", lastID)
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, params, nil, true, 100)
	if err != nil {
		return nil, err
	}

	var trades []UserTrade
	if err := json.Unmarshal(body, &trades); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return trades, nil
}

// =============================================================================
// Fee APIs
// =============================================================================

// GetTradingFee fetches futures trading fee rates
func (c *RESTClient) GetTradingFee(ctx context.Context, settle, contract string) (*TradingFee, error) {
	path := buildPath(PathFuturesFee, map[string]string{"settle": settle})

	params := url.Values{}
	if contract != "" {
		params.Set("contract", contract)
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, params, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var fee TradingFee
	if err := json.Unmarshal(body, &fee); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &fee, nil
}

// GetWalletFee fetches comprehensive fee info
func (c *RESTClient) GetWalletFee(ctx context.Context) (*WalletFee, error) {
	body, err := c.doRequest(ctx, http.MethodGet, PathWalletFee, nil, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var fee WalletFee
	if err := json.Unmarshal(body, &fee); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &fee, nil
}

// =============================================================================
// Currency/Chain APIs
// =============================================================================

// GetCurrencies fetches all currency info
func (c *RESTClient) GetCurrencies(ctx context.Context) ([]Currency, error) {
	body, err := c.doRequest(ctx, http.MethodGet, PathSpotCurrencies, nil, nil, false, 50)
	if err != nil {
		return nil, err
	}

	var currencies []Currency
	if err := json.Unmarshal(body, &currencies); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return currencies, nil
}

// GetCurrency fetches a single currency info
func (c *RESTClient) GetCurrency(ctx context.Context, currency string) (*Currency, error) {
	path := buildPath(PathSpotCurrency, map[string]string{"currency": currency})

	body, err := c.doRequest(ctx, http.MethodGet, path, nil, nil, false, 50)
	if err != nil {
		return nil, err
	}

	var result Currency
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// GetWithdrawStatus fetches withdrawal status for currencies
func (c *RESTClient) GetWithdrawStatus(ctx context.Context, currency string) ([]WithdrawStatus, error) {
	params := url.Values{}
	if currency != "" {
		params.Set("currency", currency)
	}

	body, err := c.doRequest(ctx, http.MethodGet, PathWalletWithdrawStatus, params, nil, true, 50)
	if err != nil {
		return nil, err
	}

	var status []WithdrawStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return status, nil
}

// CountdownCancelAll sets countdown to cancel all orders
func (c *RESTClient) CountdownCancelAll(ctx context.Context, settle string, timeout int, contract string) error {
	path := buildPath(PathFuturesCountdownCancel, map[string]string{"settle": settle})

	reqBody := map[string]interface{}{
		"timeout": timeout,
	}
	if contract != "" {
		reqBody["contract"] = contract
	}

	_, err := c.doRequest(ctx, http.MethodPost, path, nil, reqBody, true, 50)
	return err
}
