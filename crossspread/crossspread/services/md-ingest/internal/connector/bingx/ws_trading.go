// Package bingx provides trading client for BingX Perpetual Futures.
// This wraps REST API trading operations with callback support.
package bingx

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// TradingHandler handles trading operation callbacks
type TradingHandler struct {
	OnOrderPlaced   func(order *OrderResponse)
	OnOrderCanceled func(cancel *CancelResponse)
	OnOrderFilled   func(order *Order)
	OnError         func(err error)
}

// TradingClient wraps REST trading operations with callbacks
type TradingClient struct {
	restClient *RESTClient
	handler    *TradingHandler
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex

	// Order tracking
	pendingOrders map[int64]*OrderRequest // orderID -> original request
	ordersMu      sync.RWMutex
}

// NewTradingClient creates a new trading client
func NewTradingClient(restClient *RESTClient, handler *TradingHandler) *TradingClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &TradingClient{
		restClient:    restClient,
		handler:       handler,
		ctx:           ctx,
		cancel:        cancel,
		pendingOrders: make(map[int64]*OrderRequest),
	}
}

// =============================================================================
// Order Placement
// =============================================================================

// PlaceOrder places a new order
func (c *TradingClient) PlaceOrder(ctx context.Context, req *OrderRequest) (*OrderResponse, error) {
	resp, err := c.restClient.PlaceOrder(ctx, req)
	if err != nil {
		if c.handler != nil && c.handler.OnError != nil {
			c.handler.OnError(fmt.Errorf("place order failed: %w", err))
		}
		return nil, err
	}

	// Track pending order
	c.ordersMu.Lock()
	c.pendingOrders[resp.OrderID] = req
	c.ordersMu.Unlock()

	if c.handler != nil && c.handler.OnOrderPlaced != nil {
		c.handler.OnOrderPlaced(resp)
	}

	log.Printf("[BingX Trading] Order placed: %d %s %s %s", resp.OrderID, req.Symbol, req.Side, req.Type)
	return resp, nil
}

// PlaceLimitOrder is a convenience method for placing limit orders
func (c *TradingClient) PlaceLimitOrder(ctx context.Context, symbol, side, positionSide string, price, quantity float64, timeInForce string) (*OrderResponse, error) {
	req := &OrderRequest{
		Symbol:       symbol,
		Type:         OrderTypeLimit,
		Side:         side,
		PositionSide: positionSide,
		Price:        price,
		Quantity:     quantity,
		TimeInForce:  timeInForce,
	}
	return c.PlaceOrder(ctx, req)
}

// PlaceMarketOrder is a convenience method for placing market orders
func (c *TradingClient) PlaceMarketOrder(ctx context.Context, symbol, side, positionSide string, quantity float64) (*OrderResponse, error) {
	req := &OrderRequest{
		Symbol:       symbol,
		Type:         OrderTypeMarket,
		Side:         side,
		PositionSide: positionSide,
		Quantity:     quantity,
	}
	return c.PlaceOrder(ctx, req)
}

// PlaceStopOrder is a convenience method for placing stop orders
func (c *TradingClient) PlaceStopOrder(ctx context.Context, symbol, side, positionSide string, stopPrice, quantity float64, orderType string) (*OrderResponse, error) {
	req := &OrderRequest{
		Symbol:       symbol,
		Type:         orderType, // STOP_MARKET, TAKE_PROFIT_MARKET, STOP, TAKE_PROFIT
		Side:         side,
		PositionSide: positionSide,
		StopPrice:    stopPrice,
		Quantity:     quantity,
	}
	return c.PlaceOrder(ctx, req)
}

// PlaceBatchOrders places multiple orders
func (c *TradingClient) PlaceBatchOrders(ctx context.Context, orders []*OrderRequest) ([]*OrderResponse, error) {
	resp, err := c.restClient.PlaceBatchOrders(ctx, orders)
	if err != nil {
		if c.handler != nil && c.handler.OnError != nil {
			c.handler.OnError(fmt.Errorf("batch order failed: %w", err))
		}
		return nil, err
	}

	// Track and notify for each order
	for i, r := range resp {
		if r != nil && r.OrderID > 0 {
			c.ordersMu.Lock()
			c.pendingOrders[r.OrderID] = orders[i]
			c.ordersMu.Unlock()

			if c.handler != nil && c.handler.OnOrderPlaced != nil {
				c.handler.OnOrderPlaced(r)
			}
		}
	}

	return resp, nil
}

// =============================================================================
// Order Cancellation
// =============================================================================

// CancelOrder cancels an existing order
func (c *TradingClient) CancelOrder(ctx context.Context, symbol string, orderID int64) (*CancelResponse, error) {
	resp, err := c.restClient.CancelOrder(ctx, symbol, orderID, "")
	if err != nil {
		if c.handler != nil && c.handler.OnError != nil {
			c.handler.OnError(fmt.Errorf("cancel order failed: %w", err))
		}
		return nil, err
	}

	// Remove from tracking
	c.ordersMu.Lock()
	delete(c.pendingOrders, orderID)
	c.ordersMu.Unlock()

	if c.handler != nil && c.handler.OnOrderCanceled != nil {
		c.handler.OnOrderCanceled(resp)
	}

	log.Printf("[BingX Trading] Order canceled: %d %s", orderID, symbol)
	return resp, nil
}

