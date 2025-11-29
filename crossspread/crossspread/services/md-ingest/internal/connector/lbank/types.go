package lbank

import (
	"encoding/json"
	"time"
)

// API Base URLs
const (
	// Spot API
	SpotRestBaseURL = "https://api.lbkex.com"
	SpotWsBaseURL   = "wss://www.lbkex.net/ws/V2/"

	// Contract (Perpetual Futures) API
	ContractRestBaseURL = "https://lbkperp.lbank.com"
	ContractWsBaseURL   = "wss://lbkperpws.lbank.com/ws"

	// API Paths
	ContractPublicPath  = "/cfd/openApi/v1/pub"
	ContractPrivatePath = "/cfd/openApi/v1/prv"

	// Product Groups
	ProductGroupSwapU = "SwapU" // USDT-margined perpetuals

	// Signature Methods
	SignatureMethodRSA        = "RSA"
	SignatureMethodHmacSHA256 = "HmacSHA256"
)

// WebSocket Message Actions
const (
	ActionSubscribe   = "subscribe"
	ActionUnsubscribe = "unsubscribe"
	ActionRequest     = "request"
	ActionPing        = "ping"
	ActionPong        = "pong"
)

// Subscription Channels (Spot)
const (
	ChannelDepth = "depth"
	ChannelTrade = "trade"
	ChannelTick  = "tick"
	ChannelKbar  = "kbar"
)

// Kline Types
const (
	Kline1Min   = "minute1"
	Kline5Min   = "minute5"
	Kline15Min  = "minute15"
	Kline30Min  = "minute30"
	Kline1Hour  = "hour1"
	Kline4Hour  = "hour4"
	Kline8Hour  = "hour8"
	Kline12Hour = "hour12"
	Kline1Day   = "day1"
	Kline1Week  = "week1"
	Kline1Month = "month1"
)

// Order Side
const (
	SideBuy  = "BUY"
	SideSell = "SELL"
)

// Order Type
const (
	OrderTypeLimit  = "LIMIT"
	OrderTypeMarket = "MARKET"
)

// Order Status
const (
	OrderStatusPending   = 0  // On trading
	OrderStatusPartial   = 1  // Partially filled
	OrderStatusFilled    = 2  // Fully filled
	OrderStatusPartCanc  = 3  // Partially filled and cancelled
	OrderStatusCanceling = 4  // Cancelling
	OrderStatusCancelled = -1 // Cancelled
)

// Error codes
const (
	ErrSuccess               = 0
	ErrSystemException       = -99
	ErrNoRecordFound         = 2
	ErrRecordExists          = 3
	ErrContractNotExist      = 8
	ErrUserNotExist          = 9
	ErrOrderNotExist         = 24
	ErrInsufficientPosition  = 31
	ErrPositionLimit         = 32
	ErrInsufficientBalance   = 35
	ErrInsufficientFunds     = 36
	ErrInvalidQuantity       = 37
	ErrIllegalQuantity       = 44
	ErrIllegalPrice          = 48
	ErrPriceExceedsUpper     = 49
	ErrPriceExceedsLower     = 50
	ErrNoTradeAuth           = 51
	ErrInsufficientMargin    = 100
	ErrInvalidAPIKey         = 176
	ErrAPIKeyExpired         = 177
	ErrAPIKeyLimitExceeded   = 178
	ErrKeyIsNull             = 179
	ErrExceededQueryRate     = 183
	ErrOrderLimitExceeded    = 184
	ErrExceededMaxVolume     = 193
	ErrBelowMinVolume        = 194
	ErrAuthSyncFailed        = 10001
	ErrAuthParamsLost        = 10002
	ErrSignatureVerifyFailed = 10003
	ErrRequestTimeout        = 10004
	ErrIllegalParameter      = 10005
	ErrAuthFailed            = 10007
	ErrKeyNotExist           = 10008
	ErrNoPermission          = 10009
	ErrInvalidSignature      = 10010
	ErrRepeatRequest         = 10011
	ErrTooFrequentRequest    = 10012
)

