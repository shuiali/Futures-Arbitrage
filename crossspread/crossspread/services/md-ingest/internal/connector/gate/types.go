// Package gate provides types and clients for Gate.io exchange API integration.
// Supports REST and WebSocket APIs for futures market data, trading, and account management.
// API Documentation: https://www.gate.io/docs/developers/apiv4/en/
package gate

import (
	"encoding/json"
	"fmt"
	"time"
)

// =============================================================================
// Common Constants
// =============================================================================

// Settlement currencies
const (
	SettleBTC  = "btc"
	SettleUSDT = "usdt"
)

// Position modes
const (
	PositionModeSingle = "single" // One-way mode
	PositionModeDual   = "dual"   // Hedge mode
)

// Order sides (size)
// Positive = buy/long, Negative = sell/short

// Time in force
const (
	TIFGoodTillCancel    = "gtc" // Good till cancelled
	TIFImmediateOrCancel = "ioc" // Immediate or cancel (taker only)
	TIFPostOnly          = "poc" // Post only (maker only)
	TIFFillOrKill        = "fok" // Fill or kill
)

// Order status
const (
	OrderStatusOpen     = "open"
	OrderStatusFinished = "finished"
)

// Order finish reasons
const (
	FinishAsFilled          = "filled"
	FinishAsCancelled       = "cancelled"
	FinishAsLiquidated      = "liquidated"
	FinishAsIOC             = "ioc"
	FinishAsAutoDeleveraged = "auto_deleveraged"
	FinishAsReduceOnly      = "reduce_only"
	FinishAsPositionClosed  = "position_closed"
	FinishAsSTP             = "stp"
)

// Auto size options for dual mode
const (
	AutoSizeCloseLong  = "close_long"
	AutoSizeCloseShort = "close_short"
)

// Candlestick intervals
const (
	Interval10s = "10s"
	Interval1m  = "1m"
	Interval5m  = "5m"
	Interval15m = "15m"
	Interval30m = "30m"
	Interval1h  = "1h"
	Interval4h  = "4h"
	Interval8h  = "8h"
	Interval1d  = "1d"
	Interval7d  = "7d"
)

// WebSocket update intervals for orderbook
const (
	UpdateInterval20ms   = "20ms"
	UpdateInterval100ms  = "100ms"
	UpdateInterval1000ms = "1000ms"
)

// =============================================================================
// REST API Response Types
// =============================================================================

