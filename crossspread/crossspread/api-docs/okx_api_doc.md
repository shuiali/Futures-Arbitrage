# OKX API Documentation for Futures Arbitrage System

## Overview

**Base URLs:**
- REST API: `https://www.okx.com`
- WebSocket Public: `wss://ws.okx.com:8443/ws/v5/public`
- WebSocket Private: `wss://ws.okx.com:8443/ws/v5/private`
- WebSocket Business: `wss://ws.okx.com:8443/ws/v5/business`
- Demo Trading REST: `https://www.okx.com` (with `x-simulated-trading: 1` header)
- Demo Trading WebSocket: `wss://wspap.okx.com:8443/ws/v5/public`

**API Version:** V5

**Authentication:**
- API Key
- Secret Key
- Passphrase
- Signature: `sign = base64(hmac_sha256(timestamp + method + requestPath + body, secretKey))`

**Rate Limits:**
- IP-based and User ID-based rate limits
- Different limits per endpoint

---

## Instrument Types

| Type | Description |
|------|-------------|
| `SPOT` | Spot trading |
| `MARGIN` | Margin trading |
| `SWAP` | Perpetual futures |
| `FUTURES` | Expiry futures |
| `OPTION` | Options |

---

## REST API Endpoints

### 1. Public Data - Instruments

#### Get Instruments
Get all available trading instruments.

**Endpoint:** `GET /api/v5/public/instruments`

**Rate Limit:** 20 requests per 2 seconds (IP)

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| instType | String | Yes | SPOT, MARGIN, SWAP, FUTURES, OPTION |
| instFamily | String | Conditional | e.g., BTC-USD. Required for FUTURES/SWAP/OPTION |
| instId | String | No | Instrument ID |

**Response:**
```json
{
  "code": "0",
  "data": [{
    "instId": "BTC-USDT-SWAP",
    "instType": "SWAP",
    "baseCcy": "BTC",
    "quoteCcy": "USDT",
    "settleCcy": "USDT",
    "ctVal": "0.01",
    "ctMult": "1",
    "ctValCcy": "BTC",
    "lotSz": "1",
    "tickSz": "0.1",
    "minSz": "1",
    "lever": "125",
    "state": "live",
    "listTime": "1606468572000",
    "expTime": "",
    "groupId": "2"
  }]
}
```

**Key Response Fields:**
- `instId`: Instrument ID (e.g., BTC-USDT-SWAP)
- `ctVal`: Contract value per contract
- `ctMult`: Contract multiplier
- `tickSz`: Minimum price increment
- `lotSz`: Minimum order size
- `minSz`: Minimum order quantity
- `lever`: Maximum leverage
- `state`: live, suspend, preopen, settlement
- `groupId`: Fee group ID for trading fee calculation

---

### 2. Market Data

#### Get Tickers
Get all market tickers.

**Endpoint:** `GET /api/v5/market/tickers`

**Rate Limit:** 20 requests per 2 seconds (IP)

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| instType | String | Yes | SPOT, SWAP, FUTURES, OPTION |
| instFamily | String | No | Instrument family |

**Response:**
```json
{
  "code": "0",
  "data": [{
    "instId": "BTC-USDT-SWAP",
    "instType": "SWAP",
    "last": "43000.5",
    "lastSz": "10",
    "askPx": "43001.0",
    "askSz": "100",
    "bidPx": "43000.0",
    "bidSz": "150",
    "open24h": "42500.0",
    "high24h": "43500.0",
    "low24h": "42000.0",
    "volCcy24h": "50000",
    "vol24h": "5000000",
    "sodUtc0": "42800.0",
    "sodUtc8": "42700.0",
    "ts": "1597026383085"
  }]
}
```

#### Get Single Ticker
**Endpoint:** `GET /api/v5/market/ticker`

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| instId | String | Yes | Instrument ID |

---

#### Get Order Book
Get order book depth.

**Endpoint:** `GET /api/v5/market/books`

**Rate Limit:** 40 requests per 2 seconds (IP)

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| instId | String | Yes | Instrument ID |
| sz | String | No | Depth size (1-400, default 1) |

