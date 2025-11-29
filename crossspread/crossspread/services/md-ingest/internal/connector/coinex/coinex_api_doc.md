# CoinEx Futures API v2 Documentation

## Overview

CoinEx provides a comprehensive REST and WebSocket API for futures trading.

### Base URLs

| Type | URL |
|------|-----|
| REST API | `https://api.coinex.com/v2` |
| Futures WebSocket | `wss://socket.coinex.com/v2/futures` |
| Spot WebSocket | `wss://socket.coinex.com/v2/spot` |

### API Version
- Current version: v2
- Legacy perpetual API: v1 (deprecated)

---

## Authentication

### HTTP Authentication

All authenticated endpoints require the following headers:

| Header | Description |
|--------|-------------|
| `X-COINEX-KEY` | Your API access_id |
| `X-COINEX-SIGN` | HMAC-SHA256 signature (lowercase hex, 64 chars) |
| `X-COINEX-TIMESTAMP` | Request timestamp in milliseconds |
| `X-COINEX-WINDOWTIME` | (Optional) Validity window, default 5000ms |

### Signature Generation (HTTP)

```
prepared_str = METHOD + request_path + body(optional) + timestamp

Example for GET:
prepared_str = "GET" + "/v2/spot/pending-order?market=BTCUSDT&page=1" + "1700490703564"

Example for POST:
prepared_str = "POST" + "/v2/futures/order" + '{"market":"BTCUSDT",...}' + "1700490703564"

signed_str = HMAC-SHA256(secret_key, prepared_str).hexdigest().lower()
```

### WebSocket Authentication

Call `server.sign` method after connection:

```json
{
    "method": "server.sign",
    "params": {
        "access_id": "YOUR_ACCESS_ID",
        "signed_str": "HMAC_SHA256_OF_TIMESTAMP",
        "timestamp": 1234567890123
    },
    "id": 1
}
```

For WebSocket: `signed_str = HMAC-SHA256(secret_key, str(timestamp)).hexdigest().lower()`

---

## Rate Limits

### IP Rate Limit
- 400 requests/second

### User Rate Limits (Futures)

| Group | Rate | Endpoints |
|-------|------|-----------|
| Place & edit orders | 20r/s | POST /futures/order, /futures/stop-order, /futures/close-position, /futures/batch-order |
| Cancel orders | 40r/s | POST /futures/cancel-order, /futures/cancel-batch-order |
| Batch cancel | 20r/s | POST /futures/cancel-all-order, /futures/cancel-order-by-client-id |
| Query orders | 50r/s | GET /futures/pending-order, /futures/order-status |
| Query history | 10r/s | GET /futures/finished-order, /futures/user-deals |
| Query account | 10r/s | GET /assets/futures/balance, /futures/pending-position |

---

## REST API Endpoints

### Market Data (Public)

#### Get Market Status
```
GET /futures/market
```

Parameters:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | false | string | Market names, comma-separated (max 10) |

Response fields:
- `market`: Market name (e.g., "BTCUSDT")
- `contract_type`: "linear"
- `maker_fee_rate`: Maker fee rate
- `taker_fee_rate`: Taker fee rate
- `min_amount`: Minimum transaction volume
- `base_ccy`: Base currency
- `quote_ccy`: Quote currency
- `base_ccy_precision`: Base currency decimal places
- `quote_ccy_precision`: Quote currency decimal places
- `tick_size`: Minimum price increment
- `leverage`: Array of available leverage values
- `open_interest_volume`: Current open interest
- `is_market_available`: Whether market is open
- `is_api_trading_available`: Whether API trading is enabled

#### Get Market Ticker (24h)
```
GET /futures/ticker
```

Parameters:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | false | string | Market names, comma-separated (max 10) |

Response fields:
- `market`: Market name
- `last`: Latest price
- `open`: Opening price
- `close`: Closing price
- `high`: Highest price (24h)
- `low`: Lowest price (24h)
- `volume`: 24h filled volume
- `value`: 24h filled value
- `volume_sell`: Taker sell volume
- `volume_buy`: Taker buy volume
- `index_price`: Index price
- `mark_price`: Mark price
- `open_interest_volume`: Position size

#### Get Market Depth
```
GET /futures/depth
```

Parameters:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | true | string | Market name |
| limit | true | int | One of [5, 10, 20, 50] |
| interval | true | string | Merge interval (e.g., "0", "0.01", "1") |

Response fields:
- `market`: Market name
- `is_full`: Full or incremental
- `depth.asks`: [[price, size], ...]
- `depth.bids`: [[price, size], ...]
- `depth.last`: Latest price
- `depth.updated_at`: Timestamp (ms)
- `depth.checksum`: CRC32 checksum

