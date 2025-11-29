// Package mexc provides types and clients for MEXC exchange API integration.
// Supports REST and WebSocket APIs for futures market data, trading, and account management.
// API Documentation: https://contract.mexc.com
package mexc

import (
	"encoding/json"
	"fmt"
	"time"
)

// =============================================================================
// Common Constants
// =============================================================================

// Margin/Position types
const (
	OpenTypeIsolated = 1 // Isolated margin
	OpenTypeCross    = 2 // Cross margin
)

// Position types
const (
	PositionTypeLong  = 1 // Long position
	PositionTypeShort = 2 // Short position
)

// Order sides
const (
	SideOpenLong   = 1 // Open long
	SideCloseShort = 2 // Close short
	SideOpenShort  = 3 // Open short
	SideCloseLong  = 4 // Close long
)

// Order types
const (
	OrderTypeLimit     = 1 // Limit order
	OrderTypePostOnly  = 2 // Post-only (maker only)
	OrderTypeIOC       = 3 // Immediate or cancel
	OrderTypeFOK       = 4 // Fill or kill
	OrderTypeMarket    = 5 // Market order
	OrderTypeMarketIOC = 6 // Market IOC
)

// Position modes
const (
	PositionModeOneWay = 1 // One-way mode
	PositionModeHedge  = 2 // Hedge mode
)

// Order states
const (
	OrderStateNew       = 1 // New order
	OrderStatePartial   = 2 // Partially filled
	OrderStateFilled    = 3 // Fully filled
	OrderStateCanceled  = 4 // Canceled
	OrderStateCanceling = 5 // Canceling
)

// Contract states
const (
	ContractStateActive  = 0 // Active
	ContractStateSuspend = 1 // Suspended
)

// =============================================================================
// Timestamp Type
// =============================================================================

// Timestamp is a custom time type for MEXC API timestamps (milliseconds)
type Timestamp int64

// Time returns the time.Time representation
func (t Timestamp) Time() time.Time {
	return time.UnixMilli(int64(t))
}

// UnmarshalJSON implements json.Unmarshaler
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	var n int64
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*t = Timestamp(n)
	return nil
}

// =============================================================================
// REST API Response Types
// =============================================================================

// APIResponse is the common response wrapper for all REST API calls
type APIResponse[T any] struct {
	Success bool   `json:"success"`
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
	Data    T      `json:"data"`
}

// PaginatedResponse wraps paginated responses
type PaginatedResponse[T any] struct {
	PageNum    int `json:"pageNum"`
	PageSize   int `json:"pageSize"`
	TotalPage  int `json:"totalPage"`
	TotalCount int `json:"totalCount"`
	Data       []T `json:"data"`
}

// =============================================================================
// Contract / Instrument Types
// =============================================================================

// Contract represents a trading contract (futures instrument)
type Contract struct {
	Symbol                    string   `json:"symbol"`                    // Trading pair, e.g., "BTC_USDT"
	DisplayName               string   `json:"displayName"`               // Display name
	DisplayNameEn             string   `json:"displayNameEn"`             // English display name
	PositionOpenType          int      `json:"positionOpenType"`          // 1=isolated, 2=cross, 3=both
	BaseCoin                  string   `json:"baseCoin"`                  // Base currency, e.g., "BTC"
	QuoteCoin                 string   `json:"quoteCoin"`                 // Quote currency, e.g., "USDT"
	SettleCoin                string   `json:"settleCoin"`                // Settlement currency
	ContractSize              float64  `json:"contractSize"`              // Contract size
	MinLeverage               int      `json:"minLeverage"`               // Minimum leverage
	MaxLeverage               int      `json:"maxLeverage"`               // Maximum leverage
	PriceScale                int      `json:"priceScale"`                // Price decimal places
	VolScale                  int      `json:"volScale"`                  // Volume decimal places
	AmountScale               int      `json:"amountScale"`               // Amount decimal places
	PriceUnit                 float64  `json:"priceUnit"`                 // Minimum price increment (tick size)
	VolUnit                   float64  `json:"volUnit"`                   // Minimum volume increment (lot size)
	MinVol                    float64  `json:"minVol"`                    // Minimum order volume
	MaxVol                    float64  `json:"maxVol"`                    // Maximum order volume
	BidLimitPriceRate         float64  `json:"bidLimitPriceRate"`         // Buy limit price rate
	AskLimitPriceRate         float64  `json:"askLimitPriceRate"`         // Sell limit price rate
	TakerFeeRate              float64  `json:"takerFeeRate"`              // Taker fee rate
	MakerFeeRate              float64  `json:"makerFeeRate"`              // Maker fee rate
	MaintenanceMarginRate     float64  `json:"maintenanceMarginRate"`     // Maintenance margin rate
	InitialMarginRate         float64  `json:"initialMarginRate"`         // Initial margin rate
	RiskBaseVol               float64  `json:"riskBaseVol"`               // Risk base volume
	RiskIncrVol               float64  `json:"riskIncrVol"`               // Risk increment volume
	RiskIncrMmr               float64  `json:"riskIncrMmr"`               // Risk increment MMR
	RiskIncrImr               float64  `json:"riskIncrImr"`               // Risk increment IMR
	RiskLevelLimit            int      `json:"riskLevelLimit"`            // Risk level limit
	PriceCoefficientVariation float64  `json:"priceCoefficientVariation"` // Price coefficient variation
	State                     int      `json:"state"`                     // Contract state (0=active)
	IsNew                     bool     `json:"isNew"`                     // Is new listing
	IsHot                     bool     `json:"isHot"`                     // Is popular
	IsHidden                  bool     `json:"isHidden"`                  // Is hidden
	ConceptPlate              []string `json:"conceptPlate"`              // Category tags
	APIAllowed                bool     `json:"apiAllowed"`                // API trading allowed
}

