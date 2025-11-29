import { Controller, Get, Param, Query, UseGuards } from '@nestjs/common';
import { AuthGuard } from '@nestjs/passport';
import { AssetsService, AssetInfo, ExchangeAssetInfo } from './assets.service';

@Controller('assets')
export class AssetsController {
  constructor(private readonly assetsService: AssetsService) {}

  /**
   * Get asset info for all exchanges
   * GET /api/v1/assets
   */
  @Get()
  @UseGuards(AuthGuard('jwt'))
  async getAllAssets(): Promise<ExchangeAssetInfo[]> {
    return this.assetsService.getAllAssetInfo();
  }

  /**
   * Get asset info for a specific exchange
   * GET /api/v1/assets/exchange/:exchange
   */
  @Get('exchange/:exchange')
  @UseGuards(AuthGuard('jwt'))
  async getExchangeAssets(
    @Param('exchange') exchange: string,
  ): Promise<ExchangeAssetInfo | { error: string }> {
    const result = await this.assetsService.getAssetInfo(exchange);
    if (!result) {
      return { error: `No asset info found for exchange: ${exchange}` };
    }
    return result;
  }

  /**
   * Get asset info for a specific asset across all exchanges
   * GET /api/v1/assets/asset/:asset
   */
  @Get('asset/:asset')
  @UseGuards(AuthGuard('jwt'))
  async getAssetAcrossExchanges(
    @Param('asset') asset: string,
  ): Promise<AssetInfo[]> {
    return this.assetsService.getAssetInfoByAsset(asset);
  }

  /**
   * Get asset info for a specific asset on a specific exchange
   * GET /api/v1/assets/:exchange/:asset
   */
  @Get(':exchange/:asset')
  @UseGuards(AuthGuard('jwt'))
  async getAssetOnExchange(
    @Param('exchange') exchange: string,
    @Param('asset') asset: string,
  ): Promise<AssetInfo | { error: string }> {
    const exchangeInfo = await this.assetsService.getAssetInfo(exchange);
    if (!exchangeInfo) {
      return { error: `No asset info found for exchange: ${exchange}` };
    }

    const assetInfo = exchangeInfo.assets.find(
      (a) => a.asset.toUpperCase() === asset.toUpperCase(),
    );

    if (!assetInfo) {
      return { error: `Asset ${asset} not found on ${exchange}` };
    }

    return assetInfo;
  }
}
