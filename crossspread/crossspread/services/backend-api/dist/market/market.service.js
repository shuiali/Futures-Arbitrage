"use strict";
var __decorate = (this && this.__decorate) || function (decorators, target, key, desc) {
    var c = arguments.length, r = c < 3 ? target : desc === null ? desc = Object.getOwnPropertyDescriptor(target, key) : desc, d;
    if (typeof Reflect === "object" && typeof Reflect.decorate === "function") r = Reflect.decorate(decorators, target, key, desc);
    else for (var i = decorators.length - 1; i >= 0; i--) if (d = decorators[i]) r = (c < 3 ? d(r) : c > 3 ? d(target, key, r) : d(target, key)) || r;
    return c > 3 && r && Object.defineProperty(target, key, r), r;
};
var __metadata = (this && this.__metadata) || function (k, v) {
    if (typeof Reflect === "object" && typeof Reflect.metadata === "function") return Reflect.metadata(k, v);
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.MarketService = void 0;
const common_1 = require("@nestjs/common");
const redis_service_1 = require("../redis/redis.service");
const market_gateway_1 = require("./market.gateway");
const prisma_service_1 = require("../prisma/prisma.service");
let MarketService = class MarketService {
    constructor(redis, marketGateway, prisma) {
        this.redis = redis;
        this.marketGateway = marketGateway;
        this.prisma = prisma;
    }
    async onModuleInit() {
        this.startOrderbookListener();
        this.startSpreadListener();
        this.startTradeListener();
    }
    async startOrderbookListener() {
        const subscriber = this.redis.getSubscriber();
        await subscriber.pSubscribe('orderbook:*', (message, channel) => {
            try {
                const data = JSON.parse(message);
                const parts = channel.split(':');
                const exchange = parts[1];
                const symbol = parts[2];
                this.marketGateway.broadcastOrderbook(exchange, symbol, data);
            }
            catch (error) {
                console.error('Error processing orderbook message:', error);
            }
        });
    }
    async startSpreadListener() {
        const subscriber = this.redis.getSubscriber();
        await subscriber.pSubscribe('spread:*', (message, channel) => {
            try {
                const data = JSON.parse(message);
                const spreadId = channel.split(':').slice(1).join(':');
                this.marketGateway.broadcastSpread(spreadId, {
                    spreadPercent: data.spread_percent ?? data.spreadPercent,
                    longPrice: data.long_price ?? data.longPrice,
                    shortPrice: data.short_price ?? data.shortPrice,
                    volume24h: data.volume_24h ?? data.volume ?? 0,
                    fundingLong: data.long_funding ?? data.fundingLong ?? 0,
                    fundingShort: data.short_funding ?? data.fundingShort ?? 0,
                });
            }
            catch (error) {
                console.error('Error processing spread message:', error);
            }
        });
    }
    async startTradeListener() {
        const subscriber = this.redis.getSubscriber();
        await subscriber.pSubscribe('trade:updates:*', (message, channel) => {
            try {
                const data = JSON.parse(message);
                const userId = channel.split(':')[2];
                this.marketGateway.broadcastTradeUpdate(userId, data);
            }
            catch (error) {
                console.error('Error processing trade message:', error);
            }
        });
    }
    async getTokens() {
        const instruments = await this.prisma.instrument.findMany({
            select: {
                symbol: true,
                baseAsset: true,
                quoteAsset: true,
                exchange: {
                    select: { name: true },
                },
            },
            distinct: ['baseAsset'],
        });
        const tokenMap = new Map();
        for (const inst of instruments) {
            if (!tokenMap.has(inst.baseAsset)) {
                tokenMap.set(inst.baseAsset, {
                    symbol: inst.baseAsset,
                    exchanges: [],
                });
            }
            tokenMap.get(inst.baseAsset).exchanges.push(inst.exchange.name);
        }
        return Array.from(tokenMap.values());
    }
    async getExchanges() {
        const exchanges = await this.prisma.exchange.findMany({
            where: { isActive: true },
            select: {
                id: true,
                name: true,
                displayName: true,
                takerFee: true,
                makerFee: true,
                isActive: true,
            },
        });
        return exchanges;
    }
};
exports.MarketService = MarketService;
exports.MarketService = MarketService = __decorate([
    (0, common_1.Injectable)(),
    __metadata("design:paramtypes", [redis_service_1.RedisService,
        market_gateway_1.MarketGateway,
        prisma_service_1.PrismaService])
], MarketService);
//# sourceMappingURL=market.service.js.map