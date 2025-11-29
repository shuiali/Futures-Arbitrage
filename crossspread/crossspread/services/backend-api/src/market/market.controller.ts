import { Controller, Get, UseGuards } from '@nestjs/common';
import { AuthGuard } from '@nestjs/passport';
import { MarketService } from './market.service';

@Controller('')
@UseGuards(AuthGuard('jwt'))
export class MarketController {
  constructor(private marketService: MarketService) {}

  @Get('tokens')
  async getTokens() {
    return this.marketService.getTokens();
  }

  @Get('exchanges')
  async getExchanges() {
    return this.marketService.getExchanges();
  }
}
