# LBank API Documentation

## Overview

LBank provides separate APIs for Spot trading and Contract (Perpetual Futures) trading.
This connector primarily targets the Contract API for perpetual futures arbitrage.

## Base URLs

### Spot API
- **REST Base URL**: `https://api.lbkex.com/` (primary), `https://www.lbkex.net/`
- **WebSocket URL**: `wss://www.lbkex.net/ws/V2/`

### Contract (Perpetual Futures) API
- **REST Base URL**: `https://lbkperp.lbank.com/`
- **WebSocket URL**: `wss://lbkperpws.lbank.com/ws`
- **Public API Path**: `/cfd/openApi/v1/pub/`
- **Private API Path**: `/cfd/openApi/v1/prv/`

---

## Authentication

### Signature Methods
LBank supports two signature methods:
1. **RSA** - More secure, asymmetric encryption
2. **HmacSHA256** - Faster, symmetric encryption

### Request Headers (Contract API)
```
Content-Type: application/json
timestamp: <milliseconds timestamp>
signature_method: RSA | HmacSHA256
echostr: <30-40 character alphanumeric string>
```

### Signature Process (Contract API)
1. **Prepare string to be signed**: Sort all parameters (including `api_key`, `signature_method`, `timestamp`, `echostr`) by parameter name alphabetically, excluding `sign`
2. **MD5 Digest**: Convert parameters string to MD5 digest, uppercase
3. **Sign**: Sign the MD5 digest using RSA or HmacSHA256 with your secret key

Example parameter string:
```
api_key=xxx&asset=USDT&echostr=xxx&productGroup=SwapU&signature_method=HmacSHA256&timestamp=xxx
```

### Signature Process (Spot API)
1. **Prepare string to be signed**: Sort parameters alphabetically (excluding `sign`)
2. **Append secret key**: Concatenate with `&secret_key=your_secret_key`
3. **MD5 Hash**: Generate MD5 hash and convert to uppercase

---

## Contract API Endpoints

### Public Endpoints

#### Get Server Time
```
GET /cfd/openApi/v1/pub/getTime
```
Response:
```json
{
    "data": { "serverTime": 1665990154559 },
    "error_code": 0,
    "msg": "",
    "result": "true",
    "success": true
}
```

#### Get Contract Instruments
```
GET /cfd/openApi/v1/pub/instrument?productGroup=SwapU
```
Parameters:
- `productGroup`: Product group (e.g., "SwapU" for USDT perpetuals)

Response:
```json
[
    {
        "baseCurrency": "BTC",
        "clearCurrency": "USDT",
        "defaultLeverage": 20,
        "exchangeID": "LBANK",
        "maxOrderVolume": "100000",
        "minOrderCost": "5",
        "minOrderVolume": "0.001",
        "priceCurrency": "USDT",
        "priceLimitLowerValue": 0.5,
        "priceLimitUpperValue": 2.0,
        "priceTick": 0.1,
        "symbol": "BTCUSDT",
        "symbolName": "BTC/USDT",
        "volumeMultiple": 1,
        "volumeTick": 0.001
    }
]
```

#### Get Market Data (Tickers)
```
GET /cfd/openApi/v1/pub/marketData?productGroup=SwapU
```
Response:
```json
[
    {
        "highestPrice": "70000",
        "lastPrice": "69500",
        "lowestPrice": "68000",
        "markedPrice": "69450",
        "openPrice": "69000",
        "prePositionFeeRate": "0.0001",
        "symbol": "BTCUSDT",
        "turnover": "50000000",
        "volume": "720"
    }
]
```

#### Get Orderbook (Handicap)
```
GET /cfd/openApi/v1/pub/marketOrder?symbol=BTCUSDT&depth=50
```
Parameters:
- `symbol`: Trading pair symbol
- `depth`: Orderbook depth

