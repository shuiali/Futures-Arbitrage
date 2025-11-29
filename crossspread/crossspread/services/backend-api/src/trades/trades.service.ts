import { Injectable, NotFoundException, ForbiddenException } from '@nestjs/common';
import { PrismaService } from '../prisma/prisma.service';
import { RedisService } from '../redis/redis.service';
import { SpreadsService, SlippageResult } from '../spreads/spreads.service';

interface SlicingConfig {
  sliceSizeInCoins: number;
  intervalMs: number;
}

export interface EnterTradeRequest {
  spreadId: string;
  sizeInCoins: number;
  slicing: SlicingConfig;
  mode: 'live' | 'sim';
}

export interface Position {
  id: string;
  userId: string;
  spreadId: string;
  sizeInCoins: number;
  entryPriceLong: number;
  entryPriceShort: number;
  status: string;
  realizedPnl: number;
  unrealizedPnl: number;
  createdAt: Date;
}

@Injectable()
export class TradesService {
  constructor(
    private prisma: PrismaService,
    private redis: RedisService,
    private spreadsService: SpreadsService,
  ) {}

  async enterTrade(userId: string, request: EnterTradeRequest) {
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
      throw new NotFoundException(`Spread ${request.spreadId} not found`);
    }

    // Get user API keys for both exchanges
    const apiKeys = await this.prisma.apiKey.findMany({
      where: {
        userId,
        exchangeId: {
          in: [(spread as any).longExchangeId, (spread as any).shortExchangeId],
        },
        isActive: true,
      },
    });

    if (request.mode === 'live' && apiKeys.length < 2) {
      throw new ForbiddenException('API keys for both exchanges are required for live trading');
    }

    // Calculate slippage estimate
    const slippage = await this.spreadsService.calculateSlippage(
      request.spreadId,
      request.sizeInCoins,
    );

    // Create position record
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

    // Create trade order request
    const tradeRequest = {
      positionId: position.id,
      userId,
      spreadId: request.spreadId,
      sizeInCoins: request.sizeInCoins,
      slicing: request.slicing,
      mode: request.mode,
      longExchange: (spread as any).longExchange.name,
      shortExchange: (spread as any).shortExchange.name,
      longSymbol: (spread as any).longInstrument.symbol,
      shortSymbol: (spread as any).shortInstrument.symbol,
      action: 'enter',
    };

    // Publish to Redis stream for execution service
    await this.redis.xadd(
      'trade:requests',
      '*',
      { data: JSON.stringify(tradeRequest) },
    );

    // Create audit log
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

  async exitTrade(userId: string, positionId: string, mode: 'normal' | 'emergency') {
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
      throw new NotFoundException(`Position ${positionId} not found`);
    }

    if ((position as any).status !== 'OPEN') {
      throw new ForbiddenException('Position is not open');
    }

    // Update position status
    await this.prisma.position.update({
      where: { id: positionId },
      data: { status: 'CLOSING' },
    });

    // Create exit request
    const exitRequest = {
      positionId,
      userId,
      spreadId: (position as any).spreadId,
      sizeInCoins: (position as any).sizeInCoins,
      mode: (position as any).mode,
      exitMode: mode,
      longExchange: (position as any).spread.longExchange.name,
      shortExchange: (position as any).spread.shortExchange.name,
      longSymbol: (position as any).spread.longInstrument.symbol,
      shortSymbol: (position as any).spread.shortInstrument.symbol,
      action: 'exit',
      emergency: mode === 'emergency',
    };

    await this.redis.xadd(
      'trade:requests',
      '*',
      { data: JSON.stringify(exitRequest) },
    );

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

  async getPositions(userId: string): Promise<Position[]> {
    const positions = await this.prisma.position.findMany({
      where: { userId },
      include: {
        spread: true,
      },
      orderBy: { createdAt: 'desc' },
    });

    return positions.map((p: any) => ({
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

  async getPosition(userId: string, positionId: string) {
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
      throw new NotFoundException(`Position ${positionId} not found`);
    }

    return position;
  }

  async getOrders(userId: string) {
    const orders = await this.prisma.order.findMany({
      where: { userId },
      orderBy: { createdAt: 'desc' },
      take: 100,
    });

    return orders;
  }

  async cancelOrder(userId: string, orderId: string) {
    const order = await this.prisma.order.findFirst({
      where: { id: orderId, userId },
    });

    if (!order) {
      throw new NotFoundException(`Order ${orderId} not found`);
    }

    if ((order as any).status !== 'PENDING' && (order as any).status !== 'PARTIAL') {
      throw new ForbiddenException('Order cannot be cancelled');
    }

    // Send cancel request to execution service
    await this.redis.xadd(
      'trade:cancel',
      '*',
      {
        data: JSON.stringify({
          orderId,
          userId,
          exchangeOrderId: (order as any).exchangeOrderId,
          exchange: (order as any).exchange,
          symbol: (order as any).symbol,
        }),
      },
    );

    await this.prisma.order.update({
      where: { id: orderId },
      data: { status: 'CANCELLING' },
    });

    return { orderId, status: 'CANCELLING' };
  }
}
