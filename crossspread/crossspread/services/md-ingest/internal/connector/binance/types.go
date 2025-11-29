package binance

import (
	"encoding/json"
	"time"
)

// =============================================================================
// REST API Response Types
// =============================================================================

// ExchangeInfoResponse represents the response from GET /fapi/v1/exchangeInfo
type ExchangeInfoResponse struct {
	Timezone        string        `json:"timezone"`
	ServerTime      int64         `json:"serverTime"`
	RateLimits      []RateLimit   `json:"rateLimits"`
	ExchangeFilters []interface{} `json:"exchangeFilters"`
	Assets          []Asset       `json:"assets"`
	Symbols         []SymbolInfo  `json:"symbols"`
}

type RateLimit struct {
	RateLimitType string `json:"rateLimitType"`
	Interval      string `json:"interval"`
	IntervalNum   int    `json:"intervalNum"`
	Limit         int    `json:"limit"`
}

type Asset struct {
	Asset             string `json:"asset"`
	MarginAvailable   bool   `json:"marginAvailable"`
	AutoAssetExchange string `json:"autoAssetExchange"`
}

type SymbolInfo struct {
	Symbol                string   `json:"symbol"`
	Pair                  string   `json:"pair"`
	ContractType          string   `json:"contractType"`
	DeliveryDate          int64    `json:"deliveryDate"`
	OnboardDate           int64    `json:"onboardDate"`
	Status                string   `json:"status"`
	MaintMarginPercent    string   `json:"maintMarginPercent"`
	RequiredMarginPercent string   `json:"requiredMarginPercent"`
	BaseAsset             string   `json:"baseAsset"`
	QuoteAsset            string   `json:"quoteAsset"`
	MarginAsset           string   `json:"marginAsset"`
	PricePrecision        int      `json:"pricePrecision"`
	QuantityPrecision     int      `json:"quantityPrecision"`
	BaseAssetPrecision    int      `json:"baseAssetPrecision"`
	QuotePrecision        int      `json:"quotePrecision"`
	UnderlyingType        string   `json:"underlyingType"`
	UnderlyingSubType     []string `json:"underlyingSubType"`
	SettlePlan            int      `json:"settlePlan"`
	TriggerProtect        string   `json:"triggerProtect"`
	LiquidationFee        string   `json:"liquidationFee"`
	MarketTakeBound       string   `json:"marketTakeBound"`
	Filters               []Filter `json:"filters"`
	OrderTypes            []string `json:"orderTypes"`
	TimeInForce           []string `json:"timeInForce"`
}

type Filter struct {
	FilterType        string `json:"filterType"`
	MaxPrice          string `json:"maxPrice,omitempty"`
	MinPrice          string `json:"minPrice,omitempty"`
	TickSize          string `json:"tickSize,omitempty"`
	MaxQty            string `json:"maxQty,omitempty"`
	MinQty            string `json:"minQty,omitempty"`
	StepSize          string `json:"stepSize,omitempty"`
	Limit             int    `json:"limit,omitempty"`
	Notional          string `json:"notional,omitempty"`
	MultiplierUp      string `json:"multiplierUp,omitempty"`
	MultiplierDown    string `json:"multiplierDown,omitempty"`
	MultiplierDecimal string `json:"multiplierDecimal,omitempty"`
}

// Ticker24hr represents the response from GET /fapi/v1/ticker/24hr
type Ticker24hr struct {
	Symbol             string `json:"symbol"`
	PriceChange        string `json:"priceChange"`
	PriceChangePercent string `json:"priceChangePercent"`
	WeightedAvgPrice   string `json:"weightedAvgPrice"`
	LastPrice          string `json:"lastPrice"`
	LastQty            string `json:"lastQty"`
	OpenPrice          string `json:"openPrice"`
	HighPrice          string `json:"highPrice"`
	LowPrice           string `json:"lowPrice"`
	Volume             string `json:"volume"`
	QuoteVolume        string `json:"quoteVolume"`
	OpenTime           int64  `json:"openTime"`
	CloseTime          int64  `json:"closeTime"`
	FirstId            int64  `json:"firstId"`
	LastId             int64  `json:"lastId"`
	Count              int64  `json:"count"`
}

