// Package coinex provides types and clients for CoinEx Perpetual Futures exchange API integration.
// Supports REST and WebSocket APIs for perpetual futures market data, trading, and account management.
// API Documentation: https://docs.coinex.com/api/v2/
package coinex

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// =============================================================================
// Common Constants
// =============================================================================

// Base URLs
const (
	RESTBaseURL        = "https://api.coinex.com/v2"
	WSFuturesURL       = "wss://socket.coinex.com/v2/futures"
	WSSpotURL          = "wss://socket.coinex.com/v2/spot"
	LegacyRESTURL      = "https://api.coinex.com"
	LegacyPerpetualURL = "https://api.coinex.com/perpetual/v1"
)

// Market types
const (
	MarketTypeFutures = "FUTURES"
	MarketTypeSpot    = "SPOT"
	MarketTypeMargin  = "MARGIN"
)

// Margin modes
const (
	MarginModeCross    = "cross"
	MarginModeIsolated = "isolated"
)

// Position sides
const (
	PositionSideLong  = "long"
	PositionSideShort = "short"
)

// Order sides
const (
	OrderSideBuy  = "buy"
	OrderSideSell = "sell"
)

// Order types
const (
	OrderTypeLimit  = "limit"
	OrderTypeMarket = "market"
)

// Order status
const (
	OrderStatusOpen     = "open"
	OrderStatusPartial  = "partial"
	OrderStatusDone     = "done"
	OrderStatusCanceled = "canceled"
)

// Self-trade prevention modes
const (
	STPModeCanelTaker  = "ct"   // Cancel remaining Taker orders
	STPModeCancelMaker = "cm"   // Cancel remaining Maker orders
	STPModeBoth        = "both" // Cancel both
)

// Kline periods
const (
	Kline1Min   = "1min"
	Kline3Min   = "3min"
	Kline5Min   = "5min"
	Kline15Min  = "15min"
	Kline30Min  = "30min"
	Kline1Hour  = "1hour"
	Kline2Hour  = "2hour"
	Kline4Hour  = "4hour"
	Kline6Hour  = "6hour"
	Kline12Hour = "12hour"
	Kline1Day   = "1day"
	Kline3Day   = "3day"
	Kline1Week  = "1week"
)

// Depth levels
const (
	DepthLevel5  = 5
	DepthLevel10 = 10
	DepthLevel20 = 20
	DepthLevel50 = 50
)

// Depth intervals
const (
	DepthIntervalNone = "0"
	DepthInterval001  = "0.01"
	DepthInterval01   = "0.1"
	DepthInterval1    = "1"
	DepthInterval10   = "10"
	DepthInterval100  = "100"
	DepthInterval1000 = "1000"
)

// WebSocket methods
const (
	// Public market data
	WSMethodDepthSubscribe   = "depth.subscribe"
	WSMethodDepthUnsubscribe = "depth.unsubscribe"
	WSMethodDepthUpdate      = "depth.update"
	WSMethodDealsSubscribe   = "deals.subscribe"
	WSMethodDealsUnsubscribe = "deals.unsubscribe"
	WSMethodDealsUpdate      = "deals.update"
	WSMethodStateSubscribe   = "state.subscribe"
	WSMethodStateUnsubscribe = "state.unsubscribe"
	WSMethodStateUpdate      = "state.update"
	WSMethodBBOSubscribe     = "bbo.subscribe"
	WSMethodBBOUnsubscribe   = "bbo.unsubscribe"
	WSMethodBBOUpdate        = "bbo.update"
	WSMethodIndexSubscribe   = "index.subscribe"
	WSMethodIndexUnsubscribe = "index.unsubscribe"
	WSMethodIndexUpdate      = "index.update"

	// User data
	WSMethodServerSign          = "server.sign"
	WSMethodServerPing          = "server.ping"
	WSMethodServerPong          = "server.pong"
	WSMethodOrderSubscribe      = "order.subscribe"
	WSMethodOrderUnsubscribe    = "order.unsubscribe"
	WSMethodOrderUpdate         = "order.update"
	WSMethodPositionSubscribe   = "position.subscribe"
	WSMethodPositionUnsubscribe = "position.unsubscribe"
	WSMethodPositionUpdate      = "position.update"
	WSMethodBalanceSubscribe    = "balance.subscribe"
	WSMethodBalanceUnsubscribe  = "balance.unsubscribe"
	WSMethodBalanceUpdate       = "balance.update"
)

