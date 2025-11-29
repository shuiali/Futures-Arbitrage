package bybit

import (
	"fmt"
	"time"
)

// =============================================================================
// REST API Response Types
// =============================================================================

// BaseResponse is the common wrapper for all Bybit V5 API responses
type BaseResponse struct {
	RetCode    int    `json:"retCode"`
	RetMsg     string `json:"retMsg"`
	Time       int64  `json:"time"`
	RetExtInfo struct {
		List []ExtInfo `json:"list"`
	} `json:"retExtInfo"`
}

type ExtInfo struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// =============================================================================
// Market Data Types
// =============================================================================

// InstrumentsInfoResponse represents GET /v5/market/instruments-info response
type InstrumentsInfoResponse struct {
	BaseResponse
	Result struct {
		Category       string           `json:"category"`
		List           []InstrumentInfo `json:"list"`
		NextPageCursor string           `json:"nextPageCursor"`
	} `json:"result"`
}

type InstrumentInfo struct {
	Symbol          string `json:"symbol"`
	ContractType    string `json:"contractType"` // LinearPerpetual, LinearFutures, InversePerpetual, InverseFutures
	Status          string `json:"status"`       // Trading, Settling, Closed
	BaseCoin        string `json:"baseCoin"`
	QuoteCoin       string `json:"quoteCoin"`
	SettleCoin      string `json:"settleCoin"`
	LaunchTime      string `json:"launchTime"`
	DeliveryTime    string `json:"deliveryTime"`
	DeliveryFeeRate string `json:"deliveryFeeRate"`
	PriceScale      string `json:"priceScale"`
	LeverageFilter  struct {
		MinLeverage  string `json:"minLeverage"`
		MaxLeverage  string `json:"maxLeverage"`
		LeverageStep string `json:"leverageStep"`
	} `json:"leverageFilter"`
	PriceFilter struct {
		MinPrice string `json:"minPrice"`
		MaxPrice string `json:"maxPrice"`
		TickSize string `json:"tickSize"`
	} `json:"priceFilter"`
	LotSizeFilter struct {
		MaxOrderQty         string `json:"maxOrderQty"`
		MaxMktOrderQty      string `json:"maxMktOrderQty"`
		MinOrderQty         string `json:"minOrderQty"`
		QtyStep             string `json:"qtyStep"`
		PostOnlyMaxOrderQty string `json:"postOnlyMaxOrderQty"`
		MinNotionalValue    string `json:"minNotionalValue"`
	} `json:"lotSizeFilter"`
	UnifiedMarginTrade bool   `json:"unifiedMarginTrade"`
	FundingInterval    int    `json:"fundingInterval"` // Funding interval in minutes
	CopyTrading        string `json:"copyTrading"`
	UpperFundingRate   string `json:"upperFundingRate"`
	LowerFundingRate   string `json:"lowerFundingRate"`
	IsPreListing       bool   `json:"isPreListing"`
	PreListingInfo     struct {
		CurAuctionPhase    string `json:"curAuctionPhase"`
		AuctionEndTime     string `json:"auctionEndTime"`
		AuctionFeeInfo     struct{}
		EstimatedSpotPrice string `json:"estimatedSpotPrice"`
	} `json:"preListingInfo"`
}

// TickersResponse represents GET /v5/market/tickers response
type TickersResponse struct {
	BaseResponse
	Result struct {
		Category string       `json:"category"`
		List     []TickerInfo `json:"list"`
	} `json:"result"`
}

type TickerInfo struct {
	Symbol                 string `json:"symbol"`
	LastPrice              string `json:"lastPrice"`
	IndexPrice             string `json:"indexPrice"`
	MarkPrice              string `json:"markPrice"`
	PrevPrice24h           string `json:"prevPrice24h"`
	Price24hPcnt           string `json:"price24hPcnt"`
	HighPrice24h           string `json:"highPrice24h"`
	LowPrice24h            string `json:"lowPrice24h"`
	PrevPrice1h            string `json:"prevPrice1h"`
	OpenInterest           string `json:"openInterest"`
	OpenInterestValue      string `json:"openInterestValue"`
	Turnover24h            string `json:"turnover24h"`
	Volume24h              string `json:"volume24h"`
	FundingRate            string `json:"fundingRate"`
	NextFundingTime        string `json:"nextFundingTime"`
	PredictedDeliveryPrice string `json:"predictedDeliveryPrice"`
	BasisRate              string `json:"basisRate"`
	Basis                  string `json:"basis"`
	DeliveryFeeRate        string `json:"deliveryFeeRate"`
	DeliveryTime           string `json:"deliveryTime"`
	Ask1Size               string `json:"ask1Size"`
	Bid1Price              string `json:"bid1Price"`
	Ask1Price              string `json:"ask1Price"`
	Bid1Size               string `json:"bid1Size"`
	PreOpenPrice           string `json:"preOpenPrice"`
	PreQty                 string `json:"preQty"`
	CurPreListingPhase     string `json:"curPreListingPhase"`
}

