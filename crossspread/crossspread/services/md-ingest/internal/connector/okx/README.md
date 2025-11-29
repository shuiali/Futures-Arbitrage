# OKX Connector

Go client library for OKX exchange API integration, supporting REST and WebSocket APIs for market data, trading, and account management.

## Features

- **REST API Client**: Full support for public and private REST endpoints
- **WebSocket Market Data**: Real-time tickers, order books, trades, candlesticks, funding rates
- **WebSocket Trading**: Low-latency order placement, cancellation, and amendment via WebSocket
- **WebSocket User Data**: Real-time account, position, and order updates
- **Order Book Management**: Local order book maintenance with incremental updates
- **Position & Order Tracking**: Track positions and orders from WebSocket streams
- **Sliced Order Execution**: Execute large orders in configurable slices

## Installation

```go
import "crossspread/services/md-ingest/internal/connector/okx"
```

## Quick Start

### Basic REST Client

```go
package main

import (
    "context"
    "fmt"
    "crossspread/services/md-ingest/internal/connector/okx"
)

func main() {
    // Create REST client
    client := okx.NewRESTClient(okx.RESTClientConfig{
        APIKey:     "your-api-key",
        SecretKey:  "your-secret-key",
        Passphrase: "your-passphrase",
        DemoMode:   false, // Set true for paper trading
    })

    ctx := context.Background()

    // Get all perpetual swap instruments
    instruments, err := client.GetAllSwapInstruments(ctx)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Found %d swap instruments\n", len(instruments))

    // Get ticker for BTC-USDT-SWAP
    ticker, err := client.GetTicker(ctx, "BTC-USDT-SWAP")
    if err != nil {
        panic(err)
    }
    fmt.Printf("BTC-USDT-SWAP: Last=%s, Bid=%s, Ask=%s\n", 
        ticker.Last, ticker.BidPx, ticker.AskPx)

    // Get funding rate
    funding, err := client.GetFundingRate(ctx, "BTC-USDT-SWAP")
    if err != nil {
        panic(err)
    }
    fmt.Printf("Funding Rate: %s, Next: %s\n", 
        funding.FundingRate, funding.NextFundingRate)
}
```

### WebSocket Market Data

```go
package main

import (
    "fmt"
    "time"
    "crossspread/services/md-ingest/internal/connector/okx"
)

func main() {
    // Create handler
    handler := &okx.DefaultMarketDataHandler{
        OnTickerFunc: func(t *okx.WSTickerData) {
            fmt.Printf("[TICKER] %s: Last=%s Bid=%s Ask=%s\n",
                t.InstID, t.Last, t.BidPx, t.AskPx)
        },
        OnOrderBookFunc: func(instID, action string, book *okx.WSOrderBookData) {
            if len(book.Bids) > 0 && len(book.Asks) > 0 {
                fmt.Printf("[BOOK] %s (%s): Best Bid=%s Ask=%s\n",
                    instID, action, book.Bids[0].Price, book.Asks[0].Price)
            }
        },
        OnTradeFunc: func(t *okx.WSTradeData) {
            fmt.Printf("[TRADE] %s: %s %s @ %s\n",
                t.InstID, t.Side, t.Sz, t.Px)
        },
        OnConnectedFunc: func() {
            fmt.Println("Connected to OKX WebSocket")
        },
        OnErrorFunc: func(err error) {
            fmt.Printf("Error: %v\n", err)
        },
    }

    // Create client
    client := okx.NewMarketDataWSClient(okx.MarketDataWSConfig{
        Handler:  handler,
        DemoMode: false,
    })

    // Connect
    if err := client.Connect(); err != nil {
        panic(err)
    }
    defer client.Close()

    // Subscribe to BTC-USDT-SWAP
    client.SubscribeTicker("BTC-USDT-SWAP")
    client.SubscribeOrderBook("BTC-USDT-SWAP", okx.ChannelBooks)
    client.SubscribeTrades("BTC-USDT-SWAP")
    client.SubscribeFundingRate("BTC-USDT-SWAP")

    // Run for 60 seconds
    time.Sleep(60 * time.Second)
}
```

### WebSocket Trading (Low Latency)

