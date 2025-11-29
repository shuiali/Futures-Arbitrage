import { UsersService } from './users.service';
export declare class CreateUserDto {
    username: string;
    password: string;
    expiryDays?: number;
}
export declare class UpdateUserDto {
    isActive?: boolean;
    expiryDays?: number;
}
export declare class AddApiKeyDto {
    exchangeId: string;
    apiKey: string;
    apiSecret: string;
    passphrase?: string;
    label?: string;
}
export declare class UsersController {
    private usersService;
    constructor(usersService: UsersService);
    createUser(dto: CreateUserDto): Promise<{
        id: string;
        username: string;
        role: string;
        isActive: boolean;
        expiresAt: Date;
        createdAt: Date;
    }>;
    listUsers(page?: string, limit?: string): Promise<{
        users: {
            id: string;
            username: string;
            role: string;
            isActive: boolean;
            expiresAt: Date;
            createdAt: Date;
        }[];
        pagination: {
            page: number;
            limit: number;
            total: number;
            totalPages: number;
        };
    }>;
    getUser(userId: string): Promise<{
        id: string;
        username: string;
        role: string;
        isActive: boolean;
        expiresAt: Date;
        createdAt: Date;
        updatedAt: Date;
        _count: {
            apiKeys: number;
            positions: number;
        };
    }>;
    updateUser(userId: string, dto: UpdateUserDto): Promise<{
        id: string;
        username: string;
        role: string;
        isActive: boolean;
        expiresAt: Date;
    }>;
    deleteUser(userId: string): Promise<{
        deleted: boolean;
    }>;
    getUserPositions(req: any, userId: string): Promise<({
        spread: {
            symbol: string;
            id: string;
            createdAt: Date;
            updatedAt: Date;
            spreadPercent: number;
            longPrice: number;
            shortPrice: number;
            longExchangeId: string;
            shortExchangeId: string;
            longInstrumentId: string;
            shortInstrumentId: string;
            volume24h: number | null;
            fundingLong: number | null;
            fundingShort: number | null;
        };
    } & {
        id: string;
        createdAt: Date;
        updatedAt: Date;
        mode: string;
        spreadId: string;
        entryPriceLong: number;
        entryPriceShort: number;
        exitPriceLong: number | null;
        exitPriceShort: number | null;
        sizeInCoins: number;
        userId: string;
        targetSizeInCoins: number;
        realizedPnl: number | null;
        unrealizedPnl: number | null;
        status: string;
        closedAt: Date | null;
    })[]>;
    addApiKey(req: any, dto: AddApiKeyDto): Promise<{
        id: string;
        exchangeId: string;
        label: string;
        isActive: boolean;
        createdAt: Date;
    }>;
    listApiKeys(req: any): Promise<{
        id: any;
        exchangeId: any;
        exchangeName: any;
        label: any;
        isActive: any;
        createdAt: any;
    }[]>;
    deleteApiKey(req: any, keyId: string): Promise<{
        deleted: boolean;
    }>;
}