// =============================================================================
// Ticker Types
// =============================================================================

// Ticker represents real-time market ticker data
type Ticker struct {
	Symbol        string  `json:"symbol"`        // Contract symbol
	LastPrice     float64 `json:"lastPrice"`     // Last traded price
	Bid1          float64 `json:"bid1"`          // Best bid price
	Ask1          float64 `json:"ask1"`          // Best ask price
	Volume24      float64 `json:"volume24"`      // 24h volume (contracts)
	Amount24      float64 `json:"amount24"`      // 24h turnover
	HoldVol       float64 `json:"holdVol"`       // Open interest (contracts)
	Lower24Price  float64 `json:"lower24Price"`  // 24h low
	High24Price   float64 `json:"high24Price"`   // 24h high
	RiseFallRate  float64 `json:"riseFallRate"`  // 24h price change %
	RiseFallValue float64 `json:"riseFallValue"` // 24h price change value
	IndexPrice    float64 `json:"indexPrice"`    // Index price
	FairPrice     float64 `json:"fairPrice"`     // Mark/fair price
	FundingRate   float64 `json:"fundingRate"`   // Current funding rate
	MaxBidPrice   float64 `json:"maxBidPrice"`   // Maximum bid price allowed
	MinAskPrice   float64 `json:"minAskPrice"`   // Minimum ask price allowed
	Timestamp     int64   `json:"timestamp"`     // Data timestamp (ms)
}

// =============================================================================
// Order Book Types
// =============================================================================

// OrderBookLevel represents a single price level in orderbook
type OrderBookLevel struct {
	Price    float64 `json:"price"`    // Price
	Quantity float64 `json:"quantity"` // Quantity
	Count    int     `json:"count"`    // Number of orders at this level
}

// OrderBook represents order book depth data
type OrderBook struct {
	Asks      [][]float64 `json:"asks"`      // Ask levels [[price, qty, count], ...]
	Bids      [][]float64 `json:"bids"`      // Bid levels [[price, qty, count], ...]
	Version   int64       `json:"version"`   // Sequence number
	Timestamp int64       `json:"timestamp"` // Timestamp (ms)
}

// =============================================================================
// Trade Types
// =============================================================================

// Trade represents a single trade (deal)
type Trade struct {
	Price      float64 `json:"p"` // Trade price
	Volume     float64 `json:"v"` // Trade volume
	TradeType  int     `json:"T"` // Trade type: 1=buy, 2=sell
	OpenType   int     `json:"O"` // Open type
	MarketType int     `json:"M"` // Market type
	Timestamp  int64   `json:"t"` // Trade timestamp (ms)
}

// =============================================================================
// Kline Types
// =============================================================================

// KlineInterval represents kline interval
type KlineInterval string