// OrderbookResponse represents GET /v5/market/orderbook response
type OrderbookResponse struct {
	BaseResponse
	Result struct {
		Symbol    string     `json:"s"`
		Bids      [][]string `json:"b"` // [price, size]
		Asks      [][]string `json:"a"` // [price, size]
		Timestamp int64      `json:"ts"`
		UpdateID  int64      `json:"u"`
		Seq       int64      `json:"seq"`
		Cts       int64      `json:"cts"` // Matching engine timestamp
	} `json:"result"`
}

// KlineResponse represents GET /v5/market/kline response
type KlineResponse struct {
	BaseResponse
	Result struct {
		Symbol   string     `json:"symbol"`
		Category string     `json:"category"`
		List     [][]string `json:"list"` // [startTime, open, high, low, close, volume, turnover]
	} `json:"result"`
}

// Kline represents a single candlestick
type Kline struct {
	StartTime int64
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	Turnover  float64
}

// FundingHistoryResponse represents GET /v5/market/funding/history response
type FundingHistoryResponse struct {
	BaseResponse
	Result struct {
		Category       string               `json:"category"`
		List           []FundingHistoryItem `json:"list"`
		NextPageCursor string               `json:"nextPageCursor"`
	} `json:"result"`
}

type FundingHistoryItem struct {
	Symbol               string `json:"symbol"`
	FundingRate          string `json:"fundingRate"`
	FundingRateTimestamp string `json:"fundingRateTimestamp"`
}

// RecentTradesResponse represents GET /v5/market/recent-trade response
type RecentTradesResponse struct {
	BaseResponse
	Result struct {
		Category string        `json:"category"`
		List     []PublicTrade `json:"list"`
	} `json:"result"`
}

type PublicTrade struct {
	ExecId       string `json:"execId"`
	Symbol       string `json:"symbol"`
	Price        string `json:"price"`
	Size         string `json:"size"`
	Side         string `json:"side"` // Buy, Sell
	Time         string `json:"time"`
	IsBlockTrade bool   `json:"isBlockTrade"`
}

// OpenInterestResponse represents GET /v5/market/open-interest response
type OpenInterestResponse struct {
	BaseResponse
	Result struct {
		Category       string             `json:"category"`
		Symbol         string             `json:"symbol"`
		List           []OpenInterestItem `json:"list"`
		NextPageCursor string             `json:"nextPageCursor"`
	} `json:"result"`
}

type OpenInterestItem struct {
	OpenInterest string `json:"openInterest"`
	Timestamp    string `json:"timestamp"`
}

// RiskLimitResponse represents GET /v5/market/risk-limit response
type RiskLimitResponse struct {
	BaseResponse
	Result struct {
		Category string          `json:"category"`
		List     []RiskLimitItem `json:"list"`
	} `json:"result"`
}

type RiskLimitItem struct {
	ID                int    `json:"id"`
	Symbol            string `json:"symbol"`
	RiskLimitValue    string `json:"riskLimitValue"`
	MaintenanceMargin string `json:"maintenanceMargin"`
	InitialMargin     string `json:"initialMargin"`
	MaxLeverage       string `json:"maxLeverage"`
}

// =============================================================================
// Trading Types
// =============================================================================

// CreateOrderRequest represents the request body for POST /v5/order/create
type CreateOrderRequest struct {
	Category         string `json:"category"`
	Symbol           string `json:"symbol"`
	Side             string `json:"side"`      // Buy, Sell
	OrderType        string `json:"orderType"` // Market, Limit
	Qty              string `json:"qty"`
	Price            string `json:"price,omitempty"`
	TimeInForce      string `json:"timeInForce,omitempty"` // GTC, IOC, FOK, PostOnly
	PositionIdx      int    `json:"positionIdx,omitempty"` // 0=one-way, 1=hedge-buy, 2=hedge-sell
	OrderLinkId      string `json:"orderLinkId,omitempty"`
	ReduceOnly       bool   `json:"reduceOnly,omitempty"`
	CloseOnTrigger   bool   `json:"closeOnTrigger,omitempty"`
	TriggerPrice     string `json:"triggerPrice,omitempty"`
	TriggerDirection int    `json:"triggerDirection,omitempty"` // 1=rise, 2=fall
	TriggerBy        string `json:"triggerBy,omitempty"`        // LastPrice, IndexPrice, MarkPrice
	TakeProfit       string `json:"takeProfit,omitempty"`
	StopLoss         string `json:"stopLoss,omitempty"`
	TpslMode         string `json:"tpslMode,omitempty"` // Full, Partial
	TpOrderType      string `json:"tpOrderType,omitempty"`
	SlOrderType      string `json:"slOrderType,omitempty"`
	TpLimitPrice     string `json:"tpLimitPrice,omitempty"`
	SlLimitPrice     string `json:"slLimitPrice,omitempty"`
}

