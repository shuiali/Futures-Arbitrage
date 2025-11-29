# HTX (Huobi) USDT-Margined Futures API Documentation

## Overview
HTX (formerly Huobi) provides USDT-Margined perpetual swap contracts. This documentation covers the linear swap API endpoints required for the CrossSpread futures arbitrage platform.

## Base URLs

### REST API
- **Primary**: `https://api.hbdm.com`
- **AWS Backup**: `https://api.btcgateway.pro`
- **Vietnam Backup**: `https://api.hbdm.vn` (recommended for low latency)

### WebSocket
- **Market Data**: `wss://api.hbdm.com/linear-swap-ws`
- **Order Push**: `wss://api.hbdm.com/linear-swap-notification`
- **Index Data**: `wss://api.hbdm.com/ws_index`
- **System Status**: `wss://api.hbdm.com/center-notification`

### Backup WebSocket URLs
- **Market Data**: `wss://api.btcgateway.pro/linear-swap-ws`
- **Order Push**: `wss://api.btcgateway.pro/linear-swap-notification`
- **Index Data**: `wss://api.btcgateway.pro/ws_index`

## Authentication

### REST API Authentication
- **Signature Method**: HMAC-SHA256
- **Signature Version**: 2
- **Required Parameters**:
  - `AccessKeyId`: Your API Key
  - `SignatureMethod`: HmacSHA256
  - `SignatureVersion`: 2
  - `Timestamp`: ISO 8601 format (UTC), e.g., `2017-05-11T15:19:30`

### Signature Generation
1. Create signature string: `{HTTP_METHOD}\n{HOST}\n{PATH}\n{QUERY_STRING}`
2. Sign with HMAC-SHA256 using secret key
3. Base64 encode the signature
4. URL encode the base64 result

### WebSocket Authentication
- **Signature Version**: 2.1
- Send authentication message after connection:
```json
{
  "op": "auth",
  "type": "api",
  "AccessKeyId": "your_access_key",
  "SignatureMethod": "HmacSHA256",
  "SignatureVersion": "2.1",
  "Timestamp": "2021-01-01T00:00:00",
  "Signature": "base64_encoded_signature"
}
```

## Rate Limits

### REST API
- **Public endpoints (market data)**: 800 requests/second per IP
- **Private endpoints (trading)**: 
  - 72 requests/3 seconds per UID for read operations
  - 36 requests/3 seconds per UID for trade operations

### WebSocket
- **Subscription (sub)**: No limit
- **Request (req)**: 50 times/second max
- **Private order push**: Max 30 connections per UID

## Contract Code Format
- **Perpetual Swap**: `BTC-USDT`
- **Current Week**: `BTC-USDT-CW`
- **Next Week**: `BTC-USDT-NW`
- **Current Quarter**: `BTC-USDT-CQ`
- **Next Quarter**: `BTC-USDT-NQ`
- **Specific Date**: `BTC-USDT-210625`

## Data Compression
- **Market WebSocket**: GZIP compressed (must decompress)
- **Order WebSocket**: GZIP compressed (must decompress)

## Heartbeat
- Server sends `ping` with timestamp
- Client must respond with `pong` containing same timestamp
- Timeout: 5 seconds (connection closed if no pong)

---

## REST API Endpoints

### Reference Data

#### Get Contract Information
```
GET /linear-swap-api/v1/swap_contract_info
```
**Parameters**:
- `contract_code` (optional): e.g., `BTC-USDT`
- `support_margin_mode` (optional): `all`, `cross`, `isolated`

**Response**:
```json
{
  "status": "ok",
  "data": [{
    "symbol": "BTC",
    "contract_code": "BTC-USDT",
    "contract_size": 0.001,
    "price_tick": 0.1,
    "settlement_date": "",
    "delivery_time": "",
    "create_date": "20200325",
    "contract_status": 1,
    "support_margin_mode": "all"
  }]
}
```

#### Get Price Limit
```
GET /linear-swap-api/v1/swap_price_limit
```
**Parameters**:
- `contract_code`: e.g., `BTC-USDT`

