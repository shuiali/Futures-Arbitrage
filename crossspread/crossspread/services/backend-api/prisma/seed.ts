import { PrismaClient } from '@prisma/client';
import * as bcrypt from 'bcrypt';

const prisma = new PrismaClient();

async function main() {
  console.log('Seeding database...');

  // Create admin user
  const adminPassword = process.env.ADMIN_PASSWORD || 'admin123';
  const hashedPassword = await bcrypt.hash(adminPassword, 10);

  const admin = await prisma.user.upsert({
    where: { username: 'admin' },
    update: {},
    create: {
      username: 'admin',
      passwordHash: hashedPassword,
      role: 'admin',
      isActive: true,
      expiresAt: null, // Admin never expires
    },
  });

  console.log(`Created admin user: ${admin.username}`);

  // Create demo exchanges
  const exchanges = [
    { name: 'binance', displayName: 'Binance', takerFee: 0.0004, makerFee: 0.0002 },
    { name: 'bybit', displayName: 'Bybit', takerFee: 0.0006, makerFee: 0.0001 },
    { name: 'okx', displayName: 'OKX', takerFee: 0.0005, makerFee: 0.0002 },
    { name: 'kucoin', displayName: 'KuCoin', takerFee: 0.0006, makerFee: 0.0002 },
    { name: 'mexc', displayName: 'MEXC', takerFee: 0.0006, makerFee: 0.0002 },
    { name: 'bitget', displayName: 'Bitget', takerFee: 0.0006, makerFee: 0.0002 },
    { name: 'gateio', displayName: 'Gate.io', takerFee: 0.00075, makerFee: 0.00025 },
    { name: 'bingx', displayName: 'BingX', takerFee: 0.0005, makerFee: 0.0002 },
    { name: 'coinex', displayName: 'CoinEx', takerFee: 0.0005, makerFee: 0.0003 },
    { name: 'lbank', displayName: 'LBank', takerFee: 0.0006, makerFee: 0.0004 },
    { name: 'htx', displayName: 'HTX', takerFee: 0.0004, makerFee: 0.0002 },
  ];

  for (const exchange of exchanges) {
    await prisma.exchange.upsert({
      where: { name: exchange.name },
      update: {
        displayName: exchange.displayName,
        takerFee: exchange.takerFee,
        makerFee: exchange.makerFee,
      },
      create: exchange,
    });
  }

  console.log(`Created ${exchanges.length} exchanges`);

  // Create a demo user for testing
  const demoPassword = process.env.DEMO_PASSWORD || 'demo123';
  const demoPwHash = await bcrypt.hash(demoPassword, 10);

  const demoUser = await prisma.user.upsert({
    where: { username: 'demo' },
    update: {},
    create: {
      username: 'demo',
      passwordHash: demoPwHash,
      role: 'user',
      isActive: true,
      expiresAt: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000), // 30 days
    },
  });

  console.log(`Created demo user: ${demoUser.username}`);

  console.log('Seeding complete!');
}

main()
  .catch((e) => {
    console.error(e);
    process.exit(1);
  })
  .finally(async () => {
    await prisma.$disconnect();
  });
