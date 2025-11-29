import { Injectable, OnModuleInit } from '@nestjs/common';
import { RedisService } from '../redis/redis.service';
import { MarketGateway } from './market.gateway';
import { PrismaService } from '../prisma/prisma.service';

@Injectable()
export class MarketService implements OnModuleInit {
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
        
        // Broadcast to WebSocket clients
        this.marketGateway.broadcastSpread(spreadId, {
          spreadPercent: data.spread_percent ?? data.spreadPercent,
          longPrice: data.long_price ?? data.longPrice,
          shortPrice: data.short_price ?? data.shortPrice,
          volume24h: data.volume_24h ?? data.volume ?? 0,
          fundingLong: data.long_funding ?? data.fundingLong ?? 0,
          fundingShort: data.short_funding ?? data.fundingShort ?? 0,
        });
      } catch (error) {
        console.error('Error processing spread message:', error);
      }
    });
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
