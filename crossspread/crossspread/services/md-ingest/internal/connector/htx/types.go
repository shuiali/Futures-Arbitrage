package htx

import (
	"encoding/json"
	"sync"
	"time"
)

// API Base URLs
const (
	// REST API
	RestBaseURL     = "https://api.hbdm.com"
	RestBackupURL   = "https://api.btcgateway.pro"
	RestVietnamURL  = "https://api.hbdm.vn"
	SpotRestBaseURL = "https://api.huobi.pro"

	// WebSocket URLs
	WSMarketURL = "wss://api.hbdm.com/linear-swap-ws"
	WSOrderURL  = "wss://api.hbdm.com/linear-swap-notification"
	WSIndexURL  = "wss://api.hbdm.com/ws_index"
	WSSystemURL = "wss://api.hbdm.com/center-notification"

	// Backup WebSocket URLs
	WSMarketBackupURL = "wss://api.btcgateway.pro/linear-swap-ws"
	WSOrderBackupURL  = "wss://api.btcgateway.pro/linear-swap-notification"
	WSIndexBackupURL  = "wss://api.btcgateway.pro/ws_index"
)

// REST API Paths
const (
	// Reference Data
	PathContractInfo = "/linear-swap-api/v1/swap_contract_info"
	PathPriceLimit   = "/linear-swap-api/v1/swap_price_limit"
	PathOpenInterest = "/linear-swap-api/v1/swap_open_interest"
	PathIndex        = "/linear-swap-api/v1/swap_index"
	PathFee          = "/linear-swap-api/v1/swap_fee"

	// Market Data
	PathDepth        = "/linear-swap-ex/market/depth"
	PathBBO          = "/linear-swap-ex/market/bbo"
	PathKline        = "/linear-swap-ex/market/history/kline"
	PathTicker       = "/linear-swap-ex/market/detail/merged"
	PathBatchTicker  = "/linear-swap-ex/market/detail/batch_merged"
	PathTrade        = "/linear-swap-ex/market/trade"
	PathHistoryTrade = "/linear-swap-ex/market/history/trade"

	// Funding Rate
	PathFundingRate       = "/linear-swap-api/v1/swap_funding_rate"
	PathBatchFundingRate  = "/linear-swap-api/v1/swap_batch_funding_rate"
	PathHistoricalFunding = "/linear-swap-api/v1/swap_historical_funding_rate"

	// Account (Cross Margin)
	PathCrossAccountInfo  = "/linear-swap-api/v1/swap_cross_account_info"
	PathCrossPositionInfo = "/linear-swap-api/v1/swap_cross_position_info"

	// Trading (Cross Margin)
	PathCrossOrder         = "/linear-swap-api/v1/swap_cross_order"
	PathCrossBatchOrder    = "/linear-swap-api/v1/swap_cross_batchorder"
	PathCrossCancel        = "/linear-swap-api/v1/swap_cross_cancel"
	PathCrossCancelAll     = "/linear-swap-api/v1/swap_cross_cancelall"
	PathCrossOrderInfo     = "/linear-swap-api/v1/swap_cross_order_info"
	PathCrossOrderDetail   = "/linear-swap-api/v1/swap_cross_order_detail"
	PathCrossOpenOrders    = "/linear-swap-api/v1/swap_cross_openorders"
	PathCrossHistoryOrders = "/linear-swap-api/v1/swap_cross_hisorders_exact"
	PathCrossMatchResults  = "/linear-swap-api/v1/swap_cross_matchresults_exact"

	// Asset Transfer
	PathTransfer = "/v2/account/transfer"
)

// Authentication Constants
const (
	SignatureMethod    = "HmacSHA256"
	SignatureVersion   = "2"
	WSSignatureVersion = "2.1"
)

// Rate Limits
const (
	PublicRateLimit  = 800 // requests per second per IP
	PrivateRateLimit = 72  // requests per 3 seconds per UID (read)
	TradeRateLimit   = 36  // requests per 3 seconds per UID (trade)
)

