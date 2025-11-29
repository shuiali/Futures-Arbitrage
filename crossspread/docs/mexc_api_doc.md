# MEXC API Documentation

## Overview

MEXC provides two separate APIs:
1. **Futures/Contract API** - `https://contract.mexc.com` - For perpetual futures trading
2. **Spot API** - `https://api.mexc.com` - For spot trading

This document covers both APIs with focus on futures trading for the arbitrage platform.

---

## Part 1: Futures/Contract API

### Base Configuration

| Property | Value |
|----------|-------|
| REST Base URL | `https://contract.mexc.com` |
| WebSocket URL | `wss://contract.mexc.com/ws` |
| API Version | `/api/v1/` |
| Time Security | 10 second window (default), max 60 seconds |

### Authentication

**Headers Required for Private Endpoints:**
```
ApiKey: <your_api_key>
Request-Time: <timestamp_ms>
Signature: <hmac_sha256_signature>
Recv-Window: <optional, default 5000, max 60000>
Content-Type: application/json
```

**Signature Generation:**
```
signature = HMAC-SHA256(accessKey + timestamp + parameterString)
```

Where:
- `accessKey`: Your API key
- `timestamp`: Current time in milliseconds
- `parameterString`: Query string for GET, JSON body for POST

### Response Format

```json
{
  "success": true,
  "code": 0,
  "data": { ... }
}
```

Error Response:
```json
{
  "success": false,
  "code": 401,
  "message": "Not logged in or login has expired"
}
```

---

## Futures Public Endpoints

### 1. Get All Contract Details
**Endpoint:** `GET /api/v1/contract/detail`

**Description:** Returns all available perpetual contracts with trading parameters.

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| symbol | string | Contract symbol (e.g., "BTC_USDT") |
| displayName | string | Display name |
| displayNameEn | string | English display name |
| positionOpenType | int | Position mode: 1=isolated, 2=cross, 3=both |
| baseCoin | string | Base currency (e.g., "BTC") |
| quoteCoin | string | Quote currency (e.g., "USDT") |
| settleCoin | string | Settlement currency |
| contractSize | decimal | Contract size |
| minLeverage | int | Minimum leverage |
| maxLeverage | int | Maximum leverage |
| priceScale | int | Price decimal places |
| volScale | int | Volume decimal places |
| amountScale | int | Amount decimal places |
| priceUnit | decimal | Minimum price increment |
| volUnit | decimal | Minimum volume increment |
| minVol | decimal | Minimum order volume |
| maxVol | decimal | Maximum order volume |
| bidLimitPriceRate | decimal | Buy limit price rate |
| askLimitPriceRate | decimal | Sell limit price rate |
| takerFeeRate | decimal | Taker fee rate |
| makerFeeRate | decimal | Maker fee rate |
| maintenanceMarginRate | decimal | Maintenance margin rate |
| initialMarginRate | decimal | Initial margin rate |
| riskBaseVol | decimal | Risk base volume |
| riskIncrVol | decimal | Risk increment volume |
| riskIncrMmr | decimal | Risk increment MMR |
| riskIncrImr | decimal | Risk increment IMR |
| riskLevelLimit | int | Risk level limit |
| priceCoefficientVariation | decimal | Price coefficient variation |
| state | int | Contract state (0=active) |
| isNew | bool | Is new listing |
| isHot | bool | Is popular |
| isHidden | bool | Is hidden |
| conceptPlate | array | Category tags |
| apiAllowed | bool | API trading allowed |

---

### 2. Get All Tickers
**Endpoint:** `GET /api/v1/contract/ticker`

**Description:** Returns real-time ticker data for all contracts.

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| symbol | string | Contract symbol |
| lastPrice | decimal | Last traded price |
| bid1 | decimal | Best bid price |
| ask1 | decimal | Best ask price |
| volume24 | decimal | 24h volume (contracts) |
| amount24 | decimal | 24h turnover |
| holdVol | decimal | Open interest (contracts) |
| lower24Price | decimal | 24h low |
| high24Price | decimal | 24h high |
| riseFallRate | decimal | 24h price change % |
| riseFallValue | decimal | 24h price change value |
| indexPrice | decimal | Index price |
| fairPrice | decimal | Mark/fair price |
| fundingRate | decimal | Current funding rate |
| maxBidPrice | decimal | Maximum bid price allowed |
| minAskPrice | decimal | Minimum ask price allowed |
| timestamp | long | Data timestamp (ms) |