// Contract represents a futures contract
type Contract struct {
	Name                  string `json:"name"`                     // Contract name, e.g., "BTC_USDT"
	Type                  string `json:"type"`                     // Contract type: "direct", "inverse"
	QuantoMultiplier      string `json:"quanto_multiplier"`        // Multiplier for quanto contracts
	LeverageMin           string `json:"leverage_min"`             // Minimum leverage
	LeverageMax           string `json:"leverage_max"`             // Maximum leverage
	MaintenanceRate       string `json:"maintenance_rate"`         // Maintenance margin rate
	MarkType              string `json:"mark_type"`                // Mark price type: "index", "internal"
	MarkPrice             string `json:"mark_price"`               // Current mark price
	IndexPrice            string `json:"index_price"`              // Current index price
	LastPrice             string `json:"last_price"`               // Last traded price
	FundingRate           string `json:"funding_rate"`             // Current funding rate
	FundingRateIndicative string `json:"funding_rate_indicative"`  // Indicative next funding rate
	FundingInterval       int    `json:"funding_interval"`         // Funding interval in seconds
	FundingNextApply      int64  `json:"funding_next_apply"`       // Next funding timestamp
	OrderSizeMin          int64  `json:"order_size_min"`           // Minimum order size (contracts)
	OrderSizeMax          int64  `json:"order_size_max"`           // Maximum order size (contracts)
	OrderPriceRound       string `json:"order_price_round"`        // Price precision/tick size
	OrderPriceDeviate     string `json:"order_price_deviate"`      // Max price deviation from mark
	RiskLimitBase         string `json:"risk_limit_base"`          // Base risk limit
	RiskLimitStep         string `json:"risk_limit_step"`          // Risk limit step
	RiskLimitMax          string `json:"risk_limit_max"`           // Maximum risk limit
	MakerFeeRate          string `json:"maker_fee_rate"`           // Maker fee rate
	TakerFeeRate          string `json:"taker_fee_rate"`           // Taker fee rate
	RefDiscountRate       string `json:"ref_discount_rate"`        // Referral discount rate
	RefRebateRate         string `json:"ref_rebate_rate"`          // Referral rebate rate
	PositionSize          int64  `json:"position_size"`            // Total position size
	TradeSize             int64  `json:"trade_size"`               // Total trade volume
	ConfigChangeTime      int64  `json:"config_change_time"`       // Last config change time
	InDelisting           bool   `json:"in_delisting"`             // Is delisting
	OrdersLimit           int    `json:"orders_limit"`             // Max open orders
	EnableBonus           bool   `json:"enable_bonus"`             // Bonus enabled
	EnableCredit          bool   `json:"enable_credit"`            // Credit enabled
	CreateTime            int64  `json:"create_time"`              // Contract creation time
	FundingCapRatio       string `json:"funding_cap_ratio"`        // Funding rate cap ratio
	FundingOffset         int    `json:"funding_offset"`           // Funding offset
	ShortUsers            int    `json:"short_users"`              // Number of short users
	LongUsers             int    `json:"long_users"`               // Number of long users
	FundingImpactValue    string `json:"funding_impact_value"`     // Funding impact value
	MarkPriceRound        string `json:"mark_price_round"`         // Mark price precision
	InterestRate          string `json:"interest_rate"`            // Interest rate
	TradeID               int64  `json:"trade_id"`                 // Current trade ID
	OrderbookID           int64  `json:"orderbook_id"`             // Current orderbook ID
	Status                string `json:"status"`                   // Contract status: "trading", "delisting"
	LaunchTime            int64  `json:"launch_time"`              // Launch timestamp
	DelistingTime         int64  `json:"delisting_time,omitempty"` // Delisting timestamp
	DelistedTime          int64  `json:"delisted_time,omitempty"`  // Delisted timestamp
}

// Ticker represents market ticker data
type Ticker struct {
	Contract         string `json:"contract"`                // Contract name
	Last             string `json:"last"`                    // Last price
	ChangePercentage string `json:"change_percentage"`       // 24h change percentage
	TotalSize        string `json:"total_size"`              // Total position size
	Low24h           string `json:"low_24h"`                 // 24h low
	High24h          string `json:"high_24h"`                // 24h high
	Volume24h        string `json:"volume_24h"`              // 24h volume (deprecated)
	Volume24hBTC     string `json:"volume_24h_btc"`          // 24h volume in BTC (deprecated)
	Volume24hUSD     string `json:"volume_24h_usd"`          // 24h volume in USD (deprecated)
	Volume24hBase    string `json:"volume_24h_base"`         // 24h volume in base currency
	Volume24hQuote   string `json:"volume_24h_quote"`        // 24h volume in quote currency
	Volume24hSettle  string `json:"volume_24h_settle"`       // 24h volume in settle currency
	MarkPrice        string `json:"mark_price"`              // Mark price
	FundingRate      string `json:"funding_rate"`            // Current funding rate
	FundingRateInd   string `json:"funding_rate_indicative"` // Indicative funding rate
	IndexPrice       string `json:"index_price"`             // Index price
	QuantoBaseRate   string `json:"quanto_base_rate"`        // Quanto base rate
	LowestAsk        string `json:"lowest_ask"`              // Best ask price
	LowestSize       string `json:"lowest_size"`             // Best ask size
	HighestBid       string `json:"highest_bid"`             // Best bid price
	HighestSize      string `json:"highest_size"`            // Best bid size
	ChangeUTC0       string `json:"change_utc0"`             // Change at UTC0
	ChangeUTC8       string `json:"change_utc8"`             // Change at UTC8
	ChangePrice      string `json:"change_price"`            // 24h price change
}

// OrderBook represents order book depth
type OrderBook struct {
	ID      int64      `json:"id"`      // Orderbook ID
	Current int64      `json:"current"` // Current timestamp (ms)
	Update  int64      `json:"update"`  // Update timestamp (ms)
	Asks    [][]string `json:"asks"`    // Ask levels [[price, size], ...]
	Bids    [][]string `json:"bids"`    // Bid levels [[price, size], ...]
}