// FundingRateInfo represents the response from GET /fapi/v1/fundingRate
type FundingRateInfo struct {
	Symbol      string `json:"symbol"`
	FundingRate string `json:"fundingRate"`
	FundingTime int64  `json:"fundingTime"`
	MarkPrice   string `json:"markPrice,omitempty"`
}

// PremiumIndex represents the response from GET /fapi/v1/premiumIndex
type PremiumIndex struct {
	Symbol               string `json:"symbol"`
	MarkPrice            string `json:"markPrice"`
	IndexPrice           string `json:"indexPrice"`
	EstimatedSettlePrice string `json:"estimatedSettlePrice"`
	LastFundingRate      string `json:"lastFundingRate"`
	NextFundingTime      int64  `json:"nextFundingTime"`
	InterestRate         string `json:"interestRate"`
	Time                 int64  `json:"time"`
}

// Kline represents a single candlestick from GET /fapi/v1/klines
type Kline struct {
	OpenTime                 int64
	Open                     string
	High                     string
	Low                      string
	Close                    string
	Volume                   string
	CloseTime                int64
	QuoteAssetVolume         string
	NumberOfTrades           int64
	TakerBuyBaseAssetVolume  string
	TakerBuyQuoteAssetVolume string
}

// OpenInterest represents the response from GET /fapi/v1/openInterest
type OpenInterest struct {
	Symbol       string `json:"symbol"`
	OpenInterest string `json:"openInterest"`
	Time         int64  `json:"time"`
}

// DepthResponse represents the response from GET /fapi/v1/depth
type DepthResponse struct {
	LastUpdateId int64      `json:"lastUpdateId"`
	E            int64      `json:"E"` // Message output time
	T            int64      `json:"T"` // Transaction time
	Bids         [][]string `json:"bids"`
	Asks         [][]string `json:"asks"`
}

// =============================================================================
// Authenticated REST API Response Types
// =============================================================================

// CoinInfo represents deposit/withdrawal status from GET /sapi/v1/capital/config/getall
type CoinInfo struct {
	Coin              string        `json:"coin"`
	DepositAllEnable  bool          `json:"depositAllEnable"`
	Free              string        `json:"free"`
	Freeze            string        `json:"freeze"`
	Ipoable           string        `json:"ipoable"`
	Ipoing            string        `json:"ipoing"`
	IsLegalMoney      bool          `json:"isLegalMoney"`
	Locked            string        `json:"locked"`
	Name              string        `json:"name"`
	NetworkList       []NetworkInfo `json:"networkList"`
	Storage           string        `json:"storage"`
	Trading           bool          `json:"trading"`
	WithdrawAllEnable bool          `json:"withdrawAllEnable"`
	Withdrawing       string        `json:"withdrawing"`
}

type NetworkInfo struct {
	AddressRegex            string `json:"addressRegex"`
	Coin                    string `json:"coin"`
	DepositDesc             string `json:"depositDesc,omitempty"`
	DepositEnable           bool   `json:"depositEnable"`
	IsDefault               bool   `json:"isDefault"`
	MemoRegex               string `json:"memoRegex"`
	MinConfirm              int    `json:"minConfirm"`
	Name                    string `json:"name"`
	Network                 string `json:"network"`
	ResetAddressStatus      bool   `json:"resetAddressStatus"`
	SpecialTips             string `json:"specialTips,omitempty"`
	UnLockConfirm           int    `json:"unLockConfirm"`
	WithdrawDesc            string `json:"withdrawDesc,omitempty"`
	WithdrawEnable          bool   `json:"withdrawEnable"`
	WithdrawFee             string `json:"withdrawFee"`
	WithdrawIntegerMultiple string `json:"withdrawIntegerMultiple"`
	WithdrawMax             string `json:"withdrawMax"`
	WithdrawMin             string `json:"withdrawMin"`
	SameAddress             bool   `json:"sameAddress"`
	EstimatedArrivalTime    int    `json:"estimatedArrivalTime"`
	Busy                    bool   `json:"busy"`
}

// TradeFee represents the response from GET /sapi/v1/asset/tradeFee
type TradeFee struct {
	Symbol          string `json:"symbol"`
	MakerCommission string `json:"makerCommission"`
	TakerCommission string `json:"takerCommission"`
}