**Response:**
```json
{
  "code": "0",
  "data": [{
    "asks": [
      ["43001.0", "100", "0", "5"],
      ["43002.0", "150", "0", "3"]
    ],
    "bids": [
      ["43000.0", "200", "0", "8"],
      ["42999.0", "180", "0", "4"]
    ],
    "ts": "1597026383085"
  }]
}
```

**Order Book Array Format:** `[price, size, deprecated, orderCount]`

---

#### Get Candlesticks (Historical Price for Charts)
**Endpoint:** `GET /api/v5/market/candles`

**Rate Limit:** 40 requests per 2 seconds (IP)

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| instId | String | Yes | Instrument ID |
| bar | String | No | Bar size: 1m, 3m, 5m, 15m, 30m, 1H, 2H, 4H, 6H, 12H, 1D, 1W, 1M |
| after | String | No | Pagination - records earlier than ts |
| before | String | No | Pagination - records newer than ts |
| limit | String | No | Max 300, default 100 |

**Response:**
```json
{
  "code": "0",
  "data": [
    ["1597026383085", "43000", "43500", "42800", "43200", "1000000", "43000000", "43000000", "1"]
  ]
}
```

**Array Format:** `[ts, open, high, low, close, vol, volCcy, volCcyQuote, confirm]`

#### Get Historical Candlesticks
**Endpoint:** `GET /api/v5/market/history-candles`

Same parameters as above, retrieves data from recent years.

---

### 3. Funding Data

#### Get Funding Rate
**Endpoint:** `GET /api/v5/public/funding-rate`

**Rate Limit:** 20 requests per 2 seconds (IP + instId)

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| instId | String | Yes | Instrument ID (SWAP only) |

**Response:**
```json
{
  "code": "0",
  "data": [{
    "fundingRate": "0.0001",
    "fundingTime": "1703030400000",
    "instId": "BTC-USDT-SWAP",
    "instType": "SWAP",
    "method": "next_period",
    "nextFundingRate": "0.00015",
    "nextFundingTime": "1703059200000",
    "minFundingRate": "-0.00375",
    "maxFundingRate": "0.00375",
    "settState": "settled"
  }]
}
```

#### Get Funding Rate History
**Endpoint:** `GET /api/v5/public/funding-rate-history`

**Rate Limit:** 10 requests per 2 seconds (IP + instId)

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| instId | String | Yes | Instrument ID |
| before | String | No | Pagination |
| after | String | No | Pagination |
| limit | String | No | Max 100, default 100 |

**Response:**
```json
{
  "code": "0",
  "data": [{
    "fundingRate": "0.000227985782722",
    "fundingTime": "1703030400000",
    "instId": "BTC-USD-SWAP",
    "instType": "SWAP",
    "method": "next_period",
    "realizedRate": "0.0002279755647389"
  }]
}
```

---

### 4. Index Price

#### Get Index Tickers
**Endpoint:** `GET /api/v5/market/index-tickers`

**Rate Limit:** 20 requests per 2 seconds (IP)

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| quoteCcy | String | Conditional | Quote currency (USD/USDT/BTC/USDC) |
| instId | String | Conditional | Index ID (e.g., BTC-USD) |

**Response:**
```json
{
  "code": "0",
  "data": [{
    "instId": "BTC-USDT",
    "idxPx": "43350",
    "high24h": "43649.7",
    "low24h": "43261.9",
    "open24h": "43640.8",
    "sodUtc0": "43444.1",
    "sodUtc8": "43328.7",
    "ts": "1649419644492"
  }]
}
```

---

### 5. Trading Fees

#### Get Fee Rates
**Endpoint:** `GET /api/v5/account/trade-fee`

**Rate Limit:** 5 requests per 2 seconds (UserID)

**Permission:** Read

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| instType | String | Yes | SPOT, MARGIN, SWAP, FUTURES, OPTION |
| instId | String | No | Instrument ID (for SPOT/MARGIN) |
| instFamily | String | No | Instrument family (for FUTURES/SWAP/OPTION) |

