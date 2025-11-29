// Package kucoin provides trading operations for KuCoin Futures.
// Note: KuCoin Futures trading is REST-based, not WebSocket-based.
// This file provides a convenience wrapper around REST trading operations
// for consistent API pattern across exchanges.
package kucoin

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TradingHandler handles trading callbacks
type TradingHandler struct {
	OnOrderPlaced   func(order *OrderResponse, err error)
	OnOrderCanceled func(result *CancelResponse, err error)
	OnBatchOrders   func(orders []*BatchOrderResponse, err error)
	OnError         func(err error)
}

// TradingClient handles trading operations for KuCoin
// KuCoin uses REST API for order placement, unlike some exchanges that use WebSocket
type TradingClient struct {
	restClient *RESTClient
	handler    *TradingHandler
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewTradingClient creates a new trading client
func NewTradingClient(restClient *RESTClient, handler *TradingHandler) *TradingClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &TradingClient{
		restClient: restClient,
		handler:    handler,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// PlaceOrder places a single order
func (c *TradingClient) PlaceOrder(ctx context.Context, req *OrderRequest) (*OrderResponse, error) {
	// Validate required fields
	if req.ClientOid == "" {
		req.ClientOid = GenerateClientOid()
	}
	if req.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if req.Side == "" {
		return nil, fmt.Errorf("side is required")
	}
	if req.Type == "" {
		return nil, fmt.Errorf("type is required")
	}
	if req.Size <= 0 {
		return nil, fmt.Errorf("size must be positive")
	}
	if req.Type == OrderTypeLimit && req.Price == "" {
		return nil, fmt.Errorf("price is required for limit orders")
	}

	resp, err := c.restClient.PlaceOrder(ctx, req)

	if c.handler != nil && c.handler.OnOrderPlaced != nil {
		c.handler.OnOrderPlaced(resp, err)
	}

	return resp, err
}

// PlaceLimitOrder places a limit order with simplified parameters
func (c *TradingClient) PlaceLimitOrder(ctx context.Context, symbol, side string, size int, price string, opts ...OrderOption) (*OrderResponse, error) {
	req := &OrderRequest{
		ClientOid: GenerateClientOid(),
		Symbol:    symbol,
		Side:      side,
		Type:      OrderTypeLimit,
		Size:      size,
		Price:     price,
	}

	for _, opt := range opts {
		opt(req)
	}

	return c.PlaceOrder(ctx, req)
}

// PlaceMarketOrder places a market order with simplified parameters
func (c *TradingClient) PlaceMarketOrder(ctx context.Context, symbol, side string, size int, opts ...OrderOption) (*OrderResponse, error) {
	req := &OrderRequest{
		ClientOid: GenerateClientOid(),
		Symbol:    symbol,
		Side:      side,
		Type:      OrderTypeMarket,
		Size:      size,
	}

	for _, opt := range opts {
		opt(req)
	}

	return c.PlaceOrder(ctx, req)
}

// OrderOption defines functional option for order placement
type OrderOption func(*OrderRequest)

// WithLeverage sets leverage for the order
func WithLeverage(leverage int) OrderOption {
	return func(req *OrderRequest) {
		req.Leverage = leverage
	}
}

// WithMarginMode sets margin mode (ISOLATED or CROSS)
func WithMarginMode(mode string) OrderOption {
	return func(req *OrderRequest) {
		req.MarginMode = mode
	}
}

// WithPositionSide sets position side (BOTH, LONG, SHORT)
func WithPositionSide(side string) OrderOption {
	return func(req *OrderRequest) {
		req.PositionSide = side
	}
}

// WithTimeInForce sets time in force (GTC, IOC, FOK)
func WithTimeInForce(tif string) OrderOption {
	return func(req *OrderRequest) {
		req.TimeInForce = tif
	}
}

// WithReduceOnly sets reduce only flag
func WithReduceOnly(reduceOnly bool) OrderOption {
	return func(req *OrderRequest) {
		req.ReduceOnly = reduceOnly
	}
}

// WithPostOnly sets post only flag
func WithPostOnly(postOnly bool) OrderOption {
	return func(req *OrderRequest) {
		req.PostOnly = postOnly
	}
}

// WithClientOid sets custom client order ID
func WithClientOid(clientOid string) OrderOption {
	return func(req *OrderRequest) {
		req.ClientOid = clientOid
	}
}

// WithStop sets stop order parameters
func WithStop(stop, stopPrice, stopPriceType string) OrderOption {
	return func(req *OrderRequest) {
		req.Stop = stop
		req.StopPrice = stopPrice
		req.StopPriceType = stopPriceType
	}
}

// WithRemark sets order remark
func WithRemark(remark string) OrderOption {
	return func(req *OrderRequest) {
		req.Remark = remark
	}
}

// PlaceBatchOrders places multiple orders at once (max 10)
func (c *TradingClient) PlaceBatchOrders(ctx context.Context, orders []*OrderRequest) ([]*BatchOrderResponse, error) {
	if len(orders) == 0 {
		return nil, fmt.Errorf("at least one order is required")
	}
	if len(orders) > 10 {
		return nil, fmt.Errorf("maximum 10 orders per batch")
	}

	// Validate and set defaults for each order
	for _, req := range orders {
		if req.ClientOid == "" {
			req.ClientOid = GenerateClientOid()
		}
		if req.Symbol == "" {
			return nil, fmt.Errorf("symbol is required for all orders")
		}
		if req.Side == "" {
			return nil, fmt.Errorf("side is required for all orders")
		}
		if req.Type == "" {
			return nil, fmt.Errorf("type is required for all orders")
		}
	}

	resp, err := c.restClient.PlaceBatchOrders(ctx, orders)

	if c.handler != nil && c.handler.OnBatchOrders != nil {
		c.handler.OnBatchOrders(resp, err)
	}

	return resp, err
}

// CancelOrder cancels an order by order ID
func (c *TradingClient) CancelOrder(ctx context.Context, orderID string) (*CancelResponse, error) {
	resp, err := c.restClient.CancelOrder(ctx, orderID)

	if c.handler != nil && c.handler.OnOrderCanceled != nil {
		c.handler.OnOrderCanceled(resp, err)
	}

	return resp, err
}

// CancelOrderByClientOid cancels an order by client order ID
func (c *TradingClient) CancelOrderByClientOid(ctx context.Context, clientOid, symbol string) (*CancelResponse, error) {
	resp, err := c.restClient.CancelOrderByClientOid(ctx, clientOid, symbol)

	if c.handler != nil && c.handler.OnOrderCanceled != nil {
		c.handler.OnOrderCanceled(resp, err)
	}

	return resp, err
}

// CancelAllOrders cancels all orders, optionally filtered by symbol
func (c *TradingClient) CancelAllOrders(ctx context.Context, symbol string) (*CancelResponse, error) {
	resp, err := c.restClient.CancelAllOrders(ctx, symbol)

	if c.handler != nil && c.handler.OnOrderCanceled != nil {
		c.handler.OnOrderCanceled(resp, err)
	}

	return resp, err
}

// GetOrder fetches order details by order ID
func (c *TradingClient) GetOrder(ctx context.Context, orderID string) (*Order, error) {
	return c.restClient.GetOrder(ctx, orderID)
}

// GetOrderByClientOid fetches order by client order ID
func (c *TradingClient) GetOrderByClientOid(ctx context.Context, clientOid string) (*Order, error) {
	return c.restClient.GetOrderByClientOid(ctx, clientOid)
}

// GetOrders fetches order list with pagination
func (c *TradingClient) GetOrders(ctx context.Context, symbol, status string, pageSize, currentPage int) (*OrderList, error) {
	return c.restClient.GetOrders(ctx, symbol, status, pageSize, currentPage)
}

// GetActiveOrders fetches all active/open orders
func (c *TradingClient) GetActiveOrders(ctx context.Context, symbol string) (*OrderList, error) {
	return c.restClient.GetOrders(ctx, symbol, OrderStatusActive, 100, 1)
}

// GetFills fetches fill/trade history
func (c *TradingClient) GetFills(ctx context.Context, symbol, orderID string, pageSize, currentPage int) (*FillList, error) {
	return c.restClient.GetFills(ctx, symbol, orderID, pageSize, currentPage)
}

// GetRecentFills fetches recent fills (last 24 hours)
func (c *TradingClient) GetRecentFills(ctx context.Context, symbol string) ([]*Fill, error) {
	return c.restClient.GetRecentFills(ctx, symbol)
}

// =============================================================================
// Stop Order Operations
// =============================================================================

// PlaceStopOrder places a stop order
func (c *TradingClient) PlaceStopOrder(ctx context.Context, req *OrderRequest) (*OrderResponse, error) {
	if req.ClientOid == "" {
		req.ClientOid = GenerateClientOid()
	}
	if req.Stop == "" || req.StopPrice == "" {
		return nil, fmt.Errorf("stop and stopPrice are required for stop orders")
	}

	return c.restClient.PlaceStopOrder(ctx, req)
}

// GetStopOrders fetches stop order list
func (c *TradingClient) GetStopOrders(ctx context.Context, symbol string, pageSize, currentPage int) (*OrderList, error) {
	return c.restClient.GetStopOrders(ctx, symbol, pageSize, currentPage)
}

// CancelStopOrder cancels a stop order
func (c *TradingClient) CancelStopOrder(ctx context.Context, orderID string) (*CancelResponse, error) {
	return c.restClient.CancelStopOrder(ctx, orderID)
}

// CancelAllStopOrders cancels all stop orders
func (c *TradingClient) CancelAllStopOrders(ctx context.Context, symbol string) (*CancelResponse, error) {
	return c.restClient.CancelAllStopOrders(ctx, symbol)
}

// =============================================================================
// Position Operations
// =============================================================================

// GetPositions fetches all positions
func (c *TradingClient) GetPositions(ctx context.Context, currency string) ([]*Position, error) {
	return c.restClient.GetPositions(ctx, currency)
}

// GetPosition fetches a single position
func (c *TradingClient) GetPosition(ctx context.Context, symbol string) (*Position, error) {
	return c.restClient.GetPosition(ctx, symbol)
}

// SetAutoDepositMargin enables/disables auto-deposit margin
func (c *TradingClient) SetAutoDepositMargin(ctx context.Context, symbol string, status bool) (*Position, error) {
	return c.restClient.SetAutoDepositMargin(ctx, symbol, status)
}

// AddPositionMargin adds margin to a position
func (c *TradingClient) AddPositionMargin(ctx context.Context, symbol string, margin float64, bizNo string) (*Position, error) {
	if bizNo == "" {
		bizNo = GenerateClientOid()
	}
	return c.restClient.AddPositionMargin(ctx, symbol, margin, bizNo)
}

// =============================================================================
// Leverage & Margin Mode Operations
// =============================================================================

// ChangeLeverage changes leverage for cross margin
func (c *TradingClient) ChangeLeverage(ctx context.Context, symbol string, leverage int) error {
	return c.restClient.ChangeLeverage(ctx, symbol, leverage)
}

// ChangeMarginMode changes margin mode (ISOLATED or CROSS)
func (c *TradingClient) ChangeMarginMode(ctx context.Context, symbol, marginMode string) error {
	return c.restClient.ChangeMarginMode(ctx, symbol, marginMode)
}

// GetMaxOpenSize fetches maximum open size
func (c *TradingClient) GetMaxOpenSize(ctx context.Context, symbol string, price float64, leverage int) (map[string]interface{}, error) {
	return c.restClient.GetMaxOpenSize(ctx, symbol, price, leverage)
}

// =============================================================================
// Account Operations
// =============================================================================

// GetAccount fetches account overview
func (c *TradingClient) GetAccount(ctx context.Context, currency string) (*Account, error) {
	return c.restClient.GetAccount(ctx, currency)
}

// GetFundingHistory fetches funding fee history
func (c *TradingClient) GetFundingHistory(ctx context.Context, symbol string, startAt, endAt int64, pageSize, currentPage int) (map[string]interface{}, error) {
	return c.restClient.GetFundingHistory(ctx, symbol, startAt, endAt, pageSize, currentPage)
}

// =============================================================================
// Sliced Order Execution
// =============================================================================

// SlicedOrderConfig configures sliced order execution
type SlicedOrderConfig struct {
	Symbol        string
	Side          string
	TotalSize     int           // Total size in lots
	SliceSize     int           // Size per slice
	SliceInterval time.Duration // Interval between slices
	PriceOffset   float64       // Price offset from best bid/ask (percentage)
	MaxSlices     int           // Maximum number of slices (0 = unlimited)
	TimeInForce   string        // TIF for each slice
	Leverage      int
	MarginMode    string
	PositionSide  string
}

// SlicedOrderResult contains results of sliced order execution
type SlicedOrderResult struct {
	TotalOrders  int
	FilledOrders int
	TotalFilled  int
	OrderIDs     []string
	ClientOids   []string
	Errors       []error
}

// ExecuteSlicedOrder executes an order in multiple slices
// This is useful for reducing market impact when executing large orders
func (c *TradingClient) ExecuteSlicedOrder(ctx context.Context, cfg SlicedOrderConfig, priceGetter func() (string, error)) (*SlicedOrderResult, error) {
	if cfg.TotalSize <= 0 {
		return nil, fmt.Errorf("total size must be positive")
	}
	if cfg.SliceSize <= 0 {
		cfg.SliceSize = cfg.TotalSize // Single order if no slice size
	}
	if cfg.SliceInterval == 0 {
		cfg.SliceInterval = 100 * time.Millisecond
	}
	if cfg.TimeInForce == "" {
		cfg.TimeInForce = TIFGoodTillCancel
	}

	result := &SlicedOrderResult{
		OrderIDs:   make([]string, 0),
		ClientOids: make([]string, 0),
		Errors:     make([]error, 0),
	}

	remainingSize := cfg.TotalSize
	sliceNum := 0

	for remainingSize > 0 {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Check max slices
		if cfg.MaxSlices > 0 && sliceNum >= cfg.MaxSlices {
			break
		}

		// Calculate slice size
		currentSliceSize := cfg.SliceSize
		if currentSliceSize > remainingSize {
			currentSliceSize = remainingSize
		}

		// Get current price
		price, err := priceGetter()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("slice %d: failed to get price: %w", sliceNum, err))
			continue
		}

		// Place slice order
		req := &OrderRequest{
			ClientOid:    GenerateClientOid(),
			Symbol:       cfg.Symbol,
			Side:         cfg.Side,
			Type:         OrderTypeLimit,
			Size:         currentSliceSize,
			Price:        price,
			TimeInForce:  cfg.TimeInForce,
			Leverage:     cfg.Leverage,
			MarginMode:   cfg.MarginMode,
			PositionSide: cfg.PositionSide,
		}

		resp, err := c.PlaceOrder(ctx, req)
		result.TotalOrders++

		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("slice %d: %w", sliceNum, err))
		} else {
			result.FilledOrders++
			result.TotalFilled += currentSliceSize
			result.OrderIDs = append(result.OrderIDs, resp.OrderID)
			result.ClientOids = append(result.ClientOids, resp.ClientOid)
			remainingSize -= currentSliceSize
		}

		sliceNum++

		// Wait before next slice
		if remainingSize > 0 && cfg.SliceInterval > 0 {
			time.Sleep(cfg.SliceInterval)
		}
	}

	return result, nil
}

// Close closes the trading client
func (c *TradingClient) Close() error {
	c.cancel()
	return nil
}
