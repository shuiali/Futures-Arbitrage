// Package okx provides types and clients for OKX exchange API integration.
// Supports REST and WebSocket APIs for market data, trading, and account management.
package okx

import (
	"encoding/json"
	"time"
)

// =============================================================================
// Common Types
// =============================================================================

// OKX instrument types
const (
	InstTypeSpot    = "SPOT"
	InstTypeMargin  = "MARGIN"
	InstTypeSwap    = "SWAP"    // Perpetual futures
	InstTypeFutures = "FUTURES" // Expiry futures
	InstTypeOption  = "OPTION"
)

// Trade modes
const (
	TdModeCash     = "cash"
	TdModeIsolated = "isolated"
	TdModeCross    = "cross"
)

// Order types
const (
	OrdTypeMarket   = "market"
	OrdTypeLimit    = "limit"
	OrdTypePostOnly = "post_only"
	OrdTypeFOK      = "fok" // Fill or kill
	OrdTypeIOC      = "ioc" // Immediate or cancel
)

// Order sides
const (
	SideBuy  = "buy"
	SideSell = "sell"
)

// Position sides (for hedge mode)
const (
	PosSideLong  = "long"
	PosSideShort = "short"
	PosSideNet   = "net"
)

// Order states
const (
	OrderStateLive            = "live"
	OrderStatePartiallyFilled = "partially_filled"
	OrderStateFilled          = "filled"
	OrderStateCanceled        = "canceled"
	OrderStateMMPCanceled     = "mmp_canceled"
)

// Position modes
const (
	PosModeNet   = "net_mode"
	PosModeHedge = "long_short_mode"
)

// Timestamp is a custom time type for OKX API timestamps (milliseconds)
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
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data T      `json:"data"`
}

// =============================================================================
// Instrument Types
// =============================================================================

// Instrument represents a trading instrument
type Instrument struct {
	InstID       string    `json:"instId"`
	InstType     string    `json:"instType"`
	Uly          string    `json:"uly"`        // Underlying index
	InstFamily   string    `json:"instFamily"` // Instrument family
	BaseCcy      string    `json:"baseCcy"`
	QuoteCcy     string    `json:"quoteCcy"`
	SettleCcy    string    `json:"settleCcy"` // Settlement currency
	CtVal        string    `json:"ctVal"`     // Contract value
	CtMult       string    `json:"ctMult"`    // Contract multiplier
	CtValCcy     string    `json:"ctValCcy"`  // Contract value currency
	OptType      string    `json:"optType"`   // Option type (C=Call, P=Put)
	Stk          string    `json:"stk"`       // Strike price (options)
	ListTime     Timestamp `json:"listTime"`
	ExpTime      Timestamp `json:"expTime"`      // Expiry time
	Lever        string    `json:"lever"`        // Maximum leverage
	TickSz       string    `json:"tickSz"`       // Tick size (price increment)
	LotSz        string    `json:"lotSz"`        // Lot size (quantity increment)
	MinSz        string    `json:"minSz"`        // Minimum order size
	CtType       string    `json:"ctType"`       // Contract type: linear, inverse
	Alias        string    `json:"alias"`        // this_week, next_week, quarter, next_quarter
	State        string    `json:"state"`        // live, suspend, preopen, settlement
	MaxLmtSz     string    `json:"maxLmtSz"`     // Max limit order size
	MaxMktSz     string    `json:"maxMktSz"`     // Max market order size
	MaxTwapSz    string    `json:"maxTwapSz"`    // Max TWAP order size
	MaxIcebergSz string    `json:"maxIcebergSz"` // Max iceberg order size
	MaxTriggerSz string    `json:"maxTriggerSz"` // Max trigger order size
	MaxStopSz    string    `json:"maxStopSz"`    // Max stop order size
	GroupID      string    `json:"groupId"`      // Fee group ID
}

// =============================================================================
// Market Data Types
// =============================================================================

// Ticker represents market ticker data
type Ticker struct {
	InstID    string    `json:"instId"`
	InstType  string    `json:"instType"`
	Last      string    `json:"last"`      // Last price
	LastSz    string    `json:"lastSz"`    // Last size
	AskPx     string    `json:"askPx"`     // Best ask price
	AskSz     string    `json:"askSz"`     // Best ask size
	BidPx     string    `json:"bidPx"`     // Best bid price
	BidSz     string    `json:"bidSz"`     // Best bid size
	Open24h   string    `json:"open24h"`   // 24h open price
	High24h   string    `json:"high24h"`   // 24h high
	Low24h    string    `json:"low24h"`    // 24h low
	VolCcy24h string    `json:"volCcy24h"` // 24h volume in currency
	Vol24h    string    `json:"vol24h"`    // 24h volume in contracts
	SodUtc0   string    `json:"sodUtc0"`   // Start of day price UTC+0
	SodUtc8   string    `json:"sodUtc8"`   // Start of day price UTC+8
	Ts        Timestamp `json:"ts"`
}

