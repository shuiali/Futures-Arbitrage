# Bitget Futures API Complete Documentation

## Base URLs
| Service | URL |
|---------|-----|
| **REST API** | `https://api.bitget.com` |
| **WebSocket Public** | `wss://ws.bitget.com/v2/ws/public` |
| **WebSocket Private** | `wss://ws.bitget.com/v2/ws/private` |

## Product Types
| Type | Description |
|------|-------------|
| `USDT-FUTURES` | USDT-M Futures (perpetual, settled in USDT) |
| `USDC-FUTURES` | USDC-M Futures (perpetual, settled in USDC) |
| `COIN-FUTURES` | Coin-M Futures (perpetual, settled in crypto) |

---

## Authentication

### API Credentials
- Create API Key at: https://www.bitget.com (API Management)
- **API Key** + **Secret Key** + **Passphrase**
- Permissions: Read, Trade

### REST API Authentication Headers
| Header | Description |
|--------|-------------|
| `ACCESS-KEY` | Your API Key |
| `ACCESS-SIGN` | Base64(HMAC-SHA256(prehash, secretKey)) |
| `ACCESS-TIMESTAMP` | Unix timestamp in milliseconds |
| `ACCESS-PASSPHRASE` | API Passphrase |
| `Content-Type` | `application/json` |
| `locale` | `en-US` (optional) |

### Signature Generation (REST)
```
prehash = timestamp + method + requestPath + body
signature = Base64(HMAC_SHA256(prehash, secretKey))
```

**Example**:
```
Timestamp: 1684814440729
Method: POST
RequestPath: /api/v2/mix/order/place-order
Body: {"symbol":"BTCUSDT",...}

prehash = "1684814440729POST/api/v2/mix/order/place-order{...body...}"
signature = Base64(HMAC_SHA256(prehash, secretKey))
```

### WebSocket Authentication (Login)
```json
{
  "op": "login",
  "args": [{
    "apiKey": "your-api-key",
    "passphrase": "your-passphrase",
    "timestamp": "1695693150000",
    "sign": "Base64(HMAC_SHA256(timestamp + 'GET' + '/user/verify', secretKey))"
  }]
}
```

---

## REST API Endpoints

### 1. Market Data - Public

#### Get Contract Config (All Symbols)
**Rate Limit**: 20 req/sec/IP

```
GET /api/v2/mix/market/contracts
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| productType | String | Yes | `USDT-FUTURES`, `COIN-FUTURES`, `USDC-FUTURES` |
| symbol | String | No | Trading pair, e.g., `BTCUSDT` |

**Response Data**:
```json
{
  "symbol": "BTCUSDT",
  "baseCoin": "BTC",
  "quoteCoin": "USDT",
  "makerFeeRate": "0.0004",
  "takerFeeRate": "0.0006",
  "minTradeNum": "0.01",
  "pricePlace": "1",
  "volumePlace": "2",
  "sizeMultiplier": "0.01",
  "symbolType": "perpetual",
  "minLever": "1",
  "maxLever": "125",
  "fundInterval": "8"
}
```

#### Get All Tickers
**Rate Limit**: 20 req/sec/IP

```
GET /api/v2/mix/market/tickers
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| productType | String | Yes | `USDT-FUTURES`, `COIN-FUTURES`, `USDC-FUTURES` |

**Response Data**:
```json
{
  "symbol": "BTCUSDT",
  "lastPr": "29904.5",
  "askPr": "29904.5",
  "bidPr": "29903.5",
  "bidSz": "0.5091",
  "askSz": "2.2694",
  "high24h": "30200",
  "low24h": "29500",
  "change24h": "0.01",
  "baseVolume": "10000",
  "quoteVolume": "299000000",
  "usdtVolume": "299000000",
  "indexPrice": "29132.35",
  "fundingRate": "-0.0007",
  "holdingAmount": "125.6844",
  "markPrice": "29900",
  "ts": "1695794271400"
}
```

