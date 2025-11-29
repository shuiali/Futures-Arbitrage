// Package bitget provides types and clients for Bitget exchange API integration.
// Supports REST and WebSocket APIs for market data, trading, and account management.
// API Version: V2
package bitget

import (
	"encoding/json"
	"time"
)

// =============================================================================
// Common Types
// =============================================================================

// Product types for Bitget futures
const (
	ProductTypeUSDTFutures = "USDT-FUTURES" // USDT margined perpetual futures
	ProductTypeUSDCFutures = "USDC-FUTURES" // USDC margined perpetual futures
	ProductTypeCoinFutures = "COIN-FUTURES" // Coin margined perpetual futures
)

// Margin modes
const (
	MarginModeIsolated = "isolated"
	MarginModeCross    = "crossed"
)

// Hold modes (position mode)
const (
	HoldModeOneWay = "single_hold" // One-way mode
	HoldModeHedge  = "double_hold" // Hedge mode (long/short)
)

// Position sides
const (
	HoldSideLong  = "long"
	HoldSideShort = "short"
)

// Order sides
const (
	SideBuy  = "buy"
	SideSell = "sell"
)

// Trade sides (for fills)
const (
	TradeSideBuyer  = "Buyer"
	TradeSideSeller = "Seller"
)

// Order types
const (
	OrderTypeLimit  = "limit"
	OrderTypeMarket = "market"
)

// Order force types (time in force)
const (
	ForceGTC = "GTC"       // Good till canceled
	ForceIOC = "IOC"       // Immediate or cancel
	ForceFOK = "FOK"       // Fill or kill
	ForcePOC = "post_only" // Post only (maker only)
)

// Order statuses
const (
	OrderStatusInit     = "init"
	OrderStatusNew      = "new"
	OrderStatusLive     = "live"
	OrderStatusPartial  = "partially_filled"
	OrderStatusFilled   = "filled"
	OrderStatusCanceled = "canceled"
	OrderStatusRejected = "rejected"
)

// Plan types for algo orders
const (
	PlanTypeTP       = "take_profit"    // Take profit
	PlanTypeSL       = "stop_loss"      // Stop loss
	PlanTypeTrailing = "trailing_stop"  // Trailing stop
	PlanTypeMC       = "moving_average" // Moving average
)

// Timestamp is a custom time type for Bitget API timestamps (milliseconds as string)
type Timestamp int64

// Time returns the time.Time representation
func (t Timestamp) Time() time.Time {
	return time.UnixMilli(int64(t))
}

// UnmarshalJSON implements json.Unmarshaler for string timestamps
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		// Try as number
		var n int64
		if err := json.Unmarshal(data, &n); err != nil {
			return err
		}
		*t = Timestamp(n)
		return nil
	}
	if s == "" {
		*t = 0
		return nil
	}
	var n int64
	if err := json.Unmarshal([]byte(s), &n); err != nil {
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
	Code        string `json:"code"`
	Msg         string `json:"msg"`
	RequestTime int64  `json:"requestTime"`
	Data        T      `json:"data"`
}

// =============================================================================
// Contract / Instrument Types
// =============================================================================