// Trade represents a single trade
type Trade struct {
	ID           int64  `json:"id"`             // Trade ID
	CreateTime   int64  `json:"create_time"`    // Create timestamp (seconds)
	CreateTimeMs int64  `json:"create_time_ms"` // Create timestamp (milliseconds)
	Contract     string `json:"contract"`       // Contract name
	Size         int64  `json:"size"`           // Trade size (positive=buy, negative=sell)
	Price        string `json:"price"`          // Trade price
	IsInternal   bool   `json:"is_internal"`    // Is internal trade
}

// Candlestick represents a single candlestick/kline
type Candlestick struct {
	T   int64  `json:"t"`   // Timestamp (seconds)
	V   int64  `json:"v"`   // Volume (contracts)
	C   string `json:"c"`   // Close price
	H   string `json:"h"`   // High price
	L   string `json:"l"`   // Low price
	O   string `json:"o"`   // Open price
	Sum string `json:"sum"` // Volume in base currency
}

// FundingRateHistory represents a historical funding rate record
type FundingRateHistory struct {
	T int64  `json:"t"` // Timestamp
	R string `json:"r"` // Funding rate
}

// =============================================================================
// Account Types
// =============================================================================

// FuturesAccount represents futures account balance
type FuturesAccount struct {
	Total                  string          `json:"total"`                    // Total balance
	UnrealisedPnl          string          `json:"unrealised_pnl"`           // Unrealized PnL
	PositionMargin         string          `json:"position_margin"`          // Position margin
	OrderMargin            string          `json:"order_margin"`             // Order margin
	Available              string          `json:"available"`                // Available balance
	Point                  string          `json:"point"`                    // Point balance
	Bonus                  string          `json:"bonus"`                    // Bonus balance
	Currency               string          `json:"currency"`                 // Settlement currency
	InDualMode             bool            `json:"in_dual_mode"`             // Is in dual mode
	PositionMode           string          `json:"position_mode"`            // Position mode
	EnableCredit           bool            `json:"enable_credit"`            // Credit enabled
	EnableEvolvedClassic   bool            `json:"enable_evolved_classic"`   // New classic mode
	CrossInitialMargin     string          `json:"cross_initial_margin"`     // Cross initial margin
	CrossMaintenanceMargin string          `json:"cross_maintenance_margin"` // Cross maintenance margin
	CrossOrderMargin       string          `json:"cross_order_margin"`       // Cross order margin
	CrossUnrealisedPnl     string          `json:"cross_unrealised_pnl"`     // Cross unrealized PnL
	CrossAvailable         string          `json:"cross_available"`          // Cross available
	CrossMarginBalance     string          `json:"cross_margin_balance"`     // Cross margin balance
	CrossMMR               string          `json:"cross_mmr"`                // Cross MMR
	CrossIMR               string          `json:"cross_imr"`                // Cross IMR
	IsolatedPositionMargin string          `json:"isolated_position_margin"` // Isolated position margin
	EnableNewDualMode      bool            `json:"enable_new_dual_mode"`     // New dual mode
	MarginMode             int             `json:"margin_mode"`              // Margin mode
	EnableTieredMM         bool            `json:"enable_tiered_mm"`         // Tiered MM enabled
	History                *AccountHistory `json:"history,omitempty"`        // Account history
}

// AccountHistory represents account history statistics
type AccountHistory struct {
	DNW         string `json:"dnw"`          // Deposit and withdraw
	PnL         string `json:"pnl"`          // Trading PnL
	Fee         string `json:"fee"`          // Fees
	Refr        string `json:"refr"`         // Referral rebates
	Fund        string `json:"fund"`         // Funding fees
	PointDNW    string `json:"point_dnw"`    // Point deposit/withdraw
	PointFee    string `json:"point_fee"`    // Point fee
	PointRefr   string `json:"point_refr"`   // Point referral
	BonusDNW    string `json:"bonus_dnw"`    // Bonus transfer
	BonusOffset string `json:"bonus_offset"` // Bonus deduction
}

// =============================================================================
// Position Types
// =============================================================================