---

### 3. Get Order Book Depth
**Endpoint:** `GET /api/v1/contract/depth/{symbol}`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | Yes | Contract symbol (e.g., "BTC_USDT") |
| limit | No | Depth limit (default: 20) |

**Response Format:**
```json
{
  "success": true,
  "code": 0,
  "data": {
    "asks": [[price, quantity, count], ...],
    "bids": [[price, quantity, count], ...],
    "version": 123456789,
    "timestamp": 1234567890123
  }
}
```

**Array Fields:**
- `price`: Price level
- `quantity`: Total quantity at price level
- `count`: Number of orders at price level

---

### 4. Get Recent Trades (Deals)
**Endpoint:** `GET /api/v1/contract/deals/{symbol}`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | Yes | Contract symbol |
| limit | No | Number of trades (default: 20, max: 100) |

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| p | decimal | Trade price |
| v | decimal | Trade volume |
| T | int | Trade type: 1=buy, 2=sell |
| O | int | Open type |
| M | int | Market type |
| t | long | Trade timestamp (ms) |

---

### 5. Get Kline/Candlestick Data
**Endpoint:** `GET /api/v1/contract/kline/{symbol}`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | Yes | Contract symbol |
| interval | No | Kline interval (default: "Min1") |
| start | No | Start timestamp (seconds) |
| end | No | End timestamp (seconds) |

**Interval Values:**
- Minutes: `Min1`, `Min5`, `Min15`, `Min30`, `Min60`
- Hours: `Hour4`, `Hour8`
- Days: `Day1`, `Week1`, `Month1`

**Response Format:**
```json
{
  "success": true,
  "code": 0,
  "data": {
    "time": [1234567890, ...],
    "open": [50000.0, ...],
    "high": [50100.0, ...],
    "low": [49900.0, ...],
    "close": [50050.0, ...],
    "vol": [1000.0, ...],
    "amount": [50000000.0, ...]
  }
}
```

---

### 6. Get Index Price
**Endpoint:** `GET /api/v1/contract/index_price/{symbol}`

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| symbol | string | Contract symbol |
| indexPrice | decimal | Current index price |
| timestamp | long | Timestamp (ms) |

---

### 7. Get Fair/Mark Price
**Endpoint:** `GET /api/v1/contract/fair_price/{symbol}`

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| symbol | string | Contract symbol |
| fairPrice | decimal | Current fair/mark price |
| timestamp | long | Timestamp (ms) |

---

### 8. Get Funding Rate
**Endpoint:** `GET /api/v1/contract/funding_rate/{symbol}`

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| symbol | string | Contract symbol |
| fundingRate | decimal | Current funding rate |
| maxFundingRate | decimal | Maximum funding rate |
| minFundingRate | decimal | Minimum funding rate |
| collectCycle | int | Funding interval (hours) |
| nextSettleTime | long | Next settlement timestamp (ms) |
| timestamp | long | Current timestamp (ms) |

---

## Futures Private Endpoints

### Authentication Required for All Private Endpoints

### 1. Get Account Assets
**Endpoint:** `GET /api/v1/private/account/assets`

**Description:** Get all account asset balances.

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| currency | string | Currency symbol |
| positionMargin | decimal | Position margin |
| frozenBalance | decimal | Frozen balance |
| availableBalance | decimal | Available balance |
| cashBalance | decimal | Cash balance |
| equity | decimal | Total equity |
| unrealized | decimal | Unrealized PnL |

---

### 2. Get Specific Asset
**Endpoint:** `GET /api/v1/private/account/asset/{currency}`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| currency | Yes | Currency symbol (e.g., "USDT") |

---