// Order Status
const (
	OrderStatusReadyToSubmit1        = 1
	OrderStatusReadyToSubmit2        = 2
	OrderStatusSubmitted             = 3
	OrderStatusPartialFilled         = 4
	OrderStatusPartialFilledCanceled = 5
	OrderStatusFilled                = 6
	OrderStatusCanceled              = 7
	OrderStatusCancelling            = 11
)

// Order Price Types
const (
	OrderPriceLimit        = "limit"
	OrderPriceOpponent     = "opponent"
	OrderPriceOptimal5     = "optimal_5"
	OrderPriceOptimal10    = "optimal_10"
	OrderPriceOptimal20    = "optimal_20"
	OrderPricePostOnly     = "post_only"
	OrderPriceFOK          = "fok"
	OrderPriceIOC          = "ioc"
	OrderPriceOpponentIOC  = "opponent_ioc"
	OrderPriceOptimal5IOC  = "optimal_5_ioc"
	OrderPriceOptimal10IOC = "optimal_10_ioc"
	OrderPriceOptimal20IOC = "optimal_20_ioc"
	OrderPriceOpponentFOK  = "opponent_fok"
	OrderPriceOptimal5FOK  = "optimal_5_fok"
	OrderPriceOptimal10FOK = "optimal_10_fok"
	OrderPriceOptimal20FOK = "optimal_20_fok"
)

// Order Direction
const (
	DirectionBuy  = "buy"
	DirectionSell = "sell"
)

// Order Offset
const (
	OffsetOpen  = "open"
	OffsetClose = "close"
)

// Depth Types (step levels for price aggregation)
const (
	DepthStep0 = "step0" // No aggregation
	DepthStep1 = "step1"
	DepthStep2 = "step2"
	DepthStep3 = "step3"
	DepthStep4 = "step4"
	DepthStep5 = "step5"
)

// KLine Periods
const (
	KLinePeriod1Min  = "1min"
	KLinePeriod5Min  = "5min"
	KLinePeriod15Min = "15min"
	KLinePeriod30Min = "30min"
	KLinePeriod60Min = "60min"
	KLinePeriod4Hour = "4hour"
	KLinePeriod1Day  = "1day"
	KLinePeriod1Week = "1week"
	KLinePeriod1Mon  = "1mon"
)

// Contract Status
const (
	ContractStatusNormal       = 1
	ContractStatusSuspension   = 5
	ContractStatusSettlement   = 6
	ContractStatusInSettlement = 7
	ContractStatusSettled      = 8
)

// WebSocket Message Types
const (
	WSMsgTypeSub   = "sub"
	WSMsgTypeUnsub = "unsub"
	WSMsgTypeReq   = "req"
	WSMsgTypeAuth  = "auth"
	WSMsgTypePong  = "pong"
)

// WebSocket Topics
const (
	// Market Data Topics
	WSTopicKline   = "market.%s.kline.%s"
	WSTopicDepth   = "market.%s.depth.%s"
	WSTopicDepthHF = "market.%s.depth.size_%d.high_freq"
	WSTopicBBO     = "market.%s.bbo"
	WSTopicTrade   = "market.%s.trade.detail"
	WSTopicTicker  = "market.%s.detail"

	// Order Push Topics (Cross)
	WSTopicOrdersCross      = "orders_cross.%s"
	WSTopicMatchOrdersCross = "matchOrders_cross.%s"
	WSTopicAccountsCross    = "accounts_cross.%s"
	WSTopicPositionsCross   = "positions_cross.%s"

	// Public Topics
	WSTopicFundingRate  = "public.%s.funding_rate"
	WSTopicContractInfo = "public.%s.contract_info"
	WSTopicLiquidation  = "public.%s.liquidation_orders"
)

// ========== REST API Response Types ==========

