// Package bingx provides types and clients for BingX Perpetual Futures exchange API integration.
// Supports REST and WebSocket APIs for perpetual swap market data, trading, and account management.
// API Documentation: https://bingx-api.github.io/docs/
package bingx

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
	RESTBaseURL         = "https://open-api.bingx.com"
	WSMarketDataURL     = "wss://open-api-swap.bingx.com/swap-market"
	WSUserDataURLFormat = "wss://open-api-swap.bingx.com/swap-market?listenKey=%s"
)

// Margin modes
const (
	MarginModeIsolated = "ISOLATED"
	MarginModeCross    = "CROSSED"
)

// Position sides (for hedge mode)
const (
	PositionSideLong  = "LONG"
	PositionSideShort = "SHORT"
)

// Order sides
const (
	OrderSideBuy  = "BUY"
	OrderSideSell = "SELL"
)

// Order types
const (
	OrderTypeLimit            = "LIMIT"
	OrderTypeMarket           = "MARKET"
	OrderTypeStopMarket       = "STOP_MARKET"
	OrderTypeTakeProfitMarket = "TAKE_PROFIT_MARKET"
	OrderTypeStop             = "STOP"
	OrderTypeTakeProfit       = "TAKE_PROFIT"
	OrderTypeTriggerLimit     = "TRIGGER_LIMIT"
	OrderTypeTriggerMarket    = "TRIGGER_MARKET"
)

// Time in force
const (
	TIFGoodTillCancel    = "GTC"      // Good till cancelled
	TIFImmediateOrCancel = "IOC"      // Immediate or cancel
	TIFFillOrKill        = "FOK"      // Fill or kill
	TIFPostOnly          = "PostOnly" // Post only (maker only)
)

// Order status
const (
	OrderStatusNew             = "NEW"
	OrderStatusPending         = "PENDING"
	OrderStatusPartiallyFilled = "PARTIALLY_FILLED"
	OrderStatusFilled          = "FILLED"
	OrderStatusCanceled        = "CANCELED"
	OrderStatusFailed          = "FAILED"
)

// Execution types (from WebSocket)
const (
	ExecTypeNew      = "NEW"
	ExecTypeTrade    = "TRADE"
	ExecTypeCanceled = "CANCELED"
)

// Kline intervals
const (
	Kline1m  = "1m"
	Kline3m  = "3m"
	Kline5m  = "5m"
	Kline15m = "15m"
	Kline30m = "30m"
	Kline1h  = "1h"
	Kline2h  = "2h"
	Kline4h  = "4h"
	Kline6h  = "6h"
	Kline8h  = "8h"
	Kline12h = "12h"
	Kline1d  = "1d"
	Kline3d  = "3d"
	Kline1w  = "1w"
	Kline1M  = "1M"
)

// Income types
const (
	IncomeTypeTransfer        = "TRANSFER"
	IncomeTypeRealizedPNL     = "REALIZED_PNL"
	IncomeTypeFundingFee      = "FUNDING_FEE"
	IncomeTypeTradingFee      = "TRADING_FEE"
	IncomeTypeInsuranceClear  = "INSURANCE_CLEAR"
	IncomeTypeTrialFund       = "TRIAL_FUND"
	IncomeTypeADL             = "ADL"
	IncomeTypeSystemDeduction = "SYSTEM_DEDUCTION"
)

// WebSocket subscription data types
const (
	WSDataTypeDepth5   = "@depth5"
	WSDataTypeDepth10  = "@depth10"
	WSDataTypeDepth20  = "@depth20"
	WSDataTypeDepth50  = "@depth50"
	WSDataTypeDepth100 = "@depth100"
	WSDataTypeTrade    = "@trade"
	WSDataTypeKline    = "@kline_"
)

// WebSocket event types (for user data stream)
const (
	WSEventAccountUpdate    = "ACCOUNT_UPDATE"
	WSEventOrderTradeUpdate = "ORDER_TRADE_UPDATE"
	WSEventListenKeyExpired = "listenKeyExpired"
)

