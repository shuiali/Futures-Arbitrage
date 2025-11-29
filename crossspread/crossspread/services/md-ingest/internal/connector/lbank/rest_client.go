package lbank

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
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

// RestClient handles REST API requests for LBank
type RestClient struct {
	httpClient     *http.Client
	credentials    *Credentials
	useContractAPI bool
	productGroup   string
	requestTimeout time.Duration
}

// NewRestClient creates a new REST API client
func NewRestClient(config *ClientConfig) *RestClient {
	timeout := config.RequestTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &RestClient{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		credentials:    config.Credentials,
		useContractAPI: config.UseContractAPI,
		productGroup:   config.ProductGroup,
		requestTimeout: timeout,
	}
}

// generateEchostr generates a random echostr for contract API
func generateEchostr() string {
	return fmt.Sprintf("echostr%d%d", time.Now().UnixNano(), time.Now().Unix())[:35]
}

// signContractRequest signs a request for the contract API
func (c *RestClient) signContractRequest(params map[string]string) (string, error) {
	if c.credentials == nil {
		return "", fmt.Errorf("credentials not configured")
	}

	// Sort parameters alphabetically
	keys := make([]string, 0, len(params))
	for k := range params {
		if k != "sign" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	// Build parameter string
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, params[k]))
	}
	paramStr := strings.Join(parts, "&")

	// MD5 digest (uppercase)
	md5Hash := md5.Sum([]byte(paramStr))
	preparedStr := strings.ToUpper(hex.EncodeToString(md5Hash[:]))

	// Sign based on method
	var signature string
	switch c.credentials.SignatureMethod {
	case SignatureMethodHmacSHA256:
		h := hmac.New(sha256.New, []byte(c.credentials.SecretKey))
		h.Write([]byte(preparedStr))
		signature = hex.EncodeToString(h.Sum(nil))
	case SignatureMethodRSA:
		// RSA signing would require additional implementation
		// For now, fall back to HMAC
		h := hmac.New(sha256.New, []byte(c.credentials.SecretKey))
		h.Write([]byte(preparedStr))
		signature = hex.EncodeToString(h.Sum(nil))
	default:
		return "", fmt.Errorf("unsupported signature method: %s", c.credentials.SignatureMethod)
	}

	return signature, nil
}

// signSpotRequest signs a request for the spot API
func (c *RestClient) signSpotRequest(params map[string]string) (string, error) {
	if c.credentials == nil {
		return "", fmt.Errorf("credentials not configured")
	}

	// Sort parameters alphabetically
	keys := make([]string, 0, len(params))
	for k := range params {
		if k != "sign" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	// Build parameter string and append secret key
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, params[k]))
	}
	paramStr := strings.Join(parts, "&")
	paramStr += "&secret_key=" + c.credentials.SecretKey

	// MD5 digest (uppercase)
	md5Hash := md5.Sum([]byte(paramStr))
	return strings.ToUpper(hex.EncodeToString(md5Hash[:])), nil
}

