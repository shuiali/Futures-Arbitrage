import { UsersService } from './users.service';
export declare class InternalController {
    private usersService;
    private readonly serviceSecret;
    constructor(usersService: UsersService);
    private validateServiceAuth;
    getExchangeCredentials(exchange: string, authHeader: string): Promise<{
        apiKey: string;
        apiSecret: string;
        passphrase?: string;
        userId: string;
    }[]>;
    getAllCredentials(authHeader: string): Promise<Record<string, {
        apiKey: string;
        apiSecret: string;
        passphrase?: string;
        userId: string;
    }[]>>;
}
