# Bybit V5 API Documentation

## Overview

The Bybit V5 API provides a unified interface for trading across Spot, Derivatives (USDT/USDC Perpetual, Inverse Perpetual, Futures), and Options. This documentation covers all endpoints relevant for futures arbitrage trading.

**Base URLs:**
- **REST API Mainnet:** `https://api.bybit.com`
- **REST API Testnet:** `https://api-testnet.bybit.com`

**WebSocket URLs:**
- **Public Stream Mainnet:**
  - Spot: `wss://stream.bybit.com/v5/public/spot`
  - Linear (USDT/USDC Perps & Futures): `wss://stream.bybit.com/v5/public/linear`
  - Inverse: `wss://stream.bybit.com/v5/public/inverse`
  - Options: `wss://stream.bybit.com/v5/public/option`
- **Private Stream Mainnet:** `wss://stream.bybit.com/v5/private`
- **WebSocket Trade (Order Entry) Mainnet:** `wss://stream.bybit.com/v5/trade`
- **Testnet:** Replace `stream.bybit.com` with `stream-testnet.bybit.com`

---

## REST API

### Market Data Endpoints

#### 1. Get Instruments Info
Query for the instrument specification of online trading pairs.

```
GET /v5/market/instruments-info
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type: `spot`, `linear`, `inverse`, `option` |
| symbol | false | string | Symbol name, like `BTCUSDT` (uppercase only) |
| status | false | string | Symbol status filter. Default returns only `Trading` symbols |
| baseCoin | false | string | Base coin (uppercase). Applies to linear, inverse, option |
| limit | false | integer | Limit for data size per page. [1, 1000]. Default: 500 |
| cursor | false | string | Cursor for pagination |

**Response Parameters (Linear/Inverse):**
| Field | Type | Description |
|-------|------|-------------|
| symbol | string | Symbol name |
| contractType | string | Contract type: `LinearPerpetual`, `LinearFutures`, `InversePerpetual`, `InverseFutures` |
| status | string | Instrument status: `PreLaunch`, `Trading`, `Settling`, `Delivering`, `Closed` |
| baseCoin | string | Base coin |
| quoteCoin | string | Quote coin |
| settleCoin | string | Settle coin |
| launchTime | string | Launch timestamp (ms) |
| deliveryTime | string | Delivery timestamp (ms) |
| fundingInterval | integer | Funding interval (minutes) |
| leverageFilter | object | Min/max leverage and step |
| priceFilter | object | Min/max price and tick size |
| lotSizeFilter | object | Min/max order qty, step, notional value |
| upperFundingRate | string | Upper limit of funding rate |
| lowerFundingRate | string | Lower limit of funding rate |

**Example:**
```http
GET /v5/market/instruments-info?category=linear&symbol=BTCUSDT
```

---

#### 2. Get Tickers
Query for the latest price snapshot, best bid/ask price, and trading volume in the last 24 hours.

```
GET /v5/market/tickers
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type: `spot`, `linear`, `inverse`, `option` |
| symbol | false | string | Symbol name (uppercase only) |
| baseCoin | false | string | Base coin (option only) |

**Response Parameters (Linear/Inverse):**
| Field | Type | Description |
|-------|------|-------------|
| symbol | string | Symbol name |
| lastPrice | string | Last traded price |
| indexPrice | string | Index price |
| markPrice | string | Mark price |
| prevPrice24h | string | Market price 24 hours ago |
| price24hPcnt | string | 24h price change percentage |
| highPrice24h | string | Highest price in 24h |
| lowPrice24h | string | Lowest price in 24h |
| volume24h | string | Volume for 24h |
| turnover24h | string | Turnover for 24h |
| openInterest | string | Open interest size |
| openInterestValue | string | Open interest value |
| fundingRate | string | Current funding rate |
| nextFundingTime | string | Next funding time (ms) |
| bid1Price | string | Best bid price |
| bid1Size | string | Best bid size |
| ask1Price | string | Best ask price |
| ask1Size | string | Best ask size |
| basisRate | string | Basis rate (futures only) |
| deliveryTime | string | Delivery date (futures only) |

**Example:**
```http
GET /v5/market/tickers?category=linear&symbol=BTCUSDT
```

---

#### 3. Get Orderbook
Query for orderbook depth data.

