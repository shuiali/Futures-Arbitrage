// Package kucoin provides types and clients for KuCoin Futures exchange API integration.
// Supports REST and WebSocket APIs for futures market data, trading, and account management.
// API Documentation: https://www.kucoin.com/docs-new/introduction
package kucoin

import (
	"encoding/json"
	"fmt"
	"time"
)

// =============================================================================
// Common Constants
// =============================================================================

// Base URLs
const (
	FuturesRESTBaseURL = "https://api-futures.kucoin.com"
	SpotRESTBaseURL    = "https://api.kucoin.com"
)

// API Version
const (
	APIKeyVersion = "2"
)

// Settlement currencies
const (
	SettleUSDT = "USDT"
	SettleUSDS = "USDS"
)

// Margin modes
const (
	MarginModeIsolated = "ISOLATED"
	MarginModeCross    = "CROSS"
)

// Position sides (for hedge mode)
const (
	PositionSideBoth  = "BOTH"  // One-way mode
	PositionSideLong  = "LONG"  // Long position
	PositionSideShort = "SHORT" // Short position
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

// Time in force
const (
	TIFGoodTillCancel    = "GTC" // Good till cancelled
	TIFImmediateOrCancel = "IOC" // Immediate or cancel
	TIFFillOrKill        = "FOK" // Fill or kill
)

// Order status
const (
	OrderStatusActive = "active"
	OrderStatusDone   = "done"
)

// Stop price types
const (
	StopPriceTypeTrade = "TP" // Trade price
	StopPriceTypeIndex = "IP" // Index price
	StopPriceTypeMark  = "MP" // Mark price
)

// Stop directions
const (
	StopDirectionDown = "down"
	StopDirectionUp   = "up"
)

// Kline granularities (in minutes)
const (
	Kline1m  = 1
	Kline5m  = 5
	Kline15m = 15
	Kline30m = 30
	Kline1h  = 60
	Kline2h  = 120
	Kline4h  = 240
	Kline8h  = 480
	Kline12h = 720
	Kline1d  = 1440
	Kline1w  = 10080
)

// WebSocket message types
const (
	WSTypeWelcome     = "welcome"
	WSTypePing        = "ping"
	WSTypePong        = "pong"
	WSTypeSubscribe   = "subscribe"
	WSTypeUnsubscribe = "unsubscribe"
	WSTypeAck         = "ack"
	WSTypeMessage     = "message"
	WSTypeError       = "error"
)

// WebSocket topics
const (
	// Public topics
	WSTopicTicker        = "/contractMarket/ticker"
	WSTopicLevel2Depth5  = "/contractMarket/level2Depth5"
	WSTopicLevel2Depth50 = "/contractMarket/level2Depth50"
	WSTopicLevel2        = "/contractMarket/level2"
	WSTopicExecution     = "/contractMarket/execution"
	WSTopicInstrument    = "/contract/instrument"

	// Private topics
	WSTopicTradeOrders = "/contractMarket/tradeOrders"
	WSTopicPosition    = "/contract/position"
	WSTopicWallet      = "/contractAccount/wallet"
)

// =============================================================================
// REST API Response Wrapper
// =============================================================================

// Response represents a standard KuCoin API response
type Response struct {
	Code string          `json:"code"`
	Msg  string          `json:"msg,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

// IsSuccess checks if the response is successful
func (r *Response) IsSuccess() bool {
	return r.Code == "200000"
}

// Error returns error if response is not successful
func (r *Response) Error() error {
	if r.IsSuccess() {
		return nil
	}
	return fmt.Errorf("KuCoin API error: code=%s, msg=%s", r.Code, r.Msg)
}

// =============================================================================
// Contract Types
// =============================================================================

// Contract represents a futures contract
type Contract struct {
	Symbol               string  `json:"symbol"`                  // Contract symbol, e.g., "XBTUSDTM"
	RootSymbol           string  `json:"rootSymbol"`              // Root symbol
	Type                 string  `json:"type"`                    // Contract type
	BaseCurrency         string  `json:"baseCurrency"`            // Base currency
	QuoteCurrency        string  `json:"quoteCurrency"`           // Quote currency
	SettleCurrency       string  `json:"settleCurrency"`          // Settlement currency
	MaxOrderQty          int64   `json:"maxOrderQty"`             // Maximum order quantity
	MaxPrice             float64 `json:"maxPrice"`                // Maximum price
	LotSize              int64   `json:"lotSize"`                 // Lot size
	TickSize             float64 `json:"tickSize"`                // Tick size (price precision)
	IndexPriceTickSize   float64 `json:"indexPriceTickSize"`      // Index price tick size
	Multiplier           float64 `json:"multiplier"`              // Contract multiplier
	InitialMargin        float64 `json:"initialMargin"`           // Initial margin rate
	MaintainMargin       float64 `json:"maintainMargin"`          // Maintenance margin rate
	MaxRiskLimit         int64   `json:"maxRiskLimit"`            // Maximum risk limit
	MinRiskLimit         int64   `json:"minRiskLimit"`            // Minimum risk limit
	RiskStep             int64   `json:"riskStep"`                // Risk limit step
	MakerFeeRate         float64 `json:"makerFeeRate"`            // Maker fee rate
	TakerFeeRate         float64 `json:"takerFeeRate"`            // Taker fee rate
	TakerFixFee          float64 `json:"takerFixFee"`             // Taker fixed fee
	MakerFixFee          float64 `json:"makerFixFee"`             // Maker fixed fee
	IsDeleverage         bool    `json:"isDeleverage"`            // Is deleverage enabled
	IsQuanto             bool    `json:"isQuanto"`                // Is quanto contract
	IsInverse            bool    `json:"isInverse"`               // Is inverse contract
	MarkMethod           string  `json:"markMethod"`              // Mark price method
	FairMethod           string  `json:"fairMethod"`              // Fair price method
	FundingBaseSymbol    string  `json:"fundingBaseSymbol"`       // Funding base symbol
	FundingQuoteSymbol   string  `json:"fundingQuoteSymbol"`      // Funding quote symbol
	FundingRateSymbol    string  `json:"fundingRateSymbol"`       // Funding rate symbol
	IndexSymbol          string  `json:"indexSymbol"`             // Index symbol
	SettlementSymbol     string  `json:"settlementSymbol"`        // Settlement symbol
	Status               string  `json:"status"`                  // Contract status
	FundingFeeRate       float64 `json:"fundingFeeRate"`          // Current funding rate
	PredictedFundingRate float64 `json:"predictedFundingFeeRate"` // Predicted funding rate
	OpenInterest         string  `json:"openInterest"`            // Open interest
	TurnoverOf24h        float64 `json:"turnoverOf24h"`           // 24h turnover
	VolumeOf24h          float64 `json:"volumeOf24h"`             // 24h volume
	MarkPrice            float64 `json:"markPrice"`               // Current mark price
	IndexPrice           float64 `json:"indexPrice"`              // Current index price
	LastTradePrice       float64 `json:"lastTradePrice"`          // Last traded price
	NextFundingRateTime  int64   `json:"nextFundingRateTime"`     // Next funding time
	MaxLeverage          int     `json:"maxLeverage"`             // Maximum leverage
	LowPrice             float64 `json:"lowPrice"`                // 24h low price
	HighPrice            float64 `json:"highPrice"`               // 24h high price
}

// =============================================================================
// Ticker Types
// =============================================================================

// Ticker represents market ticker data
type Ticker struct {
	Sequence     int64  `json:"sequence"`     // Sequence number
	Symbol       string `json:"symbol"`       // Symbol
	Side         string `json:"side"`         // Last trade side
	Size         int    `json:"size"`         // Last trade size
	Price        string `json:"price"`        // Last trade price
	BestBidSize  int    `json:"bestBidSize"`  // Best bid size
	BestBidPrice string `json:"bestBidPrice"` // Best bid price
	BestAskSize  int    `json:"bestAskSize"`  // Best ask size
	BestAskPrice string `json:"bestAskPrice"` // Best ask price
	TradeID      string `json:"tradeId"`      // Last trade ID
	Ts           int64  `json:"ts"`           // Timestamp (milliseconds)
}

// AllTickersItem represents a single ticker in all tickers response
type AllTickersItem struct {
	Sequence     int64  `json:"sequence"`     // Sequence number
	Symbol       string `json:"symbol"`       // Symbol
	Side         string `json:"side"`         // Last trade side
	Size         int    `json:"size"`         // Last trade size
	TradeID      string `json:"tradeId"`      // Last trade ID
	Price        string `json:"price"`        // Last trade price
	BestBidPrice string `json:"bestBidPrice"` // Best bid price
	BestBidSize  int    `json:"bestBidSize"`  // Best bid size
	BestAskPrice string `json:"bestAskPrice"` // Best ask price
	BestAskSize  int    `json:"bestAskSize"`  // Best ask size
	Ts           int64  `json:"ts"`           // Timestamp (milliseconds)
}

// =============================================================================
// OrderBook Types
// =============================================================================

// OrderBook represents order book depth
type OrderBook struct {
	Symbol   string      `json:"symbol"`   // Symbol
	Sequence int64       `json:"sequence"` // Sequence number
	Asks     [][]float64 `json:"asks"`     // Ask levels [[price, size], ...]
	Bids     [][]float64 `json:"bids"`     // Bid levels [[price, size], ...]
	Ts       int64       `json:"ts"`       // Timestamp (milliseconds)
}

// OrderBookLevel represents a single order book level
type OrderBookLevel struct {
	Price float64
	Size  float64
}

// =============================================================================
// Trade Types
// =============================================================================

// Trade represents a single trade
type Trade struct {
	Sequence     int64  `json:"sequence"`     // Sequence number
	TradeID      string `json:"tradeId"`      // Trade ID
	TakerOrderID string `json:"takerOrderId"` // Taker order ID
	MakerOrderID string `json:"makerOrderId"` // Maker order ID
	Price        string `json:"price"`        // Trade price
	Size         int    `json:"size"`         // Trade size
	Side         string `json:"side"`         // Trade side: "buy" or "sell"
	Ts           int64  `json:"ts"`           // Timestamp (milliseconds)
}

// =============================================================================
// Kline Types
// =============================================================================

// Kline represents a candlestick/kline
type Kline struct {
	Timestamp int64  // Timestamp (milliseconds)
	Open      string // Open price
	High      string // High price
	Low       string // Low price
	Close     string // Close price
	Volume    string // Volume
}

// ParseKline parses raw kline data from API
func ParseKline(data []interface{}) (*Kline, error) {
	if len(data) < 6 {
		return nil, fmt.Errorf("invalid kline data: expected 6 elements, got %d", len(data))
	}

	k := &Kline{}

	if ts, ok := data[0].(float64); ok {
		k.Timestamp = int64(ts)
	}
	if o, ok := data[1].(string); ok {
		k.Open = o
	}
	if h, ok := data[2].(string); ok {
		k.High = h
	}
	if l, ok := data[3].(string); ok {
		k.Low = l
	}
	if c, ok := data[4].(string); ok {
		k.Close = c
	}
	if v, ok := data[5].(string); ok {
		k.Volume = v
	}

	return k, nil
}

// =============================================================================
// Funding Rate Types
// =============================================================================

// FundingRate represents current funding rate
type FundingRate struct {
	Symbol           string  `json:"symbol"`           // Funding rate symbol
	Granularity      int64   `json:"granularity"`      // Granularity (milliseconds)
	TimePoint        int64   `json:"timePoint"`        // Current time point
	Value            float64 `json:"value"`            // Current funding rate
	PredictedValue   float64 `json:"predictedValue"`   // Predicted funding rate
	FundingRateCap   float64 `json:"fundingRateCap"`   // Funding rate cap
	FundingRateFloor float64 `json:"fundingRateFloor"` // Funding rate floor
	Period           int     `json:"period"`           // Period
	FundingTime      int64   `json:"fundingTime"`      // Next funding time
}

// FundingRateHistory represents historical funding rate
type FundingRateHistory struct {
	Symbol      string  `json:"symbol"`      // Symbol
	FundingRate float64 `json:"fundingRate"` // Funding rate
	TimePoint   int64   `json:"timepoint"`   // Timestamp
}

// =============================================================================
// Mark/Index Price Types
// =============================================================================

// MarkPrice represents current mark price
type MarkPrice struct {
	Symbol      string  `json:"symbol"`      // Symbol
	Granularity int64   `json:"granularity"` // Granularity
	TimePoint   int64   `json:"timePoint"`   // Timestamp
	Value       float64 `json:"value"`       // Mark price
	IndexPrice  float64 `json:"indexPrice"`  // Index price
}

// =============================================================================
// Order Types
// =============================================================================

// OrderRequest represents order placement request
type OrderRequest struct {
	ClientOid     string `json:"clientOid"`               // Client order ID (required)
	Symbol        string `json:"symbol"`                  // Symbol (required)
	Side          string `json:"side"`                    // "buy" or "sell" (required)
	Type          string `json:"type"`                    // "limit" or "market" (required)
	Size          int    `json:"size"`                    // Order size in lots (required)
	Price         string `json:"price,omitempty"`         // Price (required for limit)
	Leverage      int    `json:"leverage,omitempty"`      // Leverage
	MarginMode    string `json:"marginMode,omitempty"`    // "ISOLATED" or "CROSS"
	PositionSide  string `json:"positionSide,omitempty"`  // "BOTH", "LONG", "SHORT"
	TimeInForce   string `json:"timeInForce,omitempty"`   // "GTC", "IOC", "FOK"
	ReduceOnly    bool   `json:"reduceOnly,omitempty"`    // Reduce only flag
	PostOnly      bool   `json:"postOnly,omitempty"`      // Post only flag
	Hidden        bool   `json:"hidden,omitempty"`        // Hidden order
	Iceberg       bool   `json:"iceberg,omitempty"`       // Iceberg order
	VisibleSize   int    `json:"visibleSize,omitempty"`   // Visible size for iceberg
	Stop          string `json:"stop,omitempty"`          // "down" or "up"
	StopPrice     string `json:"stopPrice,omitempty"`     // Stop trigger price
	StopPriceType string `json:"stopPriceType,omitempty"` // "TP", "IP", "MP"
	Remark        string `json:"remark,omitempty"`        // Order remark
}

// OrderResponse represents order placement response
type OrderResponse struct {
	OrderID   string `json:"orderId"`   // Order ID
	ClientOid string `json:"clientOid"` // Client order ID
}

// BatchOrderResponse represents batch order response
type BatchOrderResponse struct {
	OrderID   string `json:"orderId"`   // Order ID
	ClientOid string `json:"clientOid"` // Client order ID
	Symbol    string `json:"symbol"`    // Symbol
	Code      string `json:"code"`      // Response code
	Msg       string `json:"msg"`       // Response message
}

// CancelResponse represents order cancellation response
type CancelResponse struct {
	CancelledOrderIds []string `json:"cancelledOrderIds"` // Cancelled order IDs
}

// Order represents a futures order
type Order struct {
	ID             string `json:"id"`             // Order ID
	Symbol         string `json:"symbol"`         // Symbol
	Type           string `json:"type"`           // Order type
	Side           string `json:"side"`           // Order side
	Price          string `json:"price"`          // Price
	Size           int    `json:"size"`           // Order size
	Value          string `json:"value"`          // Order value
	DealValue      string `json:"dealValue"`      // Dealt value
	DealSize       int    `json:"dealSize"`       // Dealt size
	Stp            string `json:"stp"`            // Self-trade prevention
	Stop           string `json:"stop"`           // Stop type
	StopPriceType  string `json:"stopPriceType"`  // Stop price type
	StopTriggered  bool   `json:"stopTriggered"`  // Is stop triggered
	StopPrice      string `json:"stopPrice"`      // Stop price
	TimeInForce    string `json:"timeInForce"`    // Time in force
	PostOnly       bool   `json:"postOnly"`       // Post only
	Hidden         bool   `json:"hidden"`         // Hidden order
	Iceberg        bool   `json:"iceberg"`        // Iceberg order
	Leverage       string `json:"leverage"`       // Leverage
	ForceHold      bool   `json:"forceHold"`      // Force hold
	CloseOrder     bool   `json:"closeOrder"`     // Close position order
	VisibleSize    int    `json:"visibleSize"`    // Visible size
	ClientOid      string `json:"clientOid"`      // Client order ID
	Remark         string `json:"remark"`         // Remark
	Tags           string `json:"tags"`           // Tags
	IsActive       bool   `json:"isActive"`       // Is active
	CancelExist    bool   `json:"cancelExist"`    // Has cancel request
	CreatedAt      int64  `json:"createdAt"`      // Created timestamp
	UpdatedAt      int64  `json:"updatedAt"`      // Updated timestamp
	EndAt          int64  `json:"endAt"`          // End timestamp
	OrderTime      int64  `json:"orderTime"`      // Order timestamp (ns)
	SettleCurrency string `json:"settleCurrency"` // Settlement currency
	MarginMode     string `json:"marginMode"`     // Margin mode
	PositionSide   string `json:"positionSide"`   // Position side
	AvgDealPrice   string `json:"avgDealPrice"`   // Average deal price
	FilledSize     int    `json:"filledSize"`     // Filled size
	FilledValue    string `json:"filledValue"`    // Filled value
	Status         string `json:"status"`         // Order status
	ReduceOnly     bool   `json:"reduceOnly"`     // Reduce only
}

// OrderList represents paginated order list
type OrderList struct {
	CurrentPage int      `json:"currentPage"` // Current page
	PageSize    int      `json:"pageSize"`    // Page size
	TotalNum    int      `json:"totalNum"`    // Total count
	TotalPage   int      `json:"totalPage"`   // Total pages
	Items       []*Order `json:"items"`       // Orders
}

// =============================================================================
// Fill/Trade History Types
// =============================================================================

// Fill represents a trade fill
type Fill struct {
	Symbol         string `json:"symbol"`         // Symbol
	TradeID        string `json:"tradeId"`        // Trade ID
	OrderID        string `json:"orderId"`        // Order ID
	Side           string `json:"side"`           // Side
	Liquidity      string `json:"liquidity"`      // "maker" or "taker"
	ForceTaker     bool   `json:"forceTaker"`     // Force taker
	Price          string `json:"price"`          // Fill price
	Size           int    `json:"size"`           // Fill size
	Value          string `json:"value"`          // Fill value
	FeeRate        string `json:"feeRate"`        // Fee rate
	FixFee         string `json:"fixFee"`         // Fixed fee
	FeeCurrency    string `json:"feeCurrency"`    // Fee currency
	Stop           string `json:"stop"`           // Stop type
	Fee            string `json:"fee"`            // Fee
	OrderType      string `json:"orderType"`      // Order type
	TradeType      string `json:"tradeType"`      // Trade type
	CreatedAt      int64  `json:"createdAt"`      // Created timestamp
	SettleCurrency string `json:"settleCurrency"` // Settlement currency
	TradeTime      int64  `json:"tradeTime"`      // Trade timestamp (ns)
	MarginMode     string `json:"marginMode"`     // Margin mode
	PositionSide   string `json:"positionSide"`   // Position side
}

// FillList represents paginated fill list
type FillList struct {
	CurrentPage int     `json:"currentPage"` // Current page
	PageSize    int     `json:"pageSize"`    // Page size
	TotalNum    int     `json:"totalNum"`    // Total count
	TotalPage   int     `json:"totalPage"`   // Total pages
	Items       []*Fill `json:"items"`       // Fills
}

// =============================================================================
// Position Types
// =============================================================================

// Position represents a futures position
type Position struct {
	ID                string  `json:"id"`                // Position ID
	Symbol            string  `json:"symbol"`            // Symbol
	AutoDeposit       bool    `json:"autoDeposit"`       // Auto deposit margin
	CrossMode         bool    `json:"crossMode"`         // Is cross mode
	MaintMarginReq    float64 `json:"maintMarginReq"`    // Maintenance margin requirement
	RiskLimit         int64   `json:"riskLimit"`         // Risk limit
	RealLeverage      float64 `json:"realLeverage"`      // Real leverage
	DelevPercentage   float64 `json:"delevPercentage"`   // Deleverage percentage
	OpeningTimestamp  int64   `json:"openingTimestamp"`  // Opening timestamp
	CurrentTimestamp  int64   `json:"currentTimestamp"`  // Current timestamp
	CurrentQty        int     `json:"currentQty"`        // Current quantity
	CurrentCost       float64 `json:"currentCost"`       // Current cost
	CurrentComm       float64 `json:"currentComm"`       // Current commission
	UnrealisedCost    float64 `json:"unrealisedCost"`    // Unrealized cost
	RealisedGrossCost float64 `json:"realisedGrossCost"` // Realized gross cost
	RealisedCost      float64 `json:"realisedCost"`      // Realized cost
	IsOpen            bool    `json:"isOpen"`            // Is open
	MarkPrice         float64 `json:"markPrice"`         // Mark price
	MarkValue         float64 `json:"markValue"`         // Mark value
	PosCost           float64 `json:"posCost"`           // Position cost
	PosCross          float64 `json:"posCross"`          // Position cross margin
	PosCrossMargin    float64 `json:"posCrossMargin"`    // Position cross margin
	PosInit           float64 `json:"posInit"`           // Position initial margin
	PosComm           float64 `json:"posComm"`           // Position commission
	PosCommCommon     float64 `json:"posCommCommon"`     // Position common commission
	PosLoss           float64 `json:"posLoss"`           // Position loss
	PosMargin         float64 `json:"posMargin"`         // Position margin
	PosFunding        float64 `json:"posFunding"`        // Position funding
	PosMaint          float64 `json:"posMaint"`          // Position maintenance margin
	MaintMargin       float64 `json:"maintMargin"`       // Maintenance margin
	RealisedGrossPnl  float64 `json:"realisedGrossPnl"`  // Realized gross PnL
	RealisedPnl       float64 `json:"realisedPnl"`       // Realized PnL
	UnrealisedPnl     float64 `json:"unrealisedPnl"`     // Unrealized PnL
	UnrealisedPnlPcnt float64 `json:"unrealisedPnlPcnt"` // Unrealized PnL percentage
	UnrealisedRoePcnt float64 `json:"unrealisedRoePcnt"` // Unrealized ROE percentage
	AvgEntryPrice     float64 `json:"avgEntryPrice"`     // Average entry price
	LiquidationPrice  float64 `json:"liquidationPrice"`  // Liquidation price
	BankruptPrice     float64 `json:"bankruptPrice"`     // Bankruptcy price
	SettleCurrency    string  `json:"settleCurrency"`    // Settlement currency
	IsInverse         bool    `json:"isInverse"`         // Is inverse
	MaintainMargin    float64 `json:"maintainMargin"`    // Maintenance margin rate
	MarginMode        string  `json:"marginMode"`        // Margin mode
	PositionSide      string  `json:"positionSide"`      // Position side
	Leverage          float64 `json:"leverage"`          // Leverage
	DealComm          float64 `json:"dealComm"`          // Deal commission
	FundingFee        float64 `json:"fundingFee"`        // Funding fee
	Tax               float64 `json:"tax"`               // Tax
	WithdrawPnl       float64 `json:"withdrawPnl"`       // Withdraw PnL
}

// =============================================================================
// Account Types
// =============================================================================

// Account represents futures account overview
type Account struct {
	AccountEquity    float64 `json:"accountEquity"`    // Account equity
	UnrealisedPNL    float64 `json:"unrealisedPNL"`    // Unrealized PnL
	MarginBalance    float64 `json:"marginBalance"`    // Margin balance
	PositionMargin   float64 `json:"positionMargin"`   // Position margin
	OrderMargin      float64 `json:"orderMargin"`      // Order margin
	FrozenFunds      float64 `json:"frozenFunds"`      // Frozen funds
	AvailableBalance float64 `json:"availableBalance"` // Available balance
	Currency         string  `json:"currency"`         // Currency
}

// TradeFee represents trading fee rates
type TradeFee struct {
	Symbol       string `json:"symbol"`       // Symbol
	TakerFeeRate string `json:"takerFeeRate"` // Taker fee rate
	MakerFeeRate string `json:"makerFeeRate"` // Maker fee rate
}

// =============================================================================
// Leverage Types
// =============================================================================

// Leverage represents leverage setting
type Leverage struct {
	Symbol   string `json:"symbol"`   // Symbol
	Leverage string `json:"leverage"` // Leverage
}

// MarginMode represents margin mode
type MarginMode struct {
	Symbol     string `json:"symbol"`     // Symbol
	MarginMode string `json:"marginMode"` // Margin mode
}

// =============================================================================
// Risk Limit Types
// =============================================================================

// RiskLimit represents risk limit level
type RiskLimit struct {
	Symbol         string  `json:"symbol"`         // Symbol
	Level          int     `json:"level"`          // Risk limit level
	MaxRiskLimit   int64   `json:"maxRiskLimit"`   // Maximum risk limit
	MinRiskLimit   int64   `json:"minRiskLimit"`   // Minimum risk limit
	MaxLeverage    int     `json:"maxLeverage"`    // Maximum leverage
	InitialMargin  float64 `json:"initialMargin"`  // Initial margin rate
	MaintainMargin float64 `json:"maintainMargin"` // Maintenance margin rate
}

// =============================================================================
// Service Status Types
// =============================================================================

// ServiceStatus represents service status
type ServiceStatus struct {
	Status string `json:"status"` // Service status
	Msg    string `json:"msg"`    // Status message
}

// =============================================================================
// WebSocket Types
// =============================================================================

// WSToken represents WebSocket connection token
type WSToken struct {
	Token           string      `json:"token"`           // Connection token
	InstanceServers []*WSServer `json:"instanceServers"` // Server list
}

// WSServer represents WebSocket server info
type WSServer struct {
	Endpoint     string `json:"endpoint"`     // Server endpoint
	Encrypt      bool   `json:"encrypt"`      // Is encrypted
	Protocol     string `json:"protocol"`     // Protocol
	PingInterval int64  `json:"pingInterval"` // Ping interval (ms)
	PingTimeout  int64  `json:"pingTimeout"`  // Ping timeout (ms)
}

// WSMessage represents a generic WebSocket message
type WSMessage struct {
	ID             string          `json:"id,omitempty"`             // Message ID
	Type           string          `json:"type"`                     // Message type
	Topic          string          `json:"topic,omitempty"`          // Topic
	Subject        string          `json:"subject,omitempty"`        // Subject
	Data           json.RawMessage `json:"data,omitempty"`           // Message data
	PrivateChannel bool            `json:"privateChannel,omitempty"` // Is private channel
	Response       bool            `json:"response,omitempty"`       // Require response
	Sn             int64           `json:"sn,omitempty"`             // Sequence number
}

// WSSubscribeRequest represents subscribe/unsubscribe request
type WSSubscribeRequest struct {
	ID             string `json:"id"`                       // Request ID
	Type           string `json:"type"`                     // "subscribe" or "unsubscribe"
	Topic          string `json:"topic"`                    // Topic
	PrivateChannel bool   `json:"privateChannel,omitempty"` // Is private channel
	Response       bool   `json:"response"`                 // Require response
}

// WSPingMessage represents ping message
type WSPingMessage struct {
	ID   string `json:"id"`   // Message ID
	Type string `json:"type"` // "ping"
}

// =============================================================================
// WebSocket Data Types
// =============================================================================

// WSTickerData represents ticker data from WebSocket
type WSTickerData struct {
	Sequence     int64  `json:"sequence"`     // Sequence number
	Symbol       string `json:"symbol"`       // Symbol
	Side         string `json:"side"`         // Trade side
	Size         int    `json:"size"`         // Trade size
	Price        string `json:"price"`        // Trade price
	BestBidPrice string `json:"bestBidPrice"` // Best bid price
	BestBidSize  int    `json:"bestBidSize"`  // Best bid size
	BestAskPrice string `json:"bestAskPrice"` // Best ask price
	BestAskSize  int    `json:"bestAskSize"`  // Best ask size
	TradeID      string `json:"tradeId"`      // Trade ID
	Ts           int64  `json:"ts"`           // Timestamp
}

// WSLevel2Data represents level 2 orderbook data from WebSocket
type WSLevel2Data struct {
	Sequence int64       `json:"sequence"` // Sequence number
	Asks     [][]float64 `json:"asks"`     // Ask levels
	Bids     [][]float64 `json:"bids"`     // Bid levels
	Ts       int64       `json:"ts"`       // Timestamp
}

// WSLevel2Change represents level 2 incremental change
type WSLevel2Change struct {
	Sequence  int64  `json:"sequence"`  // Sequence number
	Change    string `json:"change"`    // Change: "price,side,size"
	Timestamp int64  `json:"timestamp"` // Timestamp
}

// WSExecutionData represents trade execution data from WebSocket
type WSExecutionData struct {
	Sequence     int64  `json:"sequence"`     // Sequence number
	TradeID      string `json:"tradeId"`      // Trade ID
	TakerOrderID string `json:"takerOrderId"` // Taker order ID
	MakerOrderID string `json:"makerOrderId"` // Maker order ID
	Price        string `json:"price"`        // Trade price
	Size         int    `json:"size"`         // Trade size
	Side         string `json:"side"`         // Trade side
	Ts           int64  `json:"ts"`           // Timestamp
}

// WSInstrumentMarkPrice represents mark/index price from WebSocket
type WSInstrumentMarkPrice struct {
	Granularity int64   `json:"granularity"` // Granularity
	IndexPrice  float64 `json:"indexPrice"`  // Index price
	MarkPrice   float64 `json:"markPrice"`   // Mark price
	Timestamp   int64   `json:"timestamp"`   // Timestamp
}

// WSInstrumentFundingRate represents funding rate from WebSocket
type WSInstrumentFundingRate struct {
	Granularity int64   `json:"granularity"` // Granularity
	FundingRate float64 `json:"fundingRate"` // Funding rate
	Timestamp   int64   `json:"timestamp"`   // Timestamp
}

// =============================================================================
// WebSocket Private Channel Data Types
// =============================================================================

// WSOrderChange represents order change from WebSocket
type WSOrderChange struct {
	OrderID      string `json:"orderId"`      // Order ID
	Symbol       string `json:"symbol"`       // Symbol
	Type         string `json:"type"`         // Change type
	Status       string `json:"status"`       // Order status
	MatchSize    string `json:"matchSize"`    // Match size
	MatchPrice   string `json:"matchPrice"`   // Match price
	OrderType    string `json:"orderType"`    // Order type
	Side         string `json:"side"`         // Side
	Price        string `json:"price"`        // Price
	Size         string `json:"size"`         // Size
	RemainSize   string `json:"remainSize"`   // Remaining size
	FilledSize   string `json:"filledSize"`   // Filled size
	CanceledSize string `json:"canceledSize"` // Canceled size
	TradeID      string `json:"tradeId"`      // Trade ID
	ClientOid    string `json:"clientOid"`    // Client order ID
	OrderTime    int64  `json:"orderTime"`    // Order timestamp (ns)
	OldSize      string `json:"oldSize"`      // Old size (for amend)
	Liquidity    string `json:"liquidity"`    // Liquidity: "maker" or "taker"
	Ts           int64  `json:"ts"`           // Timestamp (ns)
	MarginMode   string `json:"marginMode"`   // Margin mode
	PositionSide string `json:"positionSide"` // Position side
}

// WSPositionChange represents position change from WebSocket
type WSPositionChange struct {
	RealisedGrossPnl float64 `json:"realisedGrossPnl"` // Realized gross PnL
	Symbol           string  `json:"symbol"`           // Symbol
	CrossMode        bool    `json:"crossMode"`        // Is cross mode
	LiquidationPrice float64 `json:"liquidationPrice"` // Liquidation price
	PosLoss          float64 `json:"posLoss"`          // Position loss
	AvgEntryPrice    float64 `json:"avgEntryPrice"`    // Average entry price
	UnrealisedPnl    float64 `json:"unrealisedPnl"`    // Unrealized PnL
	MarkPrice        float64 `json:"markPrice"`        // Mark price
	PosMargin        float64 `json:"posMargin"`        // Position margin
	AutoDeposit      bool    `json:"autoDeposit"`      // Auto deposit
	RiskLimit        int64   `json:"riskLimit"`        // Risk limit
	UnrealisedCost   float64 `json:"unrealisedCost"`   // Unrealized cost
	PosComm          float64 `json:"posComm"`          // Position commission
	PosMaint         float64 `json:"posMaint"`         // Position maintenance margin
	PosCost          float64 `json:"posCost"`          // Position cost
	MaintMarginReq   float64 `json:"maintMarginReq"`   // Maintenance margin requirement
	CurrentQty       int     `json:"currentQty"`       // Current quantity
	CurrentCost      float64 `json:"currentCost"`      // Current cost
	DelevPercentage  float64 `json:"delevPercentage"`  // Deleverage percentage
	CurrentTimestamp int64   `json:"currentTimestamp"` // Current timestamp
	SettleCurrency   string  `json:"settleCurrency"`   // Settlement currency
	MarginMode       string  `json:"marginMode"`       // Margin mode
	PositionSide     string  `json:"positionSide"`     // Position side
}

// WSBalanceChange represents account balance change from WebSocket
type WSBalanceChange struct {
	Currency         string `json:"currency"`         // Currency
	HoldBalance      string `json:"holdBalance"`      // Hold balance
	AvailableBalance string `json:"availableBalance"` // Available balance
	Timestamp        int64  `json:"timestamp"`        // Timestamp
}

// =============================================================================
// Helper Functions
// =============================================================================

// Timestamp returns current timestamp in milliseconds
func Timestamp() int64 {
	return time.Now().UnixMilli()
}

// TimestampString returns current timestamp as string
func TimestampString() string {
	return fmt.Sprintf("%d", Timestamp())
}

// GenerateClientOid generates a unique client order ID
func GenerateClientOid() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