**Response**:
```json
{
  "status": "ok",
  "data": [{
    "symbol": "BTC",
    "contract_code": "BTC-USDT",
    "high_limit": 13000.00,
    "low_limit": 10000.00
  }]
}
```

#### Get Open Interest
```
GET /linear-swap-api/v1/swap_open_interest
```
**Parameters**:
- `contract_code` (optional): e.g., `BTC-USDT`

**Response**:
```json
{
  "status": "ok",
  "data": [{
    "symbol": "BTC",
    "contract_code": "BTC-USDT",
    "amount": 123456.789,
    "volume": 1234567890,
    "value": 12345678.90,
    "trade_amount": 12345.678,
    "trade_volume": 123456789,
    "trade_turnover": 1234567890.12
  }]
}
```

### Market Data

#### Get Market Depth
```
GET /linear-swap-ex/market/depth
```
**Parameters**:
- `contract_code`: e.g., `BTC-USDT`
- `type`: `step0` (no aggregation), `step1`-`step5` (price aggregation levels)

**Response**:
```json
{
  "ch": "market.BTC-USDT.depth.step0",
  "status": "ok",
  "tick": {
    "asks": [[10000.0, 1.0], [10001.0, 2.0]],
    "bids": [[9999.0, 1.0], [9998.0, 2.0]],
    "ch": "market.BTC-USDT.depth.step0",
    "id": 1603714757,
    "mrid": 12345678901,
    "ts": 1603714757852,
    "version": 1234567890
  },
  "ts": 1603714757875
}
```

#### Get BBO (Best Bid/Offer)
```
GET /linear-swap-ex/market/bbo
```
**Parameters**:
- `contract_code` (optional): e.g., `BTC-USDT`

**Response**:
```json
{
  "status": "ok",
  "ticks": [{
    "contract_code": "BTC-USDT",
    "mrid": 12345678901,
    "ask": [10000.0, 1.0],
    "bid": [9999.0, 1.0],
    "ts": 1603714757852
  }]
}
```

#### Get KLine Data
```
GET /linear-swap-ex/market/history/kline
```
**Parameters**:
- `contract_code`: e.g., `BTC-USDT`
- `period`: `1min`, `5min`, `15min`, `30min`, `60min`, `4hour`, `1day`, `1week`, `1mon`
- `size` (optional): 1-2000, default 150
- `from` (optional): Start timestamp in seconds
- `to` (optional): End timestamp in seconds

**Response**:
```json
{
  "ch": "market.BTC-USDT.kline.1min",
  "status": "ok",
  "data": [{
    "id": 1603714800,
    "open": 10000.0,
    "close": 10001.0,
    "low": 9999.0,
    "high": 10002.0,
    "amount": 123.456,
    "vol": 123456,
    "trade_turnover": 1234567.89,
    "count": 1234
  }]
}
```

#### Get Market Overview (Ticker)
```
GET /linear-swap-ex/market/detail/merged
```
**Parameters**:
- `contract_code`: e.g., `BTC-USDT`

**Response**:
```json
{
  "ch": "market.BTC-USDT.detail.merged",
  "status": "ok",
  "tick": {
    "id": 1603714757,
    "ts": 1603714757852,
    "ask": [10000.0, 1.0],
    "bid": [9999.0, 1.0],
    "open": 9900.0,
    "close": 10000.0,
    "low": 9800.0,
    "high": 10100.0,
    "amount": 12345.678,
    "vol": 123456789,
    "trade_turnover": 12345678901.23,
    "count": 123456
  }
}
```

#### Get Batch Market Overview
```
GET /linear-swap-ex/market/detail/batch_merged
```
**Parameters**:
- `contract_code` (optional): comma-separated, e.g., `BTC-USDT,ETH-USDT`

**Response**:
```json
{
  "status": "ok",
  "ticks": [{
    "contract_code": "BTC-USDT",
    "id": 1603714757,
    "ts": 1603714757852,
    "ask": [10000.0, 1.0],
    "bid": [9999.0, 1.0],
    "open": 9900.0,
    "close": 10000.0,
    "low": 9800.0,
    "high": 10100.0,
    "amount": 12345.678,
    "vol": 123456789,
    "trade_turnover": 12345678901.23,
    "count": 123456
  }]
}
```