// Position represents an open position
type Position struct {
	User                   int64       `json:"user"`                     // User ID
	Contract               string      `json:"contract"`                 // Contract name
	Size                   int64       `json:"size"`                     // Position size (positive=long)
	Leverage               string      `json:"leverage"`                 // Leverage (0=cross)
	RiskLimit              string      `json:"risk_limit"`               // Risk limit
	LeverageMax            string      `json:"leverage_max"`             // Max leverage
	MaintenanceRate        string      `json:"maintenance_rate"`         // Maintenance rate
	Value                  string      `json:"value"`                    // Position value
	Margin                 string      `json:"margin"`                   // Position margin
	EntryPrice             string      `json:"entry_price"`              // Entry price
	LiqPrice               string      `json:"liq_price"`                // Liquidation price
	MarkPrice              string      `json:"mark_price"`               // Mark price
	UnrealisedPnl          string      `json:"unrealised_pnl"`           // Unrealized PnL
	RealisedPnl            string      `json:"realised_pnl"`             // Realized PnL
	PnlPnl                 string      `json:"pnl_pnl"`                  // PnL from PnL
	PnlFund                string      `json:"pnl_fund"`                 // PnL from funding
	PnlFee                 string      `json:"pnl_fee"`                  // PnL from fees
	HistoryPnl             string      `json:"history_pnl"`              // History PnL
	LastClosePnl           string      `json:"last_close_pnl"`           // Last close PnL
	RealisedPoint          string      `json:"realised_point"`           // Realized points
	HistoryPoint           string      `json:"history_point"`            // History points
	ADLRanking             int         `json:"adl_ranking"`              // ADL ranking (1-5)
	PendingOrders          int         `json:"pending_orders"`           // Pending order count
	CloseOrder             *CloseOrder `json:"close_order,omitempty"`    // Close order info
	Mode                   string      `json:"mode"`                     // Position mode
	UpdateTime             int64       `json:"update_time"`              // Update timestamp
	UpdateID               int64       `json:"update_id"`                // Update ID
	CrossLeverageLimit     string      `json:"cross_leverage_limit"`     // Cross leverage limit
	RiskLimitTable         string      `json:"risk_limit_table"`         // Risk limit table
	AverageMaintenanceRate string      `json:"average_maintenance_rate"` // Average maintenance rate
}

// CloseOrder represents close order info
type CloseOrder struct {
	ID    int64  `json:"id"`     // Order ID
	Price string `json:"price"`  // Price
	IsLiq bool   `json:"is_liq"` // Is liquidation
}

// =============================================================================
// Order Types
// =============================================================================

// Order represents a futures order
type Order struct {
	ID           int64  `json:"id"`             // Order ID
	User         int64  `json:"user"`           // User ID
	Contract     string `json:"contract"`       // Contract name
	CreateTime   int64  `json:"create_time"`    // Create timestamp (seconds)
	CreateTimeMs int64  `json:"create_time_ms"` // Create timestamp (ms)
	Size         int64  `json:"size"`           // Order size (positive=buy)
	Iceberg      int64  `json:"iceberg"`        // Iceberg display size
	Left         int64  `json:"left"`           // Unfilled quantity
	Price        string `json:"price"`          // Order price
	FillPrice    string `json:"fill_price"`     // Fill price
	MkFr         string `json:"mkfr"`           // Maker fee rate
	TkFr         string `json:"tkfr"`           // Taker fee rate
	TIF          string `json:"tif"`            // Time in force
	RefU         int    `json:"refu"`           // Referrer user ID
	IsReduceOnly bool   `json:"is_reduce_only"` // Is reduce only
	IsClose      bool   `json:"is_close"`       // Is close position
	IsLiq        bool   `json:"is_liq"`         // Is liquidation
	Text         string `json:"text"`           // Custom order ID
	Status       string `json:"status"`         // Order status
	FinishTime   int64  `json:"finish_time"`    // Finish timestamp
	FinishAs     string `json:"finish_as"`      // Finish reason
	StpID        int64  `json:"stp_id"`         // STP group ID
	StpAct       string `json:"stp_act"`        // STP action
	AmendText    string `json:"amend_text"`     // Amendment text
}

