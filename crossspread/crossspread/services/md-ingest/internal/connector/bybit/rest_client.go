package bybit

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
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// Base URLs
	BaseURLMainnet = "https://api.bybit.com"
	BaseURLTestnet = "https://api-testnet.bybit.com"

	// API Endpoints
	EndpointInstruments    = "/v5/market/instruments-info"
	EndpointTickers        = "/v5/market/tickers"
	EndpointOrderbook      = "/v5/market/orderbook"
	EndpointKline          = "/v5/market/kline"
	EndpointFundingHistory = "/v5/market/funding/history"
	EndpointRecentTrades   = "/v5/market/recent-trade"
	EndpointOpenInterest   = "/v5/market/open-interest"
	EndpointRiskLimit      = "/v5/market/risk-limit"

	EndpointCreateOrder      = "/v5/order/create"
	EndpointAmendOrder       = "/v5/order/amend"
	EndpointCancelOrder      = "/v5/order/cancel"
	EndpointCancelAllOrders  = "/v5/order/cancel-all"
	EndpointBatchCreateOrder = "/v5/order/create-batch"
	EndpointGetOrders        = "/v5/order/realtime"
	EndpointOrderHistory     = "/v5/order/history"
	EndpointExecutions       = "/v5/execution/list"

	EndpointPositions   = "/v5/position/list"
	EndpointSetLeverage = "/v5/position/set-leverage"
	EndpointSwitchMode  = "/v5/position/switch-mode"
	EndpointClosedPnl   = "/v5/position/closed-pnl"

	EndpointWalletBalance = "/v5/account/wallet-balance"
	EndpointFeeRate       = "/v5/account/fee-rate"
	EndpointAccountInfo   = "/v5/account/info"

	EndpointCoinInfo        = "/v5/asset/coin/query-info"
	EndpointDepositRecords  = "/v5/asset/deposit/query-record"
	EndpointWithdrawRecords = "/v5/asset/withdraw/query-record"
)

// RESTClient is the Bybit REST API client with authentication support
type RESTClient struct {
	baseURL    string
	apiKey     string
	apiSecret  string
	httpClient *http.Client
	recvWindow string
}

// RESTClientConfig holds configuration for the REST client
type RESTClientConfig struct {
	BaseURL    string
	APIKey     string
	APISecret  string
	Timeout    time.Duration
	RecvWindow int64 // milliseconds, default 5000
}