// CreateOrderResponse represents the response from POST /v5/order/create
type CreateOrderResponse struct {
	BaseResponse
	Result struct {
		OrderID     string `json:"orderId"`
		OrderLinkId string `json:"orderLinkId"`
	} `json:"result"`
}

// AmendOrderRequest represents the request body for POST /v5/order/amend
type AmendOrderRequest struct {
	Category     string `json:"category"`
	Symbol       string `json:"symbol"`
	OrderID      string `json:"orderId,omitempty"`
	OrderLinkId  string `json:"orderLinkId,omitempty"`
	Qty          string `json:"qty,omitempty"`
	Price        string `json:"price,omitempty"`
	TriggerPrice string `json:"triggerPrice,omitempty"`
	TakeProfit   string `json:"takeProfit,omitempty"`
	StopLoss     string `json:"stopLoss,omitempty"`
}

// AmendOrderResponse represents the response from POST /v5/order/amend
type AmendOrderResponse struct {
	BaseResponse
	Result struct {
		OrderID     string `json:"orderId"`
		OrderLinkId string `json:"orderLinkId"`
	} `json:"result"`
}

// CancelOrderRequest represents the request body for POST /v5/order/cancel
type CancelOrderRequest struct {
	Category    string `json:"category"`
	Symbol      string `json:"symbol"`
	OrderID     string `json:"orderId,omitempty"`
	OrderLinkId string `json:"orderLinkId,omitempty"`
	OrderFilter string `json:"orderFilter,omitempty"` // Order, tpslOrder, StopOrder
}

// CancelOrderResponse represents the response from POST /v5/order/cancel
type CancelOrderResponse struct {
	BaseResponse
	Result struct {
		OrderID     string `json:"orderId"`
		OrderLinkId string `json:"orderLinkId"`
	} `json:"result"`
}

// CancelAllOrdersRequest represents the request body for POST /v5/order/cancel-all
type CancelAllOrdersRequest struct {
	Category    string `json:"category"`
	Symbol      string `json:"symbol,omitempty"`
	BaseCoin    string `json:"baseCoin,omitempty"`
	SettleCoin  string `json:"settleCoin,omitempty"`
	OrderFilter string `json:"orderFilter,omitempty"`
}

// CancelAllOrdersResponse represents the response from POST /v5/order/cancel-all
type CancelAllOrdersResponse struct {
	BaseResponse
	Result struct {
		List []struct {
			OrderID     string `json:"orderId"`
			OrderLinkId string `json:"orderLinkId"`
		} `json:"list"`
	} `json:"result"`
}

// BatchOrderRequest represents the request body for POST /v5/order/create-batch
type BatchOrderRequest struct {
	Category string               `json:"category"`
	Request  []CreateOrderRequest `json:"request"`
}

// BatchOrderResponse represents the response from POST /v5/order/create-batch
type BatchOrderResponse struct {
	BaseResponse
	Result struct {
		List []struct {
			Category    string `json:"category"`
			Symbol      string `json:"symbol"`
			OrderID     string `json:"orderId"`
			OrderLinkId string `json:"orderLinkId"`
			CreateAt    string `json:"createAt"`
		} `json:"list"`
	} `json:"result"`
}

// GetOrdersResponse represents the response from GET /v5/order/realtime
type GetOrdersResponse struct {
	BaseResponse
	Result struct {
		Category       string      `json:"category"`
		List           []OrderInfo `json:"list"`
		NextPageCursor string      `json:"nextPageCursor"`
	} `json:"result"`
}