// Contract represents a trading contract (futures instrument)
type Contract struct {
	Symbol              string   `json:"symbol"`              // Trading pair, e.g., "BTCUSDT"
	BaseCoin            string   `json:"baseCoin"`            // Base currency, e.g., "BTC"
	QuoteCoin           string   `json:"quoteCoin"`           // Quote currency, e.g., "USDT"
	BuyLimitPriceRatio  string   `json:"buyLimitPriceRatio"`  // Max buy price ratio vs mark price
	SellLimitPriceRatio string   `json:"sellLimitPriceRatio"` // Max sell price ratio vs mark price
	TakerFeeRate        string   `json:"takerFeeRate"`        // Taker fee rate
	MakerFeeRate        string   `json:"makerFeeRate"`        // Maker fee rate
	OpenCostUpRatio     string   `json:"openCostUpRatio"`     // Ratio for opening
	SupportMarginCoins  []string `json:"supportMarginCoins"`  // Supported margin coins
	MinTradeNum         string   `json:"minTradeNum"`         // Minimum order quantity
	PriceEndStep        string   `json:"priceEndStep"`        // Price step
	VolumePlace         string   `json:"volumePlace"`         // Quantity decimal places
	PricePlace          string   `json:"pricePlace"`          // Price decimal places
	SizeMultiplier      string   `json:"sizeMultiplier"`      // Contract size multiplier
	SymbolType          string   `json:"symbolType"`          // Symbol type: perpetual, delivery
	MinTradeUSDT        string   `json:"minTradeUSDT"`        // Minimum trade value in USDT
	MaxSymbolOrderNum   string   `json:"maxSymbolOrderNum"`   // Max orders per symbol
	MaxProductOrderNum  string   `json:"maxProductOrderNum"`  // Max orders per product
	MaxPositionNum      string   `json:"maxPositionNum"`      // Max positions
	SymbolStatus        string   `json:"symbolStatus"`        // Status: normal, maintain, limit
	OffTime             string   `json:"offTime"`             // Offline time
	LimitOpenTime       string   `json:"limitOpenTime"`       // Limit open time
	DeliveryTime        string   `json:"deliveryTime"`        // Delivery time (for delivery futures)
	DeliveryStartTime   string   `json:"deliveryStartTime"`   // Delivery start time
	DeliveryPeriod      string   `json:"deliveryPeriod"`      // Delivery period
	LaunchTime          string   `json:"launchTime"`          // Launch time
	FundInterval        string   `json:"fundInterval"`        // Funding interval in hours
	MinLever            string   `json:"minLever"`            // Minimum leverage
	MaxLever            string   `json:"maxLever"`            // Maximum leverage
	PosLimit            string   `json:"posLimit"`            // Position limit
	MaintainTime        string   `json:"maintainTime"`        // Maintenance time
}

// =============================================================================
// Market Data Types
// =============================================================================

// Ticker represents market ticker data
type Ticker struct {
	Symbol        string    `json:"symbol"`
	LastPr        string    `json:"lastPr"`        // Last price
	AskPr         string    `json:"askPr"`         // Best ask price
	BidPr         string    `json:"bidPr"`         // Best bid price
	AskSz         string    `json:"askSz"`         // Best ask size
	BidSz         string    `json:"bidSz"`         // Best bid size
	High24h       string    `json:"high24h"`       // 24h high
	Low24h        string    `json:"low24h"`        // 24h low
	Ts            Timestamp `json:"ts"`            // Timestamp
	Change24h     string    `json:"change24h"`     // 24h change (absolute)
	BaseVolume    string    `json:"baseVolume"`    // 24h volume in base currency
	QuoteVolume   string    `json:"quoteVolume"`   // 24h volume in quote currency
	UsdtVolume    string    `json:"usdtVolume"`    // 24h volume in USDT
	OpenUtc       string    `json:"openUtc"`       // UTC open price
	ChangeUtc24h  string    `json:"changeUtc24h"`  // UTC 24h change
	IndexPrice    string    `json:"indexPrice"`    // Index price
	FundingRate   string    `json:"fundingRate"`   // Current funding rate
	HoldingAmount string    `json:"holdingAmount"` // Open interest
	Open24h       string    `json:"open24h"`       // 24h open price
	DeliveryPrice string    `json:"deliveryPrice"` // Delivery price
	DeliveryTime  Timestamp `json:"deliveryTime"`  // Delivery time
}

// AllTickers wraps list of tickers
type AllTickers []Ticker

// OrderBookLevel represents a single order book level
type OrderBookLevel struct {
	Price string
	Size  string
}

// UnmarshalJSON implements json.Unmarshaler for OrderBookLevel
func (o *OrderBookLevel) UnmarshalJSON(data []byte) error {
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	if len(arr) >= 2 {
		o.Price = arr[0]
		o.Size = arr[1]
	}
	return nil
}

// MarshalJSON implements json.Marshaler for OrderBookLevel
func (o OrderBookLevel) MarshalJSON() ([]byte, error) {
	return json.Marshal([]string{o.Price, o.Size})
}

// OrderBook represents order book data (merge-depth endpoint)
type OrderBook struct {
	Asks           []OrderBookLevel `json:"asks"`
	Bids           []OrderBookLevel `json:"bids"`
	Ts             Timestamp        `json:"ts"`
	Scale          string           `json:"scale"`          // Precision scale
	Precision      string           `json:"precision"`      // Price precision
	IsMaxPrecision string           `json:"isMaxPrecision"` // Is max precision
}

