# Bitget Connector

Go client library for Bitget Exchange API V2, supporting REST and WebSocket APIs for futures trading.

## Features

- **REST API Client**: Full support for market data, account, position, and trading endpoints
- **WebSocket Market Data**: Real-time ticker, orderbook, trades, and candlestick streams
- **WebSocket Trading**: Low-latency order placement and cancellation via WebSocket
- **WebSocket User Data**: Real-time account, position, order, and fill updates
- **Unified Client**: Combines all APIs into a single easy-to-use interface

## Supported Product Types

- `USDT-FUTURES`: USDT margined perpetual futures
- `USDC-FUTURES`: USDC margined perpetual futures
- `COIN-FUTURES`: Coin margined perpetual futures

## Installation

```go
import "crossspread/services/md-ingest/internal/connector/bitget"
```

## Quick Start

### Basic Usage with REST API

```go
package main

import (
    "context"
    "fmt"
    "crossspread/services/md-ingest/internal/connector/bitget"
)

func main() {
    // Create client with API credentials
    client := bitget.NewClient(&bitget.ClientConfig{
        APIKey:     "your-api-key",
        SecretKey:  "your-secret-key",
        Passphrase: "your-passphrase",
        InstType:   bitget.ProductTypeUSDTFutures,
    })

    ctx := context.Background()

    // Get all contracts
    contracts, err := client.GetContracts(ctx)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Found %d contracts\n", len(contracts))

    // Get ticker
    ticker, err := client.GetTicker(ctx, "BTCUSDT")
    if err != nil {
        panic(err)
    }
    fmt.Printf("BTC Price: %s\n", ticker.LastPr)

    // Get order book
    book, err := client.GetOrderBook(ctx, "BTCUSDT", 5)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Best Bid: %s, Best Ask: %s\n", book.Bids[0].Price, book.Asks[0].Price)
}
```

### WebSocket Market Data

```go
package main

import (
    "fmt"
    "crossspread/services/md-ingest/internal/connector/bitget"
)

type MyHandler struct{}

func (h *MyHandler) OnTicker(ticker *bitget.WSTickerData) {
    fmt.Printf("Ticker: %s @ %s\n", ticker.InstID, ticker.LastPr)
}

func (h *MyHandler) OnOrderBook(instID string, action string, book *bitget.WSOrderBookData) {
    fmt.Printf("OrderBook [%s] %s: bids=%d, asks=%d\n", action, instID, len(book.Bids), len(book.Asks))
}

func (h *MyHandler) OnTrade(trade *bitget.WSTradeData) {
    fmt.Printf("Trade: %s %s @ %s\n", trade.Side, trade.Size, trade.Price)
}

func (h *MyHandler) OnCandle(instID string, channel string, candle *bitget.WSCandleData) {
    fmt.Printf("Candle [%s]: O=%s H=%s L=%s C=%s\n", channel, candle.Open, candle.High, candle.Low, candle.Close)
}

func (h *MyHandler) OnError(err error) {
    fmt.Printf("Error: %v\n", err)
}

func (h *MyHandler) OnConnected() {
    fmt.Println("Connected to market data")
}

func (h *MyHandler) OnDisconnected() {
    fmt.Println("Disconnected from market data")
}

func main() {
    client := bitget.NewClient(&bitget.ClientConfig{
        InstType: bitget.ProductTypeUSDTFutures,
    })

    // Connect to market data WebSocket
    if err := client.ConnectMarketData(&MyHandler{}); err != nil {
        panic(err)
    }
    defer client.DisconnectMarketData()

    // Subscribe to BTC ticker
    client.SubscribeTicker("BTCUSDT")

    // Subscribe to BTC order book (top 5 levels)
    client.SubscribeOrderBook("BTCUSDT", bitget.ChannelBooks5)

    // Subscribe to BTC trades
    client.SubscribeTrades("BTCUSDT")

    // Keep running
    select {}
}
```

### WebSocket Trading (Low-Latency)