**Response:**
```json
{
  "code": "0",
  "data": [{
    "level": "Lv1",
    "taker": "-0.001",
    "maker": "-0.0008",
    "takerU": "-0.0005",
    "makerU": "-0.0002",
    "delivery": "0.0005",
    "instType": "SWAP",
    "feeGroup": [{
      "groupId": "1",
      "maker": "-0.0008",
      "taker": "-0.001"
    }],
    "ts": "1763979985847"
  }]
}
```

**Note:** Negative values = fees charged, Positive values = rebates

---

### 6. Asset/Currency Information

#### Get Currencies
**Endpoint:** `GET /api/v5/asset/currencies`

**Rate Limit:** 6 requests per second (UserID)

**Permission:** Read

**Response:**
```json
{
  "code": "0",
  "data": [{
    "ccy": "BTC",
    "name": "Bitcoin",
    "chain": "BTC-Bitcoin",
    "canDep": true,
    "canWd": true,
    "canInternal": true,
    "minDep": "0.0001",
    "minWd": "0.001",
    "maxWd": "100",
    "wdTickSz": "8",
    "minFee": "0.0002",
    "maxFee": "0.0004",
    "mainNet": true,
    "needTag": false,
    "minDepArrivalConfirm": "1",
    "minWdUnlockConfirm": "2"
  }]
}
```

**Key Fields for Deposit/Withdrawal Status:**
- `canDep`: Can deposit
- `canWd`: Can withdraw
- `minDep`: Minimum deposit amount
- `minWd`: Minimum withdrawal amount
- `minFee`/`maxFee`: Withdrawal fee range

---

### 7. Trading - Order Placement

#### Place Order
**Endpoint:** `POST /api/v5/trade/order`

**Rate Limit:** 60 requests per 2 seconds (UserID)

**Permission:** Trade

