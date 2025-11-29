# Binance Futures API Documentation

## Overview

This document covers Binance USDT-Margined Futures API endpoints required for the CrossSpread arbitrage system.

## Base URLs

### Futures API (USDT-Margined)
| Environment | REST API | WebSocket Market Data | WebSocket API (Trading) | User Data Stream |
|-------------|----------|----------------------|------------------------|------------------|
| Production | `https://fapi.binance.com` | `wss://fstream.binance.com` | `wss://ws-fapi.binance.com/ws-fapi/v1` | `wss://fstream.binance.com/ws/<listenKey>` |
| Testnet | `https://demo-fapi.binance.com` | `wss://fstream.binancefuture.com` | `wss://testnet.binancefuture.com/ws-fapi/v1` | - |

### Spot API (for deposit/withdrawal status)
| Environment | REST API |
|-------------|----------|
| Production | `https://api.binance.com` |

---

## Authentication

### HMAC SHA256 Signature
```
Signature = HMAC_SHA256(secretKey, queryString + requestBody)
```

### Headers
```
X-MBX-APIKEY: <your_api_key>
```

### Request Parameters for Signed Endpoints
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| timestamp | LONG | YES | Millisecond timestamp when request was created |
| recvWindow | LONG | NO | Max milliseconds request is valid (default 5000) |
| signature | STRING | YES | HMAC SHA256 signature |

---

## Rate Limits

### IP Rate Limits
- Weight limit: 2400/minute
- Response header: `X-MBX-USED-WEIGHT-(intervalNum)(intervalLetter)`
- 429 = Rate limited, 418 = IP banned

### Order Rate Limits
- 300 orders/10 seconds
- 1200 orders/minute
- Response header: `X-MBX-ORDER-COUNT-(intervalNum)(intervalLetter)`

---

# REST API Endpoints

## Market Data (Public)

### 1. Exchange Information
Get trading rules and symbol information.

**Endpoint:** `GET /fapi/v1/exchangeInfo`  
**Weight:** 1

**Response:**
```json
{
  "timezone": "UTC",
  "serverTime": 1565613908500,
  "rateLimits": [...],
  "assets": [
    {
      "asset": "USDT",
      "marginAvailable": true,
      "autoAssetExchange": "0"
    }
  ],
  "symbols": [
    {
      "symbol": "BTCUSDT",
      "pair": "BTCUSDT",
      "contractType": "PERPETUAL",
      "deliveryDate": 4133404800000,
      "onboardDate": 1569398400000,
      "status": "TRADING",
      "baseAsset": "BTC",
      "quoteAsset": "USDT",
      "marginAsset": "USDT",
      "pricePrecision": 2,
      "quantityPrecision": 3,
      "baseAssetPrecision": 8,
      "quotePrecision": 8,
      "underlyingType": "COIN",
      "filters": [
        {
          "filterType": "PRICE_FILTER",
          "maxPrice": "4529764",
          "minPrice": "556.80",
          "tickSize": "0.10"
        },
        {
          "filterType": "LOT_SIZE",
          "maxQty": "1000",
          "minQty": "0.001",
          "stepSize": "0.001"
        },
        {
          "filterType": "MIN_NOTIONAL",
          "notional": "5"
        }
      ],
      "orderTypes": ["LIMIT", "MARKET", "STOP", "STOP_MARKET", "TAKE_PROFIT", "TAKE_PROFIT_MARKET", "TRAILING_STOP_MARKET"],
      "timeInForce": ["GTC", "IOC", "FOK", "GTX"],
      "liquidationFee": "0.012500",
      "marketTakeBound": "0.05"
    }
  ]
}
```

### 2. Order Book (Depth)
Get current orderbook depth.

**Endpoint:** `GET /fapi/v1/depth`  
**Weight:** Based on limit (5/10/20/50=2, 100=5, 500=10, 1000=20)

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | STRING | YES | e.g., "BTCUSDT" |
| limit | INT | NO | Default 500; Valid: 5, 10, 20, 50, 100, 500, 1000 |

**Response:**
```json
{
  "lastUpdateId": 1027024,
  "E": 1589436922972,
  "T": 1589436922959,
  "bids": [
    ["4.00000000", "431.00000000"]
  ],
  "asks": [
    ["4.00000200", "12.00000000"]
  ]
}
```

### 3. 24hr Ticker Price Change Statistics
Get 24-hour rolling window price change stats.