### 3. Get Open Positions
**Endpoint:** `GET /api/v1/private/position/open_positions`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | No | Filter by symbol |

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| positionId | long | Position ID |
| symbol | string | Contract symbol |
| holdVol | decimal | Position size |
| positionType | int | 1=long, 2=short |
| openType | int | 1=isolated, 2=cross |
| state | int | Position state |
| frozenVol | decimal | Frozen volume |
| closeVol | decimal | Closed volume |
| holdAvgPrice | decimal | Average entry price |
| closeAvgPrice | decimal | Average exit price |
| openAvgPrice | decimal | Average open price |
| liquidatePrice | decimal | Liquidation price |
| oim | decimal | Original initial margin |
| adlLevel | int | ADL level (1-5) |
| im | decimal | Initial margin |
| holdFee | decimal | Holding fee |
| realised | decimal | Realized PnL |
| createTime | long | Position open time |
| updateTime | long | Last update time |

---

### 4. Get Position History
**Endpoint:** `GET /api/v1/private/position/list/history_positions`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | No | Filter by symbol |
| type | No | Position type |
| page_num | No | Page number |
| page_size | No | Page size (max: 100) |

---

### 5. Submit Order
**Endpoint:** `POST /api/v1/private/order/submit`

**Request Body:**
```json
{
  "symbol": "BTC_USDT",
  "price": 50000,
  "vol": 1,
  "leverage": 10,
  "side": 1,
  "type": 1,
  "openType": 1,
  "positionId": 0,
  "externalOid": "unique_order_id",
  "stopLossPrice": 0,
  "takeProfitPrice": 0,
  "positionMode": 1,
  "reduceOnly": false
}
```

**Parameters:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| symbol | string | Yes | Contract symbol |
| price | decimal | Conditional | Order price (required for limit) |
| vol | decimal | Yes | Order volume (contracts) |
| leverage | int | No | Leverage (uses default if not set) |
| side | int | Yes | 1=open long, 2=close short, 3=open short, 4=close long |
| type | int | Yes | 1=limit, 2=post-only, 3=IOC, 4=FOK, 5=market, 6=market+IOC |
| openType | int | Yes | 1=isolated, 2=cross |
| positionId | long | No | Position ID (for close orders) |
| externalOid | string | No | Client order ID |
| stopLossPrice | decimal | No | Stop loss price |
| takeProfitPrice | decimal | No | Take profit price |
| positionMode | int | No | 1=one-way, 2=hedge |
| reduceOnly | bool | No | Reduce-only flag |

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| orderId | long | Server order ID |
| externalOid | string | Client order ID |

---

### 6. Submit Batch Orders
**Endpoint:** `POST /api/v1/private/order/submit_batch`

**Request Body:** Array of order objects (max 50)

---

### 7. Cancel Order
**Endpoint:** `POST /api/v1/private/order/cancel`

**Request Body:**
```json
{
  "symbol": "BTC_USDT",
  "orderId": 123456789
}
```

**OR with external order ID:**
```json
{
  "symbol": "BTC_USDT",
  "externalOid": "unique_order_id"
}
```

---

### 8. Cancel All Orders
**Endpoint:** `POST /api/v1/private/order/cancel_all`

**Request Body:**
```json
{
  "symbol": "BTC_USDT"
}
```

**Parameters:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| symbol | string | No | Symbol filter (cancels all if empty) |
| positionId | long | No | Position ID filter |

---

### 9. Get Open Orders
**Endpoint:** `GET /api/v1/private/order/list/open_orders/{symbol}`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | Yes | Contract symbol |
| page_num | No | Page number |
| page_size | No | Page size |

---

### 10. Get Order History
**Endpoint:** `GET /api/v1/private/order/list/history_orders`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | No | Contract symbol filter |
| states | No | Order states filter |
| category | No | Order category |
| start_time | No | Start timestamp |
| end_time | No | End timestamp |
| page_num | No | Page number |
| page_size | No | Page size (max: 100) |

---

### 11. Get Order Details
**Endpoint:** `GET /api/v1/private/order/get/{orderId}`

---

### 12. Get Order by External ID
**Endpoint:** `GET /api/v1/private/order/external/{symbol}/{externalOid}`

---

### 13. Change Leverage
**Endpoint:** `POST /api/v1/private/position/change_leverage`

