import { PrismaService } from '../prisma/prisma.service';
import { AuthService } from '../auth/auth.service';
interface CreateUserRequest {
    username: string;
    password: string;
    expiryDays?: number;
}
interface UpdateUserRequest {
    isActive?: boolean;
    expiryDays?: number;
}
interface AddApiKeyRequest {
    exchangeId: string;
    apiKey: string;
    apiSecret: string;
    passphrase?: string;
    label?: string;
}
export declare class UsersService {
    private prisma;
    private authService;
    private readonly encryptionKey;
    constructor(prisma: PrismaService, authService: AuthService);
    createUser(request: CreateUserRequest): Promise<{
        id: string;
        username: string;
        role: string;
        isActive: boolean;
        expiresAt: Date;
        createdAt: Date;
    }>;
    listUsers(page: number, limit: number): Promise<{
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
    updateUser(userId: string, request: UpdateUserRequest): Promise<{
        id: string;
        username: string;
        role: string;
        isActive: boolean;
        expiresAt: Date;
    }>;
    deleteUser(userId: string): Promise<{
        deleted: boolean;
    }>;
    getUserPositions(userId: string): Promise<({
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
    addApiKey(userId: string, request: AddApiKeyRequest): Promise<{
        id: string;
        exchangeId: string;
        label: string;
        isActive: boolean;
        createdAt: Date;
    }>;
    listApiKeys(userId: string): Promise<{
        id: any;
        exchangeId: any;
        exchangeName: any;
        label: any;
        isActive: any;
        createdAt: any;
    }[]>;
    deleteApiKey(userId: string, keyId: string): Promise<{
        deleted: boolean;
    }>;
    private encrypt;
    private decrypt;
    getDecryptedApiCredentials(exchangeName: string): Promise<{
        apiKey: string;
        apiSecret: string;
        passphrase?: string;
        userId: string;
    }[]>;
    getAllDecryptedApiCredentials(): Promise<Record<string, {
        apiKey: string;
        apiSecret: string;
        passphrase?: string;
        userId: string;
    }[]>>;
}
export {};