// API error codes
const (
	ErrCodeSuccess             = 0
	ErrCodeServiceBusy         = 3008
	ErrCodeInsufficientBalance = 3109
	ErrCodeMinQuantity         = 3127
	ErrCodePriceDiff           = 3606
	ErrCodeMarketOrderFailed   = 3620
	ErrCodeOrderCanceled       = 3621
	ErrCodePostOnlyFailed      = 3622
	ErrCodeServiceUnavailable  = 4001
	ErrCodeTimeout             = 4002
	ErrCodeInternalError       = 4003
	ErrCodeParamError          = 4004
	ErrCodeInvalidAccessID     = 4005
	ErrCodeSignatureFailed     = 4006
	ErrCodeIPProhibited        = 4007
	ErrCodeInvalidSign         = 4008
	ErrCodeInvalidMethod       = 4009
	ErrCodeExpiredRequest      = 4010
	ErrCodeUserProhibited      = 4011
	ErrCodeTradingProhibited   = 4115
	ErrCodeMarketProhibited    = 4117
	ErrCodeFuturesProhibited   = 4130
	ErrCodeRateLimit           = 4213
)

// WebSocket error codes
const (
	WSErrCodeParamError     = 20001
	WSErrCodeMethodNotFound = 20002
	WSErrCodeAuthRequired   = 21001
	WSErrCodeAuthFailed     = 21002
	WSErrCodeTimeout        = 23001
	WSErrCodeTooFrequent    = 23002
	WSErrCodeInternalError  = 24001
	WSErrCodeServiceUnavail = 24002
)

// =============================================================================
// REST API Response Wrapper
// =============================================================================

// Response represents a standard CoinEx API response
type Response struct {
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data,omitempty"`
	Message string          `json:"message,omitempty"`
}

// IsSuccess checks if the response is successful
func (r *Response) IsSuccess() bool {
	return r.Code == 0
}

// Error returns error if response is not successful
func (r *Response) Error() error {
	if r.IsSuccess() {
		return nil
	}
	return fmt.Errorf("CoinEx API error: code=%d, msg=%s", r.Code, r.Message)
}

// PaginatedResponse represents a paginated response
type PaginatedResponse struct {
	Response
	Pagination Pagination `json:"pagination,omitempty"`
}

// Pagination represents pagination info
type Pagination struct {
	Total   int  `json:"total,omitempty"`
	HasNext bool `json:"has_next"`
}

// =============================================================================
// Market Types
// =============================================================================

// Market represents a futures market
type Market struct {
	Market                 string   `json:"market"`                    // Market name, e.g., "BTCUSDT"
	ContractType           string   `json:"contract_type"`             // Contract type, e.g., "linear"
	MakerFeeRate           string   `json:"maker_fee_rate"`            // Maker fee rate
	TakerFeeRate           string   `json:"taker_fee_rate"`            // Taker fee rate
	MinAmount              string   `json:"min_amount"`                // Minimum transaction volume
	BaseCcy                string   `json:"base_ccy"`                  // Base currency
	QuoteCcy               string   `json:"quote_ccy"`                 // Quote currency
	BaseCcyPrecision       int      `json:"base_ccy_precision"`        // Base currency precision
	QuoteCcyPrecision      int      `json:"quote_ccy_precision"`       // Quote currency precision
	Status                 string   `json:"status"`                    // Market status
	TickSize               string   `json:"tick_size"`                 // Tick size
	Leverage               []string `json:"leverage"`                  // Available leverage values
	OpenInterestVolume     string   `json:"open_interest_volume"`      // Open interest volume
	IsMarketAvailable      bool     `json:"is_market_available"`       // Market availability
	IsCopyTradingAvailable bool     `json:"is_copy_trading_available"` // Copy trading availability
	IsAPITradingAvailable  bool     `json:"is_api_trading_available"`  // API trading availability
}

// =============================================================================
// Ticker Types
// =============================================================================