```
GET /v5/market/orderbook
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type: `spot`, `linear`, `inverse`, `option` |
| symbol | true | string | Symbol name (uppercase only) |
| limit | false | integer | Limit size. Linear/Inverse: [1, 500], Default: 25. Spot: [1, 200], Default: 1 |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| s | string | Symbol name |
| b | array | Bids [price, size] - sorted descending by price |
| a | array | Asks [price, size] - sorted ascending by price |
| ts | integer | Timestamp (ms) when system generates data |
| u | integer | Update ID |
| seq | integer | Cross sequence |
| cts | integer | Timestamp from matching engine |

**Example:**
```http
GET /v5/market/orderbook?category=linear&symbol=BTCUSDT&limit=50
```

---

#### 4. Get Kline (Candlestick)
Query for historical klines/candlesticks.

```
GET /v5/market/kline
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | false | string | Product type. Default: `linear` |
| symbol | true | string | Symbol name (uppercase only) |
| interval | true | string | Kline interval: `1`, `3`, `5`, `15`, `30`, `60`, `120`, `240`, `360`, `720`, `D`, `W`, `M` |
| start | false | integer | Start timestamp (ms) |
| end | false | integer | End timestamp (ms) |
| limit | false | integer | Limit [1, 1000]. Default: 200 |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| list | array | Array of candles (sorted reverse by startTime) |
| list[0] | string | Start time (ms) |
| list[1] | string | Open price |
| list[2] | string | High price |
| list[3] | string | Low price |
| list[4] | string | Close price |
| list[5] | string | Volume |
| list[6] | string | Turnover |

**Example:**
```http
GET /v5/market/kline?category=linear&symbol=BTCUSDT&interval=60&limit=200
```

---

#### 5. Get Index Price Kline
Query for historical index price klines.

```
GET /v5/market/index-price-kline
```

**Parameters:** Same as Get Kline

---

#### 6. Get Funding Rate History
Query for historical funding rates.

```
GET /v5/market/funding/history
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type: `linear`, `inverse` |
| symbol | true | string | Symbol name (uppercase only) |
| startTime | false | integer | Start timestamp (ms) |
| endTime | false | integer | End timestamp (ms) |
| limit | false | integer | Limit [1, 200]. Default: 200 |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| symbol | string | Symbol name |
| fundingRate | string | Funding rate |
| fundingRateTimestamp | string | Funding rate timestamp (ms) |

**Example:**
```http
GET /v5/market/funding/history?category=linear&symbol=BTCUSDT&limit=100
```

---

#### 7. Get Recent Public Trades
Query recent public trading history.

```
GET /v5/market/recent-trade
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type: `spot`, `linear`, `inverse`, `option` |
| symbol | false | string | Symbol name (required for spot/linear/inverse) |
| limit | false | integer | Spot: [1, 60], Default: 60. Others: [1, 1000], Default: 500 |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| execId | string | Execution ID |
| symbol | string | Symbol name |
| price | string | Trade price |
| size | string | Trade size |
| side | string | Side: `Buy`, `Sell` |
| time | string | Trade time (ms) |
| isBlockTrade | boolean | Block trade indicator |

---

#### 8. Get Open Interest
Get the open interest of each symbol.

```
GET /v5/market/open-interest
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type: `linear`, `inverse` |
| symbol | true | string | Symbol name (uppercase only) |
| intervalTime | true | string | Interval: `5min`, `15min`, `30min`, `1h`, `4h`, `1d` |
| startTime | false | integer | Start timestamp (ms) |
| endTime | false | integer | End timestamp (ms) |
| limit | false | integer | Limit [1, 200]. Default: 50 |

---

#### 9. Get Risk Limit
Query for risk limit margin parameters.

```
GET /v5/market/risk-limit
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type: `linear`, `inverse` |
| symbol | false | string | Symbol name (uppercase only) |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| id | integer | Risk ID |
| symbol | string | Symbol name |
| riskLimitValue | string | Position limit |
| maintenanceMargin | number | Maintenance margin rate |
| initialMargin | number | Initial margin rate |
| maxLeverage | string | Max allowed leverage |

---

### Trading Endpoints

#### 1. Place Order
Create a new order.

