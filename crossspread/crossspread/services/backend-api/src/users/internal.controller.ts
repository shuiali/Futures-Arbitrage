import {
  Controller,
  Get,
  Param,
  Headers,
  UnauthorizedException,
} from '@nestjs/common';
import { UsersService } from './users.service';

/**
 * Internal API controller for service-to-service communication
 * Protected by SERVICE_SECRET header authentication
 */
@Controller('internal')
export class InternalController {
  private readonly serviceSecret: string;

  constructor(private usersService: UsersService) {
    this.serviceSecret = process.env.SERVICE_SECRET || 'default-dev-secret';
    if (process.env.NODE_ENV === 'production' && this.serviceSecret === 'default-dev-secret') {
      console.warn('WARNING: Using default SERVICE_SECRET in production!');
    }
  }

  private validateServiceAuth(authHeader: string | undefined): void {
    if (!authHeader) {
      throw new UnauthorizedException('Missing authorization header');
    }
    
    const [scheme, token] = authHeader.split(' ');
    if (scheme !== 'Service' || token !== this.serviceSecret) {
      throw new UnauthorizedException('Invalid service credentials');
    }
  }

  /**
   * Get decrypted API credentials for a specific exchange
   * Used by md-ingest service to get API keys for authenticated endpoints
   * 
   * Auth: Service <SERVICE_SECRET>
   */
  @Get('credentials/:exchange')
  async getExchangeCredentials(
    @Param('exchange') exchange: string,
    @Headers('authorization') authHeader: string,
  ) {
    this.validateServiceAuth(authHeader);
    return this.usersService.getDecryptedApiCredentials(exchange);
  }

  /**
   * Get all decrypted API credentials grouped by exchange
   * Used by md-ingest service to get all API keys at startup
   * 
   * Auth: Service <SERVICE_SECRET>
   */
  @Get('credentials')
  async getAllCredentials(
    @Headers('authorization') authHeader: string,
  ) {
    this.validateServiceAuth(authHeader);
    return this.usersService.getAllDecryptedApiCredentials();
  }
}