**Request Body:**
```json
{
  "instId": "BTC-USDT-SWAP",
  "tdMode": "cross",
  "side": "buy",
  "ordType": "limit",
  "sz": "1",
  "px": "43000",
  "clOrdId": "myorder001",
  "tag": "arbitrage",
  "posSide": "long"
}
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| instId | String | Yes | Instrument ID |
| tdMode | String | Yes | Trade mode: cash, isolated, cross |
| side | String | Yes | buy, sell |
| ordType | String | Yes | market, limit, post_only, fok, ioc |
| sz | String | Yes | Quantity (contracts for futures) |
| px | String | Conditional | Price (required for limit orders) |
| clOrdId | String | No | Client order ID (max 32 chars) |
| tag | String | No | Order tag (max 16 chars) |
| posSide | String | Conditional | Position side: long, short, net |
| reduceOnly | Boolean | No | Reduce only flag |
| tgtCcy | String | No | Target currency: base_ccy, quote_ccy |

**Response:**
```json
{
  "code": "0",
  "data": [{
    "ordId": "312269865356374016",
    "clOrdId": "myorder001",
    "tag": "arbitrage",
    "sCode": "0",
    "sMsg": ""
  }]
}
```

#### Batch Place Orders
**Endpoint:** `POST /api/v5/trade/batch-orders`

**Rate Limit:** 300 requests per 2 seconds (UserID)

**Request Body:** Array of order objects (max 20)

---

#### Cancel Order
**Endpoint:** `POST /api/v5/trade/cancel-order`

**Rate Limit:** 60 requests per 2 seconds (UserID)

**Request Body:**
```json
{
  "instId": "BTC-USDT-SWAP",
  "ordId": "312269865356374016"
}
```

#### Batch Cancel Orders
**Endpoint:** `POST /api/v5/trade/cancel-batch-orders`

**Rate Limit:** 300 requests per 2 seconds (UserID)

---

#### Amend Order
**Endpoint:** `POST /api/v5/trade/amend-order`

**Rate Limit:** 60 requests per 2 seconds (UserID)

**Request Body:**
```json
{
  "instId": "BTC-USDT-SWAP",
  "ordId": "312269865356374016",
  "newSz": "2",
  "newPx": "43100"
}
```

---

### 8. Order Information

#### Get Order Details
**Endpoint:** `GET /api/v5/trade/order`

**Rate Limit:** 60 requests per 2 seconds (UserID)

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| instId | String | Yes | Instrument ID |
| ordId | String | Conditional | Order ID |
| clOrdId | String | Conditional | Client order ID |

**Response:**
```json
{
  "code": "0",
  "data": [{
    "instId": "BTC-USDT-SWAP",
    "ordId": "312269865356374016",
    "clOrdId": "myorder001",
    "px": "43000",
    "sz": "1",
    "ordType": "limit",
    "side": "buy",
    "posSide": "long",
    "tdMode": "cross",
    "fillPx": "42999.5",
    "fillSz": "1",
    "fillTime": "1708587373361",
    "avgPx": "42999.5",
    "state": "filled",
    "lever": "10",
    "fee": "-0.00043",
    "feeCcy": "USDT",
    "pnl": "0",
    "accFillSz": "1",
    "cTime": "1708587373361",
    "uTime": "1708587373362"
  }]
}
```

**Order States:**
- `live`: Active order
- `partially_filled`: Partially filled
- `filled`: Fully filled
- `canceled`: Canceled
- `mmp_canceled`: Canceled by MMP

---

#### Get Order List (Active Orders)
**Endpoint:** `GET /api/v5/trade/orders-pending`

**Rate Limit:** 60 requests per 2 seconds (UserID)

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| instType | String | No | SPOT, MARGIN, SWAP, FUTURES, OPTION |
| instId | String | No | Instrument ID |
| ordType | String | No | Order type filter |
| state | String | No | live, partially_filled |
| limit | String | No | Max 100, default 100 |

---

#### Get Order History (Last 7 Days)
**Endpoint:** `GET /api/v5/trade/orders-history`

**Rate Limit:** 40 requests per 2 seconds (UserID)

#### Get Order History (Last 3 Months)
**Endpoint:** `GET /api/v5/trade/orders-history-archive`

**Rate Limit:** 20 requests per 2 seconds (UserID)

---

### 9. Account Information

#### Get Balance
**Endpoint:** `GET /api/v5/account/balance`

**Rate Limit:** 10 requests per 2 seconds (UserID)

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| ccy | String | No | Currency filter (comma-separated, max 20) |

**Response:**
```json
{
  "code": "0",
  "data": [{
    "adjEq": "55415.624719833286",
    "totalEq": "55868.06403501676",
    "isoEq": "0",
    "ordFroz": "0",
    "imr": "0",
    "mmr": "0",
    "mgnRatio": "",
    "notionalUsd": "0",
    "details": [{
      "ccy": "USDT",
      "eq": "4992.890093622894",
      "cashBal": "4850.435693622894",
      "availBal": "4834.317093622894",
      "frozenBal": "158.573",
      "availEq": "4834.3170936228935",
      "upl": "0",
      "eqUsd": "4991.542013297616"
    }],
    "uTime": "1705564223311"
  }]
}
```

---

#### Get Positions
**Endpoint:** `GET /api/v5/account/positions`

**Rate Limit:** 10 requests per 2 seconds (UserID)

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| instType | String | No | MARGIN, SWAP, FUTURES, OPTION |
| instId | String | No | Instrument ID |
| posId | String | No | Position ID |

**Response:**
```json
{
  "code": "0",
  "data": [{
    "instId": "BTC-USDT-SWAP",
    "instType": "SWAP",
    "posId": "307173036051017730",
    "posSide": "long",
    "pos": "10",
    "availPos": "10",
    "avgPx": "42000",
    "markPx": "43000",
    "upl": "100",
    "uplRatio": "0.0238",
    "lever": "10",
    "mgnMode": "cross",
    "margin": "420",
    "mgnRatio": "11.73",
    "imr": "420",
    "mmr": "42",
    "liqPx": "38000",
    "notionalUsd": "4300",
    "adl": "1",
    "cTime": "1619507758793",
    "uTime": "1619507761462",
    "realizedPnl": "50",
    "fee": "-5",
    "fundingFee": "-2"
  }]
}
```

**Key Position Fields:**
- `pos`: Position quantity (positive = long, negative = short for net mode)
- `avgPx`: Average entry price
- `markPx`: Current mark price
- `upl`: Unrealized PnL
- `uplRatio`: Unrealized PnL ratio
- `liqPx`: Liquidation price
- `realizedPnl`: Realized PnL including fees and funding

---

### 10. Account Configuration

#### Get Account Configuration
**Endpoint:** `GET /api/v5/account/config`

**Rate Limit:** 5 requests per 2 seconds (UserID)

**Response:**
```json
{
  "code": "0",
  "data": [{
    "uid": "44705892343619584",
    "acctLv": "2",
    "posMode": "long_short_mode",
    "autoLoan": true,
    "greeksType": "PA",
    "level": "Lv1",
    "levelTmp": "",
    "ctIsoMode": "automatic",
    "mgnIsoMode": "automatic"
  }]
}
```

#### Set Position Mode
**Endpoint:** `POST /api/v5/account/set-position-mode`

**Request Body:**
```json
{
  "posMode": "long_short_mode"
}
```

**Position Modes:**
- `long_short_mode`: Hedge mode (separate long/short positions)
- `net_mode`: Net mode (single net position)

#### Set Leverage
**Endpoint:** `POST /api/v5/account/set-leverage`

**Request Body:**
```json
{
  "instId": "BTC-USDT-SWAP",
  "lever": "10",
  "mgnMode": "cross"
}
```

---

## WebSocket API

### Connection

**Public Channels:**
```
wss://ws.okx.com:8443/ws/v5/public
```

**Private Channels:**
```
wss://ws.okx.com:8443/ws/v5/private
```

**Business Channels:**
```
wss://ws.okx.com:8443/ws/v5/business
```

### Authentication (Private Channels)

**Login Request:**
```json
{
  "op": "login",
  "args": [{
    "apiKey": "your-api-key",
    "passphrase": "your-passphrase",
    "timestamp": "1538054050",
    "sign": "base64-signature"
  }]
}
```

**Signature:** `sign = base64(hmac_sha256(timestamp + 'GET' + '/users/self/verify', secretKey))`

---

### Public Channels

#### 1. Tickers Channel (Real-time Price)
**Subscribe:**
```json
{
  "op": "subscribe",
  "args": [{
    "channel": "tickers",
    "instId": "BTC-USDT-SWAP"
  }]
}
```

**Push Data:**
```json
{
  "arg": {
    "channel": "tickers",
    "instId": "BTC-USDT-SWAP"
  },
  "data": [{
    "instId": "BTC-USDT-SWAP",
    "last": "43000.5",
    "lastSz": "10",
    "askPx": "43001.0",
    "askSz": "100",
    "bidPx": "43000.0",
    "bidSz": "150",
    "open24h": "42500.0",
    "high24h": "43500.0",
    "low24h": "42000.0",
    "vol24h": "5000000",
    "volCcy24h": "50000",
    "ts": "1597026383085"
  }]
}
```

**Push Frequency:** Up to 100ms

---

#### 2. Order Book Channel (Real-time Depth)
**Subscribe:**
```json
{
  "op": "subscribe",
  "args": [{
    "channel": "books",
    "instId": "BTC-USDT-SWAP"
  }]
}
```

**Channel Options:**
- `books`: 400 levels, incremental updates every 100ms
- `books5`: 5 levels, snapshot every 200ms
- `bbo-tbt`: Best bid/offer, tick-by-tick
- `books50-l2-tbt`: 50 levels, tick-by-tick
- `books-l2-tbt`: 400 levels, tick-by-tick

**Snapshot Push:**
```json
{
  "arg": {
    "channel": "books",
    "instId": "BTC-USDT-SWAP"
  },
  "action": "snapshot",
  "data": [{
    "asks": [
      ["43001.0", "100", "0", "5"],
      ["43002.0", "150", "0", "3"]
    ],
    "bids": [
      ["43000.0", "200", "0", "8"],
      ["42999.0", "180", "0", "4"]
    ],
    "ts": "1597026383085",
    "checksum": -855196043,
    "seqId": 123456789
  }]
}
```

**Incremental Update:**
```json
{
  "arg": {
    "channel": "books",
    "instId": "BTC-USDT-SWAP"
  },
  "action": "update",
  "data": [{
    "asks": [
      ["43001.0", "120", "0", "6"]
    ],
    "bids": [],
    "ts": "1597026383185",
    "checksum": -855196044,
    "seqId": 123456790,
    "prevSeqId": 123456789
  }]
}
```

**Array Format:** `[price, size, deprecated, orderCount]`
- Size of "0" means level should be removed

---

#### 3. Candlesticks Channel
**Subscribe:**
```json
{
  "op": "subscribe",
  "args": [{
    "channel": "candle1m",
    "instId": "BTC-USDT-SWAP"
  }]
}
```

**Channel Options:** candle1s, candle1m, candle3m, candle5m, candle15m, candle30m, candle1H, candle2H, candle4H, candle6H, candle12H, candle1D, candle1W, candle1M

**Push Data:**
```json
{
  "arg": {
    "channel": "candle1m",
    "instId": "BTC-USDT-SWAP"
  },
  "data": [
    ["1597026383085", "43000", "43050", "42980", "43020", "1000", "43000000", "43000000", "0"]
  ]
}
```

---

#### 4. Trades Channel (Public Trades)
**Subscribe:**
```json
{
  "op": "subscribe",
  "args": [{
    "channel": "trades",
    "instId": "BTC-USDT-SWAP"
  }]
}
```

**Push Data:**
```json
{
  "arg": {
    "channel": "trades",
    "instId": "BTC-USDT-SWAP"
  },
  "data": [{
    "instId": "BTC-USDT-SWAP",
    "tradeId": "123456",
    "px": "43000.5",
    "sz": "10",
    "side": "buy",
    "ts": "1597026383085"
  }]
}
```

---

#### 5. Funding Rate Channel
**Subscribe:**
```json
{
  "op": "subscribe",
  "args": [{
    "channel": "funding-rate",
    "instId": "BTC-USDT-SWAP"
  }]
}
```

**Push Data:**
```json
{
  "arg": {
    "channel": "funding-rate",
    "instId": "BTC-USDT-SWAP"
  },
  "data": [{
    "fundingRate": "0.0001",
    "fundingTime": "1703030400000",
    "instId": "BTC-USDT-SWAP",
    "instType": "SWAP",
    "nextFundingRate": "0.00015",
    "nextFundingTime": "1703059200000"
  }]
}
```

---

#### 6. Index Tickers Channel
**Subscribe:**
```json
{
  "op": "subscribe",
  "args": [{
    "channel": "index-tickers",
    "instId": "BTC-USDT"
  }]
}
```

---

### Private Channels

#### 1. Account Channel
**Subscribe:**
```json
{
  "op": "subscribe",
  "args": [{
    "channel": "account"
  }]
}
```

**Push Data:**
```json
{
  "arg": {
    "channel": "account",
    "uid": "44705892343619584"
  },
  "data": [{
    "uTime": "1705564223311",
    "totalEq": "55868.06403501676",
    "adjEq": "55415.624719833286",
    "isoEq": "0",
    "ordFroz": "0",
    "imr": "0",
    "mmr": "0",
    "details": [{
      "ccy": "USDT",
      "eq": "4992.890093622894",
      "cashBal": "4850.435693622894",
      "availBal": "4834.317093622894",
      "frozenBal": "158.573",
      "upl": "0"
    }]
  }]
}
```

**Push Triggers:** Order placement, cancellation, fills, funding, etc.

---

#### 2. Positions Channel
**Subscribe:**
```json
{
  "op": "subscribe",
  "args": [{
    "channel": "positions",
    "instType": "SWAP"
  }]
}
```

**Push Data:**
```json
{
  "arg": {
    "channel": "positions",
    "instType": "SWAP",
    "uid": "44705892343619584"
  },
  "data": [{
    "instId": "BTC-USDT-SWAP",
    "instType": "SWAP",
    "posId": "307173036051017730",
    "posSide": "long",
    "pos": "10",
    "availPos": "10",
    "avgPx": "42000",
    "markPx": "43000",
    "upl": "100",
    "uplRatio": "0.0238",
    "lever": "10",
    "mgnMode": "cross",
    "liqPx": "38000",
    "realizedPnl": "50",
    "fee": "-5",
    "fundingFee": "-2",
    "pTime": "1619507761462"
  }]
}
```

---

#### 3. Balance and Position Channel (Combined)
**Subscribe:**
```json
{
  "op": "subscribe",
  "args": [{
    "channel": "balance_and_position"
  }]
}
```

**Push Data:**
```json
{
  "arg": {
    "channel": "balance_and_position",
    "uid": "44705892343619584"
  },
  "data": [{
    "pTime": "1597026383085",
    "eventType": "filled",
    "balData": [{
      "ccy": "USDT",
      "cashBal": "4850.435693622894",
      "uTime": "1597026383085"
    }],
    "posData": [{
      "posId": "307173036051017730",
      "instId": "BTC-USDT-SWAP",
      "instType": "SWAP",
      "mgnMode": "cross",
      "posSide": "long",
      "pos": "10",
      "avgPx": "42000"
    }]
  }]
}
```

**Event Types:** snapshot, delivered, exercised, transferred, filled, liquidation, funding_fee, settlement

---

#### 4. Orders Channel
**Subscribe:**
```json
{
  "op": "subscribe",
  "args": [{
    "channel": "orders",
    "instType": "SWAP"
  }]
}
```

**Push Data:**
```json
{
  "arg": {
    "channel": "orders",
    "instType": "SWAP",
    "uid": "44705892343619584"
  },
  "data": [{
    "instId": "BTC-USDT-SWAP",
    "ordId": "312269865356374016",
    "clOrdId": "myorder001",
    "px": "43000",
    "sz": "1",
    "ordType": "limit",
    "side": "buy",
    "posSide": "long",
    "tdMode": "cross",
    "fillPx": "42999.5",
    "fillSz": "1",
    "fillTime": "1708587373361",
    "avgPx": "42999.5",
    "state": "filled",
    "lever": "10",
    "fee": "-0.00043",
    "feeCcy": "USDT",
    "pnl": "0",
    "accFillSz": "1",
    "fillNotionalUsd": "42999.5",
    "execType": "M",
    "cTime": "1708587373361",
    "uTime": "1708587373362"
  }]
}
```

---

### WebSocket Trading (Low Latency)

#### Place Order via WebSocket
**URL:** `wss://ws.okx.com:8443/ws/v5/private`

