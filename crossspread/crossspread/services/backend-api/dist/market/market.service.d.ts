import { OnModuleInit } from '@nestjs/common';
import { RedisService } from '../redis/redis.service';
import { MarketGateway } from './market.gateway';
import { PrismaService } from '../prisma/prisma.service';
export declare class MarketService implements OnModuleInit {
    private redis;
    private marketGateway;
    private prisma;
    constructor(redis: RedisService, marketGateway: MarketGateway, prisma: PrismaService);
    onModuleInit(): Promise<void>;
    private startOrderbookListener;
    private startSpreadListener;
    private startTradeListener;
    getTokens(): Promise<any[]>;
    getExchanges(): Promise<{
        id: string;
        isActive: boolean;
        name: string;
        displayName: string;
        takerFee: number;
        makerFee: number;
    }[]>;
}
