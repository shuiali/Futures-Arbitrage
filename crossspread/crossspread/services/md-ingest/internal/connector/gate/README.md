# Gate.io Futures Connector

This package provides a comprehensive Go client for Gate.io Futures API, supporting both REST and WebSocket APIs for market data, trading, and account management.

## Features

### REST API
- **Market Data**: Contracts, tickers, orderbook, trades, candlesticks, funding rates
- **Account Management**: Account balance, positions, position margin/leverage
- **Order Management**: Place, cancel, amend orders; batch operations
- **Trade History**: User trades, liquidations, auto-deleverages
- **Currency Info**: Deposit/withdrawal status, chain information

### WebSocket API
- **Market Data Streams**: Real-time tickers, orderbook (full + incremental), trades, candlesticks, book ticker
- **Trading API**: Low-latency order placement, cancellation, amendment via WebSocket
- **User Data Streams**: Orders, positions, balances, user trades with authentication

## Quick Start

### Basic Usage (Public Data Only)

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "crossspread-md-ingest/internal/connector/gate"
)

func main() {
    // Create client (no credentials needed for public data)
    client := gate.NewClient(nil)
    defer client.Close()
    
    ctx := context.Background()
    settle := gate.SettleUSDT
    
    // Fetch all contracts
    contracts, err := client.GetContracts(ctx, settle)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Found %d contracts\n", len(contracts))
    
    // Fetch tickers
    tickers, err := client.GetTickers(ctx, settle)
    if err != nil {
        log.Fatal(err)
    }
    for _, t := range tickers[:5] {
        fmt.Printf("%s: %s\n", t.Contract, t.Last)
    }
}
```

### Authenticated Client

```go
package main

import (
    "context"
    "log"
    
    "crossspread-md-ingest/internal/connector/gate"
)

func main() {
    // Create client with credentials
    client := gate.NewClientWithCredentials("your-api-key", "your-api-secret")
    defer client.Close()
    
    ctx := context.Background()
    settle := gate.SettleUSDT
    
    // Fetch account info
    account, err := client.GetAccount(ctx, settle)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Balance: %s %s", account.Total, account.Currency)
    
    // Fetch positions
    positions, err := client.GetPositions(ctx, settle)
    if err != nil {
        log.Fatal(err)
    }
    for _, p := range positions {
        log.Printf("Position: %s, size: %d, PnL: %s", p.Contract, p.Size, p.UnrealisedPnl)
    }
}
```

### WebSocket Market Data

```go
package main

import (
    "log"
    "time"
    
    "crossspread-md-ingest/internal/connector/gate"
)

func main() {
    client := gate.NewClient(nil)
    defer client.Close()
    
    settle := gate.SettleUSDT
    
    // Set market data handler
    client.SetMarketDataHandler(&gate.WSMarketDataHandler{
        OnTicker: func(settle string, ticker *gate.WSTickerData) {
            log.Printf("Ticker: %s, last: %s, bid: %s, ask: %s",
                ticker.Contract, ticker.Last, ticker.HighestBid, ticker.LowestAsk)
        },
        OnOrderBook: func(settle string, ob *gate.WSOrderBookData) {
            log.Printf("OrderBook: %s, bids: %d, asks: %d",
                ob.Contract, len(ob.Bids), len(ob.Asks))
        },
        OnTrade: func(settle string, trade *gate.WSTradeData) {
            log.Printf("Trade: %s, price: %s, size: %d",
                trade.Contract, trade.Price, trade.Size)
        },
        OnConnect: func(settle string) {
            log.Printf("Connected to %s", settle)
        },
        OnDisconnect: func(settle string, err error) {
            log.Printf("Disconnected from %s: %v", settle, err)
        },
    })
    
    // Connect and subscribe
    if err := client.ConnectMarketData(settle); err != nil {
        log.Fatal(err)
    }
    
    // Subscribe to BTC orderbook
    if err := client.SubscribeOrderBook(settle, "BTC_USDT", "20", "100ms"); err != nil {
        log.Fatal(err)
    }
    
    // Subscribe to multiple tickers
    if err := client.SubscribeTickers(settle, []string{"BTC_USDT", "ETH_USDT"}); err != nil {
        log.Fatal(err)
    }
    
    // Keep running
    time.Sleep(60 * time.Second)
}
```

### WebSocket Trading (Low Latency)

```go
package main

import (
    "log"
    
    "crossspread-md-ingest/internal/connector/gate"
)

