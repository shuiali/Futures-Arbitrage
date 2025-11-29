# Gate.io API Documentation for Futures Arbitrage Connector

## Overview

Gate.io provides comprehensive REST and WebSocket APIs for both Spot and Futures (Perpetual) trading.

## Base URLs

### REST API
- **Live API**: `https://api.gateio.ws/api/v4`
- **Futures API**: `https://fx-api.gateio.ws/api/v4`

### WebSocket API
- **Spot WebSocket**: `wss://api.gateio.ws/ws/v4/`
- **Futures WebSocket**: `wss://fx-ws.gateio.ws/v4/ws/{settle}` (settle = btc, usdt)
- **Futures TestNet WebSocket**: `wss://fx-ws-testnet.gateio.ws/v4/ws/{settle}`

---

## Authentication

### REST API Authentication

All authenticated requests require the following headers:

```
KEY: <api_key>
Timestamp: <unix_timestamp_seconds>
SIGN: <signature>
```

**Signature Generation (HMAC-SHA512)**:
```
sign_string = request_method + "\n" + request_path + "\n" + query_string + "\n" + body_hash + "\n" + timestamp
body_hash = SHA512(request_body) or SHA512("") for GET requests
signature = HMAC-SHA512(sign_string, api_secret)
```

### WebSocket Authentication

For private WebSocket channels, use the login channel:

**Spot Login**:
```json
{
  "time": 1234567890,
  "channel": "spot.login",
  "event": "api",
  "payload": {
    "api_key": "your_api_key",
    "signature": "signature",
    "timestamp": "1234567890",
    "req_id": "unique_request_id"
  }
}
```

**Futures Login**:
```json
{
  "time": 1234567890,
  "channel": "futures.login",
  "event": "api",
  "payload": {
    "api_key": "your_api_key",
    "signature": "signature",
    "timestamp": "1234567890",
    "req_id": "unique_request_id"
  }
}
```

**WebSocket Signature**:
```
sign_string = "channel=" + channel + "&event=" + event + "&time=" + timestamp
signature = HMAC-SHA512(sign_string, api_secret)
```

---

## REST API Endpoints

### Futures Market Data (Public)

#### 1. List All Futures Contracts
```
GET /futures/{settle}/contracts
```

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| settle | path | Yes | Settlement currency (btc, usdt) |
| limit | query | No | Maximum records to return |
| offset | query | No | List offset |

**Response**:
```json
[
  {
    "name": "BTC_USDT",
    "type": "direct",
    "quanto_multiplier": "0.0001",
    "leverage_min": "1",
    "leverage_max": "100",
    "maintenance_rate": "0.005",
    "mark_type": "index",
    "mark_price": "37985.6",
    "index_price": "37954.92",
    "last_price": "38026",
    "funding_rate": "0.002053",
    "funding_rate_indicative": "0.000219",
    "funding_interval": 28800,
    "funding_next_apply": 1610035200,
    "order_size_min": 1,
    "order_size_max": 1000000,
    "order_price_round": "0.1",
    "maker_fee_rate": "-0.00025",
    "taker_fee_rate": "0.00075",
    "risk_limit_base": "1000000",
    "risk_limit_step": "1000000",
    "risk_limit_max": "8000000",
    "position_size": 5223816,
    "trade_size": 28530850594,
    "status": "trading"
  }
]
```

#### 2. Get Single Contract Details
```
GET /futures/{settle}/contracts/{contract}
```

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| settle | path | Yes | Settlement currency (btc, usdt) |
| contract | path | Yes | Contract name (e.g., BTC_USDT) |

#### 3. Get Futures Tickers
```
GET /futures/{settle}/tickers
```

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| settle | path | Yes | Settlement currency |
| contract | query | No | Filter by contract |

**Response**:
```json
[
  {
    "contract": "BTC_USDT",
    "last": "6432",
    "low_24h": "6278",
    "high_24h": "6790",
    "change_percentage": "4.43",
    "total_size": "32323904",
    "volume_24h": "184040233284",
    "volume_24h_base": "28613220",
    "volume_24h_quote": "184040233284",
    "volume_24h_settle": "28613220",
    "mark_price": "6534",
    "funding_rate": "0.0001",
    "funding_rate_indicative": "0.0001",
    "index_price": "6531",
    "highest_bid": "34089.7",
    "highest_size": "100",
    "lowest_ask": "34217.9",
    "lowest_size": "1000"
  }
]
```

