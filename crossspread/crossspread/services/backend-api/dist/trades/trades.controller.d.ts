import { TradesService, Position } from './trades.service';
import { SlippageResult } from '../spreads/spreads.service';
export declare class EnterTradeDto {
    spreadId: string;
    sizeInCoins: number;
    slicing: {
        sliceSizeInCoins: number;
        intervalMs: number;
    };
    mode: 'live' | 'sim';
}
export declare class ExitTradeDto {
    mode?: 'normal' | 'emergency';
}
export declare class TradesController {
    private tradesService;
    constructor(tradesService: TradesService);
    enterTrade(req: any, dto: EnterTradeDto): Promise<{
        positionId: string;
        status: string;
        slippageEstimate: SlippageResult;
    }>;
    exitTrade(req: any, positionId: string, dto: ExitTradeDto): Promise<{
        positionId: string;
        status: string;
        emergency: boolean;
    }>;
    getPositions(req: any): Promise<Position[]>;
    getPosition(req: any, positionId: string): Promise<{
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
    getOrders(req: any): Promise<{
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
    cancelOrder(req: any, orderId: string): Promise<{
        orderId: string;
        status: string;
    }>;
}