// OrderRequest represents order placement request
type OrderRequest struct {
	Contract   string `json:"contract"`              // Contract name (required)
	Size       int64  `json:"size"`                  // Order size (required, positive=buy)
	Iceberg    int64  `json:"iceberg,omitempty"`     // Iceberg display size
	Price      string `json:"price,omitempty"`       // Order price (0+ioc=market)
	Close      bool   `json:"close,omitempty"`       // Close position
	ReduceOnly bool   `json:"reduce_only,omitempty"` // Reduce only
	TIF        string `json:"tif,omitempty"`         // Time in force
	Text       string `json:"text,omitempty"`        // Custom order ID (t-xxx)
	AutoSize   string `json:"auto_size,omitempty"`   // Auto size for dual mode
	StpAct     string `json:"stp_act,omitempty"`     // STP action
}

// OrderAmendRequest represents order amendment request
type OrderAmendRequest struct {
	Price     string `json:"price,omitempty"`      // New price
	Size      int64  `json:"size,omitempty"`       // New size
	AmendText string `json:"amend_text,omitempty"` // Amendment reason
}

// =============================================================================
// User Trade Types
// =============================================================================

// UserTrade represents a user trade record
type UserTrade struct {
	ID           int64  `json:"id"`             // Trade ID
	CreateTime   int64  `json:"create_time"`    // Create timestamp (seconds)
	CreateTimeMs int64  `json:"create_time_ms"` // Create timestamp (ms)
	Contract     string `json:"contract"`       // Contract name
	OrderID      string `json:"order_id"`       // Order ID
	Size         int64  `json:"size"`           // Trade size
	CloseSize    int64  `json:"close_size"`     // Close size
	Price        string `json:"price"`          // Fill price
	Role         string `json:"role"`           // Trade role: "taker", "maker"
	Text         string `json:"text"`           // Custom order ID
	Fee          string `json:"fee"`            // Trade fee
	PointFee     string `json:"point_fee"`      // Point fee
}

// =============================================================================
// Fee Types
// =============================================================================

// TradingFee represents trading fee rates
type TradingFee struct {
	TakerFee string `json:"taker_fee"` // Taker fee rate
	MakerFee string `json:"maker_fee"` // Maker fee rate
}

// WalletFee represents comprehensive fee info
type WalletFee struct {
	UserID             int64  `json:"user_id"`               // User ID
	TakerFee           string `json:"taker_fee"`             // Spot taker fee
	MakerFee           string `json:"maker_fee"`             // Spot maker fee
	GTDiscount         bool   `json:"gt_discount"`           // GT discount enabled
	GTTakerFee         string `json:"gt_taker_fee"`          // GT taker fee
	GTMakerFee         string `json:"gt_maker_fee"`          // GT maker fee
	FuturesTakerFee    string `json:"futures_taker_fee"`     // Futures taker fee
	FuturesMakerFee    string `json:"futures_maker_fee"`     // Futures maker fee
	DeliveryTakerFee   string `json:"delivery_taker_fee"`    // Delivery taker fee
	DeliveryMakerFee   string `json:"delivery_maker_fee"`    // Delivery maker fee
	RPIMakerFee        string `json:"rpi_maker_fee"`         // RPI maker fee
	FuturesRPIMakerFee string `json:"futures_rpi_maker_fee"` // Futures RPI maker fee
	RPIMM              int    `json:"rpi_mm"`                // RPI MM tier
}

// =============================================================================
// Currency/Chain Types
// =============================================================================

// Currency represents a currency
type Currency struct {
	Currency         string      `json:"currency"`          // Currency name
	Delisted         bool        `json:"delisted"`          // Is delisted
	WithdrawDisabled bool        `json:"withdraw_disabled"` // Withdraw disabled
	WithdrawDelayed  bool        `json:"withdraw_delayed"`  // Withdraw delayed
	DepositDisabled  bool        `json:"deposit_disabled"`  // Deposit disabled
	TradeDisabled    bool        `json:"trade_disabled"`    // Trade disabled
	Chains           []ChainInfo `json:"chains,omitempty"`  // Chain info
}

// ChainInfo represents chain information for a currency
type ChainInfo struct {
	Chain              string `json:"chain"`                // Chain name
	NameCn             string `json:"name_cn"`              // Chinese name
	NameEn             string `json:"name_en"`              // English name
	ContractAddress    string `json:"contract_address"`     // Contract address
	IsDisabled         int    `json:"is_disabled"`          // Is disabled
	IsDepositDisabled  int    `json:"is_deposit_disabled"`  // Deposit disabled
	IsWithdrawDisabled int    `json:"is_withdraw_disabled"` // Withdraw disabled
}