// BaseResponse is the common response wrapper
type BaseResponse struct {
	Status  string          `json:"status"`
	ErrCode int             `json:"err_code,omitempty"`
	ErrMsg  string          `json:"err_msg,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Tick    json.RawMessage `json:"tick,omitempty"`
	Ticks   json.RawMessage `json:"ticks,omitempty"`
	Ch      string          `json:"ch,omitempty"`
	Ts      int64           `json:"ts,omitempty"`
}

// ContractInfo represents contract information
type ContractInfo struct {
	Symbol            string  `json:"symbol"`
	ContractCode      string  `json:"contract_code"`
	ContractSize      float64 `json:"contract_size"`
	PriceTick         float64 `json:"price_tick"`
	SettlementDate    string  `json:"settlement_date,omitempty"`
	DeliveryTime      string  `json:"delivery_time,omitempty"`
	CreateDate        string  `json:"create_date"`
	ContractStatus    int     `json:"contract_status"`
	SupportMarginMode string  `json:"support_margin_mode"`
	BusinessType      string  `json:"business_type,omitempty"`
	Pair              string  `json:"pair,omitempty"`
	ContractType      string  `json:"contract_type,omitempty"`
}

// PriceLimit represents price limit information
type PriceLimit struct {
	Symbol       string  `json:"symbol"`
	ContractCode string  `json:"contract_code"`
	HighLimit    float64 `json:"high_limit"`
	LowLimit     float64 `json:"low_limit"`
}

// OpenInterest represents open interest data
type OpenInterest struct {
	Symbol        string  `json:"symbol"`
	ContractCode  string  `json:"contract_code"`
	Amount        float64 `json:"amount"`
	Volume        float64 `json:"volume"`
	Value         float64 `json:"value"`
	TradeAmount   float64 `json:"trade_amount"`
	TradeVolume   float64 `json:"trade_volume"`
	TradeTurnover float64 `json:"trade_turnover"`
}

// IndexPrice represents index price data
type IndexPrice struct {
	ContractCode string  `json:"contract_code"`
	IndexPrice   float64 `json:"index_price"`
	IndexTs      int64   `json:"index_ts"`
}

// TradingFee represents trading fee information
type TradingFee struct {
	Symbol        string `json:"symbol"`
	ContractCode  string `json:"contract_code"`
	OpenMakerFee  string `json:"open_maker_fee"`
	OpenTakerFee  string `json:"open_taker_fee"`
	CloseMakerFee string `json:"close_maker_fee"`
	CloseTakerFee string `json:"close_taker_fee"`
	FeeAsset      string `json:"fee_asset"`
	DeliveryFee   string `json:"delivery_fee,omitempty"`
}

// DepthData represents order book depth
type DepthData struct {
	Asks    [][]float64 `json:"asks"`
	Bids    [][]float64 `json:"bids"`
	Ch      string      `json:"ch"`
	ID      int64       `json:"id"`
	MrID    int64       `json:"mrid"`
	Ts      int64       `json:"ts"`
	Version int64       `json:"version"`
}

// BBOData represents best bid/offer data
type BBOData struct {
	ContractCode string    `json:"contract_code"`
	MrID         int64     `json:"mrid"`
	Ask          []float64 `json:"ask"`
	Bid          []float64 `json:"bid"`
	Ts           int64     `json:"ts"`
}

// KlineData represents candlestick data
type KlineData struct {
	ID            int64   `json:"id"`
	Open          float64 `json:"open"`
	Close         float64 `json:"close"`
	Low           float64 `json:"low"`
	High          float64 `json:"high"`
	Amount        float64 `json:"amount"`
	Vol           int64   `json:"vol"`
	TradeTurnover float64 `json:"trade_turnover"`
	Count         int64   `json:"count"`
}

// TickerData represents market ticker data
type TickerData struct {
	ID            int64     `json:"id"`
	Ts            int64     `json:"ts"`
	Ask           []float64 `json:"ask"`
	Bid           []float64 `json:"bid"`
	Open          float64   `json:"open"`
	Close         float64   `json:"close"`
	Low           float64   `json:"low"`
	High          float64   `json:"high"`
	Amount        float64   `json:"amount"`
	Vol           int64     `json:"vol"`
	TradeTurnover float64   `json:"trade_turnover"`
	Count         int64     `json:"count"`
}

// BatchTickerData represents batch ticker with contract code
type BatchTickerData struct {
	ContractCode  string    `json:"contract_code"`
	ID            int64     `json:"id"`
	Ts            int64     `json:"ts"`
	Ask           []float64 `json:"ask"`
	Bid           []float64 `json:"bid"`
	Open          float64   `json:"open"`
	Close         float64   `json:"close"`
	Low           float64   `json:"low"`
	High          float64   `json:"high"`
	Amount        float64   `json:"amount"`
	Vol           int64     `json:"vol"`
	TradeTurnover float64   `json:"trade_turnover"`
	Count         int64     `json:"count"`
}

// TradeData represents recent trade data
type TradeData struct {
	ID            int64   `json:"id"`
	Price         float64 `json:"price"`
	Amount        float64 `json:"amount"`
	Direction     string  `json:"direction"`
	Ts            int64   `json:"ts"`
	Quantity      float64 `json:"quantity"`
	TradeTurnover float64 `json:"trade_turnover"`
}

// TradeTick represents trade tick data
type TradeTick struct {
	ID   int64       `json:"id"`
	Ts   int64       `json:"ts"`
	Data []TradeData `json:"data"`
}

// FundingRate represents funding rate data
type FundingRate struct {
	Symbol          string `json:"symbol"`
	ContractCode    string `json:"contract_code"`
	FeeAsset        string `json:"fee_asset"`
	FundingTime     string `json:"funding_time"`
	FundingRate     string `json:"funding_rate"`
	EstimatedRate   string `json:"estimated_rate,omitempty"`
	NextFundingTime string `json:"next_funding_time,omitempty"`
	RealizedRate    string `json:"realized_rate,omitempty"`
}

// ========== Account Types ==========

// CrossAccountInfo represents cross margin account information
type CrossAccountInfo struct {
	MarginMode            string               `json:"margin_mode"`
	MarginAccount         string               `json:"margin_account"`
	MarginAsset           string               `json:"margin_asset"`
	MarginBalance         float64              `json:"margin_balance"`
	MarginStatic          float64              `json:"margin_static"`
	MarginPosition        float64              `json:"margin_position"`
	MarginFrozen          float64              `json:"margin_frozen"`
	ProfitReal            float64              `json:"profit_real"`
	ProfitUnreal          float64              `json:"profit_unreal"`
	WithdrawAvailable     float64              `json:"withdraw_available"`
	RiskRate              float64              `json:"risk_rate,omitempty"`
	ContractDetail        []ContractDetailItem `json:"contract_detail,omitempty"`
	FuturesContractDetail []ContractDetailItem `json:"futures_contract_detail,omitempty"`
}

// ContractDetailItem represents contract detail in account
type ContractDetailItem struct {
	Symbol           string  `json:"symbol"`
	ContractCode     string  `json:"contract_code"`
	MarginPosition   float64 `json:"margin_position"`
	MarginFrozen     float64 `json:"margin_frozen"`
	MarginAvailable  float64 `json:"margin_available"`
	ProfitUnreal     float64 `json:"profit_unreal"`
	LiquidationPrice float64 `json:"liquidation_price,omitempty"`
	LeverRate        int     `json:"lever_rate"`
	AdjustFactor     float64 `json:"adjust_factor"`
	ContractType     string  `json:"contract_type,omitempty"`
	Pair             string  `json:"pair,omitempty"`
	BusinessType     string  `json:"business_type,omitempty"`
}

// CrossPositionInfo represents cross margin position information
type CrossPositionInfo struct {
	Symbol         string  `json:"symbol"`
	ContractCode   string  `json:"contract_code"`
	Volume         float64 `json:"volume"`
	Available      float64 `json:"available"`
	Frozen         float64 `json:"frozen"`
	CostOpen       float64 `json:"cost_open"`
	CostHold       float64 `json:"cost_hold"`
	ProfitUnreal   float64 `json:"profit_unreal"`
	ProfitRate     float64 `json:"profit_rate"`
	Profit         float64 `json:"profit"`
	MarginAsset    string  `json:"margin_asset"`
	PositionMargin float64 `json:"position_margin"`
	LeverRate      int     `json:"lever_rate"`
	Direction      string  `json:"direction"`
	LastPrice      float64 `json:"last_price"`
	MarginMode     string  `json:"margin_mode"`
	MarginAccount  string  `json:"margin_account"`
	ContractType   string  `json:"contract_type,omitempty"`
	Pair           string  `json:"pair,omitempty"`
	BusinessType   string  `json:"business_type,omitempty"`
}

// ========== Order Types ==========

// OrderRequest represents an order placement request
type OrderRequest struct {
	ContractCode     string  `json:"contract_code"`
	Volume           int64   `json:"volume"`
	Direction        string  `json:"direction"`
	Offset           string  `json:"offset"`
	LeverRate        int     `json:"lever_rate"`
	OrderPriceType   string  `json:"order_price_type"`
	Price            float64 `json:"price,omitempty"`
	ClientOrderID    int64   `json:"client_order_id,omitempty"`
	ReduceOnly       int     `json:"reduce_only,omitempty"`
	TpTriggerPrice   float64 `json:"tp_trigger_price,omitempty"`
	TpOrderPrice     float64 `json:"tp_order_price,omitempty"`
	TpOrderPriceType string  `json:"tp_order_price_type,omitempty"`
	SlTriggerPrice   float64 `json:"sl_trigger_price,omitempty"`
	SlOrderPrice     float64 `json:"sl_order_price,omitempty"`
	SlOrderPriceType string  `json:"sl_order_price_type,omitempty"`
}

// BatchOrderRequest represents batch order request
type BatchOrderRequest struct {
	OrdersData []OrderRequest `json:"orders_data"`
}

// OrderResponse represents order placement response
type OrderResponse struct {
	OrderID       int64  `json:"order_id"`
	OrderIDStr    string `json:"order_id_str"`
	ClientOrderID int64  `json:"client_order_id,omitempty"`
}

// BatchOrderResponse represents batch order response
type BatchOrderResponse struct {
	Errors  []BatchOrderError `json:"errors,omitempty"`
	Success []OrderResponse   `json:"success,omitempty"`
}

// BatchOrderError represents batch order error
type BatchOrderError struct {
	Index   int    `json:"index"`
	ErrCode int    `json:"err_code"`
	ErrMsg  string `json:"err_msg"`
}

// CancelRequest represents cancel order request
type CancelRequest struct {
	OrderID       string `json:"order_id,omitempty"`
	ClientOrderID string `json:"client_order_id,omitempty"`
	ContractCode  string `json:"contract_code"`
}

// CancelResponse represents cancel order response
type CancelResponse struct {
	Errors    []CancelError `json:"errors,omitempty"`
	Successes string        `json:"successes"`
}

// CancelError represents cancel order error
type CancelError struct {
	OrderID string `json:"order_id"`
	ErrCode int    `json:"err_code"`
	ErrMsg  string `json:"err_msg"`
}

// OrderInfo represents order information
type OrderInfo struct {
	Symbol          string  `json:"symbol"`
	ContractCode    string  `json:"contract_code"`
	Volume          float64 `json:"volume"`
	Price           float64 `json:"price"`
	OrderPriceType  string  `json:"order_price_type"`
	Direction       string  `json:"direction"`
	Offset          string  `json:"offset"`
	LeverRate       int     `json:"lever_rate"`
	OrderID         int64   `json:"order_id"`
	OrderIDStr      string  `json:"order_id_str"`
	ClientOrderID   int64   `json:"client_order_id,omitempty"`
	CreatedAt       int64   `json:"created_at"`
	TradeVolume     float64 `json:"trade_volume"`
	TradeTurnover   float64 `json:"trade_turnover"`
	Fee             float64 `json:"fee"`
	TradeAvgPrice   float64 `json:"trade_avg_price"`
	MarginFrozen    float64 `json:"margin_frozen"`
	Profit          float64 `json:"profit"`
	Status          int     `json:"status"`
	OrderType       int     `json:"order_type"`
	OrderSource     string  `json:"order_source"`
	FeeAsset        string  `json:"fee_asset"`
	LiquidationType string  `json:"liquidation_type,omitempty"`
	CanceledAt      int64   `json:"canceled_at,omitempty"`
	MarginAsset     string  `json:"margin_asset"`
	MarginMode      string  `json:"margin_mode"`
	MarginAccount   string  `json:"margin_account"`
	IsTpsl          int     `json:"is_tpsl,omitempty"`
	RealProfit      float64 `json:"real_profit,omitempty"`
	ReduceOnly      int     `json:"reduce_only,omitempty"`
	ContractType    string  `json:"contract_type,omitempty"`
	Pair            string  `json:"pair,omitempty"`
	BusinessType    string  `json:"business_type,omitempty"`
}

// TradeDetail represents trade detail
type TradeDetail struct {
	TradeID       int64   `json:"trade_id"`
	ID            string  `json:"id"`
	TradePrice    float64 `json:"trade_price"`
	TradeVolume   float64 `json:"trade_volume"`
	TradeTurnover float64 `json:"trade_turnover"`
	TradeFee      float64 `json:"trade_fee"`
	Role          string  `json:"role"`
	CreatedAt     int64   `json:"created_at"`
	FeeAsset      string  `json:"fee_asset"`
	RealProfit    float64 `json:"real_profit,omitempty"`
	Profit        float64 `json:"profit,omitempty"`
}

// OrderDetail represents order detail with trades
type OrderDetail struct {
	OrderInfo
	Trades      []TradeDetail `json:"trades,omitempty"`
	TotalPage   int           `json:"total_page,omitempty"`
	CurrentPage int           `json:"current_page,omitempty"`
	TotalSize   int           `json:"total_size,omitempty"`
}

// OpenOrdersResponse represents open orders list response
type OpenOrdersResponse struct {
	Orders      []OrderInfo `json:"orders"`
	TotalPage   int         `json:"total_page"`
	CurrentPage int         `json:"current_page"`
	TotalSize   int         `json:"total_size"`
}

// ========== WebSocket Types ==========

// WSRequest represents a WebSocket subscription request
type WSRequest struct {
	Sub      string `json:"sub,omitempty"`
	Unsub    string `json:"unsub,omitempty"`
	Req      string `json:"req,omitempty"`
	ID       string `json:"id,omitempty"`
	DataType string `json:"data_type,omitempty"`
}

// WSAuthRequest represents WebSocket authentication request
type WSAuthRequest struct {
	Op               string `json:"op"`
	Type             string `json:"type"`
	AccessKeyID      string `json:"AccessKeyId"`
	SignatureMethod  string `json:"SignatureMethod"`
	SignatureVersion string `json:"SignatureVersion"`
	Timestamp        string `json:"Timestamp"`
	Signature        string `json:"Signature"`
}

// WSOrderRequest represents WebSocket order subscription request
type WSOrderRequest struct {
	Op    string `json:"op"`
	Topic string `json:"topic"`
	Cid   string `json:"cid,omitempty"`
}

// WSPong represents WebSocket pong message
type WSPong struct {
	Pong int64 `json:"pong"`
}

// WSPongOp represents WebSocket pong message with op format
type WSPongOp struct {
	Op string `json:"op"`
	Ts string `json:"ts"`
}

// WSResponse represents generic WebSocket response
type WSResponse struct {
	Op      string          `json:"op,omitempty"`
	Ch      string          `json:"ch,omitempty"`
	Topic   string          `json:"topic,omitempty"`
	Ts      int64           `json:"ts,omitempty"`
	Status  string          `json:"status,omitempty"`
	ErrCode int             `json:"err-code,omitempty"`
	ErrMsg  string          `json:"err-msg,omitempty"`
	Ping    int64           `json:"ping,omitempty"`
	Subbed  string          `json:"subbed,omitempty"`
	ID      string          `json:"id,omitempty"`
	Tick    json.RawMessage `json:"tick,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Rep     string          `json:"rep,omitempty"`
}

