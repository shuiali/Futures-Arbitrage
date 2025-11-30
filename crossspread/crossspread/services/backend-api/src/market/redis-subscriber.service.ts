import { Injectable, OnModuleInit, OnModuleDestroy, Logger } from '@nestjs/common';
import { RedisService } from '../redis/redis.service';
import { MarketGateway } from './market.gateway';

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

interface RedisOrderbook {
  exchange_id: string;
  symbol: string;
  canonical: string;
  bids: Array<{ price: number; quantity: number }>;
  asks: Array<{ price: number; quantity: number }>;
  best_bid: number;
  best_ask: number;
  spread_bps: number;
  timestamp: string;
  sequence_id: number;
  is_snapshot: boolean;
}

/**
 * RedisSubscriberService subscribes to Redis pub/sub channels
 * and forwards updates to WebSocket clients via MarketGateway.
 * 
 * This bridges the gap between the Go md-ingest service (publishes to Redis)
 * and the NestJS WebSocket server (broadcasts to frontend clients).
 */
@Injectable()
export class RedisSubscriberService implements OnModuleInit, OnModuleDestroy {
  private readonly logger = new Logger(RedisSubscriberService.name);
  private isSubscribed = false;
  private orderbookPollInterval: NodeJS.Timeout | null = null;
  private spreadPollInterval: NodeJS.Timeout | null = null;

  constructor(
    private readonly redis: RedisService,
    private readonly marketGateway: MarketGateway,
  ) {}

  async onModuleInit() {
    this.logger.log('Initializing Redis subscriber service...');
    
    // Wait for Redis to be ready
    await this.waitForRedis();
    
    // Subscribe to spread updates via pub/sub
    await this.subscribeToSpreadChannels();
    
    // Start polling for orderbook updates from Redis streams
    this.startOrderbookPolling();
    
    // Start polling for spread list updates
    this.startSpreadPolling();
    
    this.isSubscribed = true;
    this.logger.log('Redis subscriber service initialized successfully');
  }

  async onModuleDestroy() {
    this.logger.log('Shutting down Redis subscriber service...');
    
    if (this.orderbookPollInterval) {
      clearInterval(this.orderbookPollInterval);
    }
    if (this.spreadPollInterval) {
      clearInterval(this.spreadPollInterval);
    }
    
    this.isSubscribed = false;
  }

  private async waitForRedis(maxAttempts = 10): Promise<void> {
    for (let i = 0; i < maxAttempts; i++) {
      try {
        const subscriber = this.redis.getSubscriber();
        if (subscriber && subscriber.isOpen) {
          return;
        }
        await new Promise(resolve => setTimeout(resolve, 1000));
      } catch (error) {
        this.logger.warn(`Waiting for Redis connection... attempt ${i + 1}/${maxAttempts}`);
        await new Promise(resolve => setTimeout(resolve, 1000));
      }
    }
    throw new Error('Failed to connect to Redis after max attempts');
  }

  /**
   * Subscribe to spread update channels via Redis Pub/Sub
   * The md-ingest service publishes to channels like:
   * - spread:{canonical} (e.g., spread:BTC)
   * - spread:{spread_id} (e.g., spread:BTC:binance:bybit)
   * - spreads:summary (aggregated top spreads)
   */
  private async subscribeToSpreadChannels(): Promise<void> {
    try {
      const subscriber = this.redis.getSubscriber();
      
      // Subscribe to pattern for all spread updates
      await subscriber.pSubscribe('spread:*', (message, channel) => {
        this.handleSpreadMessage(channel, message);
      });

      // Subscribe to summary channel
      await subscriber.subscribe('spreads:summary', (message) => {
        this.handleSpreadsSummary(message);
      });

      this.logger.log('Subscribed to Redis spread channels');
    } catch (error) {
      this.logger.error('Failed to subscribe to spread channels', error);
    }
  }

  /**
   * Handle individual spread update messages
   */
  private handleSpreadMessage(channel: string, message: string): void {
    try {
      const spread: RedisSpread = JSON.parse(message);
      
      // Extract spread ID from channel or message
      const spreadId = spread.id || channel.replace('spread:', '');
      
      // Broadcast to WebSocket clients subscribed to this spread
      this.marketGateway.broadcastSpread(spreadId, {
        spreadPercent: spread.spread_percent,
        spreadBps: spread.spread_bps,
        longPrice: spread.long_price,
        shortPrice: spread.short_price,
        fundingLong: spread.long_funding,
        fundingShort: spread.short_funding,
        volume24h: spread.volume_24h,
        minDepthUsd: spread.min_depth_usd,
      });
    } catch (error) {
      this.logger.debug(`Failed to parse spread message from ${channel}: ${error}`);
    }
  }

