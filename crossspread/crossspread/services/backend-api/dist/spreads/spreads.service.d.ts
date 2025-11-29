import { PrismaService } from '../prisma/prisma.service';
import { RedisService } from '../redis/redis.service';
export interface SpreadSummary {
    id: string;
    symbol: string;
    longExchange: string;
    shortExchange: string;
    spreadPercent: number;
    spreadBps: number;
    longPrice: number;
    shortPrice: number;
    volume24h: number;
    fundingLong: number;
    fundingShort: number;
    minDepthUsd: number;
    score: number;
    updatedAt: Date;
}
export interface OrderbookLevel {
    price: number;
    size: number;
}
export interface SlippageResult {
    entryPriceLong: number;
    entryPriceShort: number;
    exitPriceLong: number;
    exitPriceShort: number;
    entrySlippageBps: number;
    exitSlippageBps: number;
    totalFeesUsd: number;
    projectedPnlUsd: number;
    liquidityWarning: boolean;
}
export declare class SpreadsService {
    private prisma;
    private redis;
    constructor(prisma: PrismaService, redis: RedisService);
    getSpreads(token?: string, limit?: number): Promise<SpreadSummary[]>;
    getSpreadDetail(spreadId: string): Promise<{
        id: string;
        symbol: string;
        longExchange: string;
        shortExchange: string;
        longSymbol: string;
        shortSymbol: string;
        spreadPercent: number;
        spreadBps: number;
        longPrice: number;
        shortPrice: number;
        fundingLong: number;
        fundingShort: number;
        netFunding: number;
        longDepthUsd: number;
        shortDepthUsd: number;
        minDepthUsd: number;
        score: number;
        updatedAt: Date;
        longOrderbook: {
            bids: OrderbookLevel[];
            asks: OrderbookLevel[];
        };
        shortOrderbook: {
            bids: OrderbookLevel[];
            asks: OrderbookLevel[];
        };
    }>;
    getSpreadHistory(spreadId: string, from: Date, to: Date): Promise<{
        id: string;
        spreadId: string;
        spreadPercent: number;
        longPrice: number;
        shortPrice: number;
        volume: number | null;
        timestamp: Date;
    }[]>;
    calculateSlippage(spreadId: string, sizeInCoins: number): Promise<SlippageResult>;
    private getOrderbookFromCache;
    private walkBook;
    getSpreadCandles(spreadId: string, interval?: string, from?: Date, to?: Date, limit?: number): Promise<{
        time: number;
        open: number;
        high: number;
        low: number;
        close: number;
        volume: number;
    }[]>;
    private getCurrentSpreadAsCandle;
    private aggregateCandlesFromHistory;
    private getIntervalMs;
    recordSpreadTick(spreadId: string, spreadPercent: number, longPrice: number, shortPrice: number, volume?: number): Promise<void>;
}
