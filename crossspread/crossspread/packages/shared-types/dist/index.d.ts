/**
 * CrossSpread Shared Types
 * Core domain types used across all services
 */
export type ExchangeId = 'binance' | 'bybit' | 'okx' | 'kucoin' | 'mexc' | 'bitget' | 'gateio' | 'bingx' | 'coinex' | 'lbank' | 'htx';
export interface Exchange {
    id: ExchangeId;
    name: string;
    isActive: boolean;
    config: ExchangeConfig;
}
export interface ExchangeConfig {
    wsUrl: string;
    restUrl: string;
    testnetWsUrl?: string;
    testnetRestUrl?: string;
}
export type InstrumentType = 'perpetual' | 'future' | 'spot';
export interface Instrument {
    id: string;
    exchangeId: ExchangeId;
    symbol: string;
    canonicalSymbol: string;
    baseAsset: string;
    quoteAsset: string;
    instrumentType: InstrumentType;
    contractSize: number;
    tickSize: number;
    lotSize: number;
    minNotional: number;
    makerFee: number;
    takerFee: number;
    isActive: boolean;
}
export interface PriceLevel {
    price: number;
    quantity: number;
}
export interface Orderbook {
    instrumentId: string;
    exchangeId: ExchangeId;
    symbol: string;
    bids: PriceLevel[];
    asks: PriceLevel[];
    bestBid: number;
    bestAsk: number;
    spreadBps: number;
    timestamp: number;
    sequenceId?: number;
}
export interface OrderbookUpdate {
    instrumentId: string;
    exchangeId: ExchangeId;
    symbol: string;
    bids: PriceLevel[];
    asks: PriceLevel[];
    timestamp: number;
    isSnapshot: boolean;
    sequenceId?: number;
}
export interface Spread {
    id: string;
    canonicalSymbol: string;
    longExchangeId: ExchangeId;
    longInstrumentId: string;
    longPrice: number;
    longFundingRate: number;
    shortExchangeId: ExchangeId;
    shortInstrumentId: string;
    shortPrice: number;
    shortFundingRate: number;
    spreadBps: number;
    spreadPercent: number;
    netFundingRate: number;
    longBidDepth: number;
    shortAskDepth: number;
    timestamp: number;
}
export interface SpreadTick {
    spreadId: string;
    longPrice: number;
    shortPrice: number;
    spreadBps: number;
    longFundingRate?: number;
    shortFundingRate?: number;
    timestamp: number;
}
export interface SlippageResult {
    /** Average entry/exit price after walking the book */
    avgPrice: number;
    /** Total cost/proceeds in quote currency */
    totalCost: number;
    /** Number of price levels consumed */
    levelsConsumed: number;
    /** Slippage from best price in bps */
    slippageBps: number;
    /** Total fees in quote currency */
    fees: number;
    /** Whether there was sufficient liquidity */
    hasSufficientLiquidity: boolean;
    /** If insufficient, how much could be filled */
    fillableQuantity: number;
}
export interface SpreadEntrySimulation {
    sizeInCoins: number;
    longEntry: SlippageResult;
    shortEntry: SlippageResult;
    totalFees: number;
    netSpreadBps: number;
    estimatedPnlOnExit: number;
}
export type OrderSide = 'buy' | 'sell';
export type OrderType = 'limit' | 'market';
export type OrderStatus = 'pending' | 'open' | 'partial' | 'filled' | 'cancelled' | 'rejected' | 'expired';
export type ExecutionMode = 'live' | 'sim';
export interface Order {
    id: string;
    userId: string;
    exchangeId: ExchangeId;
    instrumentId: string;
    exchangeOrderId?: string;
    clientOrderId: string;
    side: OrderSide;
    orderType: OrderType;
    price?: number;
    quantity: number;
    filledQuantity: number;
    avgFillPrice?: number;
    status: OrderStatus;
    executionMode: ExecutionMode;
    parentTradeId?: string;
    sliceIndex?: number;
    totalSlices?: number;
    errorMessage?: string;
    createdAt: Date;
    updatedAt: Date;
    filledAt?: Date;
}
export interface OrderSlice {
    index: number;
    quantity: number;
    price: number;
    status: OrderStatus;
    filledQuantity: number;
    avgFillPrice?: number;
}
export type TradeStatus = 'pending' | 'entering' | 'open' | 'exiting' | 'closed' | 'cancelled' | 'failed';
export interface Trade {
    id: string;
    userId: string;
    spreadId: string;
    executionMode: ExecutionMode;
    sizeInCoins: number;
    entryLongPrice?: number;
    entryShortPrice?: number;
    entrySpreadBps?: number;
    exitLongPrice?: number;
    exitShortPrice?: number;
    exitSpreadBps?: number;
    sliceSizeCoins?: number;
    sliceIntervalMs: number;
    totalSlices?: number;
    realizedPnl?: number;
    totalFees?: number;
    status: TradeStatus;
    isEmergencyExit: boolean;
    errorMessage?: string;
    createdAt: Date;
    updatedAt: Date;
    enteredAt?: Date;
    closedAt?: Date;
    orders?: Order[];
}
export interface TradeEnterRequest {
    spreadId: string;
    sizeInCoins: number;
    slicing: {
        sliceSizeCoins?: number;
        sliceIntervalMs?: number;
    };
    mode: ExecutionMode;
    priceToleranceBps?: number;
}
export interface TradeExitRequest {
    positionId: string;
    isEmergency: boolean;
}
export type UserRole = 'admin' | 'user';
export interface User {
    id: string;
    username: string;
    role: UserRole;
    isActive: boolean;
    expiresAt?: Date;
    createdAt: Date;
    updatedAt: Date;
}
export interface UserApiKey {
    id: string;
    userId: string;
    exchangeId: ExchangeId;
    isTestnet: boolean;
    isActive: boolean;
    label?: string;
    createdAt: Date;
}
export interface CreateUserRequest {
    username: string;
    password: string;
    expiryDays?: number;
}
export interface LoginRequest {
    username: string;
    password: string;
}
export interface LoginResponse {
    accessToken: string;
    expiresIn: number;
    user: User;
}
export interface AddApiKeyRequest {
    exchangeId: ExchangeId;
    apiKey: string;
    apiSecret: string;
    passphrase?: string;
    isTestnet: boolean;
    label?: string;
}
export type WsMessageType = 'subscribe' | 'unsubscribe' | 'orderbook' | 'spread' | 'trade_update' | 'order_update' | 'error';
export interface WsMessage<T = unknown> {
    type: WsMessageType;
    channel?: string;
    data: T;
    timestamp: number;
}
export interface WsSubscribeRequest {
    type: 'subscribe';
    channels: string[];
}
export interface WsUnsubscribeRequest {
    type: 'unsubscribe';
    channels: string[];
}
export interface ApiResponse<T> {
    success: boolean;
    data?: T;
    error?: {
        code: string;
        message: string;
    };
    timestamp: number;
}
export interface PaginatedResponse<T> {
    items: T[];
    total: number;
    page: number;
    pageSize: number;
    hasMore: boolean;
}
export interface Position {
    id: string;
    userId: string;
    spreadId: string;
    spread: Spread;
    sizeInCoins: number;
    entrySpreadBps: number;
    currentSpreadBps: number;
    unrealizedPnl: number;
    unrealizedPnlPercent: number;
    enteredAt: Date;
}
export interface FundingRate {
    exchangeId: ExchangeId;
    instrumentId: string;
    symbol: string;
    fundingRate: number;
    nextFundingTime: Date;
    fundingIntervalHours: number;
}
export interface ExchangeHealth {
    exchangeId: ExchangeId;
    wsConnected: boolean;
    lastMessageAt?: Date;
    latencyMs?: number;
    errorCount: number;
}
export interface SystemHealth {
    status: 'healthy' | 'degraded' | 'unhealthy';
    exchanges: ExchangeHealth[];
    redisConnected: boolean;
    postgresConnected: boolean;
    executionServiceConnected: boolean;
    timestamp: Date;
}
//# sourceMappingURL=index.d.ts.map