// Candlestick represents OHLCV candlestick data
// Array format: [ts, open, high, low, close, baseVol, quoteVol]
type Candlestick struct {
	Ts          Timestamp
	Open        string
	High        string
	Low         string
	Close       string
	BaseVolume  string // Volume in base currency
	QuoteVolume string // Volume in quote currency
}

// UnmarshalJSON implements json.Unmarshaler for Candlestick
func (c *Candlestick) UnmarshalJSON(data []byte) error {
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	if len(arr) >= 7 {
		var ts int64
		if err := json.Unmarshal([]byte(arr[0]), &ts); err != nil {
			return err
		}
		c.Ts = Timestamp(ts)
		c.Open = arr[1]
		c.High = arr[2]
		c.Low = arr[3]
		c.Close = arr[4]
		c.BaseVolume = arr[5]
		c.QuoteVolume = arr[6]
	}
	return nil
}

// Trade represents a public trade
type Trade struct {
	TradeID string    `json:"tradeId"`
	Price   string    `json:"price"`
	Size    string    `json:"size"`
	Side    string    `json:"side"` // buy or sell
	Ts      Timestamp `json:"ts"`
	Symbol  string    `json:"symbol"`
}

// =============================================================================
// Funding Rate Types
// =============================================================================

// FundingRate represents current funding rate information
type FundingRate struct {
	Symbol      string `json:"symbol"`
	FundingRate string `json:"fundingRate"` // Current funding rate
}

// FundingRateHistory represents historical funding rate
type FundingRateHistory struct {
	Symbol      string    `json:"symbol"`
	FundingRate string    `json:"fundingRate"`
	SettleTime  Timestamp `json:"settleTime"` // Settlement timestamp
}

// =============================================================================
// Account Types
// =============================================================================

// Account represents a single account balance
type Account struct {
	MarginCoin            string `json:"marginCoin"`            // Margin coin
	Locked                string `json:"locked"`                // Locked balance
	Available             string `json:"available"`             // Available balance
	CrossedMaxAvailable   string `json:"crossedMaxAvailable"`   // Max available for cross
	IsolatedMaxAvailable  string `json:"isolatedMaxAvailable"`  // Max available for isolated
	MaxTransferOut        string `json:"maxTransferOut"`        // Max transfer out amount
	AccountEquity         string `json:"accountEquity"`         // Account equity
	UsdtEquity            string `json:"usdtEquity"`            // Equity in USDT value
	BtcEquity             string `json:"btcEquity"`             // Equity in BTC value
	CrossedRiskRate       string `json:"crossedRiskRate"`       // Cross margin risk rate
	CrossedMarginLeverage string `json:"crossedMarginLeverage"` // Cross margin leverage
	IsolatedLongLever     string `json:"isolatedLongLever"`     // Isolated long leverage
	IsolatedShortLever    string `json:"isolatedShortLever"`    // Isolated short leverage
	MarginMode            string `json:"marginMode"`            // Margin mode: crossed or isolated
	PosMode               string `json:"posMode"`               // Position mode
	UnrealizedPL          string `json:"unrealizedPL"`          // Unrealized PnL
	Coupon                string `json:"coupon"`                // Coupon balance
	CrossedUnrealizedPL   string `json:"crossedUnrealizedPL"`   // Cross unrealized PnL
	IsolatedUnrealizedPL  string `json:"isolatedUnrealizedPL"`  // Isolated unrealized PnL
}

// AccountList represents multiple account balances
type AccountList struct {
	MarginCoin           string `json:"marginCoin"`
	Locked               string `json:"locked"`
	Available            string `json:"available"`
	CrossedMaxAvailable  string `json:"crossedMaxAvailable"`
	IsolatedMaxAvailable string `json:"isolatedMaxAvailable"`
	MaxTransferOut       string `json:"maxTransferOut"`
	AccountEquity        string `json:"accountEquity"`
	UsdtEquity           string `json:"usdtEquity"`
	BtcEquity            string `json:"btcEquity"`
}

// =============================================================================
// Position Types
// =============================================================================