#### 4. Get Order Book
```
GET /futures/{settle}/order_book
```

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| settle | path | Yes | Settlement currency |
| contract | query | Yes | Contract name |
| interval | query | No | Order book depth aggregation (0, 0.1, 0.01) |
| limit | query | No | Depth limit (default 10, max 100) |
| with_id | query | No | Include orderbook ID |

**Response**:
```json
{
  "id": 123456789,
  "current": 1623898993123,
  "update": 1623898993000,
  "asks": [
    { "p": "1.52", "s": 100 }
  ],
  "bids": [
    { "p": "1.50", "s": 200 }
  ]
}
```

#### 5. Get Market Trades
```
GET /futures/{settle}/trades
```

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| settle | path | Yes | Settlement currency |
| contract | query | Yes | Contract name |
| limit | query | No | Max records (default 100) |
| offset | query | No | List offset |
| from | query | No | Start time (Unix seconds) |
| to | query | No | End time (Unix seconds) |

#### 6. Get Candlesticks (K-Lines)
```
GET /futures/{settle}/candlesticks
```

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| settle | path | Yes | Settlement currency |
| contract | query | Yes | Contract name (prefix with `mark_` or `index_` for mark/index price) |
| interval | query | No | Interval (10s, 1m, 5m, 15m, 30m, 1h, 4h, 8h, 1d, 7d) |
| from | query | No | Start time |
| to | query | No | End time |
| limit | query | No | Max records (default 100, max 2000) |

**Response**:
```json
[
  {
    "t": 1539852480,
    "v": 97151,
    "c": "1.032",
    "h": "1.032",
    "l": "1.032",
    "o": "1.032",
    "sum": "3580"
  }
]
```

#### 7. Get Funding Rate History
```
GET /futures/{settle}/funding_rate
```

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| settle | path | Yes | Settlement currency |
| contract | query | Yes | Contract name |
| limit | query | No | Max records |
| from | query | No | Start timestamp |
| to | query | No | End timestamp |

**Response**:
```json
[
  {
    "t": 1543968000,
    "r": "0.000157"
  }
]
```

### Futures Account (Private)

#### 8. Get Futures Account
```
GET /futures/{settle}/accounts
```

**Response**:
```json
{
  "total": "9707.803567115145",
  "unrealised_pnl": "3371.248828",
  "position_margin": "38.712189181",
  "order_margin": "0",
  "available": "9669.091377934145",
  "point": "0",
  "currency": "USDT",
  "in_dual_mode": false,
  "position_mode": "single",
  "enable_credit": true,
  "history": {
    "dnw": "10000",
    "pnl": "68.3685",
    "fee": "-1.645812875",
    "refr": "0",
    "fund": "-358.919120009855"
  }
}
```

#### 9. Get Positions
```
GET /futures/{settle}/positions
```

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| settle | path | Yes | Settlement currency |
| holding | query | No | Return only real positions (true/false) |
| limit | query | No | Max records |
| offset | query | No | List offset |

**Response**:
```json
[
  {
    "user": 10000,
    "contract": "BTC_USDT",
    "size": -9440,
    "leverage": "0",
    "risk_limit": "100",
    "leverage_max": "100",
    "maintenance_rate": "0.005",
    "value": "3568.62",
    "margin": "4.431548146258",
    "entry_price": "3779.55",
    "liq_price": "99999999",
    "mark_price": "3780.32",
    "unrealised_pnl": "-0.000507486844",
    "realised_pnl": "0.045543982432",
    "mode": "single",
    "adl_ranking": 5
  }
]
```

### Futures Trading (Private)

#### 10. Place Futures Order
```
POST /futures/{settle}/orders
```

