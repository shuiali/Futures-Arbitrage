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
var __param = (this && this.__param) || function (paramIndex, decorator) {
    return function (target, key) { decorator(target, key, paramIndex); }
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.MarketGateway = void 0;
const websockets_1 = require("@nestjs/websockets");
const socket_io_1 = require("socket.io");
const common_1 = require("@nestjs/common");
const redis_service_1 = require("../redis/redis.service");
const jwt_1 = require("@nestjs/jwt");
let MarketGateway = class MarketGateway {
    constructor(redis, jwtService) {
        this.redis = redis;
        this.jwtService = jwtService;
        this.clientSubscriptions = new Map();
    }
    async handleConnection(client) {
        try {
            const token = client.handshake.auth.token || client.handshake.headers.authorization?.replace('Bearer ', '');
            if (!token) {
                client.disconnect();
                return;
            }
            const payload = this.jwtService.verify(token);
            client.userId = payload.sub;
            this.clientSubscriptions.set(client.id, new Set());
            console.log(`Client connected: ${client.id}, user: ${payload.sub}`);
        }
        catch (error) {
            console.error('Connection auth failed:', error);
            client.disconnect();
        }
    }
    handleDisconnect(client) {
        this.clientSubscriptions.delete(client.id);
        console.log(`Client disconnected: ${client.id}`);
    }
    async handleSubscribe(client, data) {
        const subscriptions = this.clientSubscriptions.get(client.id);
        if (!subscriptions)
            return;
        let channel;
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
                channel = `trades:${client.userId}`;
                subscriptions.add(channel);
                client.join(channel);
                break;
        }
        return { subscribed: true, channel: data.channel };
    }
    async handleUnsubscribe(client, data) {
        const subscriptions = this.clientSubscriptions.get(client.id);
        if (!subscriptions)
            return;
        let channel;
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
                channel = `trades:${client.userId}`;
                subscriptions.delete(channel);
                client.leave(channel);
                break;
        }
        return { unsubscribed: true, channel: data.channel };
    }
    broadcastOrderbook(exchange, symbol, orderbook) {
        const channel = `orderbook:${exchange}:${symbol}`;
        this.server.to(channel).emit('orderbook', {
            exchange,
            symbol,
            ...orderbook,
            timestamp: Date.now(),
        });
    }
    broadcastSpread(spreadId, spread) {
        const channel = `spread:${spreadId}`;
        this.server.to(channel).emit('spread', {
            spreadId,
            ...spread,
            timestamp: Date.now(),
        });
    }
    broadcastTradeUpdate(userId, update) {
        const channel = `trades:${userId}`;
        this.server.to(channel).emit('trade', update);
    }
};
exports.MarketGateway = MarketGateway;
__decorate([
    (0, websockets_1.WebSocketServer)(),
    __metadata("design:type", socket_io_1.Server)
], MarketGateway.prototype, "server", void 0);
__decorate([
    (0, websockets_1.SubscribeMessage)('subscribe'),
    __param(0, (0, websockets_1.ConnectedSocket)()),
    __param(1, (0, websockets_1.MessageBody)()),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [socket_io_1.Socket, Object]),
    __metadata("design:returntype", Promise)
], MarketGateway.prototype, "handleSubscribe", null);
__decorate([
    (0, websockets_1.SubscribeMessage)('unsubscribe'),
    __param(0, (0, websockets_1.ConnectedSocket)()),
    __param(1, (0, websockets_1.MessageBody)()),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [socket_io_1.Socket, Object]),
    __metadata("design:returntype", Promise)
], MarketGateway.prototype, "handleUnsubscribe", null);
exports.MarketGateway = MarketGateway = __decorate([
    (0, websockets_1.WebSocketGateway)({
        namespace: '/ws/market',
        cors: {
            origin: process.env.CORS_ORIGIN || '*',
            credentials: true,
        },
    }),
    (0, common_1.Injectable)(),
    __metadata("design:paramtypes", [redis_service_1.RedisService,
        jwt_1.JwtService])
], MarketGateway);
//# sourceMappingURL=market.gateway.js.map