# Binance Connector - Comprehensive Implementation

## Overview

This directory contains a complete Binance Futures integration with the following capabilities:

### Files Structure

| File | Description |
|------|-------------|
| `types.go` | All REST and WebSocket API response types |
| `rest_client.go` | REST API client for market data and account endpoints |
| `ws_market_data.go` | WebSocket streams for real-time market data |
| `ws_trading.go` | WebSocket API for low-latency trading |
| `ws_user_data.go` | User data stream for account and order updates |
| `client.go` | Unified client integrating all components |
| `binance.go` | Original connector implementing base interface |

---

## REST API Capabilities (`rest_client.go`)

### Market Data (Unauthenticated)
- `FetchExchangeInfo()` - All trading pairs, filters, and rules
- `Fetch24hrTickers()` - 24h price/volume statistics
- `FetchPremiumIndex()` - Mark price, funding rate, index price
- `FetchFundingRates()` - Historical funding rates
- `FetchOpenInterest()` - Open interest for a symbol
- `FetchDepth()` - Orderbook snapshot (5-1000 levels)
- `FetchKlines()` - Historical candlesticks (1m to 1M intervals)
- `FetchHistoricalPrices()` - Parsed klines as HistoricalPrice structs

### Account Data (Authenticated - SAPI)
- `FetchCapitalConfig()` - Deposit/withdrawal status per coin
- `FetchTradeFees()` - Maker/taker fees per symbol

### Futures Account (Authenticated - FAPI)
- `FetchFuturesAccount()` - Account info, balances, positions
- `FetchPositionRisk()` - Position details with liquidation prices

### Aggregated Data
- `FetchAllTokenData()` - Comprehensive token data combining:
  - Exchange info (trading rules)
  - 24hr tickers (price/volume)
  - Premium index (funding/mark price)
  - Capital config (deposit/withdraw status)
  - Trade fees

---

## WebSocket Streams (`ws_market_data.go`)

### Stream Types
- `depth@100ms` / `depth@250ms` / `depth@500ms` - Incremental orderbook updates
- `trade` - Real-time trade stream
- `markPrice` / `markPrice@1s` - Mark price updates (1s or 3s)
- `kline_<interval>` - Real-time candlestick updates

### Features
- Multi-symbol subscription via combined stream
- Automatic reconnection on disconnect
- Orderbook manager with sequence validation
- Callbacks for trade, depth, mark price, kline events

---

## WebSocket Trading API (`ws_trading.go`)

Low-latency trading via WebSocket (no REST roundtrip).

### Operations
- `PlaceOrder()` - Place limit/market/stop orders
- `CancelOrder()` - Cancel by order ID or client order ID
- `ModifyOrder()` - Modify price/quantity of existing order
- `QueryOrder()` - Query order status
- `CancelAllOrders()` - Cancel all orders for a symbol
- `GetAccount()` - Get account info via WS

### Order Types Supported
- `LIMIT` / `MARKET` / `STOP` / `STOP_MARKET`
- `TAKE_PROFIT` / `TAKE_PROFIT_MARKET`
- `TRAILING_STOP_MARKET`

### Time in Force
- `GTC` (Good Till Cancel)
- `IOC` (Immediate or Cancel)
- `FOK` (Fill or Kill)
- `GTX` (Post Only / Good Till Crossing)

---

## User Data Stream (`ws_user_data.go`)

Real-time account and order updates.

### Event Types
- `ACCOUNT_UPDATE` - Balance and position changes
- `ORDER_TRADE_UPDATE` - Order status and fills
- `MARGIN_CALL` - Margin call warnings
- `ACCOUNT_CONFIG_UPDATE` - Leverage/margin changes

### Features
- Automatic listen key management (create + keepalive every 30min)
- Position tracker with PnL calculations
- Balance tracker
- Order tracker with open/filled/canceled states

---

## Unified Client (`client.go`)

High-level API combining all components.

### Usage
```go
// Create client
config := &binance.ClientConfig{
    APIKey:    "your-api-key",
    SecretKey: "your-secret-key",
    IsLive:    true,
}
client := binance.NewClient(config)

// Connect to all streams
err := client.ConnectAll(ctx, []string{"BTCUSDT", "ETHUSDT"})

// Fetch comprehensive token data
tokens, _ := client.FetchAllTokenData(ctx)

// Get orderbook
ob := client.GetOrderbook("BTCUSDT")
bid, bidQty, ask, askQty := ob.GetBestBidAsk()

// Place an order (via WebSocket - low latency)
result, _ := client.PlaceLimitOrder(ctx, "BTCUSDT", "BUY", 50000, 0.001)

// Get real-time positions
positions := client.GetPositions()
pnl := client.GetTotalUnrealizedPnL()

// Historical spread calculation
spreads, _ := client.FetchHistoricalSpread(ctx, "BTCUSDT", "ETHUSDT", "1h", 7*24*time.Hour)

// Cleanup
client.DisconnectAll()
```

---

## Historical Spread Calculation

The client provides `FetchHistoricalSpread()` for calculating cross-exchange or cross-pair spreads:

```go
spreads, err := client.FetchHistoricalSpread(
    ctx,
    "BTCUSDT",      // Symbol 1
    "ETHUSDT",      // Symbol 2
    "1h",           // Interval
    7*24*time.Hour, // Lookback
)

for _, s := range spreads {
    fmt.Printf("%s: %.2f bps\n", s.Timestamp, s.SpreadBps)
}
```

---

## Rate Limits

The REST client respects Binance rate limits:
- Market data: 2400 request weight/min
- Orders: 300 orders/10s per symbol
- WebSocket: 10 messages/s for subscriptions

---

## Error Handling

All methods return errors with context:
```go
if err != nil {
    // API errors include code and message
    // Network errors are wrapped
    log.Error().Err(err).Msg("Operation failed")
}
```

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `BINANCE_API_KEY` | API key for authenticated endpoints |
| `BINANCE_SECRET_KEY` | Secret key for signing |
| `BINANCE_TESTNET` | Set to `true` for testnet |
