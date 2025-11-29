package connector

import (
	"context"
	"time"
)

// ExchangeID represents supported exchange identifiers
type ExchangeID string

const (
	Binance ExchangeID = "binance"
	Bybit   ExchangeID = "bybit"
	OKX     ExchangeID = "okx"
	KuCoin  ExchangeID = "kucoin"
	MEXC    ExchangeID = "mexc"
	Bitget  ExchangeID = "bitget"
	GateIO  ExchangeID = "gateio"
	BingX   ExchangeID = "bingx"
	CoinEx  ExchangeID = "coinex"
	LBank   ExchangeID = "lbank"
	HTX     ExchangeID = "htx"
)

// PriceLevel represents a single level in the orderbook
type PriceLevel struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
}

// Orderbook represents an L2 orderbook snapshot or update
type Orderbook struct {
	ExchangeID ExchangeID   `json:"exchange_id"`
	Symbol     string       `json:"symbol"`    // Exchange-native symbol
	Canonical  string       `json:"canonical"` // Normalized symbol
	Bids       []PriceLevel `json:"bids"`      // Sorted desc by price
	Asks       []PriceLevel `json:"asks"`      // Sorted asc by price
	BestBid    float64      `json:"best_bid"`
	BestAsk    float64      `json:"best_ask"`
	SpreadBps  float64      `json:"spread_bps"`
	Timestamp  time.Time    `json:"timestamp"`
	SequenceID int64        `json:"sequence_id,omitempty"`
	IsSnapshot bool         `json:"is_snapshot"`
}

// Trade represents a single trade event
type Trade struct {
	ExchangeID ExchangeID `json:"exchange_id"`
	Symbol     string     `json:"symbol"`
	Canonical  string     `json:"canonical"`
	TradeID    string     `json:"trade_id"`
	Price      float64    `json:"price"`
	Quantity   float64    `json:"quantity"`
	Side       string     `json:"side"` // "buy" or "sell"
	Timestamp  time.Time  `json:"timestamp"`
}

// FundingRate represents funding rate info for perpetuals
type FundingRate struct {
	ExchangeID           ExchangeID `json:"exchange_id"`
	Symbol               string     `json:"symbol"`
	Canonical            string     `json:"canonical"`
	FundingRate          float64    `json:"funding_rate"`
	NextFundingTime      time.Time  `json:"next_funding_time"`
	FundingIntervalHours int        `json:"funding_interval_hours"`
	Timestamp            time.Time  `json:"timestamp"`
}

// Instrument represents a tradeable instrument
type Instrument struct {
	ExchangeID     ExchangeID `json:"exchange_id"`
	Symbol         string     `json:"symbol"`
	Canonical      string     `json:"canonical"`
	BaseAsset      string     `json:"base_asset"`
	QuoteAsset     string     `json:"quote_asset"`
	InstrumentType string     `json:"instrument_type"` // perpetual, future, spot
	ContractSize   float64    `json:"contract_size"`
	TickSize       float64    `json:"tick_size"`
	LotSize        float64    `json:"lot_size"`
	MinNotional    float64    `json:"min_notional"`
	MakerFee       float64    `json:"maker_fee"`
	TakerFee       float64    `json:"taker_fee"`
}

// PriceTicker represents current price info for a symbol (REST API response)
type PriceTicker struct {
	ExchangeID ExchangeID `json:"exchange_id"`
	Symbol     string     `json:"symbol"`
	Canonical  string     `json:"canonical"`
	Price      float64    `json:"price"`
	BidPrice   float64    `json:"bid_price,omitempty"`
	AskPrice   float64    `json:"ask_price,omitempty"`
	Volume24h  float64    `json:"volume_24h,omitempty"`
	Timestamp  time.Time  `json:"timestamp"`
}

// AssetInfo represents deposit/withdrawal status and network info
type AssetInfo struct {
	ExchangeID      ExchangeID `json:"exchange_id"`
	Asset           string     `json:"asset"`
	DepositEnabled  bool       `json:"deposit_enabled"`
	WithdrawEnabled bool       `json:"withdraw_enabled"`
	WithdrawFee     float64    `json:"withdraw_fee,omitempty"`
	MinWithdraw     float64    `json:"min_withdraw,omitempty"`
	Networks        []string   `json:"networks,omitempty"`
	Timestamp       time.Time  `json:"timestamp"`
}

// TickerWithMetadata combines price ticker with additional info for spread discovery
type TickerWithMetadata struct {
	Ticker      PriceTicker  `json:"ticker"`
	Instrument  *Instrument  `json:"instrument,omitempty"`
	FundingRate *FundingRate `json:"funding_rate,omitempty"`
	AssetInfo   *AssetInfo   `json:"asset_info,omitempty"`
}

// ConnectorConfig holds configuration for an exchange connector
type ConnectorConfig struct {
	ExchangeID     ExchangeID
	WsURL          string
	RestURL        string
	Symbols        []string // Symbols to subscribe to
	DepthLevels    int      // Number of orderbook levels to request
	ReconnectDelay time.Duration
	PingInterval   time.Duration
}