**Request Body**:
```json
{
  "contract": "BTC_USDT",
  "size": 100,
  "price": "45000",
  "tif": "gtc",
  "text": "t-my-custom-id",
  "reduce_only": false,
  "close": false,
  "iceberg": 0
}
```

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| contract | string | Yes | Contract name |
| size | integer | Yes | Order size (positive=buy, negative=sell, 0=close) |
| price | string | No | Order price (0 with tif=ioc for market order) |
| tif | string | No | Time in force (gtc, ioc, poc, fok) |
| text | string | No | Custom order ID (prefix with t-) |
| reduce_only | boolean | No | Reduce only order |
| close | boolean | No | Close position |
| iceberg | integer | No | Iceberg display size |
| auto_size | string | No | For dual mode: close_long, close_short |

**Response**:
```json
{
  "id": 15675394,
  "user": 100000,
  "contract": "BTC_USDT",
  "create_time": 1546569968,
  "size": 6024,
  "iceberg": 0,
  "left": 6024,
  "price": "3765",
  "fill_price": "0",
  "mkfr": "-0.00025",
  "tkfr": "0.00075",
  "tif": "gtc",
  "is_reduce_only": false,
  "is_close": false,
  "is_liq": false,
  "text": "t-my-custom-id",
  "status": "open",
  "finish_time": 0,
  "finish_as": ""
}
```

#### 11. Place Batch Orders
```
POST /futures/{settle}/batch_orders
```

**Request Body**: Array of order objects (same as single order)

#### 12. List Orders
```
GET /futures/{settle}/orders
```

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| settle | path | Yes | Settlement currency |
| contract | query | No | Filter by contract |
| status | query | Yes | Order status (open, finished) |
| limit | query | No | Max records |
| offset | query | No | List offset |
| last_id | query | No | Pagination cursor |

#### 13. Get Single Order
```
GET /futures/{settle}/orders/{order_id}
```

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| settle | path | Yes | Settlement currency |
| order_id | path | Yes | Order ID or custom text ID |

#### 14. Cancel Order
```
DELETE /futures/{settle}/orders/{order_id}
```

#### 15. Cancel All Orders
```
DELETE /futures/{settle}/orders
```

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| contract | query | Yes | Contract name |
| side | query | No | Side to cancel (ask, bid) |

#### 16. Amend Order
```
PUT /futures/{settle}/orders/{order_id}
```

**Request Body**:
```json
{
  "price": "45500",
  "size": 150,
  "amend_text": "reason for amendment"
}
```

### Wallet & Fees

#### 17. Get Trading Fees
```
GET /wallet/fee
```

**Response**:
```json
{
  "user_id": 10001,
  "taker_fee": "0.002",
  "maker_fee": "0.002",
  "gt_discount": false,
  "gt_taker_fee": "0",
  "gt_maker_fee": "0",
  "futures_taker_fee": "0.0005",
  "futures_maker_fee": "0.0002",
  "delivery_taker_fee": "0.0005",
  "delivery_maker_fee": "0.0002"
}
```

#### 18. Get Futures Fee Rate
```
GET /futures/{settle}/fee
```

**Response**:
```json
{
  "taker_fee": "0.00075",
  "maker_fee": "-0.00025"
}
```

---

## WebSocket API

### Connection

**Spot**: `wss://api.gateio.ws/ws/v4/`
**Futures**: `wss://fx-ws.gateio.ws/v4/ws/{settle}`

### Message Format

**Subscribe**:
```json
{
  "time": 1234567890,
  "channel": "channel_name",
  "event": "subscribe",
  "payload": ["param1", "param2"]
}
```

**Unsubscribe**:
```json
{
  "time": 1234567890,
  "channel": "channel_name",
  "event": "unsubscribe",
  "payload": ["param1", "param2"]
}
```

### Futures WebSocket Channels

#### 1. Tickers Channel
```json
{
  "time": 1234567890,
  "channel": "futures.tickers",
  "event": "subscribe",
  "payload": ["BTC_USDT"]
}
```

**Update Message**:
```json
{
  "time": 1234567890,
  "channel": "futures.tickers",
  "event": "update",
  "result": {
    "contract": "BTC_USDT",
    "last": "45000.5",
    "change_percentage": "2.5",
    "funding_rate": "0.0001",
    "funding_rate_indicative": "0.0001",
    "mark_price": "45001.2",
    "index_price": "44999.8",
    "total_size": "123456789",
    "volume_24h": "1234567890",
    "volume_24h_base": "27434",
    "volume_24h_quote": "1234567890",
    "volume_24h_settle": "27434",
    "high_24h": "46000",
    "low_24h": "44000",
    "highest_bid": "45000",
    "lowest_ask": "45001"
  }
}
```