```
POST /v5/order/create
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type: `linear`, `inverse`, `spot`, `option` |
| symbol | true | string | Symbol name (uppercase only) |
| side | true | string | `Buy` or `Sell` |
| orderType | true | string | `Market` or `Limit` |
| qty | true | string | Order quantity |
| price | false | string | Order price (required for Limit orders) |
| timeInForce | false | string | `GTC`, `IOC`, `FOK`, `PostOnly`. Default: `GTC` |
| positionIdx | false | integer | Position mode: `0` (one-way), `1` (hedge-buy), `2` (hedge-sell) |
| orderLinkId | false | string | Custom order ID (max 36 chars) |
| reduceOnly | false | boolean | Reduce only order |
| closeOnTrigger | false | boolean | Close on trigger |
| triggerPrice | false | string | Trigger price for conditional orders |
| triggerDirection | false | integer | `1` (rise) or `2` (fall) |
| triggerBy | false | string | `LastPrice`, `IndexPrice`, `MarkPrice` |
| takeProfit | false | string | Take profit price |
| stopLoss | false | string | Stop loss price |
| tpslMode | false | string | `Full` or `Partial` |
| tpOrderType | false | string | `Market` or `Limit` |
| slOrderType | false | string | `Market` or `Limit` |
| tpLimitPrice | false | string | TP limit price (when tpOrderType=Limit) |
| slLimitPrice | false | string | SL limit price (when slOrderType=Limit) |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| orderId | string | Order ID |
| orderLinkId | string | User customised order ID |

**Example:**
```json
POST /v5/order/create
{
    "category": "linear",
    "symbol": "BTCUSDT",
    "side": "Buy",
    "orderType": "Limit",
    "qty": "0.001",
    "price": "25000",
    "timeInForce": "GTC",
    "positionIdx": 0
}
```

---

#### 2. Amend Order
Modify an unfilled or partially filled order.

```
POST /v5/order/amend
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type |
| symbol | true | string | Symbol name |
| orderId | false | string | Order ID (either orderId or orderLinkId required) |
| orderLinkId | false | string | Custom order ID |
| qty | false | string | New quantity |
| price | false | string | New price |
| triggerPrice | false | string | New trigger price |
| takeProfit | false | string | New TP price ("0" to cancel) |
| stopLoss | false | string | New SL price ("0" to cancel) |

---

#### 3. Cancel Order
Cancel an active order.

```
POST /v5/order/cancel
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type |
| symbol | true | string | Symbol name |
| orderId | false | string | Order ID (either orderId or orderLinkId required) |
| orderLinkId | false | string | Custom order ID |
| orderFilter | false | string | `Order`, `tpslOrder`, `StopOrder` (spot only) |

---

#### 4. Cancel All Orders
Cancel all open orders.

```
POST /v5/order/cancel-all
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type |
| symbol | false | string | Symbol name |
| baseCoin | false | string | Base coin |
| settleCoin | false | string | Settle coin |
| orderFilter | false | string | Order filter type |

---

#### 5. Get Open & Closed Orders
Query unfilled or partially filled orders.

```
GET /v5/order/realtime
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type |
| symbol | false | string | Symbol name |
| baseCoin | false | string | Base coin |
| settleCoin | false | string | Settle coin |
| orderId | false | string | Order ID |
| orderLinkId | false | string | Custom order ID |
| openOnly | false | integer | `0` (open only, default), `1` (include recent 500 closed) |
| orderFilter | false | string | Order filter |
| limit | false | integer | Limit [1, 50]. Default: 20 |
| cursor | false | string | Cursor for pagination |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| orderId | string | Order ID |
| orderLinkId | string | Custom order ID |
| symbol | string | Symbol name |
| price | string | Order price |
| qty | string | Order quantity |
| side | string | `Buy` or `Sell` |
| orderStatus | string | Order status |
| avgPrice | string | Average filled price |
| leavesQty | string | Remaining unfilled qty |
| cumExecQty | string | Cumulative executed qty |
| cumExecValue | string | Cumulative executed value |
| cumExecFee | string | Cumulative fee |
| timeInForce | string | Time in force |
| orderType | string | Order type |
| createdTime | string | Created timestamp (ms) |
| updatedTime | string | Updated timestamp (ms) |

---

#### 6. Get Order History
Query order history.

```
GET /v5/order/history
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type |
| symbol | false | string | Symbol name |
| orderId | false | string | Order ID |
| orderLinkId | false | string | Custom order ID |
| orderStatus | false | string | Order status filter |
| startTime | false | integer | Start timestamp (ms) |
| endTime | false | integer | End timestamp (ms) |
| limit | false | integer | Limit [1, 50]. Default: 20 |
| cursor | false | string | Cursor for pagination |

---

#### 7. Get Trade History (Executions)
Query users' execution records.

```
GET /v5/execution/list
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type |
| symbol | false | string | Symbol name |
| orderId | false | string | Order ID |
| orderLinkId | false | string | Custom order ID |
| baseCoin | false | string | Base coin |
| startTime | false | integer | Start timestamp (ms) |
| endTime | false | integer | End timestamp (ms) |
| execType | false | string | Execution type |
| limit | false | integer | Limit [1, 100]. Default: 50 |
| cursor | false | string | Cursor for pagination |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| symbol | string | Symbol name |
| orderId | string | Order ID |
| orderLinkId | string | Custom order ID |
| side | string | Side |
| orderPrice | string | Order price |
| orderQty | string | Order quantity |
| leavesQty | string | Remaining qty |
| orderType | string | Order type |
| execFee | string | Execution fee |
| execId | string | Execution ID |
| execPrice | string | Execution price |
| execQty | string | Execution qty |
| execType | string | Execution type |
| execValue | string | Execution value |
| execTime | string | Execution timestamp (ms) |
| isMaker | boolean | Maker indicator |
| feeRate | string | Fee rate |
| closedSize | string | Closed position size |

---

#### 8. Batch Place Order
Place multiple orders in a single request.

```
POST /v5/order/create-batch
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type |
| request | true | array | Array of order objects (max: linear/inverse 20, spot 10) |