**Endpoint:** `GET /fapi/v1/ticker/24hr`  
**Weight:** 1 (single symbol), 40 (all symbols)

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | STRING | NO | If omitted, returns all symbols |

**Response:**
```json
{
  "symbol": "BTCUSDT",
  "priceChange": "-94.99999800",
  "priceChangePercent": "-95.960",
  "weightedAvgPrice": "0.29628482",
  "lastPrice": "4.00000200",
  "lastQty": "200.00000000",
  "openPrice": "99.00000000",
  "highPrice": "100.00000000",
  "lowPrice": "0.10000000",
  "volume": "8913.30000000",
  "quoteVolume": "15.30000000",
  "openTime": 1499783499040,
  "closeTime": 1499869899040,
  "firstId": 28385,
  "lastId": 28460,
  "count": 76
}
```

### 4. Mark Price & Funding Rate
Get mark price and funding rate.

**Endpoint:** `GET /fapi/v1/premiumIndex`  
**Weight:** 1 (single symbol), 10 (all symbols)

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | STRING | NO | If omitted, returns all symbols |

**Response:**
```json
{
  "symbol": "BTCUSDT",
  "markPrice": "11793.63104562",
  "indexPrice": "11781.80495970",
  "estimatedSettlePrice": "11781.16138815",
  "lastFundingRate": "0.00038246",
  "interestRate": "0.00010000",
  "nextFundingTime": 1597392000000,
  "time": 1597370495002
}
```

### 5. Funding Rate History
Get historical funding rates.

**Endpoint:** `GET /fapi/v1/fundingRate`  
**Weight:** Shared 500/5min/IP rate limit

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | STRING | NO | |
| startTime | LONG | NO | Timestamp in ms (inclusive) |
| endTime | LONG | NO | Timestamp in ms (inclusive) |
| limit | INT | NO | Default 100; max 1000 |

**Response:**
```json
[
  {
    "symbol": "BTCUSDT",
    "fundingRate": "-0.03750000",
    "fundingTime": 1570608000000,
    "markPrice": "34287.54619963"
  }
]
```

### 6. Kline/Candlestick Data
Get historical klines.

**Endpoint:** `GET /fapi/v1/klines`  
**Weight:** 1-10 based on limit

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | STRING | YES | |
| interval | ENUM | YES | 1m, 3m, 5m, 15m, 30m, 1h, 2h, 4h, 6h, 8h, 12h, 1d, 3d, 1w, 1M |
| startTime | LONG | NO | |
| endTime | LONG | NO | |
| limit | INT | NO | Default 500; max 1500 |

**Response:**
```json
[
  [
    1499040000000,      // Open time
    "0.01634790",       // Open
    "0.80000000",       // High
    "0.01575800",       // Low
    "0.01577100",       // Close
    "148976.11427815",  // Volume
    1499644799999,      // Close time
    "2434.19055334",    // Quote asset volume
    308,                // Number of trades
    "1756.87402397",    // Taker buy base asset volume
    "28.46694368",      // Taker buy quote asset volume
    "17928899.62484339" // Ignore
  ]
]
```

---

## Trading Endpoints (Signed)

### 1. New Order
Place a new order.

**Endpoint:** `POST /fapi/v1/order`  
**Weight:** 0 IP weight, 1 order rate limit

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | STRING | YES | |
| side | ENUM | YES | BUY, SELL |
| positionSide | ENUM | NO | BOTH (default), LONG, SHORT |
| type | ENUM | YES | LIMIT, MARKET, STOP, STOP_MARKET, TAKE_PROFIT, TAKE_PROFIT_MARKET, TRAILING_STOP_MARKET |
| timeInForce | ENUM | NO | GTC, IOC, FOK, GTX |
| quantity | DECIMAL | NO | |
| reduceOnly | STRING | NO | "true" or "false" |
| price | DECIMAL | NO | Required for LIMIT orders |
| newClientOrderId | STRING | NO | Unique ID |
| stopPrice | DECIMAL | NO | For STOP orders |
| workingType | ENUM | NO | MARK_PRICE, CONTRACT_PRICE |
| priceProtect | STRING | NO | "TRUE" or "FALSE" |
| newOrderRespType | ENUM | NO | ACK, RESULT |
| timestamp | LONG | YES | |

