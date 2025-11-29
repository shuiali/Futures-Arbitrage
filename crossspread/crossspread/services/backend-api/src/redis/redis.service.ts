import { Injectable, OnModuleInit, OnModuleDestroy } from '@nestjs/common';
import { createClient, RedisClientType } from 'redis';

@Injectable()
export class RedisService implements OnModuleInit, OnModuleDestroy {
  private client: RedisClientType;
  private subscriber: RedisClientType;

  async onModuleInit() {
    const host = process.env.REDIS_HOST || 'localhost';
    const port = parseInt(process.env.REDIS_PORT || '6379', 10);

    this.client = createClient({
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

  getClient(): RedisClientType {
    return this.client;
  }

  getSubscriber(): RedisClientType {
    return this.subscriber;
  }

  async publish(channel: string, message: string): Promise<number> {
    return this.client.publish(channel, message);
  }

  async xadd(stream: string, id: string, fields: Record<string, string>): Promise<string | null> {
    return this.client.xAdd(stream, id, fields);
  }

  async xread(
    streams: { key: string; id: string }[],
    options?: { COUNT?: number; BLOCK?: number },
  ) {
    return this.client.xRead(
      streams.map((s) => ({ key: s.key, id: s.id })),
      options,
    );
  }
}