// Ticker represents 24hr price ticker data
type Ticker struct {
	Market             string `json:"market"`               // Market name
	Last               string `json:"last"`                 // Latest price
	Open               string `json:"open"`                 // Opening price
	Close              string `json:"close"`                // Closing price
	High               string `json:"high"`                 // Highest price
	Low                string `json:"low"`                  // Lowest price
	Volume             string `json:"volume"`               // 24h filled volume
	Value              string `json:"value"`                // 24h filled value
	VolumeSell         string `json:"volume_sell"`          // Taker sell volume
	VolumeBuy          string `json:"volume_buy"`           // Taker buy volume
	IndexPrice         string `json:"index_price"`          // Index price
	MarkPrice          string `json:"mark_price"`           // Mark price
	OpenInterestVolume string `json:"open_interest_volume"` // Position size
	Period             int    `json:"period"`               // Period (86400 = 1 day)
}

// LastPrice returns the last price as float64
func (t *Ticker) LastPrice() float64 {
	v, _ := strconv.ParseFloat(t.Last, 64)
	return v
}

// =============================================================================
// OrderBook Types
// =============================================================================

// Depth represents orderbook depth data
type Depth struct {
	Market string    `json:"market"`  // Market name
	IsFull bool      `json:"is_full"` // Full or incremental
	Depth  DepthData `json:"depth"`   // Depth data
}

// DepthData represents the actual depth levels
type DepthData struct {
	Asks      [][]string `json:"asks"`       // Ask levels [[price, size], ...]
	Bids      [][]string `json:"bids"`       // Bid levels [[price, size], ...]
	Last      string     `json:"last"`       // Latest price
	UpdatedAt int64      `json:"updated_at"` // Timestamp (ms)
	Checksum  uint32     `json:"checksum"`   // CRC32 checksum
}

// DepthLevel represents a single orderbook level
type DepthLevel struct {
	Price    float64
	Quantity float64
}

// ParseAsks parses ask levels from string arrays
func (d *DepthData) ParseAsks() []DepthLevel {
	return parseDepthLevels(d.Asks)
}

// ParseBids parses bid levels from string arrays
func (d *DepthData) ParseBids() []DepthLevel {
	return parseDepthLevels(d.Bids)
}

// parseDepthLevels converts string arrays to DepthLevel
func parseDepthLevels(levels [][]string) []DepthLevel {
	result := make([]DepthLevel, 0, len(levels))
	for _, level := range levels {
		if len(level) >= 2 {
			price, _ := strconv.ParseFloat(level[0], 64)
			qty, _ := strconv.ParseFloat(level[1], 64)
			result = append(result, DepthLevel{Price: price, Quantity: qty})
		}
	}
	return result
}

// =============================================================================
// Trade Types
// =============================================================================

// Deal represents a single trade
type Deal struct {
	DealID    int64  `json:"deal_id"`    // Deal ID
	CreatedAt int64  `json:"created_at"` // Timestamp (ms)
	Side      string `json:"side"`       // "buy" or "sell"
	Price     string `json:"price"`      // Filled price
	Amount    string `json:"amount"`     // Executed amount
}

// =============================================================================
// Kline Types
// =============================================================================

// Kline represents a candlestick
type Kline struct {
	Market    string `json:"market"`     // Market name
	CreatedAt int64  `json:"created_at"` // Timestamp (ms)
	Open      string `json:"open"`       // Open price
	Close     string `json:"close"`      // Close price
	High      string `json:"high"`       // High price
	Low       string `json:"low"`        // Low price
	Volume    string `json:"volume"`     // Filled volume
	Value     string `json:"value"`      // Filled value
}

// =============================================================================
// Funding Rate Types
// =============================================================================

// FundingRate represents current funding rate
type FundingRate struct {
	Market            string `json:"market"`              // Market name
	MarkPrice         string `json:"mark_price"`          // Mark price
	LatestFundingRate string `json:"latest_funding_rate"` // Current funding rate
	NextFundingRate   string `json:"next_funding_rate"`   // Next funding rate
	MaxFundingRate    string `json:"max_funding_rate"`    // Maximum funding rate
	MinFundingRate    string `json:"min_funding_rate"`    // Minimum funding rate
	LatestFundingTime int64  `json:"latest_funding_time"` // Last funding time (ms)
	NextFundingTime   int64  `json:"next_funding_time"`   // Next funding time (ms)
}

