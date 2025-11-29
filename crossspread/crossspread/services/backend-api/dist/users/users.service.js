"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __decorate = (this && this.__decorate) || function (decorators, target, key, desc) {
    var c = arguments.length, r = c < 3 ? target : desc === null ? desc = Object.getOwnPropertyDescriptor(target, key) : desc, d;
    if (typeof Reflect === "object" && typeof Reflect.decorate === "function") r = Reflect.decorate(decorators, target, key, desc);
    else for (var i = decorators.length - 1; i >= 0; i--) if (d = decorators[i]) r = (c < 3 ? d(r) : c > 3 ? d(target, key, r) : d(target, key)) || r;
    return c > 3 && r && Object.defineProperty(target, key, r), r;
};
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
var __metadata = (this && this.__metadata) || function (k, v) {
    if (typeof Reflect === "object" && typeof Reflect.metadata === "function") return Reflect.metadata(k, v);
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.UsersService = void 0;
const common_1 = require("@nestjs/common");
const prisma_service_1 = require("../prisma/prisma.service");
const auth_service_1 = require("../auth/auth.service");
const crypto = __importStar(require("crypto"));
let UsersService = class UsersService {
    constructor(prisma, authService) {
        this.prisma = prisma;
        this.authService = authService;
        const keyBase64 = process.env.ENCRYPTION_KEY_BASE64 || '';
        this.encryptionKey = Buffer.from(keyBase64, 'base64');
    }
    async createUser(request) {
        const existing = await this.prisma.user.findUnique({
            where: { username: request.username },
        });
        if (existing) {
            throw new common_1.ConflictException('Username already exists');
        }
        const passwordHash = await this.authService.hashPassword(request.password);
        const expiresAt = request.expiryDays
            ? new Date(Date.now() + request.expiryDays * 24 * 60 * 60 * 1000)
            : null;
        const user = await this.prisma.user.create({
            data: {
                username: request.username,
                passwordHash,
                role: 'user',
                isActive: true,
                expiresAt,
            },
        });
        return {
            id: user.id,
            username: user.username,
            role: user.role,
            isActive: user.isActive,
            expiresAt: user.expiresAt,
            createdAt: user.createdAt,
        };
    }
    async listUsers(page, limit) {
        const skip = (page - 1) * limit;
        const [users, total] = await Promise.all([
            this.prisma.user.findMany({
                skip,
                take: limit,
                orderBy: { createdAt: 'desc' },
                select: {
                    id: true,
                    username: true,
                    role: true,
                    isActive: true,
                    expiresAt: true,
                    createdAt: true,
                },
            }),
            this.prisma.user.count(),
        ]);
        return {
            users,
            pagination: {
                page,
                limit,
                total,
                totalPages: Math.ceil(total / limit),
            },
        };
    }
    async getUser(userId) {
        const user = await this.prisma.user.findUnique({
            where: { id: userId },
            select: {
                id: true,
                username: true,
                role: true,
                isActive: true,
                expiresAt: true,
                createdAt: true,
                updatedAt: true,
                _count: {
                    select: {
                        positions: true,
                        apiKeys: true,
                    },
                },
            },
        });
        if (!user) {
            throw new common_1.NotFoundException(`User ${userId} not found`);
        }
        return user;
    }
    async updateUser(userId, request) {
        const user = await this.prisma.user.findUnique({
            where: { id: userId },
        });
        if (!user) {
            throw new common_1.NotFoundException(`User ${userId} not found`);
        }
        const expiresAt = request.expiryDays
            ? new Date(Date.now() + request.expiryDays * 24 * 60 * 60 * 1000)
            : undefined;
        const updated = await this.prisma.user.update({
            where: { id: userId },
            data: {
                isActive: request.isActive,
                expiresAt,
            },
        });
        return {
            id: updated.id,
            username: updated.username,
            role: updated.role,
            isActive: updated.isActive,
            expiresAt: updated.expiresAt,
        };
    }
    async deleteUser(userId) {
        const user = await this.prisma.user.findUnique({
            where: { id: userId },
        });
        if (!user) {
            throw new common_1.NotFoundException(`User ${userId} not found`);
        }
        await this.prisma.user.update({
            where: { id: userId },
            data: { isActive: false },
        });
        return { deleted: true };
    }
    async getUserPositions(userId) {
        const positions = await this.prisma.position.findMany({
            where: { userId },
            include: {
                spread: true,
            },
            orderBy: { createdAt: 'desc' },
        });
        return positions;
    }
    async addApiKey(userId, request) {
        const encryptedApiKey = this.encrypt(request.apiKey);
        const encryptedApiSecret = this.encrypt(request.apiSecret);
        const encryptedPassphrase = request.passphrase
            ? this.encrypt(request.passphrase)
            : null;
        const apiKey = await this.prisma.apiKey.create({
            data: {
                userId,
                exchangeId: request.exchangeId,
                apiKeyEncrypted: encryptedApiKey,
                apiSecretEncrypted: encryptedApiSecret,
                passphraseEncrypted: encryptedPassphrase,
                label: request.label || `${request.exchangeId} key`,
                isActive: true,
            },
        });
        return {
            id: apiKey.id,
            exchangeId: apiKey.exchangeId,
            label: apiKey.label,
            isActive: apiKey.isActive,
            createdAt: apiKey.createdAt,
        };
    }
    async listApiKeys(userId) {
        const keys = await this.prisma.apiKey.findMany({
            where: { userId },
            select: {
                id: true,
                exchangeId: true,
                label: true,
                isActive: true,
                createdAt: true,
                exchange: {
                    select: { name: true },
                },
            },
        });
        return keys.map((k) => ({
            id: k.id,
            exchangeId: k.exchangeId,
            exchangeName: k.exchange.name,
            label: k.label,
            isActive: k.isActive,
            createdAt: k.createdAt,
        }));
    }
    async deleteApiKey(userId, keyId) {
        const key = await this.prisma.apiKey.findFirst({
            where: { id: keyId, userId },
        });
        if (!key) {
            throw new common_1.NotFoundException(`API key ${keyId} not found`);
        }
        await this.prisma.apiKey.delete({
            where: { id: keyId },
        });
        return { deleted: true };
    }
    encrypt(plaintext) {
        const iv = crypto.randomBytes(12);
        const cipher = crypto.createCipheriv('aes-256-gcm', this.encryptionKey, iv);
        let encrypted = cipher.update(plaintext, 'utf8', 'base64');
        encrypted += cipher.final('base64');
        const tag = cipher.getAuthTag();
        return `${iv.toString('base64')}:${tag.toString('base64')}:${encrypted}`;
    }
    decrypt(ciphertext) {
        const parts = ciphertext.split(':');
        if (parts.length !== 3) {
            throw new Error('Invalid ciphertext format');
        }
        const iv = Buffer.from(parts[0], 'base64');
        const tag = Buffer.from(parts[1], 'base64');
        const encrypted = parts[2];
        const decipher = crypto.createDecipheriv('aes-256-gcm', this.encryptionKey, iv);
        decipher.setAuthTag(tag);
        let decrypted = decipher.update(encrypted, 'base64', 'utf8');
        decrypted += decipher.final('utf8');
        return decrypted;
    }
    async getDecryptedApiCredentials(exchangeName) {
        const keys = await this.prisma.apiKey.findMany({
            where: {
                isActive: true,
                exchange: {
                    name: exchangeName.toLowerCase(),
                },
                user: {
                    isActive: true,
                },
            },
            include: {
                exchange: true,
            },
        });
        return keys.map((key) => ({
            apiKey: this.decrypt(key.apiKeyEncrypted),
            apiSecret: this.decrypt(key.apiSecretEncrypted),
            passphrase: key.passphraseEncrypted ? this.decrypt(key.passphraseEncrypted) : undefined,
            userId: key.userId,
        }));
    }
    async getAllDecryptedApiCredentials() {
        const keys = await this.prisma.apiKey.findMany({
            where: {
                isActive: true,
                user: {
                    isActive: true,
                },
            },
            include: {
                exchange: true,
            },
        });
        const result = {};
        for (const key of keys) {
            const exchangeName = key.exchange.name.toLowerCase();
            if (!result[exchangeName]) {
                result[exchangeName] = [];
            }
            result[exchangeName].push({
                apiKey: this.decrypt(key.apiKeyEncrypted),
                apiSecret: this.decrypt(key.apiSecretEncrypted),
                passphrase: key.passphraseEncrypted ? this.decrypt(key.passphraseEncrypted) : undefined,
                userId: key.userId,
            });
        }
        return result;
    }
};
exports.UsersService = UsersService;
exports.UsersService = UsersService = __decorate([
    (0, common_1.Injectable)(),
    __metadata("design:paramtypes", [prisma_service_1.PrismaService,
        auth_service_1.AuthService])
], UsersService);
//# sourceMappingURL=users.service.js.map