#### Get Recent Trades
```
GET /linear-swap-ex/market/trade
```
**Parameters**:
- `contract_code`: e.g., `BTC-USDT`

**Response**:
```json
{
  "ch": "market.BTC-USDT.trade.detail",
  "status": "ok",
  "tick": {
    "id": 1603714757852,
    "ts": 1603714757852,
    "data": [{
      "id": 12345678901,
      "price": 10000.0,
      "amount": 1.0,
      "direction": "buy",
      "ts": 1603714757852,
      "quantity": 1.0,
      "trade_turnover": 10.0
    }]
  }
}
```

#### Get History Trades
```
GET /linear-swap-ex/market/history/trade
```
**Parameters**:
- `contract_code`: e.g., `BTC-USDT`
- `size`: 1-2000, default 1

### Funding Rate

#### Get Current Funding Rate
```
GET /linear-swap-api/v1/swap_funding_rate
```
**Parameters**:
- `contract_code`: e.g., `BTC-USDT`

**Response**:
```json
{
  "status": "ok",
  "data": {
    "symbol": "BTC",
    "contract_code": "BTC-USDT",
    "fee_asset": "USDT",
    "funding_time": "1603728000000",
    "funding_rate": "0.000100000000",
    "estimated_rate": "0.000150000000",
    "next_funding_time": "1603756800000"
  }
}
```

#### Get Batch Funding Rate
```
GET /linear-swap-api/v1/swap_batch_funding_rate
```
**Parameters**:
- `contract_code` (optional): e.g., `BTC-USDT`

#### Get Historical Funding Rate
```
GET /linear-swap-api/v1/swap_historical_funding_rate
```
**Parameters**:
- `contract_code`: e.g., `BTC-USDT`
- `page_index` (optional): default 1
- `page_size` (optional): 1-50, default 20

### Index Price

#### Get Index Price
```
GET /linear-swap-api/v1/swap_index
```
**Parameters**:
- `contract_code` (optional): e.g., `BTC-USDT`

**Response**:
```json
{
  "status": "ok",
  "data": [{
    "contract_code": "BTC-USDT",
    "index_price": 10000.0,
    "index_ts": 1603714757852
  }]
}
```

### Trading Fee

#### Get Trading Fee
```
GET /linear-swap-api/v1/swap_fee
```
**Parameters**:
- `contract_code`: e.g., `BTC-USDT`

**Response**:
```json
{
  "status": "ok",
  "data": [{
    "symbol": "BTC",
    "contract_code": "BTC-USDT",
    "open_maker_fee": "0.0002",
    "open_taker_fee": "0.0005",
    "close_maker_fee": "0.0002",
    "close_taker_fee": "0.0005",
    "fee_asset": "USDT"
  }]
}
```

---

## Account Endpoints (Cross Margin Mode)

#### Get Account Information
```
POST /linear-swap-api/v1/swap_cross_account_info
```
**Parameters**:
- `margin_account` (optional): e.g., `USDT`

**Response**:
```json
{
  "status": "ok",
  "data": [{
    "margin_mode": "cross",
    "margin_account": "USDT",
    "margin_asset": "USDT",
    "margin_balance": 1000.0,
    "margin_static": 1000.0,
    "margin_position": 100.0,
    "margin_frozen": 50.0,
    "profit_real": 10.0,
    "profit_unreal": 5.0,
    "withdraw_available": 845.0,
    "risk_rate": 10.0,
    "contract_detail": [{
      "symbol": "BTC",
      "contract_code": "BTC-USDT",
      "margin_position": 50.0,
      "margin_frozen": 25.0,
      "margin_available": 25.0,
      "profit_unreal": 2.5,
      "liquidation_price": 5000.0,
      "lever_rate": 10,
      "adjust_factor": 0.075
    }]
  }]
}
```

#### Get Position Information
```
POST /linear-swap-api/v1/swap_cross_position_info
```
**Parameters**:
- `contract_code` (optional): e.g., `BTC-USDT`