  /**
   * Handle spreads summary updates
   */
  private handleSpreadsSummary(message: string): void {
    try {
      const summary = JSON.parse(message);
      // Broadcast summary to all connected clients
      this.marketGateway.broadcastSpreadsSummary(summary);
    } catch (error) {
      this.logger.debug(`Failed to parse spreads summary: ${error}`);
    }
  }

  /**
   * Poll Redis streams for orderbook updates
   * The md-ingest stores orderbooks in streams like: orderbook:{exchange}:{symbol}
   */
  private startOrderbookPolling(): void {
    // Track last IDs for each stream we're reading
    const lastIds: Map<string, string> = new Map();
    
    this.orderbookPollInterval = setInterval(async () => {
      try {
        // Get list of active orderbook streams from Redis
        // Use streamClient for stream operations to avoid blocking the main client
        const streamClient = this.redis.getStreamClient();
        const client = this.redis.getClient();
        const keys = await client.keys('orderbook:*');
        
        if (keys.length === 0) return;
        
        // Read latest entries from each stream
        for (const key of keys.slice(0, 50)) { // Limit to 50 streams
          try {
            const lastId = lastIds.get(key) || '0';
            
            // Read new entries since last ID - use streamClient for xRead
            const result = await streamClient.xRead(
              [{ key, id: lastId }],
              { COUNT: 5, BLOCK: 0 }
            );
            
            if (!result || !Array.isArray(result) || result.length === 0) continue;
            
            for (const stream of result as Array<{ name: string; messages: Array<{ id: string; message: { [key: string]: string } }> }>) {
              for (const entry of stream.messages) {
                // Update last ID
                lastIds.set(key, entry.id);
                
                  // Debug log stream and entry id for troubleshooting
                  this.logger.debug(`Processing stream ${key} entry ${entry.id}`);

                // Parse and broadcast orderbook
                const data = entry.message?.data;
                if (data) {
                  const orderbook: RedisOrderbook = JSON.parse(data);
                  this.broadcastOrderbookUpdate(orderbook);
                }
              }
            }
          } catch (streamError) {
            // Stream might not exist yet, skip
          }
        }
      } catch (error) {
        this.logger.debug(`Orderbook polling error: ${error}`);
      }
    }, 100); // Poll every 100ms for real-time updates
  }

  /**
   * Broadcast orderbook update to WebSocket clients
   */
  private broadcastOrderbookUpdate(orderbook: RedisOrderbook): void {
    const exchange = orderbook.exchange_id;
    const symbol = orderbook.symbol;
    
    this.marketGateway.broadcastOrderbook(exchange, symbol, {
      bids: orderbook.bids.map(b => ({ price: b.price, size: b.quantity })),
      asks: orderbook.asks.map(a => ({ price: a.price, size: a.quantity })),
      bestBid: orderbook.best_bid,
      bestAsk: orderbook.best_ask,
      spreadBps: orderbook.spread_bps,
    });
  }

  /**
   * Poll for spread list updates and broadcast to keep UI in sync
   */
  private startSpreadPolling(): void {
    this.spreadPollInterval = setInterval(async () => {
      try {
        const client = this.redis.getClient();
        const data = await client.get('spreads:list');
        
        if (data && typeof data === 'string') {
          const parsed = JSON.parse(data);
          // Broadcast each spread update
          for (const spread of parsed.spreads?.slice(0, 20) || []) {
            this.marketGateway.broadcastSpread(spread.id, {
              spreadPercent: spread.spread_percent,
              spreadBps: spread.spread_bps,
              longPrice: spread.long_price,
              shortPrice: spread.short_price,
              fundingLong: spread.long_funding,
              fundingShort: spread.short_funding,
              volume24h: spread.volume_24h,
              minDepthUsd: spread.min_depth_usd,
            });
          }
        }
      } catch (error) {
        this.logger.debug(`Spread polling error: ${error}`);
      }
    }, 500); // Poll every 500ms
  }
}