// OrderInfo represents order details
type OrderInfo struct {
	OrderID            string `json:"orderId"`
	OrderLinkId        string `json:"orderLinkId"`
	BlockTradeId       string `json:"blockTradeId"`
	Symbol             string `json:"symbol"`
	Price              string `json:"price"`
	Qty                string `json:"qty"`
	Side               string `json:"side"`
	IsLeverage         string `json:"isLeverage"`
	PositionIdx        int    `json:"positionIdx"`
	OrderStatus        string `json:"orderStatus"`
	CancelType         string `json:"cancelType"`
	RejectReason       string `json:"rejectReason"`
	AvgPrice           string `json:"avgPrice"`
	LeavesQty          string `json:"leavesQty"`
	LeavesValue        string `json:"leavesValue"`
	CumExecQty         string `json:"cumExecQty"`
	CumExecValue       string `json:"cumExecValue"`
	CumExecFee         string `json:"cumExecFee"`
	TimeInForce        string `json:"timeInForce"`
	OrderType          string `json:"orderType"`
	StopOrderType      string `json:"stopOrderType"`
	OrderIv            string `json:"orderIv"`
	TriggerPrice       string `json:"triggerPrice"`
	TakeProfit         string `json:"takeProfit"`
	StopLoss           string `json:"stopLoss"`
	TpslMode           string `json:"tpslMode"`
	TpLimitPrice       string `json:"tpLimitPrice"`
	SlLimitPrice       string `json:"slLimitPrice"`
	TpTriggerBy        string `json:"tpTriggerBy"`
	SlTriggerBy        string `json:"slTriggerBy"`
	TriggerDirection   int    `json:"triggerDirection"`
	TriggerBy          string `json:"triggerBy"`
	LastPriceOnCreated string `json:"lastPriceOnCreated"`
	ReduceOnly         bool   `json:"reduceOnly"`
	CloseOnTrigger     bool   `json:"closeOnTrigger"`
	PlaceType          string `json:"placeType"`
	SmpType            string `json:"smpType"`
	SmpGroup           int    `json:"smpGroup"`
	SmpOrderId         string `json:"smpOrderId"`
	CreatedTime        string `json:"createdTime"`
	UpdatedTime        string `json:"updatedTime"`
}

// GetExecutionsResponse represents the response from GET /v5/execution/list
type GetExecutionsResponse struct {
	BaseResponse
	Result struct {
		Category       string          `json:"category"`
		List           []ExecutionInfo `json:"list"`
		NextPageCursor string          `json:"nextPageCursor"`
	} `json:"result"`
}

// ExecutionInfo represents execution details
type ExecutionInfo struct {
	Symbol          string `json:"symbol"`
	OrderID         string `json:"orderId"`
	OrderLinkId     string `json:"orderLinkId"`
	Side            string `json:"side"`
	OrderPrice      string `json:"orderPrice"`
	OrderQty        string `json:"orderQty"`
	LeavesQty       string `json:"leavesQty"`
	OrderType       string `json:"orderType"`
	StopOrderType   string `json:"stopOrderType"`
	ExecFee         string `json:"execFee"`
	ExecId          string `json:"execId"`
	ExecPrice       string `json:"execPrice"`
	ExecQty         string `json:"execQty"`
	ExecType        string `json:"execType"`
	ExecValue       string `json:"execValue"`
	ExecTime        string `json:"execTime"`
	IsMaker         bool   `json:"isMaker"`
	FeeRate         string `json:"feeRate"`
	TradeIv         string `json:"tradeIv"`
	MarkIv          string `json:"markIv"`
	MarkPrice       string `json:"markPrice"`
	IndexPrice      string `json:"indexPrice"`
	UnderlyingPrice string `json:"underlyingPrice"`
	BlockTradeId    string `json:"blockTradeId"`
	ClosedSize      string `json:"closedSize"`
	Seq             int64  `json:"seq"`
}

// =============================================================================
// Position Types
// =============================================================================

// GetPositionsResponse represents the response from GET /v5/position/list
type GetPositionsResponse struct {
	BaseResponse
	Result struct {
		Category       string         `json:"category"`
		List           []PositionInfo `json:"list"`
		NextPageCursor string         `json:"nextPageCursor"`
	} `json:"result"`
}

// PositionInfo represents position details
type PositionInfo struct {
	PositionIdx      int    `json:"positionIdx"`
	Symbol           string `json:"symbol"`
	Side             string `json:"side"` // Buy (long), Sell (short), "" (empty)
	Size             string `json:"size"`
	AvgPrice         string `json:"avgPrice"`
	PositionValue    string `json:"positionValue"`
	TradeMode        int    `json:"tradeMode"`
	PositionStatus   string `json:"positionStatus"` // Normal, Liq, Adl
	AutoAddMargin    int    `json:"autoAddMargin"`
	AdlRankIndicator int    `json:"adlRankIndicator"`
	Leverage         string `json:"leverage"`
	PositionBalance  string `json:"positionBalance"`
	MarkPrice        string `json:"markPrice"`
	LiqPrice         string `json:"liqPrice"`
	BustPrice        string `json:"bustPrice"`
	PositionIM       string `json:"positionIM"`
	PositionMM       string `json:"positionMM"`
	TpslMode         string `json:"tpslMode"`
	TakeProfit       string `json:"takeProfit"`
	StopLoss         string `json:"stopLoss"`
	TrailingStop     string `json:"trailingStop"`
	UnrealisedPnl    string `json:"unrealisedPnl"`
	CurRealisedPnl   string `json:"curRealisedPnl"`
	CumRealisedPnl   string `json:"cumRealisedPnl"`
	SessionAvgPrice  string `json:"sessionAvgPrice"`
	Delta            string `json:"delta"`
	Gamma            string `json:"gamma"`
	Vega             string `json:"vega"`
	Theta            string `json:"theta"`
	CreatedTime      string `json:"createdTime"`
	UpdatedTime      string `json:"updatedTime"`
	Seq              int64  `json:"seq"`
}

