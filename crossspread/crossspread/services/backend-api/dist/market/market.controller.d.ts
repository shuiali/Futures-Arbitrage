import { MarketService } from './market.service';
export declare class MarketController {
    private marketService;
    constructor(marketService: MarketService);
    getTokens(): Promise<any[]>;
    getExchanges(): Promise<{
        id: string;
        isActive: boolean;
        name: string;
        displayName: string;
        takerFee: number;
        makerFee: number;
    }[]>;
}
