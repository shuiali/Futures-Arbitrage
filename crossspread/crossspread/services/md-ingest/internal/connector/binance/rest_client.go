package binance

import (
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
	"time"

	"github.com/rs/zerolog/log"
)

const (
	futuresRestBaseURL = "https://fapi.binance.com"
	sapiBaseURL        = "https://api.binance.com"
	// spotRestBaseURL is available at api.binance.com if needed for spot operations
)

// RestClient handles all REST API calls to Binance
type RestClient struct {
	httpClient *http.Client
	apiKey     string
	secretKey  string
}

// NewRestClient creates a new REST client for Binance
func NewRestClient(apiKey, secretKey string) *RestClient {
	return &RestClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiKey:    apiKey,
		secretKey: secretKey,
	}
}

// =============================================================================
// Public Market Data Endpoints (No Authentication Required)
// =============================================================================

// FetchExchangeInfo fetches all trading symbols and their rules
func (c *RestClient) FetchExchangeInfo(ctx context.Context) (*ExchangeInfoResponse, error) {
	url := fmt.Sprintf("%s/fapi/v1/exchangeInfo", futuresRestBaseURL)

	resp, err := c.doRequest(ctx, "GET", url, nil, false)
	if err != nil {
		return nil, fmt.Errorf("fetch exchange info: %w", err)
	}
	defer resp.Body.Close()

	var result ExchangeInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode exchange info: %w", err)
	}

	return &result, nil
}

