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
exports.TradesService = void 0;
const common_1 = require("@nestjs/common");
const prisma_service_1 = require("../prisma/prisma.service");
const redis_service_1 = require("../redis/redis.service");
const spreads_service_1 = require("../spreads/spreads.service");
let TradesService = class TradesService {
    constructor(prisma, redis, spreadsService) {
        this.prisma = prisma;
        this.redis = redis;
        this.spreadsService = spreadsService;
    }
    async enterTrade(userId, request) {
        const spread = await this.prisma.spread.findUnique({
            where: { id: request.spreadId },
            include: {
                longExchange: true,
                shortExchange: true,
                longInstrument: true,
                shortInstrument: true,
            },
        });
        if (!spread) {
            throw new common_1.NotFoundException(`Spread ${request.spreadId} not found`);
        }
        const apiKeys = await this.prisma.apiKey.findMany({
            where: {
                userId,
                exchangeId: {
                    in: [spread.longExchangeId, spread.shortExchangeId],
                },
                isActive: true,
            },
        });
        if (request.mode === 'live' && apiKeys.length < 2) {
            throw new common_1.ForbiddenException('API keys for both exchanges are required for live trading');
        }
        const slippage = await this.spreadsService.calculateSlippage(request.spreadId, request.sizeInCoins);
        const position = await this.prisma.position.create({
            data: {
                userId,
                spreadId: request.spreadId,
                sizeInCoins: request.sizeInCoins,
                targetSizeInCoins: request.sizeInCoins,
                entryPriceLong: slippage.entryPriceLong,
                entryPriceShort: slippage.entryPriceShort,
                status: 'OPENING',
                mode: request.mode,
            },
        });
        const tradeRequest = {
            positionId: position.id,
            userId,
            spreadId: request.spreadId,
            sizeInCoins: request.sizeInCoins,
            slicing: request.slicing,
            mode: request.mode,
            longExchange: spread.longExchange.name,
            shortExchange: spread.shortExchange.name,
            longSymbol: spread.longInstrument.symbol,
            shortSymbol: spread.shortInstrument.symbol,
            action: 'enter',
        };
        await this.redis.xadd('trade:requests', '*', { data: JSON.stringify(tradeRequest) });
        await this.prisma.auditLog.create({
            data: {
                userId,
                action: 'TRADE_ENTER',
                entityType: 'position',
                entityId: position.id,
                details: JSON.stringify({
                    spreadId: request.spreadId,
                    sizeInCoins: request.sizeInCoins,
                    mode: request.mode,
                    slippageEstimate: slippage,
                }),
            },
        });
        return {
            positionId: position.id,
            status: 'OPENING',
            slippageEstimate: slippage,
        };
    }
    async exitTrade(userId, positionId, mode) {
        const position = await this.prisma.position.findFirst({
            where: { id: positionId, userId },
            include: {
                spread: {
                    include: {
                        longExchange: true,
                        shortExchange: true,
                        longInstrument: true,
                        shortInstrument: true,
                    },
                },
            },
        });
        if (!position) {
            throw new common_1.NotFoundException(`Position ${positionId} not found`);
        }
        if (position.status !== 'OPEN') {
            throw new common_1.ForbiddenException('Position is not open');
        }
        await this.prisma.position.update({
            where: { id: positionId },
            data: { status: 'CLOSING' },
        });
        const exitRequest = {
            positionId,
            userId,
            spreadId: position.spreadId,
            sizeInCoins: position.sizeInCoins,
            mode: position.mode,
            exitMode: mode,
            longExchange: position.spread.longExchange.name,
            shortExchange: position.spread.shortExchange.name,
            longSymbol: position.spread.longInstrument.symbol,
            shortSymbol: position.spread.shortInstrument.symbol,
            action: 'exit',
            emergency: mode === 'emergency',
        };
        await this.redis.xadd('trade:requests', '*', { data: JSON.stringify(exitRequest) });
        await this.prisma.auditLog.create({
            data: {
                userId,
                action: mode === 'emergency' ? 'TRADE_EMERGENCY_EXIT' : 'TRADE_EXIT',
                entityType: 'position',
                entityId: positionId,
                details: JSON.stringify({ exitMode: mode }),
            },
        });
        return {
            positionId,
            status: 'CLOSING',
            emergency: mode === 'emergency',
        };
    }
    async getPositions(userId) {
        const positions = await this.prisma.position.findMany({
            where: { userId },
            include: {
                spread: true,
            },
            orderBy: { createdAt: 'desc' },
        });
        return positions.map((p) => ({
            id: p.id,
            userId: p.userId,
            spreadId: p.spreadId,
            sizeInCoins: p.sizeInCoins,
            entryPriceLong: p.entryPriceLong,
            entryPriceShort: p.entryPriceShort,
            status: p.status,
            realizedPnl: p.realizedPnl || 0,
            unrealizedPnl: p.unrealizedPnl || 0,
            createdAt: p.createdAt,
        }));
    }
    async getPosition(userId, positionId) {
        const position = await this.prisma.position.findFirst({
            where: { id: positionId, userId },
            include: {
                spread: {
                    include: {
                        longExchange: true,
                        shortExchange: true,
                    },
                },
                orders: true,
                trades: true,
            },
        });
        if (!position) {
            throw new common_1.NotFoundException(`Position ${positionId} not found`);
        }
        return position;
    }
    async getOrders(userId) {
        const orders = await this.prisma.order.findMany({
            where: { userId },
            orderBy: { createdAt: 'desc' },
            take: 100,
        });
        return orders;
    }
    async cancelOrder(userId, orderId) {
        const order = await this.prisma.order.findFirst({
            where: { id: orderId, userId },
        });
        if (!order) {
            throw new common_1.NotFoundException(`Order ${orderId} not found`);
        }
        if (order.status !== 'PENDING' && order.status !== 'PARTIAL') {
            throw new common_1.ForbiddenException('Order cannot be cancelled');
        }
        await this.redis.xadd('trade:cancel', '*', {
            data: JSON.stringify({
                orderId,
                userId,
                exchangeOrderId: order.exchangeOrderId,
                exchange: order.exchange,
                symbol: order.symbol,
            }),
        });
        await this.prisma.order.update({
            where: { id: orderId },
            data: { status: 'CANCELLING' },
        });
        return { orderId, status: 'CANCELLING' };
    }
};
exports.TradesService = TradesService;
exports.TradesService = TradesService = __decorate([
    (0, common_1.Injectable)(),
    __metadata("design:paramtypes", [prisma_service_1.PrismaService,
        redis_service_1.RedisService,
        spreads_service_1.SpreadsService])
], TradesService);
//# sourceMappingURL=trades.service.js.map