```go
package main

import (
    "context"
    "fmt"
    "time"
    "crossspread/services/md-ingest/internal/connector/okx"
)

func main() {
    handler := &okx.DefaultTradingHandler{
        OnOrderResultFunc: func(id string, result *okx.OrderResult) {
            fmt.Printf("[ORDER] ID=%s OrderID=%s\n", id, result.OrdID)
        },
        OnAuthenticatedFunc: func() {
            fmt.Println("Authenticated!")
        },
    }

    client := okx.NewTradingWSClient(okx.TradingWSConfig{
        APIKey:     "your-api-key",
        SecretKey:  "your-secret-key",
        Passphrase: "your-passphrase",
        Handler:    handler,
        DemoMode:   true, // Use demo mode for testing
    })

    if err := client.Connect(); err != nil {
        panic(err)
    }
    defer client.Close()

    // Wait for authentication
    time.Sleep(2 * time.Second)

    ctx := context.Background()

    // Place a limit order
    result, err := client.PlaceLimitOrderSync(ctx, 
        "BTC-USDT-SWAP", // Instrument
        okx.SideBuy,     // Side
        "1",             // Size (contracts)
        "40000",         // Price
        okx.TdModeCross, // Trade mode
        okx.PosSideLong, // Position side
    )
    if err != nil {
        fmt.Printf("Order failed: %v\n", err)
        return
    }
    fmt.Printf("Order placed: %s\n", result.OrdID)

    // Cancel the order
    cancelResult, err := client.CancelOrderByIDSync(ctx, 
        "BTC-USDT-SWAP", 
        result.OrdID,
    )
    if err != nil {
        fmt.Printf("Cancel failed: %v\n", err)
        return
    }
    fmt.Printf("Order cancelled: %s\n", cancelResult.OrdID)
}
```

### Unified Client

```go
package main

import (
    "context"
    "fmt"
    "time"
    "crossspread/services/md-ingest/internal/connector/okx"
)

func main() {
    // Create unified client with all handlers
    client := okx.NewClient(okx.ClientConfig{
        APIKey:     "your-api-key",
        SecretKey:  "your-secret-key",
        Passphrase: "your-passphrase",
        DemoMode:   true,
        MarketDataHandler: &okx.DefaultMarketDataHandler{
            OnTickerFunc: func(t *okx.WSTickerData) {
                fmt.Printf("[TICKER] %s: %s\n", t.InstID, t.Last)
            },
        },
        TradingHandler: &okx.DefaultTradingHandler{
            OnOrderResultFunc: func(id string, result *okx.OrderResult) {
                fmt.Printf("[ORDER] %s\n", result.OrdID)
            },
        },
        UserDataHandler: &okx.DefaultUserDataHandler{
            OnPositionFunc: func(p *okx.WSPositionData) {
                fmt.Printf("[POSITION] %s: %s @ %s\n", 
                    p.InstID, p.Pos, p.AvgPx)
            },
        },
    })

    // Connect all WebSocket clients
    if err := client.Connect(); err != nil {
        panic(err)
    }
    defer client.Close()

    // Subscribe to market data
    client.SubscribeToInstrument("BTC-USDT-SWAP")
    
    // Subscribe to user data
    client.UserData.SubscribeAll()

    ctx := context.Background()

    // Place order via WebSocket (low latency)
    result, err := client.PlaceLimitOrder(ctx,
        "BTC-USDT-SWAP",
        okx.SideBuy,
        "1",
        "40000",
        okx.TdModeCross,
        okx.PosSideLong,
    )
    if err != nil {
        fmt.Printf("Order failed: %v\n", err)
    } else {
        fmt.Printf("Order: %s\n", result.OrdID)
    }

    time.Sleep(30 * time.Second)
}
```

## API Reference

### REST Client Methods

#### Public Data
- `GetInstruments(ctx, instType, uly, instFamily, instID)` - Get instruments
- `GetAllSwapInstruments(ctx)` - Get all perpetual swaps
- `GetAllFuturesInstruments(ctx, instFamily)` - Get all futures

#### Market Data
- `GetTickers(ctx, instType, instFamily)` - Get all tickers
- `GetTicker(ctx, instID)` - Get single ticker
- `GetOrderBook(ctx, instID, depth)` - Get order book
- `GetCandles(ctx, instID, bar, after, before, limit)` - Get candlesticks
- `GetHistoryCandles(ctx, instID, bar, after, before, limit)` - Get historical candlesticks
- `GetTrades(ctx, instID, limit)` - Get public trades
- `GetIndexTickers(ctx, quoteCcy, instID)` - Get index tickers