// FundingRateHistory represents historical funding rate
type FundingRateHistory struct {
	Market                 string `json:"market"`                   // Market name
	FundingTime            int64  `json:"funding_time"`             // Funding time (ms)
	TheoreticalFundingRate string `json:"theoretical_funding_rate"` // Theoretical rate
	ActualFundingRate      string `json:"actual_funding_rate"`      // Actual rate
}

// =============================================================================
// Index Types
// =============================================================================

// Index represents market index data
type Index struct {
	Market    string        `json:"market"`     // Market name
	CreatedAt int64         `json:"created_at"` // Timestamp (ms)
	Price     string        `json:"price"`      // Index price
	Sources   []IndexSource `json:"sources"`    // Index sources
}

// IndexSource represents an index component
type IndexSource struct {
	Exchange    string `json:"exchange"`     // Exchange name
	CreatedAt   int64  `json:"created_at"`   // Data collection time
	IndexWeight string `json:"index_weight"` // Index weight
}

// =============================================================================
// Balance Types
// =============================================================================

// FuturesBalance represents futures account balance
type FuturesBalance struct {
	Ccy           string `json:"ccy"`            // Currency name
	Available     string `json:"available"`      // Available balance
	Frozen        string `json:"frozen"`         // Frozen balance
	Margin        string `json:"margin"`         // Position margin
	Transferrable string `json:"transferrable"`  // Transferable balance
	UnrealizedPnl string `json:"unrealized_pnl"` // Unrealized PnL
}

// SpotBalance represents spot account balance
type SpotBalance struct {
	Ccy       string `json:"ccy"`       // Currency name
	Available string `json:"available"` // Available balance
	Frozen    string `json:"frozen"`    // Frozen balance
}

// =============================================================================
// Position Types
// =============================================================================

// Position represents a futures position
type Position struct {
	PositionID             int64  `json:"position_id"`              // Position ID
	Market                 string `json:"market"`                   // Market name
	MarketType             string `json:"market_type"`              // Market type
	Side                   string `json:"side"`                     // "long" or "short"
	MarginMode             string `json:"margin_mode"`              // "cross" or "isolated"
	OpenInterest           string `json:"open_interest"`            // Position size
	CloseAvbl              string `json:"close_avbl"`               // Amount available to close
	AthPositionAmount      string `json:"ath_position_amount"`      // ATH position amount
	UnrealizedPnl          string `json:"unrealized_pnl"`           // Unrealized PnL
	RealizedPnl            string `json:"realized_pnl"`             // Realized PnL
	AvgEntryPrice          string `json:"avg_entry_price"`          // Average entry price
	CmlPositionValue       string `json:"cml_position_value"`       // Cumulative position value
	MaxPositionValue       string `json:"max_position_value"`       // Max position value
	TakeProfitPrice        string `json:"take_profit_price"`        // Take profit price
	StopLossPrice          string `json:"stop_loss_price"`          // Stop loss price
	TakeProfitType         string `json:"take_profit_type"`         // Take profit trigger type
	StopLossType           string `json:"stop_loss_type"`           // Stop loss trigger type
	Leverage               string `json:"leverage"`                 // Leverage
	MarginAvbl             string `json:"margin_avbl"`              // Available margin
	AthMarginSize          string `json:"ath_margin_size"`          // ATH margin amount
	PositionMarginRate     string `json:"position_margin_rate"`     // Position margin rate
	MaintenanceMarginRate  string `json:"maintenance_margin_rate"`  // Maintenance margin rate
	MaintenanceMarginValue string `json:"maintenance_margin_value"` // Maintenance margin amount
	LiqPrice               string `json:"liq_price"`                // Liquidation price
	BkrPrice               string `json:"bkr_price"`                // Bankruptcy price
	AdlLevel               int    `json:"adl_level"`                // ADL risk level (1-5)
	SettlePrice            string `json:"settle_price"`             // Settlement price
	SettleValue            string `json:"settle_value"`             // Settlement value
	FirstFilledPrice       string `json:"first_filled_price"`       // First filled price
	LatestFilledPrice      string `json:"latest_filled_price"`      // Latest filled price
	CreatedAt              int64  `json:"created_at"`               // Creation time (ms)
	UpdatedAt              int64  `json:"updated_at"`               // Update time (ms)
}

