# BingX Perpetual Futures API Documentation

## Overview

- **Base URL**: `https://open-api.bingx.com`
- **WebSocket Market Data URL**: `wss://open-api-swap.bingx.com/swap-market`
- **WebSocket User Data URL**: `wss://open-api-swap.bingx.com/swap-market?listenKey={listenKey}`

## Authentication

### Creating API Key
Create an API key through BingX website: User Center → API Management (Perpetual Contract)

### Request Headers
```
X-BX-APIKEY: {your_api_key}
```

### Signature Generation
1. Sort all parameters alphabetically
2. Concatenate parameters: `param1=value1&param2=value2&timestamp={timestamp}`
3. Generate HMAC-SHA256 signature using your secret key
4. Append signature to request: `&signature={signature}`

### Example (Python)
```python
import hmac
import hashlib
import time

def sign(secret, params_str):
    return hmac.new(secret.encode(), params_str.encode(), hashlib.sha256).hexdigest()

timestamp = str(int(time.time() * 1000))
params = f"symbol=BTC-USDT&timestamp={timestamp}"
signature = sign(api_secret, params)
```

---

## REST API Endpoints

### Market Data (Public)

#### Get All Contracts
```
GET /openApi/swap/v2/quote/contracts
```

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| symbol | string | Trading pair, e.g., BTC-USDT |
| pricePrecision | int | Price precision |
| quantityPrecision | int | Quantity precision |
| tradeMinLimit | int | Minimum trading unit (contracts) |
| currency | string | Settlement and margin currency |
| asset | string | Contract trading asset |
| status | int | 0=offline, 1=online |
| maxLongLeverage | int | Maximum leverage for long positions |
| maxShortLeverage | int | Maximum leverage for short positions |
| feeRate | string | Trading fee rate |

---

#### Get Latest Price
```
GET /openApi/swap/v2/quote/price
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | No | Trading pair, e.g., BTC-USDT. If not sent, returns all pairs |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| symbol | string | Trading pair |
| price | string | Price |
| time | int64 | Matching engine time |

---

#### Get Order Book Depth
```
GET /openApi/swap/v2/quote/depth
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair, e.g., BTC-USDT |
| limit | int | No | Default: 20, Max: 1000 |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| bids | array | [price, quantity] |
| asks | array | [price, quantity] |
| T | int64 | Timestamp (milliseconds) |

---

#### Get Recent Trades
```
GET /openApi/swap/v2/quote/trades
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair, e.g., BTC-USDT |
| limit | int | No | Default: 500, Max: 1000 |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| time | int64 | Transaction time |
| isBuyerMaker | bool | Whether buyer is maker |
| price | string | Transaction price |
| qty | string | Transaction quantity |
| quoteQty | string | Turnover |

---

#### Get K-Line Data
```
GET /openApi/swap/v2/quote/klines
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair, e.g., BTC-USDT |
| interval | string | Yes | 1m, 3m, 5m, 15m, 30m, 1h, 2h, 4h, 6h, 8h, 12h, 1d, 3d, 1w, 1M |
| startTime | int64 | No | Start time (milliseconds) |
| endTime | int64 | No | End time (milliseconds) |
| limit | int | No | Default: 500, Max: 1400 |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| open | string | Open price |
| close | string | Close price |
| high | string | High price |
| low | string | Low price |
| volume | string | Volume |
| time | int64 | Time |

---

#### Get Mark Price and Funding Rate
```
GET /openApi/swap/v2/quote/premiumIndex
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | No | Trading pair, e.g., BTC-USDT |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| symbol | string | Trading pair |
| lastFundingRate | string | Last funding rate |
| markPrice | string | Current mark price |
| indexPrice | string | Index price |
| nextFundingTime | int64 | Next funding time (milliseconds) |

---

#### Get Funding Rate History
```
GET /openApi/swap/v2/quote/fundingRate
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair |
| startTime | int64 | No | Start time (milliseconds) |
| endTime | int64 | No | End time (milliseconds) |
| limit | int32 | No | Default: 100, Max: 1000 |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| symbol | string | Trading pair |
| fundingRate | string | Funding rate |
| fundingTime | int64 | Funding time (milliseconds) |

---

#### Get Open Interest
```
GET /openApi/swap/v2/quote/openInterest
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair, e.g., BTC-USDT |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| openInterest | string | Position amount |
| symbol | string | Contract name |
| time | int64 | Matching engine time |

---

#### Get 24hr Price Ticker
```
GET /openApi/swap/v2/quote/ticker
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | No | Trading pair. If not sent, returns all |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| symbol | string | Trading pair |
| priceChange | string | Price change |
| priceChangePercent | string | Price change percent |
| lastPrice | string | Last price |
| lastQty | string | Last quantity |
| highPrice | string | High price |
| lowPrice | string | Low price |
| volume | string | Volume |
| quoteVolume | string | Quote volume |
| openPrice | string | Open price |
| openTime | int64 | Open time |
| closeTime | int64 | Close time |

