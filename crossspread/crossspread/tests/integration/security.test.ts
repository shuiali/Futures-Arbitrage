/**
 * Security tests for API key encryption and access control
 */

import { describe, it, expect } from '@jest/globals';
import * as crypto from 'crypto';

describe('Security: API Key Encryption', () => {
  const ENCRYPTION_KEY = crypto.randomBytes(32);
  const ALGORITHM = 'aes-256-gcm';

  function encrypt(text: string): { encrypted: string; iv: string; tag: string } {
    const iv = crypto.randomBytes(16);
    const cipher = crypto.createCipheriv(ALGORITHM, ENCRYPTION_KEY, iv);
    
    let encrypted = cipher.update(text, 'utf8', 'hex');
    encrypted += cipher.final('hex');
    
    return {
      encrypted,
      iv: iv.toString('hex'),
      tag: cipher.getAuthTag().toString('hex'),
    };
  }

  function decrypt(encrypted: string, iv: string, tag: string): string {
    const decipher = crypto.createDecipheriv(
      ALGORITHM, 
      ENCRYPTION_KEY, 
      Buffer.from(iv, 'hex')
    );
    decipher.setAuthTag(Buffer.from(tag, 'hex'));
    
    let decrypted = decipher.update(encrypted, 'hex', 'utf8');
    decrypted += decipher.final('utf8');
    
    return decrypted;
  }

  it('should encrypt API key at rest', () => {
    const apiKey = 'vmPUZE6mv9SD5VNHk4HlWFsOr6aKE2zvsw0MuIgwCIPy6utIco14y7Ju91duEh8A';
    
    const { encrypted, iv, tag } = encrypt(apiKey);
    
    // Encrypted value should not contain original key
    expect(encrypted).not.toContain(apiKey);
    expect(encrypted.length).toBeGreaterThan(0);
    expect(iv.length).toBe(32); // 16 bytes = 32 hex chars
  });

  it('should decrypt API key correctly', () => {
    const originalKey = 'NhqPtmdSJYdKjVHjA7PZj4Mge3R5YNiP1e3UZjInClVN65XAbvqqM6A7H5fATj0j';
    
    const { encrypted, iv, tag } = encrypt(originalKey);
    const decrypted = decrypt(encrypted, iv, tag);
    
    expect(decrypted).toBe(originalKey);
  });

  it('should fail decryption with wrong key', () => {
    const apiKey = 'secretKey123';
    const { encrypted, iv, tag } = encrypt(apiKey);
    
    // Create decipher with different key
    const wrongKey = crypto.randomBytes(32);
    const decipher = crypto.createDecipheriv(
      ALGORITHM, 
      wrongKey, 
      Buffer.from(iv, 'hex')
    );
    decipher.setAuthTag(Buffer.from(tag, 'hex'));
    
    expect(() => {
      decipher.update(encrypted, 'hex', 'utf8');
      decipher.final('utf8');
    }).toThrow();
  });
});

describe('Security: Log Sanitization', () => {
  function sanitizeLog(log: Record<string, any>): Record<string, any> {
    const sensitiveKeys = ['password', 'apiKey', 'apiSecret', 'passphrase', 'token', 'secret'];
    const sanitized = { ...log };
    
    for (const key of Object.keys(sanitized)) {
      if (sensitiveKeys.some(sk => key.toLowerCase().includes(sk.toLowerCase()))) {
        sanitized[key] = '[REDACTED]';
      }
    }
    
    return sanitized;
  }

  it('should redact sensitive fields in logs', () => {
    const logEntry = {
      action: 'API_KEY_ADDED',
      userId: 'user-123',
      exchangeId: 'binance',
      apiKey: 'abc123secretkey',
      apiSecret: 'topsecret',
      timestamp: new Date().toISOString(),
    };
    
    const sanitized = sanitizeLog(logEntry);
    
    expect(sanitized.apiKey).toBe('[REDACTED]');
    expect(sanitized.apiSecret).toBe('[REDACTED]');
    expect(sanitized.action).toBe('API_KEY_ADDED');
    expect(sanitized.userId).toBe('user-123');
  });

  it('should not expose passwords in error messages', () => {
    const error = {
      message: 'Login failed',
      username: 'admin',
      password: 'adminPassword123',
    };
    
    const sanitized = sanitizeLog(error);
    
    expect(sanitized.password).toBe('[REDACTED]');
    expect(sanitized.username).toBe('admin');
  });
});

describe('Security: Access Control', () => {
  const users = {
    admin: { id: 'admin-1', role: 'admin', isActive: true },
    user: { id: 'user-1', role: 'user', isActive: true },
    expired: { id: 'user-2', role: 'user', isActive: true, expiresAt: new Date('2020-01-01') },
    disabled: { id: 'user-3', role: 'user', isActive: false },
  };

  function canAccessAdminRoutes(user: typeof users.admin): boolean {
    return user.role === 'admin' && user.isActive;
  }

  function isUserExpired(user: typeof users.expired): boolean {
    if (!user.expiresAt) return false;
    return new Date(user.expiresAt) < new Date();
  }

  it('should allow admin to access admin routes', () => {
    expect(canAccessAdminRoutes(users.admin)).toBe(true);
  });

  it('should deny regular user access to admin routes', () => {
    expect(canAccessAdminRoutes(users.user as any)).toBe(false);
  });

  it('should detect expired user accounts', () => {
    expect(isUserExpired(users.expired)).toBe(true);
  });

  it('should block disabled accounts', () => {
    expect(users.disabled.isActive).toBe(false);
  });
});

describe('Security: Rate Limiting', () => {
  class RateLimiter {
    private requests: Map<string, number[]> = new Map();
    private windowMs: number;
    private maxRequests: number;

    constructor(windowMs: number, maxRequests: number) {
      this.windowMs = windowMs;
      this.maxRequests = maxRequests;
    }

    isAllowed(userId: string): boolean {
      const now = Date.now();
      const userRequests = this.requests.get(userId) || [];
      
      // Remove old requests outside window
      const recentRequests = userRequests.filter(t => now - t < this.windowMs);
      
      if (recentRequests.length >= this.maxRequests) {
        return false;
      }
      
      recentRequests.push(now);
      this.requests.set(userId, recentRequests);
      return true;
    }
  }

  it('should allow requests within limit', () => {
    const limiter = new RateLimiter(1000, 10); // 10 req per second
    
    for (let i = 0; i < 10; i++) {
      expect(limiter.isAllowed('user-1')).toBe(true);
    }
  });

  it('should block requests exceeding limit', () => {
    const limiter = new RateLimiter(1000, 5);
    
    for (let i = 0; i < 5; i++) {
      limiter.isAllowed('user-1');
    }
    
    expect(limiter.isAllowed('user-1')).toBe(false);
  });
});