// ErrorMessages maps error codes to descriptions
var ErrorMessages = map[int]string{
	ErrSuccess:               "Success",
	ErrSystemException:       "System exception, please try again later",
	ErrNoRecordFound:         "No record found",
	ErrRecordExists:          "Record already exists",
	ErrContractNotExist:      "Contract product does not exist",
	ErrUserNotExist:          "User does not exist",
	ErrOrderNotExist:         "Order does not exist",
	ErrInsufficientPosition:  "Insufficient positions, cannot close position",
	ErrPositionLimit:         "Position limit",
	ErrInsufficientBalance:   "Insufficient balance",
	ErrInsufficientFunds:     "Insufficient funds",
	ErrInvalidQuantity:       "Invalid quantity",
	ErrIllegalQuantity:       "Quantity is illegal",
	ErrIllegalPrice:          "Price is illegal",
	ErrPriceExceedsUpper:     "Price exceeds upper limit",
	ErrPriceExceedsLower:     "Price exceeds lower limit",
	ErrNoTradeAuth:           "No transaction authority",
	ErrInsufficientMargin:    "Insufficient margin",
	ErrInvalidAPIKey:         "Invalid API KEY",
	ErrAPIKeyExpired:         "API key has expired",
	ErrAPIKeyLimitExceeded:   "API key limit exceeded",
	ErrKeyIsNull:             "Key is null",
	ErrExceededQueryRate:     "Exceeded maximum query count per second",
	ErrOrderLimitExceeded:    "Order limit exceeded",
	ErrExceededMaxVolume:     "Exceeded maximum trading volume",
	ErrBelowMinVolume:        "Less than minimum trading volume",
	ErrAuthSyncFailed:        "Authentication synchronization failed",
	ErrAuthParamsLost:        "Authentication parameters lost",
	ErrSignatureVerifyFailed: "Authentication and signature verification failed",
	ErrRequestTimeout:        "Request timed out",
	ErrIllegalParameter:      "Illegal parameter",
	ErrAuthFailed:            "Authentication failed",
	ErrKeyNotExist:           "Key does not exist",
	ErrNoPermission:          "No permission",
	ErrInvalidSignature:      "Invalid signature",
	ErrRepeatRequest:         "Repeat request",
	ErrTooFrequentRequest:    "Request is too frequent",
}

// APIResponse is the generic response wrapper for contract API
type APIResponse struct {
	Result    interface{} `json:"result"`
	ErrorCode int         `json:"error_code"`
	Msg       string      `json:"msg"`
	Data      interface{} `json:"data"`
	Success   bool        `json:"success"`
}