// FetchTicker24hr fetches 24hr ticker data for all symbols or a specific symbol
func (c *RestClient) FetchTicker24hr(ctx context.Context, symbol string) ([]Ticker24hr, error) {
	url := fmt.Sprintf("%s/fapi/v1/ticker/24hr", futuresRestBaseURL)
	if symbol != "" {
		url = fmt.Sprintf("%s?symbol=%s", url, symbol)
	}

	resp, err := c.doRequest(ctx, "GET", url, nil, false)
	if err != nil {
		return nil, fmt.Errorf("fetch ticker 24hr: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// If single symbol, response is object; if all symbols, response is array
	if symbol != "" {
		var ticker Ticker24hr
		if err := json.Unmarshal(body, &ticker); err != nil {
			return nil, fmt.Errorf("decode ticker: %w", err)
		}
		return []Ticker24hr{ticker}, nil
	}

	var tickers []Ticker24hr
	if err := json.Unmarshal(body, &tickers); err != nil {
		return nil, fmt.Errorf("decode tickers: %w", err)
	}

	return tickers, nil
}

// FetchPremiumIndex fetches mark price and funding rate for all symbols
func (c *RestClient) FetchPremiumIndex(ctx context.Context, symbol string) ([]PremiumIndex, error) {
	url := fmt.Sprintf("%s/fapi/v1/premiumIndex", futuresRestBaseURL)
	if symbol != "" {
		url = fmt.Sprintf("%s?symbol=%s", url, symbol)
	}

	resp, err := c.doRequest(ctx, "GET", url, nil, false)
	if err != nil {
		return nil, fmt.Errorf("fetch premium index: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if symbol != "" {
		var index PremiumIndex
		if err := json.Unmarshal(body, &index); err != nil {
			return nil, fmt.Errorf("decode premium index: %w", err)
		}
		return []PremiumIndex{index}, nil
	}

	var indices []PremiumIndex
	if err := json.Unmarshal(body, &indices); err != nil {
		return nil, fmt.Errorf("decode premium indices: %w", err)
	}

	return indices, nil
}

// FetchFundingRates fetches funding rate history
func (c *RestClient) FetchFundingRates(ctx context.Context, symbol string, limit int) ([]FundingRateInfo, error) {
	url := fmt.Sprintf("%s/fapi/v1/fundingRate?symbol=%s", futuresRestBaseURL, symbol)
	if limit > 0 {
		url = fmt.Sprintf("%s&limit=%d", url, limit)
	}

	resp, err := c.doRequest(ctx, "GET", url, nil, false)
	if err != nil {
		return nil, fmt.Errorf("fetch funding rates: %w", err)
	}
	defer resp.Body.Close()

	var rates []FundingRateInfo
	if err := json.NewDecoder(resp.Body).Decode(&rates); err != nil {
		return nil, fmt.Errorf("decode funding rates: %w", err)
	}

	return rates, nil
}

// FetchKlines fetches candlestick/kline data for historical price charts
func (c *RestClient) FetchKlines(ctx context.Context, symbol, interval string, limit int, startTime, endTime int64) ([]Kline, error) {
	url := fmt.Sprintf("%s/fapi/v1/klines?symbol=%s&interval=%s", futuresRestBaseURL, symbol, interval)

	if limit > 0 {
		url = fmt.Sprintf("%s&limit=%d", url, limit)
	}
	if startTime > 0 {
		url = fmt.Sprintf("%s&startTime=%d", url, startTime)
	}
	if endTime > 0 {
		url = fmt.Sprintf("%s&endTime=%d", url, endTime)
	}

	resp, err := c.doRequest(ctx, "GET", url, nil, false)
	if err != nil {
		return nil, fmt.Errorf("fetch klines: %w", err)
	}
	defer resp.Body.Close()

	var rawKlines [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawKlines); err != nil {
		return nil, fmt.Errorf("decode klines: %w", err)
	}

	klines := make([]Kline, 0, len(rawKlines))
	for _, k := range rawKlines {
		if len(k) < 11 {
			continue
		}
		kline := Kline{
			OpenTime:                 int64(k[0].(float64)),
			Open:                     k[1].(string),
			High:                     k[2].(string),
			Low:                      k[3].(string),
			Close:                    k[4].(string),
			Volume:                   k[5].(string),
			CloseTime:                int64(k[6].(float64)),
			QuoteAssetVolume:         k[7].(string),
			NumberOfTrades:           int64(k[8].(float64)),
			TakerBuyBaseAssetVolume:  k[9].(string),
			TakerBuyQuoteAssetVolume: k[10].(string),
		}
		klines = append(klines, kline)
	}

	return klines, nil
}

// FetchOpenInterest fetches open interest for a symbol
func (c *RestClient) FetchOpenInterest(ctx context.Context, symbol string) (*OpenInterest, error) {
	url := fmt.Sprintf("%s/fapi/v1/openInterest?symbol=%s", futuresRestBaseURL, symbol)

	resp, err := c.doRequest(ctx, "GET", url, nil, false)
	if err != nil {
		return nil, fmt.Errorf("fetch open interest: %w", err)
	}
	defer resp.Body.Close()

	var oi OpenInterest
	if err := json.NewDecoder(resp.Body).Decode(&oi); err != nil {
		return nil, fmt.Errorf("decode open interest: %w", err)
	}

	return &oi, nil
}

// FetchDepth fetches orderbook depth
func (c *RestClient) FetchDepth(ctx context.Context, symbol string, limit int) (*DepthResponse, error) {
	url := fmt.Sprintf("%s/fapi/v1/depth?symbol=%s&limit=%d", futuresRestBaseURL, symbol, limit)

	resp, err := c.doRequest(ctx, "GET", url, nil, false)
	if err != nil {
		return nil, fmt.Errorf("fetch depth: %w", err)
	}
	defer resp.Body.Close()

	var depth DepthResponse
	if err := json.NewDecoder(resp.Body).Decode(&depth); err != nil {
		return nil, fmt.Errorf("decode depth: %w", err)
	}

	return &depth, nil
}

// =============================================================================
// Authenticated Endpoints (API Key Required)
// =============================================================================

// FetchCoinInfo fetches deposit/withdrawal status for all coins
func (c *RestClient) FetchCoinInfo(ctx context.Context) ([]CoinInfo, error) {
	if c.apiKey == "" || c.secretKey == "" {
		return nil, fmt.Errorf("API key and secret key required for this endpoint")
	}

	params := url.Values{}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	signature := c.sign(params.Encode())
	params.Set("signature", signature)

	url := fmt.Sprintf("%s/sapi/v1/capital/config/getall?%s", sapiBaseURL, params.Encode())

	resp, err := c.doRequest(ctx, "GET", url, nil, true)
	if err != nil {
		return nil, fmt.Errorf("fetch coin info: %w", err)
	}
	defer resp.Body.Close()

	var coins []CoinInfo
	if err := json.NewDecoder(resp.Body).Decode(&coins); err != nil {
		return nil, fmt.Errorf("decode coin info: %w", err)
	}

	return coins, nil
}

// FetchTradeFees fetches trading fees for all symbols
func (c *RestClient) FetchTradeFees(ctx context.Context, symbol string) ([]TradeFee, error) {
	if c.apiKey == "" || c.secretKey == "" {
		return nil, fmt.Errorf("API key and secret key required for this endpoint")
	}

	params := url.Values{}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	if symbol != "" {
		params.Set("symbol", symbol)
	}

	signature := c.sign(params.Encode())
	params.Set("signature", signature)

	url := fmt.Sprintf("%s/sapi/v1/asset/tradeFee?%s", sapiBaseURL, params.Encode())

	resp, err := c.doRequest(ctx, "GET", url, nil, true)
	if err != nil {
		return nil, fmt.Errorf("fetch trade fees: %w", err)
	}
	defer resp.Body.Close()

	var fees []TradeFee
	if err := json.NewDecoder(resp.Body).Decode(&fees); err != nil {
		return nil, fmt.Errorf("decode trade fees: %w", err)
	}

	return fees, nil
}

// FetchFuturesAccount fetches futures account info including positions
func (c *RestClient) FetchFuturesAccount(ctx context.Context) (*FuturesAccountInfo, error) {
	if c.apiKey == "" || c.secretKey == "" {
		return nil, fmt.Errorf("API key and secret key required for this endpoint")
	}

	params := url.Values{}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	signature := c.sign(params.Encode())
	params.Set("signature", signature)

	url := fmt.Sprintf("%s/fapi/v2/account?%s", futuresRestBaseURL, params.Encode())

	resp, err := c.doRequest(ctx, "GET", url, nil, true)
	if err != nil {
		return nil, fmt.Errorf("fetch futures account: %w", err)
	}
	defer resp.Body.Close()

	var account FuturesAccountInfo
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("decode futures account: %w", err)
	}

	return &account, nil
}

// FetchPositionRisk fetches position risk for all symbols
func (c *RestClient) FetchPositionRisk(ctx context.Context, symbol string) ([]PositionRisk, error) {
	if c.apiKey == "" || c.secretKey == "" {
		return nil, fmt.Errorf("API key and secret key required for this endpoint")
	}

	params := url.Values{}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	if symbol != "" {
		params.Set("symbol", symbol)
	}

	signature := c.sign(params.Encode())
	params.Set("signature", signature)

	url := fmt.Sprintf("%s/fapi/v2/positionRisk?%s", futuresRestBaseURL, params.Encode())

	resp, err := c.doRequest(ctx, "GET", url, nil, true)
	if err != nil {
		return nil, fmt.Errorf("fetch position risk: %w", err)
	}
	defer resp.Body.Close()

	var positions []PositionRisk
	if err := json.NewDecoder(resp.Body).Decode(&positions); err != nil {
		return nil, fmt.Errorf("decode position risk: %w", err)
	}

	return positions, nil
}

// CreateListenKey creates a listen key for user data stream
func (c *RestClient) CreateListenKey(ctx context.Context) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("API key required for this endpoint")
	}

	url := fmt.Sprintf("%s/fapi/v1/listenKey", futuresRestBaseURL)

	resp, err := c.doRequest(ctx, "POST", url, nil, true)
	if err != nil {
		return "", fmt.Errorf("create listen key: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		ListenKey string `json:"listenKey"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode listen key: %w", err)
	}

	return result.ListenKey, nil
}