Response:
```json
{
    "asks": [
        { "orders": 5, "price": 69500, "volume": 1.5 },
        { "orders": 3, "price": 69510, "volume": 2.0 }
    ],
    "bids": [
        { "orders": 4, "price": 69490, "volume": 1.2 },
        { "orders": 2, "price": 69480, "volume": 0.8 }
    ],
    "symbol": "BTCUSDT"
}
```

### Private Endpoints (Require Authentication)

#### Get Account Info
```
POST /cfd/openApi/v1/prv/account
```
Body:
```json
{
    "api_key": "xxx",
    "productGroup": "SwapU",
    "asset": "USDT",
    "sign": "xxx"
}
```

#### Place Order
```
POST /cfd/openApi/v1/prv/order/create
```
Body:
```json
{
    "api_key": "xxx",
    "symbol": "BTCUSDT",
    "side": "BUY",
    "type": "LIMIT",
    "price": "69000",
    "volume": "0.01",
    "sign": "xxx"
}
```

#### Cancel Order
```
POST /cfd/openApi/v1/prv/order/cancel
```
Body:
```json
{
    "api_key": "xxx",
    "symbol": "BTCUSDT",
    "orderId": "xxx",
    "sign": "xxx"
}
```

#### Get Positions
```
POST /cfd/openApi/v1/prv/position
```
Body:
```json
{
    "api_key": "xxx",
    "productGroup": "SwapU",
    "sign": "xxx"
}
```

---

## Spot API Endpoints

### Public Market Data

#### Get Ticker
```
GET /v2/ticker/24hr.do?symbol=btc_usdt
```
Response:
```json
{
    "result": true,
    "data": [{
        "symbol": "btc_usdt",
        "ticker": {
            "high": "70000",
            "low": "68000",
            "vol": "1500",
            "change": "2.5",
            "turnover": "100000000",
            "latest": "69500"
        },
        "timestamp": 1665990154559
    }]
}
```

#### Get Orderbook Depth
```
GET /v2/depth.do?symbol=btc_usdt&size=100
```
Parameters:
- `symbol`: Trading pair (lowercase with underscore)
- `size`: 1-200 levels

Response:
```json
{
    "result": true,
    "data": {
        "asks": [[69510, 1.5], [69520, 2.0]],
        "bids": [[69500, 1.2], [69490, 0.8]],
        "timestamp": 1665990154559
    }
}
```

#### Get Recent Trades
```
GET /v2/supplement/trades.do?symbol=btc_usdt&size=100
```

#### Get Kline/Candlestick
```
GET /v2/kline.do?symbol=btc_usdt&size=100&type=min1&time=1665990154
```
Types: minute1, minute5, minute15, minute30, hour1, hour4, hour8, hour12, day1, week1, month1

### Wallet Endpoints

#### Get Asset Configs (Deposit/Withdrawal Status)
```
GET /v2/assetConfigs.do
```
Response:
```json
[
    {
        "assetCode": "btc",
        "chain": "BTC",
        "canWithDraw": true,
        "canDeposit": true,
        "minWithDraw": "0.001",
        "fee": "0.0005"
    }
]
```

#### Get Trading Fee
```
POST /v2/supplement/customer_trade_fee.do
```
Response:
```json
{
    "result": true,
    "data": [{
        "symbol": "btc_usdt",
        "makerCommission": "0.001",
        "takerCommission": "0.001"
    }]
}
```

### Trading Endpoints (Spot)

#### Place Order
```
POST /v2/supplement/create_order.do
```
Parameters:
- `api_key`: API key
- `symbol`: Trading pair
- `type`: buy/sell
- `price`: Order price
- `amount`: Order amount
- `sign`: Signature

#### Cancel Order
```
POST /v2/supplement/cancel_order.do
```

---

## Spot WebSocket API

### Connection
URL: `wss://www.lbkex.net/ws/V2/`

### Heartbeat
```json
// Ping
{"action":"ping", "ping":"<uuid>"}
// Pong (response)
{"action":"pong", "pong":"<uuid>"}
```

