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
exports.TradesController = exports.ExitTradeDto = exports.EnterTradeDto = void 0;
const common_1 = require("@nestjs/common");
const passport_1 = require("@nestjs/passport");
const trades_service_1 = require("./trades.service");
class EnterTradeDto {
}
exports.EnterTradeDto = EnterTradeDto;
class ExitTradeDto {
}
exports.ExitTradeDto = ExitTradeDto;
let TradesController = class TradesController {
    constructor(tradesService) {
        this.tradesService = tradesService;
    }
    async enterTrade(req, dto) {
        return this.tradesService.enterTrade(req.user.id, dto);
    }
    async exitTrade(req, positionId, dto) {
        return this.tradesService.exitTrade(req.user.id, positionId, dto.mode || 'normal');
    }
    async getPositions(req) {
        return this.tradesService.getPositions(req.user.id);
    }
    async getPosition(req, positionId) {
        return this.tradesService.getPosition(req.user.id, positionId);
    }
    async getOrders(req) {
        return this.tradesService.getOrders(req.user.id);
    }
    async cancelOrder(req, orderId) {
        return this.tradesService.cancelOrder(req.user.id, orderId);
    }
};
exports.TradesController = TradesController;
__decorate([
    (0, common_1.Post)('enter'),
    (0, common_1.HttpCode)(common_1.HttpStatus.CREATED),
    __param(0, (0, common_1.Request)()),
    __param(1, (0, common_1.Body)()),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [Object, EnterTradeDto]),
    __metadata("design:returntype", Promise)
], TradesController.prototype, "enterTrade", null);
__decorate([
    (0, common_1.Post)('exit/:positionId'),
    (0, common_1.HttpCode)(common_1.HttpStatus.OK),
    __param(0, (0, common_1.Request)()),
    __param(1, (0, common_1.Param)('positionId')),
    __param(2, (0, common_1.Body)()),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [Object, String, ExitTradeDto]),
    __metadata("design:returntype", Promise)
], TradesController.prototype, "exitTrade", null);
__decorate([
    (0, common_1.Get)('positions'),
    __param(0, (0, common_1.Request)()),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [Object]),
    __metadata("design:returntype", Promise)
], TradesController.prototype, "getPositions", null);
__decorate([
    (0, common_1.Get)('positions/:positionId'),
    __param(0, (0, common_1.Request)()),
    __param(1, (0, common_1.Param)('positionId')),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [Object, String]),
    __metadata("design:returntype", Promise)
], TradesController.prototype, "getPosition", null);
__decorate([
    (0, common_1.Get)('orders'),
    __param(0, (0, common_1.Request)()),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [Object]),
    __metadata("design:returntype", Promise)
], TradesController.prototype, "getOrders", null);
__decorate([
    (0, common_1.Post)('orders/:orderId/cancel'),
    (0, common_1.HttpCode)(common_1.HttpStatus.OK),
    __param(0, (0, common_1.Request)()),
    __param(1, (0, common_1.Param)('orderId')),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [Object, String]),
    __metadata("design:returntype", Promise)
], TradesController.prototype, "cancelOrder", null);
exports.TradesController = TradesController = __decorate([
    (0, common_1.Controller)('trade'),
    (0, common_1.UseGuards)((0, passport_1.AuthGuard)('jwt')),
    __metadata("design:paramtypes", [trades_service_1.TradesService])
], TradesController);
//# sourceMappingURL=trades.controller.js.map