**Response:**
```json
{
  "orderId": 22542179,
  "symbol": "BTCUSDT",
  "status": "NEW",
  "clientOrderId": "testOrder",
  "price": "9000",
  "avgPrice": "0.00000",
  "origQty": "10",
  "executedQty": "0",
  "cumQty": "0",
  "cumQuote": "0",
  "timeInForce": "GTC",
  "type": "LIMIT",
  "reduceOnly": false,
  "closePosition": false,
  "side": "BUY",
  "positionSide": "BOTH",
  "stopPrice": "0",
  "workingType": "CONTRACT_PRICE",
  "priceProtect": false,
  "origType": "LIMIT",
  "updateTime": 1566818724722
}
```

### 2. Place Multiple Orders (Batch)
Place up to 5 orders at once.

**Endpoint:** `POST /fapi/v1/batchOrders`  
**Weight:** 5

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| batchOrders | LIST<JSON> | YES | Max 5 orders |
| timestamp | LONG | YES | |

**Example:**
```
/fapi/v1/batchOrders?batchOrders=[{"type":"LIMIT","timeInForce":"GTC","symbol":"BTCUSDT","side":"BUY","price":"10001","quantity":"0.001"}]
```

### 3. Cancel Order
Cancel an active order.

**Endpoint:** `DELETE /fapi/v1/order`  
**Weight:** 1

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | STRING | YES | |
| orderId | LONG | NO | Either orderId or origClientOrderId required |
| origClientOrderId | STRING | NO | |
| timestamp | LONG | YES | |

### 4. Query Order
Check order status.

**Endpoint:** `GET /fapi/v1/order`  
**Weight:** 1

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | STRING | YES | |
| orderId | LONG | NO | Either orderId or origClientOrderId required |
| origClientOrderId | STRING | NO | |
| timestamp | LONG | YES | |

---

## Account Endpoints (Signed)

### 1. Account Information V3
Get current account information.

**Endpoint:** `GET /fapi/v3/account`  
**Weight:** 5

**Response:**
```json
{
  "totalInitialMargin": "0.00000000",
  "totalMaintMargin": "0.00000000",
  "totalWalletBalance": "103.12345678",
  "totalUnrealizedProfit": "0.00000000",
  "totalMarginBalance": "103.12345678",
  "totalPositionInitialMargin": "0.00000000",
  "totalOpenOrderInitialMargin": "0.00000000",
  "totalCrossWalletBalance": "103.12345678",
  "totalCrossUnPnl": "0.00000000",
  "availableBalance": "103.12345678",
  "maxWithdrawAmount": "103.12345678",
  "assets": [
    {
      "asset": "USDT",
      "walletBalance": "23.72469206",
      "unrealizedProfit": "0.00000000",
      "marginBalance": "23.72469206",
      "maintMargin": "0.00000000",
      "initialMargin": "0.00000000",
      "positionInitialMargin": "0.00000000",
      "openOrderInitialMargin": "0.00000000",
      "crossWalletBalance": "23.72469206",
      "crossUnPnl": "0.00000000",
      "availableBalance": "23.72469206",
      "maxWithdrawAmount": "23.72469206",
      "updateTime": 1625474304765
    }
  ],
  "positions": [
    {
      "symbol": "BTCUSDT",
      "positionSide": "BOTH",
      "positionAmt": "1.000",
      "unrealizedProfit": "0.00000000",
      "isolatedMargin": "0.00000000",
      "notional": "0",
      "isolatedWallet": "0",
      "initialMargin": "0",
      "maintMargin": "0",
      "updateTime": 0
    }
  ]
}
```

### 2. Futures Account Balance V3
Get account balance.

**Endpoint:** `GET /fapi/v3/balance`  
**Weight:** 5

**Response:**
```json
[
  {
    "accountAlias": "SgsR",
    "asset": "USDT",
    "balance": "122607.35137903",
    "crossWalletBalance": "23.72469206",
    "crossUnPnl": "0.00000000",
    "availableBalance": "23.72469206",
    "maxWithdrawAmount": "23.72469206",
    "marginAvailable": true,
    "updateTime": 1617939110373
  }
]
```

### 3. User Commission Rate
Get user's commission/fee rates.

**Endpoint:** `GET /fapi/v1/commissionRate`  
**Weight:** 20

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | STRING | YES | |
| timestamp | LONG | YES | |