---

### Account & Trading (Private)

#### Get Account Balance
```
GET /openApi/swap/v2/user/balance
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| timestamp | int64 | Yes | Request timestamp (milliseconds) |
| recvWindow | int64 | No | Request valid time window (milliseconds) |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| asset | string | Asset name |
| balance | string | Total balance |
| equity | string | Equity |
| unrealizedProfit | string | Unrealized PnL |
| realisedProfit | string | Realized PnL |
| availableMargin | string | Available margin |
| usedMargin | string | Used margin |
| freezedMargin | string | Frozen margin |

---

#### Get Positions
```
GET /openApi/swap/v2/user/positions
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | No | Trading pair, e.g., BTC-USDT |
| timestamp | int64 | Yes | Request timestamp (milliseconds) |
| recvWindow | int64 | No | Request valid time window |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| symbol | string | Trading pair |
| positionId | string | Position ID |
| positionSide | string | Position side: LONG/SHORT |
| isolated | bool | true=isolated, false=cross |
| positionAmt | string | Position amount |
| availableAmt | string | Available amount to close |
| unrealizedProfit | string | Unrealized PnL |
| realisedProfit | string | Realized PnL |
| initialMargin | string | Margin |
| avgPrice | string | Average entry price |
| leverage | int | Leverage |
| liquidationPrice | string | Liquidation price |

---

#### Place Order
```
POST /openApi/swap/v2/trade/order
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair, e.g., BTC-USDT |
| type | string | Yes | LIMIT, MARKET, STOP_MARKET, TAKE_PROFIT_MARKET, STOP, TAKE_PROFIT, TRIGGER_LIMIT, TRIGGER_MARKET |
| side | string | Yes | BUY or SELL |
| positionSide | string | No | LONG or SHORT (default: LONG) |
| price | float64 | No | Order price (required for LIMIT) |
| quantity | float64 | No | Order quantity |
| stopPrice | float64 | No | Trigger price (for STOP/TAKE_PROFIT orders) |
| timestamp | int64 | Yes | Request timestamp (milliseconds) |
| clientOrderID | string | No | Custom order ID (1-40 characters) |
| recvWindow | int64 | No | Request valid time window |
| timeInForce | string | No | PostOnly, GTC, IOC, FOK |

**Order Type Requirements:**
- LIMIT: quantity, price required
- MARKET: quantity required
- TRIGGER_LIMIT, STOP, TAKE_PROFIT: quantity, stopPrice, price required
- STOP_MARKET, TAKE_PROFIT_MARKET, TRIGGER_MARKET: quantity, stopPrice required

**Position Side Combinations:**
- Open Long: side=BUY & positionSide=LONG
- Close Long: side=SELL & positionSide=LONG
- Open Short: side=SELL & positionSide=SHORT
- Close Short: side=BUY & positionSide=SHORT

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| symbol | string | Trading pair |
| orderId | int64 | Order ID |
| side | string | Buy/Sell direction |
| type | string | Order type |
| positionSide | string | Position side |
| clientOrderID | string | Custom order ID |

---

#### Batch Place Orders
```
POST /openApi/swap/v2/trade/batchOrders
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| batchOrders | LIST<Order> | Yes | Order list (max 5 orders) |
| timestamp | int64 | Yes | Request timestamp (milliseconds) |
| recvWindow | int64 | No | Request valid time window |

---

#### Cancel Order
```
DELETE /openApi/swap/v2/trade/order
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair |
| orderId | int64 | No | Order ID |
| clientOrderId | string | No | Custom order ID |
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

---

#### Cancel Batch Orders
```
DELETE /openApi/swap/v2/trade/batchOrders
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair |
| orderIdList | LIST<int64> | No | Order ID list (max 10) |
| ClientOrderIDList | LIST<string> | No | Custom order ID list (max 10) |
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

---

#### Cancel All Orders
```
DELETE /openApi/swap/v2/trade/allOpenOrders
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair |
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

---

#### Close All Positions
```
POST /openApi/swap/v2/trade/closeAllPositions
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

---

#### Query Order
```
GET /openApi/swap/v2/trade/order
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair |
| orderId | int64 | No | Order ID |
| clientOrderId | string | No | Custom order ID |
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| symbol | string | Trading pair |
| orderId | int64 | Order ID |
| price | string | Order price |
| origQty | string | Original quantity |
| executedQty | string | Executed quantity |
| avgPrice | string | Average price |
| cumQuote | string | Cumulative quote |
| stopPrice | string | Trigger price |
| type | string | Order type |
| side | string | Buy/Sell |
| positionSide | string | Position side |
| status | string | NEW, PENDING, PARTIALLY_FILLED, FILLED, CANCELED, FAILED |
| profit | string | PnL |
| commission | string | Fee |
| time | int64 | Order time |
| updateTime | int64 | Update time |

---

#### Query All Open Orders
```
GET /openApi/swap/v2/trade/openOrders
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | No | Trading pair |
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

