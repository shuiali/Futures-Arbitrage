import { Module } from '@nestjs/common';
import { JwtModule } from '@nestjs/jwt';
import { MarketGateway } from './market.gateway';
import { MarketService } from './market.service';
import { MarketController } from './market.controller';
import { RedisSubscriberService } from './redis-subscriber.service';
import { RedisModule } from '../redis/redis.module';

@Module({
  imports: [
    JwtModule.register({
      secret: process.env.JWT_SECRET || 'changeme',
    }),
    RedisModule,
  ],
  controllers: [MarketController],
  providers: [MarketGateway, MarketService, RedisSubscriberService],
  exports: [MarketGateway, MarketService],
})
export class MarketModule {}