func main() {
    client := gate.NewClientWithCredentials("your-api-key", "your-api-secret")
    defer client.Close()
    
    settle := gate.SettleUSDT
    
    // Set trading handler
    client.SetTradingHandler(&gate.WSTradingHandler{
        OnOrderPlaced: func(reqID string, order *gate.Order, err error) {
            if err != nil {
                log.Printf("Order failed: %v", err)
                return
            }
            log.Printf("Order placed: ID=%d, status=%s", order.ID, order.Status)
        },
        OnOrderCanceled: func(reqID string, order *gate.Order, err error) {
            log.Printf("Order canceled: ID=%d", order.ID)
        },
        OnLogin: func(settle string, success bool, err error) {
            log.Printf("Login result: success=%v, err=%v", success, err)
        },
    })
    
    // Connect trading WebSocket (includes authentication)
    if err := client.ConnectTrading(settle); err != nil {
        log.Fatal(err)
    }
    
    // Place order via WebSocket (low latency)
    order, err := client.PlaceOrderWS(settle, gate.NewLimitOrder("BTC_USDT", 1, "50000", gate.TIFGoodTillCancel))
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Order placed: %d", order.ID)
    
    // Async order placement (lowest latency)
    reqID, err := client.PlaceOrderAsync(settle, gate.NewLimitOrder("ETH_USDT", 1, "3000", ""))
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Order request submitted: %s", reqID)
}
```

### User Data Streams

```go
package main

import (
    "log"
    "time"
    
    "crossspread-md-ingest/internal/connector/gate"
)

func main() {
    client := gate.NewClientWithCredentials("your-api-key", "your-api-secret")
    defer client.Close()
    
    settle := gate.SettleUSDT
    
    // Set user data handler
    client.SetUserDataHandler(&gate.WSUserDataHandler{
        OnOrder: func(settle string, order *gate.WSOrderData) {
            log.Printf("Order update: %d, status=%s, left=%d", order.ID, order.Status, order.Left)
        },
        OnPosition: func(settle string, pos *gate.WSPositionData) {
            log.Printf("Position update: %s, size=%d, pnl=%s", pos.Contract, pos.Size, pos.UnrealisedPnl)
        },
        OnBalance: func(settle string, bal *gate.WSBalanceData) {
            log.Printf("Balance update: %s, change=%s", bal.Balance, bal.Change)
        },
        OnUserTrade: func(settle string, trade *gate.WSUserTradeData) {
            log.Printf("Trade: %s, price=%s, size=%d, fee=%s", 
                trade.Contract, trade.Price, trade.Size, trade.Fee)
        },
    })
    
    // Connect and subscribe to all user data
    if err := client.ConnectUserData(settle); err != nil {
        log.Fatal(err)
    }
    if err := client.SubscribeAllUserData(settle); err != nil {
        log.Fatal(err)
    }
    
    time.Sleep(60 * time.Second)
}
```

### Using the Connector Interface

```go
package main

import (
    "context"
    "log"
    
    "crossspread-md-ingest/internal/connector"
    "crossspread-md-ingest/internal/connector/gate"
)