// Position represents a futures position
type Position struct {
	Symbol            string    `json:"symbol"`
	MarginCoin        string    `json:"marginCoin"`
	HoldSide          string    `json:"holdSide"`         // long or short
	OpenDelegateSize  string    `json:"openDelegateSize"` // Open order size
	MarginSize        string    `json:"marginSize"`       // Margin size
	Available         string    `json:"available"`        // Available to close
	Locked            string    `json:"locked"`           // Locked
	Total             string    `json:"total"`            // Total position
	Leverage          string    `json:"leverage"`
	AchievedProfits   string    `json:"achievedProfits"`   // Realized profits
	OpenPriceAvg      string    `json:"openPriceAvg"`      // Average open price
	MarginMode        string    `json:"marginMode"`        // crossed or isolated
	PosMode           string    `json:"posMode"`           // Position mode
	UnrealizedPL      string    `json:"unrealizedPL"`      // Unrealized PnL
	LiquidationPrice  string    `json:"liquidationPrice"`  // Liquidation price
	KeepMarginRate    string    `json:"keepMarginRate"`    // Maintenance margin rate
	MarkPrice         string    `json:"markPrice"`         // Mark price
	MarginRatio       string    `json:"marginRatio"`       // Margin ratio
	BreakEvenPrice    string    `json:"breakEvenPrice"`    // Break-even price
	TotalFee          string    `json:"totalFee"`          // Total fees
	DeductedFee       string    `json:"deductedFee"`       // Deducted fees
	GrantMarginAmount string    `json:"grantMarginAmount"` // Granted margin
	CTime             Timestamp `json:"cTime"`             // Create time
	UTime             Timestamp `json:"uTime"`             // Update time
}

// PositionHistory represents historical position data
type PositionHistory struct {
	Symbol        string    `json:"symbol"`
	MarginCoin    string    `json:"marginCoin"`
	HoldSide      string    `json:"holdSide"`
	OpenAvgPrice  string    `json:"openAvgPrice"`
	CloseAvgPrice string    `json:"closeAvgPrice"`
	MarginMode    string    `json:"marginMode"`
	OpenTotalPos  string    `json:"openTotalPos"`
	CloseTotalPos string    `json:"closeTotalPos"`
	PNL           string    `json:"pnl"`          // Total PnL
	NetProfit     string    `json:"netProfit"`    // Net profit
	TotalFunding  string    `json:"totalFunding"` // Total funding fees
	OpenFee       string    `json:"openFee"`
	CloseFee      string    `json:"closeFee"`
	CTime         Timestamp `json:"ctime"`
	UTime         Timestamp `json:"utime"`
}

// =============================================================================
// Order Types
// =============================================================================

// Order represents an order
type Order struct {
	Symbol           string      `json:"symbol"`
	OrderID          string      `json:"orderId"`
	ClientOID        string      `json:"clientOid"` // Client order ID
	Size             string      `json:"size"`      // Order size
	OrderType        string      `json:"orderType"` // limit or market
	Side             string      `json:"side"`      // buy or sell
	PosSide          string      `json:"posSide"`   // Position side
	MarginCoin       string      `json:"marginCoin"`
	MarginMode       string      `json:"marginMode"`
	EnterPointSource string      `json:"enterPointSource"` // Entry source
	TradeSide        string      `json:"tradeSide"`        // Trade side
	HoldMode         string      `json:"holdMode"`         // Hold mode
	Price            string      `json:"price"`            // Order price
	NewSize          string      `json:"newSize"`          // New size (after modify)
	NotionalUsd      string      `json:"notionalUsd"`      // Notional in USD
	Leverage         string      `json:"leverage"`
	BaseVolume       string      `json:"baseVolume"`   // Filled volume in base
	QuoteVolume      string      `json:"quoteVolume"`  // Filled volume in quote
	PriceAvg         string      `json:"priceAvg"`     // Average fill price
	State            string      `json:"state"`        // Order state
	Force            string      `json:"force"`        // Time in force: GTC, IOC, FOK, post_only
	TotalProfits     string      `json:"totalProfits"` // Total profits
	ReduceOnly       string      `json:"reduceOnly"`   // Reduce only flag
	CTime            Timestamp   `json:"cTime"`
	UTime            Timestamp   `json:"uTime"`
	FeeDetail        []FeeDetail `json:"feeDetail,omitempty"`
}

// FeeDetail represents fee breakdown
type FeeDetail struct {
	FeeCoin string `json:"feeCoin"`
	Fee     string `json:"fee"`
}