// FuturesAccountInfo represents the response from GET /fapi/v2/account
type FuturesAccountInfo struct {
	FeeTier                     int            `json:"feeTier"`
	CanTrade                    bool           `json:"canTrade"`
	CanDeposit                  bool           `json:"canDeposit"`
	CanWithdraw                 bool           `json:"canWithdraw"`
	UpdateTime                  int64          `json:"updateTime"`
	MultiAssetsMargin           bool           `json:"multiAssetsMargin"`
	TotalInitialMargin          string         `json:"totalInitialMargin"`
	TotalMaintMargin            string         `json:"totalMaintMargin"`
	TotalWalletBalance          string         `json:"totalWalletBalance"`
	TotalUnrealizedProfit       string         `json:"totalUnrealizedProfit"`
	TotalMarginBalance          string         `json:"totalMarginBalance"`
	TotalPositionInitialMargin  string         `json:"totalPositionInitialMargin"`
	TotalOpenOrderInitialMargin string         `json:"totalOpenOrderInitialMargin"`
	TotalCrossWalletBalance     string         `json:"totalCrossWalletBalance"`
	TotalCrossUnPnl             string         `json:"totalCrossUnPnl"`
	AvailableBalance            string         `json:"availableBalance"`
	MaxWithdrawAmount           string         `json:"maxWithdrawAmount"`
	Assets                      []AccountAsset `json:"assets"`
	Positions                   []PositionInfo `json:"positions"`
}

type AccountAsset struct {
	Asset                  string `json:"asset"`
	WalletBalance          string `json:"walletBalance"`
	UnrealizedProfit       string `json:"unrealizedProfit"`
	MarginBalance          string `json:"marginBalance"`
	MaintMargin            string `json:"maintMargin"`
	InitialMargin          string `json:"initialMargin"`
	PositionInitialMargin  string `json:"positionInitialMargin"`
	OpenOrderInitialMargin string `json:"openOrderInitialMargin"`
	CrossWalletBalance     string `json:"crossWalletBalance"`
	CrossUnPnl             string `json:"crossUnPnl"`
	AvailableBalance       string `json:"availableBalance"`
	MaxWithdrawAmount      string `json:"maxWithdrawAmount"`
	MarginAvailable        bool   `json:"marginAvailable"`
	UpdateTime             int64  `json:"updateTime"`
}

type PositionInfo struct {
	Symbol                 string `json:"symbol"`
	InitialMargin          string `json:"initialMargin"`
	MaintMargin            string `json:"maintMargin"`
	UnrealizedProfit       string `json:"unrealizedProfit"`
	PositionInitialMargin  string `json:"positionInitialMargin"`
	OpenOrderInitialMargin string `json:"openOrderInitialMargin"`
	Leverage               string `json:"leverage"`
	Isolated               bool   `json:"isolated"`
	EntryPrice             string `json:"entryPrice"`
	MaxNotional            string `json:"maxNotional"`
	BidNotional            string `json:"bidNotional"`
	AskNotional            string `json:"askNotional"`
	PositionSide           string `json:"positionSide"`
	PositionAmt            string `json:"positionAmt"`
	UpdateTime             int64  `json:"updateTime"`
}

// PositionRisk represents the response from GET /fapi/v2/positionRisk
type PositionRisk struct {
	Symbol           string `json:"symbol"`
	PositionAmt      string `json:"positionAmt"`
	EntryPrice       string `json:"entryPrice"`
	MarkPrice        string `json:"markPrice"`
	UnRealizedProfit string `json:"unRealizedProfit"`
	LiquidationPrice string `json:"liquidationPrice"`
	Leverage         string `json:"leverage"`
	MaxNotionalValue string `json:"maxNotionalValue"`
	MarginType       string `json:"marginType"`
	IsolatedMargin   string `json:"isolatedMargin"`
	IsAutoAddMargin  string `json:"isAutoAddMargin"`
	PositionSide     string `json:"positionSide"`
	Notional         string `json:"notional"`
	IsolatedWallet   string `json:"isolatedWallet"`
	UpdateTime       int64  `json:"updateTime"`
}

// =============================================================================
// WebSocket Stream Types
// =============================================================================