// OrderBookLevel represents a single order book level
// Format: [price, size, deprecated, orderCount]
type OrderBookLevel struct {
	Price      string
	Size       string
	Deprecated string
	OrderCount string
}

// UnmarshalJSON implements json.Unmarshaler for OrderBookLevel
func (o *OrderBookLevel) UnmarshalJSON(data []byte) error {
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	if len(arr) >= 4 {
		o.Price = arr[0]
		o.Size = arr[1]
		o.Deprecated = arr[2]
		o.OrderCount = arr[3]
	}
	return nil
}

// MarshalJSON implements json.Marshaler for OrderBookLevel
func (o OrderBookLevel) MarshalJSON() ([]byte, error) {
	return json.Marshal([]string{o.Price, o.Size, o.Deprecated, o.OrderCount})
}

// OrderBook represents order book data
type OrderBook struct {
	Asks      []OrderBookLevel `json:"asks"`
	Bids      []OrderBookLevel `json:"bids"`
	Ts        Timestamp        `json:"ts"`
	Checksum  int64            `json:"checksum"`  // CRC32 checksum
	SeqID     int64            `json:"seqId"`     // Sequence ID
	PrevSeqID int64            `json:"prevSeqId"` // Previous sequence ID (for updates)
}

// Candlestick represents OHLCV candlestick data
// Array format: [ts, open, high, low, close, vol, volCcy, volCcyQuote, confirm]
type Candlestick struct {
	Ts          Timestamp
	Open        string
	High        string
	Low         string
	Close       string
	Vol         string // Volume in contracts
	VolCcy      string // Volume in coin
	VolCcyQuote string // Volume in quote currency
	Confirm     string // "0" = uncompleted, "1" = completed
}

// UnmarshalJSON implements json.Unmarshaler for Candlestick
func (c *Candlestick) UnmarshalJSON(data []byte) error {
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	if len(arr) >= 9 {
		var ts int64
		if err := json.Unmarshal([]byte(arr[0]), &ts); err != nil {
			return err
		}
		c.Ts = Timestamp(ts)
		c.Open = arr[1]
		c.High = arr[2]
		c.Low = arr[3]
		c.Close = arr[4]
		c.Vol = arr[5]
		c.VolCcy = arr[6]
		c.VolCcyQuote = arr[7]
		c.Confirm = arr[8]
	}
	return nil
}

// Trade represents a public trade
type Trade struct {
	InstID  string    `json:"instId"`
	TradeID string    `json:"tradeId"`
	Px      string    `json:"px"`
	Sz      string    `json:"sz"`
	Side    string    `json:"side"`
	Ts      Timestamp `json:"ts"`
}

// IndexTicker represents index price data
type IndexTicker struct {
	InstID  string    `json:"instId"`
	IdxPx   string    `json:"idxPx"` // Index price
	High24h string    `json:"high24h"`
	Low24h  string    `json:"low24h"`
	Open24h string    `json:"open24h"`
	SodUtc0 string    `json:"sodUtc0"`
	SodUtc8 string    `json:"sodUtc8"`
	Ts      Timestamp `json:"ts"`
}

// =============================================================================
// Funding Rate Types
// =============================================================================

// FundingRate represents funding rate information
type FundingRate struct {
	InstID          string    `json:"instId"`
	InstType        string    `json:"instType"`
	FundingRate     string    `json:"fundingRate"`
	NextFundingRate string    `json:"nextFundingRate"`
	FundingTime     Timestamp `json:"fundingTime"`
	NextFundingTime Timestamp `json:"nextFundingTime"`
	MinFundingRate  string    `json:"minFundingRate"`
	MaxFundingRate  string    `json:"maxFundingRate"`
	SettState       string    `json:"settState"` // settled, processing
	Method          string    `json:"method"`    // next_period, current_period
}

// FundingRateHistory represents historical funding rate
type FundingRateHistory struct {
	InstID       string    `json:"instId"`
	InstType     string    `json:"instType"`
	FundingRate  string    `json:"fundingRate"`
	RealizedRate string    `json:"realizedRate"`
	FundingTime  Timestamp `json:"fundingTime"`
	Method       string    `json:"method"`
}

// =============================================================================
// Currency / Asset Types
// =============================================================================

