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
exports.SpreadsService = void 0;
const common_1 = require("@nestjs/common");
const prisma_service_1 = require("../prisma/prisma.service");
const redis_service_1 = require("../redis/redis.service");
let SpreadsService = class SpreadsService {
    constructor(prisma, redis) {
        this.prisma = prisma;
        this.redis = redis;
    }
    async getSpreads(token, limit = 50) {
        const data = await this.redis.getClient().get('spreads:list');
        if (!data) {
            console.log('No spreads:list found in Redis');
            return [];
        }
        try {
            const parsed = JSON.parse(data);
            let spreads = parsed.spreads || [];
            if (token) {
                const upperToken = token.toUpperCase();
                spreads = spreads.filter((s) => s.canonical.toUpperCase().includes(upperToken));
            }
            spreads = spreads
                .sort((a, b) => b.score - a.score)
                .slice(0, limit);
            return spreads.map((s) => ({
                id: s.id,
                symbol: s.canonical,
                longExchange: s.long_exchange,
                shortExchange: s.short_exchange,
                spreadPercent: s.spread_percent,
                spreadBps: s.spread_bps,
                longPrice: s.long_price,
                shortPrice: s.short_price,
                volume24h: s.volume_24h || 0,
                fundingLong: s.long_funding || 0,
                fundingShort: s.short_funding || 0,
                minDepthUsd: s.min_depth_usd || 0,
                score: s.score || 0,
                updatedAt: new Date(s.updated_at),
            }));
        }
        catch (error) {
            console.error('Error parsing spreads list:', error);
            return [];
        }
    }
    async getSpreadDetail(spreadId) {
        const key = `spread:data:${spreadId}`;
        const data = await this.redis.getClient().get(key);
        if (!data) {
            throw new common_1.NotFoundException(`Spread ${spreadId} not found`);
        }
        const spread = JSON.parse(data);
        const [longOrderbook, shortOrderbook] = await Promise.all([
            this.getOrderbookFromCache(spread.long_symbol, spread.long_exchange),
            this.getOrderbookFromCache(spread.short_symbol, spread.short_exchange),
        ]);
        return {
            id: spread.id,
            symbol: spread.canonical,
            longExchange: spread.long_exchange,
            shortExchange: spread.short_exchange,
            longSymbol: spread.long_symbol,
            shortSymbol: spread.short_symbol,
            spreadPercent: spread.spread_percent,
            spreadBps: spread.spread_bps,
            longPrice: spread.long_price,
            shortPrice: spread.short_price,
            fundingLong: spread.long_funding,
            fundingShort: spread.short_funding,
            netFunding: spread.net_funding,
            longDepthUsd: spread.long_depth_usd,
            shortDepthUsd: spread.short_depth_usd,
            minDepthUsd: spread.min_depth_usd,
            score: spread.score,
            updatedAt: new Date(spread.updated_at),
            longOrderbook,
            shortOrderbook,
        };
    }
    async getSpreadHistory(spreadId, from, to) {
        const history = await this.prisma.spreadHistory.findMany({
            where: {
                spreadId,
                timestamp: {
                    gte: from,
                    lte: to,
                },
            },
            orderBy: { timestamp: 'asc' },
        });
        return history;
    }
    async calculateSlippage(spreadId, sizeInCoins) {
        const key = `spread:data:${spreadId}`;
        const data = await this.redis.getClient().get(key);
        if (!data) {
            throw new common_1.NotFoundException(`Spread ${spreadId} not found`);
        }
        const spread = JSON.parse(data);
        const [longOrderbook, shortOrderbook] = await Promise.all([
            this.getOrderbookFromCache(spread.long_symbol, spread.long_exchange),
            this.getOrderbookFromCache(spread.short_symbol, spread.short_exchange),
        ]);
        const entryLong = this.walkBook(longOrderbook.asks, sizeInCoins);
        const entryShort = this.walkBook(shortOrderbook.bids, sizeInCoins);
        const exitLong = this.walkBook(longOrderbook.bids, sizeInCoins);
        const exitShort = this.walkBook(shortOrderbook.asks, sizeInCoins);
        const longTakerFee = 0.0005;
        const shortTakerFee = 0.0005;
        const entryNotional = (entryLong.avgPrice + entryShort.avgPrice) * sizeInCoins;
        const exitNotional = (exitLong.avgPrice + exitShort.avgPrice) * sizeInCoins;
        const totalFees = (entryNotional + exitNotional) * (longTakerFee + shortTakerFee);
        const entrySpread = entryShort.avgPrice - entryLong.avgPrice;
        const exitSpread = exitShort.avgPrice - exitLong.avgPrice;
        const projectedPnl = (entrySpread - exitSpread) * sizeInCoins - totalFees;
        const midLong = (longOrderbook.bids[0]?.price + longOrderbook.asks[0]?.price) / 2 || entryLong.avgPrice;
        const midShort = (shortOrderbook.bids[0]?.price + shortOrderbook.asks[0]?.price) / 2 || entryShort.avgPrice;
        return {
            entryPriceLong: entryLong.avgPrice,
            entryPriceShort: entryShort.avgPrice,
            exitPriceLong: exitLong.avgPrice,
            exitPriceShort: exitShort.avgPrice,
            entrySlippageBps: ((entryLong.avgPrice - midLong) / midLong) * 10000,
            exitSlippageBps: ((midLong - exitLong.avgPrice) / midLong) * 10000,
            totalFeesUsd: totalFees,
            projectedPnlUsd: projectedPnl,
            liquidityWarning: !entryLong.filled || !entryShort.filled,
        };
    }
    async getOrderbookFromCache(symbol, exchange) {
        const key = `orderbook:${exchange}:${symbol}`;
        try {
            const result = await this.redis.getClient().xRevRange(key, '+', '-', { COUNT: 1 });
            if (!result || result.length === 0) {
                console.log(`No orderbook data found for ${key}`);
                return { bids: [], asks: [] };
            }
            const latestEntry = result[0];
            const dataField = latestEntry.message?.data;
            if (!dataField) {
                console.log(`No data field in orderbook stream entry for ${key}`);
                return { bids: [], asks: [] };
            }
            const orderbook = JSON.parse(dataField);
            return {
                bids: (orderbook.bids || []).map((b) => ({
                    price: b.price,
                    size: b.quantity,
                })),
                asks: (orderbook.asks || []).map((a) => ({
                    price: a.price,
                    size: a.quantity,
                })),
            };
        }
        catch (error) {
            console.error(`Error reading orderbook from stream ${key}:`, error);
            return { bids: [], asks: [] };
        }
    }
    walkBook(levels, sizeNeeded) {
        let remaining = sizeNeeded;
        let totalCost = 0;
        for (const level of levels) {
            const fill = Math.min(remaining, level.size);
            totalCost += fill * level.price;
            remaining -= fill;
            if (remaining <= 0) {
                break;
            }
        }
        const filledSize = sizeNeeded - remaining;
        const avgPrice = filledSize > 0 ? totalCost / filledSize : 0;
        return {
            avgPrice,
            totalCost,
            filled: remaining <= 0,
        };
    }
    async getSpreadCandles(spreadId, interval = '1m', from, to, limit = 500) {
        const fromDate = from || new Date(Date.now() - 24 * 60 * 60 * 1000);
        const toDate = to || new Date();
        try {
            const candles = await this.prisma.spreadCandle.findMany({
                where: {
                    spreadId,
                    interval,
                    openTime: {
                        gte: fromDate,
                        lte: toDate,
                    },
                },
                orderBy: { openTime: 'asc' },
                take: limit,
            });
            if (candles.length > 0) {
                return candles.map(c => ({
                    time: Math.floor(c.openTime.getTime() / 1000),
                    open: c.open,
                    high: c.high,
                    low: c.low,
                    close: c.close,
                    volume: c.volume,
                }));
            }
        }
        catch (error) {
            console.error('Error fetching candles from DB:', error);
        }
        const historyCandles = await this.aggregateCandlesFromHistory(spreadId, interval, fromDate, toDate, limit);
        if (historyCandles.length > 0) {
            return historyCandles;
        }
        return this.getCurrentSpreadAsCandle(spreadId, interval);
    }
    async getCurrentSpreadAsCandle(spreadId, interval) {
        const key = `spread:data:${spreadId}`;
        const data = await this.redis.getClient().get(key);
        if (!data) {
            return [];
        }
        const spread = JSON.parse(data);
        const currentSpread = spread.spread_percent;
        const intervalMs = this.getIntervalMs(interval);
        const now = Date.now();
        const candleTime = Math.floor(now / intervalMs) * intervalMs;
        return [{
                time: Math.floor(candleTime / 1000),
                open: currentSpread,
                high: currentSpread,
                low: currentSpread,
                close: currentSpread,
                volume: 0,
            }];
    }
    async aggregateCandlesFromHistory(spreadId, interval, from, to, limit) {
        const intervalMs = this.getIntervalMs(interval);
        const history = await this.prisma.spreadHistory.findMany({
            where: {
                spreadId,
                timestamp: {
                    gte: from,
                    lte: to,
                },
            },
            orderBy: { timestamp: 'asc' },
        });
        if (history.length === 0) {
            return [];
        }
        const candleMap = new Map();
        for (const h of history) {
            const candleTime = Math.floor(h.timestamp.getTime() / intervalMs) * intervalMs;
            if (!candleMap.has(candleTime)) {
                candleMap.set(candleTime, {
                    open: h.spreadPercent,
                    high: h.spreadPercent,
                    low: h.spreadPercent,
                    close: h.spreadPercent,
                    volume: h.volume || 0,
                    trades: 1,
                });
            }
            else {
                const candle = candleMap.get(candleTime);
                candle.high = Math.max(candle.high, h.spreadPercent);
                candle.low = Math.min(candle.low, h.spreadPercent);
                candle.close = h.spreadPercent;
                candle.volume += h.volume || 0;
                candle.trades += 1;
            }
        }
        const candles = Array.from(candleMap.entries())
            .map(([time, data]) => ({
            time: Math.floor(time / 1000),
            open: data.open,
            high: data.high,
            low: data.low,
            close: data.close,
            volume: data.volume,
        }))
            .sort((a, b) => a.time - b.time)
            .slice(-limit);
        return candles;
    }
    getIntervalMs(interval) {
        const intervals = {
            '1m': 60 * 1000,
            '5m': 5 * 60 * 1000,
            '15m': 15 * 60 * 1000,
            '30m': 30 * 60 * 1000,
            '1h': 60 * 60 * 1000,
            '4h': 4 * 60 * 60 * 1000,
            '1d': 24 * 60 * 60 * 1000,
        };
        return intervals[interval] || 60 * 1000;
    }
    async recordSpreadTick(spreadId, spreadPercent, longPrice, shortPrice, volume) {
        const now = new Date();
        await this.prisma.spreadHistory.create({
            data: {
                spreadId,
                spreadPercent,
                longPrice,
                shortPrice,
                volume,
                timestamp: now,
            },
        });
        const intervals = ['1m', '5m', '15m', '1h'];
        for (const interval of intervals) {
            const intervalMs = this.getIntervalMs(interval);
            const openTime = new Date(Math.floor(now.getTime() / intervalMs) * intervalMs);
            const closeTime = new Date(openTime.getTime() + intervalMs);
            await this.prisma.spreadCandle.upsert({
                where: {
                    spreadId_interval_openTime: {
                        spreadId,
                        interval,
                        openTime,
                    },
                },
                create: {
                    spreadId,
                    interval,
                    openTime,
                    closeTime,
                    open: spreadPercent,
                    high: spreadPercent,
                    low: spreadPercent,
                    close: spreadPercent,
                    volume: volume || 0,
                    trades: 1,
                },
                update: {
                    high: { set: spreadPercent },
                    low: { set: spreadPercent },
                    close: spreadPercent,
                    volume: { increment: volume || 0 },
                    trades: { increment: 1 },
                },
            });
            await this.prisma.$executeRaw `
        UPDATE spread_candles 
        SET high = GREATEST(high, ${spreadPercent}),
            low = LEAST(low, ${spreadPercent})
        WHERE spread_id = ${spreadId} 
          AND interval = ${interval} 
          AND open_time = ${openTime}
      `;
        }
    }
};
exports.SpreadsService = SpreadsService;
exports.SpreadsService = SpreadsService = __decorate([
    (0, common_1.Injectable)(),
    __metadata("design:paramtypes", [prisma_service_1.PrismaService,
        redis_service_1.RedisService])
], SpreadsService);
//# sourceMappingURL=spreads.service.js.map