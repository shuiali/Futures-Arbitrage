import { SpreadsService, SpreadSummary, SlippageResult } from './spreads.service';
export declare class GetSpreadsDto {
    token?: string;
    limit?: number;
}
export declare class SpreadsController {
    private spreadsService;
    constructor(spreadsService: SpreadsService);
    getSpreads(query: GetSpreadsDto): Promise<SpreadSummary[]>;
    getSpreadDetail(spreadId: string): Promise<any>;
    getSpreadHistory(spreadId: string, from?: string, to?: string): Promise<{
        id: string;
        spreadId: string;
        spreadPercent: number;
        longPrice: number;
        shortPrice: number;
        volume: number | null;
        timestamp: Date;
    }[]>;
    getSpreadCandles(spreadId: string, interval?: string, from?: string, to?: string, limit?: string): Promise<{
        time: number;
        open: number;
        high: number;
        low: number;
        close: number;
        volume: number;
    }[]>;
    calculateSlippage(spreadId: string, sizeInCoins: string): Promise<SlippageResult>;
}
