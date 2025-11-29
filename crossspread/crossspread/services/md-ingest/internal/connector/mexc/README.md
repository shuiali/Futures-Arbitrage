# MEXC Exchange Connector

Go client library for MEXC Futures API supporting both REST and WebSocket connections.

## Features

- **REST API Client**: Full implementation of MEXC Futures REST API
  - Public endpoints: contracts, tickers, depth, trades, klines, funding rates
  - Private endpoints: account, positions, orders (place, cancel, query)
  - HMAC-SHA256 authentication
  - Rate limiting support

- **WebSocket Clients**:
  - Market Data: ticker, orderbook depth, trades, klines (public)
  - Trading: order placement, cancellation via WebSocket (private)
  - User Data: positions, account updates, order updates (private)
  - Automatic reconnection and ping/pong handling

- **Unified Client**: Single client interface combining REST + WebSocket

## Usage

### Basic Market Data (Public)

```go
import "crossspread-md-ingest/internal/connector/mexc"

// Create connector
connector := mexc.NewMEXCConnector([]string{"BTC_USDT"}, 20)

// Fetch instruments
instruments, err := connector.FetchInstruments(ctx)

// Fetch tickers
tickers, err := connector.FetchPriceTickers(ctx)

// Fetch funding rates
rates, err := connector.FetchFundingRates(ctx)

// Connect WebSocket for real-time data
connector.SetOrderbookHandler(func(ob *connector.Orderbook) {
    fmt.Printf("Orderbook: %s bid=%f ask=%f\n", ob.Symbol, ob.BestBid, ob.BestAsk)
})

err := connector.Connect(ctx)
```

### Trading with Credentials

```go
// Create connector with credentials
connector := mexc.NewMEXCConnectorWithCredentials(
    []string{"BTC_USDT"},
    20,
    "your-api-key",
    "your-secret-key",
)

// Get underlying client for trading operations
client := connector.Client()

// Place order via REST
order, err := client.PlaceOrder(ctx, &mexc.OrderRequest{
    Symbol:    "BTC_USDT",
    Price:     50000.0,
    Vol:       0.01,
    Side:      mexc.SideOpenLong,
    OrderType: mexc.OrderTypeLimitGTC,
    OpenType:  mexc.OpenTypeCross,
})

// Or via WebSocket for lower latency
wsOrder, err := client.PlaceOrderWS(&mexc.PlaceOrderRequest{
    Symbol:    "BTC_USDT",
    Price:     "50000",
    Volume:    "0.01",
    Side:      mexc.SideOpenLong,
    OrderType: mexc.OrderTypeLimitGTC,
    OpenType:  mexc.OpenTypeCross,
})

// Cancel order
err = client.CancelOrder(ctx, &mexc.CancelOrderRequest{
    Symbol:  "BTC_USDT",
    OrderID: 12345,
})
```

### Using Unified Client Directly

```go
// Create client
client, err := mexc.NewClient(&mexc.ClientConfig{
    APIKey:    "your-api-key",
    SecretKey: "your-secret-key",
})

// Set handlers
client.SetMarketDataHandler(myHandler)
client.SetUserDataHandler(myUserHandler)

// Connect WebSocket streams
client.ConnectMarketData()
client.ConnectUserData()

// Subscribe to symbols
client.SubscribeDepth("BTC_USDT")
client.SubscribeTicker("ETH_USDT")

// Subscribe to user data
client.SubscribeUserData()

// REST operations
contracts, err := client.GetContracts(ctx)
positions, err := client.GetOpenPositions(ctx, "BTC_USDT")
```

## File Structure

```
mexc/
├── types.go           # All API types and constants
├── rest_client.go     # REST API client implementation
├── ws_market_data.go  # WebSocket market data client
├── ws_trading.go      # WebSocket trading client
├── ws_user_data.go    # WebSocket user data client
├── client.go          # Unified client combining REST + WS
├── mexc.go           # Connector interface implementation
└── README.md
```

## API Reference

### REST Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/contract/detail` | GET | Get all contracts |
| `/api/v1/contract/ticker` | GET | Get all tickers |
| `/api/v1/contract/depth/{symbol}` | GET | Get orderbook depth |
| `/api/v1/contract/deals/{symbol}` | GET | Get recent trades |
| `/api/v1/contract/kline/{symbol}` | GET | Get kline data |
| `/api/v1/contract/funding_rate/{symbol}` | GET | Get funding rate |
| `/api/v1/private/account/assets` | GET | Get account assets |
| `/api/v1/private/position/open_positions` | GET | Get open positions |
| `/api/v1/private/order/submit` | POST | Place order |
| `/api/v1/private/order/cancel` | POST | Cancel order |
| `/api/v1/private/order/list/open_orders/{symbol}` | GET | Get open orders |

### WebSocket Channels

**Public:**
- `sub.ticker` / `push.ticker` - Ticker updates
- `sub.depth` / `push.depth` - Orderbook depth updates
- `sub.deal` / `push.deal` - Trade updates
- `sub.kline` / `push.kline` - Kline updates

**Private (requires authentication):**
- `sub.personal.order` / `push.personal.order` - Order updates
- `sub.personal.position` / `push.personal.position` - Position updates
- `sub.personal.asset` / `push.personal.asset` - Asset updates

### Authentication

MEXC uses HMAC-SHA256 for authentication:

```
signature = HMAC_SHA256(apiKey + timestamp + parameterString, secretKey)
```

Headers:
- `ApiKey`: Your API key
- `Request-Time`: Current timestamp in milliseconds
- `Signature`: HMAC-SHA256 signature
- `Recv-Window`: Request validity window (optional, default 5000ms)

## Order Types

| Value | Type |
|-------|------|
| 1 | Limit GTC (Good Till Cancel) |
| 2 | Post Only |
| 3 | IOC (Immediate or Cancel) |
| 4 | FOK (Fill or Kill) |
| 5 | Market |
| 6 | Convert to limit at market price |

## Order Sides

| Value | Side |
|-------|------|
| 1 | Open Long |
| 2 | Close Short |
| 3 | Open Short |
| 4 | Close Long |

## Position Types

| Value | Type |
|-------|------|
| 1 | Isolated Margin |
| 2 | Cross Margin |