### Subscribe to Orderbook Depth
```json
{
    "action": "subscribe",
    "subscribe": "depth",
    "depth": "100",
    "pair": "btc_usdt"
}
```
Response:
```json
{
    "depth": {
        "asks": [[69510, 1.5], [69520, 2.0]],
        "bids": [[69500, 1.2], [69490, 0.8]]
    },
    "count": 100,
    "type": "depth",
    "pair": "btc_usdt",
    "SERVER": "V2",
    "TS": "2019-06-28T17:49:22.722"
}
```

### Subscribe to Trades
```json
{
    "action": "subscribe",
    "subscribe": "trade",
    "pair": "btc_usdt"
}
```

### Subscribe to Ticker
```json
{
    "action": "subscribe",
    "subscribe": "tick",
    "pair": "btc_usdt"
}
```

### Subscribe to Kline
```json
{
    "action": "subscribe",
    "subscribe": "kbar",
    "kbar": "5min",
    "pair": "btc_usdt"
}
```

### Unsubscribe
```json
{
    "action": "unsubscribe",
    "subscribe": "depth",
    "depth": "100",
    "pair": "btc_usdt"
}
```

### Request Data (One-time)
```json
{
    "action": "request",
    "request": "depth",
    "depth": "100",
    "pair": "btc_usdt"
}
```

---

## Private WebSocket API (Spot)

### Get Subscribe Key
```
POST /v2/subscribe/get_key.do
```
Returns a subscribe key valid for 60 minutes.

### Refresh Subscribe Key
```
POST /v2/subscribe/refresh_key.do
```

### Subscribe to Order Updates
Connect to: `wss://www.lbkex.net/ws/V2/?subscribeKey=<key>`
```json
{
    "action": "subscribe",
    "subscribe": "orderUpdate",
    "pair": "btc_usdt"
}
```

### Subscribe to Asset Updates
```json
{
    "action": "subscribe",
    "subscribe": "assetUpdate"
}
```

---

## Contract WebSocket API

### Connection
URL: `wss://lbkperpws.lbank.com/ws`

### Authentication
After connecting, send authentication message with signed request.

### Subscribe to Market Data
```json
{
    "action": "subscribe",
    "channel": "depth",
    "symbol": "BTCUSDT"
}
```

---

## Error Codes (Contract API)

| Code | Description |
|------|-------------|
| 0 | Success |
| -99 | System exception |
| 8 | Contract product does not exist |
| 24 | Order does not exist |
| 31 | Insufficient positions |
| 35 | Insufficient balance |
| 37 | Invalid quantity |
| 48 | Illegal price |
| 49 | Price exceeds upper limit |
| 50 | Price exceeds lower limit |
| 100 | Insufficient margin |
| 176 | Invalid API KEY |
| 177 | API key has expired |
| 178 | API key limit exceeded |
| 183 | Exceeded maximum query count per second |
| 184 | Order limit exceeded |
| 193 | Exceeded maximum trading volume |
| 194 | Less than minimum trading volume |
| 10003 | Authentication and signature verification failed |
| 10004 | Request timed out |
| 10010 | Invalid signature |
| 10012 | Request is too frequent |

---

## Rate Limits

### Spot API
- General requests: 200 requests per 10 seconds
- Order placement/cancellation: 500 requests per 10 seconds

### Contract API
- Public endpoints: Higher limits
- Private endpoints: Rate limited per API key

---

## Symbol Format

### Spot
- Format: `base_quote` (lowercase with underscore)
- Examples: `btc_usdt`, `eth_btc`

### Contract (Perpetual)
- Format: `BASQUOTE` (uppercase, no separator)
- Examples: `BTCUSDT`, `ETHUSDT`

---

## Notes

1. API keys not bound to IP are only valid for 30 days
2. Contract API uses JSON body for POST requests
3. Spot API uses form-urlencoded for POST requests
4. WebSocket connections require periodic ping/pong to stay alive
5. Contract funding rate is provided in `prePositionFeeRate` field
6. Product group "SwapU" refers to USDT-margined perpetuals