// SpotAPIResponse is the response wrapper for spot API
type SpotAPIResponse struct {
	Result    bool        `json:"result"`
	ErrorCode int         `json:"error_code,omitempty"`
	Msg       string      `json:"msg,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

// ContractInstrument represents a contract trading instrument
type ContractInstrument struct {
	Symbol               string  `json:"symbol"`
	SymbolName           string  `json:"symbolName"`
	BaseCurrency         string  `json:"baseCurrency"`
	ClearCurrency        string  `json:"clearCurrency"`
	PriceCurrency        string  `json:"priceCurrency"`
	ExchangeID           string  `json:"exchangeID"`
	DefaultLeverage      float64 `json:"defaultLeverage"`
	MaxOrderVolume       string  `json:"maxOrderVolume"`
	MinOrderCost         string  `json:"minOrderCost"`
	MinOrderVolume       string  `json:"minOrderVolume"`
	PriceTick            float64 `json:"priceTick"`
	VolumeTick           float64 `json:"volumeTick"`
	VolumeMultiple       float64 `json:"volumeMultiple"`
	PriceLimitUpperValue float64 `json:"priceLimitUpperValue"`
	PriceLimitLowerValue float64 `json:"priceLimitLowerValue"`
}

// ContractMarketData represents market data for a contract
type ContractMarketData struct {
	Symbol             string `json:"symbol"`
	LastPrice          string `json:"lastPrice"`
	MarkedPrice        string `json:"markedPrice"`
	HighestPrice       string `json:"highestPrice"`
	LowestPrice        string `json:"lowestPrice"`
	OpenPrice          string `json:"openPrice"`
	Volume             string `json:"volume"`
	Turnover           string `json:"turnover"`
	PrePositionFeeRate string `json:"prePositionFeeRate"` // Funding rate
}

// ContractOrderbookLevel represents a single orderbook level
type ContractOrderbookLevel struct {
	Orders int     `json:"orders"`
	Price  float64 `json:"price"`
	Volume float64 `json:"volume"`
}

// ContractOrderbook represents the contract orderbook
type ContractOrderbook struct {
	Symbol string                   `json:"symbol"`
	Asks   []ContractOrderbookLevel `json:"asks"`
	Bids   []ContractOrderbookLevel `json:"bids"`
}

// ContractAccount represents account info
type ContractAccount struct {
	Asset           string  `json:"asset"`
	AvailableMargin float64 `json:"availableMargin"`
	FrozenMargin    float64 `json:"frozenMargin"`
	UnrealizedPnL   float64 `json:"unrealizedPnL"`
	RealizedPnL     float64 `json:"realizedPnL"`
}

// ContractPosition represents a futures position
type ContractPosition struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"`
	Volume           float64 `json:"volume"`
	AvailableVolume  float64 `json:"availableVolume"`
	AvgPrice         float64 `json:"avgPrice"`
	Leverage         float64 `json:"leverage"`
	Margin           float64 `json:"margin"`
	UnrealizedPnL    float64 `json:"unrealizedPnL"`
	LiquidationPrice float64 `json:"liquidationPrice"`
}

// ContractOrder represents a futures order
type ContractOrder struct {
	OrderID    string  `json:"orderId"`
	Symbol     string  `json:"symbol"`
	Side       string  `json:"side"`
	Type       string  `json:"type"`
	Price      float64 `json:"price"`
	Volume     float64 `json:"volume"`
	FilledVol  float64 `json:"filledVolume"`
	AvgPrice   float64 `json:"avgPrice"`
	Status     int     `json:"status"`
	CreateTime int64   `json:"createTime"`
	UpdateTime int64   `json:"updateTime"`
}

// SpotTicker represents spot ticker data
type SpotTicker struct {
	Symbol    string         `json:"symbol"`
	Timestamp int64          `json:"timestamp"`
	Ticker    SpotTickerData `json:"ticker"`
}

// SpotTickerData contains ticker statistics
type SpotTickerData struct {
	High     string `json:"high"`
	Low      string `json:"low"`
	Vol      string `json:"vol"`
	Change   string `json:"change"`
	Turnover string `json:"turnover"`
	Latest   string `json:"latest"`
}

// SpotOrderbook represents spot orderbook
type SpotOrderbook struct {
	Asks      [][]interface{} `json:"asks"` // [[price, qty], ...]
	Bids      [][]interface{} `json:"bids"`
	Timestamp int64           `json:"timestamp"`
}

// SpotTrade represents a spot trade
type SpotTrade struct {
	DateMs int64   `json:"date_ms"`
	Amount float64 `json:"amount"`
	Price  float64 `json:"price"`
	Type   string  `json:"type"` // buy/sell
	TID    string  `json:"tid"`
}

// SpotOrder represents a spot order
type SpotOrder struct {
	OrderID    string  `json:"order_id"`
	Symbol     string  `json:"symbol"`
	Type       string  `json:"type"` // buy/sell
	Price      float64 `json:"price"`
	Amount     float64 `json:"amount"`
	DealAmount float64 `json:"deal_amount"`
	AvgPrice   float64 `json:"avg_price"`
	Status     int     `json:"status"`
	CreateTime int64   `json:"create_time"`
}

// SpotAssetConfig represents asset deposit/withdrawal config
type SpotAssetConfig struct {
	AssetCode   string      `json:"assetCode"`
	Chain       string      `json:"chain"`
	CanWithdraw bool        `json:"canWithDraw"`
	CanDeposit  bool        `json:"canDeposit"`
	MinWithdraw json.Number `json:"minWithDraw"`
	Fee         json.Number `json:"fee"`
}