// WithdrawStatus represents withdrawal status for a currency
type WithdrawStatus struct {
	Currency        string `json:"currency"`         // Currency
	Name            string `json:"name"`             // Currency name
	NameCn          string `json:"name_cn"`          // Chinese name
	Deposit         string `json:"deposit"`          // Deposit status
	WithdrawPercent string `json:"withdraw_percent"` // Withdraw percent
	WithdrawFix     string `json:"withdraw_fix"`     // Fixed withdraw fee
}

// =============================================================================
// WebSocket Message Types
// =============================================================================

// WSMessage represents a WebSocket message
type WSMessage struct {
	Time    int64           `json:"time"`              // Timestamp (seconds)
	ID      int64           `json:"id,omitempty"`      // Request ID
	Channel string          `json:"channel"`           // Channel name
	Event   string          `json:"event"`             // Event type
	Payload json.RawMessage `json:"payload,omitempty"` // Subscribe params
	Result  json.RawMessage `json:"result,omitempty"`  // Update data
	Error   *WSError        `json:"error,omitempty"`   // Error info
	ReqID   string          `json:"req_id,omitempty"`  // Request ID for trading API
}

// WSError represents WebSocket error
type WSError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// WSSubscribeRequest represents subscribe/unsubscribe request
type WSSubscribeRequest struct {
	Time    int64    `json:"time"`    // Timestamp
	Channel string   `json:"channel"` // Channel name
	Event   string   `json:"event"`   // "subscribe" or "unsubscribe"
	Payload []string `json:"payload"` // Subscribe parameters
}

// WSSubscription is an alias for WSSubscribeRequest
type WSSubscription = WSSubscribeRequest

// WSLoginRequest represents WebSocket login request
type WSLoginRequest struct {
	Time    int64          `json:"time"`    // Timestamp
	Channel string         `json:"channel"` // "futures.login"
	Event   string         `json:"event"`   // "api"
	Payload WSLoginPayload `json:"payload"`
}

// WSLoginPayload represents login payload
type WSLoginPayload struct {
	APIKey    string `json:"api_key"`   // API key
	Signature string `json:"signature"` // Signature
	Timestamp string `json:"timestamp"` // Timestamp string
	ReqID     string `json:"req_id"`    // Request ID
}

// =============================================================================
// WebSocket Trading API Types
// =============================================================================

// WSOrderPlaceRequest represents WebSocket order placement request
type WSOrderPlaceRequest struct {
	Time    int64               `json:"time"`
	Channel string              `json:"channel"` // "futures.order_place"
	Event   string              `json:"event"`   // "api"
	Payload WSOrderPlacePayload `json:"payload"`
}

// WSOrderPlacePayload represents order place payload
type WSOrderPlacePayload struct {
	ReqID    string       `json:"req_id"`    // Request ID
	ReqParam OrderRequest `json:"req_param"` // Order parameters
}

// WSOrderCancelRequest represents WebSocket order cancel request
type WSOrderCancelRequest struct {
	Time    int64                `json:"time"`
	Channel string               `json:"channel"` // "futures.order_cancel"
	Event   string               `json:"event"`   // "api"
	Payload WSOrderCancelPayload `json:"payload"`
}

// WSOrderCancelPayload represents order cancel payload
type WSOrderCancelPayload struct {
	ReqID    string                `json:"req_id"`
	ReqParam WSOrderCancelReqParam `json:"req_param"`
}

// WSOrderCancelReqParam represents cancel request parameters
type WSOrderCancelReqParam struct {
	OrderID string `json:"order_id"` // Order ID to cancel
}

// WSOrderAmendRequest represents WebSocket order amend request
type WSOrderAmendRequest struct {
	Time    int64               `json:"time"`
	Channel string              `json:"channel"` // "futures.order_amend"
	Event   string              `json:"event"`   // "api"
	Payload WSOrderAmendPayload `json:"payload"`
}

// WSOrderAmendPayload represents order amend payload
type WSOrderAmendPayload struct {
	ReqID    string               `json:"req_id"`
	ReqParam WSOrderAmendReqParam `json:"req_param"`
}