```go
package main

import (
    "context"
    "fmt"
    "crossspread/services/md-ingest/internal/connector/bitget"
)

type TradingHandler struct{}

func (h *TradingHandler) OnOrderResponse(id string, result *bitget.WSTradeResponse) {
    fmt.Printf("Order Response [%s]: code=%s, msg=%s\n", id, result.Code, result.Msg)
}

func (h *TradingHandler) OnError(err error) {
    fmt.Printf("Error: %v\n", err)
}

func (h *TradingHandler) OnConnected() {
    fmt.Println("Connected to trading")
}

func (h *TradingHandler) OnDisconnected() {
    fmt.Println("Disconnected from trading")
}

func main() {
    client := bitget.NewClient(&bitget.ClientConfig{
        APIKey:     "your-api-key",
        SecretKey:  "your-secret-key",
        Passphrase: "your-passphrase",
        InstType:   bitget.ProductTypeUSDTFutures,
    })

    // Connect to trading WebSocket
    if err := client.ConnectTrading(&TradingHandler{}); err != nil {
        panic(err)
    }
    defer client.DisconnectTrading()

    ctx := context.Background()

    // Place a limit order via WebSocket
    resp, err := client.WSPlaceOrder(ctx, &bitget.WSPlaceOrderArg{
        InstID:     "BTCUSDT",
        MarginCoin: "USDT",
        Size:       "0.001",
        Price:      "50000",
        Side:       bitget.SideBuy,
        OrderType:  bitget.OrderTypeLimit,
        Force:      bitget.ForceGTC,
    })
    if err != nil {
        panic(err)
    }
    fmt.Printf("Order placed: %+v\n", resp)
}
```

### WebSocket User Data

```go
package main

import (
    "fmt"
    "crossspread/services/md-ingest/internal/connector/bitget"
)

type UserHandler struct{}

func (h *UserHandler) OnAccount(data *bitget.WSAccountData) {
    fmt.Printf("Account: %s, Available: %s\n", data.MarginCoin, data.Available)
}

func (h *UserHandler) OnEquity(data *bitget.WSEquityData) {
    fmt.Printf("Equity: Total=%s\n", data.TotalEquity)
}

func (h *UserHandler) OnPosition(data *bitget.WSPositionData) {
    fmt.Printf("Position: %s %s, Size: %s, PnL: %s\n", data.InstID, data.HoldSide, data.Total, data.UnrealizedPL)
}

func (h *UserHandler) OnOrder(data *bitget.WSOrderData) {
    fmt.Printf("Order: %s %s %s @ %s, Status: %s\n", data.InstID, data.Side, data.Size, data.Price, data.Status)
}

func (h *UserHandler) OnFill(data *bitget.WSFillData) {
    fmt.Printf("Fill: %s %s @ %s, Profit: %s\n", data.Symbol, data.Side, data.PriceAvg, data.Profit)
}

func (h *UserHandler) OnError(err error) {
    fmt.Printf("Error: %v\n", err)
}

func (h *UserHandler) OnConnected() {
    fmt.Println("Connected to user data")
}

func (h *UserHandler) OnDisconnected() {
    fmt.Println("Disconnected from user data")
}

func main() {
    client := bitget.NewClient(&bitget.ClientConfig{
        APIKey:     "your-api-key",
        SecretKey:  "your-secret-key",
        Passphrase: "your-passphrase",
        InstType:   bitget.ProductTypeUSDTFutures,
    })

    // Connect to user data WebSocket
    if err := client.ConnectUserData(&UserHandler{}); err != nil {
        panic(err)
    }
    defer client.DisconnectUserData()

    // Subscribe to all user data channels
    client.UserData().SubscribeAll("USDT")

    // Keep running
    select {}
}
```

## REST API Endpoints

### Market Data (Public)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GetContracts` | `/api/v2/mix/market/contracts` | Get all contracts |
| `GetTickers` | `/api/v2/mix/market/tickers` | Get all tickers |
| `GetTicker` | `/api/v2/mix/market/ticker` | Get single ticker |
| `GetOrderBook` | `/api/v2/mix/market/merge-depth` | Get order book |
| `GetCandles` | `/api/v2/mix/market/candles` | Get candlesticks |
| `GetHistoryCandles` | `/api/v2/mix/market/history-candles` | Get historical candlesticks |
| `GetCurrentFundingRate` | `/api/v2/mix/market/current-fund-rate` | Get funding rate |
| `GetHistoryFundingRate` | `/api/v2/mix/market/history-fund-rate` | Get historical funding |
| `GetTrades` | `/api/v2/mix/market/fills` | Get recent trades |