Each order in `request` array has the same parameters as Place Order.

**Example:**
```json
POST /v5/order/create-batch
{
    "category": "linear",
    "request": [
        {
            "symbol": "BTCUSDT",
            "side": "Buy",
            "orderType": "Limit",
            "qty": "0.001",
            "price": "25000",
            "timeInForce": "GTC"
        },
        {
            "symbol": "ETHUSDT",
            "side": "Buy",
            "orderType": "Limit",
            "qty": "0.01",
            "price": "1800",
            "timeInForce": "GTC"
        }
    ]
}
```

---

### Position Endpoints

#### 1. Get Position Info
Query real-time position data.

```
GET /v5/position/list
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type: `linear`, `inverse`, `option` |
| symbol | false | string | Symbol name |
| baseCoin | false | string | Base coin (option only) |
| settleCoin | false | string | Settle coin |
| limit | false | integer | Limit [1, 200]. Default: 20 |
| cursor | false | string | Cursor for pagination |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| positionIdx | integer | Position index: `0` (one-way), `1` (buy-hedge), `2` (sell-hedge) |
| symbol | string | Symbol name |
| side | string | Position side: `Buy` (long), `Sell` (short), `""` (empty) |
| size | string | Position size (always positive) |
| avgPrice | string | Average entry price |
| positionValue | string | Position value |
| leverage | string | Position leverage |
| markPrice | string | Mark price |
| liqPrice | string | Liquidation price |
| positionIM | string | Initial margin |
| positionMM | string | Maintenance margin |
| takeProfit | string | Take profit price |
| stopLoss | string | Stop loss price |
| trailingStop | string | Trailing stop |
| unrealisedPnl | string | Unrealised PnL |
| curRealisedPnl | string | Realised PnL for current position |
| cumRealisedPnl | string | Cumulative realised PnL |
| positionStatus | string | Status: `Normal`, `Liq`, `Adl` |
| adlRankIndicator | integer | ADL rank indicator |
| autoAddMargin | integer | Auto add margin: `0` (false), `1` (true) |
| createdTime | string | Position created timestamp (ms) |
| updatedTime | string | Position updated timestamp (ms) |

---

#### 2. Set Leverage
Set the leverage for a symbol.

```
POST /v5/position/set-leverage
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type: `linear`, `inverse` |
| symbol | true | string | Symbol name |
| buyLeverage | true | string | Buy leverage [1, max] |
| sellLeverage | true | string | Sell leverage [1, max] |

---

#### 3. Switch Position Mode
Switch between one-way and hedge mode (USDT perpetual only).

```
POST /v5/position/switch-mode
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | `linear` |
| symbol | false | string | Symbol name |
| coin | false | string | Coin (for batch switch) |
| mode | true | integer | `0` (one-way), `3` (hedge mode) |

---

#### 4. Get Closed PnL
Query closed profit and loss records.

```
GET /v5/position/closed-pnl
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type: `linear` |
| symbol | false | string | Symbol name |
| startTime | false | integer | Start timestamp (ms) |
| endTime | false | integer | End timestamp (ms) |
| limit | false | integer | Limit [1, 100]. Default: 50 |
| cursor | false | string | Cursor for pagination |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| symbol | string | Symbol name |
| orderId | string | Order ID |
| side | string | Side |
| qty | string | Order qty |
| orderPrice | string | Order price |
| orderType | string | Order type |
| closedSize | string | Closed size |
| avgEntryPrice | string | Average entry price |
| avgExitPrice | string | Average exit price |
| closedPnl | string | Closed PnL |
| leverage | string | Leverage |
| createdTime | string | Created timestamp (ms) |

---

### Account Endpoints

#### 1. Get Wallet Balance
Query wallet balance and asset information.