#### 2. Trades Channel
```json
{
  "time": 1234567890,
  "channel": "futures.trades",
  "event": "subscribe",
  "payload": ["BTC_USDT"]
}
```

**Update Message**:
```json
{
  "time": 1234567890,
  "channel": "futures.trades",
  "event": "update",
  "result": [
    {
      "id": 123456,
      "create_time": 1234567890,
      "create_time_ms": 1234567890123,
      "contract": "BTC_USDT",
      "size": 100,
      "price": "45000.5",
      "is_internal": false
    }
  ]
}
```

#### 3. Order Book Channel (Full Snapshot)
```json
{
  "time": 1234567890,
  "channel": "futures.order_book",
  "event": "subscribe",
  "payload": ["BTC_USDT", "100ms", "20"]
}
```

**Parameters**:
- Contract name
- Update interval: "100ms" or "1000ms"
- Depth levels: "5", "10", "20", "50", "100"

**Update Message**:
```json
{
  "time": 1234567890,
  "channel": "futures.order_book",
  "event": "all",
  "result": {
    "t": 1234567890123,
    "id": 123456789,
    "contract": "BTC_USDT",
    "asks": [
      { "p": "45001", "s": 100 }
    ],
    "bids": [
      { "p": "45000", "s": 200 }
    ]
  }
}
```

#### 4. Order Book Update Channel (Incremental)
```json
{
  "time": 1234567890,
  "channel": "futures.order_book_update",
  "event": "subscribe",
  "payload": ["BTC_USDT", "100ms"]
}
```

**Update Intervals**: "20ms", "100ms"

**Update Message**:
```json
{
  "time": 1234567890,
  "channel": "futures.order_book_update",
  "event": "update",
  "result": {
    "t": 1234567890123,
    "s": "BTC_USDT",
    "U": 123456787,
    "u": 123456789,
    "a": [
      { "p": "45001", "s": 100 }
    ],
    "b": [
      { "p": "45000", "s": 0 }
    ]
  }
}
```

#### 5. Best Bid/Offer (BBO) Channel
```json
{
  "time": 1234567890,
  "channel": "futures.book_ticker",
  "event": "subscribe",
  "payload": ["BTC_USDT"]
}
```

**Update Message (10ms interval)**:
```json
{
  "time": 1234567890,
  "channel": "futures.book_ticker",
  "event": "update",
  "result": {
    "t": 1234567890123,
    "u": 123456789,
    "s": "BTC_USDT",
    "b": "45000",
    "B": 100,
    "a": "45001",
    "A": 200
  }
}
```

#### 6. Candlesticks Channel
```json
{
  "time": 1234567890,
  "channel": "futures.candlesticks",
  "event": "subscribe",
  "payload": ["1m", "BTC_USDT"]
}
```

**Intervals**: "10s", "1m", "5m", "15m", "30m", "1h", "4h", "8h", "1d", "7d"

### Private Futures WebSocket Channels

#### 7. Orders Channel (Requires Login)
```json
{
  "time": 1234567890,
  "channel": "futures.orders",
  "event": "subscribe",
  "payload": ["!all"]
}
```

**Update Message**:
```json
{
  "time": 1234567890,
  "channel": "futures.orders",
  "event": "update",
  "result": [
    {
      "contract": "BTC_USDT",
      "create_time": 1234567890,
      "create_time_ms": 1234567890123,
      "fill_price": "45000",
      "finish_as": "filled",
      "finish_time": 1234567891,
      "finish_time_ms": 1234567891123,
      "iceberg": 0,
      "id": 123456789,
      "is_close": false,
      "is_liq": false,
      "is_reduce_only": false,
      "left": 0,
      "mkfr": "-0.00025",
      "price": "45000",
      "size": 100,
      "status": "finished",
      "text": "t-my-order",
      "tif": "gtc",
      "tkfr": "0.00075",
      "user": "10001"
    }
  ]
}
```