#### Funding Rate
- `GetFundingRate(ctx, instID)` - Get current funding rate
- `GetFundingRateHistory(ctx, instID, before, after, limit)` - Get historical funding rates

#### Asset
- `GetCurrencies(ctx)` - Get all currencies with deposit/withdrawal status

#### Account
- `GetBalance(ctx, ccy)` - Get account balance
- `GetPositions(ctx, instType, instID, posID)` - Get positions
- `GetAccountConfig(ctx)` - Get account configuration
- `SetPositionMode(ctx, posMode)` - Set position mode
- `SetLeverage(ctx, instID, lever, mgnMode, posSide)` - Set leverage
- `GetTradeFee(ctx, instType, instID, instFamily)` - Get trading fees

#### Trading
- `PlaceOrder(ctx, req)` - Place single order
- `PlaceBatchOrders(ctx, orders)` - Place batch orders (max 20)
- `CancelOrder(ctx, req)` - Cancel order
- `CancelBatchOrders(ctx, orders)` - Cancel batch orders
- `AmendOrder(ctx, req)` - Amend order
- `GetOrder(ctx, instID, orderID, clOrdID)` - Get order details
- `GetPendingOrders(ctx, ...)` - Get active orders
- `GetOrdersHistory(ctx, ...)` - Get order history

### WebSocket Channels

#### Market Data (Public)
- `tickers` - Price ticker updates
- `books` - Order book (400 levels, 100ms)
- `books5` - Order book (5 levels, 200ms)
- `bbo-tbt` - Best bid/offer (tick-by-tick)
- `books50-l2-tbt` - Order book (50 levels, tick-by-tick)
- `books-l2-tbt` - Order book (400 levels, tick-by-tick)
- `trades` - Public trades
- `candle{period}` - Candlesticks (1m, 5m, 15m, 1H, 4H, 1D, etc.)
- `funding-rate` - Funding rate updates
- `index-tickers` - Index price updates

#### User Data (Private)
- `account` - Account balance updates
- `positions` - Position updates
- `balance_and_position` - Combined balance and position updates
- `orders` - Order status updates

#### Trading Operations (Private)
- `order` - Place single order
- `batch-orders` - Place batch orders
- `cancel-order` - Cancel single order
- `batch-cancel-orders` - Cancel batch orders
- `amend-order` - Amend order

## Instrument Naming Convention

- **Perpetual Swaps**: `{BASE}-{QUOTE}-SWAP` (e.g., `BTC-USDT-SWAP`)
- **Futures**: `{BASE}-{QUOTE}-{EXPIRY}` (e.g., `BTC-USDT-231229`)
- **Spot**: `{BASE}-{QUOTE}` (e.g., `BTC-USDT`)

## Trade Modes

| Mode | Description |
|------|-------------|
| `cash` | Spot trading |
| `isolated` | Isolated margin |
| `cross` | Cross margin |

## Position Sides (Hedge Mode)

| Side | Description |
|------|-------------|
| `long` | Long position |
| `short` | Short position |
| `net` | Net position (one-way mode) |

## Error Handling

```go
result, err := client.PlaceOrder(ctx, req)
if err != nil {
    if apiErr, ok := err.(*okx.APIError); ok {
        fmt.Printf("API Error %s: %s\n", apiErr.Code, apiErr.Message)
        
        switch apiErr.Code {
        case okx.ErrCodeInsufficientBalance:
            // Handle insufficient balance
        case okx.ErrCodePriceOutOfRange:
            // Handle price out of range
        default:
            // Handle other errors
        }
    }
}
```

## Rate Limits

| Endpoint Type | Rate Limit |
|--------------|------------|
| Public Data | 20-40 req/2s (IP) |
| Trading | 60 req/2s (UserID) |
| Batch Trading | 300 req/2s (UserID) |
| Account Info | 5-10 req/2s (UserID) |
| WebSocket Connections | 3 per IP |
| WebSocket Messages | 100/s per connection |

## Demo/Paper Trading

Set `DemoMode: true` to use paper trading endpoints:

```go
client := okx.NewRESTClient(okx.RESTClientConfig{
    // ...
    DemoMode: true, // Adds x-simulated-trading: 1 header
})
```

## License

MIT