// OrderResult represents order placement result
type OrderResult struct {
	OrderID   string `json:"orderId"`
	ClientOID string `json:"clientOid"`
}

// BatchOrderResult represents batch order placement result
type BatchOrderResult struct {
	OrderInfo []OrderResult `json:"orderInfo"`
	Failure   []FailedOrder `json:"failure,omitempty"`
}

// FailedOrder represents a failed order in batch
type FailedOrder struct {
	OrderID   string `json:"orderId"`
	ClientOID string `json:"clientOid"`
	ErrorCode string `json:"errorCode"`
	ErrorMsg  string `json:"errorMsg"`
}

// CancelResult represents order cancellation result
type CancelResult struct {
	OrderID   string `json:"orderId"`
	ClientOID string `json:"clientOid"`
}

// =============================================================================
// Order Request Types
// =============================================================================

// PlaceOrderRequest represents order placement request
type PlaceOrderRequest struct {
	Symbol                 string `json:"symbol"`
	ProductType            string `json:"productType"`
	MarginMode             string `json:"marginMode,omitempty"` // crossed or isolated
	MarginCoin             string `json:"marginCoin"`
	Size                   string `json:"size"`
	Price                  string `json:"price,omitempty"`                  // Required for limit
	Side                   string `json:"side"`                             // buy or sell
	TradeSide              string `json:"tradeSide,omitempty"`              // open or close
	OrderType              string `json:"orderType"`                        // limit or market
	Force                  string `json:"force,omitempty"`                  // GTC, IOC, FOK, post_only
	ClientOID              string `json:"clientOid,omitempty"`              // Client order ID
	ReduceOnly             string `json:"reduceOnly,omitempty"`             // true or false
	PresetStopSurplusPrice string `json:"presetStopSurplusPrice,omitempty"` // TP price
	PresetStopLossPrice    string `json:"presetStopLossPrice,omitempty"`    // SL price
}

// CancelOrderRequest represents order cancellation request
type CancelOrderRequest struct {
	Symbol      string `json:"symbol"`
	ProductType string `json:"productType"`
	OrderID     string `json:"orderId,omitempty"`
	ClientOID   string `json:"clientOid,omitempty"`
	MarginCoin  string `json:"marginCoin,omitempty"`
}

// BatchPlaceOrderRequest represents batch order request
type BatchPlaceOrderRequest struct {
	Symbol      string           `json:"symbol"`
	ProductType string           `json:"productType"`
	MarginMode  string           `json:"marginMode,omitempty"`
	MarginCoin  string           `json:"marginCoin"`
	OrderList   []PlaceOrderItem `json:"orderList"`
}

// PlaceOrderItem represents a single order in batch
type PlaceOrderItem struct {
	Size       string `json:"size"`
	Price      string `json:"price,omitempty"`
	Side       string `json:"side"`
	TradeSide  string `json:"tradeSide,omitempty"`
	OrderType  string `json:"orderType"`
	Force      string `json:"force,omitempty"`
	ClientOID  string `json:"clientOid,omitempty"`
	ReduceOnly string `json:"reduceOnly,omitempty"`
}

// BatchCancelOrderRequest represents batch cancel request
type BatchCancelOrderRequest struct {
	Symbol      string       `json:"symbol"`
	ProductType string       `json:"productType"`
	MarginCoin  string       `json:"marginCoin,omitempty"`
	OrderIDList []CancelItem `json:"orderIdList"`
}

// CancelItem represents a single cancel in batch
type CancelItem struct {
	OrderID   string `json:"orderId,omitempty"`
	ClientOID string `json:"clientOid,omitempty"`
}

// ModifyOrderRequest represents order modification request
type ModifyOrderRequest struct {
	Symbol      string `json:"symbol"`
	ProductType string `json:"productType"`
	OrderID     string `json:"orderId,omitempty"`
	ClientOID   string `json:"clientOid,omitempty"`
	NewSize     string `json:"newSize,omitempty"`
	NewPrice    string `json:"newPrice,omitempty"`
}

// =============================================================================
// WebSocket Types
// =============================================================================

// WSRequest represents a WebSocket request
type WSRequest struct {
	Op   string        `json:"op"`
	Args []interface{} `json:"args"`
}

// WSLoginRequest represents WebSocket login request
type WSLoginRequest struct {
	Op   string       `json:"op"`
	Args []WSLoginArg `json:"args"`
}