#### Get Single Ticker
**Rate Limit**: 20 req/sec/IP

```
GET /api/v2/mix/market/ticker
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | String | Yes | Trading pair |
| productType | String | Yes | Product type |

#### Get Merge Market Depth (Orderbook)
**Rate Limit**: 20 req/sec/IP

```
GET /api/v2/mix/market/merge-depth
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | String | Yes | Trading pair |
| productType | String | Yes | Product type |
| precision | String | No | `scale0/scale1/scale2/scale3` (scale0 = no merge) |
| limit | String | No | `1/5/15/50/max` (default: 100) |

**Response Data**:
```json
{
  "asks": [["26347.5", "0.25"], ["26348.0", "0.16"]],
  "bids": [["26346.5", "0.16"], ["26346.0", "0.32"]],
  "ts": "1695870968804",
  "scale": "0.1",
  "precision": "scale0"
}
```

#### Get Mark/Index/Market Prices
**Rate Limit**: 20 req/sec/UID

```
GET /api/v2/mix/market/symbol-price
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | String | Yes | Trading pair |
| productType | String | Yes | Product type |

**Response Data**:
```json
{
  "symbol": "BTCUSDT",
  "price": "26242",
  "indexPrice": "34867",
  "markPrice": "25555",
  "ts": "1695793390482"
}
```

#### Get Candlestick Data
**Rate Limit**: 20 req/sec/IP

```
GET /api/v2/mix/market/candles
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | String | Yes | Trading pair |
| productType | String | Yes | Product type |
| granularity | String | Yes | `1m/3m/5m/15m/30m/1H/4H/6H/12H/1D/3D/1W/1M` (also UTC variants: `6Hutc/12Hutc/1Dutc/3Dutc/1Wutc/1Mutc`) |
| startTime | String | No | Unix ms timestamp |
| endTime | String | No | Unix ms timestamp |
| kLineType | String | No | `MARKET/MARK/INDEX` (default: MARKET) |
| limit | String | No | Default: 100, Max: 1000 |

**Response Data** (Array format):
```
[timestamp, open, high, low, close, baseVolume, quoteVolume]
```

#### Get Historical Candlestick
**Rate Limit**: 20 req/sec/IP

```
GET /api/v2/mix/market/history-candles
```

Same parameters as candles, Max limit: 200

#### Get Recent Transactions (Trades)
**Rate Limit**: 20 req/sec/IP

```
GET /api/v2/mix/market/fills
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | String | Yes | Trading pair |
| productType | String | Yes | Product type |
| limit | String | No | Default: 100, Max: 100 |

**Response Data**:
```json
{
  "tradeId": "1",
  "price": "29990.5",
  "size": "0.0166",
  "side": "sell",
  "ts": "1627116776464",
  "symbol": "BTCUSDT"
}
```

### 2. Funding Rate

#### Get Current Funding Rate
**Rate Limit**: 20 req/sec/IP

```
GET /api/v2/mix/market/current-fund-rate
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | String | No | Trading pair (omit for all) |
| productType | String | Yes | Product type |

**Response Data**:
```json
{
  "symbol": "BTCUSDT",
  "fundingRate": "0.000068",
  "fundingRateInterval": "8",
  "nextUpdate": "1743062400000",
  "minFundingRate": "-0.003",
  "maxFundingRate": "0.003"
}
```

#### Get Historical Funding Rates
**Rate Limit**: 20 req/sec/IP

```
GET /api/v2/mix/market/history-fund-rate
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | String | Yes | Trading pair |
| productType | String | Yes | Product type |
| pageSize | String | No | Default: 20, Max: 100 |
| pageNo | String | No | Page number |

### 3. Fee Rates

#### Get VIP Fee Rate
**Rate Limit**: 10 req/sec/IP

```
GET /api/v2/mix/market/vip-fee-rate
```

**Response Data**:
```json
{
  "level": "1",
  "dealAmount": "100000",
  "assetAmount": "50000",
  "takerFeeRate": "0.0006",
  "makerFeeRate": "0.0004",
  "btcWithdrawAmount": "300",
  "usdtWithdrawAmount": "5000000"
}
```

### 4. Account Endpoints (Private)

#### Get Single Account
**Rate Limit**: 10 req/sec/UID

```
GET /api/v2/mix/account/account
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | String | Yes | Trading pair |
| productType | String | Yes | Product type |
| marginCoin | String | Yes | Margin coin |

