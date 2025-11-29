package htx

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
	"sort"
	"strconv"
	"strings"
	"time"
)

// RestClient handles REST API requests for HTX
type RestClient struct {
	baseURL     string
	httpClient  *http.Client
	credentials *Credentials
	rateLimiter *RateLimiter
}

// NewRestClient creates a new REST client
func NewRestClient(credentials *Credentials) *RestClient {
	return &RestClient{
		baseURL: RestBaseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		credentials: credentials,
		rateLimiter: NewRateLimiter(PrivateRateLimit, 3*time.Second),
	}
}

// NewRestClientWithURL creates a new REST client with custom base URL
func NewRestClientWithURL(baseURL string, credentials *Credentials) *RestClient {
	return &RestClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		credentials: credentials,
		rateLimiter: NewRateLimiter(PrivateRateLimit, 3*time.Second),
	}
}

// SetBaseURL sets the base URL
func (c *RestClient) SetBaseURL(baseURL string) {
	c.baseURL = baseURL
}

// generateSignature generates HMAC-SHA256 signature for HTX API
func (c *RestClient) generateSignature(method, host, path string, params map[string]string) (string, string) {
	// Get timestamp
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05")

	// Add required auth params
	params["AccessKeyId"] = c.credentials.APIKey
	params["SignatureMethod"] = SignatureMethod
	params["SignatureVersion"] = SignatureVersion
	params["Timestamp"] = timestamp

	// Sort params alphabetically
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build query string
	var queryParts []string
	for _, k := range keys {
		queryParts = append(queryParts, fmt.Sprintf("%s=%s", k, url.QueryEscape(params[k])))
	}
	queryString := strings.Join(queryParts, "&")

	// Build signature payload
	payload := fmt.Sprintf("%s\n%s\n%s\n%s", method, host, path, queryString)

	// Calculate HMAC-SHA256
	h := hmac.New(sha256.New, []byte(c.credentials.SecretKey))
	h.Write([]byte(payload))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return signature, timestamp
}

