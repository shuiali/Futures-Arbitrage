import {
  Controller,
  Get,
  Post,
  Body,
  Param,
  UseGuards,
  Request,
  HttpCode,
  HttpStatus,
} from '@nestjs/common';
import { AuthGuard } from '@nestjs/passport';
import { TradesService, Position } from './trades.service';
import { SlippageResult } from '../spreads/spreads.service';

export class EnterTradeDto {
  spreadId: string;
  sizeInCoins: number;
  slicing: {
    sliceSizeInCoins: number;
    intervalMs: number;
  };
  mode: 'live' | 'sim';
}

export class ExitTradeDto {
  mode?: 'normal' | 'emergency';
}

@Controller('trade')
@UseGuards(AuthGuard('jwt'))
export class TradesController {
  constructor(private tradesService: TradesService) {}

  @Post('enter')
  @HttpCode(HttpStatus.CREATED)
  async enterTrade(@Request() req: any, @Body() dto: EnterTradeDto): Promise<{ positionId: string; status: string; slippageEstimate: SlippageResult }> {
    return this.tradesService.enterTrade(req.user.id, dto);
  }

  @Post('exit/:positionId')
  @HttpCode(HttpStatus.OK)
  async exitTrade(
    @Request() req: any,
    @Param('positionId') positionId: string,
    @Body() dto: ExitTradeDto,
  ) {
    return this.tradesService.exitTrade(req.user.id, positionId, dto.mode || 'normal');
  }

  @Get('positions')
  async getPositions(@Request() req: any): Promise<Position[]> {
    return this.tradesService.getPositions(req.user.id);
  }

  @Get('positions/:positionId')
  async getPosition(@Request() req: any, @Param('positionId') positionId: string) {
    return this.tradesService.getPosition(req.user.id, positionId);
  }

  @Get('orders')
  async getOrders(@Request() req: any) {
    return this.tradesService.getOrders(req.user.id);
  }

  @Post('orders/:orderId/cancel')
  @HttpCode(HttpStatus.OK)
  async cancelOrder(@Request() req: any, @Param('orderId') orderId: string) {
    return this.tradesService.cancelOrder(req.user.id, orderId);
  }
}