// WSLoginArg represents WebSocket login arguments
type WSLoginArg struct {
	APIKey     string `json:"apiKey"`
	Passphrase string `json:"passphrase"`
	Timestamp  string `json:"timestamp"`
	Sign       string `json:"sign"`
}

// WSSubscribeRequest represents WebSocket subscription request
type WSSubscribeRequest struct {
	Op   string           `json:"op"`
	Args []WSSubscribeArg `json:"args"`
}

// WSSubscribeArg represents WebSocket subscription arguments
type WSSubscribeArg struct {
	InstType string `json:"instType"`         // USDT-FUTURES, USDC-FUTURES, etc.
	Channel  string `json:"channel"`          // ticker, books, trade, etc.
	InstID   string `json:"instId,omitempty"` // Symbol (optional for some channels)
	Coin     string `json:"coin,omitempty"`   // Coin for account channel
}

// WSResponse represents a WebSocket response
type WSResponse struct {
	Event  string          `json:"event,omitempty"`
	Code   string          `json:"code,omitempty"`
	Msg    string          `json:"msg,omitempty"`
	Action string          `json:"action,omitempty"` // snapshot, update
	Arg    json.RawMessage `json:"arg,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
	ConnID string          `json:"connId,omitempty"`
}

// WSTradeRequest represents WebSocket trading request
type WSTradeRequest struct {
	ID   string        `json:"id"`
	Op   string        `json:"op"`
	Args []interface{} `json:"args"`
}

// WSTradeResponse represents WebSocket trading response
type WSTradeResponse struct {
	ID      string          `json:"id"`
	Op      string          `json:"op"`
	Code    string          `json:"code"`
	Msg     string          `json:"msg"`
	Data    json.RawMessage `json:"data,omitempty"`
	ReqTime Timestamp       `json:"reqTime,omitempty"`
}

// =============================================================================
// WebSocket Push Data Types
// =============================================================================

// WSTickerData represents ticker push data
type WSTickerData struct {
	InstID        string    `json:"instId"`
	LastPr        string    `json:"lastPr"`
	AskPr         string    `json:"askPr"`
	BidPr         string    `json:"bidPr"`
	AskSz         string    `json:"askSz"`
	BidSz         string    `json:"bidSz"`
	High24h       string    `json:"high24h"`
	Low24h        string    `json:"low24h"`
	Change24h     string    `json:"change24h"`
	BaseVolume    string    `json:"baseVolume"`
	QuoteVolume   string    `json:"quoteVolume"`
	IndexPrice    string    `json:"indexPrice"`
	FundingRate   string    `json:"fundingRate"`
	HoldingAmount string    `json:"holdingAmount"`
	Open24h       string    `json:"open24h"`
	Ts            Timestamp `json:"ts"`
}

// WSOrderBookData represents order book push data
type WSOrderBookData struct {
	Asks     []OrderBookLevel `json:"asks"`
	Bids     []OrderBookLevel `json:"bids"`
	Checksum int64            `json:"checksum"`
	Ts       Timestamp        `json:"ts"`
}

// WSTradeData represents trade push data
type WSTradeData struct {
	Ts      Timestamp `json:"ts"`
	Price   string    `json:"price"`
	Size    string    `json:"size"`
	Side    string    `json:"side"`
	TradeID string    `json:"tradeId"`
}

// WSCandleData represents candlestick push data
// Array format: [ts, o, h, l, c, baseVol, quoteVol]
type WSCandleData struct {
	Ts          Timestamp
	Open        string
	High        string
	Low         string
	Close       string
	BaseVolume  string
	QuoteVolume string
}

// UnmarshalJSON implements json.Unmarshaler for WSCandleData
func (c *WSCandleData) UnmarshalJSON(data []byte) error {
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	if len(arr) >= 7 {
		var ts int64
		if err := json.Unmarshal([]byte(arr[0]), &ts); err != nil {
			return err
		}
		c.Ts = Timestamp(ts)
		c.Open = arr[1]
		c.High = arr[2]
		c.Low = arr[3]
		c.Close = arr[4]
		c.BaseVolume = arr[5]
		c.QuoteVolume = arr[6]
	}
	return nil
}

// WSAccountData represents account push data
type WSAccountData struct {
	MarginCoin           string `json:"marginCoin"`
	Locked               string `json:"locked"`
	Available            string `json:"available"`
	MaxOpenPosAvailable  string `json:"maxOpenPosAvailable"`
	MaxTransferOut       string `json:"maxTransferOut"`
	Equity               string `json:"equity"`
	UsdtEquity           string `json:"usdtEquity"`
	BtcEquity            string `json:"btcEquity"`
	UnrealizedPL         string `json:"unrealizedPL"`
	Coupon               string `json:"coupon"`
	CrossedUnrealizedPL  string `json:"crossedUnrealizedPL"`
	IsolatedUnrealizedPL string `json:"isolatedUnrealizedPL"`
}

// WSEquityData represents equity push data
type WSEquityData struct {
	TotalEquity    string `json:"totalEquity"`
	IsolatedEquity string `json:"isolatedEquity"`
	CrossedEquity  string `json:"crossedEquity"`
}

// WSPositionData represents position push data
type WSPositionData struct {
	InstID           string    `json:"instId"`
	PosID            string    `json:"posId"`
	MarginCoin       string    `json:"marginCoin"`
	MarginSize       string    `json:"marginSize"`
	MarginMode       string    `json:"marginMode"`
	HoldSide         string    `json:"holdSide"`
	HoldMode         string    `json:"holdMode"`
	Total            string    `json:"total"`
	Available        string    `json:"available"`
	Frozen           string    `json:"frozen"`
	OpenPriceAvg     string    `json:"openPriceAvg"`
	Leverage         string    `json:"leverage"`
	AchievedProfits  string    `json:"achievedProfits"`
	UnrealizedPL     string    `json:"unrealizedPL"`
	UnrealizedPLR    string    `json:"unrealizedPLR"` // Unrealized PnL ratio
	LiquidationPrice string    `json:"liquidationPrice"`
	KeepMarginRate   string    `json:"keepMarginRate"`
	MarginRate       string    `json:"marginRate"`
	CTime            Timestamp `json:"cTime"`
	UTime            Timestamp `json:"uTime"`
}

// WSOrderData represents order push data
type WSOrderData struct {
	InstID           string      `json:"instId"`
	OrderID          string      `json:"orderId"`
	ClientOID        string      `json:"clientOid"`
	Size             string      `json:"size"`
	NewSize          string      `json:"newSize"`
	Notional         string      `json:"notional"`
	OrderType        string      `json:"orderType"`
	Force            string      `json:"force"`
	Side             string      `json:"side"`
	PosSide          string      `json:"posSide"`
	FillPrice        string      `json:"fillPrice"`
	TradeID          string      `json:"tradeId"`
	BaseVolume       string      `json:"baseVolume"`
	FillTime         Timestamp   `json:"fillTime"`
	FillFee          string      `json:"fillFee"`
	FillFeeCoin      string      `json:"fillFeeCoin"`
	TradeScope       string      `json:"tradeScope"`
	AccBaseVolume    string      `json:"accBaseVolume"`
	PriceAvg         string      `json:"priceAvg"`
	Status           string      `json:"status"`
	CTime            Timestamp   `json:"cTime"`
	UTime            Timestamp   `json:"uTime"`
	FeeDetail        []FeeDetail `json:"feeDetail,omitempty"`
	EnterPointSource string      `json:"enterPointSource"`
	TradeSide        string      `json:"tradeSide"`
	PosMode          string      `json:"posMode"`
	MarginCoin       string      `json:"marginCoin"`
	MarginMode       string      `json:"marginMode"`
	Leverage         string      `json:"leverage"`
	Price            string      `json:"price"`
	ReduceOnly       string      `json:"reduceOnly"`
}

// WSFillData represents fill push data
type WSFillData struct {
	TradeID          string      `json:"tradeId"`
	OrderID          string      `json:"orderId"`
	Symbol           string      `json:"symbol"`
	Side             string      `json:"side"`
	PriceAvg         string      `json:"priceAvg"`
	BaseVolume       string      `json:"baseVolume"`
	QuoteVolume      string      `json:"quoteVolume"`
	Profit           string      `json:"profit"`
	EnterPointSource string      `json:"enterPointSource"`
	TradeSide        string      `json:"tradeSide"`
	PosMode          string      `json:"posMode"`
	MarginCoin       string      `json:"marginCoin"`
	MarginMode       string      `json:"marginMode"`
	FeeDetail        []FeeDetail `json:"feeDetail,omitempty"`
	CTime            Timestamp   `json:"cTime"`
	UTime            Timestamp   `json:"uTime"`
}

// WSPlaceOrderArg represents WebSocket place order argument
type WSPlaceOrderArg struct {
	InstID               string `json:"instId"`
	MarginCoin           string `json:"marginCoin"`
	Size                 string `json:"size"`
	Price                string `json:"price,omitempty"`
	Side                 string `json:"side"`
	TradeSide            string `json:"tradeSide,omitempty"`
	OrderType            string `json:"orderType"`
	Force                string `json:"force,omitempty"`
	ClientOID            string `json:"clientOid,omitempty"`
	ReduceOnly           string `json:"reduceOnly,omitempty"`
	PresetTPTriggerPrice string `json:"presetStopSurplusPrice,omitempty"`
	PresetSLTriggerPrice string `json:"presetStopLossPrice,omitempty"`
}

// WSCancelOrderArg represents WebSocket cancel order argument
type WSCancelOrderArg struct {
	InstID    string `json:"instId"`
	OrderID   string `json:"orderId,omitempty"`
	ClientOID string `json:"clientOid,omitempty"`
}

// =============================================================================
// API Error
// =============================================================================

// APIError represents an API error
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"msg"`
}