**Response:**
```json
{
  "symbol": "BTCUSDT",
  "makerCommissionRate": "0.0002",
  "takerCommissionRate": "0.0004"
}
```

### 4. Get Income History
Query income/PnL history.

**Endpoint:** `GET /fapi/v1/income`  
**Weight:** 30

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | STRING | NO | |
| incomeType | STRING | NO | TRANSFER, REALIZED_PNL, FUNDING_FEE, COMMISSION, etc. |
| startTime | LONG | NO | |
| endTime | LONG | NO | |
| limit | INT | NO | Default 100; max 1000 |
| timestamp | LONG | YES | |

**Response:**
```json
[
  {
    "symbol": "BTCUSDT",
    "incomeType": "REALIZED_PNL",
    "income": "-0.37500000",
    "asset": "USDT",
    "info": "TRADE",
    "time": 1570608000000,
    "tranId": 9689322392,
    "tradeId": "2059192"
  }
]
```

### 5. Leverage Brackets
Query notional and leverage brackets.

**Endpoint:** `GET /fapi/v1/leverageBracket`  
**Weight:** 1

**Response:**
```json
[
  {
    "symbol": "BTCUSDT",
    "brackets": [
      {
        "bracket": 1,
        "initialLeverage": 125,
        "notionalCap": 50000,
        "notionalFloor": 0,
        "maintMarginRatio": 0.004,
        "cum": 0.0
      }
    ]
  }
]
```

---

## User Data Stream

### Start User Data Stream
Create a listenKey for WebSocket user data stream.

**Endpoint:** `POST /fapi/v1/listenKey`  
**Weight:** 1

**Response:**
```json
{
  "listenKey": "pqia91ma19a5s61cv6a81va65sdf19v8a65a1a5s61cv6a81va65sdf19v8a65a1"
}
```

### Keepalive User Data Stream
Extend listenKey validity (60 minutes).

**Endpoint:** `PUT /fapi/v1/listenKey`  
**Weight:** 1

### Close User Data Stream
Close user data stream.

**Endpoint:** `DELETE /fapi/v1/listenKey`  
**Weight:** 1

---

# WebSocket Market Streams

**Base URL:** `wss://fstream.binance.com`

**Connection Methods:**
- Single stream: `/ws/<streamName>`
- Combined streams: `/stream?streams=<stream1>/<stream2>/<stream3>`

**Example:**
```
wss://fstream.binance.com/ws/btcusdt@aggTrade
wss://fstream.binance.com/stream?streams=btcusdt@depth@100ms/btcusdt@markPrice@1s
```

### Connection Rules
- Max 1024 streams per connection
- 24-hour connection limit
- 10 incoming messages/second limit
- Server sends ping every 3 minutes, must respond with pong within 10 minutes

---

## 1. Aggregate Trade Streams
Market trades aggregated by price and side every 100ms.

**Stream Name:** `<symbol>@aggTrade`

**Payload:**
```json
{
  "e": "aggTrade",
  "E": 123456789,
  "s": "BTCUSDT",
  "a": 5933014,
  "p": "0.001",
  "q": "100",
  "f": 100,
  "l": 105,
  "T": 123456785,
  "m": true
}
```

| Field | Description |
|-------|-------------|
| e | Event type |
| E | Event time |
| s | Symbol |
| a | Aggregate trade ID |
| p | Price |
| q | Quantity |
| f | First trade ID |
| l | Last trade ID |
| T | Trade time |
| m | Is buyer the market maker? |

---

## 2. Mark Price Stream
Mark price and funding rate updates.

**Stream Name:** `<symbol>@markPrice` or `<symbol>@markPrice@1s`  
**Update Speed:** 3000ms or 1000ms

**Payload:**
```json
{
  "e": "markPriceUpdate",
  "E": 1562305380000,
  "s": "BTCUSDT",
  "p": "11794.15000000",
  "i": "11784.62659091",
  "P": "11784.25641265",
  "r": "0.00038167",
  "T": 1562306400000
}
```

| Field | Description |
|-------|-------------|
| p | Mark price |
| i | Index price |
| P | Estimated settle price |
| r | Funding rate |
| T | Next funding time |

---

## 3. Kline/Candlestick Streams
Kline updates every 250ms.

**Stream Name:** `<symbol>@kline_<interval>`  
**Intervals:** 1m, 3m, 5m, 15m, 30m, 1h, 2h, 4h, 6h, 8h, 12h, 1d, 3d, 1w, 1M