**Request:**
```json
{
  "id": "1234",
  "op": "order",
  "args": [{
    "instId": "BTC-USDT-SWAP",
    "tdMode": "cross",
    "side": "buy",
    "ordType": "limit",
    "sz": "1",
    "px": "43000"
  }]
}
```

**Response:**
```json
{
  "id": "1234",
  "op": "order",
  "code": "0",
  "msg": "",
  "data": [{
    "ordId": "312269865356374016",
    "clOrdId": "",
    "tag": "",
    "sCode": "0",
    "sMsg": ""
  }]
}
```

#### Batch Orders via WebSocket
**Request:**
```json
{
  "id": "1234",
  "op": "batch-orders",
  "args": [
    {
      "instId": "BTC-USDT-SWAP",
      "tdMode": "cross",
      "side": "buy",
      "ordType": "limit",
      "sz": "1",
      "px": "43000"
    },
    {
      "instId": "BTC-USDT-SWAP",
      "tdMode": "cross",
      "side": "sell",
      "ordType": "limit",
      "sz": "1",
      "px": "43100"
    }
  ]
}
```

#### Cancel Order via WebSocket
**Request:**
```json
{
  "id": "1234",
  "op": "cancel-order",
  "args": [{
    "instId": "BTC-USDT-SWAP",
    "ordId": "312269865356374016"
  }]
}
```