// SetLeverageRequest represents the request body for POST /v5/position/set-leverage
type SetLeverageRequest struct {
	Category     string `json:"category"`
	Symbol       string `json:"symbol"`
	BuyLeverage  string `json:"buyLeverage"`
	SellLeverage string `json:"sellLeverage"`
}

// SetLeverageResponse represents the response from POST /v5/position/set-leverage
type SetLeverageResponse struct {
	BaseResponse
	Result struct{} `json:"result"`
}

// SwitchPositionModeRequest represents the request body for POST /v5/position/switch-mode
type SwitchPositionModeRequest struct {
	Category string `json:"category"`
	Symbol   string `json:"symbol,omitempty"`
	Coin     string `json:"coin,omitempty"`
	Mode     int    `json:"mode"` // 0=one-way, 3=hedge mode
}

// GetClosedPnlResponse represents the response from GET /v5/position/closed-pnl
type GetClosedPnlResponse struct {
	BaseResponse
	Result struct {
		Category       string          `json:"category"`
		List           []ClosedPnlItem `json:"list"`
		NextPageCursor string          `json:"nextPageCursor"`
	} `json:"result"`
}

type ClosedPnlItem struct {
	Symbol        string `json:"symbol"`
	OrderID       string `json:"orderId"`
	Side          string `json:"side"`
	Qty           string `json:"qty"`
	OrderPrice    string `json:"orderPrice"`
	OrderType     string `json:"orderType"`
	ExecType      string `json:"execType"`
	ClosedSize    string `json:"closedSize"`
	CumEntryValue string `json:"cumEntryValue"`
	AvgEntryPrice string `json:"avgEntryPrice"`
	CumExitValue  string `json:"cumExitValue"`
	AvgExitPrice  string `json:"avgExitPrice"`
	ClosedPnl     string `json:"closedPnl"`
	FillCount     string `json:"fillCount"`
	Leverage      string `json:"leverage"`
	CreatedTime   string `json:"createdTime"`
	UpdatedTime   string `json:"updatedTime"`
}

// =============================================================================
// Account Types
// =============================================================================

// GetWalletBalanceResponse represents the response from GET /v5/account/wallet-balance
type GetWalletBalanceResponse struct {
	BaseResponse
	Result struct {
		List []WalletInfo `json:"list"`
	} `json:"result"`
}

type WalletInfo struct {
	AccountType            string     `json:"accountType"`
	AccountLTV             string     `json:"accountLTV"`
	AccountIMRate          string     `json:"accountIMRate"`
	AccountMMRate          string     `json:"accountMMRate"`
	TotalEquity            string     `json:"totalEquity"`
	TotalWalletBalance     string     `json:"totalWalletBalance"`
	TotalMarginBalance     string     `json:"totalMarginBalance"`
	TotalAvailableBalance  string     `json:"totalAvailableBalance"`
	TotalPerpUPL           string     `json:"totalPerpUPL"`
	TotalInitialMargin     string     `json:"totalInitialMargin"`
	TotalMaintenanceMargin string     `json:"totalMaintenanceMargin"`
	Coin                   []CoinInfo `json:"coin"`
}

type CoinInfo struct {
	Coin                string `json:"coin"`
	Equity              string `json:"equity"`
	UsdValue            string `json:"usdValue"`
	WalletBalance       string `json:"walletBalance"`
	Free                string `json:"free"`
	Locked              string `json:"locked"`
	BorrowAmount        string `json:"borrowAmount"`
	AvailableToWithdraw string `json:"availableToWithdraw"`
	AccruedInterest     string `json:"accruedInterest"`
	TotalOrderIM        string `json:"totalOrderIM"`
	TotalPositionIM     string `json:"totalPositionIM"`
	TotalPositionMM     string `json:"totalPositionMM"`
	UnrealisedPnl       string `json:"unrealisedPnl"`
	CumRealisedPnl      string `json:"cumRealisedPnl"`
	Bonus               string `json:"bonus"`
	MarginCollateral    bool   `json:"marginCollateral"`
	CollateralSwitch    bool   `json:"collateralSwitch"`
}

// GetFeeRateResponse represents the response from GET /v5/account/fee-rate
type GetFeeRateResponse struct {
	BaseResponse
	Result struct {
		List []FeeRateInfo `json:"list"`
	} `json:"result"`
}

type FeeRateInfo struct {
	Symbol       string `json:"symbol"`
	BaseCoin     string `json:"baseCoin"`
	TakerFeeRate string `json:"takerFeeRate"`
	MakerFeeRate string `json:"makerFeeRate"`
}