const (
	KlineMin1   KlineInterval = "Min1"
	KlineMin5   KlineInterval = "Min5"
	KlineMin15  KlineInterval = "Min15"
	KlineMin30  KlineInterval = "Min30"
	KlineMin60  KlineInterval = "Min60"
	KlineHour4  KlineInterval = "Hour4"
	KlineHour8  KlineInterval = "Hour8"
	KlineDay1   KlineInterval = "Day1"
	KlineWeek1  KlineInterval = "Week1"
	KlineMonth1 KlineInterval = "Month1"
)

// Kline represents candlestick data
type Kline struct {
	Time   []int64   `json:"time"`   // Timestamps
	Open   []float64 `json:"open"`   // Open prices
	High   []float64 `json:"high"`   // High prices
	Low    []float64 `json:"low"`    // Low prices
	Close  []float64 `json:"close"`  // Close prices
	Vol    []float64 `json:"vol"`    // Volumes
	Amount []float64 `json:"amount"` // Turnover amounts
}

// KlineBar represents a single kline bar
type KlineBar struct {
	Time   int64   `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Vol    float64 `json:"vol"`
	Amount float64 `json:"amount"`
}

// =============================================================================
// Funding Rate Types
// =============================================================================

// FundingRate represents funding rate data
type FundingRate struct {
	Symbol         string  `json:"symbol"`         // Contract symbol
	FundingRate    float64 `json:"fundingRate"`    // Current funding rate
	MaxFundingRate float64 `json:"maxFundingRate"` // Maximum funding rate
	MinFundingRate float64 `json:"minFundingRate"` // Minimum funding rate
	CollectCycle   int     `json:"collectCycle"`   // Funding interval (hours)
	NextSettleTime int64   `json:"nextSettleTime"` // Next settlement timestamp (ms)
	Timestamp      int64   `json:"timestamp"`      // Current timestamp (ms)
}

// IndexPrice represents index price data
type IndexPrice struct {
	Symbol     string  `json:"symbol"`
	IndexPrice float64 `json:"indexPrice"`
	Timestamp  int64   `json:"timestamp"`
}

// FairPrice represents fair/mark price data
type FairPrice struct {
	Symbol    string  `json:"symbol"`
	FairPrice float64 `json:"fairPrice"`
	Timestamp int64   `json:"timestamp"`
}

// =============================================================================
// Account Types
// =============================================================================

// AccountAsset represents account balance for a currency
type AccountAsset struct {
	Currency         string  `json:"currency"`         // Currency symbol
	PositionMargin   float64 `json:"positionMargin"`   // Position margin
	FrozenBalance    float64 `json:"frozenBalance"`    // Frozen balance
	AvailableBalance float64 `json:"availableBalance"` // Available balance
	CashBalance      float64 `json:"cashBalance"`      // Cash balance
	Equity           float64 `json:"equity"`           // Total equity
	Unrealized       float64 `json:"unrealized"`       // Unrealized PnL
}

// =============================================================================
// Position Types
// =============================================================================

// Position represents an open position
type Position struct {
	PositionID     int64   `json:"positionId"`     // Position ID
	Symbol         string  `json:"symbol"`         // Contract symbol
	HoldVol        float64 `json:"holdVol"`        // Position size
	PositionType   int     `json:"positionType"`   // 1=long, 2=short
	OpenType       int     `json:"openType"`       // 1=isolated, 2=cross
	State          int     `json:"state"`          // Position state
	FrozenVol      float64 `json:"frozenVol"`      // Frozen volume
	CloseVol       float64 `json:"closeVol"`       // Closed volume
	HoldAvgPrice   float64 `json:"holdAvgPrice"`   // Average entry price
	CloseAvgPrice  float64 `json:"closeAvgPrice"`  // Average exit price
	OpenAvgPrice   float64 `json:"openAvgPrice"`   // Average open price
	LiquidatePrice float64 `json:"liquidatePrice"` // Liquidation price
	OIM            float64 `json:"oim"`            // Original initial margin
	ADLLevel       int     `json:"adlLevel"`       // ADL level (1-5)
	IM             float64 `json:"im"`             // Initial margin
	HoldFee        float64 `json:"holdFee"`        // Holding fee
	Realised       float64 `json:"realised"`       // Realized PnL
	Unrealised     float64 `json:"unrealised"`     // Unrealized PnL
	Leverage       int     `json:"leverage"`       // Current leverage
	CreateTime     int64   `json:"createTime"`     // Position open time
	UpdateTime     int64   `json:"updateTime"`     // Last update time
}

// =============================================================================
// Order Types
// =============================================================================

// Order represents an order
type Order struct {
	OrderID         int64   `json:"orderId"`         // Server order ID
	Symbol          string  `json:"symbol"`          // Contract symbol
	PositionID      int64   `json:"positionId"`      // Position ID
	Price           float64 `json:"price"`           // Order price
	Vol             float64 `json:"vol"`             // Order volume
	DealVol         float64 `json:"dealVol"`         // Filled volume
	DealAvgPrice    float64 `json:"dealAvgPrice"`    // Average fill price
	OpenType        int     `json:"openType"`        // 1=isolated, 2=cross
	Side            int     `json:"side"`            // Order side
	Type            int     `json:"type"`            // Order type
	Category        int     `json:"category"`        // Order category
	State           int     `json:"state"`           // Order state
	ExternalOID     string  `json:"externalOid"`     // Client order ID
	Leverage        int     `json:"leverage"`        // Leverage
	StopLossPrice   float64 `json:"stopLossPrice"`   // Stop loss price
	TakeProfitPrice float64 `json:"takeProfitPrice"` // Take profit price
	Fee             float64 `json:"fee"`             // Trading fee
	FeeCurrency     string  `json:"feeCurrency"`     // Fee currency
	CreateTime      int64   `json:"createTime"`      // Order create time
	UpdateTime      int64   `json:"updateTime"`      // Last update time
}

// OrderRequest represents an order submission request
type OrderRequest struct {
	Symbol          string  `json:"symbol"`                    // Contract symbol
	Price           float64 `json:"price,omitempty"`           // Order price
	Vol             float64 `json:"vol"`                       // Order volume
	Leverage        int     `json:"leverage,omitempty"`        // Leverage
	Side            int     `json:"side"`                      // Order side
	Type            int     `json:"type"`                      // Order type
	OpenType        int     `json:"openType"`                  // Margin type
	PositionID      int64   `json:"positionId,omitempty"`      // Position ID
	ExternalOID     string  `json:"externalOid,omitempty"`     // Client order ID
	StopLossPrice   float64 `json:"stopLossPrice,omitempty"`   // Stop loss
	TakeProfitPrice float64 `json:"takeProfitPrice,omitempty"` // Take profit
	PositionMode    int     `json:"positionMode,omitempty"`    // Position mode
	ReduceOnly      bool    `json:"reduceOnly,omitempty"`      // Reduce only flag
}

// OrderResponse represents order submission response
type OrderResponse struct {
	OrderID     int64  `json:"orderId"`     // Server order ID
	ExternalOID string `json:"externalOid"` // Client order ID
}

// CancelOrderRequest represents cancel order request
type CancelOrderRequest struct {
	Symbol      string `json:"symbol,omitempty"`      // Contract symbol
	OrderID     int64  `json:"orderId,omitempty"`     // Order ID
	ExternalOID string `json:"externalOid,omitempty"` // Client order ID
}

// =============================================================================
// Leverage Types
// =============================================================================

// LeverageRequest represents leverage change request
type LeverageRequest struct {
	Symbol       string `json:"symbol"`       // Contract symbol
	Leverage     int    `json:"leverage"`     // New leverage
	OpenType     int    `json:"openType"`     // 1=isolated, 2=cross
	PositionType int    `json:"positionType"` // 1=long, 2=short
}

// =============================================================================
// WebSocket Types
// =============================================================================

// WSMessage represents a generic WebSocket message
type WSMessage struct {
	Method  string          `json:"method,omitempty"`
	Channel string          `json:"channel,omitempty"`
	Param   json.RawMessage `json:"param,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Symbol  string          `json:"symbol,omitempty"`
	Ts      int64           `json:"ts,omitempty"`
}