**Response**:
```json
{
  "status": "ok",
  "data": [{
    "symbol": "BTC",
    "contract_code": "BTC-USDT",
    "volume": 10.0,
    "available": 10.0,
    "frozen": 0.0,
    "cost_open": 10000.0,
    "cost_hold": 10000.0,
    "profit_unreal": 100.0,
    "profit_rate": 0.01,
    "profit": 100.0,
    "margin_asset": "USDT",
    "position_margin": 100.0,
    "lever_rate": 10,
    "direction": "buy",
    "last_price": 10100.0,
    "margin_mode": "cross",
    "margin_account": "USDT"
  }]
}
```

---

## Trading Endpoints (Cross Margin Mode)

#### Place Order
```
POST /linear-swap-api/v1/swap_cross_order
```
**Parameters**:
- `contract_code`: e.g., `BTC-USDT`
- `volume`: Order quantity (contracts)
- `direction`: `buy` or `sell`
- `offset`: `open` or `close`
- `lever_rate`: 1-125
- `order_price_type`: `limit`, `opponent` (BBO), `optimal_5`, `optimal_10`, `optimal_20`, `post_only`, `fok`, `ioc`, `opponent_ioc`, `optimal_5_ioc`, etc.
- `price` (optional): Required for limit orders
- `client_order_id` (optional): 1-9223372036854775807

**Response**:
```json
{
  "status": "ok",
  "data": {
    "order_id": 12345678901,
    "order_id_str": "12345678901",
    "client_order_id": 12345678901
  }
}
```

#### Place Batch Orders
```
POST /linear-swap-api/v1/swap_cross_batchorder
```
**Parameters**:
- `orders_data`: Array of order objects (max 10)

#### Cancel Order
```
POST /linear-swap-api/v1/swap_cross_cancel
```
**Parameters**:
- `order_id` (optional): Order ID or comma-separated IDs
- `client_order_id` (optional): Client order ID or comma-separated IDs
- `contract_code`: e.g., `BTC-USDT`

**Response**:
```json
{
  "status": "ok",
  "data": {
    "errors": [],
    "successes": "12345678901,12345678902"
  }
}
```

#### Cancel All Orders
```
POST /linear-swap-api/v1/swap_cross_cancelall
```
**Parameters**:
- `contract_code`: e.g., `BTC-USDT`
- `direction` (optional): `buy` or `sell`
- `offset` (optional): `open` or `close`

#### Get Order Info
```
POST /linear-swap-api/v1/swap_cross_order_info
```
**Parameters**:
- `order_id` (optional): Order ID or comma-separated IDs
- `client_order_id` (optional): Client order ID or comma-separated IDs
- `contract_code`: e.g., `BTC-USDT`

**Response**:
```json
{
  "status": "ok",
  "data": [{
    "symbol": "BTC",
    "contract_code": "BTC-USDT",
    "volume": 1,
    "price": 10000.0,
    "order_price_type": "limit",
    "direction": "buy",
    "offset": "open",
    "lever_rate": 10,
    "order_id": 12345678901,
    "order_id_str": "12345678901",
    "client_order_id": null,
    "created_at": 1603714757852,
    "trade_volume": 1,
    "trade_turnover": 10.0,
    "fee": -0.005,
    "trade_avg_price": 10000.0,
    "margin_frozen": 0,
    "profit": 0,
    "status": 6,
    "order_type": 1,
    "order_source": "api",
    "fee_asset": "USDT",
    "margin_mode": "cross",
    "margin_account": "USDT"
  }]
}
```

#### Get Open Orders
```
POST /linear-swap-api/v1/swap_cross_openorders
```
**Parameters**:
- `contract_code`: e.g., `BTC-USDT`
- `page_index` (optional): default 1
- `page_size` (optional): 1-50, default 20
- `sort_by` (optional): `created_at` or `update_time`
- `trade_type` (optional): 0 (all), 1 (buy long), 2 (sell short), 3 (buy short), 4 (sell long)

---

## WebSocket Subscriptions

### Market Data WebSocket (wss://api.hbdm.com/linear-swap-ws)