// =============================================================================
// Order Types
// =============================================================================

// OrderRequest represents order placement request
type OrderRequest struct {
	Market     string `json:"market"`              // Market name (required)
	MarketType string `json:"market_type"`         // "FUTURES" (required)
	Side       string `json:"side"`                // "buy" or "sell" (required)
	Type       string `json:"type"`                // "limit" or "market" (required)
	Amount     string `json:"amount"`              // Order amount (required)
	Price      string `json:"price,omitempty"`     // Order price (required for limit)
	ClientID   string `json:"client_id,omitempty"` // User-defined ID (1-32 chars)
	IsHide     bool   `json:"is_hide,omitempty"`   // Hide order in public depth
	STPMode    string `json:"stp_mode,omitempty"`  // Self-trade prevention mode
}

// Order represents a futures order
type Order struct {
	OrderID          int64  `json:"order_id"`           // Order ID
	Market           string `json:"market"`             // Market name
	MarketType       string `json:"market_type"`        // Market type
	Side             string `json:"side"`               // "buy" or "sell"
	Type             string `json:"type"`               // "limit" or "market"
	Amount           string `json:"amount"`             // Order amount
	Price            string `json:"price"`              // Order price
	UnfilledAmount   string `json:"unfilled_amount"`    // Remaining unfilled
	FilledAmount     string `json:"filled_amount"`      // Filled amount
	FilledValue      string `json:"filled_value"`       // Filled value
	ClientID         string `json:"client_id"`          // Client ID
	Fee              string `json:"fee"`                // Trading fee
	FeeCcy           string `json:"fee_ccy"`            // Fee currency
	MakerFeeRate     string `json:"maker_fee_rate"`     // Maker fee rate
	TakerFeeRate     string `json:"taker_fee_rate"`     // Taker fee rate
	LastFilledAmount string `json:"last_filled_amount"` // Last filled amount
	LastFilledPrice  string `json:"last_filled_price"`  // Last filled price
	RealizedPnl      string `json:"realized_pnl"`       // Realized PnL
	CreatedAt        int64  `json:"created_at"`         // Creation time (ms)
	UpdatedAt        int64  `json:"updated_at"`         // Update time (ms)
}

// CancelOrderRequest represents cancel order request
type CancelOrderRequest struct {
	Market     string `json:"market"`      // Market name
	MarketType string `json:"market_type"` // "FUTURES"
	OrderID    int64  `json:"order_id"`    // Order ID
}

// CancelByClientIDRequest represents cancel by client ID request
type CancelByClientIDRequest struct {
	Market     string `json:"market,omitempty"` // Market name (optional)
	MarketType string `json:"market_type"`      // "FUTURES"
	ClientID   string `json:"client_id"`        // User-defined ID
}

// CancelAllOrdersRequest represents cancel all orders request
type CancelAllOrdersRequest struct {
	Market     string `json:"market"`         // Market name
	MarketType string `json:"market_type"`    // "FUTURES"
	Side       string `json:"side,omitempty"` // Optional: "buy" or "sell"
}

// ClosePositionRequest represents close position request
type ClosePositionRequest struct {
	Market     string `json:"market"`              // Market name
	MarketType string `json:"market_type"`         // "FUTURES"
	Type       string `json:"type"`                // "limit" or "market"
	Price      string `json:"price,omitempty"`     // Required for limit
	Amount     string `json:"amount,omitempty"`    // Amount to close (null = all)
	ClientID   string `json:"client_id,omitempty"` // User-defined ID
	IsHide     bool   `json:"is_hide,omitempty"`   // Hide order
	STPMode    string `json:"stp_mode,omitempty"`  // Self-trade prevention
}

