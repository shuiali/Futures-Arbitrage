import { Injectable, OnModuleInit, OnModuleDestroy, Logger } from '@nestjs/common';
import { PrismaService } from '../prisma/prisma.service';
import { RedisService } from '../redis/redis.service';

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

/**
 * SpreadHistoryRecorder subscribes to Redis spread updates and
 * persists them to PostgreSQL for historical chart data.
 * 
 * This solves the issue of charts not showing historical data.
 */
@Injectable()
export class SpreadHistoryRecorder implements OnModuleInit, OnModuleDestroy {
  private readonly logger = new Logger(SpreadHistoryRecorder.name);
  private recordInterval: NodeJS.Timeout | null = null;
  private candleUpdateInterval: NodeJS.Timeout | null = null;
  private lastRecordedSpreads: Map<string, number> = new Map(); // spreadId -> lastRecordedTime
  private spreadBuffer: Map<string, RedisSpread> = new Map(); // Buffer spreads between intervals

  constructor(
    private readonly prisma: PrismaService,
    private readonly redis: RedisService,
  ) {}

  async onModuleInit() {
    this.logger.log('Initializing spread history recorder...');
    
    // Start recording spread ticks every 5 seconds
    this.startRecording();
    
    // Start candle aggregation every minute
    this.startCandleAggregation();
    
    // Subscribe to Redis spread updates
    await this.subscribeToSpreads();
    
    this.logger.log('Spread history recorder initialized');
  }

  async onModuleDestroy() {
    this.logger.log('Shutting down spread history recorder...');
    
    if (this.recordInterval) {
      clearInterval(this.recordInterval);
    }
    if (this.candleUpdateInterval) {
      clearInterval(this.candleUpdateInterval);
    }
  }

  /**
   * Subscribe to Redis spread updates and buffer them
   */
  private async subscribeToSpreads(): Promise<void> {
    try {
      const subscriber = this.redis.getSubscriber();
      
      await subscriber.pSubscribe('spread:*', (message, channel) => {
        try {
          const spread: RedisSpread = JSON.parse(message);
          if (spread.id) {
            this.spreadBuffer.set(spread.id, spread);
          }
        } catch (error) {
          // Ignore parse errors
        }
      });
      
      this.logger.log('Subscribed to Redis spreads for history recording');
    } catch (error) {
      this.logger.error('Failed to subscribe to spreads', error);
    }
  }

  /**
   * Record spread ticks periodically
   */
  private startRecording(): void {
    // Record every 5 seconds
    this.recordInterval = setInterval(async () => {
      await this.recordBufferedSpreads();
    }, 5000);
  }

  /**
   * Record all buffered spreads to database
   */
  private async recordBufferedSpreads(): Promise<void> {
    const now = Date.now();
    const spreadsToRecord: RedisSpread[] = [];
    
    // Also try to get from Redis spreads:list if buffer is empty
    if (this.spreadBuffer.size === 0) {
      try {
        const client = this.redis.getClient();
        const data = await client.get('spreads:list');
        if (data && typeof data === 'string') {
          const parsed = JSON.parse(data);
          for (const spread of parsed.spreads || []) {
            this.spreadBuffer.set(spread.id, spread);
          }
        }
      } catch (error) {
        // Ignore
      }
    }
    
    // Filter spreads that haven't been recorded recently (at least 5 seconds gap)
    for (const [spreadId, spread] of this.spreadBuffer.entries()) {
      const lastRecorded = this.lastRecordedSpreads.get(spreadId) || 0;
      if (now - lastRecorded >= 5000) {
        spreadsToRecord.push(spread);
        this.lastRecordedSpreads.set(spreadId, now);
      }
    }
    
    // Clear buffer after processing
    this.spreadBuffer.clear();
    
    if (spreadsToRecord.length === 0) return;
    
    // Batch insert spread history records
    try {
      const records = spreadsToRecord.map(spread => ({
        spreadId: spread.id,
        spreadPercent: spread.spread_percent,
        longPrice: spread.long_price,
        shortPrice: spread.short_price,
        volume: spread.volume_24h || 0,
        timestamp: new Date(),
      }));
      
      await this.prisma.spreadHistory.createMany({
        data: records,
        skipDuplicates: true,
      });
      
      this.logger.debug(`Recorded ${records.length} spread history entries`);
    } catch (error) {
      this.logger.error('Failed to record spread history', error);
    }
  }

  /**
   * Aggregate ticks into candles periodically
   */
  private startCandleAggregation(): void {
    // Run candle aggregation every minute
    this.candleUpdateInterval = setInterval(async () => {
      await this.aggregateCandles();
    }, 60000);
    
    // Also run once on startup after a delay
    setTimeout(() => this.aggregateCandles(), 10000);
  }

  /**
   * Aggregate recent ticks into OHLC candles
   */
  private async aggregateCandles(): Promise<void> {
    const intervals = ['1m', '5m', '15m', '1h'];
    const now = new Date();
    
    try {
      // Get unique spread IDs from recent history
      const recentHistory = await this.prisma.spreadHistory.findMany({
        where: {
          timestamp: {
            gte: new Date(now.getTime() - 24 * 60 * 60 * 1000), // Last 24 hours
          },
        },
        select: {
          spreadId: true,
        },
        distinct: ['spreadId'],
      });
      
      for (const { spreadId } of recentHistory) {
        for (const interval of intervals) {
          await this.updateCandle(spreadId, interval, now);
        }
      }
      
      this.logger.debug(`Aggregated candles for ${recentHistory.length} spreads`);
    } catch (error) {
      this.logger.error('Failed to aggregate candles', error);
    }
  }

  /**
   * Update a single candle for a spread
   */
  private async updateCandle(spreadId: string, interval: string, now: Date): Promise<void> {
    const intervalMs = this.getIntervalMs(interval);
    const openTime = new Date(Math.floor(now.getTime() / intervalMs) * intervalMs);
    const closeTime = new Date(openTime.getTime() + intervalMs);
    
    try {
      // Get ticks for this candle period
      const ticks = await this.prisma.spreadHistory.findMany({
        where: {
          spreadId,
          timestamp: {
            gte: openTime,
            lt: closeTime,
          },
        },
        orderBy: { timestamp: 'asc' },
      });
      
      if (ticks.length === 0) return;
      
      const open = ticks[0].spreadPercent;
      const close = ticks[ticks.length - 1].spreadPercent;
      const high = Math.max(...ticks.map(t => t.spreadPercent));
      const low = Math.min(...ticks.map(t => t.spreadPercent));
      const volume = ticks.reduce((sum, t) => sum + (t.volume || 0), 0);
      
      // Upsert candle
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
          open,
          high,
          low,
          close,
          volume,
          trades: ticks.length,
        },
        update: {
          high,
          low,
          close,
          volume,
          trades: ticks.length,
        },
      });
    } catch (error) {
      // Ignore individual candle errors
    }
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
}