#### 8. User Trades Channel
```json
{
  "time": 1234567890,
  "channel": "futures.usertrades",
  "event": "subscribe",
  "payload": ["!all"]
}
```

**Update Message**:
```json
{
  "time": 1234567890,
  "channel": "futures.usertrades",
  "event": "update",
  "result": [
    {
      "id": "123456",
      "create_time": 1234567890,
      "create_time_ms": 1234567890123,
      "contract": "BTC_USDT",
      "order_id": "987654321",
      "size": 100,
      "price": "45000",
      "role": "taker",
      "fee": "0.00337"
    }
  ]
}
```

#### 9. Positions Channel
```json
{
  "time": 1234567890,
  "channel": "futures.positions",
  "event": "subscribe",
  "payload": ["!all"]
}
```

**Update Message**:
```json
{
  "time": 1234567890,
  "channel": "futures.positions",
  "event": "update",
  "result": [
    {
      "contract": "BTC_USDT",
      "cross_leverage_limit": "25",
      "entry_price": "45000",
      "history_pnl": "100.5",
      "history_point": "0",
      "last_close_pnl": "50.25",
      "leverage": "10",
      "leverage_max": "100",
      "liq_price": "40000",
      "maintenance_rate": "0.005",
      "margin": "450",
      "mode": "single",
      "realised_pnl": "12.5",
      "realised_point": "0",
      "risk_limit": "1000000",
      "size": 100,
      "time": 1234567890,
      "time_ms": 1234567890123,
      "unrealised_pnl": "25.5",
      "user": "10001",
      "value": "4500"
    }
  ]
}
```

#### 10. Balances Channel
```json
{
  "time": 1234567890,
  "channel": "futures.balances",
  "event": "subscribe",
  "payload": ["!all"]
}
```

**Update Message**:
```json
{
  "time": 1234567890,
  "channel": "futures.balances",
  "event": "update",
  "result": [
    {
      "balance": "10000",
      "change": "-450",
      "text": "position margin",
      "time": 1234567890,
      "time_ms": 1234567890123,
      "type": "position_margin",
      "user": "10001"
    }
  ]
}
```

### WebSocket Trading API

#### 11. WebSocket Order Placement
```json
{
  "time": 1234567890,
  "channel": "futures.order_place",
  "event": "api",
  "payload": {
    "req_id": "unique_request_id",
    "req_param": {
      "contract": "BTC_USDT",
      "size": 100,
      "price": "45000",
      "tif": "gtc",
      "text": "t-ws-order"
    }
  }
}
```

#### 12. WebSocket Batch Order Placement
```json
{
  "time": 1234567890,
  "channel": "futures.order_batch_place",
  "event": "api",
  "payload": {
    "req_id": "unique_request_id",
    "req_param": [
      {
        "contract": "BTC_USDT",
        "size": 100,
        "price": "45000"
      },
      {
        "contract": "ETH_USDT",
        "size": 10,
        "price": "3000"
      }
    ]
  }
}
```

#### 13. WebSocket Order Cancel
```json
{
  "time": 1234567890,
  "channel": "futures.order_cancel",
  "event": "api",
  "payload": {
    "req_id": "unique_request_id",
    "req_param": {
      "order_id": "123456789"
    }
  }
}
```

#### 14. WebSocket Cancel by IDs
```json
{
  "time": 1234567890,
  "channel": "futures.order_cancel_ids",
  "event": "api",
  "payload": {
    "req_id": "unique_request_id",
    "req_param": ["123456789", "987654321"]
  }
}
```

#### 15. WebSocket Cancel by Contract/Side
```json
{
  "time": 1234567890,
  "channel": "futures.order_cancel_cp",
  "event": "api",
  "payload": {
    "req_id": "unique_request_id",
    "req_param": {
      "contract": "BTC_USDT",
      "side": "ask"
    }
  }
}
```

#### 16. WebSocket Order Amend
```json
{
  "time": 1234567890,
  "channel": "futures.order_amend",
  "event": "api",
  "payload": {
    "req_id": "unique_request_id",
    "req_param": {
      "order_id": "123456789",
      "price": "45500"
    }
  }
}
```