// WSTradeEvent represents real-time trade data from @trade stream
type WSTradeEvent struct {
	EventType     string `json:"e"` // Event type: "trade"
	EventTime     int64  `json:"E"` // Event time
	Symbol        string `json:"s"` // Symbol
	TradeId       int64  `json:"t"` // Trade ID
	Price         string `json:"p"` // Price
	Quantity      string `json:"q"` // Quantity
	BuyerOrderId  int64  `json:"b"` // Buyer order ID
	SellerOrderId int64  `json:"a"` // Seller order ID
	TradeTime     int64  `json:"T"` // Trade time
	IsBuyerMaker  bool   `json:"m"` // Is the buyer the market maker?
}

// WSDepthEvent represents orderbook depth updates from @depth stream
type WSDepthEvent struct {
	EventType     string     `json:"e"`  // Event type: "depthUpdate"
	EventTime     int64      `json:"E"`  // Event time
	TransactTime  int64      `json:"T"`  // Transaction time
	Symbol        string     `json:"s"`  // Symbol
	FirstUpdateId int64      `json:"U"`  // First update ID in event
	FinalUpdateId int64      `json:"u"`  // Final update ID in event
	PrevFinalId   int64      `json:"pu"` // Previous final update ID
	Bids          [][]string `json:"b"`  // Bids to be updated
	Asks          [][]string `json:"a"`  // Asks to be updated
}

// WSMarkPriceEvent represents mark price updates from @markPrice stream
type WSMarkPriceEvent struct {
	EventType       string `json:"e"` // Event type: "markPriceUpdate"
	EventTime       int64  `json:"E"` // Event time
	Symbol          string `json:"s"` // Symbol
	MarkPrice       string `json:"p"` // Mark price
	IndexPrice      string `json:"i"` // Index price
	EstSettlePrice  string `json:"P"` // Estimated Settle Price
	FundingRate     string `json:"r"` // Funding rate
	NextFundingTime int64  `json:"T"` // Next funding time
}

// WSKlineEvent represents kline/candlestick updates from @kline stream
type WSKlineEvent struct {
	EventType string      `json:"e"` // Event type: "kline"
	EventTime int64       `json:"E"` // Event time
	Symbol    string      `json:"s"` // Symbol
	Kline     WSKlineData `json:"k"` // Kline data
}

type WSKlineData struct {
	StartTime           int64  `json:"t"` // Kline start time
	CloseTime           int64  `json:"T"` // Kline close time
	Symbol              string `json:"s"` // Symbol
	Interval            string `json:"i"` // Interval
	FirstTradeId        int64  `json:"f"` // First trade ID
	LastTradeId         int64  `json:"L"` // Last trade ID
	Open                string `json:"o"` // Open price
	Close               string `json:"c"` // Close price
	High                string `json:"h"` // High price
	Low                 string `json:"l"` // Low price
	Volume              string `json:"v"` // Base asset volume
	NumberOfTrades      int64  `json:"n"` // Number of trades
	IsClosed            bool   `json:"x"` // Is this kline closed?
	QuoteAssetVolume    string `json:"q"` // Quote asset volume
	TakerBuyBaseVolume  string `json:"V"` // Taker buy base asset volume
	TakerBuyQuoteVolume string `json:"Q"` // Taker buy quote asset volume
}

// WSMiniTickerEvent represents mini ticker updates from @miniTicker stream
type WSMiniTickerEvent struct {
	EventType   string `json:"e"` // Event type: "24hrMiniTicker"
	EventTime   int64  `json:"E"` // Event time
	Symbol      string `json:"s"` // Symbol
	Close       string `json:"c"` // Close price
	Open        string `json:"o"` // Open price
	High        string `json:"h"` // High price
	Low         string `json:"l"` // Low price
	Volume      string `json:"v"` // Total traded base asset volume
	QuoteVolume string `json:"q"` // Total traded quote asset volume
}

// =============================================================================
// User Data Stream Types
// =============================================================================

// UserDataEvent is the base structure for all user data events
type UserDataEvent struct {
	EventType string `json:"e"` // Event type
	EventTime int64  `json:"E"` // Event time
}

