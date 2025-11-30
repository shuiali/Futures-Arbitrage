import {
  WebSocketGateway,
  WebSocketServer,
  SubscribeMessage,
  OnGatewayConnection,
  OnGatewayDisconnect,
  MessageBody,
  ConnectedSocket,
} from '@nestjs/websockets';
import { Server, Socket } from 'socket.io';
import { Injectable, UseGuards } from '@nestjs/common';
import { RedisService } from '../redis/redis.service';
import { JwtService } from '@nestjs/jwt';

interface SubscribeRequest {
  channel: 'orderbook' | 'spread' | 'trades';
  symbol?: string;
  spreadId?: string;
  exchanges?: string[];
}

@WebSocketGateway({
  namespace: '/ws/market',
  cors: {
    origin: process.env.CORS_ORIGIN || '*',
    credentials: true,
  },
})
@Injectable()
export class MarketGateway implements OnGatewayConnection, OnGatewayDisconnect {
  @WebSocketServer()
  server: Server;

  private clientSubscriptions: Map<string, Set<string>> = new Map();

  constructor(
    private redis: RedisService,
    private jwtService: JwtService,
  ) {}

  async handleConnection(client: Socket) {
    try {
      const token = client.handshake.auth.token || client.handshake.headers.authorization?.replace('Bearer ', '');
      
      if (!token) {
        client.disconnect();
        return;
      }

      const payload = this.jwtService.verify(token);
      (client as any).userId = payload.sub;
      
      this.clientSubscriptions.set(client.id, new Set());
      console.log(`Client connected: ${client.id}, user: ${payload.sub}`);
    } catch (error) {
      console.error('Connection auth failed:', error);
      client.disconnect();
    }
  }

  handleDisconnect(client: Socket) {
    this.clientSubscriptions.delete(client.id);
    console.log(`Client disconnected: ${client.id}`);
  }

  @SubscribeMessage('subscribe')
  async handleSubscribe(
    @ConnectedSocket() client: Socket,
    @MessageBody() data: SubscribeRequest,
  ) {
    const subscriptions = this.clientSubscriptions.get(client.id);
    if (!subscriptions) return;

    let channel: string;

    switch (data.channel) {
      case 'orderbook':
        if (data.symbol && data.exchanges) {
          for (const exchange of data.exchanges) {
            channel = `orderbook:${exchange}:${data.symbol}`;
            subscriptions.add(channel);
            client.join(channel);
          }
        }
        break;

      case 'spread':
        if (data.spreadId) {
          channel = `spread:${data.spreadId}`;
          subscriptions.add(channel);
          client.join(channel);
        }
        break;

      case 'trades':
        channel = `trades:${(client as any).userId}`;
        subscriptions.add(channel);
        client.join(channel);
        break;
    }

    return { subscribed: true, channel: data.channel };
  }

  @SubscribeMessage('unsubscribe')
  async handleUnsubscribe(
    @ConnectedSocket() client: Socket,
    @MessageBody() data: SubscribeRequest,
  ) {
    const subscriptions = this.clientSubscriptions.get(client.id);
    if (!subscriptions) return;

    let channel: string;

    switch (data.channel) {
      case 'orderbook':
        if (data.symbol && data.exchanges) {
          for (const exchange of data.exchanges) {
            channel = `orderbook:${exchange}:${data.symbol}`;
            subscriptions.delete(channel);
            client.leave(channel);
          }
        }
        break;

      case 'spread':
        if (data.spreadId) {
          channel = `spread:${data.spreadId}`;
          subscriptions.delete(channel);
          client.leave(channel);
        }
        break;

      case 'trades':
        channel = `trades:${(client as any).userId}`;
        subscriptions.delete(channel);
        client.leave(channel);
        break;
    }

    return { unsubscribed: true, channel: data.channel };
  }

  // Called by other services to broadcast updates
  broadcastOrderbook(exchange: string, symbol: string, orderbook: any) {
    const channel = `orderbook:${exchange}:${symbol}`;
    this.server.to(channel).emit('orderbook', {
      exchange,
      symbol,
      ...orderbook,
      timestamp: Date.now(),
    });
  }

  broadcastSpread(spreadId: string, spread: any) {
    const channel = `spread:${spreadId}`;
    this.server.to(channel).emit('spread', {
      spreadId,
      ...spread,
      timestamp: Date.now(),
    });
  }

  broadcastTradeUpdate(userId: string, update: any) {
    const channel = `trades:${userId}`;
    this.server.to(channel).emit('trade', update);
  }

  // Broadcast spreads summary to all connected clients
  broadcastSpreadsSummary(summary: any) {
    this.server.emit('spreads:summary', {
      ...summary,
      timestamp: Date.now(),
    });
  }

  // Broadcast to all clients watching any spread (for list updates)
  broadcastSpreadListUpdate(spreads: any[]) {
    this.server.emit('spreads:list', {
      spreads,
      timestamp: Date.now(),
    });
  }
}
