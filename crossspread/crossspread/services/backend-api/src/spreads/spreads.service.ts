import { Injectable, NotFoundException } from '@nestjs/common';
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

interface RedisSpread {
  id: string;
  canonical: string;
  long_exchange: string;
  short_exchange: string;
  long_symbol: string;
  short_symbol: string;
  long_price: number;
  short_price: number;
  spread_percent: number;
  spread_bps: number;
  long_funding: number;
  short_funding: number;
  net_funding: number;
  long_depth_usd: number;
  short_depth_usd: number;
  min_depth_usd: number;
  volume_24h: number;
  score: number;
  updated_at: string;
}

interface SpreadsList {
  timestamp: string;
  count: number;
  spreads: RedisSpread[];
}

@Injectable()
export class SpreadsService {
  constructor(
    private prisma: PrismaService,
    private redis: RedisService,
  ) {}

  async getSpreads(token?: string, limit: number = 50): Promise<SpreadSummary[]> {
    // Read spreads list from Redis (populated by md-ingest)
    const data = await this.redis.getClient().get('spreads:list');
    
    if (!data) {
      console.log('No spreads:list found in Redis');
      return [];
    }

    try {
      const parsed: SpreadsList = JSON.parse(data as string);
      let spreads = parsed.spreads || [];
      
      // Filter by token if provided
      if (token) {
        const upperToken = token.toUpperCase();
        spreads = spreads.filter((s: RedisSpread) => 
          s.canonical.toUpperCase().includes(upperToken)
        );
      }
      
      // Sort by score descending and limit
      spreads = spreads
        .sort((a: RedisSpread, b: RedisSpread) => b.score - a.score)
        .slice(0, limit);
      
      return spreads.map((s: RedisSpread) => ({
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
    } catch (error) {
      console.error('Error parsing spreads list:', error);
      return [];
    }
  }

  async getSpreadDetail(spreadId: string) {
    // Get spread data from Redis
    const key = `spread:data:${spreadId}`;
    const data = await this.redis.getClient().get(key);
    
    if (!data) {
      throw new NotFoundException(`Spread ${spreadId} not found`);
    }

    const spread: RedisSpread = JSON.parse(data as string);

    // Get current orderbooks from Redis cache
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

  async getSpreadHistory(spreadId: string, from: Date, to: Date) {
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

  async calculateSlippage(spreadId: string, sizeInCoins: number): Promise<SlippageResult> {
    // Get spread data from Redis
    const key = `spread:data:${spreadId}`;
    const data = await this.redis.getClient().get(key);
    
    if (!data) {
      throw new NotFoundException(`Spread ${spreadId} not found`);
    }

    const spread: RedisSpread = JSON.parse(data as string);

    // Get orderbooks from cache
    const [longOrderbook, shortOrderbook] = await Promise.all([
      this.getOrderbookFromCache(spread.long_symbol, spread.long_exchange),
      this.getOrderbookFromCache(spread.short_symbol, spread.short_exchange),
    ]);

    // Walk the book for entry (long: buy asks, short: sell bids)
    const entryLong = this.walkBook(longOrderbook.asks, sizeInCoins);
    const entryShort = this.walkBook(shortOrderbook.bids, sizeInCoins);

    // Walk the book for exit (long: sell bids, short: buy asks)
    const exitLong = this.walkBook(longOrderbook.bids, sizeInCoins);
    const exitShort = this.walkBook(shortOrderbook.asks, sizeInCoins);

    // Default taker fees
    const longTakerFee = 0.0005;
    const shortTakerFee = 0.0005;

    const entryNotional = (entryLong.avgPrice + entryShort.avgPrice) * sizeInCoins;
    const exitNotional = (exitLong.avgPrice + exitShort.avgPrice) * sizeInCoins;
    const totalFees = (entryNotional + exitNotional) * (longTakerFee + shortTakerFee);

    // Calculate PnL
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

  private async getOrderbookFromCache(
    symbol: string,
    exchange: string,
  ): Promise<{ bids: OrderbookLevel[]; asks: OrderbookLevel[] }> {
    const key = `orderbook:${exchange}:${symbol}`;
    
    try {
      // Orderbooks are stored as Redis Streams, get the latest entry
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
      
      // Transform to OrderbookLevel format
      // The md-ingest stores bids/asks as objects with {price, quantity}
      return {
        bids: (orderbook.bids || []).map((b: { price: number; quantity: number }) => ({
          price: b.price,
          size: b.quantity,
        })),
        asks: (orderbook.asks || []).map((a: { price: number; quantity: number }) => ({
          price: a.price,
          size: a.quantity,
        })),
      };
    } catch (error) {
      console.error(`Error reading orderbook from stream ${key}:`, error);
      return { bids: [], asks: [] };
    }
  }

  private walkBook(
    levels: OrderbookLevel[],
    sizeNeeded: number,
  ): { avgPrice: number; totalCost: number; filled: boolean } {
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

  // ========================================
  // OHLC Candlestick Methods
  // ========================================

  async getSpreadCandles(
    spreadId: string,
    interval: string = '1m',
    from?: Date,
    to?: Date,
    limit: number = 500,
  ) {
    const fromDate = from || new Date(Date.now() - 24 * 60 * 60 * 1000); // Default: last 24h
    const toDate = to || new Date();

    // First try to get from candles table
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

      // If we have candles, return them
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
    } catch (error) {
      console.error('Error fetching candles from DB:', error);
    }

    // Try to get history data
    const historyCandles = await this.aggregateCandlesFromHistory(spreadId, interval, fromDate, toDate, limit);
    if (historyCandles.length > 0) {
      return historyCandles;
    }

    // No historical data - return single point with current spread
    return this.getCurrentSpreadAsCandle(spreadId, interval);
  }

  // Return current spread as a single candle point (for real-time chart)
  private async getCurrentSpreadAsCandle(
    spreadId: string,
    interval: string,
  ) {
    // Get current spread data from Redis
    const key = `spread:data:${spreadId}`;
    const data = await this.redis.getClient().get(key);
    
    if (!data) {
      return [];
    }

    const spread: RedisSpread = JSON.parse(data as string);
    const currentSpread = spread.spread_percent;
    const intervalMs = this.getIntervalMs(interval);
    const now = Date.now();
    const candleTime = Math.floor(now / intervalMs) * intervalMs;
    
    // Return just the current candle - chart will build up over time
    return [{
      time: Math.floor(candleTime / 1000),
      open: currentSpread,
      high: currentSpread,
      low: currentSpread,
      close: currentSpread,
      volume: 0,
    }];
  }

  private async aggregateCandlesFromHistory(
    spreadId: string,
    interval: string,
    from: Date,
    to: Date,
    limit: number,
  ) {
    // Get interval in milliseconds
    const intervalMs = this.getIntervalMs(interval);
    
    // Get raw history
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

    // Group into candles
    const candleMap = new Map<number, {
      open: number;
      high: number;
      low: number;
      close: number;
      volume: number;
      trades: number;
    }>();

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
      } else {
        const candle = candleMap.get(candleTime)!;
        candle.high = Math.max(candle.high, h.spreadPercent);
        candle.low = Math.min(candle.low, h.spreadPercent);
        candle.close = h.spreadPercent;
        candle.volume += h.volume || 0;
        candle.trades += 1;
      }
    }

    // Convert to array and sort
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

  private getIntervalMs(interval: string): number {
    const intervals: Record<string, number> = {
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

  // Store a new spread tick and update candle
  async recordSpreadTick(
    spreadId: string,
    spreadPercent: number,
    longPrice: number,
    shortPrice: number,
    volume?: number,
  ) {
    const now = new Date();
    
    // Save to history
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

    // Update 1m candle (for real-time updates)
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
          high: { set: spreadPercent }, // This will be handled with raw query
          low: { set: spreadPercent },
          close: spreadPercent,
          volume: { increment: volume || 0 },
          trades: { increment: 1 },
        },
      });

      // Use raw query to properly update high/low
      await this.prisma.$executeRaw`
        UPDATE spread_candles 
        SET high = GREATEST(high, ${spreadPercent}),
            low = LEAST(low, ${spreadPercent})
        WHERE spread_id = ${spreadId} 
          AND interval = ${interval} 
          AND open_time = ${openTime}
      `;
    }
  }
}