// AdjustLeverageRequest represents leverage adjustment request
type AdjustLeverageRequest struct {
	Market     string `json:"market"`      // Market name
	MarketType string `json:"market_type"` // "FUTURES"
	MarginMode string `json:"margin_mode"` // "cross" or "isolated"
	Leverage   int    `json:"leverage"`    // Leverage value
}

// AdjustLeverageResponse represents leverage adjustment response
type AdjustLeverageResponse struct {
	MarginMode string `json:"margin_mode"` // Position type
	Leverage   int    `json:"leverage"`    // Leverage
}

// =============================================================================
// Deposit/Withdrawal Types
// =============================================================================

// DepositRecord represents a deposit record
type DepositRecord struct {
	DepositID         int64  `json:"deposit_id"`           // Deposit ID
	CreatedAt         int64  `json:"created_at"`           // Creation time
	TxID              string `json:"tx_id"`                // Transaction hash
	Ccy               string `json:"ccy"`                  // Currency name
	Chain             string `json:"chain"`                // Chain name
	DepositMethod     string `json:"deposit_method"`       // "on_chain" or "inter_user"
	Amount            string `json:"amount"`               // Deposit amount
	ActualAmount      string `json:"actual_amount"`        // Actual amount
	ToAddress         string `json:"to_address"`           // Arrival address
	Confirmations     int    `json:"confirmations"`        // Number of confirmations
	Status            string `json:"status"`               // Deposit status
	TxExplorerURL     string `json:"tx_explorer_url"`      // Explorer link
	ToAddrExplorerURL string `json:"to_addr_explorer_url"` // Address explorer link
	Remark            string `json:"remark"`               // Note
}

// WithdrawRecord represents a withdrawal record
type WithdrawRecord struct {
	WithdrawID         int64  `json:"withdraw_id"`          // Withdrawal ID
	CreatedAt          int64  `json:"created_at"`           // Creation time
	Ccy                string `json:"ccy"`                  // Currency name
	Chain              string `json:"chain"`                // Chain name
	WithdrawMethod     string `json:"withdraw_method"`      // "on_chain" or "inter_user"
	Memo               string `json:"memo"`                 // Memo
	Amount             string `json:"amount"`               // Withdrawal amount
	ActualAmount       string `json:"actual_amount"`        // Actual amount
	TxFee              string `json:"tx_fee"`               // Withdrawal fee
	TxID               string `json:"tx_id"`                // Transaction hash
	ToAddress          string `json:"to_address"`           // Withdrawal address
	Confirmations      int    `json:"confirmations"`        // Number of confirmations
	ExplorerAddressURL string `json:"explorer_address_url"` // Address explorer link
	ExplorerTxURL      string `json:"explorer_tx_url"`      // Transaction explorer link
	Status             string `json:"status"`               // Withdrawal status
	Remark             string `json:"remark"`               // Note
}

// =============================================================================
// WebSocket Types
// =============================================================================

// WSRequest represents a WebSocket request
type WSRequest struct {
	Method string      `json:"method"` // Method name
	Params interface{} `json:"params"` // Parameters
	ID     int         `json:"id"`     // Request ID
}

// WSResponse represents a WebSocket response
type WSResponse struct {
	ID      int             `json:"id"`      // Request ID
	Code    int             `json:"code"`    // Response code
	Message string          `json:"message"` // Response message
	Method  string          `json:"method"`  // Method name (for push)
	Data    json.RawMessage `json:"data"`    // Response data
}

// IsSuccess checks if the response is successful
func (r *WSResponse) IsSuccess() bool {
	return r.Code == 0
}

// WSSignParams represents WebSocket sign parameters
type WSSignParams struct {
	AccessID  string `json:"access_id"`  // API access ID
	SignedStr string `json:"signed_str"` // Signature
	Timestamp int64  `json:"timestamp"`  // Timestamp (ms)
}

// WSAuthParams is an alias for WSSignParams
type WSAuthParams = WSSignParams

// WSDepthSubscribeParams represents depth subscription parameters
type WSDepthSubscribeParams struct {
	MarketList [][]interface{} `json:"market_list"` // [[market, limit, interval, is_full], ...]
}

// WSMarketListParams represents market list subscription parameters
type WSMarketListParams struct {
	MarketList []string `json:"market_list"` // List of market names
}