// NewRESTClient creates a new Bybit REST API client
func NewRESTClient(config RESTClientConfig) *RESTClient {
	if config.BaseURL == "" {
		config.BaseURL = BaseURLMainnet
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.RecvWindow == 0 {
		config.RecvWindow = 5000
	}

	return &RESTClient{
		baseURL:   config.BaseURL,
		apiKey:    config.APIKey,
		apiSecret: config.APISecret,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		recvWindow: strconv.FormatInt(config.RecvWindow, 10),
	}
}

// generateSignature creates HMAC SHA256 signature for authenticated requests
// signature = HMAC_SHA256(api_secret, timestamp + api_key + recv_window + query_string_or_body)
func (c *RESTClient) generateSignature(timestamp string, payload string) string {
	data := timestamp + c.apiKey + c.recvWindow + payload
	h := hmac.New(sha256.New, []byte(c.apiSecret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// doRequest executes an HTTP request with optional authentication
func (c *RESTClient) doRequest(ctx context.Context, method, endpoint string, params map[string]string, body interface{}, authenticated bool) ([]byte, error) {
	var reqURL string
	var reqBody io.Reader
	var payload string

	// Build query string for GET requests or body for POST requests
	if method == http.MethodGet && params != nil {
		queryParams := url.Values{}
		// Sort keys for consistent signature
		keys := make([]string, 0, len(params))
		for k := range params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			queryParams.Add(k, params[k])
		}
		payload = queryParams.Encode()
		reqURL = fmt.Sprintf("%s%s?%s", c.baseURL, endpoint, payload)
	} else {
		reqURL = fmt.Sprintf("%s%s", c.baseURL, endpoint)
		if body != nil {
			bodyBytes, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			payload = string(bodyBytes)
			reqBody = bytes.NewReader(bodyBytes)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add authentication headers if required
	if authenticated && c.apiKey != "" && c.apiSecret != "" {
		timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
		signature := c.generateSignature(timestamp, payload)

		req.Header.Set("X-BAPI-API-KEY", c.apiKey)
		req.Header.Set("X-BAPI-TIMESTAMP", timestamp)
		req.Header.Set("X-BAPI-SIGN", signature)
		req.Header.Set("X-BAPI-RECV-WINDOW", c.recvWindow)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// =============================================================================
// Market Data Endpoints (Public - No Auth Required)
// =============================================================================

// GetInstruments fetches all available instruments for a category
func (c *RESTClient) GetInstruments(ctx context.Context, category string) (*InstrumentsInfoResponse, error) {
	params := map[string]string{
		"category": category,
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointInstruments, params, nil, false)
	if err != nil {
		return nil, err
	}

	var resp InstrumentsInfoResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// GetTickers fetches ticker information for all symbols or a specific symbol
func (c *RESTClient) GetTickers(ctx context.Context, category string, symbol string) (*TickersResponse, error) {
	params := map[string]string{
		"category": category,
	}
	if symbol != "" {
		params["symbol"] = symbol
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointTickers, params, nil, false)
	if err != nil {
		return nil, err
	}

	var resp TickersResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// GetOrderbook fetches orderbook depth for a symbol
func (c *RESTClient) GetOrderbook(ctx context.Context, category, symbol string, limit int) (*OrderbookResponse, error) {
	params := map[string]string{
		"category": category,
		"symbol":   symbol,
	}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointOrderbook, params, nil, false)
	if err != nil {
		return nil, err
	}

	var resp OrderbookResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// GetKline fetches kline/candlestick data
func (c *RESTClient) GetKline(ctx context.Context, category, symbol, interval string, startTime, endTime int64, limit int) (*KlineResponse, error) {
	params := map[string]string{
		"category": category,
		"symbol":   symbol,
		"interval": interval, // 1, 3, 5, 15, 30, 60, 120, 240, 360, 720, D, M, W
	}
	if startTime > 0 {
		params["start"] = strconv.FormatInt(startTime, 10)
	}
	if endTime > 0 {
		params["end"] = strconv.FormatInt(endTime, 10)
	}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointKline, params, nil, false)
	if err != nil {
		return nil, err
	}

	var resp KlineResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// GetFundingHistory fetches historical funding rates
func (c *RESTClient) GetFundingHistory(ctx context.Context, category, symbol string, startTime, endTime int64, limit int) (*FundingHistoryResponse, error) {
	params := map[string]string{
		"category": category,
		"symbol":   symbol,
	}
	if startTime > 0 {
		params["startTime"] = strconv.FormatInt(startTime, 10)
	}
	if endTime > 0 {
		params["endTime"] = strconv.FormatInt(endTime, 10)
	}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointFundingHistory, params, nil, false)
	if err != nil {
		return nil, err
	}

	var resp FundingHistoryResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// GetRecentTrades fetches recent public trades
func (c *RESTClient) GetRecentTrades(ctx context.Context, category, symbol string, limit int) (*RecentTradesResponse, error) {
	params := map[string]string{
		"category": category,
	}
	if symbol != "" {
		params["symbol"] = symbol
	}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointRecentTrades, params, nil, false)
	if err != nil {
		return nil, err
	}

	var resp RecentTradesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// GetOpenInterest fetches open interest data
func (c *RESTClient) GetOpenInterest(ctx context.Context, category, symbol, intervalTime string, startTime, endTime int64, limit int) (*OpenInterestResponse, error) {
	params := map[string]string{
		"category":     category,
		"symbol":       symbol,
		"intervalTime": intervalTime, // 5min, 15min, 30min, 1h, 4h, 1d
	}
	if startTime > 0 {
		params["startTime"] = strconv.FormatInt(startTime, 10)
	}
	if endTime > 0 {
		params["endTime"] = strconv.FormatInt(endTime, 10)
	}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointOpenInterest, params, nil, false)
	if err != nil {
		return nil, err
	}

	var resp OpenInterestResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// GetRiskLimit fetches risk limit info
func (c *RESTClient) GetRiskLimit(ctx context.Context, category, symbol string) (*RiskLimitResponse, error) {
	params := map[string]string{
		"category": category,
	}
	if symbol != "" {
		params["symbol"] = symbol
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointRiskLimit, params, nil, false)
	if err != nil {
		return nil, err
	}

	var resp RiskLimitResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// =============================================================================
// Trading Endpoints (Private - Auth Required)
// =============================================================================

// CreateOrder places a new order
func (c *RESTClient) CreateOrder(ctx context.Context, req *CreateOrderRequest) (*CreateOrderResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, EndpointCreateOrder, nil, req, true)
	if err != nil {
		return nil, err
	}

	var resp CreateOrderResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	log.Debug().
		Str("orderId", resp.Result.OrderID).
		Str("symbol", req.Symbol).
		Str("side", req.Side).
		Str("qty", req.Qty).
		Msg("Order created")

	return &resp, nil
}

// AmendOrder modifies an existing order
func (c *RESTClient) AmendOrder(ctx context.Context, req *AmendOrderRequest) (*AmendOrderResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, EndpointAmendOrder, nil, req, true)
	if err != nil {
		return nil, err
	}

	var resp AmendOrderResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// CancelOrder cancels an existing order
func (c *RESTClient) CancelOrder(ctx context.Context, req *CancelOrderRequest) (*CancelOrderResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, EndpointCancelOrder, nil, req, true)
	if err != nil {
		return nil, err
	}

	var resp CancelOrderResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	log.Debug().
		Str("orderId", resp.Result.OrderID).
		Msg("Order cancelled")

	return &resp, nil
}

// CancelAllOrders cancels all open orders
func (c *RESTClient) CancelAllOrders(ctx context.Context, req *CancelAllOrdersRequest) (*CancelAllOrdersResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, EndpointCancelAllOrders, nil, req, true)
	if err != nil {
		return nil, err
	}

	var resp CancelAllOrdersResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	log.Debug().
		Int("count", len(resp.Result.List)).
		Msg("All orders cancelled")

	return &resp, nil
}

// BatchCreateOrders places multiple orders in a single request
func (c *RESTClient) BatchCreateOrders(ctx context.Context, req *BatchOrderRequest) (*BatchOrderResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, EndpointBatchCreateOrder, nil, req, true)
	if err != nil {
		return nil, err
	}

	var resp BatchOrderResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	log.Debug().
		Int("count", len(resp.Result.List)).
		Msg("Batch orders created")

	return &resp, nil
}

// GetOpenOrders fetches open orders
func (c *RESTClient) GetOpenOrders(ctx context.Context, category string, symbol string, limit int) (*GetOrdersResponse, error) {
	params := map[string]string{
		"category": category,
	}
	if symbol != "" {
		params["symbol"] = symbol
	}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointGetOrders, params, nil, true)
	if err != nil {
		return nil, err
	}

	var resp GetOrdersResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// GetOrderHistory fetches order history
func (c *RESTClient) GetOrderHistory(ctx context.Context, category string, symbol string, startTime, endTime int64, limit int) (*GetOrdersResponse, error) {
	params := map[string]string{
		"category": category,
	}
	if symbol != "" {
		params["symbol"] = symbol
	}
	if startTime > 0 {
		params["startTime"] = strconv.FormatInt(startTime, 10)
	}
	if endTime > 0 {
		params["endTime"] = strconv.FormatInt(endTime, 10)
	}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointOrderHistory, params, nil, true)
	if err != nil {
		return nil, err
	}

	var resp GetOrdersResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// GetExecutions fetches execution/trade history
func (c *RESTClient) GetExecutions(ctx context.Context, category string, symbol string, startTime, endTime int64, limit int) (*GetExecutionsResponse, error) {
	params := map[string]string{
		"category": category,
	}
	if symbol != "" {
		params["symbol"] = symbol
	}
	if startTime > 0 {
		params["startTime"] = strconv.FormatInt(startTime, 10)
	}
	if endTime > 0 {
		params["endTime"] = strconv.FormatInt(endTime, 10)
	}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointExecutions, params, nil, true)
	if err != nil {
		return nil, err
	}

	var resp GetExecutionsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// =============================================================================
// Position Endpoints (Private - Auth Required)
// =============================================================================

// GetPositions fetches current positions
func (c *RESTClient) GetPositions(ctx context.Context, category string, symbol string, limit int) (*GetPositionsResponse, error) {
	params := map[string]string{
		"category": category,
	}
	if symbol != "" {
		params["symbol"] = symbol
	}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointPositions, params, nil, true)
	if err != nil {
		return nil, err
	}

	var resp GetPositionsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// SetLeverage sets leverage for a symbol
func (c *RESTClient) SetLeverage(ctx context.Context, req *SetLeverageRequest) (*SetLeverageResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, EndpointSetLeverage, nil, req, true)
	if err != nil {
		return nil, err
	}

	var resp SetLeverageResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// SwitchPositionMode switches between one-way and hedge mode
func (c *RESTClient) SwitchPositionMode(ctx context.Context, req *SwitchPositionModeRequest) error {
	data, err := c.doRequest(ctx, http.MethodPost, EndpointSwitchMode, nil, req, true)
	if err != nil {
		return err
	}

	var resp BaseResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return nil
}

// GetClosedPnl fetches closed PnL records
func (c *RESTClient) GetClosedPnl(ctx context.Context, category string, symbol string, startTime, endTime int64, limit int) (*GetClosedPnlResponse, error) {
	params := map[string]string{
		"category": category,
	}
	if symbol != "" {
		params["symbol"] = symbol
	}
	if startTime > 0 {
		params["startTime"] = strconv.FormatInt(startTime, 10)
	}
	if endTime > 0 {
		params["endTime"] = strconv.FormatInt(endTime, 10)
	}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointClosedPnl, params, nil, true)
	if err != nil {
		return nil, err
	}

	var resp GetClosedPnlResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// =============================================================================
// Account Endpoints (Private - Auth Required)
// =============================================================================

// GetWalletBalance fetches wallet balance
func (c *RESTClient) GetWalletBalance(ctx context.Context, accountType string, coin string) (*GetWalletBalanceResponse, error) {
	params := map[string]string{
		"accountType": accountType,
	}
	if coin != "" {
		params["coin"] = coin
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointWalletBalance, params, nil, true)
	if err != nil {
		return nil, err
	}

	var resp GetWalletBalanceResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// GetFeeRate fetches trading fee rates
func (c *RESTClient) GetFeeRate(ctx context.Context, category string, symbol string) (*GetFeeRateResponse, error) {
	params := map[string]string{
		"category": category,
	}
	if symbol != "" {
		params["symbol"] = symbol
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointFeeRate, params, nil, true)
	if err != nil {
		return nil, err
	}

	var resp GetFeeRateResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// GetAccountInfo fetches account information
func (c *RESTClient) GetAccountInfo(ctx context.Context) (*GetAccountInfoResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, EndpointAccountInfo, nil, nil, true)
	if err != nil {
		return nil, err
	}

	var resp GetAccountInfoResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// =============================================================================
// Asset Endpoints (Private - Auth Required)
// =============================================================================

// GetCoinInfo fetches coin information including deposit/withdraw status
func (c *RESTClient) GetCoinInfo(ctx context.Context, coin string) (*GetCoinInfoResponse, error) {
	params := map[string]string{}
	if coin != "" {
		params["coin"] = coin
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointCoinInfo, params, nil, true)
	if err != nil {
		return nil, err
	}

	var resp GetCoinInfoResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// GetDepositRecords fetches deposit history
func (c *RESTClient) GetDepositRecords(ctx context.Context, coin string, startTime, endTime int64, limit int) (*GetDepositRecordsResponse, error) {
	params := map[string]string{}
	if coin != "" {
		params["coin"] = coin
	}
	if startTime > 0 {
		params["startTime"] = strconv.FormatInt(startTime, 10)
	}
	if endTime > 0 {
		params["endTime"] = strconv.FormatInt(endTime, 10)
	}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointDepositRecords, params, nil, true)
	if err != nil {
		return nil, err
	}

	var resp GetDepositRecordsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// GetWithdrawRecords fetches withdrawal history
func (c *RESTClient) GetWithdrawRecords(ctx context.Context, coin string, withdrawType int, startTime, endTime int64, limit int) (*GetWithdrawRecordsResponse, error) {
	params := map[string]string{}
	if coin != "" {
		params["coin"] = coin
	}
	if withdrawType >= 0 {
		params["withdrawType"] = strconv.Itoa(withdrawType)
	}
	if startTime > 0 {
		params["startTime"] = strconv.FormatInt(startTime, 10)
	}
	if endTime > 0 {
		params["endTime"] = strconv.FormatInt(endTime, 10)
	}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}

	data, err := c.doRequest(ctx, http.MethodGet, EndpointWithdrawRecords, params, nil, true)
	if err != nil {
		return nil, err
	}

	var resp GetWithdrawRecordsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.RetCode != 0 {
		return nil, fmt.Errorf("API error %d: %s", resp.RetCode, resp.RetMsg)
	}

	return &resp, nil
}

// =============================================================================
// Helper Methods
// =============================================================================

// GetLinearInstruments is a convenience method to get all linear (USDT) perpetuals
func (c *RESTClient) GetLinearInstruments(ctx context.Context) (*InstrumentsInfoResponse, error) {
	return c.GetInstruments(ctx, string(CategoryLinear))
}

// GetLinearTickers is a convenience method to get all linear perpetual tickers
func (c *RESTClient) GetLinearTickers(ctx context.Context) (*TickersResponse, error) {
	return c.GetTickers(ctx, string(CategoryLinear), "")
}

// GetLinearOrderbook is a convenience method to get orderbook for a linear perpetual
func (c *RESTClient) GetLinearOrderbook(ctx context.Context, symbol string, limit int) (*OrderbookResponse, error) {
	return c.GetOrderbook(ctx, string(CategoryLinear), symbol, limit)
}

// GetLinearPositions is a convenience method to get positions for linear perpetuals
func (c *RESTClient) GetLinearPositions(ctx context.Context, symbol string) (*GetPositionsResponse, error) {
	return c.GetPositions(ctx, string(CategoryLinear), symbol, 200)
}

// PlaceLimitOrder is a convenience method to place a limit order
func (c *RESTClient) PlaceLimitOrder(ctx context.Context, symbol string, side OrderSide, qty, price string) (*CreateOrderResponse, error) {
	req := &CreateOrderRequest{
		Category:    string(CategoryLinear),
		Symbol:      symbol,
		Side:        string(side),
		OrderType:   string(OrderTypeLimit),
		Qty:         qty,
		Price:       price,
		TimeInForce: string(TimeInForceGTC),
		PositionIdx: 0, // one-way mode
	}
	return c.CreateOrder(ctx, req)
}

// PlaceMarketOrder is a convenience method to place a market order
func (c *RESTClient) PlaceMarketOrder(ctx context.Context, symbol string, side OrderSide, qty string) (*CreateOrderResponse, error) {
	req := &CreateOrderRequest{
		Category:    string(CategoryLinear),
		Symbol:      symbol,
		Side:        string(side),
		OrderType:   string(OrderTypeMarket),
		Qty:         qty,
		PositionIdx: 0,
	}
	return c.CreateOrder(ctx, req)
}

// NormalizeSymbol extracts base asset from symbol (e.g., BTCUSDT -> BTC)
func NormalizeSymbol(symbol string) string {
	symbol = strings.ToUpper(symbol)
	suffixes := []string{"USDT", "USDC", "USD", "PERP"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(symbol, suffix) {
			return strings.TrimSuffix(symbol, suffix)
		}
	}
	return symbol
}