**Response Data**:
```json
{
  "marginCoin": "USDT",
  "locked": "0",
  "available": "13168.86",
  "crossedMaxAvailable": "13168.86",
  "isolatedMaxAvailable": "13168.86",
  "maxTransferOut": "13168.86",
  "accountEquity": "13178.86",
  "usdtEquity": "13178.86",
  "btcEquity": "0.344746",
  "crossedRiskRate": "0",
  "crossedMarginLeverage": "20",
  "isolatedLongLever": "20",
  "isolatedShortLever": "20",
  "marginMode": "crossed",
  "posMode": "hedge_mode",
  "unrealizedPL": "",
  "crossedUnrealizedPL": "23",
  "isolatedUnrealizedPL": "0"
}
```

#### Get Account List
**Rate Limit**: 10 req/sec/UID

```
GET /api/v2/mix/account/accounts
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| productType | String | Yes | Product type |

#### Change Leverage
**Rate Limit**: 5 req/sec/UID

```
POST /api/v2/mix/account/set-leverage
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | String | Yes | Trading pair |
| productType | String | Yes | Product type |
| marginCoin | String | Yes | Margin coin (capitalized) |
| leverage | String | No | Leverage (for cross or one-way isolated) |
| longLeverage | String | No | Long leverage (hedge mode isolated) |
| shortLeverage | String | No | Short leverage (hedge mode isolated) |
| holdSide | String | No | `long/short` (required for hedge isolated) |

### 5. Position Endpoints (Private)

#### Get Historical Position
**Rate Limit**: 20 req/sec/UID

```
GET /api/v2/mix/position/history-position
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | String | No | Trading pair |
| productType | String | No | Default: USDT-FUTURES |
| idLessThan | String | No | Pagination (older data) |
| startTime | String | No | Unix ms timestamp |
| endTime | String | No | Unix ms timestamp |
| limit | String | No | Default: 20, Max: 100 |

**Response Data**:
```json
{
  "positionId": "xxx",
  "marginCoin": "USDT",
  "symbol": "BTCUSDT",
  "holdSide": "long",
  "openAvgPrice": "32000",
  "closeAvgPrice": "32500",
  "marginMode": "isolated",
  "openTotalPos": "0.01",
  "closeTotalPos": "0.01",
  "pnl": "14.1",
  "netProfit": "12.1",
  "totalFunding": "0.1",
  "openFee": "0.01",
  "closeFee": "0.01",
  "posMode": "one_way_mode",
  "ctime": "1988824171000",
  "utime": "1988824171000"
}
```

### 6. Trading Endpoints (Private)

#### Place Order
**Rate Limit**: 10 req/sec/UID (1 req/sec for copy traders)

```
POST /api/v2/mix/order/place-order
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | String | Yes | Trading pair (e.g., `ETHUSDT`) |
| productType | String | Yes | Product type |
| marginMode | String | Yes | `isolated/crossed` |
| marginCoin | String | Yes | Margin coin (capitalized) |
| size | String | Yes | Amount (base coin) |
| price | String | No | Required for limit orders |
| side | String | Yes | `buy/sell` |
| tradeSide | String | No | `open/close` (required in hedge mode) |
| orderType | String | Yes | `limit/market` |
| force | String | No | `gtc/ioc/fok/post_only` (required for limit) |
| clientOid | String | No | Custom order ID |
| reduceOnly | String | No | `YES/NO` (one-way mode only) |
| presetStopSurplusPrice | String | No | Take-profit price |
| presetStopLossPrice | String | No | Stop-loss price |
| stpMode | String | No | `none/cancel_taker/cancel_maker/cancel_both` |

