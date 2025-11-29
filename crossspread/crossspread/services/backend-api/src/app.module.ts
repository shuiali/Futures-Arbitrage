import { Module } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';
import { AuthModule } from './auth/auth.module';
import { SpreadsModule } from './spreads/spreads.module';
import { TradesModule } from './trades/trades.module';
import { UsersModule } from './users/users.module';
import { MarketModule } from './market/market.module';
import { PrismaModule } from './prisma/prisma.module';
import { RedisModule } from './redis/redis.module';
import { AssetsModule } from './assets/assets.module';

@Module({
  imports: [
    ConfigModule.forRoot({
      isGlobal: true,
    }),
    PrismaModule,
    RedisModule,
    AuthModule,
    UsersModule,
    SpreadsModule,
    TradesModule,
    MarketModule,
    AssetsModule,
  ],
})
export class AppModule {}