// GetAccountInfoResponse represents the response from GET /v5/account/info
type GetAccountInfoResponse struct {
	BaseResponse
	Result struct {
		UnifiedMarginStatus int    `json:"unifiedMarginStatus"`
		MarginMode          string `json:"marginMode"` // ISOLATED_MARGIN, REGULAR_MARGIN, PORTFOLIO_MARGIN
		SpotHedgingStatus   string `json:"spotHedgingStatus"`
		DcpStatus           string `json:"dcpStatus"`
		TimeWindow          int    `json:"timeWindow"`
		SmpGroup            int    `json:"smpGroup"`
		IsMasterTrader      bool   `json:"isMasterTrader"`
		UpdatedTime         string `json:"updatedTime"`
	} `json:"result"`
}

// =============================================================================
// Asset Types
// =============================================================================

// GetCoinInfoResponse represents the response from GET /v5/asset/coin/query-info
type GetCoinInfoResponse struct {
	BaseResponse
	Result struct {
		Rows []CoinAssetInfo `json:"rows"`
	} `json:"result"`
}

type CoinAssetInfo struct {
	Name         string      `json:"name"`
	Coin         string      `json:"coin"`
	RemainAmount string      `json:"remainAmount"`
	Chains       []ChainInfo `json:"chains"`
}

type ChainInfo struct {
	Chain                 string `json:"chain"`
	ChainType             string `json:"chainType"`
	Confirmation          string `json:"confirmation"`
	WithdrawFee           string `json:"withdrawFee"`
	DepositMin            string `json:"depositMin"`
	WithdrawMin           string `json:"withdrawMin"`
	MinAccuracy           string `json:"minAccuracy"`
	ChainDeposit          string `json:"chainDeposit"`  // 0=suspend, 1=normal
	ChainWithdraw         string `json:"chainWithdraw"` // 0=suspend, 1=normal
	WithdrawPercentageFee string `json:"withdrawPercentageFee"`
	ContractAddress       string `json:"contractAddress"`
}

// GetDepositRecordsResponse represents the response from GET /v5/asset/deposit/query-record
type GetDepositRecordsResponse struct {
	BaseResponse
	Result struct {
		Rows           []DepositRecord `json:"rows"`
		NextPageCursor string          `json:"nextPageCursor"`
	} `json:"result"`
}

type DepositRecord struct {
	Coin              string `json:"coin"`
	Chain             string `json:"chain"`
	Amount            string `json:"amount"`
	TxID              string `json:"txID"`
	Status            int    `json:"status"`
	ToAddress         string `json:"toAddress"`
	Tag               string `json:"tag"`
	DepositFee        string `json:"depositFee"`
	SuccessAt         string `json:"successAt"`
	Confirmations     string `json:"confirmations"`
	TxIndex           string `json:"txIndex"`
	BlockHash         string `json:"blockHash"`
	BatchReleaseLimit string `json:"batchReleaseLimit"`
	DepositType       int    `json:"depositType"`
}

// GetWithdrawRecordsResponse represents the response from GET /v5/asset/withdraw/query-record
type GetWithdrawRecordsResponse struct {
	BaseResponse
	Result struct {
		Rows           []WithdrawRecord `json:"rows"`
		NextPageCursor string           `json:"nextPageCursor"`
	} `json:"result"`
}

type WithdrawRecord struct {
	WithdrawId   string `json:"withdrawId"`
	TxID         string `json:"txID"`
	WithdrawType int    `json:"withdrawType"`
	Coin         string `json:"coin"`
	Chain        string `json:"chain"`
	Amount       string `json:"amount"`
	WithdrawFee  string `json:"withdrawFee"`
	Status       string `json:"status"`
	ToAddress    string `json:"toAddress"`
	Tag          string `json:"tag"`
	CreateTime   string `json:"createTime"`
	UpdateTime   string `json:"updateTime"`
}

// =============================================================================
// WebSocket Types
// =============================================================================

// WSMessage represents a generic WebSocket message
type WSMessage struct {
	Topic string `json:"topic"`
	Type  string `json:"type"` // snapshot, delta
	Ts    int64  `json:"ts"`
	Data  any    `json:"data"`
}

// WSOperation represents WebSocket operation messages
type WSOperation struct {
	Op   string   `json:"op"`
	Args []string `json:"args,omitempty"`
}

// WSAuthOperation represents WebSocket authentication message
type WSAuthOperation struct {
	Op   string   `json:"op"`
	Args []string `json:"args"`
}

// WSResponse represents a WebSocket response
type WSResponse struct {
	Success bool   `json:"success"`
	RetMsg  string `json:"ret_msg"`
	Op      string `json:"op"`
	ConnId  string `json:"conn_id"`
}

// WSOrderbookData represents orderbook data from WebSocket
type WSOrderbookData struct {
	Symbol   string     `json:"s"`
	Bids     [][]string `json:"b"` // [price, size]
	Asks     [][]string `json:"a"` // [price, size]
	UpdateID int64      `json:"u"`
	Seq      int64      `json:"seq"`
}

