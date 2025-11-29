import { Module } from '@nestjs/common';
import { SpreadsController } from './spreads.controller';
import { SpreadsService } from './spreads.service';

@Module({
  controllers: [SpreadsController],
  providers: [SpreadsService],
  exports: [SpreadsService],
})
export class SpreadsModule {}
