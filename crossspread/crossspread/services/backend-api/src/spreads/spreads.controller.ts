import {
  Controller,
  Get,
  Post,
  Body,
  Param,
  Query,
  UseGuards,
  Request,
} from '@nestjs/common';
import { AuthGuard } from '@nestjs/passport';
import { SpreadsService, SpreadSummary, SlippageResult } from './spreads.service';

export class GetSpreadsDto {
  token?: string;
  limit?: number;
}

@Controller('spreads')
@UseGuards(AuthGuard('jwt'))
export class SpreadsController {
  constructor(private spreadsService: SpreadsService) {}

  @Get()
  async getSpreads(@Query() query: GetSpreadsDto): Promise<SpreadSummary[]> {
    return this.spreadsService.getSpreads(query.token, query.limit || 50);
  }

  @Get(':spreadId')
  async getSpreadDetail(@Param('spreadId') spreadId: string): Promise<any> {
    return this.spreadsService.getSpreadDetail(spreadId);
  }

  @Get(':spreadId/history')
  async getSpreadHistory(
    @Param('spreadId') spreadId: string,
    @Query('from') from?: string,
    @Query('to') to?: string,
  ) {
    const fromDate = from ? new Date(from) : new Date(Date.now() - 24 * 60 * 60 * 1000);
    const toDate = to ? new Date(to) : new Date();
    return this.spreadsService.getSpreadHistory(spreadId, fromDate, toDate);
  }

  @Get(':spreadId/candles')
  async getSpreadCandles(
    @Param('spreadId') spreadId: string,
    @Query('interval') interval?: string,
    @Query('from') from?: string,
    @Query('to') to?: string,
    @Query('limit') limit?: string,
  ) {
    const fromDate = from ? new Date(from) : undefined;
    const toDate = to ? new Date(to) : undefined;
    const candleLimit = limit ? parseInt(limit, 10) : 500;
    return this.spreadsService.getSpreadCandles(spreadId, interval || '1m', fromDate, toDate, candleLimit);
  }

  @Get(':spreadId/slippage')
  async calculateSlippage(
    @Param('spreadId') spreadId: string,
    @Query('sizeInCoins') sizeInCoins: string,
  ): Promise<SlippageResult> {
    return this.spreadsService.calculateSlippage(spreadId, parseFloat(sizeInCoins));
  }
}