// Currency represents currency/asset information
type Currency struct {
	Ccy                  string `json:"ccy"`                  // Currency
	Name                 string `json:"name"`                 // Currency name
	Chain                string `json:"chain"`                // Chain
	CanDep               bool   `json:"canDep"`               // Can deposit
	CanWd                bool   `json:"canWd"`                // Can withdraw
	CanInternal          bool   `json:"canInternal"`          // Can internal transfer
	MinDep               string `json:"minDep"`               // Minimum deposit
	MinWd                string `json:"minWd"`                // Minimum withdrawal
	MaxWd                string `json:"maxWd"`                // Maximum withdrawal
	WdTickSz             string `json:"wdTickSz"`             // Withdrawal decimal places
	WdQuota              string `json:"wdQuota"`              // 24h withdrawal quota
	UsedWdQuota          string `json:"usedWdQuota"`          // Used withdrawal quota
	MinFee               string `json:"minFee"`               // Minimum withdrawal fee
	MaxFee               string `json:"maxFee"`               // Maximum withdrawal fee
	MainNet              bool   `json:"mainNet"`              // Is main net
	NeedTag              bool   `json:"needTag"`              // Needs tag/memo
	MinDepArrivalConfirm string `json:"minDepArrivalConfirm"` // Min confirmations for deposit
	MinWdUnlockConfirm   string `json:"minWdUnlockConfirm"`   // Min confirmations for withdrawal unlock
	DepQuotaFixed        string `json:"depQuotaFixed"`        // Fixed deposit quota
	UsedDepQuotaFixed    string `json:"usedDepQuotaFixed"`    // Used fixed deposit quota
	DepQuoteDailyLayer2  string `json:"depQuoteDailyLayer2"`  // Daily deposit quota for layer 2
}

// =============================================================================
// Trading Fee Types
// =============================================================================

// TradeFee represents trading fee rates
type TradeFee struct {
	Level    string     `json:"level"`    // Fee level
	Taker    string     `json:"taker"`    // Taker fee rate
	Maker    string     `json:"maker"`    // Maker fee rate
	TakerU   string     `json:"takerU"`   // Taker fee rate for USDT/USDC
	MakerU   string     `json:"makerU"`   // Maker fee rate for USDT/USDC
	Delivery string     `json:"delivery"` // Delivery fee
	Exercise string     `json:"exercise"` // Exercise fee
	InstType string     `json:"instType"`
	Ts       Timestamp  `json:"ts"`
	FeeGroup []FeeGroup `json:"feeGroup,omitempty"`
}

// FeeGroup represents fee group details
type FeeGroup struct {
	GroupID string `json:"groupId"`
	Maker   string `json:"maker"`
	Taker   string `json:"taker"`
}

// =============================================================================
// Account Types
// =============================================================================

// AccountBalance represents account balance information
type AccountBalance struct {
	UTime       Timestamp       `json:"uTime"`
	TotalEq     string          `json:"totalEq"`     // Total equity in USD
	AdjEq       string          `json:"adjEq"`       // Adjusted equity
	IsoEq       string          `json:"isoEq"`       // Isolated margin equity
	OrdFroz     string          `json:"ordFroz"`     // Margin frozen for orders
	Imr         string          `json:"imr"`         // Initial margin requirement
	Mmr         string          `json:"mmr"`         // Maintenance margin requirement
	MgnRatio    string          `json:"mgnRatio"`    // Margin ratio
	NotionalUsd string          `json:"notionalUsd"` // Notional value in USD
	Upl         string          `json:"upl"`         // Unrealized PnL
	BorrowFroz  string          `json:"borrowFroz"`  // Frozen for borrow
	Details     []BalanceDetail `json:"details"`
}

// BalanceDetail represents per-currency balance details
type BalanceDetail struct {
	Ccy             string    `json:"ccy"`
	Eq              string    `json:"eq"`              // Equity
	CashBal         string    `json:"cashBal"`         // Cash balance
	AvailBal        string    `json:"availBal"`        // Available balance
	FrozenBal       string    `json:"frozenBal"`       // Frozen balance
	AvailEq         string    `json:"availEq"`         // Available equity
	Upl             string    `json:"upl"`             // Unrealized PnL
	UplLiab         string    `json:"uplLiab"`         // Unrealized PnL for liability
	CrossLiab       string    `json:"crossLiab"`       // Cross liabilities
	IsoLiab         string    `json:"isoLiab"`         // Isolated liabilities
	MgnRatio        string    `json:"mgnRatio"`        // Margin ratio
	Interest        string    `json:"interest"`        // Interest
	Twap            string    `json:"twap"`            // TWAP margin
	MaxLoan         string    `json:"maxLoan"`         // Max loan
	EqUsd           string    `json:"eqUsd"`           // Equity in USD
	BorrowFroz      string    `json:"borrowFroz"`      // Frozen for borrow
	NotionalLever   string    `json:"notionalLever"`   // Notional leverage
	StgyEq          string    `json:"stgyEq"`          // Strategy equity
	IsoUpl          string    `json:"isoUpl"`          // Isolated unrealized PnL
	SpotInUseAmt    string    `json:"spotInUseAmt"`    // Spot in use amount
	ClSpotInUseAmt  string    `json:"clSpotInUseAmt"`  // Cross liab spot in use
	MaxSpotInUseAmt string    `json:"maxSpotInUseAmt"` // Max spot in use
	SpotIsoBal      string    `json:"spotIsoBal"`      // Spot isolated balance
	SmtSyncEq       string    `json:"smtSyncEq"`       // Smart sync equity
	UTime           Timestamp `json:"uTime"`
}