---

#### Query Order History
```
GET /openApi/swap/v2/trade/allOrders
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair |
| orderId | int64 | No | Return orders after this orderId |
| startTime | int64 | No | Start time (milliseconds) |
| endTime | int64 | No | End time (milliseconds) |
| limit | int | Yes | Default: 500, Max: 1000 |
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

---

#### Query Trade History
```
GET /openApi/swap/v2/trade/allFillOrders
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair |
| orderId | int64 | No | Return trades after this orderId |
| startTs | int64 | No | Start time (milliseconds) |
| endTs | int64 | No | End time (milliseconds) |
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

---

#### Query Margin Type
```
GET /openApi/swap/v2/trade/marginType
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair |
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| marginType | string | ISOLATED or CROSSED |

---

#### Switch Margin Type
```
POST /openApi/swap/v2/trade/marginType
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair |
| marginType | string | Yes | ISOLATED or CROSSED |
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

---

#### Query Leverage
```
GET /openApi/swap/v2/trade/leverage
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair |
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| longLeverage | int64 | Long position leverage |
| shortLeverage | int64 | Short position leverage |

---

#### Switch Leverage
```
POST /openApi/swap/v2/trade/leverage
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair |
| side | string | Yes | LONG or SHORT |
| leverage | int | Yes | Leverage value |
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

---

#### Adjust Position Margin
```
POST /openApi/swap/v2/trade/positionMargin
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair |
| amount | float64 | Yes | Margin amount |
| type | int | Yes | 1=Add, 2=Reduce |
| positionSide | string | No | LONG or SHORT |
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

---

#### Get Income History
```
GET /openApi/swap/v2/user/income
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | No | Trading pair |
| incomeType | string | No | TRANSFER, REALIZED_PNL, FUNDING_FEE, TRADING_FEE, INSURANCE_CLEAR, TRIAL_FUND, ADL, SYSTEM_DEDUCTION |
| startTime | int64 | No | Start time (milliseconds) |
| endTime | int64 | No | End time (milliseconds) |
| limit | int | No | Default: 100, Max: 1000 |
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

---

#### Get Commission Rate
```
GET /openApi/swap/v2/user/commissionRate
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| symbol | string | Yes | Trading pair |
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| symbol | string | Trading pair |
| takerCommissionRate | string | Taker fee rate |
| makerCommissionRate | string | Maker fee rate |

---

### Listen Key (User Data Stream)

#### Create Listen Key
```
POST /openApi/user/auth/userDataStream
```

**Headers:**
| Header | Required | Description |
|--------|----------|-------------|
| X-BX-APIKEY | Yes | API Key |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| listenKey | string | Listen key for WebSocket subscription |

---

#### Extend Listen Key
```
PUT /openApi/user/auth/userDataStream
```

Extends validity to 60 minutes. Recommended to send ping every 30 minutes.

**Headers:**
| Header | Required | Description |
|--------|----------|-------------|
| X-BX-APIKEY | Yes | API Key |

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| listenKey | string | Yes | Listen key to extend |

---

#### Delete Listen Key
```
DELETE /openApi/user/auth/userDataStream
```

**Headers:**
| Header | Required | Description |
|--------|----------|-------------|
| X-BX-APIKEY | Yes | API Key |

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| listenKey | string | Yes | Listen key to delete |

---

## WebSocket API

### Market Data Stream

**Connection URL:** `wss://open-api-swap.bingx.com/swap-market`

#### Subscribe Message Format
```json
{
    "id": "unique_id",
    "reqType": "sub",
    "dataType": "{subscription_type}"
}
```

#### Unsubscribe Message Format
```json
{
    "id": "unique_id",
    "reqType": "unsub",
    "dataType": "{subscription_type}"
}
```

---

#### Market Depth Subscription
```
dataType: market.depth.{symbol}@{levels}
```

Example: `market.depth.BTC-USDT@100`

Levels: 5, 10, 20, 50, 100

**Push Data:**
| Parameter | Type | Description |
|-----------|------|-------------|
| dataType | string | Subscription type |
| data | object | Depth data |
| asks | array | Ask orders [price, quantity] |
| bids | array | Bid orders [price, quantity] |
| T | int64 | Timestamp |

---

#### Trade Subscription
```
dataType: market.trade.detail.{symbol}
```

Example: `market.trade.detail.BTC-USDT`