**Request Body:**
```json
{
  "symbol": "BTC_USDT",
  "leverage": 20,
  "openType": 1,
  "positionType": 1
}
```

**Parameters:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| symbol | string | Yes | Contract symbol |
| leverage | int | Yes | New leverage value |
| openType | int | Yes | 1=isolated, 2=cross |
| positionType | int | No | 1=long, 2=short |

---

### 14. Change Position Mode
**Endpoint:** `POST /api/v1/private/position/change_position_mode`

**Request Body:**
```json
{
  "symbol": "BTC_USDT",
  "positionMode": 2
}
```

---

### 15. Change Margin
**Endpoint:** `POST /api/v1/private/position/change_margin`

**Request Body:**
```json
{
  "positionId": 123456,
  "amount": 100,
  "type": "ADD"
}
```

---

## Futures WebSocket API

### Connection
```
wss://contract.mexc.com/ws
```

### Ping/Pong
The server sends ping frames. Client must respond with pong to maintain connection.

```json
{"method": "ping"}
```

### Subscribe Format
```json
{
  "method": "sub.{channel}",
  "param": {
    "symbol": "BTC_USDT"
  }
}
```

### Unsubscribe Format
```json
{
  "method": "unsub.{channel}",
  "param": {
    "symbol": "BTC_USDT"
  }
}
```

### Public Channels

#### 1. Ticker Channel
```json
{
  "method": "sub.ticker",
  "param": {
    "symbol": "BTC_USDT"
  }
}
```

**Push Data:**
```json
{
  "channel": "push.ticker",
  "data": {
    "symbol": "BTC_USDT",
    "lastPrice": 50000.0,
    "bid1": 49999.0,
    "ask1": 50001.0,
    "volume24": 10000.0,
    "holdVol": 50000.0,
    "riseFallRate": 0.02,
    "fairPrice": 50000.5,
    "fundingRate": 0.0001,
    "indexPrice": 50000.3,
    "timestamp": 1234567890123
  },
  "symbol": "BTC_USDT",
  "ts": 1234567890123
}
```

#### 2. Order Book Depth Channel
```json
{
  "method": "sub.depth",
  "param": {
    "symbol": "BTC_USDT"
  }
}
```

**Initial Snapshot:**
```json
{
  "channel": "push.depth",
  "data": {
    "asks": [[price, qty, count], ...],
    "bids": [[price, qty, count], ...],
    "version": 123456
  },
  "symbol": "BTC_USDT",
  "ts": 1234567890123
}
```

**Incremental Update:**
```json
{
  "channel": "push.depth",
  "data": {
    "asks": [[price, qty, count], ...],
    "bids": [[price, qty, count], ...],
    "version": 123457
  },
  "symbol": "BTC_USDT",
  "ts": 1234567890124
}
```

#### 3. Full Depth Snapshot
```json
{
  "method": "sub.depth.full",
  "param": {
    "symbol": "BTC_USDT",
    "limit": 20
  }
}
```

#### 4. Trades Channel
```json
{
  "method": "sub.deal",
  "param": {
    "symbol": "BTC_USDT"
  }
}
```

**Push Data:**
```json
{
  "channel": "push.deal",
  "data": {
    "p": 50000.0,
    "v": 1.5,
    "T": 1,
    "t": 1234567890123
  },
  "symbol": "BTC_USDT",
  "ts": 1234567890123
}
```

#### 5. Kline Channel
```json
{
  "method": "sub.kline",
  "param": {
    "symbol": "BTC_USDT",
    "interval": "Min1"
  }
}
```

### Private Channels (Require Authentication)

#### Authentication
```json
{
  "method": "login",
  "param": {
    "apiKey": "your_api_key",
    "reqTime": 1234567890123,
    "signature": "hmac_sha256_signature"
  }
}
```

#### 1. Account Updates
```json
{
  "method": "sub.personal.asset"
}
```

#### 2. Position Updates
```json
{
  "method": "sub.personal.position"
}
```

#### 3. Order Updates
```json
{
  "method": "sub.personal.order"
}
```

---

## Part 2: Spot API

### Base Configuration