```
GET /v5/account/wallet-balance
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| accountType | true | string | Account type: `UNIFIED` |
| coin | false | string | Coin name (comma-separated for multiple) |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| accountType | string | Account type |
| totalEquity | string | Total equity (USD) |
| totalWalletBalance | string | Total wallet balance (USD) |
| totalMarginBalance | string | Total margin balance (USD) |
| totalAvailableBalance | string | Total available balance (USD) |
| totalPerpUPL | string | Total Perp/Futures unrealised PnL (USD) |
| totalInitialMargin | string | Total initial margin (USD) |
| totalMaintenanceMargin | string | Total maintenance margin (USD) |
| accountIMRate | string | Account IM rate |
| accountMMRate | string | Account MM rate |
| coin | array | Coin details array |
| > coin | string | Coin name |
| > equity | string | Coin equity |
| > usdValue | string | USD value |
| > walletBalance | string | Wallet balance |
| > borrowAmount | string | Borrow amount |
| > unrealisedPnl | string | Unrealised PnL |
| > cumRealisedPnl | string | Cumulative realised PnL |
| > locked | string | Locked amount (spot orders) |
| > marginCollateral | boolean | Can be used as collateral |
| > collateralSwitch | boolean | Collateral enabled by user |

---

#### 2. Get Fee Rate
Get the trading fee rate.

```
GET /v5/account/fee-rate
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| category | true | string | Product type: `spot`, `linear`, `inverse`, `option` |
| symbol | false | string | Symbol name |
| baseCoin | false | string | Base coin (option only) |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| symbol | string | Symbol name |
| baseCoin | string | Base coin |
| takerFeeRate | string | Taker fee rate |
| makerFeeRate | string | Maker fee rate |

---

#### 3. Get Account Info
Query account information (margin mode, etc.).

```
GET /v5/account/info
```

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| unifiedMarginStatus | integer | Account status |
| marginMode | string | `ISOLATED_MARGIN`, `REGULAR_MARGIN`, `PORTFOLIO_MARGIN` |
| spotHedgingStatus | string | Spot hedging: `ON`, `OFF` |
| updatedTime | string | Updated timestamp (ms) |

---

### Asset Endpoints

#### 1. Get Coin Info
Query coin information including deposit/withdraw status.

```
GET /v5/asset/coin/query-info
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| coin | false | string | Coin (uppercase only) |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| name | string | Coin name |
| coin | string | Coin symbol |
| remainAmount | string | Max withdraw per transaction |
| chains | array | Chain information |
| > chain | string | Chain name |
| > chainType | string | Chain type |
| > confirmation | string | Confirmations for deposit |
| > withdrawFee | string | Withdraw fee |
| > depositMin | string | Min deposit |
| > withdrawMin | string | Min withdraw |
| > minAccuracy | string | Precision |
| > chainDeposit | string | Deposit status: `0` (suspend), `1` (normal) |
| > chainWithdraw | string | Withdraw status: `0` (suspend), `1` (normal) |
| > contractAddress | string | Contract address |

---

#### 2. Get Deposit Records
Query deposit records.

```
GET /v5/asset/deposit/query-record
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| coin | false | string | Coin |
| startTime | false | integer | Start timestamp (ms) |
| endTime | false | integer | End timestamp (ms) |
| limit | false | integer | Limit [1, 50]. Default: 50 |
| cursor | false | string | Cursor for pagination |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| coin | string | Coin |
| chain | string | Chain |
| amount | string | Amount |
| txID | string | Transaction ID |
| status | integer | Deposit status |
| toAddress | string | Deposit address |
| successAt | string | Success timestamp (ms) |
| confirmations | string | Confirmations |

---

#### 3. Get Withdrawal Records
Query withdrawal records.

```
GET /v5/asset/withdraw/query-record
```

**Request Parameters:**
| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| coin | false | string | Coin |
| withdrawType | false | integer | `0` (on-chain), `1` (off-chain), `2` (all) |
| startTime | false | integer | Start timestamp (ms) |
| endTime | false | integer | End timestamp (ms) |
| limit | false | integer | Limit [1, 50]. Default: 50 |
| cursor | false | string | Cursor for pagination |

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| coin | string | Coin |
| chain | string | Chain |
| amount | string | Amount |
| txID | string | Transaction ID |
| status | string | Withdraw status |
| toAddress | string | To address |
| withdrawFee | string | Withdraw fee |
| createTime | string | Create timestamp (ms) |
| updateTime | string | Update timestamp (ms) |
| withdrawId | string | Withdraw ID |
| withdrawType | integer | Withdraw type |

---

## WebSocket API

### Connection & Authentication

**Public WebSocket:** No authentication required.

**Private WebSocket Authentication:**
```json
{
    "op": "auth",
    "args": [
        "API_KEY",
        1711010121452,  // expires timestamp (ms)
        "SIGNATURE"     // HMAC SHA256 signature
    ]
}
```

**Signature Generation:**
```
signature = HMAC_SHA256(api_secret, "GET/realtime" + expires)
```

**Heartbeat (Ping):**
```json
{
    "op": "ping"
}
```

Send ping every 20 seconds to maintain connection.

---

### Public WebSocket Topics

#### 1. Orderbook Stream
Subscribe to orderbook depth updates.

**Topic:** `orderbook.{depth}.{symbol}`