// WSOrderAmendReqParam represents amend request parameters
type WSOrderAmendReqParam struct {
	OrderID string `json:"order_id"`        // Order ID to amend
	Price   string `json:"price,omitempty"` // New price
	Size    int64  `json:"size,omitempty"`  // New size
}

// =============================================================================
// WebSocket Push Data Types
// =============================================================================

// WSTickerData represents WebSocket ticker update
type WSTickerData struct {
	Contract         string `json:"contract"`
	Last             string `json:"last"`
	ChangePercentage string `json:"change_percentage"`
	FundingRate      string `json:"funding_rate"`
	FundingRateInd   string `json:"funding_rate_indicative"`
	MarkPrice        string `json:"mark_price"`
	IndexPrice       string `json:"index_price"`
	TotalSize        string `json:"total_size"`
	Volume24h        string `json:"volume_24h"`
	Volume24hBase    string `json:"volume_24h_base"`
	Volume24hQuote   string `json:"volume_24h_quote"`
	Volume24hSettle  string `json:"volume_24h_settle"`
	High24h          string `json:"high_24h"`
	Low24h           string `json:"low_24h"`
	HighestBid       string `json:"highest_bid"`
	LowestAsk        string `json:"lowest_ask"`
}

// WSTradeData represents WebSocket trade update
type WSTradeData struct {
	ID           int64  `json:"id"`
	CreateTime   int64  `json:"create_time"`
	CreateTimeMs int64  `json:"create_time_ms"`
	Contract     string `json:"contract"`
	Size         int64  `json:"size"` // Positive=buy, negative=sell
	Price        string `json:"price"`
	IsInternal   bool   `json:"is_internal"`
}

// WSOrderBookEntry represents a single orderbook level with price and size as objects
type WSOrderBookEntry struct {
	P string  `json:"p"` // Price
	S float64 `json:"s"` // Size (contracts)
}

// WSOrderBookData represents WebSocket orderbook snapshot
type WSOrderBookData struct {
	T        int64              `json:"t"`        // Timestamp (ms)
	ID       int64              `json:"id"`       // Orderbook ID
	Contract string             `json:"contract"` // Contract name
	Asks     []WSOrderBookEntry `json:"asks"`     // [{p, s}, ...]
	Bids     []WSOrderBookEntry `json:"bids"`     // [{p, s}, ...]
}

// WSOrderBookUpdate represents WebSocket orderbook incremental update
type WSOrderBookUpdate struct {
	T  int64      `json:"t"` // Timestamp (ms)
	S  string     `json:"s"` // Contract name
	U  int64      `json:"U"` // First update ID
	UU int64      `json:"u"` // Last update ID
	A  [][]string `json:"a"` // Ask updates [[price, size], ...] size=0 means delete
	B  [][]string `json:"b"` // Bid updates [[price, size], ...]
}

// WSBookTickerData represents WebSocket best bid/offer
type WSBookTickerData struct {
	T  int64  `json:"t"` // Timestamp (ms)
	U  int64  `json:"u"` // Update ID
	S  string `json:"s"` // Contract name
	B  string `json:"b"` // Best bid price
	BS int64  `json:"B"` // Best bid size
	A  string `json:"a"` // Best ask price
	AS int64  `json:"A"` // Best ask size
}

// WSCandlestickData represents WebSocket candlestick update
type WSCandlestickData struct {
	T int64  `json:"t"` // Timestamp
	V int64  `json:"v"` // Volume
	C string `json:"c"` // Close
	H string `json:"h"` // High
	L string `json:"l"` // Low
	O string `json:"o"` // Open
	N string `json:"n"` // Interval
	A string `json:"a"` // Amount
}

// WSKlineData is an alias for WSCandlestickData
type WSKlineData = WSCandlestickData

// =============================================================================
// WebSocket Private Push Types
// =============================================================================

