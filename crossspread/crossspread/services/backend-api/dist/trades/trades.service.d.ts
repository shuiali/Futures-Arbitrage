import { PrismaService } from '../prisma/prisma.service';
import { RedisService } from '../redis/redis.service';
import { SpreadsService, SlippageResult } from '../spreads/spreads.service';
interface SlicingConfig {
    sliceSizeInCoins: number;
    intervalMs: number;
}
export interface EnterTradeRequest {
    spreadId: string;
    sizeInCoins: number;
    slicing: SlicingConfig;
    mode: 'live' | 'sim';
}
export interface Position {
    id: string;
    userId: string;
    spreadId: string;
    sizeInCoins: number;
    entryPriceLong: number;
    entryPriceShort: number;
    status: string;
    realizedPnl: number;
    unrealizedPnl: number;
    createdAt: Date;
}
export declare class TradesService {
    private prisma;
    private redis;
    private spreadsService;
    constructor(prisma: PrismaService, redis: RedisService, spreadsService: SpreadsService);
    enterTrade(userId: string, request: EnterTradeRequest): Promise<{
        positionId: string;
        status: string;
        slippageEstimate: SlippageResult;
    }>;
    exitTrade(userId: string, positionId: string, mode: 'normal' | 'emergency'): Promise<{
        positionId: string;
        status: string;
        emergency: boolean;
    }>;
    getPositions(userId: string): Promise<Position[]>;
    getPosition(userId: string, positionId: string): Promise<{
        spread: {
            longExchange: {
                id: string;
                isActive: boolean;
                createdAt: Date;
                updatedAt: Date;
                name: string;
                displayName: string;
                takerFee: number;
                makerFee: number;
            };
            shortExchange: {
                id: string;
                isActive: boolean;
                createdAt: Date;
                updatedAt: Date;
                name: string;
                displayName: string;
                takerFee: number;
                makerFee: number;
            };
        } & {
            symbol: string;
            id: string;
            createdAt: Date;
            updatedAt: Date;
            spreadPercent: number;
            longPrice: number;
            shortPrice: number;
            longExchangeId: string;
            shortExchangeId: string;
            longInstrumentId: string;
            shortInstrumentId: string;
            volume24h: number | null;
            fundingLong: number | null;
            fundingShort: number | null;
        };
        orders: {
            symbol: string;
            exchange: string;
            id: string;
            createdAt: Date;
            updatedAt: Date;
            type: string;
            userId: string;
            status: string;
            positionId: string | null;
            exchangeOrderId: string | null;
            side: string;
            price: number;
            quantity: number;
            filledQuantity: number;
            sliceIndex: number | null;
        }[];
        trades: {
            symbol: string;
            exchange: string;
            id: string;
            createdAt: Date;
            userId: string;
            positionId: string | null;
            side: string;
            price: number;
            quantity: number;
            orderId: string | null;
            fee: number;
            feeCurrency: string | null;
        }[];
    } & {
        id: string;
        createdAt: Date;
        updatedAt: Date;
        mode: string;
        spreadId: string;
        entryPriceLong: number;
        entryPriceShort: number;
        exitPriceLong: number | null;
        exitPriceShort: number | null;
        sizeInCoins: number;
        userId: string;
        targetSizeInCoins: number;
        realizedPnl: number | null;
        unrealizedPnl: number | null;
        status: string;
        closedAt: Date | null;
    }>;
    getOrders(userId: string): Promise<{
        symbol: string;
        exchange: string;
        id: string;
        createdAt: Date;
        updatedAt: Date;
        type: string;
        userId: string;
        status: string;
        positionId: string | null;
        exchangeOrderId: string | null;
        side: string;
        price: number;
        quantity: number;
        filledQuantity: number;
        sliceIndex: number | null;
    }[]>;
    cancelOrder(userId: string, orderId: string): Promise<{
        orderId: string;
        status: string;
    }>;
}
export {};