// KeepAliveListenKey keeps the listen key alive (must be called every 30 minutes)
func (c *RestClient) KeepAliveListenKey(ctx context.Context) error {
	if c.apiKey == "" {
		return fmt.Errorf("API key required for this endpoint")
	}

	url := fmt.Sprintf("%s/fapi/v1/listenKey", futuresRestBaseURL)

	resp, err := c.doRequest(ctx, "PUT", url, nil, true)
	if err != nil {
		return fmt.Errorf("keepalive listen key: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// =============================================================================
// High-Level Data Aggregation
// =============================================================================

// FetchAllTokenData fetches comprehensive data for all tokens
func (c *RestClient) FetchAllTokenData(ctx context.Context) (map[string]*TokenData, error) {
	tokens := make(map[string]*TokenData)

	// Fetch exchange info (symbols and trading rules)
	log.Debug().Msg("Fetching Binance exchange info")
	exchangeInfo, err := c.FetchExchangeInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch exchange info: %w", err)
	}

	// Initialize token data from exchange info
	for _, s := range exchangeInfo.Symbols {
		if s.Status != "TRADING" || s.ContractType != "PERPETUAL" {
			continue
		}

		td := &TokenData{
			Symbol:       s.Symbol,
			BaseAsset:    s.BaseAsset,
			QuoteAsset:   s.QuoteAsset,
			ContractType: s.ContractType,
			Status:       s.Status,
			UpdatedAt:    time.Now(),
		}

		// Parse trading rules from filters
		for _, f := range s.Filters {
			switch f.FilterType {
			case "PRICE_FILTER":
				td.TickSize, _ = strconv.ParseFloat(f.TickSize, 64)
			case "LOT_SIZE":
				td.StepSize, _ = strconv.ParseFloat(f.StepSize, 64)
			case "MIN_NOTIONAL":
				td.MinNotional, _ = strconv.ParseFloat(f.Notional, 64)
			}
		}

		tokens[s.Symbol] = td
	}

	log.Debug().Int("symbols", len(tokens)).Msg("Initialized token data from exchange info")

	// Fetch 24hr tickers (price, volume)
	log.Debug().Msg("Fetching Binance 24hr tickers")
	tickers, err := c.FetchTicker24hr(ctx, "")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to fetch 24hr tickers")
	} else {
		for _, t := range tickers {
			if td, ok := tokens[t.Symbol]; ok {
				td.LastPrice, _ = strconv.ParseFloat(t.LastPrice, 64)
				td.Volume24h, _ = strconv.ParseFloat(t.Volume, 64)
				td.QuoteVolume24h, _ = strconv.ParseFloat(t.QuoteVolume, 64)
				td.PriceChange24h, _ = strconv.ParseFloat(t.PriceChange, 64)
				td.PriceChangePct24h, _ = strconv.ParseFloat(t.PriceChangePercent, 64)
				td.HighPrice24h, _ = strconv.ParseFloat(t.HighPrice, 64)
				td.LowPrice24h, _ = strconv.ParseFloat(t.LowPrice, 64)
			}
		}
		log.Debug().Int("tickers", len(tickers)).Msg("Updated token data with 24hr tickers")
	}

	// Fetch premium index (mark price, funding rate)
	log.Debug().Msg("Fetching Binance premium index")
	indices, err := c.FetchPremiumIndex(ctx, "")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to fetch premium index")
	} else {
		for _, idx := range indices {
			if td, ok := tokens[idx.Symbol]; ok {
				td.MarkPrice, _ = strconv.ParseFloat(idx.MarkPrice, 64)
				td.IndexPrice, _ = strconv.ParseFloat(idx.IndexPrice, 64)
				td.FundingRate, _ = strconv.ParseFloat(idx.LastFundingRate, 64)
				td.NextFundingTime = time.UnixMilli(idx.NextFundingTime)
			}
		}
		log.Debug().Int("indices", len(indices)).Msg("Updated token data with premium index")
	}

	// Fetch deposit/withdraw status if authenticated
	if c.apiKey != "" && c.secretKey != "" {
		log.Debug().Msg("Fetching Binance coin info (authenticated)")
		coins, err := c.FetchCoinInfo(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to fetch coin info")
		} else {
			// Create a map of coin info by asset name
			coinMap := make(map[string]*CoinInfo)
			for i := range coins {
				coinMap[coins[i].Coin] = &coins[i]
			}

			// Update token data with deposit/withdraw status
			for _, td := range tokens {
				if coin, ok := coinMap[td.BaseAsset]; ok {
					td.DepositEnabled = coin.DepositAllEnable
					td.WithdrawEnabled = coin.WithdrawAllEnable

					td.Networks = make([]NetworkStatus, 0, len(coin.NetworkList))
					for _, n := range coin.NetworkList {
						fee, _ := strconv.ParseFloat(n.WithdrawFee, 64)
						minW, _ := strconv.ParseFloat(n.WithdrawMin, 64)
						td.Networks = append(td.Networks, NetworkStatus{
							Network:         n.Network,
							DepositEnabled:  n.DepositEnable,
							WithdrawEnabled: n.WithdrawEnable,
							WithdrawFee:     fee,
							MinWithdraw:     minW,
							Busy:            n.Busy,
						})
					}
				}
			}
			log.Debug().Int("coins", len(coins)).Msg("Updated token data with coin info")
		}
	}

	return tokens, nil
}