| Property | Value |
|----------|-------|
| REST Base URL | `https://api.mexc.com` |
| API Version | `/api/v3/` |
| WebSocket URL | `wss://wbs.mexc.com/ws` |

### Authentication

**Headers:**
```
X-MEXC-APIKEY: <your_api_key>
Content-Type: application/json
```

**Signature (for private endpoints):**
```javascript
const timestamp = Date.now();
const queryString = buildQueryString({ ...params, timestamp });
const signature = crypto
  .createHmac('sha256', apiSecret)
  .update(queryString)
  .digest('hex');

// Append signature to request: ?...&signature={signature}
```

---

## Spot Public Endpoints

### 1. Ping
**Endpoint:** `GET /api/v3/ping`

### 2. Server Time
**Endpoint:** `GET /api/v3/time`

**Response:**
```json
{
  "serverTime": 1234567890123
}
```

### 3. Exchange Information
**Endpoint:** `GET /api/v3/exchangeInfo`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | No | Single symbol |
| symbols | No | Multiple symbols (comma-separated) |

**Response Fields:**
```json
{
  "timezone": "UTC",
  "serverTime": 1234567890123,
  "symbols": [{
    "symbol": "BTCUSDT",
    "status": "TRADING",
    "baseAsset": "BTC",
    "baseAssetPrecision": 8,
    "quoteAsset": "USDT",
    "quotePrecision": 8,
    "quoteAssetPrecision": 8,
    "baseCommissionPrecision": 8,
    "quoteCommissionPrecision": 8,
    "orderTypes": ["LIMIT", "MARKET", "LIMIT_MAKER"],
    "isSpotTradingAllowed": true,
    "isMarginTradingAllowed": false,
    "permissions": ["SPOT"],
    "filters": [...],
    "maxQuoteAmount": "5000000",
    "makerCommission": "0.002",
    "takerCommission": "0.002"
  }]
}
```

### 4. Order Book
**Endpoint:** `GET /api/v3/depth`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | Yes | Symbol (e.g., "BTCUSDT") |
| limit | No | Depth limit (5, 10, 20, 50, 100, 500, 1000, 5000) |

**Response:**
```json
{
  "lastUpdateId": 1234567890,
  "bids": [["50000.00", "1.5"], ...],
  "asks": [["50001.00", "2.0"], ...]
}
```

### 5. Recent Trades
**Endpoint:** `GET /api/v3/trades`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | Yes | Symbol |
| limit | No | Number of trades (default: 500, max: 1000) |

### 6. Historical Trades
**Endpoint:** `GET /api/v3/historicalTrades`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | Yes | Symbol |
| limit | No | Default: 500, max: 1000 |
| fromId | No | Trade ID to fetch from |

### 7. Aggregate Trades
**Endpoint:** `GET /api/v3/aggTrades`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | Yes | Symbol |
| fromId | No | Start aggregate trade ID |
| startTime | No | Start timestamp |
| endTime | No | End timestamp |
| limit | No | Default: 500, max: 1000 |

### 8. Kline Data
**Endpoint:** `GET /api/v3/klines`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | Yes | Symbol |
| interval | Yes | Kline interval |
| startTime | No | Start timestamp |
| endTime | No | End timestamp |
| limit | No | Default: 500, max: 1000 |

**Interval Values:**
`1m`, `5m`, `15m`, `30m`, `60m`, `4h`, `1d`, `1w`, `1M`

### 9. Current Average Price
**Endpoint:** `GET /api/v3/avgPrice`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | Yes | Symbol |

### 10. 24hr Ticker Statistics
**Endpoint:** `GET /api/v3/ticker/24hr`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | No | Symbol (returns all if not specified) |

### 11. Price Ticker
**Endpoint:** `GET /api/v3/ticker/price`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | No | Symbol (returns all if not specified) |

### 12. Order Book Ticker
**Endpoint:** `GET /api/v3/ticker/bookTicker`

**Parameters:**
| Name | Required | Description |
|------|----------|-------------|
| symbol | No | Symbol (returns all if not specified) |

---

## Spot Private Endpoints

### 1. Test New Order (SIGNED)
**Endpoint:** `POST /api/v3/order/test`