#### Amend Order via WebSocket
**Request:**
```json
{
  "id": "1234",
  "op": "amend-order",
  "args": [{
    "instId": "BTC-USDT-SWAP",
    "ordId": "312269865356374016",
    "newSz": "2",
    "newPx": "43100"
  }]
}
```

---

## Important Notes for Arbitrage System

### 1. Instrument Naming Convention
- **SWAP (Perpetual):** `{BASE}-{QUOTE}-SWAP` (e.g., `BTC-USDT-SWAP`)
- **Futures:** `{BASE}-{QUOTE}-{EXPIRY}` (e.g., `BTC-USDT-231229`)
- **Spot:** `{BASE}-{QUOTE}` (e.g., `BTC-USDT`)

### 2. Contract Specifications
- `ctVal`: Contract value (e.g., 0.01 BTC per contract for BTC-USDT-SWAP)
- `ctMult`: Contract multiplier (usually 1)
- `lotSz`: Minimum order size increment
- `tickSz`: Minimum price increment

### 3. Position Size Calculation
```
Position Value (USDT) = pos × ctVal × markPx
Contracts = size_in_coins / ctVal
```

### 4. Slippage Estimation
Walk the order book to estimate fill prices:
```python
def estimate_fill_price(orderbook, side, size_in_coins, ctVal):
    levels = orderbook['asks'] if side == 'buy' else orderbook['bids']
    remaining = size_in_coins / ctVal  # Convert to contracts
    total_cost = 0
    filled = 0
    
    for price, qty, _, _ in levels:
        available = float(qty)
        fill_qty = min(remaining, available)
        total_cost += float(price) * fill_qty
        filled += fill_qty
        remaining -= fill_qty
        if remaining <= 0:
            break
    
    return total_cost / filled if filled > 0 else None
```