// CancelOrderByClientID cancels an order by client order ID
func (c *TradingClient) CancelOrderByClientID(ctx context.Context, symbol, clientOrderID string) (*CancelResponse, error) {
	resp, err := c.restClient.CancelOrder(ctx, symbol, 0, clientOrderID)
	if err != nil {
		if c.handler != nil && c.handler.OnError != nil {
			c.handler.OnError(fmt.Errorf("cancel order failed: %w", err))
		}
		return nil, err
	}

	// Remove from tracking
	c.ordersMu.Lock()
	delete(c.pendingOrders, resp.OrderID)
	c.ordersMu.Unlock()

	if c.handler != nil && c.handler.OnOrderCanceled != nil {
		c.handler.OnOrderCanceled(resp)
	}

	return resp, nil
}

// CancelBatchOrders cancels multiple orders
func (c *TradingClient) CancelBatchOrders(ctx context.Context, symbol string, orderIDs []int64) ([]*CancelResponse, error) {
	resp, err := c.restClient.CancelBatchOrders(ctx, symbol, orderIDs, nil)
	if err != nil {
		if c.handler != nil && c.handler.OnError != nil {
			c.handler.OnError(fmt.Errorf("batch cancel failed: %w", err))
		}
		return nil, err
	}

	// Remove from tracking and notify
	c.ordersMu.Lock()
	for _, id := range orderIDs {
		delete(c.pendingOrders, id)
	}
	c.ordersMu.Unlock()

	if c.handler != nil && c.handler.OnOrderCanceled != nil {
		for _, r := range resp {
			c.handler.OnOrderCanceled(r)
		}
	}

	return resp, nil
}

// CancelAllOrders cancels all open orders for a symbol
func (c *TradingClient) CancelAllOrders(ctx context.Context, symbol string) error {
	err := c.restClient.CancelAllOrders(ctx, symbol)
	if err != nil {
		if c.handler != nil && c.handler.OnError != nil {
			c.handler.OnError(fmt.Errorf("cancel all orders failed: %w", err))
		}
		return err
	}

	// Clear tracking for this symbol
	c.ordersMu.Lock()
	for id, req := range c.pendingOrders {
		if req.Symbol == symbol {
			delete(c.pendingOrders, id)
		}
	}
	c.ordersMu.Unlock()

	log.Printf("[BingX Trading] All orders canceled for %s", symbol)
	return nil
}

// =============================================================================
// Position Management
// =============================================================================

// CloseAllPositions closes all open positions
func (c *TradingClient) CloseAllPositions(ctx context.Context) error {
	err := c.restClient.CloseAllPositions(ctx)
	if err != nil {
		if c.handler != nil && c.handler.OnError != nil {
			c.handler.OnError(fmt.Errorf("close all positions failed: %w", err))
		}
		return err
	}

	// Clear all pending orders
	c.ordersMu.Lock()
	c.pendingOrders = make(map[int64]*OrderRequest)
	c.ordersMu.Unlock()

	log.Printf("[BingX Trading] All positions closed")
	return nil
}

// ClosePosition closes a specific position by placing a market order
func (c *TradingClient) ClosePosition(ctx context.Context, position *Position) (*OrderResponse, error) {
	// Determine close order parameters
	var side string
	if position.PositionSide == PositionSideLong {
		side = OrderSideSell
	} else {
		side = OrderSideBuy
	}

	// Parse position amount
	var quantity float64
	fmt.Sscanf(position.PositionAmt, "%f", &quantity)
	if quantity < 0 {
		quantity = -quantity
	}

	return c.PlaceMarketOrder(ctx, position.Symbol, side, position.PositionSide, quantity)
}

// =============================================================================
// Order Query
// =============================================================================

// GetOrder queries an order by ID
func (c *TradingClient) GetOrder(ctx context.Context, symbol string, orderID int64) (*Order, error) {
	return c.restClient.QueryOrder(ctx, symbol, orderID, "")
}

// GetOrderByClientID queries an order by client order ID
func (c *TradingClient) GetOrderByClientID(ctx context.Context, symbol, clientOrderID string) (*Order, error) {
	return c.restClient.QueryOrder(ctx, symbol, 0, clientOrderID)
}

// GetOpenOrders fetches all open orders
func (c *TradingClient) GetOpenOrders(ctx context.Context, symbol string) ([]*Order, error) {
	return c.restClient.GetOpenOrders(ctx, symbol)
}

// GetAllOrders fetches order history
func (c *TradingClient) GetAllOrders(ctx context.Context, symbol string, startTime, endTime int64, limit int) ([]*Order, error) {
	return c.restClient.GetAllOrders(ctx, symbol, 0, startTime, endTime, limit)
}

// GetFills fetches trade fill history
func (c *TradingClient) GetFills(ctx context.Context, symbol string, startTime, endTime int64) ([]*Fill, error) {
	return c.restClient.GetAllFills(ctx, symbol, 0, startTime, endTime)
}