// Error implements error interface
func (e *APIError) Error() string {
	return "Bitget API error " + e.Code + ": " + e.Message
}

// Common error codes
const (
	ErrCodeSuccess             = "00000"
	ErrCodeSignatureError      = "40101"
	ErrCodeTimestampExpired    = "40102"
	ErrCodeApiKeyInvalid       = "40103"
	ErrCodePassphraseInvalid   = "40104"
	ErrCodeIPNotAllowed        = "40105"
	ErrCodePermissionDenied    = "40106"
	ErrCodeSymbolNotFound      = "40301"
	ErrCodeOrderNotFound       = "40302"
	ErrCodeOrderFailed         = "40303"
	ErrCodeInsufficientBalance = "40304"
	ErrCodeOrderSizeTooSmall   = "40305"
	ErrCodeLeverageInvalid     = "40306"
	ErrCodePositionNotFound    = "40307"
	ErrCodeRateLimitExceeded   = "40901"
	ErrCodeSystemError         = "50000"
)

// IsSuccess checks if the response code indicates success
func IsSuccess(code string) bool {
	return code == ErrCodeSuccess
}

// =============================================================================
// Candlestick Granularities
// =============================================================================

// Candle granularities for REST and WebSocket
const (
	Granularity1m  = "1m"
	Granularity3m  = "3m"
	Granularity5m  = "5m"
	Granularity15m = "15m"
	Granularity30m = "30m"
	Granularity1H  = "1H"
	Granularity2H  = "2H"
	Granularity4H  = "4H"
	Granularity6H  = "6H"
	Granularity12H = "12H"
	Granularity1D  = "1D"
	Granularity3D  = "3D"
	Granularity1W  = "1W"
	Granularity1M  = "1M"
)

// WebSocket channel names
const (
	ChannelTicker    = "ticker"
	ChannelBooks     = "books"   // Full orderbook
	ChannelBooks1    = "books1"  // Top 1 level
	ChannelBooks5    = "books5"  // Top 5 levels
	ChannelBooks15   = "books15" // Top 15 levels
	ChannelTrade     = "trade"
	ChannelCandle1m  = "candle1m"
	ChannelCandle5m  = "candle5m"
	ChannelCandle15m = "candle15m"
	ChannelCandle30m = "candle30m"
	ChannelCandle1H  = "candle1H"
	ChannelCandle4H  = "candle4H"
	ChannelCandle12H = "candle12H"
	ChannelCandle1D  = "candle1D"
	ChannelCandle1W  = "candle1W"
	ChannelAccount   = "account"
	ChannelEquity    = "equity"
	ChannelPositions = "positions"
	ChannelOrders    = "orders"
	ChannelFill      = "fill"
)

// WebSocket trading channels
const (
	WSOpPlaceOrder       = "place-order"
	WSOpBatchPlaceOrder  = "batch-place-order"
	WSOpCancelOrder      = "cancel-order"
	WSOpBatchCancelOrder = "batch-cancel-order"
)