func main() {
    // Create connector for integration with spread discovery
    conn := gate.NewGateConnector(
        []string{"BTC_USDT", "ETH_USDT"},  // Symbols to track
        20,                                  // Orderbook depth
        gate.SettleUSDT,                     // Settle currency
    )
    
    // Set handlers
    conn.SetOrderbookHandler(func(ob *connector.Orderbook) {
        log.Printf("Orderbook: %s, bid=%f, ask=%f, spread=%f bps",
            ob.Symbol, ob.BestBid, ob.BestAsk, ob.SpreadBps)
    })
    
    conn.SetErrorHandler(func(err error) {
        log.Printf("Error: %v", err)
    })
    
    // Connect
    ctx := context.Background()
    if err := conn.Connect(ctx); err != nil {
        log.Fatal(err)
    }
    defer conn.Disconnect()
    
    // Fetch instruments
    instruments, err := conn.FetchInstruments(ctx)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Found %d instruments", len(instruments))
    
    // Fetch funding rates
    rates, err := conn.FetchFundingRates(ctx)
    if err != nil {
        log.Fatal(err)
    }
    for _, r := range rates[:5] {
        log.Printf("%s: funding rate = %f", r.Symbol, r.FundingRate)
    }
}
```

## API Coverage

### REST API Endpoints

| Category | Endpoint | Status |
|----------|----------|--------|
| Market Data | Get Contracts | ✅ |
| Market Data | Get Tickers | ✅ |
| Market Data | Get OrderBook | ✅ |
| Market Data | Get Trades | ✅ |
| Market Data | Get Candlesticks | ✅ |
| Market Data | Get Funding Rates | ✅ |
| Account | Get Account | ✅ |
| Positions | Get Positions | ✅ |
| Positions | Get Position | ✅ |
| Positions | Update Margin | ✅ |
| Positions | Update Leverage | ✅ |
| Orders | Place Order | ✅ |
| Orders | Place Batch Orders | ✅ |
| Orders | Get Orders | ✅ |
| Orders | Get Order | ✅ |
| Orders | Cancel Order | ✅ |
| Orders | Cancel All Orders | ✅ |
| Orders | Amend Order | ✅ |
| Trades | Get My Trades | ✅ |
| Wallet | Get Currencies | ✅ |
| Wallet | Get Withdraw Status | ✅ |

### WebSocket Channels

| Category | Channel | Status |
|----------|---------|--------|
| Market Data | futures.tickers | ✅ |
| Market Data | futures.trades | ✅ |
| Market Data | futures.order_book | ✅ |
| Market Data | futures.order_book_update | ✅ |
| Market Data | futures.book_ticker | ✅ |
| Market Data | futures.candlesticks | ✅ |
| Trading | futures.order_place | ✅ |
| Trading | futures.order_cancel | ✅ |
| Trading | futures.order_amend | ✅ |
| Trading | futures.order_batch_place | ✅ |
| User Data | futures.orders | ✅ |
| User Data | futures.positions | ✅ |
| User Data | futures.balances | ✅ |
| User Data | futures.usertrades | ✅ |
| User Data | futures.liquidates | ✅ |
| User Data | futures.auto_orders | ✅ |

## Settlement Currencies

Gate.io supports two settlement currencies for futures:
- `usdt` - USDT-settled perpetual contracts (default)
- `btc` - BTC-settled perpetual contracts

Use the constants:
```go
gate.SettleUSDT // "usdt"
gate.SettleBTC  // "btc"
```

## Order Types and Time-in-Force

```go
// Time in Force options
gate.TIFGoodTillCancel    // "gtc" - Good till cancelled
gate.TIFImmediateOrCancel // "ioc" - Immediate or cancel (taker only)
gate.TIFPostOnly          // "poc" - Post only (maker only)
gate.TIFFillOrKill        // "fok" - Fill or kill

// Order request helpers
gate.NewLimitOrder(contract, size, price, tif)
gate.NewMarketOrder(contract, size)
gate.NewReduceOnlyOrder(contract, size, price)
gate.NewCloseOrder(contract, size, price)
```

## Authentication

Gate.io uses HMAC-SHA512 for API authentication:

```
signature = HMAC-SHA512(sign_string, secret_key)
sign_string = method + "\n" + path + "\n" + query_string + "\n" + body_hash + "\n" + timestamp
```

Headers:
- `KEY`: API key
- `Timestamp`: Unix timestamp in seconds
- `SIGN`: HMAC-SHA512 signature

## Rate Limiting

The client implements automatic rate limiting:
- Public endpoints: 100 requests/second
- Private endpoints: 50 requests/second

## Error Handling

The client returns typed errors when available:

```go
var apiErr *gate.APIError
if errors.As(err, &apiErr) {
    log.Printf("API Error: [%s] %s", apiErr.Label, apiErr.Message)
}

// Common error labels
gate.ErrLabelInsufficientBalance
gate.ErrLabelOrderNotFound
gate.ErrLabelInvalidPrice
gate.ErrLabelFOKNotFill
```

## Testnet Support

```go
client := gate.NewTestnetClient("your-api-key", "your-api-secret")
```

## Files

| File | Description |
|------|-------------|
| `types.go` | All API types and constants |
| `rest_client.go` | REST API client with authentication |
| `ws_market_data.go` | WebSocket market data client |
| `ws_trading.go` | WebSocket trading API client |
| `ws_user_data.go` | WebSocket user data client |
| `client.go` | Unified client combining all APIs |
| `gate.go` | Connector interface implementation |

## API Documentation

- [Gate.io API v4 Documentation](https://www.gate.io/docs/developers/apiv4/en/)
- [Futures REST API](https://www.gate.io/docs/developers/futures/en/)
- [Futures WebSocket API](https://www.gate.io/docs/developers/futures/ws/en/)