// Position represents an open position
type Position struct {
	InstID         string           `json:"instId"`
	InstType       string           `json:"instType"`
	PosID          string           `json:"posId"`                    // Position ID
	PosSide        string           `json:"posSide"`                  // long, short, net
	Pos            string           `json:"pos"`                      // Position quantity
	AvailPos       string           `json:"availPos"`                 // Available position (closeable)
	AvgPx          string           `json:"avgPx"`                    // Average entry price
	MarkPx         string           `json:"markPx"`                   // Mark price
	IdxPx          string           `json:"idxPx"`                    // Index price
	Upl            string           `json:"upl"`                      // Unrealized PnL
	UplRatio       string           `json:"uplRatio"`                 // Unrealized PnL ratio
	UplLastPx      string           `json:"uplLastPx"`                // UPL based on last price
	UplRatioLastPx string           `json:"uplRatioLastPx"`           // UPL ratio based on last price
	Lever          string           `json:"lever"`                    // Leverage
	MgnMode        string           `json:"mgnMode"`                  // Margin mode: cross, isolated
	Margin         string           `json:"margin"`                   // Margin
	MgnRatio       string           `json:"mgnRatio"`                 // Margin ratio
	Mmr            string           `json:"mmr"`                      // Maintenance margin requirement
	Imr            string           `json:"imr"`                      // Initial margin requirement
	LiqPx          string           `json:"liqPx"`                    // Liquidation price
	NotionalUsd    string           `json:"notionalUsd"`              // Notional value in USD
	ADL            string           `json:"adl"`                      // Auto-deleveraging indicator (1-5)
	Interest       string           `json:"interest"`                 // Interest
	TradeID        string           `json:"tradeId"`                  // Last trade ID
	Last           string           `json:"last"`                     // Last price
	DeltaBS        string           `json:"deltaBS"`                  // Delta (options)
	DeltaPA        string           `json:"deltaPA"`                  // Delta (options)
	GammaBS        string           `json:"gammaBS"`                  // Gamma (options)
	GammaPA        string           `json:"gammaPA"`                  // Gamma (options)
	ThetaBS        string           `json:"thetaBS"`                  // Theta (options)
	ThetaPA        string           `json:"thetaPA"`                  // Theta (options)
	VegaBS         string           `json:"vegaBS"`                   // Vega (options)
	VegaPA         string           `json:"vegaPA"`                   // Vega (options)
	BePx           string           `json:"bePx"`                     // Breakeven price
	CTime          Timestamp        `json:"cTime"`                    // Creation time
	UTime          Timestamp        `json:"uTime"`                    // Update time
	PTime          Timestamp        `json:"pTime"`                    // Push time
	RealizedPnl    string           `json:"realizedPnl"`              // Realized PnL
	Fee            string           `json:"fee"`                      // Accumulated fees
	FundingFee     string           `json:"fundingFee"`               // Accumulated funding fees
	CloseOrderAlgo []CloseOrderAlgo `json:"closeOrderAlgo,omitempty"` // TP/SL orders
	BizRefID       string           `json:"bizRefId"`                 // Business reference ID
	BizRefType     string           `json:"bizRefType"`               // Business reference type
}

// CloseOrderAlgo represents TP/SL order attached to position
type CloseOrderAlgo struct {
	AlgoID          string `json:"algoId"`
	SlTriggerPx     string `json:"slTriggerPx"`
	SlTriggerPxType string `json:"slTriggerPxType"`
	TpTriggerPx     string `json:"tpTriggerPx"`
	TpTriggerPxType string `json:"tpTriggerPxType"`
	CloseFraction   string `json:"closeFraction"`
}