Checksum calculation:
```
checksum_string = bid1_price:bid1_amount:bid2_price:bid2_amount:ask1_price:ask1_amount:...
checksum = crc32(checksum_string)
```

#### Get Market Trades
```
GET /futures/deals
```

Parameters:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | true | string | Market name |
| limit | false | int | Default 100, max 1000 |
| last_id | false | int | Starting TxID (0 for latest) |

Response fields:
- `deal_id`: Transaction ID
- `created_at`: Timestamp (ms)
- `side`: "buy" or "sell"
- `price`: Filled price
- `amount`: Executed amount

#### Get Market Klines
```
GET /futures/kline
```

Parameters:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | true | string | Market name |
| period | true | string | One of: 1min, 3min, 5min, 15min, 30min, 1hour, 2hour, 4hour, 6hour, 12hour, 1day, 3day, 1week |
| price_type | false | string | Default "latest_price" |
| limit | false | int | Default 100, max 1000 |

Response fields:
- `market`: Market name
- `created_at`: Timestamp (ms)
- `open`: Opening price
- `close`: Closing price
- `high`: Highest price
- `low`: Lowest price
- `volume`: Filled volume
- `value`: Filled value

#### Get Funding Rate
```
GET /futures/funding-rate
```

Parameters:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | false | string | Market names, comma-separated |

Response fields:
- `market`: Market name
- `mark_price`: Mark price
- `latest_funding_rate`: Current funding rate
- `next_funding_rate`: Next funding rate
- `max_funding_rate`: Maximum funding rate
- `min_funding_rate`: Minimum funding rate
- `latest_funding_time`: Last funding collection time (ms)
- `next_funding_time`: Next funding collection time (ms)

#### Get Funding Rate History
```
GET /futures/funding-rate-history
```

Parameters:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | true | string | Market name |
| start_time | false | int | Query start time |
| end_time | false | int | Query end time |
| page | false | int | Default 1 |
| limit | false | int | Default 10 |

#### Get Market Index
```
GET /futures/index
```

Parameters:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | false | string | Market names, comma-separated |

Response fields:
- `market`: Market name
- `created_at`: Timestamp (ms)
- `price`: Index price
- `sources`: Array of exchange sources with weights

---

### Account & Balance (Private)

#### Get Futures Balance
```
GET /assets/futures/balance
```

Response fields:
- `ccy`: Currency name
- `available`: Available balance
- `frozen`: Frozen balance
- `margin`: Position margin
- `transferrable`: Balance available for transfer
- `unrealized_pnl`: Unrealized PnL

#### Get Spot Balance
```
GET /assets/spot/balance
```

Response fields:
- `ccy`: Currency name
- `available`: Available balance
- `frozen`: Frozen balance

#### Get Deposit History
```
GET /assets/deposit-history
```

Parameters:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| ccy | false | string | Currency name |
| tx_id | false | string | Transaction ID |
| status | false | string | Deposit status |
| page | false | int | Default 1 |
| limit | false | int | Default 10 |

#### Get Withdrawal History
```
GET /assets/withdraw
```

Parameters:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| ccy | false | string | Currency name |
| withdraw_id | false | int | Withdrawal ID |
| status | false | string | Withdrawal status |
| page | false | int | Default 1 |
| limit | false | int | Default 10 |

---

### Trading (Private)

#### Place Order
```
POST /futures/order
```

Request body:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | true | string | Market name |
| market_type | true | string | "FUTURES" |
| side | true | string | "buy" or "sell" |
| type | true | string | "limit" or "market" |
| amount | true | string | Order amount |
| price | false | string | Required for limit orders |
| client_id | false | string | User-defined ID (1-32 chars) |
| is_hide | false | bool | Hide order in public depth |
| stp_mode | false | string | Self-trade prevention: "ct", "cm", "both" |

Response fields:
- `order_id`: Order ID
- `market`: Market name
- `market_type`: "FUTURES"
- `side`: "buy" or "sell"
- `type`: "limit" or "market"
- `amount`: Order amount
- `price`: Order price
- `unfilled_amount`: Remaining unfilled
- `filled_amount`: Filled amount
- `filled_value`: Filled value
- `client_id`: Client ID
- `fee`: Trading fee
- `fee_ccy`: Fee currency
- `maker_fee_rate`: Maker fee rate
- `taker_fee_rate`: Taker fee rate
- `realized_pnl`: Realized PnL
- `created_at`: Order creation time (ms)
- `updated_at`: Order update time (ms)

#### Cancel Order
```
POST /futures/cancel-order
```

Request body:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | true | string | Market name |
| market_type | true | string | "FUTURES" |
| order_id | true | int | Order ID |

#### Cancel Order by Client ID
```
POST /futures/cancel-order-by-client-id
```

Request body:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | false | string | Market name |
| market_type | true | string | "FUTURES" |
| client_id | true | string | User-defined ID |