#### Subscribe to KLine
```json
{
  "sub": "market.BTC-USDT.kline.1min",
  "id": "id1"
}
```

**Push Data**:
```json
{
  "ch": "market.BTC-USDT.kline.1min",
  "ts": 1603714757852,
  "tick": {
    "id": 1603714800,
    "mrid": 12345678901,
    "open": 10000.0,
    "close": 10001.0,
    "low": 9999.0,
    "high": 10002.0,
    "amount": 123.456,
    "vol": 123456,
    "trade_turnover": 1234567.89,
    "count": 1234
  }
}
```

#### Subscribe to Market Depth
```json
{
  "sub": "market.BTC-USDT.depth.step0",
  "id": "id2"
}
```

#### Subscribe to Incremental Depth
```json
{
  "sub": "market.BTC-USDT.depth.size_20.high_freq",
  "data_type": "incremental",
  "id": "id3"
}
```

**Push Data**:
```json
{
  "ch": "market.BTC-USDT.depth.size_20.high_freq",
  "tick": {
    "asks": [[10000.0, 1.0]],
    "bids": [[9999.0, 1.0]],
    "ch": "market.BTC-USDT.depth.size_20.high_freq",
    "event": "update",
    "id": 12345678901,
    "mrid": 12345678901,
    "ts": 1603714757852,
    "version": 1234567890
  },
  "ts": 1603714757875
}
```

#### Subscribe to BBO
```json
{
  "sub": "market.BTC-USDT.bbo",
  "id": "id4"
}
```

**Push Data**:
```json
{
  "ch": "market.BTC-USDT.bbo",
  "ts": 1603714757852,
  "tick": {
    "mrid": 12345678901,
    "id": 12345678901,
    "bid": [9999.0, 1.0],
    "ask": [10000.0, 1.0],
    "ts": 1603714757852,
    "version": 1234567890
  }
}
```

#### Subscribe to Trades
```json
{
  "sub": "market.BTC-USDT.trade.detail",
  "id": "id5"
}
```

**Push Data**:
```json
{
  "ch": "market.BTC-USDT.trade.detail",
  "ts": 1603714757852,
  "tick": {
    "id": 12345678901,
    "ts": 1603714757852,
    "data": [{
      "id": 12345678901,
      "amount": 1.0,
      "price": 10000.0,
      "direction": "buy",
      "ts": 1603714757852,
      "quantity": 1.0,
      "trade_turnover": 10.0
    }]
  }
}
```

### Order Push WebSocket (wss://api.hbdm.com/linear-swap-notification)

#### Authentication
```json
{
  "op": "auth",
  "type": "api",
  "AccessKeyId": "your_access_key",
  "SignatureMethod": "HmacSHA256",
  "SignatureVersion": "2.1",
  "Timestamp": "2021-01-01T00:00:00",
  "Signature": "base64_encoded_signature"
}
```

#### Subscribe to Orders (Cross)
```json
{
  "op": "sub",
  "topic": "orders_cross.BTC-USDT"
}
```

**Push Data**:
```json
{
  "op": "notify",
  "topic": "orders_cross.BTC-USDT",
  "ts": 1603714757852,
  "uid": "123456789",
  "symbol": "BTC",
  "contract_code": "BTC-USDT",
  "volume": 1,
  "price": 10000.0,
  "order_price_type": "limit",
  "direction": "buy",
  "offset": "open",
  "status": 6,
  "lever_rate": 10,
  "order_id": 12345678901,
  "order_id_str": "12345678901",
  "client_order_id": null,
  "order_source": "api",
  "order_type": 1,
  "created_at": 1603714757852,
  "trade_volume": 1,
  "trade_turnover": 10.0,
  "fee": -0.005,
  "trade_avg_price": 10000.0,
  "margin_frozen": 0,
  "profit": 0,
  "margin_mode": "cross",
  "margin_account": "USDT",
  "trade": [{
    "trade_id": 12345678901,
    "id": "12345678901-12345678901-1",
    "trade_volume": 1,
    "trade_price": 10000.0,
    "trade_turnover": 10.0,
    "trade_fee": -0.005,
    "role": "taker",
    "created_at": 1603714757852,
    "fee_asset": "USDT"
  }]
}
```

