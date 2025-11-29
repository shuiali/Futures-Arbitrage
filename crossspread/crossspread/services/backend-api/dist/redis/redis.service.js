"use strict";
var __decorate = (this && this.__decorate) || function (decorators, target, key, desc) {
    var c = arguments.length, r = c < 3 ? target : desc === null ? desc = Object.getOwnPropertyDescriptor(target, key) : desc, d;
    if (typeof Reflect === "object" && typeof Reflect.decorate === "function") r = Reflect.decorate(decorators, target, key, desc);
    else for (var i = decorators.length - 1; i >= 0; i--) if (d = decorators[i]) r = (c < 3 ? d(r) : c > 3 ? d(target, key, r) : d(target, key)) || r;
    return c > 3 && r && Object.defineProperty(target, key, r), r;
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.RedisService = void 0;
const common_1 = require("@nestjs/common");
const redis_1 = require("redis");
let RedisService = class RedisService {
    async onModuleInit() {
        const host = process.env.REDIS_HOST || 'localhost';
        const port = parseInt(process.env.REDIS_PORT || '6379', 10);
        this.client = (0, redis_1.createClient)({
            url: `redis://${host}:${port}`,
        });
        this.subscriber = this.client.duplicate();
        await Promise.all([
            this.client.connect(),
            this.subscriber.connect(),
        ]);
    }
    async onModuleDestroy() {
        await Promise.all([
            this.client.disconnect(),
            this.subscriber.disconnect(),
        ]);
    }
    getClient() {
        return this.client;
    }
    getSubscriber() {
        return this.subscriber;
    }
    async publish(channel, message) {
        return this.client.publish(channel, message);
    }
    async xadd(stream, id, fields) {
        return this.client.xAdd(stream, id, fields);
    }
    async xread(streams, options) {
        return this.client.xRead(streams.map((s) => ({ key: s.key, id: s.id })), options);
    }
};
exports.RedisService = RedisService;
exports.RedisService = RedisService = __decorate([
    (0, common_1.Injectable)()
], RedisService);
//# sourceMappingURL=redis.service.js.map