// SpotUserInfo represents user account info
type SpotUserInfo struct {
	Result bool             `json:"result"`
	Info   SpotUserInfoData `json:"info"`
}

// SpotUserInfoData contains account balances
type SpotUserInfoData struct {
	Freeze map[string]float64 `json:"freeze"`
	Asset  map[string]float64 `json:"asset"`
	Free   map[string]float64 `json:"free"`
}

// WsMessage represents a WebSocket message
type WsMessage struct {
	Action    string `json:"action,omitempty"`
	Subscribe string `json:"subscribe,omitempty"`
	Request   string `json:"request,omitempty"`
	Channel   string `json:"channel,omitempty"`
	Depth     string `json:"depth,omitempty"`
	Kbar      string `json:"kbar,omitempty"`
	Pair      string `json:"pair,omitempty"`
	Symbol    string `json:"symbol,omitempty"`
	Ping      string `json:"ping,omitempty"`
	Pong      string `json:"pong,omitempty"`
}

// WsDepthResponse represents WebSocket depth update
type WsDepthResponse struct {
	Type   string `json:"type"`
	Pair   string `json:"pair"`
	Server string `json:"SERVER"`
	TS     string `json:"TS"`
	Count  int    `json:"count"`
	Depth  struct {
		Asks [][]float64 `json:"asks"`
		Bids [][]float64 `json:"bids"`
	} `json:"depth"`
}

// WsTradeResponse represents WebSocket trade update
type WsTradeResponse struct {
	Type   string `json:"type"`
	Pair   string `json:"pair"`
	Server string `json:"SERVER"`
	TS     string `json:"TS"`
	Trade  struct {
		Volume    float64 `json:"volume"`
		Amount    float64 `json:"amount"`
		Price     float64 `json:"price"`
		Direction string  `json:"direction"`
		TS        string  `json:"TS"`
	} `json:"trade"`
}

// WsTickResponse represents WebSocket ticker update
type WsTickResponse struct {
	Type   string `json:"type"`
	Pair   string `json:"pair"`
	Server string `json:"SERVER"`
	TS     string `json:"TS"`
	Tick   struct {
		High     float64 `json:"high"`
		Low      float64 `json:"low"`
		Latest   float64 `json:"latest"`
		Vol      float64 `json:"vol"`
		Turnover float64 `json:"turnover"`
		Change   float64 `json:"change"`
		ToCNY    float64 `json:"to_cny"`
		ToUSD    float64 `json:"to_usd"`
		CNY      float64 `json:"cny"`
		USD      float64 `json:"usd"`
		Dir      string  `json:"dir"`
	} `json:"tick"`
}

// WsKbarResponse represents WebSocket kline update
type WsKbarResponse struct {
	Type   string `json:"type"`
	Pair   string `json:"pair"`
	Server string `json:"SERVER"`
	TS     string `json:"TS"`
	Kbar   struct {
		T    string  `json:"t"`
		O    float64 `json:"o"`
		H    float64 `json:"h"`
		L    float64 `json:"l"`
		C    float64 `json:"c"`
		V    float64 `json:"v"`
		A    float64 `json:"a"`
		N    int     `json:"n"`
		Slot string  `json:"slot"`
	} `json:"kbar"`
}

// Credentials holds API authentication info
type Credentials struct {
	APIKey          string
	SecretKey       string
	SignatureMethod string // RSA or HmacSHA256
}

// ClientConfig holds client configuration
type ClientConfig struct {
	Credentials    *Credentials
	UseContractAPI bool   // Use contract API vs spot API
	ProductGroup   string // Product group for contract API
	DepthLevels    int    // Orderbook depth levels
	ReconnectDelay time.Duration
	PingInterval   time.Duration
	RequestTimeout time.Duration
}

// DefaultClientConfig returns default configuration
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		UseContractAPI: true,
		ProductGroup:   ProductGroupSwapU,
		DepthLevels:    50,
		ReconnectDelay: 5 * time.Second,
		PingInterval:   30 * time.Second,
		RequestTimeout: 10 * time.Second,
	}
}