// AccountConfig represents account configuration
type AccountConfig struct {
	UID             string   `json:"uid"`
	AcctLv          string   `json:"acctLv"`          // Account level: 1=Spot, 2=Futures, 3=Multi-currency margin, 4=Portfolio margin
	PosMode         string   `json:"posMode"`         // Position mode: long_short_mode, net_mode
	AutoLoan        bool     `json:"autoLoan"`        // Auto loan enabled
	GreeksType      string   `json:"greeksType"`      // Greeks display type: PA, BS
	Level           string   `json:"level"`           // Fee level
	LevelTmp        string   `json:"levelTmp"`        // Temporary fee level
	CtIsoMode       string   `json:"ctIsoMode"`       // Contract isolated margin mode
	MgnIsoMode      string   `json:"mgnIsoMode"`      // Margin isolated margin mode
	SpotOffsetType  string   `json:"spotOffsetType"`  // Spot offset type
	RoleType        string   `json:"roleType"`        // Role type: 0=General, 1=Leading trader
	TraderInsts     []string `json:"traderInsts"`     // Trader instruments
	SpotRoleType    string   `json:"spotRoleType"`    // Spot role type
	SpotTraderInsts []string `json:"spotTraderInsts"` // Spot trader instruments
	OpAuth          string   `json:"opAuth"`          // Option auth
	IP              string   `json:"ip"`              // IP addresses
	Perm            string   `json:"perm"`            // Permissions
	KycLv           string   `json:"kycLv"`           // KYC level
	Label           string   `json:"label"`           // Account label
	MainUID         string   `json:"mainUid"`         // Main account UID
}

// =============================================================================
// Order Types
// =============================================================================

// Order represents an order
type Order struct {
	InstID            string            `json:"instId"`
	OrderID           string            `json:"ordId"`
	ClOrdID           string            `json:"clOrdId"`
	Tag               string            `json:"tag"`
	Px                string            `json:"px"`          // Price
	Sz                string            `json:"sz"`          // Size
	NotionalUsd       string            `json:"notionalUsd"` // Notional value in USD
	OrdType           string            `json:"ordType"`     // Order type
	Side              string            `json:"side"`
	PosSide           string            `json:"posSide"`                  // Position side
	TdMode            string            `json:"tdMode"`                   // Trade mode
	TgtCcy            string            `json:"tgtCcy"`                   // Target currency: base_ccy, quote_ccy
	FillPx            string            `json:"fillPx"`                   // Last fill price
	FillSz            string            `json:"fillSz"`                   // Last fill size
	FillTime          Timestamp         `json:"fillTime"`                 // Last fill time
	FillFee           string            `json:"fillFee"`                  // Last fill fee
	FillFeeCcy        string            `json:"fillFeeCcy"`               // Last fill fee currency
	FillPnl           string            `json:"fillPnl"`                  // Last fill PnL
	AccFillSz         string            `json:"accFillSz"`                // Accumulated fill size
	AvgPx             string            `json:"avgPx"`                    // Average fill price
	State             string            `json:"state"`                    // Order state
	Lever             string            `json:"lever"`                    // Leverage
	AttachAlgoClOrdID string            `json:"attachAlgoClOrdId"`        // Attached algo order ID
	TpTriggerPx       string            `json:"tpTriggerPx"`              // Take profit trigger price
	TpTriggerPxType   string            `json:"tpTriggerPxType"`          // TP trigger price type
	TpOrdPx           string            `json:"tpOrdPx"`                  // TP order price
	SlTriggerPx       string            `json:"slTriggerPx"`              // Stop loss trigger price
	SlTriggerPxType   string            `json:"slTriggerPxType"`          // SL trigger price type
	SlOrdPx           string            `json:"slOrdPx"`                  // SL order price
	AttachAlgoOrds    []AttachAlgoOrder `json:"attachAlgoOrds,omitempty"` // Attached algo orders
	StpID             string            `json:"stpId"`                    // Self-trade prevention ID
	StpMode           string            `json:"stpMode"`                  // STP mode
	FeeCcy            string            `json:"feeCcy"`                   // Fee currency
	Fee               string            `json:"fee"`                      // Fee
	RebateCcy         string            `json:"rebateCcy"`                // Rebate currency
	Rebate            string            `json:"rebate"`                   // Rebate
	Pnl               string            `json:"pnl"`                      // PnL
	Source            string            `json:"source"`                   // Order source
	CancelSource      string            `json:"cancelSource"`             // Cancel source
	Category          string            `json:"category"`                 // Category
	UTime             Timestamp         `json:"uTime"`                    // Update time
	CTime             Timestamp         `json:"cTime"`                    // Creation time
	ReqID             string            `json:"reqId"`                    // Request ID for amend
	AmendResult       string            `json:"amendResult"`              // Amend result
	ReduceOnly        string            `json:"reduceOnly"`               // Reduce only
	QuickMgnType      string            `json:"quickMgnType"`             // Quick margin type
	AlgoClOrdID       string            `json:"algoClOrdId"`              // Algo client order ID
	AlgoID            string            `json:"algoId"`                   // Algo order ID
	IsTpLimit         string            `json:"isTpLimit"`                // Is TP limit
	ExecType          string            `json:"execType"`                 // Execution type: T=Taker, M=Maker
	TradeID           string            `json:"tradeId"`                  // Trade ID
	LastPx            string            `json:"lastPx"`                   // Last price
	FillNotionalUsd   string            `json:"fillNotionalUsd"`          // Fill notional USD
}