**Position Modes**:
- **Hedge Mode**: 
  - Open long: `side=buy, tradeSide=open`
  - Close long: `side=buy, tradeSide=close`
  - Open short: `side=sell, tradeSide=open`
  - Close short: `side=sell, tradeSide=close`
- **One-Way Mode**: 
  - Buy: `side=buy`
  - Sell: `side=sell`
  - Ignore `tradeSide`

**Response Data**:
```json
{
  "orderId": "121211212122",
  "clientOid": "121211212122"
}
```

#### Batch Order
**Rate Limit**: 5 req/sec/UID

```
POST /api/v2/mix/order/batch-place-order
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | String | Yes | Trading pair |
| productType | String | Yes | Product type |
| marginCoin | String | Yes | Margin coin |
| marginMode | String | Yes | `isolated/crossed` |
| orderList | Array | Yes | Order list (max 50) |

#### Cancel Order
**Rate Limit**: 10 req/sec/UID

```
POST /api/v2/mix/order/cancel-order
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | String | Yes | Trading pair |
| productType | String | Yes | Product type |
| marginCoin | String | No | Margin coin |
| orderId | String | No | Order ID (or clientOid) |
| clientOid | String | No | Client order ID (or orderId) |

#### Get Pending Orders
**Rate Limit**: 10 req/sec/UID

