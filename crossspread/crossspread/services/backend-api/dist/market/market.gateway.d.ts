import { OnGatewayConnection, OnGatewayDisconnect } from '@nestjs/websockets';
import { Server, Socket } from 'socket.io';
import { RedisService } from '../redis/redis.service';
import { JwtService } from '@nestjs/jwt';
interface SubscribeRequest {
    channel: 'orderbook' | 'spread' | 'trades';
    symbol?: string;
    spreadId?: string;
    exchanges?: string[];
}
export declare class MarketGateway implements OnGatewayConnection, OnGatewayDisconnect {
    private redis;
    private jwtService;
    server: Server;
    private clientSubscriptions;
    constructor(redis: RedisService, jwtService: JwtService);
    handleConnection(client: Socket): Promise<void>;
    handleDisconnect(client: Socket): void;
    handleSubscribe(client: Socket, data: SubscribeRequest): Promise<{
        subscribed: boolean;
        channel: "spread" | "trades" | "orderbook";
    }>;
    handleUnsubscribe(client: Socket, data: SubscribeRequest): Promise<{
        unsubscribed: boolean;
        channel: "spread" | "trades" | "orderbook";
    }>;
    broadcastOrderbook(exchange: string, symbol: string, orderbook: any): void;
    broadcastSpread(spreadId: string, spread: any): void;
    broadcastTradeUpdate(userId: string, update: any): void;
}
export {};