// WSSubscribeRequest represents subscription request
type WSSubscribeRequest struct {
	Method string                 `json:"method"`
	Param  map[string]interface{} `json:"param"`
}

// WSLoginRequest represents WebSocket login request
type WSLoginRequest struct {
	Method string       `json:"method"`
	Param  WSLoginParam `json:"param"`
}

// WSLoginParam represents login parameters
type WSLoginParam struct {
	APIKey    string `json:"apiKey"`
	ReqTime   int64  `json:"reqTime"`
	Signature string `json:"signature"`
}

// =============================================================================
// WebSocket Push Data Types
// =============================================================================

// WSTickerData represents WebSocket ticker push data
type WSTickerData struct {
	Symbol       string  `json:"symbol"`
	LastPrice    float64 `json:"lastPrice"`
	Bid1         float64 `json:"bid1"`
	Ask1         float64 `json:"ask1"`
	Volume24     float64 `json:"volume24"`
	HoldVol      float64 `json:"holdVol"`
	RiseFallRate float64 `json:"riseFallRate"`
	FairPrice    float64 `json:"fairPrice"`
	FundingRate  float64 `json:"fundingRate"`
	IndexPrice   float64 `json:"indexPrice"`
	Timestamp    int64   `json:"timestamp"`
}

// WSDepthData represents WebSocket orderbook push data
type WSDepthData struct {
	Asks    [][]float64 `json:"asks"`    // [[price, qty, count], ...]
	Bids    [][]float64 `json:"bids"`    // [[price, qty, count], ...]
	Version int64       `json:"version"` // Sequence number
}