```
GET /api/v2/mix/order/orders-pending
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| productType | String | Yes | Product type |
| symbol | String | No | Trading pair |
| orderId | String | No | Order ID |
| clientOid | String | No | Client order ID |
| status | String | No | `live/partially_filled` |
| idLessThan | String | No | Pagination |
| startTime | String | No | Unix ms timestamp |
| endTime | String | No | Unix ms timestamp |
| limit | String | No | Max: 100 |

**Response Data**:
```json
{
  "entrustedList": [{
    "symbol": "ETHUSDT",
    "size": "100",
    "orderId": "123",
    "clientOid": "12321",
    "baseVolume": "12.1",
    "price": "1900",
    "priceAvg": "1903",
    "status": "partially_filled",
    "side": "buy",
    "force": "gtc",
    "posSide": "long",
    "marginCoin": "USDT",
    "leverage": "20",
    "marginMode": "crossed",
    "tradeSide": "open",
    "posMode": "hedge_mode",
    "orderType": "limit",
    "cTime": "1627293504612",
    "uTime": "1627293505612"
  }],
  "endId": "123"
}
```

---

## WebSocket API

### 1. Connection & Heartbeat

**Ping/Pong**:
- Send `"ping"` text frame every 30 seconds
- Server responds with `"pong"`
- Connection closes after 2 minutes without ping

**Subscription Format**:
```json
{
  "op": "subscribe",
  "args": [{
    "instType": "USDT-FUTURES",
    "channel": "ticker",
    "instId": "BTCUSDT"
  }]
}
```

**Unsubscribe Format**:
```json
{
  "op": "unsubscribe",
  "args": [{
    "instType": "USDT-FUTURES",
    "channel": "ticker",
    "instId": "BTCUSDT"
  }]
}
```

### 2. Public Channels

#### Ticker Channel (Market)
**Channel**: `ticker`
**Update Frequency**: 300-400ms on change

```json
{
  "op": "subscribe",
  "args": [{
    "instType": "USDT-FUTURES",
    "channel": "ticker",
    "instId": "BTCUSDT"
  }]
}
```

**Push Data**:
```json
{
  "action": "snapshot",
  "arg": {"instType": "USDT-FUTURES", "channel": "ticker", "instId": "BTCUSDT"},
  "data": [{
    "instId": "BTCUSDT",
    "lastPr": "27000.5",
    "bidPr": "27000",
    "askPr": "27000.5",
    "bidSz": "2.71",
    "askSz": "8.76",
    "open24h": "27000.5",
    "high24h": "30668.5",
    "low24h": "26999.0",
    "change24h": "-0.00002",
    "fundingRate": "0.000010",
    "nextFundingTime": "1695722400000",
    "markPrice": "27000.0",
    "indexPrice": "25702.4",
    "holdingAmount": "929.502",
    "baseVolume": "368.900",
    "quoteVolume": "10152429.961",
    "ts": "1695715383021"
  }],
  "ts": 1695715383039
}
```

#### Depth Channel (Orderbook)
**Channels**: `books/books1/books5/books15`
**Update Frequency**: 
- `books/books5/books15`: 150ms
- `books1`: 100ms (20ms for BTC/ETH/XRP/SOL/SUI/DOGE/ADA/PEPE/LINK/HBAR)

```json
{
  "op": "subscribe",
  "args": [{
    "instType": "USDT-FUTURES",
    "channel": "books5",
    "instId": "BTCUSDT"
  }]
}
```

**Push Data**:
```json
{
  "action": "snapshot",
  "arg": {"instType": "USDT-FUTURES", "channel": "books5", "instId": "BTCUSDT"},
  "data": [{
    "asks": [["27000.5", "8.760"], ["27001.0", "0.400"]],
    "bids": [["27000.0", "2.710"], ["26999.5", "1.460"]],
    "checksum": 0,
    "seq": 123,
    "ts": "1695716059516"
  }],
  "ts": 1695716059516
}
```

**Checksum Verification**:
Build string from top 25 levels:
```
bid1_price:bid1_size:ask1_price:ask1_size:bid2_price:bid2_size:ask2_price:ask2_size...
```
Calculate CRC32 (32-bit signed integer).

**Incremental Updates** (books channel only):
- `action: "update"` contains changes
- Amount `0` = delete level
- Insert/update levels by price

#### Candlestick Channel
**Channels**: `candle1m/candle5m/candle15m/candle30m/candle1H/candle4H/candle12H/candle1D/candle1W/candle6H/candle3D/candle1M` (also UTC variants)

```json
{
  "op": "subscribe",
  "args": [{
    "instType": "USDT-FUTURES",
    "channel": "candle1m",
    "instId": "BTCUSDT"
  }]
}
```

**Push Data**:
```json
{
  "action": "snapshot",
  "arg": {"instType": "USDT-FUTURES", "channel": "candle1m", "instId": "BTCUSDT"},
  "data": [["1695685500000", "27000", "27000.5", "27000", "27000.5", "0.057", "1539.0155", "1539.0155"]],
  "ts": 1695715462250
}
```

Format: `[timestamp, open, high, low, close, baseVolume, quoteVolume, usdtVolume]`

#### Public Trade Channel
**Channel**: `trade`

```json
{
  "op": "subscribe",
  "args": [{
    "instType": "USDT-FUTURES",
    "channel": "trade",
    "instId": "BTCUSDT"
  }]
}
```

**Push Data**:
```json
{
  "action": "snapshot",
  "arg": {"instType": "USDT-FUTURES", "channel": "trade", "instId": "BTCUSDT"},
  "data": [{
    "ts": "1695716760565",
    "price": "27000.5",
    "size": "0.001",
    "side": "buy",
    "tradeId": "1111111111"
  }],
  "ts": 1695716761589
}
```

### 3. Private Channels (Require Login)

#### Account Channel
**Channel**: `account`

```json
{
  "op": "subscribe",
  "args": [{
    "instType": "USDT-FUTURES",
    "channel": "account",
    "coin": "default"
  }]
}
```

**Push Data**:
```json
{
  "action": "snapshot",
  "arg": {"instType": "USDT-FUTURES", "channel": "account", "coin": "default"},
  "data": [{
    "marginCoin": "USDT",
    "frozen": "0",
    "available": "11.985",
    "maxOpenPosAvailable": "11.985",
    "maxTransferOut": "11.985",
    "equity": "11.985",
    "usdtEquity": "11.985",
    "crossedRiskRate": "0",
    "unrealizedPL": "0"
  }],
  "ts": 1695717225146
}
```

#### Position Channel
**Channel**: `positions`

```json
{
  "op": "subscribe",
  "args": [{
    "instType": "USDT-FUTURES",
    "channel": "positions",
    "instId": "default"
  }]
}
```

**Push Data**:
```json
{
  "action": "snapshot",
  "arg": {"instType": "USDT-FUTURES", "channel": "positions", "instId": "default"},
  "data": [{
    "posId": "1",
    "instId": "ETHUSDT",
    "marginCoin": "USDT",
    "marginSize": "9.5",
    "marginMode": "crossed",
    "holdSide": "short",
    "posMode": "hedge_mode",
    "total": "0.1",
    "available": "0.1",
    "frozen": "0",
    "openPriceAvg": "1900",
    "leverage": 20,
    "achievedProfits": "0",
    "unrealizedPL": "0",
    "unrealizedPLR": "0",
    "liquidationPrice": "5788.108",
    "keepMarginRate": "0.005",
    "markPrice": "2500",
    "breakEvenPrice": "24778.97",
    "cTime": "1695649246169",
    "uTime": "1695711602568"
  }],
  "ts": 1695717430441
}
```

#### Order Channel
**Channel**: `orders`

```json
{
  "op": "subscribe",
  "args": [{
    "instType": "USDT-FUTURES",
    "channel": "orders",
    "instId": "default"
  }]
}
```

**Push Data**:
```json
{
  "action": "snapshot",
  "arg": {"instType": "USDT-FUTURES", "channel": "orders", "instId": "default"},
  "data": [{
    "orderId": "13333333",
    "clientOid": "12354678",
    "instId": "ETHUSDT",
    "price": "3000",
    "size": "0.4",
    "side": "buy",
    "posSide": "long",
    "tradeSide": "open",
    "orderType": "limit",
    "force": "gtc",
    "status": "live",
    "marginMode": "crossed",
    "marginCoin": "USDT",
    "leverage": "12",
    "posMode": "hedge_mode",
    "accBaseVolume": "0",
    "notionalUsd": "1200",
    "cTime": "1760461517274",
    "uTime": "1760461517274"
  }],
  "ts": 1760461517285
}
```

**Status Values**: `live`, `partially_filled`, `filled`, `canceled`

#### Equity Channel
**Channel**: `equity`

```json
{
  "op": "subscribe",
  "args": [{
    "instType": "USDT-FUTURES",
    "channel": "equity"
  }]
}
```

**Push Data**:
```json
{
  "action": "snapshot",
  "arg": {"instType": "USDT-FUTURES", "channel": "equity"},
  "data": [{
    "btcEquity": "0.0021",
    "usdtEquity": "13.985",
    "unrealizedPL": "0"
  }],
  "ts": 1695717225146
}
```

### 4. WebSocket Trading (Low Latency)

> **Note**: Contact Bitget BD/RM to apply for access permissions.

#### Place Order via WebSocket
**Channel**: `place-order`

```json
{
  "op": "trade",
  "args": [{
    "channel": "place-order",
    "id": "unique-request-id",
    "instId": "BTCUSDT",
    "instType": "USDT-FUTURES",
    "params": {
      "orderType": "limit",
      "side": "buy",
      "size": "2",
      "tradeSide": "open",
      "price": "501",
      "marginCoin": "USDT",
      "force": "gtc",
      "marginMode": "crossed",
      "clientOid": "custom-order-id"
    }
  }]
}
```

**Response**:
```json
{
  "event": "trade",
  "arg": [{
    "id": "unique-request-id",
    "instType": "USDT-FUTURES",
    "channel": "place-order",
    "instId": "BTCUSDT",
    "params": {
      "orderId": "xxxxx",
      "clientOid": "custom-order-id"
    }
  }],
  "code": 0,
  "msg": "Success"
}
```

#### Cancel Order via WebSocket
**Channel**: `cancel-order`

```json
{
  "op": "trade",
  "args": [{
    "channel": "cancel-order",
    "id": "unique-request-id",
    "instId": "BTCUSDT",
    "instType": "USDT-FUTURES",
    "params": {
      "orderId": "xxxxxxxxxx",
      "clientOid": "custom-order-id"
    }
  }]
}
```

---

## Spot Market API (for deposit/withdrawal status)

### Get Coin Info
**Rate Limit**: 3 req/sec/IP

```
GET /api/v2/spot/public/coins
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| coin | String | No | Coin name (omit for all) |