**Push Data:**
| Parameter | Type | Description |
|-----------|------|-------------|
| dataType | string | Subscription type |
| data | object | Trade data |
| trades | array | Trade list |
| time | int64 | Trade time |
| makerSide | string | Bid/Ask |
| price | string | Trade price |
| volume | string | Trade quantity |

---

#### K-Line Subscription
```
dataType: market.kline.{symbol}_{interval}
```

Example: `market.kline.BTC-USDT_1m`

Intervals: 1m, 3m, 5m, 15m, 30m, 1h, 2h, 4h, 6h, 8h, 12h, 1d, 3d, 1w, 1M

**Push Data:**
| Parameter | Type | Description |
|-----------|------|-------------|
| dataType | string | Subscription type |
| data | object | K-line data |
| c | string | Close price |
| h | string | High price |
| l | string | Low price |
| o | string | Open price |
| v | string | Volume |
| T | int64 | Time |

---

### User Data Stream

**Connection URL:** `wss://open-api-swap.bingx.com/swap-market?listenKey={listenKey}`

#### Account Update Event
Event type: `ACCOUNT_UPDATE`

**Push Data:**
| Parameter | Type | Description |
|-----------|------|-------------|
| e | string | Event type: ACCOUNT_UPDATE |
| E | int64 | Event time |
| a | object | Account update |
| B | array | Balance list |
| a | string | Asset |
| wb | string | Wallet balance |
| cw | string | Cross wallet balance |
| bc | string | Balance change |
| P | array | Position list |
| s | string | Symbol |
| pa | string | Position amount |
| ep | string | Entry price |
| up | string | Unrealized PnL |
| mt | string | Margin type |
| iw | string | Isolated wallet (if isolated) |
| ps | string | Position side |

---

#### Order Update Event
Event type: `ORDER_TRADE_UPDATE`

**Push Data:**
| Parameter | Type | Description |
|-----------|------|-------------|
| e | string | Event type: ORDER_TRADE_UPDATE |
| E | int64 | Event time |
| o | object | Order info |
| s | string | Symbol |
| c | string | Client order ID |
| i | int64 | Order ID |
| S | string | Side: BUY/SELL |
| o | string | Order type |
| q | string | Order quantity |
| p | string | Order price |
| ap | string | Average price |
| x | string | Execution type: NEW, TRADE, CANCELED |
| X | string | Order status: NEW, PARTIALLY_FILLED, FILLED, CANCELED |
| N | string | Commission asset |
| n | string | Commission |
| T | int64 | Trade time |
| wt | string | Trigger price type |
| ps | string | Position side |
| rp | string | Realized PnL |
| z | string | Cumulative filled quantity |

---

#### Listen Key Expired Event
Event type: `listenKeyExpired`

**Push Data:**
| Parameter | Type | Description |
|-----------|------|-------------|
| e | string | Event type: listenKeyExpired |
| E | int64 | Event time |
| listenKey | string | Expired listen key |

---

## Common Error Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 100001 | Signature verification failed |
| 100202 | Insufficient balance |
| 100400 | Invalid parameter |
| 100440 | Order price exceeds limit |
| 100500 | Internal server error |
| 100503 | Server busy |
| 80001 | Request frequency limit |
| 80012 | Service unavailable |
| 80014 | Invalid symbol |
| 80016 | Order does not exist |
| 80017 | Position does not exist |

---

## Rate Limits

- Market endpoints: 20 requests/second
- Account endpoints: 10 requests/second
- Trade endpoints: 10 requests/second
- WebSocket: 10 subscriptions/connection

---

## Symbol Format

- Trading pairs use format: `{BASE}-{QUOTE}`, e.g., `BTC-USDT`, `ETH-USDT`
- Always use uppercase letters

---

## Timestamp

- All timestamps are in milliseconds
- Request timestamp must be within ±5000ms of server time
- Use `recvWindow` parameter to adjust valid time window

---

## Spot API (For Asset Info)

### Get Deposit/Withdraw Status
```
GET /openApi/wallets/v1/capital/config/getall
```

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| timestamp | int64 | Yes | Request timestamp |
| recvWindow | int64 | No | Request valid time window |

**Response:**
| Parameter | Type | Description |
|-----------|------|-------------|
| coin | string | Coin name |
| name | string | Coin full name |
| networkList | array | Network list |
| network | string | Network name |
| withdrawEnable | bool | Withdraw enabled |
| depositEnable | bool | Deposit enabled |
| withdrawFee | string | Withdraw fee |
| withdrawMin | string | Minimum withdraw |
| withdrawMax | string | Maximum withdraw |

---

## Notes

1. All private endpoints require authentication (API Key + Signature)
2. WebSocket connections should implement heartbeat (ping/pong every 30 seconds)
3. For user data streams, create a new listenKey before the old one expires (60 minutes)
4. Order quantity is in contract units (base asset), not USDT
5. Position side must be specified for hedge mode (LONG/SHORT)