// WSTickerData represents ticker data from WebSocket
type WSTickerData struct {
	Symbol            string `json:"symbol"`
	LastPrice         string `json:"lastPrice"`
	MarkPrice         string `json:"markPrice"`
	IndexPrice        string `json:"indexPrice"`
	Price24hPcnt      string `json:"price24hPcnt"`
	HighPrice24h      string `json:"highPrice24h"`
	LowPrice24h       string `json:"lowPrice24h"`
	Volume24h         string `json:"volume24h"`
	Turnover24h       string `json:"turnover24h"`
	OpenInterest      string `json:"openInterest"`
	OpenInterestValue string `json:"openInterestValue"`
	FundingRate       string `json:"fundingRate"`
	NextFundingTime   string `json:"nextFundingTime"`
	Bid1Price         string `json:"bid1Price"`
	Bid1Size          string `json:"bid1Size"`
	Ask1Price         string `json:"ask1Price"`
	Ask1Size          string `json:"ask1Size"`
}

// WSTradeData represents trade data from WebSocket
type WSTradeData struct {
	Timestamp    int64  `json:"T"`
	Symbol       string `json:"s"`
	Side         string `json:"S"` // Buy, Sell
	Size         string `json:"v"`
	Price        string `json:"p"`
	Direction    string `json:"L"`
	TradeID      string `json:"i"`
	IsBlockTrade bool   `json:"BT"`
}

// =============================================================================
// Private WebSocket Types
// =============================================================================

// WSOrderUpdate represents order update from private WebSocket
type WSOrderUpdate struct {
	Category     string `json:"category"`
	OrderID      string `json:"orderId"`
	OrderLinkId  string `json:"orderLinkId"`
	Symbol       string `json:"symbol"`
	Price        string `json:"price"`
	Qty          string `json:"qty"`
	Side         string `json:"side"`
	OrderStatus  string `json:"orderStatus"`
	AvgPrice     string `json:"avgPrice"`
	LeavesQty    string `json:"leavesQty"`
	CumExecQty   string `json:"cumExecQty"`
	CumExecValue string `json:"cumExecValue"`
	CumExecFee   string `json:"cumExecFee"`
	TimeInForce  string `json:"timeInForce"`
	OrderType    string `json:"orderType"`
	TriggerPrice string `json:"triggerPrice"`
	TakeProfit   string `json:"takeProfit"`
	StopLoss     string `json:"stopLoss"`
	ReduceOnly   bool   `json:"reduceOnly"`
	ClosedPnl    string `json:"closedPnl"`
	CreatedTime  string `json:"createdTime"`
	UpdatedTime  string `json:"updatedTime"`
}

// WSPositionUpdate represents position update from private WebSocket
type WSPositionUpdate struct {
	Category       string `json:"category"`
	Symbol         string `json:"symbol"`
	Side           string `json:"side"`
	Size           string `json:"size"`
	PositionIdx    int    `json:"positionIdx"`
	PositionValue  string `json:"positionValue"`
	EntryPrice     string `json:"entryPrice"`
	MarkPrice      string `json:"markPrice"`
	Leverage       string `json:"leverage"`
	PositionIM     string `json:"positionIM"`
	PositionMM     string `json:"positionMM"`
	LiqPrice       string `json:"liqPrice"`
	TakeProfit     string `json:"takeProfit"`
	StopLoss       string `json:"stopLoss"`
	UnrealisedPnl  string `json:"unrealisedPnl"`
	CurRealisedPnl string `json:"curRealisedPnl"`
	CumRealisedPnl string `json:"cumRealisedPnl"`
	PositionStatus string `json:"positionStatus"`
	UpdatedTime    string `json:"updatedTime"`
}

// WSExecutionUpdate represents execution update from private WebSocket
type WSExecutionUpdate struct {
	Category    string `json:"category"`
	Symbol      string `json:"symbol"`
	OrderID     string `json:"orderId"`
	OrderLinkId string `json:"orderLinkId"`
	Side        string `json:"side"`
	OrderPrice  string `json:"orderPrice"`
	OrderQty    string `json:"orderQty"`
	LeavesQty   string `json:"leavesQty"`
	OrderType   string `json:"orderType"`
	ExecFee     string `json:"execFee"`
	ExecId      string `json:"execId"`
	ExecPrice   string `json:"execPrice"`
	ExecQty     string `json:"execQty"`
	ExecType    string `json:"execType"`
	ExecValue   string `json:"execValue"`
	ExecTime    string `json:"execTime"`
	IsMaker     bool   `json:"isMaker"`
	FeeRate     string `json:"feeRate"`
	ClosedSize  string `json:"closedSize"`
	ExecPnl     string `json:"execPnl"`
}