// WSAuthResponse represents WebSocket auth response
type WSAuthResponse struct {
	Op      string `json:"op"`
	Type    string `json:"type"`
	ErrCode int    `json:"err-code"`
	ErrMsg  string `json:"err-msg,omitempty"`
	Ts      int64  `json:"ts"`
}

// WSDepthTick represents WebSocket depth tick data
type WSDepthTick struct {
	Asks    [][]float64 `json:"asks"`
	Bids    [][]float64 `json:"bids"`
	Ch      string      `json:"ch,omitempty"`
	Event   string      `json:"event,omitempty"`
	ID      int64       `json:"id"`
	MrID    int64       `json:"mrid"`
	Ts      int64       `json:"ts"`
	Version int64       `json:"version"`
}

// WSKlineTick represents WebSocket kline tick data
type WSKlineTick struct {
	ID            int64   `json:"id"`
	MrID          int64   `json:"mrid"`
	Open          float64 `json:"open"`
	Close         float64 `json:"close"`
	Low           float64 `json:"low"`
	High          float64 `json:"high"`
	Amount        float64 `json:"amount"`
	Vol           int64   `json:"vol"`
	TradeTurnover float64 `json:"trade_turnover"`
	Count         int64   `json:"count"`
}

// WSBBOTick represents WebSocket BBO tick data
type WSBBOTick struct {
	MrID    int64     `json:"mrid"`
	ID      int64     `json:"id"`
	Bid     []float64 `json:"bid"`
	Ask     []float64 `json:"ask"`
	Ts      int64     `json:"ts"`
	Version int64     `json:"version"`
}

