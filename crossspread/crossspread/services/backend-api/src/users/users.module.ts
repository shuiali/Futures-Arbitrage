import { Module } from '@nestjs/common';
import { UsersController } from './users.controller';
import { InternalController } from './internal.controller';
import { UsersService } from './users.service';
import { AuthModule } from '../auth/auth.module';

@Module({
  imports: [AuthModule],
  controllers: [UsersController, InternalController],
  providers: [UsersService],
  exports: [UsersService],
})
export class UsersModule {}