// AccountUpdateEvent represents ACCOUNT_UPDATE events
type AccountUpdateEvent struct {
	EventType     string            `json:"e"` // "ACCOUNT_UPDATE"
	EventTime     int64             `json:"E"` // Event time
	TransactTime  int64             `json:"T"` // Transaction time
	AccountUpdate AccountUpdateData `json:"a"` // Account update data
}

type AccountUpdateData struct {
	Reason    string           `json:"m"` // Reason type: DEPOSIT, WITHDRAW, ORDER, FUNDING_FEE, etc.
	Balances  []BalanceUpdate  `json:"B"` // Balance updates
	Positions []PositionUpdate `json:"P"` // Position updates
}

type BalanceUpdate struct {
	Asset              string `json:"a"`  // Asset
	WalletBalance      string `json:"wb"` // Wallet balance
	CrossWalletBalance string `json:"cw"` // Cross wallet balance
	BalanceChange      string `json:"bc"` // Balance change except PnL and commission
}

type PositionUpdate struct {
	Symbol              string `json:"s"`   // Symbol
	PositionAmt         string `json:"pa"`  // Position amount
	EntryPrice          string `json:"ep"`  // Entry price
	BreakEvenPrice      string `json:"bep"` // Break-even price
	AccumulatedRealized string `json:"cr"`  // Accumulated realized PnL
	UnrealizedPnL       string `json:"up"`  // Unrealized PnL
	MarginType          string `json:"mt"`  // Margin type
	IsolatedWallet      string `json:"iw"`  // Isolated wallet
	PositionSide        string `json:"ps"`  // Position side
}

// OrderUpdateEvent represents ORDER_TRADE_UPDATE events
type OrderUpdateEvent struct {
	EventType    string          `json:"e"` // "ORDER_TRADE_UPDATE"
	EventTime    int64           `json:"E"` // Event time
	TransactTime int64           `json:"T"` // Transaction time
	Order        OrderUpdateData `json:"o"` // Order update data
}

type OrderUpdateData struct {
	Symbol              string `json:"s"`   // Symbol
	ClientOrderId       string `json:"c"`   // Client order ID
	Side                string `json:"S"`   // Side (BUY/SELL)
	OrderType           string `json:"o"`   // Order type
	TimeInForce         string `json:"f"`   // Time in force
	OriginalQty         string `json:"q"`   // Original quantity
	OriginalPrice       string `json:"p"`   // Original price
	AveragePrice        string `json:"ap"`  // Average price
	StopPrice           string `json:"sp"`  // Stop price
	ExecutionType       string `json:"x"`   // Execution type (NEW, TRADE, CANCELED, etc.)
	OrderStatus         string `json:"X"`   // Order status
	OrderId             int64  `json:"i"`   // Order ID
	LastFilledQty       string `json:"l"`   // Order last filled quantity
	CumulativeFilledQty string `json:"z"`   // Order filled accumulated quantity
	LastFilledPrice     string `json:"L"`   // Last filled price
	CommissionAsset     string `json:"N"`   // Commission asset
	Commission          string `json:"n"`   // Commission
	TradeTime           int64  `json:"T"`   // Trade time
	TradeId             int64  `json:"t"`   // Trade ID
	BidsNotional        string `json:"b"`   // Bids notional
	AsksNotional        string `json:"a"`   // Asks notional
	IsMaker             bool   `json:"m"`   // Is this trade the maker side?
	IsReduceOnly        bool   `json:"R"`   // Is this reduce only
	WorkingType         string `json:"wt"`  // Working type
	OriginalOrderType   string `json:"ot"`  // Original order type
	PositionSide        string `json:"ps"`  // Position side
	IsClosePosition     bool   `json:"cp"`  // If close-all
	ActivationPrice     string `json:"AP"`  // Activation price
	CallbackRate        string `json:"cr"`  // Callback rate
	PriceProtect        bool   `json:"pP"`  // Price protect
	RealizedProfit      string `json:"rp"`  // Realized profit
	STPMode             string `json:"V"`   // STP mode
	PriceMatch          string `json:"pm"`  // Price match mode
	GTDTime             int64  `json:"gtd"` // TIF GTD order auto cancel time
}

// MarginCallEvent represents MARGIN_CALL events
type MarginCallEvent struct {
	EventType       string       `json:"e"`  // "MARGIN_CALL"
	EventTime       int64        `json:"E"`  // Event time
	CrossWalletBal  string       `json:"cw"` // Cross wallet balance
	MarginPositions []MarginCall `json:"p"`  // Positions in margin call
}

