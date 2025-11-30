import { Module } from '@nestjs/common';
import { SpreadsController } from './spreads.controller';
import { SpreadsService } from './spreads.service';
import { SpreadHistoryRecorder } from './spread-history-recorder.service';
import { RedisModule } from '../redis/redis.module';

@Module({
  imports: [RedisModule],
  controllers: [SpreadsController],
  providers: [SpreadsService, SpreadHistoryRecorder],
  exports: [SpreadsService],
})
export class SpreadsModule {}