// AttachAlgoOrder represents attached algo order
type AttachAlgoOrder struct {
	AttachAlgoClOrdID string `json:"attachAlgoClOrdId"`
	TpTriggerPx       string `json:"tpTriggerPx"`
	TpTriggerPxType   string `json:"tpTriggerPxType"`
	TpOrdPx           string `json:"tpOrdPx"`
	SlTriggerPx       string `json:"slTriggerPx"`
	SlTriggerPxType   string `json:"slTriggerPxType"`
	SlOrdPx           string `json:"slOrdPx"`
	Sz                string `json:"sz"`
	AttachAlgoID      string `json:"attachAlgoId"`
}

// OrderResult represents order placement result
type OrderResult struct {
	OrdID   string `json:"ordId"`
	ClOrdID string `json:"clOrdId"`
	Tag     string `json:"tag"`
	SCode   string `json:"sCode"` // Sub-code
	SMsg    string `json:"sMsg"`  // Sub-message
}

// CancelResult represents order cancellation result
type CancelResult struct {
	OrdID   string `json:"ordId"`
	ClOrdID string `json:"clOrdId"`
	SCode   string `json:"sCode"`
	SMsg    string `json:"sMsg"`
}

// AmendResult represents order amendment result
type AmendResult struct {
	OrdID   string `json:"ordId"`
	ClOrdID string `json:"clOrdId"`
	ReqID   string `json:"reqId"`
	SCode   string `json:"sCode"`
	SMsg    string `json:"sMsg"`
}

// =============================================================================
// Order Request Types
// =============================================================================

// PlaceOrderRequest represents order placement request
type PlaceOrderRequest struct {
	InstID       string `json:"instId"`
	TdMode       string `json:"tdMode"`                 // cash, isolated, cross
	Side         string `json:"side"`                   // buy, sell
	OrdType      string `json:"ordType"`                // market, limit, post_only, fok, ioc
	Sz           string `json:"sz"`                     // Size
	Px           string `json:"px,omitempty"`           // Price (required for limit)
	ClOrdID      string `json:"clOrdId,omitempty"`      // Client order ID (max 32)
	Tag          string `json:"tag,omitempty"`          // Order tag (max 16)
	PosSide      string `json:"posSide,omitempty"`      // long, short, net
	ReduceOnly   bool   `json:"reduceOnly,omitempty"`   // Reduce only
	TgtCcy       string `json:"tgtCcy,omitempty"`       // Target currency
	Ccy          string `json:"ccy,omitempty"`          // Currency for margin
	BanAmend     bool   `json:"banAmend,omitempty"`     // Ban amendment
	QuickMgnType string `json:"quickMgnType,omitempty"` // Quick margin type
	StpID        string `json:"stpId,omitempty"`        // STP ID
	StpMode      string `json:"stpMode,omitempty"`      // STP mode
	// TP/SL parameters
	TpTriggerPx     string `json:"tpTriggerPx,omitempty"`
	TpTriggerPxType string `json:"tpTriggerPxType,omitempty"` // last, index, mark
	TpOrdPx         string `json:"tpOrdPx,omitempty"`
	SlTriggerPx     string `json:"slTriggerPx,omitempty"`
	SlTriggerPxType string `json:"slTriggerPxType,omitempty"`
	SlOrdPx         string `json:"slOrdPx,omitempty"`
}

// CancelOrderRequest represents order cancellation request
type CancelOrderRequest struct {
	InstID  string `json:"instId"`
	OrdID   string `json:"ordId,omitempty"`
	ClOrdID string `json:"clOrdId,omitempty"`
}

// AmendOrderRequest represents order amendment request
type AmendOrderRequest struct {
	InstID    string `json:"instId"`
	OrdID     string `json:"ordId,omitempty"`
	ClOrdID   string `json:"clOrdId,omitempty"`
	ReqID     string `json:"reqId,omitempty"`     // Request ID
	CxlOnFail bool   `json:"cxlOnFail,omitempty"` // Cancel on failure
	NewSz     string `json:"newSz,omitempty"`     // New size
	NewPx     string `json:"newPx,omitempty"`     // New price
	// New TP/SL parameters
	NewTpTriggerPx     string `json:"newTpTriggerPx,omitempty"`
	NewTpTriggerPxType string `json:"newTpTriggerPxType,omitempty"`
	NewTpOrdPx         string `json:"newTpOrdPx,omitempty"`
	NewSlTriggerPx     string `json:"newSlTriggerPx,omitempty"`
	NewSlTriggerPxType string `json:"newSlTriggerPxType,omitempty"`
	NewSlOrdPx         string `json:"newSlOrdPx,omitempty"`
}

// =============================================================================
// WebSocket Types
// =============================================================================