// WSDepthUpdate represents depth update push data
type WSDepthUpdate struct {
	Market string    `json:"market"`  // Market name
	IsFull bool      `json:"is_full"` // Full or incremental
	Depth  DepthData `json:"depth"`   // Depth data
}

// WSDealsUpdate represents deals update push data
type WSDealsUpdate struct {
	Market   string `json:"market"`    // Market name
	DealList []Deal `json:"deal_list"` // List of deals
}

// WSBBOUpdate represents BBO update push data
type WSBBOUpdate struct {
	Market       string `json:"market"`         // Market name
	UpdatedAt    int64  `json:"updated_at"`     // Timestamp (ms)
	BestBidPrice string `json:"best_bid_price"` // Best bid price
	BestBidSize  string `json:"best_bid_size"`  // Best bid size
	BestAskPrice string `json:"best_ask_price"` // Best ask price
	BestAskSize  string `json:"best_ask_size"`  // Best ask size
}

// WSStateUpdate represents market state update push data
type WSStateUpdate struct {
	StateList []MarketState `json:"state_list"` // List of market states
}

// MarketState represents market state data
type MarketState struct {
	Market            string `json:"market"`              // Market name
	Last              string `json:"last"`                // Latest price
	Open              string `json:"open"`                // Opening price
	Close             string `json:"close"`               // Closing price
	High              string `json:"high"`                // Highest price
	Low               string `json:"low"`                 // Lowest price
	Volume            string `json:"volume"`              // 24h volume
	Value             string `json:"value"`               // 24h value
	VolumeSell        string `json:"volume_sell"`         // Best ask size
	VolumeBuy         string `json:"volume_buy"`          // Best bid size
	OpenInterestSize  string `json:"open_interest_size"`  // Current position
	InsuranceFundSize string `json:"insurance_fund_size"` // Insurance fund
	MarkPrice         string `json:"mark_price"`          // Mark price
	IndexPrice        string `json:"index_price"`         // Index price
	LatestFundingRate string `json:"latest_funding_rate"` // Current funding rate
	NextFundingRate   string `json:"next_funding_rate"`   // Next funding rate
	LatestFundingTime int64  `json:"latest_funding_time"` // Last funding time (ms)
	NextFundingTime   int64  `json:"next_funding_time"`   // Next funding time (ms)
	Period            int    `json:"period"`              // Period (86400 = 1 day)
}

// WSIndexUpdate represents index update push data
type WSIndexUpdate struct {
	Market     string `json:"market"`      // Market name
	IndexPrice string `json:"index_price"` // Index price
	MarkPrice  string `json:"mark_price"`  // Mark price
}

// WSOrderUpdate represents order update push data
type WSOrderUpdate struct {
	Event string      `json:"event"` // Event type: "put", "update", "finish"
	Order OrderDetail `json:"order"` // Order detail
}

// OrderDetail represents order detail in WebSocket push
type OrderDetail struct {
	OrderID          int64  `json:"order_id"`           // Order ID
	StopID           int64  `json:"stop_id"`            // Stop order ID
	Market           string `json:"market"`             // Market name
	Side             string `json:"side"`               // "buy" or "sell"
	Type             string `json:"type"`               // Order type
	Amount           string `json:"amount"`             // Order amount
	Price            string `json:"price"`              // Order price
	UnfilledAmount   string `json:"unfilled_amount"`    // Remaining unfilled
	FilledAmount     string `json:"filled_amount"`      // Filled amount
	FilledValue      string `json:"filled_value"`       // Filled value
	Fee              string `json:"fee"`                // Trading fee
	FeeCcy           string `json:"fee_ccy"`            // Fee currency
	TakerFeeRate     string `json:"taker_fee_rate"`     // Taker fee rate
	MakerFeeRate     string `json:"maker_fee_rate"`     // Maker fee rate
	ClientID         string `json:"client_id"`          // Client ID
	LastFilledAmount string `json:"last_filled_amount"` // Last filled amount
	LastFilledPrice  string `json:"last_filled_price"`  // Last filled price
	CreatedAt        int64  `json:"created_at"`         // Creation time (ms)
	UpdatedAt        int64  `json:"updated_at"`         // Update time (ms)
}