**Payload:**
```json
{
  "e": "kline",
  "E": 1638747660000,
  "s": "BTCUSDT",
  "k": {
    "t": 1638747660000,
    "T": 1638747719999,
    "s": "BTCUSDT",
    "i": "1m",
    "f": 100,
    "L": 200,
    "o": "0.0010",
    "c": "0.0020",
    "h": "0.0025",
    "l": "0.0015",
    "v": "1000",
    "n": 100,
    "x": false,
    "q": "1.0000",
    "V": "500",
    "Q": "0.500"
  }
}
```

---

## 4. Individual Symbol Ticker Streams
24hr ticker for single symbol.

**Stream Name:** `<symbol>@ticker`  
**Update Speed:** 2000ms

**Payload:**
```json
{
  "e": "24hrTicker",
  "E": 123456789,
  "s": "BTCUSDT",
  "p": "0.0015",
  "P": "250.00",
  "w": "0.0018",
  "c": "0.0025",
  "Q": "10",
  "o": "0.0010",
  "h": "0.0025",
  "l": "0.0010",
  "v": "10000",
  "q": "18",
  "O": 0,
  "C": 86400000,
  "F": 0,
  "L": 18150,
  "n": 18151
}
```

---

## 5. All Market Tickers Streams
24hr ticker for all symbols.

**Stream Name:** `!ticker@arr`  
**Update Speed:** 1000ms

---

## 6. Partial Book Depth Streams
Top N levels of orderbook.

**Stream Name:** `<symbol>@depth<levels>` or `<symbol>@depth<levels>@100ms`  
**Levels:** 5, 10, 20  
**Update Speed:** 250ms, 500ms, or 100ms

**Payload:**
```json
{
  "e": "depthUpdate",
  "E": 1571889248277,
  "T": 1571889248276,
  "s": "BTCUSDT",
  "U": 390497796,
  "u": 390497878,
  "pu": 390497794,
  "b": [
    ["7403.89", "0.002"],
    ["7403.90", "3.906"]
  ],
  "a": [
    ["7405.96", "3.340"],
    ["7406.63", "4.525"]
  ]
}
```

---

## 7. Diff. Book Depth Streams
Orderbook diff updates.

**Stream Name:** `<symbol>@depth` or `<symbol>@depth@100ms`  
**Update Speed:** 250ms, 500ms, or 100ms

---

# WebSocket API (Low-Latency Trading)

**Base URL:** `wss://ws-fapi.binance.com/ws-fapi/v1`

The WebSocket API allows placing orders with lower latency than REST API.

## Authentication

### Session Login
```json
{
  "id": "c174a2b1-3f51-4580-b200-8528bd237cb7",
  "method": "session.logon",
  "params": {
    "apiKey": "vmPUZE6mv9SD5VNHk4HlWFsOr6aKE2zvsw0MuIgwCIPy6utIco14y7Ju91duEh8A",
    "signature": "1cf54395b336b0a9727ef27d5d98987962bc47aca6e13fe978612d0adee066ed",
    "timestamp": 1649729878532
  }
}
```

## Place Order
**Method:** `order.place`

**Request:**
```json
{
  "id": "3f7df6e3-2df4-44b9-9919-d2f38f90a99a",
  "method": "order.place",
  "params": {
    "apiKey": "HMOchcfii9ZRZnhjp2XjGXhsOBd6msAhKz9joQaWwZ7arcJTlD2hGPHQj1lGdTjR",
    "positionSide": "BOTH",
    "price": 43187.00,
    "quantity": 0.1,
    "side": "BUY",
    "symbol": "BTCUSDT",
    "timeInForce": "GTC",
    "timestamp": 1702555533821,
    "type": "LIMIT",
    "signature": "0f04368b2d22aafd0ggc8809ea34297eff602272917b5f01267db4efbc1c9422"
  }
}
```

**Response:**
```json
{
  "id": "3f7df6e3-2df4-44b9-9919-d2f38f90a99a",
  "status": 200,
  "result": {
    "orderId": 325078477,
    "symbol": "BTCUSDT",
    "status": "NEW",
    "clientOrderId": "iCXL1BywlBaf2sesNUrVl3",
    "price": "43187.00",
    "avgPrice": "0.00",
    "origQty": "0.100",
    "executedQty": "0.000",
    "cumQty": "0.000",
    "cumQuote": "0.00000",
    "timeInForce": "GTC",
    "type": "LIMIT",
    "side": "BUY",
    "positionSide": "BOTH",
    "updateTime": 1702555534435
  },
  "rateLimits": [...]
}
```