// WSTradeTick represents WebSocket trade tick data
type WSTradeTick struct {
	ID   int64       `json:"id"`
	Ts   int64       `json:"ts"`
	Data []TradeData `json:"data"`
}

// WSOrderNotify represents WebSocket order notification
type WSOrderNotify struct {
	Op              string        `json:"op"`
	Topic           string        `json:"topic"`
	Ts              int64         `json:"ts"`
	UID             string        `json:"uid"`
	Symbol          string        `json:"symbol"`
	ContractCode    string        `json:"contract_code"`
	Volume          float64       `json:"volume"`
	Price           float64       `json:"price"`
	OrderPriceType  string        `json:"order_price_type"`
	Direction       string        `json:"direction"`
	Offset          string        `json:"offset"`
	Status          int           `json:"status"`
	LeverRate       int           `json:"lever_rate"`
	OrderID         int64         `json:"order_id"`
	OrderIDStr      string        `json:"order_id_str"`
	ClientOrderID   int64         `json:"client_order_id,omitempty"`
	OrderSource     string        `json:"order_source"`
	OrderType       int           `json:"order_type"`
	CreatedAt       int64         `json:"created_at"`
	TradeVolume     float64       `json:"trade_volume"`
	TradeTurnover   float64       `json:"trade_turnover"`
	Fee             float64       `json:"fee"`
	TradeAvgPrice   float64       `json:"trade_avg_price"`
	MarginFrozen    float64       `json:"margin_frozen"`
	Profit          float64       `json:"profit"`
	MarginMode      string        `json:"margin_mode"`
	MarginAccount   string        `json:"margin_account"`
	Trade           []TradeDetail `json:"trade,omitempty"`
	CanceledAt      int64         `json:"canceled_at,omitempty"`
	FeeAsset        string        `json:"fee_asset"`
	MarginAsset     string        `json:"margin_asset"`
	LiquidationType string        `json:"liquidation_type,omitempty"`
	IsTpsl          int           `json:"is_tpsl,omitempty"`
	RealProfit      float64       `json:"real_profit,omitempty"`
	ReduceOnly      int           `json:"reduce_only,omitempty"`
}