**Response Data**:
```json
{
  "coinId": "1",
  "coin": "BTC",
  "transfer": "true",
  "chains": [{
    "chain": "BTC",
    "needTag": "false",
    "withdrawable": "true",
    "rechargeable": "true",
    "withdrawFee": "0.005",
    "depositConfirm": "1",
    "withdrawConfirm": "1",
    "minDepositAmount": "0.001",
    "minWithdrawAmount": "0.001",
    "contractAddress": "",
    "congestion": "normal"
  }]
}
```

### Get Spot Ticker
**Rate Limit**: 20 req/sec/IP

```
GET /api/v2/spot/market/tickers
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | String | No | Trading pair (omit for all) |

---

## Rate Limits Summary

| Endpoint Category | Rate Limit |
|-------------------|------------|
| Market Data (Public) | 20 req/sec/IP |
| Account/Position | 10 req/sec/UID |
| Place/Cancel Order | 10 req/sec/UID |
| Batch Order | 5 req/sec/UID |
| Change Leverage | 5 req/sec/UID |
| VIP Fee Rate | 10 req/sec/IP |
| Coin Info | 3 req/sec/IP |

---

## Error Codes

| Code | Message |
|------|---------|
| 00000 | Success |
| 40001 | Invalid parameter |
| 40002 | Signature verification failed |
| 40007 | Insufficient balance |
| 40010 | Order does not exist |
| 40014 | Order has been filled or cancelled |
| 40015 | Order cancellation failed |
| 45001 | Service temporarily unavailable |

---

## Implementation Notes for Connector

### Required REST Endpoints
1. **Market Data**:
   - `GET /api/v2/mix/market/contracts` - All symbols & fee rates
   - `GET /api/v2/mix/market/tickers` - All prices & volumes
   - `GET /api/v2/mix/market/ticker` - Single ticker
   - `GET /api/v2/mix/market/merge-depth` - Orderbook
   - `GET /api/v2/mix/market/candles` - Candlesticks
   - `GET /api/v2/mix/market/history-candles` - Historical candles
   - `GET /api/v2/mix/market/fills` - Recent trades
   - `GET /api/v2/mix/market/current-fund-rate` - Funding rate
   - `GET /api/v2/mix/market/history-fund-rate` - Historical funding
   - `GET /api/v2/spot/public/coins` - Deposit/withdrawal status

2. **Account & Position**:
   - `GET /api/v2/mix/account/account` - Account info
   - `GET /api/v2/mix/account/accounts` - All accounts
   - `POST /api/v2/mix/account/set-leverage` - Set leverage
   - `GET /api/v2/mix/position/history-position` - Position history

3. **Trading**:
   - `POST /api/v2/mix/order/place-order` - Place order
   - `POST /api/v2/mix/order/batch-place-order` - Batch orders
   - `POST /api/v2/mix/order/cancel-order` - Cancel order
   - `GET /api/v2/mix/order/orders-pending` - Pending orders

### Required WebSocket Channels
1. **Public**:
   - `ticker` - Real-time price
   - `books/books5/books15` - Orderbook depth
   - `trade` - Public trades
   - `candle1m` (etc.) - Candlesticks

2. **Private**:
   - `account` - Account updates
   - `positions` - Position updates
   - `orders` - Order updates
   - `equity` - PnL updates

3. **Trading** (if enabled):
   - `place-order` - Low-latency order placement
   - `cancel-order` - Low-latency order cancellation