// WSTradeData represents WebSocket trade push data
type WSTradeData struct {
	Price     float64 `json:"p"`
	Volume    float64 `json:"v"`
	TradeType int     `json:"T"` // 1=buy, 2=sell
	Timestamp int64   `json:"t"`
}

// WSKlineData represents WebSocket kline push data
type WSKlineData struct {
	Symbol string  `json:"symbol"`
	Open   float64 `json:"o"`
	High   float64 `json:"h"`
	Low    float64 `json:"l"`
	Close  float64 `json:"c"`
	Volume float64 `json:"q"`
	Amount float64 `json:"a"`
	Time   int64   `json:"t"`
}

// =============================================================================
// WebSocket Private Push Data
// =============================================================================

// WSAccountUpdate represents WebSocket account update
type WSAccountUpdate struct {
	Currency         string  `json:"currency"`
	AvailableBalance float64 `json:"availableBalance"`
	FrozenBalance    float64 `json:"frozenBalance"`
	PositionMargin   float64 `json:"positionMargin"`
	Equity           float64 `json:"equity"`
	Unrealized       float64 `json:"unrealized"`
}

// WSPositionUpdate represents WebSocket position update
type WSPositionUpdate struct {
	PositionID     int64   `json:"positionId"`
	Symbol         string  `json:"symbol"`
	HoldVol        float64 `json:"holdVol"`
	PositionType   int     `json:"positionType"`
	OpenType       int     `json:"openType"`
	State          int     `json:"state"`
	HoldAvgPrice   float64 `json:"holdAvgPrice"`
	LiquidatePrice float64 `json:"liquidatePrice"`
	IM             float64 `json:"im"`
	Realised       float64 `json:"realised"`
	Unrealised     float64 `json:"unrealised"`
	ADLLevel       int     `json:"adlLevel"`
	Leverage       int     `json:"leverage"`
	UpdateTime     int64   `json:"updateTime"`
}

// WSOrderUpdate represents WebSocket order update
type WSOrderUpdate struct {
	OrderID      int64   `json:"orderId"`
	Symbol       string  `json:"symbol"`
	PositionID   int64   `json:"positionId"`
	Price        float64 `json:"price"`
	Vol          float64 `json:"vol"`
	DealVol      float64 `json:"dealVol"`
	DealAvgPrice float64 `json:"dealAvgPrice"`
	Side         int     `json:"side"`
	Type         int     `json:"type"`
	State        int     `json:"state"`
	ExternalOID  string  `json:"externalOid"`
	CreateTime   int64   `json:"createTime"`
	UpdateTime   int64   `json:"updateTime"`
}

// =============================================================================
// Error Types
// =============================================================================

// APIError represents an API error
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("MEXC API error %d: %s", e.Code, e.Message)
}

// Common error codes
const (
	ErrCodeSuccess              = 0
	ErrCodeUnauthorized         = 401
	ErrCodeNotFound             = 404
	ErrCodeInvalidParam         = 10001
	ErrCodeInsufficient         = 10002
	ErrCodeOrderNotFound        = 10003
	ErrCodePositionNotFound     = 10004
	ErrCodeSymbolNotFound       = 10005
	ErrCodePriceOutOfRange      = 10006
	ErrCodeVolumeTooSmall       = 10007
	ErrCodeVolumeTooLarge       = 10008
	ErrCodeLeverageInvalid      = 10009
	ErrCodePositionModeConflict = 10010
)
