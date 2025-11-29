import { Injectable, NotFoundException, ConflictException } from '@nestjs/common';
import { PrismaService } from '../prisma/prisma.service';
import { AuthService } from '../auth/auth.service';
import * as crypto from 'crypto';

interface CreateUserRequest {
  username: string;
  password: string;
  expiryDays?: number;
}

interface UpdateUserRequest {
  isActive?: boolean;
  expiryDays?: number;
}

interface AddApiKeyRequest {
  exchangeId: string;
  apiKey: string;
  apiSecret: string;
  passphrase?: string;
  label?: string;
}

@Injectable()
export class UsersService {
  private readonly encryptionKey: Buffer;

  constructor(
    private prisma: PrismaService,
    private authService: AuthService,
  ) {
    const keyBase64 = process.env.ENCRYPTION_KEY_BASE64 || '';
    this.encryptionKey = Buffer.from(keyBase64, 'base64');
  }

  async createUser(request: CreateUserRequest) {
    // Check if username exists
    const existing = await this.prisma.user.findUnique({
      where: { username: request.username },
    });

    if (existing) {
      throw new ConflictException('Username already exists');
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

  async listUsers(page: number, limit: number) {
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

  async getUser(userId: string) {
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
      throw new NotFoundException(`User ${userId} not found`);
    }

    return user;
  }

  async updateUser(userId: string, request: UpdateUserRequest) {
    const user = await this.prisma.user.findUnique({
      where: { id: userId },
    });

    if (!user) {
      throw new NotFoundException(`User ${userId} not found`);
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

  async deleteUser(userId: string) {
    const user = await this.prisma.user.findUnique({
      where: { id: userId },
    });

    if (!user) {
      throw new NotFoundException(`User ${userId} not found`);
    }

    // Soft delete by deactivating
    await this.prisma.user.update({
      where: { id: userId },
      data: { isActive: false },
    });

    return { deleted: true };
  }

  async getUserPositions(userId: string) {
    const positions = await this.prisma.position.findMany({
      where: { userId },
      include: {
        spread: true,
      },
      orderBy: { createdAt: 'desc' },
    });

    return positions;
  }

  async addApiKey(userId: string, request: AddApiKeyRequest) {
    // Encrypt API key and secret
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

  async listApiKeys(userId: string) {
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

    return keys.map((k: any) => ({
      id: k.id,
      exchangeId: k.exchangeId,
      exchangeName: k.exchange.name,
      label: k.label,
      isActive: k.isActive,
      createdAt: k.createdAt,
    }));
  }

  async deleteApiKey(userId: string, keyId: string) {
    const key = await this.prisma.apiKey.findFirst({
      where: { id: keyId, userId },
    });

    if (!key) {
      throw new NotFoundException(`API key ${keyId} not found`);
    }

    await this.prisma.apiKey.delete({
      where: { id: keyId },
    });

    return { deleted: true };
  }

  private encrypt(plaintext: string): string {
    const iv = crypto.randomBytes(12);
    const cipher = crypto.createCipheriv('aes-256-gcm', this.encryptionKey, iv);
    
    let encrypted = cipher.update(plaintext, 'utf8', 'base64');
    encrypted += cipher.final('base64');
    
    const tag = cipher.getAuthTag();
    
    // Format: iv:tag:ciphertext (all base64)
    return `${iv.toString('base64')}:${tag.toString('base64')}:${encrypted}`;
  }

  private decrypt(ciphertext: string): string {
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

  /**
   * Get decrypted API credentials for a specific exchange (for internal service use)
   * This should only be called by internal services with proper authentication
   */
  async getDecryptedApiCredentials(exchangeName: string): Promise<{
    apiKey: string;
    apiSecret: string;
    passphrase?: string;
    userId: string;
  }[]> {
    // Get all active API keys for the exchange
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

  /**
   * Get all decrypted API credentials grouped by exchange (for internal service use)
   */
  async getAllDecryptedApiCredentials(): Promise<Record<string, {
    apiKey: string;
    apiSecret: string;
    passphrase?: string;
    userId: string;
  }[]>> {
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

    const result: Record<string, { apiKey: string; apiSecret: string; passphrase?: string; userId: string }[]> = {};

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
}
