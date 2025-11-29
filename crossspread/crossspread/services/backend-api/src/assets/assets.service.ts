import { Injectable, Logger } from '@nestjs/common';
import { RedisService } from '../redis/redis.service';

// Types for asset info
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

// Exchange API configurations
const EXCHANGE_CONFIGS: Record<string, { baseUrl: string; endpoint: string }> = {
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

// Cache TTL in seconds (5 minutes)
const CACHE_TTL = 300;

@Injectable()
export class AssetsService {
  private readonly logger = new Logger(AssetsService.name);

  constructor(private redis: RedisService) {}

  /**
   * Get asset info for a specific exchange
   */
  async getAssetInfo(exchange: string): Promise<ExchangeAssetInfo | null> {
    const normalizedExchange = exchange.toLowerCase();
    
    // Check cache first
    const cached = await this.getCachedAssetInfo(normalizedExchange);
    if (cached) {
      return cached;
    }

    // Fetch from exchange API
    const assets = await this.fetchAssetInfoFromExchange(normalizedExchange);
    if (!assets || assets.length === 0) {
      return null;
    }

    const result: ExchangeAssetInfo = {
      exchange: normalizedExchange,
      assets,
      lastUpdated: new Date(),
    };

    // Cache the result
    await this.cacheAssetInfo(normalizedExchange, result);

    return result;
  }

  /**
   * Get asset info for all exchanges
   */
  async getAllAssetInfo(): Promise<ExchangeAssetInfo[]> {
    const exchanges = Object.keys(EXCHANGE_CONFIGS);
    const results: ExchangeAssetInfo[] = [];

    await Promise.all(
      exchanges.map(async (exchange) => {
        try {
          const info = await this.getAssetInfo(exchange);
          if (info) {
            results.push(info);
          }
        } catch (error) {
          this.logger.warn(`Failed to fetch asset info for ${exchange}:`, error);
        }
      }),
    );

    return results;
  }

  /**
   * Get asset info for a specific asset across all exchanges
   */
  async getAssetInfoByAsset(asset: string): Promise<AssetInfo[]> {
    const allExchanges = await this.getAllAssetInfo();
    const results: AssetInfo[] = [];

    for (const exchange of allExchanges) {
      const assetInfo = exchange.assets.find(
        (a) => a.asset.toUpperCase() === asset.toUpperCase(),
      );
      if (assetInfo) {
        results.push(assetInfo);
      }
    }

    return results;
  }

  private async getCachedAssetInfo(exchange: string): Promise<ExchangeAssetInfo | null> {
    try {
      const client = this.redis.getClient();
      const cached = await client.get(`assetinfo:${exchange}`);
      if (cached && typeof cached === 'string') {
        const parsed = JSON.parse(cached);
        parsed.lastUpdated = new Date(parsed.lastUpdated);
        parsed.assets = parsed.assets.map((a: any) => ({
          ...a,
          timestamp: new Date(a.timestamp),
        }));
        return parsed;
      }
    } catch (error) {
      this.logger.warn(`Error reading cache for ${exchange}:`, error);
    }
    return null;
  }

  private async cacheAssetInfo(exchange: string, data: ExchangeAssetInfo): Promise<void> {
    try {
      const client = this.redis.getClient();
      await client.set(
        `assetinfo:${exchange}`,
        JSON.stringify(data),
        { EX: CACHE_TTL },
      );
    } catch (error) {
      this.logger.warn(`Error caching asset info for ${exchange}:`, error);
    }
  }

  private async fetchAssetInfoFromExchange(exchange: string): Promise<AssetInfo[]> {
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
    } catch (error) {
      this.logger.error(`Failed to fetch from ${exchange}:`, error);
      return [];
    }
  }

  private parseExchangeResponse(exchange: string, data: any): AssetInfo[] {
    const assets: AssetInfo[] = [];
    const assetMap = new Map<string, AssetInfo>();

    try {
      switch (exchange) {
        case 'binance':
          // Binance returns { symbols: [...] }
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
          // Bybit returns { result: { list: [...] } }
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
          // OKX returns { data: [...] }
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
          // KuCoin returns { code: "200000", data: [...] }
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
          // Bitget returns { code: "00000", data: [...] }
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
          // GateIO returns array of contracts directly
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
          // MEXC returns { success: true, data: [...] }
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
          // HTX returns { status: "ok", data: [...] }
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
          // CoinEx returns { code: 0, data: [...] }
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
          // BingX returns { code: 0, data: [...] }
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
          // LBank returns { result: true, data: [...] }
          for (const inst of data.data || []) {
            if (inst.symbol) {
              // Extract base asset from symbol like "BTC_USDT"
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
    } catch (error) {
      this.logger.error(`Error parsing response from ${exchange}:`, error);
    }

    return Array.from(assetMap.values());
  }
}