#### 17. WebSocket Get Order Status
```json
{
  "time": 1234567890,
  "channel": "futures.order_status",
  "event": "api",
  "payload": {
    "req_id": "unique_request_id",
    "req_param": {
      "order_id": "123456789"
    }
  }
}
```

#### 18. WebSocket List Orders
```json
{
  "time": 1234567890,
  "channel": "futures.order_list",
  "event": "api",
  "payload": {
    "req_id": "unique_request_id",
    "req_param": {
      "contract": "BTC_USDT",
      "status": "open"
    }
  }
}
```

---

## Spot WebSocket Channels (Reference)

### Public Channels
- `spot.tickers` - Ticker updates
- `spot.trades` / `spot.trades_v2` - Trade updates
- `spot.candlesticks` - Candlestick updates
- `spot.book_ticker` - Best bid/offer (10ms)
- `spot.order_book` - Full orderbook snapshot
- `spot.order_book_update` - Incremental orderbook updates
- `spot.obu` - Order book update V2

### Private Channels
- `spot.orders` / `spot.orders_v2` - Order updates
- `spot.usertrades` / `spot.usertrades_v2` - User trade updates
- `spot.balances` - Spot balance updates
- `spot.cross_balances` - Cross margin balance updates
- `spot.margin_balances` - Margin balance updates

### Spot WebSocket Trading API
- `spot.login` - Authentication
- `spot.order_place` - Place order
- `spot.order_cancel` - Cancel order
- `spot.order_cancel_ids` - Cancel multiple orders
- `spot.order_cancel_cp` - Cancel by currency pair
- `spot.order_amend` - Amend order
- `spot.order_status` - Get order status
- `spot.order_list` - List orders

---

## Rate Limits

### REST API
- Public endpoints: 900 requests/second per IP
- Private endpoints: 900 requests/second per user
- Order endpoints: Additional fill ratio limits

### WebSocket
- Rate limit headers in responses:
  - `x_gate_ratelimit_requests_remain` - Remaining requests
  - `x_gate_ratelimit_limit` - Request limit
  - `x_gate_ratelimit_reset_timestamp` - Reset timestamp

---

## Error Codes

| Label | Description |
|-------|-------------|
| USER_NOT_FOUND | User has no futures account |
| CONTRACT_NOT_FOUND | Contract not found |
| RISK_LIMIT_EXCEEDED | Risk limit exceeded |
| INSUFFICIENT_BALANCE | Insufficient balance |
| ORDER_NOT_FOUND | Order not found |
| ORDER_FINISHED | Order already finished |
| INVALID_PRICE | Invalid price |
| INVALID_SIZE | Invalid size |
| FOK_NOT_FILL | FOK order cannot be fully filled |
| INITIAL_MARGIN_TOO_LOW | Insufficient initial margin |
| ORDER_BOOK_NOT_FOUND | Insufficient liquidity |
| CANCEL_FAIL | Order cancel failed |

---

## Order Status

| Status | Description |
|--------|-------------|
| open | Order is active |
| finished | Order is completed |

## Order Finish Reasons

| Reason | Description |
|--------|-------------|
| filled | Fully filled |
| cancelled | Manually cancelled |
| liquidated | Cancelled due to liquidation |
| ioc | IOC order finished immediately |
| auto_deleveraged | Finished by ADL |
| reduce_only | Cancelled due to reduce-only |
| position_closed | Cancelled because position was closed |
| stp | Cancelled due to self-trade prevention |

---

## Notes for Implementation

1. **Settlement Currency**: Gate.io uses `{settle}` path parameter for futures (btc, usdt)
2. **Contract Naming**: Format is `{BASE}_{QUOTE}` (e.g., BTC_USDT)
3. **Order Size**: Specified in contracts, not base currency
4. **Quanto Multiplier**: Each contract represents `quanto_multiplier` of base currency
5. **Price Precision**: Use `order_price_round` from contract details
6. **Dual Mode**: Support for hedge mode with separate long/short positions
7. **WebSocket Ping**: Send ping frames to keep connection alive
8. **Order Book Management**: Use incremental updates with sequence numbers (U, u) for consistency