// WSAccountNotify represents WebSocket account notification
type WSAccountNotify struct {
	Op    string             `json:"op"`
	Topic string             `json:"topic"`
	Ts    int64              `json:"ts"`
	UID   string             `json:"uid"`
	Event string             `json:"event"`
	Data  []CrossAccountInfo `json:"data"`
}

// WSPositionNotify represents WebSocket position notification
type WSPositionNotify struct {
	Op    string              `json:"op"`
	Topic string              `json:"topic"`
	Ts    int64               `json:"ts"`
	UID   string              `json:"uid"`
	Event string              `json:"event"`
	Data  []CrossPositionInfo `json:"data"`
}

// WSFundingRateNotify represents WebSocket funding rate notification
type WSFundingRateNotify struct {
	Op    string      `json:"op"`
	Topic string      `json:"topic"`
	Ts    int64       `json:"ts"`
	Data  FundingRate `json:"data"`
}

// ========== Internal Types ==========

// Credentials holds API credentials
type Credentials struct {
	APIKey    string
	SecretKey string
}

// OrderBook represents the local order book
type OrderBook struct {
	Symbol    string
	Asks      [][]float64
	Bids      [][]float64
	Timestamp int64
	Version   int64
	mu        sync.RWMutex
}

// NewOrderBook creates a new order book
func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		Symbol: symbol,
		Asks:   make([][]float64, 0),
		Bids:   make([][]float64, 0),
	}
}