// FetchHistoricalPrices fetches historical OHLCV data for spread charting
func (c *RestClient) FetchHistoricalPrices(ctx context.Context, symbol, interval string, startTime, endTime time.Time) ([]HistoricalPrice, error) {
	var start, end int64
	if !startTime.IsZero() {
		start = startTime.UnixMilli()
	}
	if !endTime.IsZero() {
		end = endTime.UnixMilli()
	}

	klines, err := c.FetchKlines(ctx, symbol, interval, 1500, start, end)
	if err != nil {
		return nil, err
	}

	prices := make([]HistoricalPrice, 0, len(klines))
	for _, k := range klines {
		open, _ := strconv.ParseFloat(k.Open, 64)
		high, _ := strconv.ParseFloat(k.High, 64)
		low, _ := strconv.ParseFloat(k.Low, 64)
		closeP, _ := strconv.ParseFloat(k.Close, 64)
		vol, _ := strconv.ParseFloat(k.Volume, 64)

		prices = append(prices, HistoricalPrice{
			Timestamp: time.UnixMilli(k.OpenTime),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closeP,
			Volume:    vol,
		})
	}

	return prices, nil
}

// =============================================================================
// Helper Methods
// =============================================================================

func (c *RestClient) doRequest(ctx context.Context, method, url string, body io.Reader, authenticated bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	if authenticated && c.apiKey != "" {
		req.Header.Set("X-MBX-APIKEY", c.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

func (c *RestClient) sign(queryString string) string {
	mac := hmac.New(sha256.New, []byte(c.secretKey))
	mac.Write([]byte(queryString))
	return hex.EncodeToString(mac.Sum(nil))
}