// WSWalletUpdate represents wallet update from private WebSocket
type WSWalletUpdate struct {
	AccountType            string     `json:"accountType"`
	TotalEquity            string     `json:"totalEquity"`
	TotalWalletBalance     string     `json:"totalWalletBalance"`
	TotalMarginBalance     string     `json:"totalMarginBalance"`
	TotalAvailableBalance  string     `json:"totalAvailableBalance"`
	TotalPerpUPL           string     `json:"totalPerpUPL"`
	TotalInitialMargin     string     `json:"totalInitialMargin"`
	TotalMaintenanceMargin string     `json:"totalMaintenanceMargin"`
	AccountIMRate          string     `json:"accountIMRate"`
	AccountMMRate          string     `json:"accountMMRate"`
	Coin                   []CoinInfo `json:"coin"`
}

// =============================================================================
// WebSocket Trade API Types (Low Latency Order Entry)
// =============================================================================

// WSTradeRequest represents a WebSocket trade request
type WSTradeRequest struct {
	ReqId  string                   `json:"reqId"`
	Header map[string]string        `json:"header"`
	Op     string                   `json:"op"`
	Args   []map[string]interface{} `json:"args"`
}

// WSTradeResponse represents a WebSocket trade response
type WSTradeResponse struct {
	ReqId   string `json:"reqId"`
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Op      string `json:"op"`
	Data    struct {
		OrderID     string `json:"orderId"`
		OrderLinkId string `json:"orderLinkId"`
	} `json:"data"`
	Header struct {
		XBapiLimit               string `json:"X-Bapi-Limit"`
		XBapiLimitStatus         string `json:"X-Bapi-Limit-Status"`
		XBapiLimitResetTimestamp string `json:"X-Bapi-Limit-Reset-Timestamp"`
		Traceid                  string `json:"Traceid"`
		Timenow                  string `json:"Timenow"`
	} `json:"header"`
	ConnId string `json:"connId"`
}

// =============================================================================
// Helper Types
// =============================================================================

// OrderStatus represents order status enum
type OrderStatus string

const (
	OrderStatusNew             OrderStatus = "New"
	OrderStatusPartiallyFilled OrderStatus = "PartiallyFilled"
	OrderStatusFilled          OrderStatus = "Filled"
	OrderStatusCancelled       OrderStatus = "Cancelled"
	OrderStatusRejected        OrderStatus = "Rejected"
	OrderStatusPendingCancel   OrderStatus = "PendingCancel"
	OrderStatusUntriggered     OrderStatus = "Untriggered"
	OrderStatusTriggered       OrderStatus = "Triggered"
	OrderStatusDeactivated     OrderStatus = "Deactivated"
)

// OrderSide represents order side enum
type OrderSide string

const (
	OrderSideBuy  OrderSide = "Buy"
	OrderSideSell OrderSide = "Sell"
)

// OrderType represents order type enum
type OrderType string

const (
	OrderTypeMarket OrderType = "Market"
	OrderTypeLimit  OrderType = "Limit"
)

// TimeInForce represents time in force enum
type TimeInForce string

const (
	TimeInForceGTC      TimeInForce = "GTC"
	TimeInForceIOC      TimeInForce = "IOC"
	TimeInForceFOK      TimeInForce = "FOK"
	TimeInForcePostOnly TimeInForce = "PostOnly"
)

// Category represents product category enum
type Category string

const (
	CategoryLinear  Category = "linear"
	CategoryInverse Category = "inverse"
	CategorySpot    Category = "spot"
	CategoryOption  Category = "option"
)

// ExecutionType represents execution type enum
type ExecutionType string

const (
	ExecTypeTrade    ExecutionType = "Trade"
	ExecTypeAdlTrade ExecutionType = "AdlTrade"
	ExecTypeFunding  ExecutionType = "Funding"
	ExecTypeBust     ExecutionType = "BustTrade"
	ExecTypeSettle   ExecutionType = "Settle"
)

// ParsedKline converts raw kline data to Kline struct
func ParseKlineData(data []string) (*Kline, error) {
	if len(data) < 7 {
		return nil, nil
	}

	var startTime int64
	var open, high, low, close, volume, turnover float64

	fmt.Sscanf(data[0], "%d", &startTime)
	fmt.Sscanf(data[1], "%f", &open)
	fmt.Sscanf(data[2], "%f", &high)
	fmt.Sscanf(data[3], "%f", &low)
	fmt.Sscanf(data[4], "%f", &close)
	fmt.Sscanf(data[5], "%f", &volume)
	fmt.Sscanf(data[6], "%f", &turnover)

	return &Kline{
		StartTime: startTime,
		Open:      open,
		High:      high,
		Low:       low,
		Close:     close,
		Volume:    volume,
		Turnover:  turnover,
	}, nil
}

// Timestamp helper
func UnixMilliToTime(ms int64) time.Time {
	return time.UnixMilli(ms)
}