### 5. Funding Rate Collection
- Default: Every 8 hours (00:00, 08:00, 16:00 UTC)
- Can be more frequent for volatile pairs (6h, 4h, 2h, 1h)
- Check `fundingTime` and `nextFundingTime` for actual schedule

### 6. Rate Limits Summary
| Endpoint Type | Rate Limit |
|--------------|------------|
| Public data (IP) | 20-40 req/2s |
| Trading (UserID) | 60 req/2s |
| Batch trading | 300 req/2s |
| Account info | 5-10 req/2s |
| WebSocket connections | 3 per IP |
| WebSocket messages | 100/s per connection |

### 7. Error Codes to Handle
| Code | Description |
|------|-------------|
| 0 | Success |
| 50000 | Body cannot be empty |
| 50001 | Service temporarily unavailable |
| 51000 | Parameter error |
| 51001 | Instrument ID does not exist |
| 51004 | Order amount too small |
| 51008 | Order placement failed |
| 51010 | Account frozen |
| 51020 | Order count exceeds limit |
| 51121 | Order price too high/low |

### 8. WebSocket Keepalive
Send ping frame every 25 seconds to maintain connection:
```json
"ping"
```

Response:
```json
"pong"
```

---

## API Summary for Arbitrage Use Cases

### Fetching All Tokens and Their Info
1. `GET /api/v5/public/instruments` - Get all instruments
2. `GET /api/v5/market/tickers` - Get current prices and volume
3. `GET /api/v5/public/funding-rate` - Get funding rates (SWAP only)
4. `GET /api/v5/asset/currencies` - Get deposit/withdrawal status
5. `GET /api/v5/account/trade-fee` - Get trading fees

### Real-time Data (WebSocket)
1. `tickers` channel - Real-time price updates
2. `books` / `books-l2-tbt` channel - Order book depth
3. `funding-rate` channel - Funding rate updates

### Low-latency Order Execution (WebSocket)
1. `order` operation - Place single order
2. `batch-orders` operation - Place multiple orders
3. `cancel-order` operation - Cancel order
4. `amend-order` operation - Modify order

### Position and PnL Monitoring (WebSocket)
1. `positions` channel - Position updates
2. `account` channel - Balance updates
3. `orders` channel - Order status updates
4. `balance_and_position` channel - Combined updates

### Historical Data for Charts
1. `GET /api/v5/market/candles` - Recent candlesticks
2. `GET /api/v5/market/history-candles` - Historical candlesticks
3. `GET /api/v5/public/funding-rate-history` - Historical funding rates