**Depths:**
- Linear/Inverse: `1` (10ms), `50` (20ms), `200` (100ms), `1000` (200ms)
- Spot: `1` (10ms), `50` (20ms), `200` (200ms), `1000` (200ms)
- Option: `25` (20ms), `100` (100ms)

**Subscribe:**
```json
{
    "op": "subscribe",
    "args": ["orderbook.50.BTCUSDT"]
}
```

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| topic | string | Topic name |
| type | string | `snapshot` or `delta` |
| ts | number | Timestamp (ms) |
| data.s | string | Symbol |
| data.b | array | Bids [price, size] |
| data.a | array | Asks [price, size] |
| data.u | integer | Update ID |
| data.seq | integer | Cross sequence |
| cts | number | Matching engine timestamp |

**Delta Processing:**
- Size = 0: delete entry
- New price: insert entry
- Existing price: update size

---

#### 2. Ticker Stream
Subscribe to ticker updates.

**Topic:** `tickers.{symbol}`

**Push Frequency:** Derivatives/Options: 100ms, Spot: 50ms

**Subscribe:**
```json
{
    "op": "subscribe",
    "args": ["tickers.BTCUSDT"]
}
```

**Response Parameters (Linear/Inverse):**
| Field | Type | Description |
|-------|------|-------------|
| symbol | string | Symbol name |
| lastPrice | string | Last price |
| markPrice | string | Mark price |
| indexPrice | string | Index price |
| price24hPcnt | string | 24h price change % |
| highPrice24h | string | 24h high |
| lowPrice24h | string | 24h low |
| volume24h | string | 24h volume |
| turnover24h | string | 24h turnover |
| openInterest | string | Open interest |
| openInterestValue | string | Open interest value |
| fundingRate | string | Current funding rate |
| nextFundingTime | string | Next funding time (ms) |
| bid1Price | string | Best bid price |
| bid1Size | string | Best bid size |
| ask1Price | string | Best ask price |
| ask1Size | string | Best ask size |

---

#### 3. Trade Stream
Subscribe to recent trades.

**Topic:** `publicTrade.{symbol}`

**Push Frequency:** Real-time

**Subscribe:**
```json
{
    "op": "subscribe",
    "args": ["publicTrade.BTCUSDT"]
}
```

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| T | number | Trade timestamp (ms) |
| s | string | Symbol |
| S | string | Side: `Buy`, `Sell` |
| v | string | Trade size |
| p | string | Trade price |
| L | string | Price direction |
| i | string | Trade ID |
| BT | boolean | Block trade |

---

### Private WebSocket Topics

#### 1. Order Stream
Subscribe to order updates.

**Topic:** `order` (all categories) or `order.linear`, `order.inverse`, `order.spot`, `order.option`

**Subscribe:**
```json
{
    "op": "subscribe",
    "args": ["order"]
}
```

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| category | string | Product type |
| orderId | string | Order ID |
| orderLinkId | string | Custom order ID |
| symbol | string | Symbol |
| price | string | Order price |
| qty | string | Order qty |
| side | string | Side |
| orderStatus | string | Order status |
| avgPrice | string | Average filled price |
| leavesQty | string | Remaining qty |
| cumExecQty | string | Executed qty |
| cumExecValue | string | Executed value |
| cumExecFee | string | Executed fee |
| timeInForce | string | Time in force |
| orderType | string | Order type |
| triggerPrice | string | Trigger price |
| takeProfit | string | TP price |
| stopLoss | string | SL price |
| reduceOnly | boolean | Reduce only |
| closedPnl | string | Closed PnL |
| createdTime | string | Created timestamp (ms) |
| updatedTime | string | Updated timestamp (ms) |

---

#### 2. Position Stream
Subscribe to position updates.

**Topic:** `position` (all categories) or `position.linear`, `position.inverse`, `position.option`

**Subscribe:**
```json
{
    "op": "subscribe",
    "args": ["position"]
}
```

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| category | string | Product type |
| symbol | string | Symbol |
| side | string | Position side |
| size | string | Position size |
| positionIdx | integer | Position index |
| positionValue | string | Position value |
| entryPrice | string | Entry price |
| markPrice | string | Mark price |
| leverage | string | Leverage |
| positionIM | string | Initial margin |
| positionMM | string | Maintenance margin |
| liqPrice | string | Liquidation price |
| takeProfit | string | TP price |
| stopLoss | string | SL price |
| unrealisedPnl | string | Unrealised PnL |
| curRealisedPnl | string | Current realised PnL |
| cumRealisedPnl | string | Cumulative realised PnL |
| positionStatus | string | Status |
| updatedTime | string | Updated timestamp (ms) |

---

#### 3. Execution Stream
Subscribe to trade executions.