---

# User Data Stream Events

**Connect:** `wss://fstream.binance.com/ws/<listenKey>`

## ORDER_TRADE_UPDATE Event
Pushed when order status changes.

**Payload:**
```json
{
  "e": "ORDER_TRADE_UPDATE",
  "E": 1568879465651,
  "T": 1568879465650,
  "o": {
    "s": "BTCUSDT",
    "c": "TEST",
    "S": "SELL",
    "o": "LIMIT",
    "f": "GTC",
    "q": "0.001",
    "p": "9000",
    "ap": "0",
    "sp": "0",
    "x": "NEW",
    "X": "NEW",
    "i": 8886774,
    "l": "0",
    "z": "0",
    "L": "0",
    "N": "USDT",
    "n": "0",
    "T": 1568879465650,
    "t": 0,
    "b": "0",
    "a": "9.91",
    "m": false,
    "R": false,
    "wt": "CONTRACT_PRICE",
    "ot": "LIMIT",
    "ps": "BOTH",
    "cp": false,
    "rp": "0"
  }
}
```

| Field | Description |
|-------|-------------|
| s | Symbol |
| c | Client order ID |
| S | Side (BUY/SELL) |
| o | Order type |
| f | Time in force |
| q | Original quantity |
| p | Original price |
| ap | Average price |
| x | Execution type (NEW, CANCELED, TRADE, EXPIRED) |
| X | Order status (NEW, PARTIALLY_FILLED, FILLED, CANCELED, EXPIRED) |
| i | Order ID |
| l | Last filled quantity |
| z | Cumulative filled quantity |
| L | Last filled price |
| n | Commission |
| N | Commission asset |
| rp | Realized profit |

---

# Spot API - Deposit/Withdrawal Status

## All Coins Information
Get deposit/withdrawal availability for all coins.

**Endpoint:** `GET /sapi/v1/capital/config/getall`  
**Weight:** 10

**Response:**
```json
[
  {
    "coin": "BTC",
    "depositAllEnable": true,
    "withdrawAllEnable": true,
    "name": "Bitcoin",
    "free": "0.08074558",
    "locked": "0",
    "freeze": "0",
    "withdrawing": "0",
    "trading": true,
    "networkList": [
      {
        "network": "BTC",
        "coin": "BTC",
        "withdrawIntegerMultiple": "0.00000001",
        "isDefault": true,
        "depositEnable": true,
        "withdrawEnable": true,
        "depositDesc": "",
        "withdrawDesc": "",
        "name": "Bitcoin",
        "withdrawFee": "0.0005",
        "withdrawMin": "0.001",
        "withdrawMax": "7500",
        "minConfirm": 1,
        "unLockConfirm": 2,
        "estimatedArrivalTime": 30,
        "busy": false
      }
    ]
  }
]
```

---

# Error Codes

| Code | Description |
|------|-------------|
| -1000 | Unknown error |
| -1001 | Disconnected |
| -1002 | Unauthorized |
| -1003 | Too many requests |
| -1008 | System throttled |
| -1015 | Too many orders |
| -1021 | Timestamp outside recvWindow |
| -1022 | Invalid signature |
| -1102 | Mandatory parameter missing |
| -1121 | Invalid symbol |
| -2010 | New order rejected |
| -2011 | Cancel rejected |
| -2013 | Order does not exist |
| -2014 | API-key format invalid |
| -2015 | Invalid API-key/IP/permissions |
| -2019 | Margin insufficient |
| -2021 | Order would immediately trigger |
| -4003 | Quantity less than min |
| -4014 | Price less than min |
| -4015 | Price greater than max |

---

# Implementation Notes

1. **Rate Limiting**: Use WebSocket streams for market data to avoid REST API rate limits
2. **Order Placement**: Use WebSocket API (`ws-fapi`) for lowest latency order placement
3. **Depth Management**: Subscribe to `@depth@100ms` for fastest orderbook updates
4. **Position Tracking**: Use user data stream for real-time position/order updates
5. **Timestamp Sync**: Ensure server time sync, use `/fapi/v1/time` to check
6. **Signature**: Always put signature as the last parameter in query string