// WSRequest represents a WebSocket request
type WSRequest struct {
	Op   string        `json:"op"`
	Args []interface{} `json:"args"`
}

// WSRequestWithID represents a WebSocket request with ID (for trading)
type WSRequestWithID struct {
	ID   string        `json:"id"`
	Op   string        `json:"op"`
	Args []interface{} `json:"args"`
}

// WSResponse represents a WebSocket response
type WSResponse struct {
	Event   string          `json:"event,omitempty"`
	Code    string          `json:"code,omitempty"`
	Msg     string          `json:"msg,omitempty"`
	ConnID  string          `json:"connId,omitempty"`
	Channel string          `json:"channel,omitempty"`
	Arg     json.RawMessage `json:"arg,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Action  string          `json:"action,omitempty"` // snapshot, update
}

// WSTradeResponse represents a WebSocket trading response
type WSTradeResponse struct {
	ID   string          `json:"id"`
	Op   string          `json:"op"`
	Code string          `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data,omitempty"`
}

// WSLoginArg represents WebSocket login arguments
type WSLoginArg struct {
	APIKey     string `json:"apiKey"`
	Passphrase string `json:"passphrase"`
	Timestamp  string `json:"timestamp"`
	Sign       string `json:"sign"`
}

// WSSubscribeArg represents WebSocket subscription arguments
type WSSubscribeArg struct {
	Channel    string `json:"channel"`
	InstID     string `json:"instId,omitempty"`
	InstType   string `json:"instType,omitempty"`
	InstFamily string `json:"instFamily,omitempty"`
	Ccy        string `json:"ccy,omitempty"`
}

// WSChannelArg represents channel argument in push data
type WSChannelArg struct {
	Channel    string `json:"channel"`
	InstID     string `json:"instId,omitempty"`
	InstType   string `json:"instType,omitempty"`
	InstFamily string `json:"instFamily,omitempty"`
	UID        string `json:"uid,omitempty"`
}

// =============================================================================
// WebSocket Push Data Types
// =============================================================================

// WSTickerData represents ticker push data
type WSTickerData struct {
	InstID    string    `json:"instId"`
	InstType  string    `json:"instType"`
	Last      string    `json:"last"`
	LastSz    string    `json:"lastSz"`
	AskPx     string    `json:"askPx"`
	AskSz     string    `json:"askSz"`
	BidPx     string    `json:"bidPx"`
	BidSz     string    `json:"bidSz"`
	Open24h   string    `json:"open24h"`
	High24h   string    `json:"high24h"`
	Low24h    string    `json:"low24h"`
	VolCcy24h string    `json:"volCcy24h"`
	Vol24h    string    `json:"vol24h"`
	SodUtc0   string    `json:"sodUtc0"`
	SodUtc8   string    `json:"sodUtc8"`
	Ts        Timestamp `json:"ts"`
}

// WSOrderBookData represents order book push data
type WSOrderBookData struct {
	Asks      []OrderBookLevel `json:"asks"`
	Bids      []OrderBookLevel `json:"bids"`
	Ts        Timestamp        `json:"ts"`
	Checksum  int64            `json:"checksum"`
	SeqID     int64            `json:"seqId"`
	PrevSeqID int64            `json:"prevSeqId,omitempty"`
}

// WSTradeData represents trade push data
type WSTradeData struct {
	InstID  string    `json:"instId"`
	TradeID string    `json:"tradeId"`
	Px      string    `json:"px"`
	Sz      string    `json:"sz"`
	Side    string    `json:"side"`
	Ts      Timestamp `json:"ts"`
	Count   string    `json:"count,omitempty"`
}

// WSCandleData represents candlestick push data
type WSCandleData struct {
	Ts          Timestamp `json:"ts"`
	Open        string    `json:"open"`
	High        string    `json:"high"`
	Low         string    `json:"low"`
	Close       string    `json:"close"`
	Vol         string    `json:"vol"`
	VolCcy      string    `json:"volCcy"`
	VolCcyQuote string    `json:"volCcyQuote"`
	Confirm     string    `json:"confirm"`
}

// WSFundingRateData represents funding rate push data
type WSFundingRateData struct {
	InstID          string    `json:"instId"`
	InstType        string    `json:"instType"`
	FundingRate     string    `json:"fundingRate"`
	NextFundingRate string    `json:"nextFundingRate"`
	FundingTime     Timestamp `json:"fundingTime"`
	NextFundingTime Timestamp `json:"nextFundingTime"`
	Method          string    `json:"method"`
}

