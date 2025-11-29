import { OnModuleInit, OnModuleDestroy } from '@nestjs/common';
import { RedisClientType } from 'redis';
export declare class RedisService implements OnModuleInit, OnModuleDestroy {
    private client;
    private subscriber;
    onModuleInit(): Promise<void>;
    onModuleDestroy(): Promise<void>;
    getClient(): RedisClientType;
    getSubscriber(): RedisClientType;
    publish(channel: string, message: string): Promise<number>;
    xadd(stream: string, id: string, fields: Record<string, string>): Promise<string | null>;
    xread(streams: {
        key: string;
        id: string;
    }[], options?: {
        COUNT?: number;
        BLOCK?: number;
    }): Promise<{} | {
        name: string;
        messages: {
            id: string;
            message: {
                [x: string]: string;
            };
            millisElapsedFromDelivery?: number;
            deliveriesCounter?: number;
        }[];
    }[]>;
}