// OrderbookHandler is called when orderbook updates are received
type OrderbookHandler func(ob *Orderbook)

// TradeHandler is called when trades are received
type TradeHandler func(trade *Trade)

// FundingHandler is called when funding rates are updated
type FundingHandler func(fr *FundingRate)

// ErrorHandler is called when errors occur
type ErrorHandler func(err error)

// Connector defines the interface for exchange market data connectors
type Connector interface {
	// ID returns the exchange identifier
	ID() ExchangeID

	// Connect establishes WebSocket connection
	Connect(ctx context.Context) error

	// ConnectForSymbols establishes WebSocket connection for specific symbols only
	// Used for Phase 2 selective subscription based on discovered spreads
	ConnectForSymbols(ctx context.Context, symbols []string) error

	// Disconnect closes the connection
	Disconnect() error

	// Subscribe subscribes to orderbook updates for symbols
	Subscribe(symbols []string) error

	// Unsubscribe removes subscriptions
	Unsubscribe(symbols []string) error

	// FetchInstruments fetches all available instruments from REST API
	FetchInstruments(ctx context.Context) ([]Instrument, error)

	// FetchOrderbookSnapshot fetches current orderbook via REST (for resync)
	FetchOrderbookSnapshot(ctx context.Context, symbol string, depth int) (*Orderbook, error)

	// FetchFundingRates fetches current funding rates
	FetchFundingRates(ctx context.Context) ([]FundingRate, error)

	// FetchPriceTickers fetches current prices for all symbols (Phase 1 REST)
	FetchPriceTickers(ctx context.Context) ([]PriceTicker, error)

	// FetchAssetInfo fetches deposit/withdrawal status for assets (Phase 1 REST)
	FetchAssetInfo(ctx context.Context) ([]AssetInfo, error)

	// SetOrderbookHandler sets the callback for orderbook updates
	SetOrderbookHandler(handler OrderbookHandler)

	// SetTradeHandler sets the callback for trade updates
	SetTradeHandler(handler TradeHandler)

	// SetFundingHandler sets the callback for funding rate updates
	SetFundingHandler(handler FundingHandler)

	// SetErrorHandler sets the callback for errors
	SetErrorHandler(handler ErrorHandler)

	// IsConnected returns true if WebSocket is connected
	IsConnected() bool

	// LastMessageTime returns timestamp of last received message
	LastMessageTime() time.Time
}

// BaseConnector provides common functionality for connectors
type BaseConnector struct {
	config           ConnectorConfig
	orderbookHandler OrderbookHandler
	tradeHandler     TradeHandler
	fundingHandler   FundingHandler
	errorHandler     ErrorHandler
	connected        bool
	lastMessageTime  time.Time
}

// NewBaseConnector creates a new base connector
func NewBaseConnector(config ConnectorConfig) *BaseConnector {
	return &BaseConnector{
		config: config,
	}
}

// ID returns the exchange ID
func (c *BaseConnector) ID() ExchangeID {
	return c.config.ExchangeID
}

// SetOrderbookHandler sets the orderbook handler
func (c *BaseConnector) SetOrderbookHandler(handler OrderbookHandler) {
	c.orderbookHandler = handler
}

// SetTradeHandler sets the trade handler
func (c *BaseConnector) SetTradeHandler(handler TradeHandler) {
	c.tradeHandler = handler
}

// SetFundingHandler sets the funding handler
func (c *BaseConnector) SetFundingHandler(handler FundingHandler) {
	c.fundingHandler = handler
}

// SetErrorHandler sets the error handler
func (c *BaseConnector) SetErrorHandler(handler ErrorHandler) {
	c.errorHandler = handler
}

// IsConnected returns connection status
func (c *BaseConnector) IsConnected() bool {
	return c.connected
}

// LastMessageTime returns the last message timestamp
func (c *BaseConnector) LastMessageTime() time.Time {
	return c.lastMessageTime
}

// EmitOrderbook sends orderbook to handler
func (c *BaseConnector) EmitOrderbook(ob *Orderbook) {
	c.lastMessageTime = time.Now()
	if c.orderbookHandler != nil {
		c.orderbookHandler(ob)
	}
}

// EmitTrade sends trade to handler
func (c *BaseConnector) EmitTrade(trade *Trade) {
	c.lastMessageTime = time.Now()
	if c.tradeHandler != nil {
		c.tradeHandler(trade)
	}
}

// EmitFunding sends funding rate to handler
func (c *BaseConnector) EmitFunding(fr *FundingRate) {
	c.lastMessageTime = time.Now()
	if c.fundingHandler != nil {
		c.fundingHandler(fr)
	}
}

// EmitError sends error to handler
func (c *BaseConnector) EmitError(err error) {
	if c.errorHandler != nil {
		c.errorHandler(err)
	}
}

// SetConnected updates connection status
func (c *BaseConnector) SetConnected(connected bool) {
	c.connected = connected
}