// doContractRequest performs a request to the contract API
func (c *RestClient) doContractRequest(ctx context.Context, method, endpoint string, params map[string]string, result interface{}) error {
	baseURL := ContractRestBaseURL + endpoint

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	echostr := generateEchostr()

	// Add auth params for signing
	if c.credentials != nil {
		if params == nil {
			params = make(map[string]string)
		}
		params["api_key"] = c.credentials.APIKey
		params["timestamp"] = timestamp
		params["echostr"] = echostr
		params["signature_method"] = c.credentials.SignatureMethod

		sign, err := c.signContractRequest(params)
		if err != nil {
			return fmt.Errorf("signing failed: %w", err)
		}
		params["sign"] = sign
	}

	var req *http.Request
	var err error

	if method == http.MethodGet {
		// Build query string
		if len(params) > 0 {
			v := url.Values{}
			for k, val := range params {
				v.Set(k, val)
			}
			baseURL += "?" + v.Encode()
		}
		req, err = http.NewRequestWithContext(ctx, method, baseURL, nil)
	} else {
		// POST with JSON body
		body, _ := json.Marshal(params)
		req, err = http.NewRequestWithContext(ctx, method, baseURL, bytes.NewReader(body))
		if req != nil {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	if err != nil {
		return fmt.Errorf("creating request failed: %w", err)
	}

	// Set headers
	if c.credentials != nil {
		req.Header.Set("timestamp", timestamp)
		req.Header.Set("signature_method", c.credentials.SignatureMethod)
		req.Header.Set("echostr", echostr)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("parsing response failed: %w", err)
		}
	}

	return nil
}

// doSpotRequest performs a request to the spot API
func (c *RestClient) doSpotRequest(ctx context.Context, method, endpoint string, params map[string]string, result interface{}) error {
	baseURL := SpotRestBaseURL + endpoint

	// Add auth params for signing
	if c.credentials != nil && params != nil {
		params["api_key"] = c.credentials.APIKey
		sign, err := c.signSpotRequest(params)
		if err != nil {
			return fmt.Errorf("signing failed: %w", err)
		}
		params["sign"] = sign
	}

	var req *http.Request
	var err error

	if method == http.MethodGet {
		if len(params) > 0 {
			v := url.Values{}
			for k, val := range params {
				v.Set(k, val)
			}
			baseURL += "?" + v.Encode()
		}
		req, err = http.NewRequestWithContext(ctx, method, baseURL, nil)
	} else {
		// POST with form data
		v := url.Values{}
		for k, val := range params {
			v.Set(k, val)
		}
		req, err = http.NewRequestWithContext(ctx, method, baseURL, strings.NewReader(v.Encode()))
		if req != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}

	if err != nil {
		return fmt.Errorf("creating request failed: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("parsing response failed: %w", err)
		}
	}

	return nil
}

// ==================== Contract API Methods ====================

// GetContractServerTime gets the server time
func (c *RestClient) GetContractServerTime(ctx context.Context) (int64, error) {
	var result struct {
		Data struct {
			ServerTime int64 `json:"serverTime"`
		} `json:"data"`
		ErrorCode int  `json:"error_code"`
		Success   bool `json:"success"`
	}

	err := c.doContractRequest(ctx, http.MethodGet, ContractPublicPath+"/getTime", nil, &result)
	if err != nil {
		return 0, err
	}

	return result.Data.ServerTime, nil
}

// GetContractInstruments fetches all available perpetual contracts
func (c *RestClient) GetContractInstruments(ctx context.Context) ([]ContractInstrument, error) {
	// Try wrapped response format first (newer API)
	var wrappedResult struct {
		Data []ContractInstrument `json:"data"`
	}

	params := map[string]string{
		"productGroup": c.productGroup,
	}

	err := c.doContractRequest(ctx, http.MethodGet, ContractPublicPath+"/instrument", params, &wrappedResult)
	if err == nil && len(wrappedResult.Data) > 0 {
		return wrappedResult.Data, nil
	}

	// Fallback to direct array format (older API)
	var result []ContractInstrument
	err = c.doContractRequest(ctx, http.MethodGet, ContractPublicPath+"/instrument", params, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetContractMarketData fetches market data (tickers) for all contracts
func (c *RestClient) GetContractMarketData(ctx context.Context) ([]ContractMarketData, error) {
	params := map[string]string{
		"productGroup": c.productGroup,
	}

	// LBank returns wrapped response: {"data": [...]}
	var wrappedResult struct {
		Data []ContractMarketData `json:"data"`
	}

	err := c.doContractRequest(ctx, http.MethodGet, ContractPublicPath+"/marketData", params, &wrappedResult)
	if err != nil {
		return nil, err
	}

	return wrappedResult.Data, nil
}

// GetContractOrderbook fetches the orderbook for a specific symbol
func (c *RestClient) GetContractOrderbook(ctx context.Context, symbol string, depth int) (*ContractOrderbook, error) {
	var result ContractOrderbook

	params := map[string]string{
		"symbol": symbol,
		"depth":  strconv.Itoa(depth),
	}

	err := c.doContractRequest(ctx, http.MethodGet, ContractPublicPath+"/marketOrder", params, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// GetContractAccount fetches account info (authenticated)
func (c *RestClient) GetContractAccount(ctx context.Context, asset string) (*ContractAccount, error) {
	if c.credentials == nil {
		return nil, fmt.Errorf("authentication required")
	}

	var result struct {
		Data      ContractAccount `json:"data"`
		ErrorCode int             `json:"error_code"`
		Success   bool            `json:"success"`
	}

	params := map[string]string{
		"productGroup": c.productGroup,
		"asset":        asset,
	}

	err := c.doContractRequest(ctx, http.MethodPost, ContractPrivatePath+"/account", params, &result)
	if err != nil {
		return nil, err
	}

	if result.ErrorCode != 0 {
		return nil, fmt.Errorf("error %d: %s", result.ErrorCode, ErrorMessages[result.ErrorCode])
	}

	return &result.Data, nil
}

// GetContractPositions fetches all positions (authenticated)
func (c *RestClient) GetContractPositions(ctx context.Context) ([]ContractPosition, error) {
	if c.credentials == nil {
		return nil, fmt.Errorf("authentication required")
	}

	var result struct {
		Data      []ContractPosition `json:"data"`
		ErrorCode int                `json:"error_code"`
		Success   bool               `json:"success"`
	}

	params := map[string]string{
		"productGroup": c.productGroup,
	}

	err := c.doContractRequest(ctx, http.MethodPost, ContractPrivatePath+"/position", params, &result)
	if err != nil {
		return nil, err
	}

	if result.ErrorCode != 0 {
		return nil, fmt.Errorf("error %d: %s", result.ErrorCode, ErrorMessages[result.ErrorCode])
	}

	return result.Data, nil
}

// PlaceContractOrder places a new order (authenticated)
func (c *RestClient) PlaceContractOrder(ctx context.Context, symbol, side, orderType string, price, volume float64) (*ContractOrder, error) {
	if c.credentials == nil {
		return nil, fmt.Errorf("authentication required")
	}

	var result struct {
		Data      ContractOrder `json:"data"`
		ErrorCode int           `json:"error_code"`
		Success   bool          `json:"success"`
	}

	params := map[string]string{
		"symbol": symbol,
		"side":   side,
		"type":   orderType,
		"volume": strconv.FormatFloat(volume, 'f', -1, 64),
	}

	if orderType == OrderTypeLimit {
		params["price"] = strconv.FormatFloat(price, 'f', -1, 64)
	}

	err := c.doContractRequest(ctx, http.MethodPost, ContractPrivatePath+"/order/create", params, &result)
	if err != nil {
		return nil, err
	}

	if result.ErrorCode != 0 {
		return nil, fmt.Errorf("error %d: %s", result.ErrorCode, ErrorMessages[result.ErrorCode])
	}

	return &result.Data, nil
}

// CancelContractOrder cancels an existing order (authenticated)
func (c *RestClient) CancelContractOrder(ctx context.Context, symbol, orderID string) error {
	if c.credentials == nil {
		return fmt.Errorf("authentication required")
	}

	var result struct {
		ErrorCode int  `json:"error_code"`
		Success   bool `json:"success"`
	}

	params := map[string]string{
		"symbol":  symbol,
		"orderId": orderID,
	}

	err := c.doContractRequest(ctx, http.MethodPost, ContractPrivatePath+"/order/cancel", params, &result)
	if err != nil {
		return err
	}

	if result.ErrorCode != 0 {
		return fmt.Errorf("error %d: %s", result.ErrorCode, ErrorMessages[result.ErrorCode])
	}

	return nil
}

// ==================== Spot API Methods ====================

// GetSpotTickers fetches all spot tickers
func (c *RestClient) GetSpotTickers(ctx context.Context) ([]SpotTicker, error) {
	var result struct {
		Data []SpotTicker `json:"data"`
	}

	params := map[string]string{
		"symbol": "all",
	}

	err := c.doSpotRequest(ctx, http.MethodGet, "/v2/ticker/24hr.do", params, &result)
	if err != nil {
		// Try parsing as array directly
		var tickers []SpotTicker
		params2 := map[string]string{"symbol": "all"}
		err2 := c.doSpotRequest(ctx, http.MethodGet, "/v1/ticker.do", params2, &tickers)
		if err2 != nil {
			return nil, err
		}
		return tickers, nil
	}

	return result.Data, nil
}

// GetSpotTicker fetches a single spot ticker
func (c *RestClient) GetSpotTicker(ctx context.Context, symbol string) (*SpotTicker, error) {
	var result SpotTicker

	params := map[string]string{
		"symbol": symbol,
	}

	err := c.doSpotRequest(ctx, http.MethodGet, "/v1/ticker.do", params, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// GetSpotOrderbook fetches the spot orderbook
func (c *RestClient) GetSpotOrderbook(ctx context.Context, symbol string, size int) (*SpotOrderbook, error) {
	var result struct {
		Result bool          `json:"result"`
		Data   SpotOrderbook `json:"data"`
	}

	params := map[string]string{
		"symbol": symbol,
		"size":   strconv.Itoa(size),
	}

	err := c.doSpotRequest(ctx, http.MethodGet, "/v2/depth.do", params, &result)
	if err != nil {
		// Try v1 endpoint
		var ob SpotOrderbook
		err2 := c.doSpotRequest(ctx, http.MethodGet, "/v1/depth.do", params, &ob)
		if err2 != nil {
			return nil, err
		}
		return &ob, nil
	}

	return &result.Data, nil
}

// GetSpotTrades fetches recent trades
func (c *RestClient) GetSpotTrades(ctx context.Context, symbol string, size int) ([]SpotTrade, error) {
	var result []SpotTrade

	params := map[string]string{
		"symbol": symbol,
		"size":   strconv.Itoa(size),
	}

	err := c.doSpotRequest(ctx, http.MethodGet, "/v1/trades.do", params, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetSpotAssetConfigs fetches deposit/withdrawal configurations
func (c *RestClient) GetSpotAssetConfigs(ctx context.Context) ([]SpotAssetConfig, error) {
	// Try wrapped response format first: {"result":true, "data":[...]}
	var wrappedResult struct {
		Result bool              `json:"result"`
		Data   []SpotAssetConfig `json:"data"`
	}

	err := c.doSpotRequest(ctx, http.MethodGet, "/v2/assetConfigs.do", nil, &wrappedResult)
	if err == nil && wrappedResult.Result && len(wrappedResult.Data) > 0 {
		return wrappedResult.Data, nil
	}

	// Try direct array format
	var result []SpotAssetConfig
	err = c.doSpotRequest(ctx, http.MethodGet, "/v2/assetConfigs.do", nil, &result)
	if err == nil && len(result) > 0 {
		return result, nil
	}

	log.Warn().Err(err).Msg("Failed to fetch asset configs from v2, trying v1")
	// Try v1 endpoint with wrapped format
	err = c.doSpotRequest(ctx, http.MethodGet, "/v1/withdrawConfigs.do", nil, &wrappedResult)
	if err == nil && wrappedResult.Result && len(wrappedResult.Data) > 0 {
		return wrappedResult.Data, nil
	}

	// Try v1 with direct format
	err = c.doSpotRequest(ctx, http.MethodGet, "/v1/withdrawConfigs.do", nil, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetSpotUserInfo fetches authenticated user account info
func (c *RestClient) GetSpotUserInfo(ctx context.Context) (*SpotUserInfo, error) {
	if c.credentials == nil {
		return nil, fmt.Errorf("authentication required")
	}

	var result SpotUserInfo

	params := map[string]string{}

	err := c.doSpotRequest(ctx, http.MethodPost, "/v1/user_info.do", params, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// GetSpotCurrencyPairs fetches all available trading pairs
func (c *RestClient) GetSpotCurrencyPairs(ctx context.Context) ([]string, error) {
	var result []string

	err := c.doSpotRequest(ctx, http.MethodGet, "/v1/currencyPairs.do", nil, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// PlaceSpotOrder places a spot order (authenticated)
func (c *RestClient) PlaceSpotOrder(ctx context.Context, symbol, orderType string, price, amount float64) (string, error) {
	if c.credentials == nil {
		return "", fmt.Errorf("authentication required")
	}

	var result struct {
		Result  string `json:"result"`
		OrderID string `json:"order_id"`
	}

	params := map[string]string{
		"symbol": symbol,
		"type":   orderType, // buy/sell
		"price":  strconv.FormatFloat(price, 'f', -1, 64),
		"amount": strconv.FormatFloat(amount, 'f', -1, 64),
	}

	err := c.doSpotRequest(ctx, http.MethodPost, "/v1/create_order.do", params, &result)
	if err != nil {
		return "", err
	}

	if result.Result != "true" {
		return "", fmt.Errorf("order placement failed")
	}

	return result.OrderID, nil
}

// CancelSpotOrder cancels a spot order (authenticated)
func (c *RestClient) CancelSpotOrder(ctx context.Context, symbol, orderID string) error {
	if c.credentials == nil {
		return fmt.Errorf("authentication required")
	}

	var result struct {
		Result  string `json:"result"`
		OrderID string `json:"order_id"`
	}

	params := map[string]string{
		"symbol":   symbol,
		"order_id": orderID,
	}

	err := c.doSpotRequest(ctx, http.MethodPost, "/v1/cancel_order.do", params, &result)
	if err != nil {
		return err
	}

	if result.Result != "true" {
		return fmt.Errorf("order cancellation failed")
	}

	return nil
}

// GetSubscribeKey gets a subscribe key for private WebSocket (valid 60 min)
func (c *RestClient) GetSubscribeKey(ctx context.Context) (string, error) {
	if c.credentials == nil {
		return "", fmt.Errorf("authentication required")
	}

	var result struct {
		Result bool   `json:"result"`
		Data   string `json:"data"`
	}

	params := map[string]string{}

	err := c.doSpotRequest(ctx, http.MethodPost, "/v2/subscribe/get_key.do", params, &result)
	if err != nil {
		return "", err
	}

	if !result.Result {
		return "", fmt.Errorf("failed to get subscribe key")
	}

	return result.Data, nil
}

// RefreshSubscribeKey refreshes the subscribe key validity
func (c *RestClient) RefreshSubscribeKey(ctx context.Context, subscribeKey string) error {
	if c.credentials == nil {
		return fmt.Errorf("authentication required")
	}

	var result struct {
		Result bool `json:"result"`
	}

	params := map[string]string{
		"subscribeKey": subscribeKey,
	}

	err := c.doSpotRequest(ctx, http.MethodPost, "/v2/subscribe/refresh_key.do", params, &result)
	if err != nil {
		return err
	}

	if !result.Result {
		return fmt.Errorf("failed to refresh subscribe key")
	}

	return nil
}
