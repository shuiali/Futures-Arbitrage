import { RedisService } from '../redis/redis.service';
export interface AssetInfo {
    exchangeId: string;
    asset: string;
    depositEnabled: boolean;
    withdrawEnabled: boolean;
    withdrawFee?: number;
    minWithdraw?: number;
    networks?: string[];
    timestamp: Date;
}
export interface ExchangeAssetInfo {
    exchange: string;
    assets: AssetInfo[];
    lastUpdated: Date;
}
export declare class AssetsService {
    private redis;
    private readonly logger;
    constructor(redis: RedisService);
    getAssetInfo(exchange: string): Promise<ExchangeAssetInfo | null>;
    getAllAssetInfo(): Promise<ExchangeAssetInfo[]>;
    getAssetInfoByAsset(asset: string): Promise<AssetInfo[]>;
    private getCachedAssetInfo;
    private cacheAssetInfo;
    private fetchAssetInfoFromExchange;
    private parseExchangeResponse;
}