### Account (Private)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GetAccount` | `/api/v2/mix/account/account` | Get single account |
| `GetAccounts` | `/api/v2/mix/account/accounts` | Get all accounts |
| `SetLeverage` | `/api/v2/mix/account/set-leverage` | Set leverage |
| `SetMarginMode` | `/api/v2/mix/account/set-margin-mode` | Set margin mode |
| `SetPositionMode` | `/api/v2/mix/account/set-position-mode` | Set position mode |

### Position (Private)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GetPositions` | `/api/v2/mix/position/all-position` | Get all positions |
| `GetSinglePosition` | `/api/v2/mix/position/single-position` | Get single position |
| `GetHistoryPositions` | `/api/v2/mix/position/history-position` | Get history positions |

### Trading (Private)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `PlaceOrder` | `/api/v2/mix/order/place-order` | Place order |
| `BatchPlaceOrder` | `/api/v2/mix/order/batch-place-order` | Batch place orders |
| `CancelOrder` | `/api/v2/mix/order/cancel-order` | Cancel order |
| `BatchCancelOrder` | `/api/v2/mix/order/batch-cancel-orders` | Batch cancel orders |
| `ModifyOrder` | `/api/v2/mix/order/modify-order` | Modify order |
| `GetPendingOrders` | `/api/v2/mix/order/orders-pending` | Get pending orders |
| `GetHistoryOrders` | `/api/v2/mix/order/orders-history` | Get history orders |
| `GetOrderDetail` | `/api/v2/mix/order/detail` | Get order detail |
| `GetFillHistory` | `/api/v2/mix/order/fill-history` | Get fill history |

## WebSocket Channels

### Public Channels

| Channel | Description |
|---------|-------------|
| `ticker` | Real-time ticker updates |
| `books` | Full order book |
| `books1` | Top 1 level order book |
| `books5` | Top 5 levels order book |
| `books15` | Top 15 levels order book |
| `trade` | Real-time trades |
| `candle1m` | 1-minute candlesticks |
| `candle5m` | 5-minute candlesticks |
| `candle15m` | 15-minute candlesticks |
| `candle1H` | 1-hour candlesticks |
| `candle1D` | 1-day candlesticks |

### Private Channels

| Channel | Description |
|---------|-------------|
| `account` | Account balance updates |
| `equity` | Equity updates |
| `positions` | Position updates |
| `orders` | Order updates |
| `fill` | Fill/trade updates |

### Trading Operations (WebSocket)

| Operation | Description |
|-----------|-------------|
| `place-order` | Place single order |
| `batch-place-order` | Place multiple orders |
| `cancel-order` | Cancel single order |
| `batch-cancel-order` | Cancel multiple orders |

## Authentication

### REST API

```
ACCESS-KEY: API Key
ACCESS-SIGN: Base64(HMAC-SHA256(timestamp + method + requestPath + body, secretKey))
ACCESS-TIMESTAMP: Unix timestamp in milliseconds
ACCESS-PASSPHRASE: API passphrase
```

### WebSocket

```
sign = Base64(HMAC-SHA256(timestamp + 'GET' + '/user/verify', secretKey))
```

## Error Handling

```go
result, err := client.PlaceOrder(ctx, req)
if err != nil {
    if apiErr, ok := err.(*bitget.APIError); ok {
        fmt.Printf("API Error: code=%s, msg=%s\n", apiErr.Code, apiErr.Message)
    } else {
        fmt.Printf("Error: %v\n", err)
    }
}
```

## Common Error Codes

| Code | Description |
|------|-------------|
| `00000` | Success |
| `40101` | Signature error |
| `40102` | Timestamp expired |
| `40103` | Invalid API key |
| `40104` | Invalid passphrase |
| `40301` | Symbol not found |
| `40302` | Order not found |
| `40304` | Insufficient balance |
| `40901` | Rate limit exceeded |

## Rate Limits

- Public endpoints: 20 requests/second
- Private endpoints: 10 requests/second
- WebSocket connections: Managed automatically with ping/pong

## License

MIT