// =============================================================================
// Leverage & Margin
// =============================================================================

// SetLeverage sets leverage for a symbol and side
func (c *TradingClient) SetLeverage(ctx context.Context, symbol string, leverage int) error {
	// Set both long and short leverage
	if err := c.restClient.SetLeverage(ctx, symbol, PositionSideLong, leverage); err != nil {
		return fmt.Errorf("failed to set long leverage: %w", err)
	}
	if err := c.restClient.SetLeverage(ctx, symbol, PositionSideShort, leverage); err != nil {
		return fmt.Errorf("failed to set short leverage: %w", err)
	}
	log.Printf("[BingX Trading] Leverage set to %dx for %s", leverage, symbol)
	return nil
}

// SetMarginMode sets margin mode for a symbol
func (c *TradingClient) SetMarginMode(ctx context.Context, symbol, marginMode string) error {
	err := c.restClient.SetMarginType(ctx, symbol, marginMode)
	if err != nil {
		return fmt.Errorf("failed to set margin mode: %w", err)
	}
	log.Printf("[BingX Trading] Margin mode set to %s for %s", marginMode, symbol)
	return nil
}

// =============================================================================
// Slicing Order Execution
// =============================================================================

// SlicedOrderConfig holds configuration for sliced order execution
type SlicedOrderConfig struct {
	Symbol        string
	Side          string
	PositionSide  string
	TotalQuantity float64
	SliceSize     float64       // Quantity per slice
	SliceInterval time.Duration // Interval between slices
	PriceOffset   float64       // Price offset from current price (for limit orders)
	UseMarket     bool          // Use market orders instead of limit
	MaxSlices     int           // Maximum number of slices (0 = unlimited)
}

// SlicedOrderResult holds the result of sliced order execution
type SlicedOrderResult struct {
	TotalQuantity     float64
	FilledQuantity    float64
	RemainingQuantity float64
	OrderIDs          []int64
	Errors            []error
	StartTime         time.Time
	EndTime           time.Time
}

// ExecuteSlicedOrder executes an order in multiple slices
func (c *TradingClient) ExecuteSlicedOrder(ctx context.Context, config *SlicedOrderConfig) (*SlicedOrderResult, error) {
	result := &SlicedOrderResult{
		TotalQuantity: config.TotalQuantity,
		StartTime:     time.Now(),
		OrderIDs:      make([]int64, 0),
		Errors:        make([]error, 0),
	}

	remaining := config.TotalQuantity
	sliceCount := 0

	for remaining > 0 {
		select {
		case <-ctx.Done():
			result.RemainingQuantity = remaining
			result.EndTime = time.Now()
			return result, ctx.Err()
		default:
		}

		// Check max slices
		if config.MaxSlices > 0 && sliceCount >= config.MaxSlices {
			break
		}

		// Calculate slice quantity
		quantity := config.SliceSize
		if quantity > remaining {
			quantity = remaining
		}

		// Place order
		var resp *OrderResponse
		var err error

		if config.UseMarket {
			resp, err = c.PlaceMarketOrder(ctx, config.Symbol, config.Side, config.PositionSide, quantity)
		} else {
			// For limit orders, we need current price
			// This should be obtained from market data client
			// For now, we use market orders
			resp, err = c.PlaceMarketOrder(ctx, config.Symbol, config.Side, config.PositionSide, quantity)
		}

		if err != nil {
			result.Errors = append(result.Errors, err)
			log.Printf("[BingX Trading] Slice %d failed: %v", sliceCount+1, err)
		} else {
			result.OrderIDs = append(result.OrderIDs, resp.OrderID)
			result.FilledQuantity += quantity
			remaining -= quantity
			log.Printf("[BingX Trading] Slice %d placed: %d, qty=%f", sliceCount+1, resp.OrderID, quantity)
		}

		sliceCount++

		// Wait between slices
		if remaining > 0 && config.SliceInterval > 0 {
			time.Sleep(config.SliceInterval)
		}
	}

	result.RemainingQuantity = remaining
	result.EndTime = time.Now()

	return result, nil
}

// =============================================================================
// Pending Order Management
// =============================================================================

// GetPendingOrderCount returns the number of tracked pending orders
func (c *TradingClient) GetPendingOrderCount() int {
	c.ordersMu.RLock()
	defer c.ordersMu.RUnlock()
	return len(c.pendingOrders)
}

// GetPendingOrders returns a copy of pending orders
func (c *TradingClient) GetPendingOrders() map[int64]*OrderRequest {
	c.ordersMu.RLock()
	defer c.ordersMu.RUnlock()

	result := make(map[int64]*OrderRequest, len(c.pendingOrders))
	for k, v := range c.pendingOrders {
		result[k] = v
	}
	return result
}

// ClearPendingOrder removes an order from tracking
func (c *TradingClient) ClearPendingOrder(orderID int64) {
	c.ordersMu.Lock()
	defer c.ordersMu.Unlock()
	delete(c.pendingOrders, orderID)
}

// =============================================================================
// Lifecycle
// =============================================================================

// Close closes the trading client
func (c *TradingClient) Close() error {
	c.cancel()
	return nil
}