// API error codes
const (
	ErrCodeSuccess             = 0
	ErrCodeSignatureFailed     = 100001
	ErrCodeInsufficientBalance = 100202
	ErrCodeInvalidParameter    = 100400
	ErrCodePriceLimit          = 100440
	ErrCodeServerError         = 100500
	ErrCodeServerBusy          = 100503
	ErrCodeRateLimit           = 80001
	ErrCodeServiceUnavailable  = 80012
	ErrCodeInvalidSymbol       = 80014
	ErrCodeOrderNotExist       = 80016
	ErrCodePositionNotExist    = 80017
)

// =============================================================================
// REST API Response Wrapper
// =============================================================================

// Response represents a standard BingX API response
type Response struct {
	Code    int             `json:"code"`
	Msg     string          `json:"msg,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Success bool            `json:"success,omitempty"`
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
	return fmt.Errorf("BingX API error: code=%d, msg=%s", r.Code, r.Msg)
}

// =============================================================================
// Contract Types
// =============================================================================

// Contract represents a perpetual swap contract
type Contract struct {
	Symbol            string  `json:"symbol"`            // Trading pair, e.g., BTC-USDT
	PricePrecision    int     `json:"pricePrecision"`    // Price precision
	QuantityPrecision int     `json:"quantityPrecision"` // Quantity precision
	TradeMinLimit     int     `json:"tradeMinLimit"`     // Minimum trading unit (contracts)
	Currency          string  `json:"currency"`          // Settlement and margin currency
	Asset             string  `json:"asset"`             // Contract trading asset
	Status            int     `json:"status"`            // 0=offline, 1=online
	MaxLongLeverage   int     `json:"maxLongLeverage"`   // Maximum leverage for long
	MaxShortLeverage  int     `json:"maxShortLeverage"`  // Maximum leverage for short
	FeeRate           string  `json:"feeRate"`           // Trading fee rate
	Size              string  `json:"size"`              // Contract size
	ContractID        string  `json:"contractId"`        // Contract ID
	MaxOrderQty       float64 `json:"maxOrderQty"`       // Maximum order quantity
	MinOrderQty       float64 `json:"minOrderQty"`       // Minimum order quantity
	MaxPositionVolume float64 `json:"maxPositionVolume"` // Maximum position volume
}

// IsOnline returns whether the contract is active for trading
func (c *Contract) IsOnline() bool {
	return c.Status == 1
}

// =============================================================================
// Ticker Types
// =============================================================================

// Ticker represents 24hr price ticker data
type Ticker struct {
	Symbol             string `json:"symbol"`             // Trading pair
	PriceChange        string `json:"priceChange"`        // Price change
	PriceChangePercent string `json:"priceChangePercent"` // Price change percent
	LastPrice          string `json:"lastPrice"`          // Last price
	LastQty            string `json:"lastQty"`            // Last quantity
	HighPrice          string `json:"highPrice"`          // High price
	LowPrice           string `json:"lowPrice"`           // Low price
	Volume             string `json:"volume"`             // Volume
	QuoteVolume        string `json:"quoteVolume"`        // Quote volume
	OpenPrice          string `json:"openPrice"`          // Open price
	OpenTime           int64  `json:"openTime"`           // Open time
	CloseTime          int64  `json:"closeTime"`          // Close time
}

// Price represents current price for a symbol
type Price struct {
	Symbol string `json:"symbol"` // Trading pair
	Price  string `json:"price"`  // Current price
	Time   int64  `json:"time"`   // Timestamp
}

// =============================================================================
// OrderBook Types
// =============================================================================

// OrderBook represents order book depth data
type OrderBook struct {
	Symbol string     `json:"symbol,omitempty"` // Symbol (only in REST response)
	Bids   [][]string `json:"bids"`             // Bid levels [[price, quantity], ...]
	Asks   [][]string `json:"asks"`             // Ask levels [[price, quantity], ...]
	T      int64      `json:"T"`                // Timestamp (milliseconds)
}

// OrderBookLevel represents a single order book level
type OrderBookLevel struct {
	Price    float64
	Quantity float64
}

// ParseBids parses bid levels from string arrays
func (o *OrderBook) ParseBids() []OrderBookLevel {
	return parseOrderBookLevels(o.Bids)
}

// ParseAsks parses ask levels from string arrays
func (o *OrderBook) ParseAsks() []OrderBookLevel {
	return parseOrderBookLevels(o.Asks)
}

// parseOrderBookLevels converts string arrays to OrderBookLevel
func parseOrderBookLevels(levels [][]string) []OrderBookLevel {
	result := make([]OrderBookLevel, 0, len(levels))
	for _, level := range levels {
		if len(level) >= 2 {
			var price, qty float64
			fmt.Sscanf(level[0], "%f", &price)
			fmt.Sscanf(level[1], "%f", &qty)
			result = append(result, OrderBookLevel{Price: price, Quantity: qty})
		}
	}
	return result
}

// =============================================================================
// Trade Types
// =============================================================================

// Trade represents a single trade
type Trade struct {
	Time         int64  `json:"time"`         // Transaction time
	IsBuyerMaker bool   `json:"isBuyerMaker"` // Whether buyer is maker
	Price        string `json:"price"`        // Transaction price
	Qty          string `json:"qty"`          // Transaction quantity
	QuoteQty     string `json:"quoteQty"`     // Turnover
}

// =============================================================================
// Kline Types
// =============================================================================

// Kline represents a candlestick/kline
type Kline struct {
	Open   string `json:"open"`   // Open price
	High   string `json:"high"`   // High price
	Low    string `json:"low"`    // Low price
	Close  string `json:"close"`  // Close price
	Volume string `json:"volume"` // Volume
	Time   int64  `json:"time"`   // Time
}

// =============================================================================
// Funding Rate Types
// =============================================================================

// PremiumIndex represents mark price and funding rate
type PremiumIndex struct {
	Symbol          string `json:"symbol"`          // Trading pair
	LastFundingRate string `json:"lastFundingRate"` // Last funding rate
	MarkPrice       string `json:"markPrice"`       // Current mark price
	IndexPrice      string `json:"indexPrice"`      // Index price
	NextFundingTime int64  `json:"nextFundingTime"` // Next funding time (ms)
}

// FundingRateHistory represents historical funding rate
type FundingRateHistory struct {
	Symbol      string `json:"symbol"`      // Trading pair
	FundingRate string `json:"fundingRate"` // Funding rate
	FundingTime int64  `json:"fundingTime"` // Funding time (ms)
}

// =============================================================================
// Open Interest Types
// =============================================================================

// OpenInterest represents open interest data
type OpenInterest struct {
	OpenInterest string `json:"openInterest"` // Position amount
	Symbol       string `json:"symbol"`       // Contract name
	Time         int64  `json:"time"`         // Timestamp
}

// =============================================================================
// Account Types
// =============================================================================

// AccountBalance represents account balance
type AccountBalance struct {
	Asset            string `json:"asset"`            // Asset name
	Balance          string `json:"balance"`          // Total balance
	Equity           string `json:"equity"`           // Equity
	UnrealizedProfit string `json:"unrealizedProfit"` // Unrealized PnL
	RealisedProfit   string `json:"realisedProfit"`   // Realized PnL
	AvailableMargin  string `json:"availableMargin"`  // Available margin
	UsedMargin       string `json:"usedMargin"`       // Used margin
	FreezedMargin    string `json:"freezedMargin"`    // Frozen margin
}

// =============================================================================
// Position Types
// =============================================================================

// Position represents a futures position
type Position struct {
	Symbol           string `json:"symbol"`           // Trading pair
	PositionID       string `json:"positionId"`       // Position ID
	PositionSide     string `json:"positionSide"`     // Position side: LONG/SHORT
	Isolated         bool   `json:"isolated"`         // true=isolated, false=cross
	PositionAmt      string `json:"positionAmt"`      // Position amount
	AvailableAmt     string `json:"availableAmt"`     // Available amount to close
	UnrealizedProfit string `json:"unrealizedProfit"` // Unrealized PnL
	RealisedProfit   string `json:"realisedProfit"`   // Realized PnL
	InitialMargin    string `json:"initialMargin"`    // Margin
	AvgPrice         string `json:"avgPrice"`         // Average entry price
	Leverage         int    `json:"leverage"`         // Leverage
	LiquidationPrice string `json:"liquidationPrice"` // Liquidation price
}

// =============================================================================
// Order Types
// =============================================================================

// OrderRequest represents order placement request
type OrderRequest struct {
	Symbol        string  `json:"symbol"`                  // Trading pair (required)
	Type          string  `json:"type"`                    // Order type (required)
	Side          string  `json:"side"`                    // BUY or SELL (required)
	PositionSide  string  `json:"positionSide,omitempty"`  // LONG or SHORT
	Price         float64 `json:"price,omitempty"`         // Order price
	Quantity      float64 `json:"quantity,omitempty"`      // Order quantity
	StopPrice     float64 `json:"stopPrice,omitempty"`     // Trigger price
	ClientOrderID string  `json:"clientOrderID,omitempty"` // Custom order ID (1-40 chars)
	TimeInForce   string  `json:"timeInForce,omitempty"`   // PostOnly, GTC, IOC, FOK
	ReduceOnly    bool    `json:"reduceOnly,omitempty"`    // Reduce only
}

// OrderResponse represents order placement response
type OrderResponse struct {
	Symbol        string `json:"symbol"`        // Trading pair
	OrderID       int64  `json:"orderId"`       // Order ID
	Side          string `json:"side"`          // Buy/Sell direction
	Type          string `json:"type"`          // Order type
	PositionSide  string `json:"positionSide"`  // Position side
	ClientOrderID string `json:"clientOrderID"` // Custom order ID
}

// BatchOrderRequest represents batch order request
type BatchOrderRequest struct {
	BatchOrders []OrderRequest `json:"batchOrders"` // Order list (max 5)
}

// Order represents a futures order
type Order struct {
	Symbol        string `json:"symbol"`        // Trading pair
	OrderID       int64  `json:"orderId"`       // Order ID
	Price         string `json:"price"`         // Order price
	OrigQty       string `json:"origQty"`       // Original quantity
	ExecutedQty   string `json:"executedQty"`   // Executed quantity
	AvgPrice      string `json:"avgPrice"`      // Average price
	CumQuote      string `json:"cumQuote"`      // Cumulative quote
	StopPrice     string `json:"stopPrice"`     // Trigger price
	Type          string `json:"type"`          // Order type
	Side          string `json:"side"`          // Buy/Sell
	PositionSide  string `json:"positionSide"`  // Position side
	Status        string `json:"status"`        // Order status
	Profit        string `json:"profit"`        // PnL
	Commission    string `json:"commission"`    // Fee
	Time          int64  `json:"time"`          // Order time
	UpdateTime    int64  `json:"updateTime"`    // Update time
	ClientOrderID string `json:"clientOrderId"` // Custom order ID
	TimeInForce   string `json:"timeInForce"`   // Time in force
	ReduceOnly    bool   `json:"reduceOnly"`    // Reduce only
	WorkingType   string `json:"workingType"`   // Working type
}

// CancelResponse represents order cancellation response
type CancelResponse struct {
	Symbol        string `json:"symbol"`        // Trading pair
	OrderID       int64  `json:"orderId"`       // Order ID
	Side          string `json:"side"`          // Buy/Sell
	PositionSide  string `json:"positionSide"`  // Position side
	Status        string `json:"status"`        // Order status
	ClientOrderID string `json:"clientOrderId"` // Custom order ID
}

// =============================================================================
// Fill/Trade History Types
// =============================================================================

// Fill represents a trade fill
type Fill struct {
	Symbol          string `json:"symbol"`          // Symbol
	OrderID         int64  `json:"orderId"`         // Order ID
	TradeID         int64  `json:"tradeId"`         // Trade ID
	Price           string `json:"price"`           // Fill price
	Qty             string `json:"qty"`             // Fill quantity
	QuoteQty        string `json:"quoteQty"`        // Quote quantity
	Commission      string `json:"commission"`      // Commission
	CommissionAsset string `json:"commissionAsset"` // Commission asset
	RealizedPnl     string `json:"realizedPnl"`     // Realized PnL
	Side            string `json:"side"`            // Side
	PositionSide    string `json:"positionSide"`    // Position side
	Buyer           bool   `json:"buyer"`           // Is buyer
	Maker           bool   `json:"maker"`           // Is maker
	Time            int64  `json:"time"`            // Trade time
}

// =============================================================================
// Leverage Types
// =============================================================================

// Leverage represents leverage setting
type Leverage struct {
	LongLeverage  int64 `json:"longLeverage"`  // Long position leverage
	ShortLeverage int64 `json:"shortLeverage"` // Short position leverage
}

// LeverageChangeRequest represents leverage change request
type LeverageChangeRequest struct {
	Symbol   string `json:"symbol"`   // Trading pair
	Side     string `json:"side"`     // LONG or SHORT
	Leverage int    `json:"leverage"` // Leverage value
}

// MarginType represents margin type response
type MarginType struct {
	MarginType string `json:"marginType"` // ISOLATED or CROSSED
}

// =============================================================================
// Commission Types
// =============================================================================

// CommissionRate represents trading fee rates
type CommissionRate struct {
	Symbol              string `json:"symbol"`              // Trading pair
	TakerCommissionRate string `json:"takerCommissionRate"` // Taker fee rate
	MakerCommissionRate string `json:"makerCommissionRate"` // Maker fee rate
}

// =============================================================================
// Income Types
// =============================================================================

// Income represents income/loss record
type Income struct {
	Symbol     string `json:"symbol"`     // Trading pair
	IncomeType string `json:"incomeType"` // Income type
	Income     string `json:"income"`     // Income amount
	Asset      string `json:"asset"`      // Asset
	Info       string `json:"info"`       // Information
	Time       int64  `json:"time"`       // Timestamp
	TranID     int64  `json:"tranId"`     // Transaction ID
}

// =============================================================================
// Listen Key Types
// =============================================================================

// ListenKey represents user data stream listen key
type ListenKey struct {
	ListenKey string `json:"listenKey"` // Listen key
}

// =============================================================================
// WebSocket Types
// =============================================================================

// WSSubscribeRequest represents subscribe/unsubscribe request
type WSSubscribeRequest struct {
	ID       string `json:"id"`       // Unique ID
	ReqType  string `json:"reqType"`  // "sub" or "unsub"
	DataType string `json:"dataType"` // Subscription type, e.g., "BTC-USDT@depth20"
}

// WSMessage represents a generic WebSocket message
type WSMessage struct {
	Code     int             `json:"code,omitempty"`     // Response code
	DataType string          `json:"dataType,omitempty"` // Data type
	Data     json.RawMessage `json:"data,omitempty"`     // Message data
	S        string          `json:"s,omitempty"`        // Symbol (for some messages)
}

// =============================================================================
// WebSocket Market Data Types
// =============================================================================

// WSDepthData represents orderbook depth data from WebSocket
type WSDepthData struct {
	Bids [][]string `json:"bids"` // Bid levels [[price, quantity], ...]
	Asks [][]string `json:"asks"` // Ask levels [[price, quantity], ...]
	T    int64      `json:"T"`    // Timestamp (milliseconds)
}

// WSTradeData represents trade data from WebSocket
type WSTradeData struct {
	Trades []WSTradeItem `json:"trades"` // Trade list
}

// WSTradeItem represents a single trade from WebSocket
type WSTradeItem struct {
	Time      int64  `json:"time"`      // Trade time
	MakerSide string `json:"makerSide"` // Bid/Ask
	Price     string `json:"price"`     // Trade price
	Volume    string `json:"volume"`    // Trade quantity
}

// WSKlineData represents kline data from WebSocket
type WSKlineData struct {
	C string `json:"c"` // Close price
	H string `json:"h"` // High price
	L string `json:"l"` // Low price
	O string `json:"o"` // Open price
	V string `json:"v"` // Volume
	T int64  `json:"T"` // Time
}

// =============================================================================
// WebSocket User Data Types
// =============================================================================

// WSUserDataEvent represents base event from user data stream
type WSUserDataEvent struct {
	E string `json:"e"` // Event type
	T int64  `json:"E"` // Event time
}

// WSAccountUpdate represents account update event
type WSAccountUpdate struct {
	E string              `json:"e"` // Event type: ACCOUNT_UPDATE
	T int64               `json:"E"` // Event time
	A WSAccountUpdateData `json:"a"` // Account update data
}

// WSAccountUpdateData represents account update data
type WSAccountUpdateData struct {
	B []WSBalanceData  `json:"B"` // Balance list
	P []WSPositionData `json:"P"` // Position list
}

// WSBalanceData represents balance data in WebSocket
type WSBalanceData struct {
	A  string `json:"a"`  // Asset
	WB string `json:"wb"` // Wallet balance
	CW string `json:"cw"` // Cross wallet balance
	BC string `json:"bc"` // Balance change
}

// WSPositionData represents position data in WebSocket
type WSPositionData struct {
	S  string `json:"s"`  // Symbol
	PA string `json:"pa"` // Position amount
	EP string `json:"ep"` // Entry price
	UP string `json:"up"` // Unrealized PnL
	MT string `json:"mt"` // Margin type
	IW string `json:"iw"` // Isolated wallet (if isolated)
	PS string `json:"ps"` // Position side
}

// WSOrderTradeUpdate represents order/trade update event
type WSOrderTradeUpdate struct {
	E string      `json:"e"` // Event type: ORDER_TRADE_UPDATE
	T int64       `json:"E"` // Event time
	O WSOrderData `json:"o"` // Order info
}

// WSOrderData represents order data in WebSocket
type WSOrderData struct {
	S  string `json:"s"`  // Symbol
	C  string `json:"c"`  // Client order ID
	I  int64  `json:"i"`  // Order ID
	SD string `json:"S"`  // Side: BUY/SELL
	OT string `json:"o"`  // Order type
	Q  string `json:"q"`  // Order quantity
	P  string `json:"p"`  // Order price
	AP string `json:"ap"` // Average price
	X  string `json:"x"`  // Execution type: NEW, TRADE, CANCELED
	XS string `json:"X"`  // Order status
	N  string `json:"N"`  // Commission asset
	NC string `json:"n"`  // Commission
	T  int64  `json:"T"`  // Trade time
	WT string `json:"wt"` // Trigger price type
	PS string `json:"ps"` // Position side
	RP string `json:"rp"` // Realized PnL
	Z  string `json:"z"`  // Cumulative filled quantity
}

// WSListenKeyExpired represents listen key expired event
type WSListenKeyExpired struct {
	E         string `json:"e"`         // Event type: listenKeyExpired
	T         int64  `json:"E"`         // Event time
	ListenKey string `json:"listenKey"` // Expired listen key
}

// =============================================================================
// Wallet / Asset Info Types
// =============================================================================

// AssetConfig represents asset deposit/withdraw configuration
type AssetConfig struct {
	Coin        string          `json:"coin"`        // Coin name
	Name        string          `json:"name"`        // Coin full name
	NetworkList []NetworkConfig `json:"networkList"` // Network list
}

// NetworkConfig represents network configuration for deposit/withdraw
type NetworkConfig struct {
	Network        string `json:"network"`        // Network name
	WithdrawEnable bool   `json:"withdrawEnable"` // Withdraw enabled
	DepositEnable  bool   `json:"depositEnable"`  // Deposit enabled
	WithdrawFee    string `json:"withdrawFee"`    // Withdraw fee
	WithdrawMin    string `json:"withdrawMin"`    // Minimum withdraw
	WithdrawMax    string `json:"withdrawMax"`    // Maximum withdraw
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

// GenerateClientOrderID generates a unique client order ID
func GenerateClientOrderID() string {
	return fmt.Sprintf("bx%d", time.Now().UnixNano())
}

// FormatSymbol formats symbol for BingX API (BASE-QUOTE format)
func FormatSymbol(base, quote string) string {
	return fmt.Sprintf("%s-%s", base, quote)
}

// ParseSymbol parses BingX symbol into base and quote assets
func ParseSymbol(symbol string) (base, quote string) {
	for i, ch := range symbol {
		if ch == '-' {
			return symbol[:i], symbol[i+1:]
		}
	}
	return symbol, ""
}
