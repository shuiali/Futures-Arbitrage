import { JwtService } from '@nestjs/jwt';
import { PrismaService } from '../prisma/prisma.service';
export declare class AuthService {
    private prisma;
    private jwtService;
    constructor(prisma: PrismaService, jwtService: JwtService);
    validateUser(username: string, password: string): Promise<any>;
    login(user: {
        id: string;
        username: string;
        role: string;
    }): Promise<{
        accessToken: string;
        expiresIn: number;
        user: {
            id: string;
            username: string;
            role: string;
        };
    }>;
    hashPassword(password: string): Promise<string>;
}