// WSAccountData represents account push data
type WSAccountData struct {
	UTime       Timestamp       `json:"uTime"`
	TotalEq     string          `json:"totalEq"`
	AdjEq       string          `json:"adjEq"`
	IsoEq       string          `json:"isoEq"`
	OrdFroz     string          `json:"ordFroz"`
	Imr         string          `json:"imr"`
	Mmr         string          `json:"mmr"`
	MgnRatio    string          `json:"mgnRatio"`
	NotionalUsd string          `json:"notionalUsd"`
	Details     []BalanceDetail `json:"details"`
}

// WSPositionData represents position push data
type WSPositionData struct {
	InstID      string    `json:"instId"`
	InstType    string    `json:"instType"`
	PosID       string    `json:"posId"`
	PosSide     string    `json:"posSide"`
	Pos         string    `json:"pos"`
	AvailPos    string    `json:"availPos"`
	AvgPx       string    `json:"avgPx"`
	MarkPx      string    `json:"markPx"`
	Upl         string    `json:"upl"`
	UplRatio    string    `json:"uplRatio"`
	Lever       string    `json:"lever"`
	MgnMode     string    `json:"mgnMode"`
	LiqPx       string    `json:"liqPx"`
	NotionalUsd string    `json:"notionalUsd"`
	ADL         string    `json:"adl"`
	RealizedPnl string    `json:"realizedPnl"`
	Fee         string    `json:"fee"`
	FundingFee  string    `json:"fundingFee"`
	CTime       Timestamp `json:"cTime"`
	UTime       Timestamp `json:"uTime"`
	PTime       Timestamp `json:"pTime"`
}

// WSOrderData represents order push data
type WSOrderData struct {
	InstID          string    `json:"instId"`
	InstType        string    `json:"instType"`
	OrderID         string    `json:"ordId"`
	ClOrdID         string    `json:"clOrdId"`
	Tag             string    `json:"tag"`
	Px              string    `json:"px"`
	Sz              string    `json:"sz"`
	NotionalUsd     string    `json:"notionalUsd"`
	OrdType         string    `json:"ordType"`
	Side            string    `json:"side"`
	PosSide         string    `json:"posSide"`
	TdMode          string    `json:"tdMode"`
	FillPx          string    `json:"fillPx"`
	FillSz          string    `json:"fillSz"`
	FillTime        Timestamp `json:"fillTime"`
	AccFillSz       string    `json:"accFillSz"`
	AvgPx           string    `json:"avgPx"`
	State           string    `json:"state"`
	Lever           string    `json:"lever"`
	Fee             string    `json:"fee"`
	FeeCcy          string    `json:"feeCcy"`
	Pnl             string    `json:"pnl"`
	Rebate          string    `json:"rebate"`
	RebateCcy       string    `json:"rebateCcy"`
	ExecType        string    `json:"execType"` // T=Taker, M=Maker
	TradeID         string    `json:"tradeId"`
	FillNotionalUsd string    `json:"fillNotionalUsd"`
	CTime           Timestamp `json:"cTime"`
	UTime           Timestamp `json:"uTime"`
}

// WSBalanceAndPositionData represents combined balance and position push
type WSBalanceAndPositionData struct {
	PTime     Timestamp               `json:"pTime"`
	EventType string                  `json:"eventType"` // snapshot, filled, liquidation, etc.
	BalData   []WSBalanceData         `json:"balData"`
	PosData   []WSPositionSummaryData `json:"posData"`
}

// WSBalanceData represents balance data in combined push
type WSBalanceData struct {
	Ccy     string    `json:"ccy"`
	CashBal string    `json:"cashBal"`
	UTime   Timestamp `json:"uTime"`
}

// WSPositionSummaryData represents position summary in combined push
type WSPositionSummaryData struct {
	PosID    string `json:"posId"`
	InstID   string `json:"instId"`
	InstType string `json:"instType"`
	MgnMode  string `json:"mgnMode"`
	PosSide  string `json:"posSide"`
	Pos      string `json:"pos"`
	AvgPx    string `json:"avgPx"`
	Ccy      string `json:"ccy"`
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
	return "OKX API error " + e.Code + ": " + e.Message
}

// Common error codes
const (
	ErrCodeSuccess             = "0"
	ErrCodeBodyEmpty           = "50000"
	ErrCodeServiceUnavailable  = "50001"
	ErrCodeParamError          = "51000"
	ErrCodeInstNotExist        = "51001"
	ErrCodeOrderSizeTooSmall   = "51004"
	ErrCodeOrderPlaceFailed    = "51008"
	ErrCodeAccountFrozen       = "51010"
	ErrCodeOrderCountExceed    = "51020"
	ErrCodePriceOutOfRange     = "51121"
	ErrCodeInsufficientBalance = "51119"
	ErrCodePositionNotExist    = "51603"
	ErrCodeReduceOnlyFailed    = "51004"
)

// IsSuccess checks if the response code indicates success
func IsSuccess(code string) bool {
	return code == ErrCodeSuccess
}
