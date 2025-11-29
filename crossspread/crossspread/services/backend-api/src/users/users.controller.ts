import {
  Controller,
  Get,
  Post,
  Put,
  Delete,
  Body,
  Param,
  Query,
  UseGuards,
  Request,
} from '@nestjs/common';
import { AuthGuard } from '@nestjs/passport';
import { IsString, IsOptional, IsNumber, IsBoolean } from 'class-validator';
import { Roles, RolesGuard } from '../auth/roles.guard';
import { UsersService } from './users.service';

export class CreateUserDto {
  @IsString()
  username: string;

  @IsString()
  password: string;

  @IsOptional()
  @IsNumber()
  expiryDays?: number;
}

export class UpdateUserDto {
  @IsOptional()
  @IsBoolean()
  isActive?: boolean;

  @IsOptional()
  @IsNumber()
  expiryDays?: number;
}

export class AddApiKeyDto {
  @IsString()
  exchangeId: string;

  @IsString()
  apiKey: string;

  @IsString()
  apiSecret: string;

  @IsOptional()
  @IsString()
  passphrase?: string;

  @IsOptional()
  @IsString()
  label?: string;
}

@Controller('')
@UseGuards(AuthGuard('jwt'))
export class UsersController {
  constructor(private usersService: UsersService) {}

  // Admin endpoints
  @Post('admin/users')
  @UseGuards(RolesGuard)
  @Roles('admin')
  async createUser(@Body() dto: CreateUserDto) {
    return this.usersService.createUser(dto);
  }

  @Get('admin/users')
  @UseGuards(RolesGuard)
  @Roles('admin')
  async listUsers(@Query('page') page = '1', @Query('limit') limit = '20') {
    return this.usersService.listUsers(parseInt(page), parseInt(limit));
  }

  @Get('admin/users/:userId')
  @UseGuards(RolesGuard)
  @Roles('admin')
  async getUser(@Param('userId') userId: string) {
    return this.usersService.getUser(userId);
  }

  @Put('admin/users/:userId')
  @UseGuards(RolesGuard)
  @Roles('admin')
  async updateUser(@Param('userId') userId: string, @Body() dto: UpdateUserDto) {
    return this.usersService.updateUser(userId, dto);
  }

  @Delete('admin/users/:userId')
  @UseGuards(RolesGuard)
  @Roles('admin')
  async deleteUser(@Param('userId') userId: string) {
    return this.usersService.deleteUser(userId);
  }

  // User endpoints for API keys
  @Get('users/:userId/positions')
  async getUserPositions(@Request() req: any, @Param('userId') userId: string) {
    // Users can only access their own positions, admins can access any
    if (req.user.role !== 'admin' && req.user.id !== userId) {
      userId = req.user.id;
    }
    return this.usersService.getUserPositions(userId);
  }

  @Post('api_keys')
  async addApiKey(@Request() req: any, @Body() dto: AddApiKeyDto) {
    return this.usersService.addApiKey(req.user.id, dto);
  }

  @Get('api_keys')
  async listApiKeys(@Request() req: any) {
    return this.usersService.listApiKeys(req.user.id);
  }

  @Delete('api_keys/:keyId')
  async deleteApiKey(@Request() req: any, @Param('keyId') keyId: string) {
    return this.usersService.deleteApiKey(req.user.id, keyId);
  }
}