#### Cancel All Orders
```
POST /futures/cancel-all-order
```

Request body:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | true | string | Market name |
| market_type | true | string | "FUTURES" |
| side | false | string | "buy" or "sell" |

#### Close Position
```
POST /futures/close-position
```

Request body:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | true | string | Market name |
| market_type | true | string | "FUTURES" |
| type | true | string | "limit" or "market" |
| price | false | string | Required for limit close |
| amount | false | string | Amount to close (null = all) |
| client_id | false | string | User-defined ID |

#### Get Pending Orders
```
GET /futures/pending-order
```

Parameters:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | false | string | Market name |
| market_type | true | string | "FUTURES" |
| side | false | string | "buy" or "sell" |
| client_id | false | string | User-defined ID |
| page | false | int | Default 1 |
| limit | false | int | Default 10 |

#### Get Filled Orders
```
GET /futures/finished-order
```

Parameters:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | false | string | Market name |
| market_type | true | string | "FUTURES" |
| side | false | string | "buy" or "sell" |
| page | false | int | Default 1 |
| limit | false | int | Default 10 |

---

### Position Management (Private)

#### Get Current Positions
```
GET /futures/pending-position
```

Parameters:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | false | string | Market name |
| market_type | true | string | "FUTURES" |
| page | false | int | Default 1 |
| limit | false | int | Default 10 |

Response fields:
- `position_id`: Position ID
- `market`: Market name
- `market_type`: "FUTURES"
- `side`: "long" or "short"
- `margin_mode`: "cross" or "isolated"
- `open_interest`: Position size
- `close_avbl`: Amount available to close
- `unrealized_pnl`: Unrealized PnL
- `realized_pnl`: Realized PnL
- `avg_entry_price`: Average entry price
- `leverage`: Leverage
- `margin_avbl`: Available margin
- `liq_price`: Liquidation price
- `bkr_price`: Bankruptcy price
- `adl_level`: ADL risk level (1-5)
- `settle_price`: Settlement (mark) price
- `created_at`: Position creation time (ms)
- `updated_at`: Position update time (ms)

#### Adjust Position Leverage
```
POST /futures/adjust-position-leverage
```

Request body:
| Name | Required | Type | Description |
|------|----------|------|-------------|
| market | true | string | Market name |
| market_type | true | string | "FUTURES" |
| margin_mode | true | string | "cross" or "isolated" |
| leverage | true | int | Leverage value |

---

## WebSocket API

### Connection

Connect to: `wss://socket.coinex.com/v2/futures`

All responses are compressed with gzip/deflate.

### Subscription Format

Request:
```json
{
    "method": "CHANNEL.subscribe",
    "params": {...},
    "id": 1
}
```

Response:
```json
{
    "id": 1,
    "code": 0,
    "message": "OK"
}
```

Push:
```json
{
    "method": "CHANNEL.update",
    "data": {...},
    "id": null
}
```

### Market Data Subscriptions

#### Depth Subscription
```json
{
    "method": "depth.subscribe",
    "params": {
        "market_list": [
            ["BTCUSDT", 10, "0", true],
            ["ETHUSDT", 10, "0", false]
        ]
    },
    "id": 1
}
```

Parameters: [market, limit, interval, is_full]
- limit: One of [5, 10, 20, 50]
- interval: Merge interval (e.g., "0", "0.01")
- is_full: true = full push, false = incremental

Push: `depth.update`
```json
{
    "method": "depth.update",
    "data": {
        "market": "BTCUSDT",
        "is_full": true,
        "depth": {
            "asks": [["30740.00", "0.31763545"]],
            "bids": [["30736.00", "0.04857373"]],
            "last": "30746.28",
            "updated_at": 1689152421692,
            "checksum": 2578768879
        }
    }
}
```

Unsubscribe: `depth.unsubscribe`

#### Trades Subscription
```json
{
    "method": "deals.subscribe",
    "params": {
        "market_list": ["BTCUSDT", "ETHUSDT"]
    },
    "id": 1
}
```

Push: `deals.update`
```json
{
    "method": "deals.update",
    "data": {
        "market": "BTCUSDT",
        "deal_list": [
            {
                "deal_id": 3514376759,
                "created_at": 1689152421692,
                "side": "buy",
                "price": "30718.42",
                "amount": "0.00000325"
            }
        ]
    }
}
```

#### BBO (Best Bid/Offer) Subscription
```json
{
    "method": "bbo.subscribe",
    "params": {
        "market_list": ["BTCUSDT", "ETHUSDT"]
    },
    "id": 1
}
```

Push: `bbo.update`
```json
{
    "method": "bbo.update",
    "data": {
        "market": "BTCUSDT",
        "updated_at": 1642145331234,
        "best_bid_price": "20000",
        "best_bid_size": "0.1",
        "best_ask_price": "20001",
        "best_ask_size": "0.15"
    }
}
```