// Update updates the order book with new data
func (ob *OrderBook) Update(asks, bids [][]float64, ts, version int64) {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	ob.Asks = asks
	ob.Bids = bids
	ob.Timestamp = ts
	ob.Version = version
}

// GetSnapshot returns a copy of the order book
func (ob *OrderBook) GetSnapshot() ([][]float64, [][]float64, int64) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	asks := make([][]float64, len(ob.Asks))
	for i, ask := range ob.Asks {
		asks[i] = make([]float64, len(ask))
		copy(asks[i], ask)
	}

	bids := make([][]float64, len(ob.Bids))
	for i, bid := range ob.Bids {
		bids[i] = make([]float64, len(bid))
		copy(bids[i], bid)
	}

	return asks, bids, ob.Timestamp
}

// Subscription represents a WebSocket subscription
type Subscription struct {
	Topic    string
	Callback func(data []byte)
}

// SubscriptionManager manages WebSocket subscriptions
type SubscriptionManager struct {
	subscriptions map[string]*Subscription
	mu            sync.RWMutex
}

// NewSubscriptionManager creates a new subscription manager
func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{
		subscriptions: make(map[string]*Subscription),
	}
}

// Add adds a subscription
func (sm *SubscriptionManager) Add(topic string, callback func(data []byte)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.subscriptions[topic] = &Subscription{
		Topic:    topic,
		Callback: callback,
	}
}

