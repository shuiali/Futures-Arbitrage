# Bybit V5 API Connector

Comprehensive Bybit V5 API connector for the CrossSpread arbitrage system. This connector provides full access to Bybit's REST and WebSocket APIs for market data, trading, and account management.

## Features

### REST API (`rest_client.go`)
- **Market Data (Public)**
  - Get all instruments info (`GetInstruments`)
  - Get tickers with price, volume, funding rate (`GetTickers`)
  - Get orderbook depth (`GetOrderbook`)
  - Get kline/candlestick data (`GetKline`)
  - Get historical funding rates (`GetFundingHistory`)
  - Get recent public trades (`GetRecentTrades`)
  - Get open interest (`GetOpenInterest`)
  - Get risk limit info (`GetRiskLimit`)

- **Trading (Private - Authenticated)**
  - Create order (`CreateOrder`)
  - Amend order (`AmendOrder`)
  - Cancel order (`CancelOrder`)
  - Cancel all orders (`CancelAllOrders`)
  - Batch create orders (`BatchCreateOrders`)
  - Get open orders (`GetOpenOrders`)
  - Get order history (`GetOrderHistory`)
  - Get executions/fills (`GetExecutions`)

- **Positions (Private)**
  - Get positions (`GetPositions`)
  - Set leverage (`SetLeverage`)
  - Switch position mode (`SwitchPositionMode`)
  - Get closed PnL (`GetClosedPnl`)

- **Account (Private)**
  - Get wallet balance (`GetWalletBalance`)
  - Get fee rate (`GetFeeRate`)
  - Get account info (`GetAccountInfo`)

- **Assets (Private)**
  - Get coin info (deposit/withdrawal status) (`GetCoinInfo`)
  - Get deposit records (`GetDepositRecords`)
  - Get withdrawal records (`GetWithdrawRecords`)

### Market Data WebSocket (`ws_market_data.go`)
Real-time public market data streams:
- **Orderbook** - Depth updates (1/50/200/500 levels) with snapshot + delta
- **Tickers** - Real-time price, volume, funding rate updates
- **Trades** - Public trade stream

Features:
- Automatic reconnection with exponential backoff
- Local orderbook maintenance with delta updates
- Configurable callbacks for each data type

### Trading WebSocket (`ws_trading.go`)
Low-latency order entry via WebSocket:
- Create order (`CreateOrder`)
- Amend order (`AmendOrder`)
- Cancel order (`CancelOrder`)
- Batch create orders (`BatchCreateOrders`)

Features:
- Sub-millisecond order placement latency
- Request/response matching with request IDs
- Automatic authentication

### User Data WebSocket (`ws_user_data.go`)
Real-time private account streams:
- **Order updates** - Real-time order status changes
- **Position updates** - Real-time position changes, PnL
- **Execution updates** - Real-time fill notifications
- **Wallet updates** - Real-time balance changes

Features:
- Automatic reconnection and resubscription
- Category-specific subscriptions (linear, inverse, spot, option)

### Unified Client (`client.go`)
Single interface combining all components:
- Manages all connections
- Slippage calculator (walks the orderbook)
- Convenience methods for common operations

## Usage Examples

### Initialize Client
```go
client := bybit.NewClient(bybit.ClientConfig{
    APIKey:           "your-api-key",
    APISecret:        "your-api-secret",
    UseTestnet:       false,
    EnableMarketData: true,
    EnableTrading:    true,
    EnableUserData:   true,
    Category:         "linear",
    OrderbookDepth:   bybit.Depth50,
})

ctx := context.Background()
if err := client.Connect(ctx); err != nil {
    log.Fatal().Err(err).Msg("Failed to connect")
}
defer client.Disconnect()
```

### Subscribe to Orderbook
```go
client.SubscribeOrderbook([]string{"BTCUSDT", "ETHUSDT"}, func(symbol string, data *bybit.WSOrderbookData, isSnapshot bool, ts int64) {
    log.Info().
        Str("symbol", symbol).
        Int("bids", len(data.Bids)).
        Int("asks", len(data.Asks)).
        Bool("snapshot", isSnapshot).
        Msg("Orderbook update")
})
```