**Topic:** `execution` (all categories) or `execution.linear`, `execution.inverse`, `execution.spot`, `execution.option`

**Subscribe:**
```json
{
    "op": "subscribe",
    "args": ["execution"]
}
```

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| category | string | Product type |
| symbol | string | Symbol |
| orderId | string | Order ID |
| orderLinkId | string | Custom order ID |
| side | string | Side |
| orderPrice | string | Order price |
| orderQty | string | Order qty |
| leavesQty | string | Remaining qty |
| orderType | string | Order type |
| execFee | string | Execution fee |
| execId | string | Execution ID |
| execPrice | string | Execution price |
| execQty | string | Execution qty |
| execType | string | Execution type |
| execValue | string | Execution value |
| execTime | string | Execution timestamp (ms) |
| isMaker | boolean | Maker indicator |
| feeRate | string | Fee rate |
| closedSize | string | Closed size |
| execPnl | string | Execution PnL |

---

#### 4. Wallet Stream
Subscribe to wallet balance updates.

**Topic:** `wallet`

**Subscribe:**
```json
{
    "op": "subscribe",
    "args": ["wallet"]
}
```

**Response Parameters:**
| Field | Type | Description |
|-------|------|-------------|
| accountType | string | Account type |
| totalEquity | string | Total equity (USD) |
| totalWalletBalance | string | Total wallet balance (USD) |
| totalMarginBalance | string | Total margin balance (USD) |
| totalAvailableBalance | string | Total available balance (USD) |
| totalPerpUPL | string | Total Perp/Futures UPL |
| totalInitialMargin | string | Total IM |
| totalMaintenanceMargin | string | Total MM |
| accountIMRate | string | Account IM rate |
| accountMMRate | string | Account MM rate |
| coin | array | Coin details |
| > coin | string | Coin name |
| > equity | string | Equity |
| > usdValue | string | USD value |
| > walletBalance | string | Wallet balance |
| > borrowAmount | string | Borrow amount |
| > unrealisedPnl | string | Unrealised PnL |
| > cumRealisedPnl | string | Cumulative realised PnL |

---

### WebSocket Trade (Low Latency Order Entry)

**URL:** `wss://stream.bybit.com/v5/trade`

**Supports:** USDT Contract, USDC Contract, Spot, Options, Inverse contract

#### Authentication
```json
{
    "op": "auth",
    "args": [
        "API_KEY",
        1711010121452,
        "SIGNATURE"
    ]
}
```

#### Create Order
```json
{
    "reqId": "unique-request-id",
    "header": {
        "X-BAPI-TIMESTAMP": "1711001595207",
        "X-BAPI-RECV-WINDOW": "5000"
    },
    "op": "order.create",
    "args": [{
        "category": "linear",
        "symbol": "BTCUSDT",
        "side": "Buy",
        "orderType": "Limit",
        "qty": "0.001",
        "price": "25000",
        "timeInForce": "GTC"
    }]
}
```

#### Amend Order
```json
{
    "reqId": "unique-request-id",
    "header": {
        "X-BAPI-TIMESTAMP": "1711001595207"
    },
    "op": "order.amend",
    "args": [{
        "category": "linear",
        "symbol": "BTCUSDT",
        "orderId": "xxx-xxx",
        "price": "25500"
    }]
}
```

#### Cancel Order
```json
{
    "reqId": "unique-request-id",
    "header": {
        "X-BAPI-TIMESTAMP": "1711001595207"
    },
    "op": "order.cancel",
    "args": [{
        "category": "linear",
        "symbol": "BTCUSDT",
        "orderId": "xxx-xxx"
    }]
}
```

#### Batch Create Orders
```json
{
    "reqId": "unique-request-id",
    "header": {
        "X-BAPI-TIMESTAMP": "1711001595207"
    },
    "op": "order.create-batch",
    "args": [{
        "category": "linear",
        "request": [
            {
                "symbol": "BTCUSDT",
                "side": "Buy",
                "orderType": "Limit",
                "qty": "0.001",
                "price": "25000"
            },
            {
                "symbol": "ETHUSDT",
                "side": "Buy",
                "orderType": "Limit",
                "qty": "0.01",
                "price": "1800"
            }
        ]
    }]
}
```

#### Response Format
```json
{
    "reqId": "unique-request-id",
    "retCode": 0,
    "retMsg": "OK",
    "op": "order.create",
    "data": {
        "orderId": "xxx-xxx",
        "orderLinkId": ""
    },
    "header": {
        "X-Bapi-Limit": "10",
        "X-Bapi-Limit-Status": "9",
        "X-Bapi-Limit-Reset-Timestamp": "1711001595208",
        "Traceid": "xxx",
        "Timenow": "1711001595209"
    },
    "connId": "connection-id"
}
```

