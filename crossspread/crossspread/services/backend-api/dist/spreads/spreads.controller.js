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
var __param = (this && this.__param) || function (paramIndex, decorator) {
    return function (target, key) { decorator(target, key, paramIndex); }
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.SpreadsController = exports.GetSpreadsDto = void 0;
const common_1 = require("@nestjs/common");
const passport_1 = require("@nestjs/passport");
const spreads_service_1 = require("./spreads.service");
class GetSpreadsDto {
}
exports.GetSpreadsDto = GetSpreadsDto;
let SpreadsController = class SpreadsController {
    constructor(spreadsService) {
        this.spreadsService = spreadsService;
    }
    async getSpreads(query) {
        return this.spreadsService.getSpreads(query.token, query.limit || 50);
    }
    async getSpreadDetail(spreadId) {
        return this.spreadsService.getSpreadDetail(spreadId);
    }
    async getSpreadHistory(spreadId, from, to) {
        const fromDate = from ? new Date(from) : new Date(Date.now() - 24 * 60 * 60 * 1000);
        const toDate = to ? new Date(to) : new Date();
        return this.spreadsService.getSpreadHistory(spreadId, fromDate, toDate);
    }
    async getSpreadCandles(spreadId, interval, from, to, limit) {
        const fromDate = from ? new Date(from) : undefined;
        const toDate = to ? new Date(to) : undefined;
        const candleLimit = limit ? parseInt(limit, 10) : 500;
        return this.spreadsService.getSpreadCandles(spreadId, interval || '1m', fromDate, toDate, candleLimit);
    }
    async calculateSlippage(spreadId, sizeInCoins) {
        return this.spreadsService.calculateSlippage(spreadId, parseFloat(sizeInCoins));
    }
};
exports.SpreadsController = SpreadsController;
__decorate([
    (0, common_1.Get)(),
    __param(0, (0, common_1.Query)()),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [GetSpreadsDto]),
    __metadata("design:returntype", Promise)
], SpreadsController.prototype, "getSpreads", null);
__decorate([
    (0, common_1.Get)(':spreadId'),
    __param(0, (0, common_1.Param)('spreadId')),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [String]),
    __metadata("design:returntype", Promise)
], SpreadsController.prototype, "getSpreadDetail", null);
__decorate([
    (0, common_1.Get)(':spreadId/history'),
    __param(0, (0, common_1.Param)('spreadId')),
    __param(1, (0, common_1.Query)('from')),
    __param(2, (0, common_1.Query)('to')),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [String, String, String]),
    __metadata("design:returntype", Promise)
], SpreadsController.prototype, "getSpreadHistory", null);
__decorate([
    (0, common_1.Get)(':spreadId/candles'),
    __param(0, (0, common_1.Param)('spreadId')),
    __param(1, (0, common_1.Query)('interval')),
    __param(2, (0, common_1.Query)('from')),
    __param(3, (0, common_1.Query)('to')),
    __param(4, (0, common_1.Query)('limit')),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [String, String, String, String, String]),
    __metadata("design:returntype", Promise)
], SpreadsController.prototype, "getSpreadCandles", null);
__decorate([
    (0, common_1.Get)(':spreadId/slippage'),
    __param(0, (0, common_1.Param)('spreadId')),
    __param(1, (0, common_1.Query)('sizeInCoins')),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [String, String]),
    __metadata("design:returntype", Promise)
], SpreadsController.prototype, "calculateSlippage", null);
exports.SpreadsController = SpreadsController = __decorate([
    (0, common_1.Controller)('spreads'),
    (0, common_1.UseGuards)((0, passport_1.AuthGuard)('jwt')),
    __metadata("design:paramtypes", [spreads_service_1.SpreadsService])
], SpreadsController);
//# sourceMappingURL=spreads.controller.js.map