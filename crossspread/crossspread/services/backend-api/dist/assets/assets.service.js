"use strict";
var __decorate = (this && this.__decorate) || function (decorators, target, key, desc) {
    var c = arguments.length, r = c < 3 ? target : desc === null ? desc = Object.getOwnPropertyDescriptor(target, key) : desc, d;
    if (typeof Reflect === "object" && typeof Reflect.decorate === "function") r = Reflect.decorate(decorators, target, key, desc);
    else for (var i = decorators.length - 1; i >= 0; i--) if (d = decorators[i]) r = (c < 3 ? d(r) : c > 3 ? d(target, key, r) : d(target, key)) || r;
    return c > 3 && r && Object.defineProperty(target, key, r), r;
};
var __metadata = (this && this.__metadata) || function (k, v) {
    if (typeof Reflect === "object" && typeof Reflect.metadata === "function") return Reflect.metadata(k, v);
};
var AssetsService_1;
Object.defineProperty(exports, "__esModule", { value: true });
exports.AssetsService = void 0;
const common_1 = require("@nestjs/common");
const redis_service_1 = require("../redis/redis.service");
const EXCHANGE_CONFIGS = {
    binance: {
        baseUrl: 'https://fapi.binance.com',
        endpoint: '/fapi/v1/exchangeInfo',
    },
    bybit: {
        baseUrl: 'https://api.bybit.com',
        endpoint: '/v5/market/instruments-info?category=linear',
    },
    okx: {
        baseUrl: 'https://www.okx.com',
        endpoint: '/api/v5/public/instruments?instType=SWAP',
    },
    kucoin: {
        baseUrl: 'https://api-futures.kucoin.com',
        endpoint: '/api/v1/contracts/active',
    },
    bitget: {
        baseUrl: 'https://api.bitget.com',
        endpoint: '/api/v2/mix/market/contracts?productType=USDT-FUTURES',
    },
    gateio: {
        baseUrl: 'https://api.gateio.ws',
        endpoint: '/api/v4/futures/usdt/contracts',
    },
    mexc: {
        baseUrl: 'https://contract.mexc.com',
        endpoint: '/api/v1/contract/detail',
    },
    htx: {
        baseUrl: 'https://api.hbdm.com',
        endpoint: '/linear-swap-api/v1/swap_contract_info',
    },
    coinex: {
        baseUrl: 'https://api.coinex.com/v2',
        endpoint: '/futures/market',
    },
    bingx: {
        baseUrl: 'https://open-api.bingx.com',
        endpoint: '/openApi/swap/v2/quote/contracts',
    },
    lbank: {
        baseUrl: 'https://lbkperp.lbank.com',
        endpoint: '/cfd/openApi/v1/pub/instrument',
    },
};
const CACHE_TTL = 300;
let AssetsService = AssetsService_1 = class AssetsService {
    constructor(redis) {
        this.redis = redis;
        this.logger = new common_1.Logger(AssetsService_1.name);
    }
    async getAssetInfo(exchange) {
        const normalizedExchange = exchange.toLowerCase();
        const cached = await this.getCachedAssetInfo(normalizedExchange);
        if (cached) {
            return cached;
        }
        const assets = await this.fetchAssetInfoFromExchange(normalizedExchange);
        if (!assets || assets.length === 0) {
            return null;
        }
        const result = {
            exchange: normalizedExchange,
            assets,
            lastUpdated: new Date(),
        };
        await this.cacheAssetInfo(normalizedExchange, result);
        return result;
    }
    async getAllAssetInfo() {
        const exchanges = Object.keys(EXCHANGE_CONFIGS);
        const results = [];
        await Promise.all(exchanges.map(async (exchange) => {
            try {
                const info = await this.getAssetInfo(exchange);
                if (info) {
                    results.push(info);
                }
            }
            catch (error) {
                this.logger.warn(`Failed to fetch asset info for ${exchange}:`, error);
            }
        }));
        return results;
    }
    async getAssetInfoByAsset(asset) {
        const allExchanges = await this.getAllAssetInfo();
        const results = [];
        for (const exchange of allExchanges) {
            const assetInfo = exchange.assets.find((a) => a.asset.toUpperCase() === asset.toUpperCase());
            if (assetInfo) {
                results.push(assetInfo);
            }
        }
        return results;
    }
    async getCachedAssetInfo(exchange) {
        try {
            const client = this.redis.getClient();
            const cached = await client.get(`assetinfo:${exchange}`);
            if (cached && typeof cached === 'string') {
                const parsed = JSON.parse(cached);
                parsed.lastUpdated = new Date(parsed.lastUpdated);
                parsed.assets = parsed.assets.map((a) => ({
                    ...a,
                    timestamp: new Date(a.timestamp),
                }));
                return parsed;
            }
        }
        catch (error) {
            this.logger.warn(`Error reading cache for ${exchange}:`, error);
        }
        return null;
    }
    async cacheAssetInfo(exchange, data) {
        try {
            const client = this.redis.getClient();
            await client.set(`assetinfo:${exchange}`, JSON.stringify(data), { EX: CACHE_TTL });
        }
        catch (error) {
            this.logger.warn(`Error caching asset info for ${exchange}:`, error);
        }
    }
    async fetchAssetInfoFromExchange(exchange) {
        const config = EXCHANGE_CONFIGS[exchange];
        if (!config) {
            this.logger.warn(`Unknown exchange: ${exchange}`);
            return [];
        }
        try {
            const response = await fetch(`${config.baseUrl}${config.endpoint}`, {
                headers: {
                    'Accept': 'application/json',
                    'User-Agent': 'CrossSpread-Backend/1.0',
                },
            });
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            const data = await response.json();
            return this.parseExchangeResponse(exchange, data);
        }
        catch (error) {
            this.logger.error(`Failed to fetch from ${exchange}:`, error);
            return [];
        }
    }
    parseExchangeResponse(exchange, data) {
        const assets = [];
        const assetMap = new Map();
        try {
            switch (exchange) {
                case 'binance':
                    for (const symbol of data.symbols || []) {
                        if (symbol.status === 'TRADING' && symbol.baseAsset) {
                            const asset = symbol.baseAsset;
                            if (!assetMap.has(asset)) {
                                assetMap.set(asset, {
                                    exchangeId: exchange,
                                    asset,
                                    depositEnabled: true,
                                    withdrawEnabled: true,
                                    timestamp: new Date(),
                                });
                            }
                        }
                    }
                    break;
                case 'bybit':
                    for (const item of data.result?.list || []) {
                        if (item.status === 'Trading' && item.baseCoin) {
                            const asset = item.baseCoin;
                            if (!assetMap.has(asset)) {
                                assetMap.set(asset, {
                                    exchangeId: exchange,
                                    asset,
                                    depositEnabled: true,
                                    withdrawEnabled: true,
                                    timestamp: new Date(),
                                });
                            }
                        }
                    }
                    break;
                case 'okx':
                    for (const inst of data.data || []) {
                        if (inst.state === 'live' && inst.ctValCcy) {
                            const asset = inst.ctValCcy;
                            if (!assetMap.has(asset)) {
                                assetMap.set(asset, {
                                    exchangeId: exchange,
                                    asset,
                                    depositEnabled: true,
                                    withdrawEnabled: true,
                                    timestamp: new Date(),
                                });
                            }
                        }
                    }
                    break;
                case 'kucoin':
                    for (const contract of data.data || []) {
                        const asset = contract.baseCurrency;
                        if (asset && !assetMap.has(asset)) {
                            assetMap.set(asset, {
                                exchangeId: exchange,
                                asset,
                                depositEnabled: true,
                                withdrawEnabled: true,
                                timestamp: new Date(),
                            });
                        }
                    }
                    break;
                case 'bitget':
                    for (const contract of data.data || []) {
                        if (contract.symbolStatus === 'normal' && contract.baseCoin) {
                            const asset = contract.baseCoin;
                            if (!assetMap.has(asset)) {
                                assetMap.set(asset, {
                                    exchangeId: exchange,
                                    asset,
                                    depositEnabled: true,
                                    withdrawEnabled: true,
                                    timestamp: new Date(),
                                });
                            }
                        }
                    }
                    break;
                case 'gateio':
                    for (const contract of data || []) {
                        if (!contract.in_delisting && contract.underlying) {
                            const asset = contract.underlying;
                            if (!assetMap.has(asset)) {
                                assetMap.set(asset, {
                                    exchangeId: exchange,
                                    asset,
                                    depositEnabled: true,
                                    withdrawEnabled: true,
                                    timestamp: new Date(),
                                });
                            }
                        }
                    }
                    break;
                case 'mexc':
                    for (const contract of data.data || []) {
                        if (contract.state === 0 && contract.baseCoin) {
                            const asset = contract.baseCoin;
                            if (!assetMap.has(asset)) {
                                assetMap.set(asset, {
                                    exchangeId: exchange,
                                    asset,
                                    depositEnabled: contract.apiAllowed !== false,
                                    withdrawEnabled: contract.apiAllowed !== false,
                                    timestamp: new Date(),
                                });
                            }
                        }
                    }
                    break;
                case 'htx':
                    for (const contract of data.data || []) {
                        if (contract.contract_status === 1 && contract.symbol) {
                            const asset = contract.symbol;
                            if (!assetMap.has(asset)) {
                                assetMap.set(asset, {
                                    exchangeId: exchange,
                                    asset,
                                    depositEnabled: true,
                                    withdrawEnabled: true,
                                    timestamp: new Date(),
                                });
                            }
                        }
                    }
                    break;
                case 'coinex':
                    for (const market of data.data || []) {
                        if (market.is_api_trading_available !== false && market.base_ccy) {
                            const asset = market.base_ccy;
                            if (!assetMap.has(asset)) {
                                assetMap.set(asset, {
                                    exchangeId: exchange,
                                    asset,
                                    depositEnabled: market.status === 'available',
                                    withdrawEnabled: market.status === 'available',
                                    timestamp: new Date(),
                                });
                            }
                        }
                    }
                    break;
                case 'bingx':
                    for (const contract of data.data || []) {
                        if (contract.status === 1 && contract.asset) {
                            const asset = contract.asset;
                            if (!assetMap.has(asset)) {
                                assetMap.set(asset, {
                                    exchangeId: exchange,
                                    asset,
                                    depositEnabled: contract.apiStateOpen === 1,
                                    withdrawEnabled: contract.apiStateOpen === 1,
                                    timestamp: new Date(),
                                });
                            }
                        }
                    }
                    break;
                case 'lbank':
                    for (const inst of data.data || []) {
                        if (inst.symbol) {
                            const parts = inst.symbol.split('_');
                            const asset = parts[0];
                            if (asset && !assetMap.has(asset)) {
                                assetMap.set(asset, {
                                    exchangeId: exchange,
                                    asset,
                                    depositEnabled: true,
                                    withdrawEnabled: true,
                                    timestamp: new Date(),
                                });
                            }
                        }
                    }
                    break;
                default:
                    this.logger.warn(`No parser for exchange: ${exchange}`);
            }
        }
        catch (error) {
            this.logger.error(`Error parsing response from ${exchange}:`, error);
        }
        return Array.from(assetMap.values());
    }
};
exports.AssetsService = AssetsService;
exports.AssetsService = AssetsService = AssetsService_1 = __decorate([
    (0, common_1.Injectable)(),
    __metadata("design:paramtypes", [redis_service_1.RedisService])
], AssetsService);
//# sourceMappingURL=assets.service.js.map