### Place a Limit Order (WebSocket - Low Latency)
```go
resp, err := client.PlaceLimitOrder(ctx, "BTCUSDT", bybit.OrderSideBuy, "0.001", "50000")
if err != nil {
    log.Error().Err(err).Msg("Failed to place order")
    return
}
log.Info().
    Str("orderId", resp.OrderID).
    Dur("latency", resp.Latency).
    Msg("Order placed")
```

### Get Positions
```go
positions, err := client.GetPositions(ctx, "BTCUSDT")
if err != nil {
    log.Error().Err(err).Msg("Failed to get positions")
    return
}
for _, pos := range positions.Result.List {
    log.Info().
        Str("symbol", pos.Symbol).
        Str("side", pos.Side).
        Str("size", pos.Size).
        Str("pnl", pos.UnrealisedPnl).
        Msg("Position")
}
```

### Calculate Slippage
```go
slippage, err := client.CalculateSlippage(ctx, "BTCUSDT", bybit.OrderSideBuy, 1.0) // 1 BTC
if err != nil {
    log.Error().Err(err).Msg("Failed to calculate slippage")
    return
}
log.Info().
    Float64("avgPrice", slippage.AvgPrice).
    Float64("slippageBps", slippage.SlippageBps).
    Int("levels", slippage.FilledLevels).
    Bool("insufficientLiq", slippage.InsufficientLiq).
    Msg("Slippage calculation")
```

### Subscribe to User Data Streams
```go
client.SubscribeOrderUpdates("linear", func(update *bybit.WSOrderUpdate) {
    log.Info().
        Str("orderId", update.OrderID).
        Str("status", update.OrderStatus).
        Str("filled", update.CumExecQty).
        Msg("Order update")
})

client.SubscribePositionUpdates("linear", func(update *bybit.WSPositionUpdate) {
    log.Info().
        Str("symbol", update.Symbol).
        Str("size", update.Size).
        Str("pnl", update.UnrealisedPnl).
        Msg("Position update")
})
```

## API Rate Limits

| Endpoint | Default | VIP |
|----------|---------|-----|
| POST /v5/order/create | 10/s | 20/s |
| POST /v5/order/cancel | 10/s | 20/s |
| GET /v5/order/realtime | 50/s | 50/s |
| GET /v5/position/list | 50/s | 50/s |
| WebSocket connections | 500/5min | - |

## WebSocket URLs

| Type | Mainnet | Testnet |
|------|---------|---------|
| Public Linear | wss://stream.bybit.com/v5/public/linear | wss://stream-testnet.bybit.com/v5/public/linear |
| Private | wss://stream.bybit.com/v5/private | wss://stream-testnet.bybit.com/v5/private |
| Trade | wss://stream.bybit.com/v5/trade | wss://stream-testnet.bybit.com/v5/trade |

## File Structure

```
bybit/
├── types.go          # All API types (REST + WebSocket)
├── rest_client.go    # REST API client with auth
├── ws_market_data.go # Public market data WebSocket
├── ws_trading.go     # Low-latency order WebSocket
├── ws_user_data.go   # Private user data WebSocket
├── client.go         # Unified client
├── bybit.go          # Connector interface implementation
└── README.md         # This file
```

## Authentication

### REST API
```
Headers:
- X-BAPI-API-KEY: your_api_key
- X-BAPI-TIMESTAMP: current_timestamp_ms
- X-BAPI-SIGN: HMAC_SHA256(api_secret, timestamp + api_key + recv_window + payload)
- X-BAPI-RECV-WINDOW: 5000
```

### WebSocket
```json
{
    "op": "auth",
    "args": [
        "API_KEY",
        "EXPIRES_TIMESTAMP",
        "HMAC_SHA256(api_secret, 'GET/realtime' + expires)"
    ]
}
```

## Error Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 10001 | Parameter error |
| 10003 | Invalid API key |
| 10004 | Invalid signature |
| 10006 | Too many requests |
| 110001 | Order does not exist |
| 110004 | Insufficient balance |
| 110008 | Exceed maximum order qty |

## References

- [Bybit V5 API Documentation](https://bybit-exchange.github.io/docs/v5/intro)
- [API Explorer](https://bybit-exchange.github.io/docs/api-explorer/v5/market/instrument)
- [Go SDK](https://github.com/bybit-exchange/bybit.go.api)