Same parameters as New Order, but doesn't create actual order.

### 2. New Order (SIGNED)
**Endpoint:** `POST /api/v3/order`

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| symbol | string | Yes | Symbol |
| side | string | Yes | "BUY" or "SELL" |
| type | string | Yes | Order type |
| timeInForce | string | No | "GTC", "IOC", "FOK" |
| quantity | decimal | Conditional | Order quantity |
| quoteOrderQty | decimal | Conditional | Quote order quantity |
| price | decimal | Conditional | Order price |
| newClientOrderId | string | No | Client order ID |
| stopPrice | decimal | Conditional | Stop price |
| icebergQty | decimal | No | Iceberg quantity |
| newOrderRespType | string | No | "ACK", "RESULT", "FULL" |
| recvWindow | long | No | Max: 60000, default: 5000 |
| timestamp | long | Yes | Request timestamp |

**Order Types:**
- `LIMIT` - Requires: `timeInForce`, `quantity`, `price`
- `MARKET` - Requires: `quantity` OR `quoteOrderQty`
- `STOP_LOSS` - Requires: `quantity`, `stopPrice`
- `STOP_LOSS_LIMIT` - Requires: `timeInForce`, `quantity`, `price`, `stopPrice`
- `TAKE_PROFIT` - Requires: `quantity`, `stopPrice`
- `TAKE_PROFIT_LIMIT` - Requires: `timeInForce`, `quantity`, `price`, `stopPrice`
- `LIMIT_MAKER` - Requires: `quantity`, `price`

### 3. Cancel Order (SIGNED)
**Endpoint:** `DELETE /api/v3/order`

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| symbol | string | Yes | Symbol |
| orderId | long | Conditional | Order ID |
| origClientOrderId | string | Conditional | Original client order ID |
| newClientOrderId | string | No | New client order ID for cancel |

### 4. Cancel All Open Orders (SIGNED)
**Endpoint:** `DELETE /api/v3/openOrders`

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| symbol | string | Yes | Symbol |

### 5. Query Order (SIGNED)
**Endpoint:** `GET /api/v3/order`

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| symbol | string | Yes | Symbol |
| orderId | long | Conditional | Order ID |
| origClientOrderId | string | Conditional | Original client order ID |

### 6. Current Open Orders (SIGNED)
**Endpoint:** `GET /api/v3/openOrders`

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| symbol | string | Yes | Symbol |

### 7. All Orders (SIGNED)
**Endpoint:** `GET /api/v3/allOrders`

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| symbol | string | Yes | Symbol |
| orderId | long | No | Starting order ID |
| startTime | long | No | Start timestamp |
| endTime | long | No | End timestamp |
| limit | int | No | Default: 500, max: 1000 |

### 8. Account Information (SIGNED)
**Endpoint:** `GET /api/v3/account`

**Response:**
```json
{
  "makerCommission": 0,
  "takerCommission": 0,
  "buyerCommission": 0,
  "sellerCommission": 0,
  "canTrade": true,
  "canWithdraw": true,
  "canDeposit": true,
  "updateTime": null,
  "accountType": "SPOT",
  "balances": [
    {
      "asset": "USDT",
      "free": "1000.00",
      "locked": "0.00"
    }
  ],
  "permissions": ["SPOT"]
}
```

### 9. Account Trade List (SIGNED)
**Endpoint:** `GET /api/v3/myTrades`

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| symbol | string | Yes | Symbol |
| orderId | long | No | Order ID |
| startTime | long | No | Start timestamp |
| endTime | long | No | End timestamp |
| fromId | long | No | Trade ID to fetch from |
| limit | int | No | Default: 500, max: 1000 |

---

## Spot WebSocket API

### Connection
```
wss://wbs.mexc.com/ws
```

### Subscribe Format
```json
{
  "method": "SUBSCRIPTION",
  "params": [
    "spot@public.deals.v3.api@BTCUSDT",
    "spot@public.depth.v3.api@BTCUSDT@5"
  ]
}
```

### Channels

#### 1. Trades
```
spot@public.deals.v3.api@{symbol}
```