#### Subscribe to Match Orders (Cross)
```json
{
  "op": "sub",
  "topic": "matchOrders_cross.BTC-USDT"
}
```

#### Subscribe to Accounts (Cross)
```json
{
  "op": "sub",
  "topic": "accounts_cross.USDT"
}
```

**Push Data**:
```json
{
  "op": "notify",
  "topic": "accounts_cross.USDT",
  "ts": 1603714757852,
  "uid": "123456789",
  "event": "order.match",
  "data": [{
    "margin_mode": "cross",
    "margin_account": "USDT",
    "margin_asset": "USDT",
    "margin_balance": 1000.0,
    "margin_static": 1000.0,
    "margin_position": 100.0,
    "margin_frozen": 50.0,
    "profit_real": 10.0,
    "profit_unreal": 5.0,
    "withdraw_available": 845.0,
    "risk_rate": 10.0
  }]
}
```

#### Subscribe to Positions (Cross)
```json
{
  "op": "sub",
  "topic": "positions_cross.BTC-USDT"
}
```

**Push Data**:
```json
{
  "op": "notify",
  "topic": "positions_cross.BTC-USDT",
  "ts": 1603714757852,
  "uid": "123456789",
  "event": "order.match",
  "data": [{
    "symbol": "BTC",
    "contract_code": "BTC-USDT",
    "volume": 10.0,
    "available": 10.0,
    "frozen": 0.0,
    "cost_open": 10000.0,
    "cost_hold": 10000.0,
    "profit_unreal": 100.0,
    "profit_rate": 0.01,
    "profit": 100.0,
    "margin_asset": "USDT",
    "position_margin": 100.0,
    "lever_rate": 10,
    "direction": "buy",
    "last_price": 10100.0,
    "margin_mode": "cross",
    "margin_account": "USDT"
  }]
}
```

### Funding Rate WebSocket
```json
{
  "op": "sub",
  "topic": "public.BTC-USDT.funding_rate"
}
```

---

## Order Status Codes
| Status | Description |
|--------|-------------|
| 1 | Ready to submit |
| 2 | Ready to submit |
| 3 | Submitted |
| 4 | Partial filled |
| 5 | Partial filled then cancelled |
| 6 | Fully filled |
| 7 | Cancelled |
| 11 | Cancelling |

## Order Price Types
| Type | Description |
|------|-------------|
| `limit` | Limit order |
| `opponent` | BBO price |
| `optimal_5` | Optimal 5 levels |
| `optimal_10` | Optimal 10 levels |
| `optimal_20` | Optimal 20 levels |
| `post_only` | Maker only |
| `fok` | Fill or Kill |
| `ioc` | Immediate or Cancel |
| `opponent_ioc` | BBO IOC |
| `optimal_5_ioc` | Optimal 5 IOC |
| `optimal_10_ioc` | Optimal 10 IOC |
| `optimal_20_ioc` | Optimal 20 IOC |
| `opponent_fok` | BBO FOK |
| `optimal_5_fok` | Optimal 5 FOK |
| `optimal_10_fok` | Optimal 10 FOK |
| `optimal_20_fok` | Optimal 20 FOK |

## Error Codes
| Code | Description |
|------|-------------|
| 1000 | System error |
| 1001 | System busy |
| 1002 | Query error |
| 1003 | Timeout |
| 1004 | Volume must be greater than 0 |
| 1012 | Signature error |
| 1013 | API key permission error |
| 1014 | Timestamp error |
| 1017 | IP not in whitelist |
| 1030 | Contract does not exist |
| 1031 | Contract not available |
| 1032 | Trading suspended |
| 1033 | Orders not available |
| 1034 | Position not available |
| 1040 | Order price exceeds limit |
| 1041 | Order price exceeds precision |
| 1042 | Order volume exceeds limit |
| 1047 | Insufficient margin |
| 1048 | Insufficient position |
| 1050 | Client order id duplicate |
| 1051 | Order not exist |
| 1056 | Settlement in progress |
| 1057 | Contract in settlement |
| 1080 | Position query error during settlement |