#### Market State (Ticker) Subscription
```json
{
    "method": "state.subscribe",
    "params": {
        "market_list": ["BTCUSDT"]
    },
    "id": 1
}
```

Push: `state.update`
```json
{
    "method": "state.update",
    "data": {
        "state_list": [
            {
                "market": "BTCUSDT",
                "last": "30000",
                "open": "29500",
                "close": "30000",
                "high": "30500",
                "low": "29000",
                "volume": "1000",
                "value": "30000000",
                "volume_sell": "400",
                "volume_buy": "600",
                "open_interest_size": "5000",
                "mark_price": "30001",
                "index_price": "30002",
                "latest_funding_rate": "0.0001",
                "next_funding_rate": "0.0001",
                "latest_funding_time": 1642145331234,
                "next_funding_time": 1642231731234
            }
        ]
    }
}
```

#### Index Price Subscription
```json
{
    "method": "index.subscribe",
    "params": {
        "market_list": ["BTCUSDT"]
    },
    "id": 1
}
```

Push: `index.update`
```json
{
    "method": "index.update",
    "data": {
        "market": "BTCUSDT",
        "index_price": "20000",
        "mark_price": "20000"
    }
}
```

### User Data Subscriptions (Authenticated)

Call `server.sign` first to authenticate.

#### Order Subscription
```json
{
    "method": "order.subscribe",
    "params": {
        "market_list": ["BTCUSDT"]
    },
    "id": 1
}
```

Push: `order.update`
```json
{
    "method": "order.update",
    "data": {
        "event": "put",
        "order": {
            "order_id": 98388656341,
            "stop_id": 0,
            "market": "BTCUSDT",
            "side": "buy",
            "type": "limit",
            "amount": "0.0010",
            "price": "50000.00",
            "unfilled_amount": "0.0010",
            "filled_amount": "0",
            "filled_value": "0",
            "fee": "0",
            "fee_ccy": "USDT",
            "taker_fee_rate": "0.00046",
            "maker_fee_rate": "0",
            "client_id": "",
            "created_at": 1689145715129,
            "updated_at": 1689145715129
        }
    }
}
```

Event types: "put", "update", "finish"

#### Position Subscription
```json
{
    "method": "position.subscribe",
    "params": {
        "market_list": ["BTCUSDT"]
    },
    "id": 1
}
```

Push: `position.update`
```json
{
    "method": "position.update",
    "data": {
        "event": "",
        "position": {
            "position_id": 246830219,
            "market": "BTCUSDT",
            "side": "long",
            "margin_mode": "cross",
            "open_interest": "0.0010",
            "close_avbl": "0.0010",
            "unrealized_pnl": "0.00",
            "realized_pnl": "-0.01413182",
            "avg_entry_price": "30721.35",
            "leverage": "50",
            "margin_avbl": "0.61442700",
            "liq_price": "31179.87",
            "bkr_price": "31335.77",
            "adl_level": 5,
            "settle_price": "30721.35",
            "created_at": 1642145331234,
            "updated_at": 1642145331234
        }
    }
}
```

---

## Error Codes

### HTTP Error Codes

| Code | Description |
|------|-------------|
| 3008 | Service busy, retry later |
| 3109 | Insufficient balance |
| 3127 | Order quantity below minimum |
| 3606 | Price difference too large |
| 3620 | Market order unavailable (insufficient depth) |
| 3621 | Order cannot be completely filled (cancelled) |
| 3622 | Post-only order cannot be maker (cancelled) |
| 4001 | Service unavailable |
| 4002 | Request timeout |
| 4004 | Parameter error |
| 4005 | Invalid access_id |
| 4006 | Signature verification failed |
| 4007 | IP prohibited |
| 4010 | Expired request |
| 4115 | User trading prohibited |
| 4117 | Market trading prohibited |
| 4130 | Futures trading prohibited |
| 4213 | Rate limit triggered |

### WebSocket Error Codes

| Code | Description |
|------|-------------|
| 20001 | Parameter error |
| 20002 | Method not found |
| 21001 | Authentication required |
| 21002 | Authentication failed |
| 23001 | Request timeout |
| 23002 | Too frequent requests |
| 24001 | Internal error |
| 24002 | Service temporarily unavailable |

---

## Response Format

### HTTP Success Response
```json
{
    "code": 0,
    "data": {...},
    "message": "OK"
}
```

### HTTP Paginated Response
```json
{
    "code": 0,
    "data": [...],
    "pagination": {
        "total": 100,
        "has_next": true
    },
    "message": "OK"
}
```

### WebSocket Response
```json
{
    "id": 1,
    "code": 0,
    "message": "OK"
}
```