#### 2. Order Book Depth
```
spot@public.depth.v3.api@{symbol}@{levels}
```
Levels: 5, 10, 20

#### 3. Klines
```
spot@public.kline.v3.api@{symbol}@{interval}
```

---

## Rate Limits

### Futures API
- Public endpoints: 20 requests per second
- Private endpoints: 10 requests per second
- Order placement: 5 requests per second per symbol

### Spot API
- Weight-based system similar to Binance
- Check response headers for rate limit info:
  - `X-MBX-USED-WEIGHT-1M`: Used weight per minute
  - `X-MBX-ORDER-COUNT-1S`: Order count per second

---

## Error Codes

### Common Error Codes
| Code | Message | Description |
|------|---------|-------------|
| 0 | Success | Request successful |
| 401 | Not logged in | Authentication failed |
| 404 | Not Found | Endpoint not found |
| 500 | Internal Error | Server error |
| 10001 | Invalid parameter | Parameter validation failed |
| 10002 | Insufficient balance | Not enough funds |
| 10003 | Order not found | Order doesn't exist |
| 10004 | Position not found | Position doesn't exist |
| 10005 | Symbol not found | Invalid symbol |
| 10006 | Price out of range | Price exceeds limits |
| 10007 | Volume too small | Below minimum volume |
| 10008 | Volume too large | Exceeds maximum volume |
| 10009 | Leverage not allowed | Invalid leverage |
| 10010 | Position mode conflict | Wrong position mode |

---

## Code Examples

### Go Example - Sign Request for Futures
```go
package mexc

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "strconv"
    "time"
)

func generateSignature(apiKey, apiSecret string, timestamp int64, params string) string {
    message := apiKey + strconv.FormatInt(timestamp, 10) + params
    h := hmac.New(sha256.New, []byte(apiSecret))
    h.Write([]byte(message))
    return hex.EncodeToString(h.Sum(nil))
}

func buildHeaders(apiKey, apiSecret string, params string) map[string]string {
    timestamp := time.Now().UnixMilli()
    signature := generateSignature(apiKey, apiSecret, timestamp, params)
    
    return map[string]string{
        "ApiKey":       apiKey,
        "Request-Time": strconv.FormatInt(timestamp, 10),
        "Signature":    signature,
        "Content-Type": "application/json",
    }
}
```

### Go Example - Sign Request for Spot
```go
package mexc

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "net/url"
    "strconv"
    "time"
)

func signSpotRequest(apiSecret string, params url.Values) string {
    timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
    params.Set("timestamp", timestamp)
    
    queryString := params.Encode()
    h := hmac.New(sha256.New, []byte(apiSecret))
    h.Write([]byte(queryString))
    signature := hex.EncodeToString(h.Sum(nil))
    
    params.Set("signature", signature)
    return params.Encode()
}
```

---

## SDK References

- **JavaScript/TypeScript**: [mexc-api-sdk](https://github.com/mexcdevelop/mexc-api-sdk)
- **Python**: `from mexc_sdk import Spot`
- **Go**: `mexcsdk.NewSpot(&apiKey, &apiSecret)`
- **Java**: `new Spot(apiKey, apiSecret)`
- **C#**: `new Spot(apiKey, apiSecret)`

---

## Important Notes for Arbitrage Platform

1. **Symbol Format**: Use underscore separator for futures (e.g., "BTC_USDT"), no separator for spot (e.g., "BTCUSDT")

2. **Order Types for Low Slippage**:
   - Use `type=1` (LIMIT) for entry orders
   - Use `type=3` (IOC) for partial fills
   - Use `type=5` (MARKET) only for emergency exits

3. **Leverage Management**: Always set leverage via API before placing orders

4. **Position Modes**: 
   - `positionMode=1`: One-way mode
   - `positionMode=2`: Hedge mode (recommended for arbitrage)

5. **Funding Rate**: Check `collectCycle` for funding interval (typically 8 hours)

6. **WebSocket Reconnection**: Implement automatic reconnection with exponential backoff

7. **Rate Limiting**: Implement request queuing to avoid hitting rate limits

8. **Timestamp Sync**: Keep local clock synchronized with server time