---

## Rate Limits

### IP Limits
- **HTTP:** 600 requests per 5-second window per IP
- **WebSocket:** Max 500 connections per 5-minute window

### API Rate Limits (per UID per second)

| Endpoint | Default | VIP |
|----------|---------|-----|
| POST /v5/order/create | 10/s | 20/s |
| POST /v5/order/amend | 10/s | 10/s |
| POST /v5/order/cancel | 10/s | 20/s |
| POST /v5/order/cancel-all | 10/s | 20/s |
| POST /v5/order/create-batch | 10/s | 20/s |
| POST /v5/order/amend-batch | 10/s | 20/s |
| POST /v5/order/cancel-batch | 10/s | 20/s |
| GET /v5/order/realtime | 50/s | 50/s |
| GET /v5/order/history | 50/s | 50/s |
| GET /v5/execution/list | 50/s | 50/s |
| GET /v5/position/list | 50/s | 50/s |
| GET /v5/account/wallet-balance | 50/s | 50/s |
| GET /v5/account/fee-rate | 10/s | 10/s |
| POST /v5/position/set-leverage | 10/s | 10/s |

### Batch Endpoint Rules
- Batch endpoints have separate rate limits from single order endpoints
- Rate consumption = number of requests × number of orders per request
- Linear/Inverse: 1-20 orders per batch
- Spot: 1-10 orders per batch

### Rate Limit Headers
```
X-Bapi-Limit: 100
X-Bapi-Limit-Status: 99
X-Bapi-Limit-Reset-Timestamp: 1672738134824
```

---

## Authentication

### REST API
**Required Headers:**
```
X-BAPI-API-KEY: your_api_key
X-BAPI-TIMESTAMP: current_timestamp_ms
X-BAPI-SIGN: signature
X-BAPI-RECV-WINDOW: 5000  (optional, default 5000ms)
```

**Signature Generation:**
```
signature = HMAC_SHA256(api_secret, timestamp + api_key + recv_window + query_string_or_body)
```

### Order Status Enum
| Status | Description |
|--------|-------------|
| New | Order placed |
| PartiallyFilled | Partially filled |
| Filled | Fully filled |
| Cancelled | Cancelled |
| Rejected | Rejected |
| PendingCancel | Pending cancel |
| Untriggered | Conditional order not triggered |
| Triggered | Conditional order triggered |
| Deactivated | Conditional order cancelled |

### Execution Type Enum
| Type | Description |
|------|-------------|
| Trade | Normal trade |
| AdlTrade | ADL trade |
| Funding | Funding fee |
| BustTrade | Liquidation trade |
| Settle | Settlement |

---

## Error Codes

| Code | Message |
|------|---------|
| 0 | Success |
| 10001 | Parameter error |
| 10003 | Invalid API key |
| 10004 | Invalid sign |
| 10005 | Permission denied |
| 10006 | Too many requests |
| 10010 | Unmatched IP |
| 10016 | Internal server error |
| 10017 | Invalid access token |
| 10018 | Invalid user status |
| 10027 | Trading banned |
| 110001 | Order does not exist |
| 110003 | Invalid position mode |
| 110004 | Insufficient balance |
| 110005 | Position value exceeded |
| 110006 | Insufficient available balance |
| 110007 | Exceeded position limit |
| 110008 | Exceed maximum order qty |
| 110009 | Order already filled/cancelled |
| 110010 | Price too high |
| 110011 | Price too low |
| 110012 | Invalid orderType |
| 110013 | Invalid qty |
| 110014 | Leverage not changed |
| 110015 | Position not found |
| 110017 | ReduceOnly rules violation |
| 110018 | User ID mismatch |
| 110019 | OrderLinkId duplicated |
| 110020 | Order not modifiable |
| 110021 | Order qty or price exceeded |
| 110022 | Close position only |
| 110024 | Margin mode not supported |
| 110025 | Unable to set auto-add margin |
| 110043 | Risk limit exceeded |
| 110044 | Position leverage mismatch |

---

## Useful Links

- **API Documentation:** https://bybit-exchange.github.io/docs/v5/intro
- **API Explorer:** https://bybit-exchange.github.io/docs/api-explorer/v5/market/instrument
- **Python SDK (pybit):** https://github.com/bybit-exchange/pybit
- **Node.js SDK:** https://www.npmjs.com/package/bybit-api
- **Go SDK:** https://github.com/bybit-exchange/bybit.go.api
- **API Usage Examples:** https://github.com/bybit-exchange/api-usage-examples
- **Postman Collection:** https://github.com/bybit-exchange/QuickStartWithPostman
- **Historical Data:** https://www.bybit.com/derivatives/en/history-data