// Remove removes a subscription
func (sm *SubscriptionManager) Remove(topic string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.subscriptions, topic)
}

// Get gets a subscription
func (sm *SubscriptionManager) Get(topic string) (*Subscription, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	sub, ok := sm.subscriptions[topic]
	return sub, ok
}

// GetAll gets all subscriptions
func (sm *SubscriptionManager) GetAll() []*Subscription {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	subs := make([]*Subscription, 0, len(sm.subscriptions))
	for _, sub := range sm.subscriptions {
		subs = append(subs, sub)
	}
	return subs
}

// ConnectionState represents WebSocket connection state
type ConnectionState int

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
	StateAuthenticated
	StateReconnecting
)

// String returns the string representation of the connection state
func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateAuthenticated:
		return "authenticated"
	case StateReconnecting:
		return "reconnecting"
	default:
		return "unknown"
	}
}

// RateLimiter implements rate limiting
type RateLimiter struct {
	tokens     chan struct{}
	refillRate time.Duration
	mu         sync.Mutex
	stopChan   chan struct{}
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(maxRequests int, period time.Duration) *RateLimiter {
	rl := &RateLimiter{
		tokens:     make(chan struct{}, maxRequests),
		refillRate: period / time.Duration(maxRequests),
		stopChan:   make(chan struct{}),
	}

	// Fill initial tokens
	for i := 0; i < maxRequests; i++ {
		rl.tokens <- struct{}{}
	}

	// Start refill goroutine
	go rl.refill()

	return rl
}

// refill periodically refills tokens
func (rl *RateLimiter) refill() {
	ticker := time.NewTicker(rl.refillRate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			select {
			case rl.tokens <- struct{}{}:
			default:
				// Bucket full
			}
		case <-rl.stopChan:
			return
		}
	}
}

// Acquire acquires a token (blocks if none available)
func (rl *RateLimiter) Acquire() {
	<-rl.tokens
}

// TryAcquire tries to acquire a token without blocking
func (rl *RateLimiter) TryAcquire() bool {
	select {
	case <-rl.tokens:
		return true
	default:
		return false
	}
}

// Stop stops the rate limiter
func (rl *RateLimiter) Stop() {
	close(rl.stopChan)
}