// WSPositionUpdate represents position update push data
type WSPositionUpdate struct {
	Event    string         `json:"event"`    // Event type
	Position PositionDetail `json:"position"` // Position detail
}

// PositionDetail represents position detail in WebSocket push
type PositionDetail struct {
	PositionID             int64  `json:"position_id"`              // Position ID
	Market                 string `json:"market"`                   // Market name
	Side                   string `json:"side"`                     // "long" or "short"
	MarginMode             string `json:"margin_mode"`              // "cross" or "isolated"
	OpenInterest           string `json:"open_interest"`            // Position size
	CloseAvbl              string `json:"close_avbl"`               // Amount available to close
	AthPositionAmount      string `json:"ath_position_amount"`      // ATH position amount
	UnrealizedPnl          string `json:"unrealized_pnl"`           // Unrealized PnL
	RealizedPnl            string `json:"realized_pnl"`             // Realized PnL
	AvgEntryPrice          string `json:"avg_entry_price"`          // Average entry price
	CmlPositionValue       string `json:"cml_position_value"`       // Cumulative position value
	MaxPositionValue       string `json:"max_position_value"`       // Max position value
	TakeProfitPrice        string `json:"take_profit_price"`        // Take profit price
	StopLossPrice          string `json:"stop_loss_price"`          // Stop loss price
	TakeProfitType         string `json:"take_profit_type"`         // Take profit trigger type
	StopLossType           string `json:"stop_loss_type"`           // Stop loss trigger type
	Leverage               string `json:"leverage"`                 // Leverage
	MarginAvbl             string `json:"margin_avbl"`              // Available margin
	AthMarginSize          string `json:"ath_margin_size"`          // ATH margin amount
	PositionMarginRate     string `json:"position_margin_rate"`     // Position margin rate
	MaintenanceMarginRate  string `json:"maintenance_margin_rate"`  // Maintenance margin rate
	MaintenanceMarginValue string `json:"maintenance_margin_value"` // Maintenance margin amount
	LiqPrice               string `json:"liq_price"`                // Liquidation price
	BkrPrice               string `json:"bkr_price"`                // Bankruptcy price
	AdlLevel               int    `json:"adl_level"`                // ADL risk level
	SettlePrice            string `json:"settle_price"`             // Settlement price
	SettleValue            string `json:"settle_value"`             // Settlement value
	FirstFilledPrice       string `json:"first_filled_price"`       // First filled price
	LatestFilledPrice      string `json:"latest_filled_price"`      // Latest filled price
	CreatedAt              int64  `json:"created_at"`               // Creation time (ms)
	UpdatedAt              int64  `json:"updated_at"`               // Update time (ms)
}

// WSBalanceUpdate represents balance update push data
type WSBalanceUpdate struct {
	Event   string        `json:"event"`   // Event type
	Balance BalanceDetail `json:"balance"` // Balance detail
}

// BalanceDetail represents balance detail in WebSocket push
type BalanceDetail struct {
	Ccy             string `json:"ccy"`              // Currency
	Available       string `json:"available"`        // Available balance
	Frozen          string `json:"frozen"`           // Frozen balance
	Margin          string `json:"margin"`           // Margin balance
	TransferBalance string `json:"transfer_balance"` // Transferable balance
	UnrealizedPnl   string `json:"unrealized_pnl"`   // Unrealized PnL
	Equity          string `json:"equity"`           // Total equity
	UpdatedAt       int64  `json:"updated_at"`       // Update time (ms)
}

// =============================================================================
// Helper Functions
// =============================================================================

// ParseTimestamp parses millisecond timestamp to time.Time
func ParseTimestamp(ms int64) time.Time {
	return time.UnixMilli(ms)
}

// FormatTimestamp formats time.Time to millisecond timestamp
func FormatTimestamp(t time.Time) int64 {
	return t.UnixMilli()
}

// StringToFloat64 converts string to float64
func StringToFloat64(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// Float64ToString converts float64 to string
func Float64ToString(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// Int64ToString converts int64 to string
func Int64ToString(i int64) string {
	return strconv.FormatInt(i, 10)
}
