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
exports.InternalController = void 0;
const common_1 = require("@nestjs/common");
const users_service_1 = require("./users.service");
let InternalController = class InternalController {
    constructor(usersService) {
        this.usersService = usersService;
        this.serviceSecret = process.env.SERVICE_SECRET || 'default-dev-secret';
        if (process.env.NODE_ENV === 'production' && this.serviceSecret === 'default-dev-secret') {
            console.warn('WARNING: Using default SERVICE_SECRET in production!');
        }
    }
    validateServiceAuth(authHeader) {
        if (!authHeader) {
            throw new common_1.UnauthorizedException('Missing authorization header');
        }
        const [scheme, token] = authHeader.split(' ');
        if (scheme !== 'Service' || token !== this.serviceSecret) {
            throw new common_1.UnauthorizedException('Invalid service credentials');
        }
    }
    async getExchangeCredentials(exchange, authHeader) {
        this.validateServiceAuth(authHeader);
        return this.usersService.getDecryptedApiCredentials(exchange);
    }
    async getAllCredentials(authHeader) {
        this.validateServiceAuth(authHeader);
        return this.usersService.getAllDecryptedApiCredentials();
    }
};
exports.InternalController = InternalController;
__decorate([
    (0, common_1.Get)('credentials/:exchange'),
    __param(0, (0, common_1.Param)('exchange')),
    __param(1, (0, common_1.Headers)('authorization')),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [String, String]),
    __metadata("design:returntype", Promise)
], InternalController.prototype, "getExchangeCredentials", null);
__decorate([
    (0, common_1.Get)('credentials'),
    __param(0, (0, common_1.Headers)('authorization')),
    __metadata("design:type", Function),
    __metadata("design:paramtypes", [String]),
    __metadata("design:returntype", Promise)
], InternalController.prototype, "getAllCredentials", null);
exports.InternalController = InternalController = __decorate([
    (0, common_1.Controller)('internal'),
    __metadata("design:paramtypes", [users_service_1.UsersService])
], InternalController);
//# sourceMappingURL=internal.controller.js.map