type MarginCall struct {
	Symbol         string `json:"s"`  // Symbol
	PositionSide   string `json:"ps"` // Position side
	PositionAmt    string `json:"pa"` // Position amount
	MarginType     string `json:"mt"` // Margin type
	IsolatedWallet string `json:"iw"` // Isolated wallet
	MarkPrice      string `json:"mp"` // Mark price
	UnrealizedPnL  string `json:"up"` // Unrealized PnL
	MaintMargin    string `json:"mm"` // Maintenance margin required
}

// =============================================================================
// WebSocket API Types (for trading)
// =============================================================================

// WSAPIRequest represents a WebSocket API request
type WSAPIRequest struct {
	ID     string                 `json:"id"`
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

// WSAPIResponse represents a WebSocket API response
type WSAPIResponse struct {
	ID         string           `json:"id"`
	Status     int              `json:"status"`
	Result     json.RawMessage  `json:"result,omitempty"`
	Error      *WSAPIError      `json:"error,omitempty"`
	RateLimits []WSAPIRateLimit `json:"rateLimits,omitempty"`
}

type WSAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
}

type WSAPIRateLimit struct {
	RateLimitType string `json:"rateLimitType"`
	Interval      string `json:"interval"`
	IntervalNum   int    `json:"intervalNum"`
	Limit         int    `json:"limit"`
	Count         int    `json:"count"`
}

// OrderResult represents the result of an order operation
type OrderResult struct {
	OrderId                 int64  `json:"orderId"`
	Symbol                  string `json:"symbol"`
	Status                  string `json:"status"`
	ClientOrderId           string `json:"clientOrderId"`
	Price                   string `json:"price"`
	AvgPrice                string `json:"avgPrice"`
	OrigQty                 string `json:"origQty"`
	ExecutedQty             string `json:"executedQty"`
	CumQty                  string `json:"cumQty"`
	CumQuote                string `json:"cumQuote"`
	TimeInForce             string `json:"timeInForce"`
	Type                    string `json:"type"`
	ReduceOnly              bool   `json:"reduceOnly"`
	ClosePosition           bool   `json:"closePosition"`
	Side                    string `json:"side"`
	PositionSide            string `json:"positionSide"`
	StopPrice               string `json:"stopPrice"`
	WorkingType             string `json:"workingType"`
	PriceProtect            bool   `json:"priceProtect"`
	OrigType                string `json:"origType"`
	PriceMatch              string `json:"priceMatch"`
	SelfTradePreventionMode string `json:"selfTradePreventionMode"`
	GoodTillDate            int64  `json:"goodTillDate"`
	UpdateTime              int64  `json:"updateTime"`
}

// =============================================================================
// Internal Types for Spread Service
// =============================================================================

// TokenData aggregates all data for a single token from Binance
type TokenData struct {
	Symbol       string
	BaseAsset    string
	QuoteAsset   string
	ContractType string
	Status       string

	// Price & Volume
	LastPrice         float64
	MarkPrice         float64
	IndexPrice        float64
	Volume24h         float64
	QuoteVolume24h    float64
	PriceChange24h    float64
	PriceChangePct24h float64
	HighPrice24h      float64
	LowPrice24h       float64

	// Funding
	FundingRate     float64
	NextFundingTime time.Time

	// Open Interest
	OpenInterest float64

	// Trading Rules
	TickSize    float64
	StepSize    float64
	MinNotional float64
	MaxLeverage int

	// Fees (requires auth)
	MakerFee float64
	TakerFee float64

	// Deposit/Withdraw Status (requires auth)
	DepositEnabled  bool
	WithdrawEnabled bool
	Networks        []NetworkStatus

	// Orderbook
	BestBid    float64
	BestBidQty float64
	BestAsk    float64
	BestAskQty float64

	UpdatedAt time.Time
}

type NetworkStatus struct {
	Network         string
	DepositEnabled  bool
	WithdrawEnabled bool
	WithdrawFee     float64
	MinWithdraw     float64
	Busy            bool
}

// HistoricalPrice represents a single historical price point
type HistoricalPrice struct {
	Timestamp time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
}
