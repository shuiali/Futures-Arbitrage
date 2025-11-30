import { Injectable, OnModuleInit } from '@nestjs/common';
import { RedisService } from '../redis/redis.service';
import { MarketGateway } from './market.gateway';
import { PrismaService } from '../prisma/prisma.service';

@Injectable()
export class MarketService implements OnModuleInit {
  private spreadHistoryBuffer: Map<string, { lastRecorded: number; data: any }> = new Map();
  private readonly HISTORY_RECORD_INTERVAL_MS = 5000; // Record every 5 seconds

  constructor(
    private redis: RedisService,
    private marketGateway: MarketGateway,
    private prisma: PrismaService,
  ) {}

  async onModuleInit() {
    // Start listening to Redis streams for market data
    this.startOrderbookListener();
    this.startSpreadListener();
    this.startTradeListener();
  }

  private async startOrderbookListener() {
    const subscriber = this.redis.getSubscriber();
    
    // Subscribe to orderbook updates pattern
    await subscriber.pSubscribe('orderbook:*', (message, channel) => {
      try {
        const data = JSON.parse(message);
        const parts = channel.split(':');
        const exchange = parts[1];
        const symbol = parts[2];
        
        this.marketGateway.broadcastOrderbook(exchange, symbol, data);
      } catch (error) {
        console.error('Error processing orderbook message:', error);
      }
    });
  }

  private async startSpreadListener() {
    const subscriber = this.redis.getSubscriber();
    
    await subscriber.pSubscribe('spread:*', (message, channel) => {
      try {
        const data = JSON.parse(message);
        const spreadId = channel.split(':').slice(1).join(':'); // Handle spread:BTC:okx:binance format
        
        // Skip summary channels
        if (spreadId === 'summary' || spreadId.startsWith('data:')) {
          return;
        }
        
        const spreadPercent = data.spread_percent ?? data.spreadPercent ?? 0;
        const longPrice = data.long_price ?? data.longPrice ?? 0;
        const shortPrice = data.short_price ?? data.shortPrice ?? 0;
        const volume24h = data.volume_24h ?? data.volume ?? 0;
        
        // Broadcast to WebSocket clients
        this.marketGateway.broadcastSpread(spreadId, {
          spreadPercent,
          longPrice,
          shortPrice,
          volume24h,
          fundingLong: data.long_funding ?? data.fundingLong ?? 0,
          fundingShort: data.short_funding ?? data.fundingShort ?? 0,
        });
        
        // Record to history (throttled)
        this.recordSpreadHistoryThrottled(spreadId, spreadPercent, longPrice, shortPrice, volume24h);
      } catch (error) {
        console.error('Error processing spread message:', error);
      }
    });
  }

  private async recordSpreadHistoryThrottled(
    spreadId: string,
    spreadPercent: number,
    longPrice: number,
    shortPrice: number,
    volume24h: number,
  ) {
    const now = Date.now();
    const lastEntry = this.spreadHistoryBuffer.get(spreadId);
    
    // Only record if enough time has passed
    if (lastEntry && (now - lastEntry.lastRecorded) < this.HISTORY_RECORD_INTERVAL_MS) {
      return;
    }
    
    // Update buffer
    this.spreadHistoryBuffer.set(spreadId, { 
      lastRecorded: now, 
      data: { spreadPercent, longPrice, shortPrice, volume24h } 
    });
    
    // Record to database
    try {
      await this.prisma.spreadHistory.create({
        data: {
          spreadId,
          spreadPercent,
          longPrice,
          shortPrice,
          volume: volume24h,
          timestamp: new Date(),
        },
      });
      
      // Update or create candles
      await this.updateCandles(spreadId, spreadPercent, volume24h);
    } catch (error) {
      // Ignore foreign key errors for unknown spreads
      if (!error.message?.includes('Foreign key constraint')) {
        console.error('Error recording spread history:', error);
      }
    }
  }

  private async updateCandles(spreadId: string, spreadPercent: number, volume: number) {
    const now = new Date();
    const intervals = ['1m', '5m', '15m', '1h'];
    const intervalMs: Record<string, number> = {
      '1m': 60 * 1000,
      '5m': 5 * 60 * 1000,
      '15m': 15 * 60 * 1000,
      '1h': 60 * 60 * 1000,
    };
    
    for (const interval of intervals) {
      const ms = intervalMs[interval];
      const openTime = new Date(Math.floor(now.getTime() / ms) * ms);
      const closeTime = new Date(openTime.getTime() + ms);
      
      try {
        // Try to get existing candle
        const existing = await this.prisma.spreadCandle.findUnique({
          where: {
            spreadId_interval_openTime: {
              spreadId,
              interval,
              openTime,
            },
          },
        });
        
        if (existing) {
          // Update existing candle
          await this.prisma.spreadCandle.update({
            where: {
              spreadId_interval_openTime: {
                spreadId,
                interval,
                openTime,
              },
            },
            data: {
              high: Math.max(existing.high, spreadPercent),
              low: Math.min(existing.low, spreadPercent),
              close: spreadPercent,
              trades: { increment: 1 },
            },
          });
        } else {
          // Create new candle
          await this.prisma.spreadCandle.create({
            data: {
              spreadId,
              interval,
              openTime,
              closeTime,
              open: spreadPercent,
              high: spreadPercent,
              low: spreadPercent,
              close: spreadPercent,
              volume: 0,
              trades: 1,
            },
          });
        }
      } catch (error) {
        // Ignore errors, candle updates are best-effort
      }
    }
  }

  private async startTradeListener() {
    const subscriber = this.redis.getSubscriber();
    
    await subscriber.pSubscribe('trade:updates:*', (message, channel) => {
      try {
        const data = JSON.parse(message);
        const userId = channel.split(':')[2];
        
        this.marketGateway.broadcastTradeUpdate(userId, data);
      } catch (error) {
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

    // Group by base asset
    const tokenMap = new Map<string, any>();
    
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
}