// doPublicRequest performs a public (unauthenticated) API request
func (c *RestClient) doPublicRequest(ctx context.Context, method, path string, params map[string]string) ([]byte, error) {
	// Build URL with params
	reqURL, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if len(params) > 0 {
		q := reqURL.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		reqURL.RawQuery = q.Encode()
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// doPrivateRequest performs an authenticated API request
func (c *RestClient) doPrivateRequest(ctx context.Context, method, path string, params map[string]string, body interface{}) ([]byte, error) {
	// Rate limit
	c.rateLimiter.Acquire()

	// Parse host from baseURL
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	host := u.Host

	// Initialize params if nil
	if params == nil {
		params = make(map[string]string)
	}

	// Generate signature
	signature, _ := c.generateSignature(method, host, path, params)
	params["Signature"] = signature

	// Build URL with auth params
	reqURL, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	q := reqURL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	reqURL.RawQuery = q.Encode()

	// Create request body if provided
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// parseResponse parses base response and checks for errors
func (c *RestClient) parseResponse(body []byte) (*BaseResponse, error) {
	var resp BaseResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.Status != "ok" {
		return nil, fmt.Errorf("API error %d: %s", resp.ErrCode, resp.ErrMsg)
	}

	return &resp, nil
}

// ========== Reference Data APIs ==========

// GetContractInfo gets contract information
func (c *RestClient) GetContractInfo(ctx context.Context, contractCode string) ([]ContractInfo, error) {
	params := make(map[string]string)
	if contractCode != "" {
		params["contract_code"] = contractCode
	}

	body, err := c.doPublicRequest(ctx, http.MethodGet, PathContractInfo, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var contracts []ContractInfo
	if err := json.Unmarshal(resp.Data, &contracts); err != nil {
		return nil, fmt.Errorf("unmarshal contracts: %w", err)
	}

	return contracts, nil
}

// GetPriceLimit gets price limit for a contract
func (c *RestClient) GetPriceLimit(ctx context.Context, contractCode string) (*PriceLimit, error) {
	params := map[string]string{
		"contract_code": contractCode,
	}

	body, err := c.doPublicRequest(ctx, http.MethodGet, PathPriceLimit, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var limits []PriceLimit
	if err := json.Unmarshal(resp.Data, &limits); err != nil {
		return nil, fmt.Errorf("unmarshal price limit: %w", err)
	}

	if len(limits) == 0 {
		return nil, fmt.Errorf("no price limit data")
	}

	return &limits[0], nil
}

// GetOpenInterest gets open interest
func (c *RestClient) GetOpenInterest(ctx context.Context, contractCode string) ([]OpenInterest, error) {
	params := make(map[string]string)
	if contractCode != "" {
		params["contract_code"] = contractCode
	}

	body, err := c.doPublicRequest(ctx, http.MethodGet, PathOpenInterest, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var oi []OpenInterest
	if err := json.Unmarshal(resp.Data, &oi); err != nil {
		return nil, fmt.Errorf("unmarshal open interest: %w", err)
	}

	return oi, nil
}

// GetIndex gets index price
func (c *RestClient) GetIndex(ctx context.Context, contractCode string) ([]IndexPrice, error) {
	params := make(map[string]string)
	if contractCode != "" {
		params["contract_code"] = contractCode
	}

	body, err := c.doPublicRequest(ctx, http.MethodGet, PathIndex, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var indices []IndexPrice
	if err := json.Unmarshal(resp.Data, &indices); err != nil {
		return nil, fmt.Errorf("unmarshal index: %w", err)
	}

	return indices, nil
}

// GetTradingFee gets trading fee
func (c *RestClient) GetTradingFee(ctx context.Context, contractCode string) ([]TradingFee, error) {
	params := map[string]string{
		"contract_code": contractCode,
	}

	body, err := c.doPrivateRequest(ctx, http.MethodGet, PathFee, params, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var fees []TradingFee
	if err := json.Unmarshal(resp.Data, &fees); err != nil {
		return nil, fmt.Errorf("unmarshal fees: %w", err)
	}

	return fees, nil
}

// ========== Market Data APIs ==========

// GetDepth gets order book depth
func (c *RestClient) GetDepth(ctx context.Context, contractCode, depthType string) (*DepthData, error) {
	params := map[string]string{
		"contract_code": contractCode,
		"type":          depthType,
	}

	body, err := c.doPublicRequest(ctx, http.MethodGet, PathDepth, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var depth DepthData
	if err := json.Unmarshal(resp.Tick, &depth); err != nil {
		return nil, fmt.Errorf("unmarshal depth: %w", err)
	}

	return &depth, nil
}

// GetBBO gets best bid/offer
func (c *RestClient) GetBBO(ctx context.Context, contractCode string) ([]BBOData, error) {
	params := make(map[string]string)
	if contractCode != "" {
		params["contract_code"] = contractCode
	}

	body, err := c.doPublicRequest(ctx, http.MethodGet, PathBBO, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var bbo []BBOData
	if err := json.Unmarshal(resp.Ticks, &bbo); err != nil {
		return nil, fmt.Errorf("unmarshal bbo: %w", err)
	}

	return bbo, nil
}

// GetKline gets kline/candlestick data
func (c *RestClient) GetKline(ctx context.Context, contractCode, period string, size int, from, to int64) ([]KlineData, error) {
	params := map[string]string{
		"contract_code": contractCode,
		"period":        period,
	}

	if size > 0 {
		params["size"] = strconv.Itoa(size)
	}
	if from > 0 {
		params["from"] = strconv.FormatInt(from, 10)
	}
	if to > 0 {
		params["to"] = strconv.FormatInt(to, 10)
	}

	body, err := c.doPublicRequest(ctx, http.MethodGet, PathKline, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var klines []KlineData
	if err := json.Unmarshal(resp.Data, &klines); err != nil {
		return nil, fmt.Errorf("unmarshal kline: %w", err)
	}

	return klines, nil
}

// GetTicker gets market ticker
func (c *RestClient) GetTicker(ctx context.Context, contractCode string) (*TickerData, error) {
	params := map[string]string{
		"contract_code": contractCode,
	}

	body, err := c.doPublicRequest(ctx, http.MethodGet, PathTicker, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var ticker TickerData
	if err := json.Unmarshal(resp.Tick, &ticker); err != nil {
		return nil, fmt.Errorf("unmarshal ticker: %w", err)
	}

	return &ticker, nil
}

// GetBatchTicker gets batch ticker data
func (c *RestClient) GetBatchTicker(ctx context.Context, contractCodes []string) ([]BatchTickerData, error) {
	params := make(map[string]string)
	if len(contractCodes) > 0 {
		params["contract_code"] = strings.Join(contractCodes, ",")
	}

	body, err := c.doPublicRequest(ctx, http.MethodGet, PathBatchTicker, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var tickers []BatchTickerData
	if err := json.Unmarshal(resp.Ticks, &tickers); err != nil {
		return nil, fmt.Errorf("unmarshal tickers: %w", err)
	}

	return tickers, nil
}

// GetRecentTrades gets recent trades
func (c *RestClient) GetRecentTrades(ctx context.Context, contractCode string) (*TradeTick, error) {
	params := map[string]string{
		"contract_code": contractCode,
	}

	body, err := c.doPublicRequest(ctx, http.MethodGet, PathTrade, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var tick TradeTick
	if err := json.Unmarshal(resp.Tick, &tick); err != nil {
		return nil, fmt.Errorf("unmarshal trades: %w", err)
	}

	return &tick, nil
}

// GetHistoryTrades gets history trades
func (c *RestClient) GetHistoryTrades(ctx context.Context, contractCode string, size int) ([]TradeTick, error) {
	params := map[string]string{
		"contract_code": contractCode,
		"size":          strconv.Itoa(size),
	}

	body, err := c.doPublicRequest(ctx, http.MethodGet, PathHistoryTrade, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var trades []TradeTick
	if err := json.Unmarshal(resp.Data, &trades); err != nil {
		return nil, fmt.Errorf("unmarshal history trades: %w", err)
	}

	return trades, nil
}

// ========== Funding Rate APIs ==========

// GetFundingRate gets current funding rate
func (c *RestClient) GetFundingRate(ctx context.Context, contractCode string) (*FundingRate, error) {
	params := map[string]string{
		"contract_code": contractCode,
	}

	body, err := c.doPublicRequest(ctx, http.MethodGet, PathFundingRate, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var fr FundingRate
	if err := json.Unmarshal(resp.Data, &fr); err != nil {
		return nil, fmt.Errorf("unmarshal funding rate: %w", err)
	}

	return &fr, nil
}

// GetBatchFundingRate gets funding rates for multiple contracts
func (c *RestClient) GetBatchFundingRate(ctx context.Context, contractCode string) ([]FundingRate, error) {
	params := make(map[string]string)
	if contractCode != "" {
		params["contract_code"] = contractCode
	}

	body, err := c.doPublicRequest(ctx, http.MethodGet, PathBatchFundingRate, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var rates []FundingRate
	if err := json.Unmarshal(resp.Data, &rates); err != nil {
		return nil, fmt.Errorf("unmarshal funding rates: %w", err)
	}

	return rates, nil
}

// GetHistoricalFundingRate gets historical funding rates
func (c *RestClient) GetHistoricalFundingRate(ctx context.Context, contractCode string, pageIndex, pageSize int) ([]FundingRate, error) {
	params := map[string]string{
		"contract_code": contractCode,
	}
	if pageIndex > 0 {
		params["page_index"] = strconv.Itoa(pageIndex)
	}
	if pageSize > 0 {
		params["page_size"] = strconv.Itoa(pageSize)
	}

	body, err := c.doPublicRequest(ctx, http.MethodGet, PathHistoricalFunding, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	// Response has nested structure
	var result struct {
		Data []FundingRate `json:"data"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal historical funding: %w", err)
	}

	return result.Data, nil
}

// ========== Account APIs (Cross Margin) ==========

// GetCrossAccountInfo gets cross margin account information
func (c *RestClient) GetCrossAccountInfo(ctx context.Context, marginAccount string) ([]CrossAccountInfo, error) {
	params := make(map[string]string)
	if marginAccount != "" {
		params["margin_account"] = marginAccount
	}

	body, err := c.doPrivateRequest(ctx, http.MethodPost, PathCrossAccountInfo, params, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var accounts []CrossAccountInfo
	if err := json.Unmarshal(resp.Data, &accounts); err != nil {
		return nil, fmt.Errorf("unmarshal accounts: %w", err)
	}

	return accounts, nil
}

// GetCrossPositionInfo gets cross margin position information
func (c *RestClient) GetCrossPositionInfo(ctx context.Context, contractCode string) ([]CrossPositionInfo, error) {
	params := make(map[string]string)
	if contractCode != "" {
		params["contract_code"] = contractCode
	}

	body, err := c.doPrivateRequest(ctx, http.MethodPost, PathCrossPositionInfo, params, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var positions []CrossPositionInfo
	if err := json.Unmarshal(resp.Data, &positions); err != nil {
		return nil, fmt.Errorf("unmarshal positions: %w", err)
	}

	return positions, nil
}

// ========== Trading APIs (Cross Margin) ==========

// PlaceCrossOrder places a cross margin order
func (c *RestClient) PlaceCrossOrder(ctx context.Context, req *OrderRequest) (*OrderResponse, error) {
	params := make(map[string]string)

	body, err := c.doPrivateRequest(ctx, http.MethodPost, PathCrossOrder, params, req)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var orderResp OrderResponse
	if err := json.Unmarshal(resp.Data, &orderResp); err != nil {
		return nil, fmt.Errorf("unmarshal order response: %w", err)
	}

	return &orderResp, nil
}

// PlaceCrossBatchOrder places multiple cross margin orders
func (c *RestClient) PlaceCrossBatchOrder(ctx context.Context, orders []OrderRequest) (*BatchOrderResponse, error) {
	params := make(map[string]string)
	reqBody := &BatchOrderRequest{OrdersData: orders}

	body, err := c.doPrivateRequest(ctx, http.MethodPost, PathCrossBatchOrder, params, reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var batchResp BatchOrderResponse
	if err := json.Unmarshal(resp.Data, &batchResp); err != nil {
		return nil, fmt.Errorf("unmarshal batch response: %w", err)
	}

	return &batchResp, nil
}

// CancelCrossOrder cancels a cross margin order
func (c *RestClient) CancelCrossOrder(ctx context.Context, contractCode string, orderID, clientOrderID string) (*CancelResponse, error) {
	params := make(map[string]string)
	reqBody := &CancelRequest{
		ContractCode: contractCode,
	}
	if orderID != "" {
		reqBody.OrderID = orderID
	}
	if clientOrderID != "" {
		reqBody.ClientOrderID = clientOrderID
	}

	body, err := c.doPrivateRequest(ctx, http.MethodPost, PathCrossCancel, params, reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var cancelResp CancelResponse
	if err := json.Unmarshal(resp.Data, &cancelResp); err != nil {
		return nil, fmt.Errorf("unmarshal cancel response: %w", err)
	}

	return &cancelResp, nil
}

// CancelAllCrossOrders cancels all cross margin orders
func (c *RestClient) CancelAllCrossOrders(ctx context.Context, contractCode, direction, offset string) (*CancelResponse, error) {
	params := make(map[string]string)
	reqBody := map[string]string{
		"contract_code": contractCode,
	}
	if direction != "" {
		reqBody["direction"] = direction
	}
	if offset != "" {
		reqBody["offset"] = offset
	}

	body, err := c.doPrivateRequest(ctx, http.MethodPost, PathCrossCancelAll, params, reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var cancelResp CancelResponse
	if err := json.Unmarshal(resp.Data, &cancelResp); err != nil {
		return nil, fmt.Errorf("unmarshal cancel response: %w", err)
	}

	return &cancelResp, nil
}

// GetCrossOrderInfo gets cross margin order information
func (c *RestClient) GetCrossOrderInfo(ctx context.Context, contractCode, orderID, clientOrderID string) ([]OrderInfo, error) {
	params := make(map[string]string)
	reqBody := map[string]string{
		"contract_code": contractCode,
	}
	if orderID != "" {
		reqBody["order_id"] = orderID
	}
	if clientOrderID != "" {
		reqBody["client_order_id"] = clientOrderID
	}

	body, err := c.doPrivateRequest(ctx, http.MethodPost, PathCrossOrderInfo, params, reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var orders []OrderInfo
	if err := json.Unmarshal(resp.Data, &orders); err != nil {
		return nil, fmt.Errorf("unmarshal orders: %w", err)
	}

	return orders, nil
}

// GetCrossOrderDetail gets cross margin order detail with trades
func (c *RestClient) GetCrossOrderDetail(ctx context.Context, contractCode string, orderID int64, pageIndex, pageSize int) (*OrderDetail, error) {
	params := make(map[string]string)
	reqBody := map[string]interface{}{
		"contract_code": contractCode,
		"order_id":      orderID,
	}
	if pageIndex > 0 {
		reqBody["page_index"] = pageIndex
	}
	if pageSize > 0 {
		reqBody["page_size"] = pageSize
	}

	body, err := c.doPrivateRequest(ctx, http.MethodPost, PathCrossOrderDetail, params, reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var detail OrderDetail
	if err := json.Unmarshal(resp.Data, &detail); err != nil {
		return nil, fmt.Errorf("unmarshal order detail: %w", err)
	}

	return &detail, nil
}

// GetCrossOpenOrders gets cross margin open orders
func (c *RestClient) GetCrossOpenOrders(ctx context.Context, contractCode string, pageIndex, pageSize int) (*OpenOrdersResponse, error) {
	params := make(map[string]string)
	reqBody := map[string]interface{}{
		"contract_code": contractCode,
	}
	if pageIndex > 0 {
		reqBody["page_index"] = pageIndex
	}
	if pageSize > 0 {
		reqBody["page_size"] = pageSize
	}

	body, err := c.doPrivateRequest(ctx, http.MethodPost, PathCrossOpenOrders, params, reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(body)
	if err != nil {
		return nil, err
	}

	var openOrders OpenOrdersResponse
	if err := json.Unmarshal(resp.Data, &openOrders); err != nil {
		return nil, fmt.Errorf("unmarshal open orders: %w", err)
	}

	return &openOrders, nil
}

// Close stops the REST client
func (c *RestClient) Close() {
	if c.rateLimiter != nil {
		c.rateLimiter.Stop()
	}
}
