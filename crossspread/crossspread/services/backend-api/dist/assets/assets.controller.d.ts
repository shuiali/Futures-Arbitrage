import { AssetsService, AssetInfo, ExchangeAssetInfo } from './assets.service';
export declare class AssetsController {
    private readonly assetsService;
    constructor(assetsService: AssetsService);
    getAllAssets(): Promise<ExchangeAssetInfo[]>;
    getExchangeAssets(exchange: string): Promise<ExchangeAssetInfo | {
        error: string;
    }>;
    getAssetAcrossExchanges(asset: string): Promise<AssetInfo[]>;
    getAssetOnExchange(exchange: string, asset: string): Promise<AssetInfo | {
        error: string;
    }>;
}
