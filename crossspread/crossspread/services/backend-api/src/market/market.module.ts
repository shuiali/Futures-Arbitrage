import { Module } from '@nestjs/common';
import { JwtModule } from '@nestjs/jwt';
import { MarketGateway } from './market.gateway';
import { MarketService } from './market.service';
import { MarketController } from './market.controller';

@Module({
  imports: [
    JwtModule.register({
      secret: process.env.JWT_SECRET || 'changeme',
    }),
  ],
  controllers: [MarketController],
  providers: [MarketGateway, MarketService],
  exports: [MarketGateway, MarketService],
})
export class MarketModule {}