// WSOrderData represents WebSocket order update
type WSOrderData struct {
	Contract     string `json:"contract"`
	CreateTime   int64  `json:"create_time"`
	CreateTimeMs int64  `json:"create_time_ms"`
	FillPrice    string `json:"fill_price"`
	FinishAs     string `json:"finish_as"`
	FinishTime   int64  `json:"finish_time"`
	FinishTimeMs int64  `json:"finish_time_ms"`
	Iceberg      int64  `json:"iceberg"`
	ID           int64  `json:"id"`
	IsClose      bool   `json:"is_close"`
	IsLiq        bool   `json:"is_liq"`
	IsReduceOnly bool   `json:"is_reduce_only"`
	Left         int64  `json:"left"`
	MkFr         string `json:"mkfr"`
	Price        string `json:"price"`
	Size         int64  `json:"size"`
	Status       string `json:"status"`
	Text         string `json:"text"`
	TIF          string `json:"tif"`
	TkFr         string `json:"tkfr"`
	User         string `json:"user"`
}

// WSUserTradeData represents WebSocket user trade update
type WSUserTradeData struct {
	ID           string `json:"id"`
	CreateTime   int64  `json:"create_time"`
	CreateTimeMs int64  `json:"create_time_ms"`
	Contract     string `json:"contract"`
	OrderID      string `json:"order_id"`
	Size         int64  `json:"size"`
	Price        string `json:"price"`
	Role         string `json:"role"` // "taker" or "maker"
	Fee          string `json:"fee"`
}

// WSPositionData represents WebSocket position update
type WSPositionData struct {
	Contract           string `json:"contract"`
	CrossLeverageLimit string `json:"cross_leverage_limit"`
	EntryPrice         string `json:"entry_price"`
	HistoryPnl         string `json:"history_pnl"`
	HistoryPoint       string `json:"history_point"`
	LastClosePnl       string `json:"last_close_pnl"`
	Leverage           string `json:"leverage"`
	LeverageMax        string `json:"leverage_max"`
	LiqPrice           string `json:"liq_price"`
	MaintenanceRate    string `json:"maintenance_rate"`
	Margin             string `json:"margin"`
	Mode               string `json:"mode"`
	RealisedPnl        string `json:"realised_pnl"`
	RealisedPoint      string `json:"realised_point"`
	RiskLimit          string `json:"risk_limit"`
	Size               int64  `json:"size"`
	Time               int64  `json:"time"`
	TimeMs             int64  `json:"time_ms"`
	UnrealisedPnl      string `json:"unrealised_pnl"`
	User               string `json:"user"`
	Value              string `json:"value"`
}

// WSBalanceData represents WebSocket balance update
type WSBalanceData struct {
	Balance string `json:"balance"`
	Change  string `json:"change"`
	Text    string `json:"text"`
	Time    int64  `json:"time"`
	TimeMs  int64  `json:"time_ms"`
	Type    string `json:"type"`
	User    string `json:"user"`
}

// =============================================================================
// Error Types
// =============================================================================

// APIError represents an API error response
type APIError struct {
	Label   string `json:"label"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Gate.io API error [%s]: %s", e.Label, e.Message)
}

// Common error labels
const (
	ErrLabelUserNotFound        = "USER_NOT_FOUND"
	ErrLabelContractNotFound    = "CONTRACT_NOT_FOUND"
	ErrLabelRiskLimitExceeded   = "RISK_LIMIT_EXCEEDED"
	ErrLabelInsufficientBalance = "INSUFFICIENT_BALANCE"
	ErrLabelOrderNotFound       = "ORDER_NOT_FOUND"
	ErrLabelOrderFinished       = "ORDER_FINISHED"
	ErrLabelInvalidPrice        = "INVALID_PRICE"
	ErrLabelInvalidSize         = "INVALID_SIZE"
	ErrLabelFOKNotFill          = "FOK_NOT_FILL"
	ErrLabelInitialMarginTooLow = "INITIAL_MARGIN_TOO_LOW"
	ErrLabelOrderBookNotFound   = "ORDER_BOOK_NOT_FOUND"
	ErrLabelCancelFail          = "CANCEL_FAIL"
)

// =============================================================================
// Utility Functions
// =============================================================================

// ToTime converts Unix timestamp (seconds) to time.Time
func ToTime(ts int64) time.Time {
	return time.Unix(ts, 0)
}

// ToTimeMs converts Unix timestamp (milliseconds) to time.Time
func ToTimeMs(ts int64) time.Time {
	return time.UnixMilli(ts)
}
