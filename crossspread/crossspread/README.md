# CrossSpread - Futures Arbitrage Platform

A self-hosted futures arbitrage SaaS for crypto derivatives across multiple exchanges.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Frontend (React + TypeScript)                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐ │
│  │ Spread List  │  │Spread Detail │  │ Trade Ticket │  │    Admin UI      │ │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Backend API (NestJS/TypeScript)                      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐ │
│  │   Auth/JWT   │  │  Spreads API │  │  Trade API   │  │   WebSocket      │ │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
        │                    │                    │
        ▼                    ▼                    ▼
┌───────────────┐  ┌─────────────────┐  ┌────────────────────────────────────┐
│   PostgreSQL  │  │  Redis Streams  │  │    Execution Service (Rust)        │
│  + TimescaleDB│  │   (Pub/Sub)     │  │  ┌────────────┐  ┌──────────────┐  │
└───────────────┘  └─────────────────┘  │  │  Slicing   │  │   Exchange   │  │
                           │            │  │  Engine    │  │   Adapters   │  │
                           ▼            │  └────────────┘  └──────────────┘  │
              ┌─────────────────────┐   └────────────────────────────────────┘
              │  Market Data Ingest │
              │  ┌───────┐ ┌──────┐ │
              │  │Binance│ │Bybit │ │   ◄── WebSocket L2 + REST resync
              │  │  OKX  │ │KuCoin│ │
              │  └───────┘ └──────┘ │
              └─────────────────────┘
```

## Quick Start

### Prerequisites

- Docker & Docker Compose
- Node.js 20+ (for local development)
- Go 1.21+ (for market data service)
- Rust 1.74+ (for execution service)

### Development Setup

```bash
# Clone and navigate to project
cd crossspread

# Copy environment file
cp .env.example .env
# Edit .env and set your encryption key (32 bytes base64)

# Start infrastructure services (PostgreSQL, Redis)
docker-compose -f infra/docker-compose.yml up -d postgres redis

# Run database migrations
cd services/backend-api
npm install
npx prisma migrate dev

# Start backend API
npm run start:dev

# In another terminal, start market data ingest
cd services/md-ingest
go run cmd/ingest/main.go

# In another terminal, start frontend
cd web/web-frontend
npm install
npm run dev

# Access UI
open http://localhost:3000
```

### Production Deployment

```bash
# Build and start all services
docker-compose -f infra/docker-compose.yml up -d --build

# Run migrations
docker-compose exec backend-api npx prisma migrate deploy

# Create admin user
docker-compose exec backend-api npm run seed:admin

# Access UI
open http://localhost:3000
```

## Project Structure

```
crossspread/
├─ infra/                    # Infrastructure configs
│  ├─ docker-compose.yml     # Local development
│  └─ helm/                  # Kubernetes deployment
├─ services/
│  ├─ execution-rust/        # Low-latency order execution (Rust)
│  ├─ md-ingest/             # Market data ingestion (Go)
│  ├─ backend-api/           # REST/WS API (NestJS)
│  └─ sim-backtest/          # Simulation engine (Python)
├─ web/
│  ├─ terminal/              # Trading terminal UI (React)
│  └─ admin/                 # Admin dashboard (React)
├─ packages/
│  └─ shared-types/          # Shared TypeScript types
├─ db/
│  ├─ migrations/            # SQL migrations
│  └─ schema/                # Database schema docs
├─ docs/                     # Documentation
└─ tests/
   ├─ integration/           # Integration tests
   └─ e2e/                   # End-to-end tests
```

## Two-Phase Market Data Architecture

The market data ingestion service uses an optimized two-phase approach to minimize unnecessary WebSocket subscriptions and reduce bandwidth/rate limit usage:

### Phase 1: REST-Based Discovery
1. Fetch all instruments (trading pairs) from each exchange via REST API
2. Fetch current prices via `/ticker/price` endpoints
3. Fetch funding rates via REST API
4. Fetch asset info (deposit/withdrawal status) where available
5. Discover preliminary spread opportunities using REST data
6. Identify which symbols actually have tradeable spreads

### Phase 2: Selective WebSocket Subscription
1. Based on discovered spreads, determine the minimum set of symbols needed
2. Connect to WebSocket APIs only for those specific symbols
3. Subscribe to L2 orderbook updates for real-time spread monitoring
4. Periodically refresh REST data to discover new opportunities

### Benefits
- **Reduced Bandwidth**: Only subscribe to symbols with actual spread opportunities
- **Lower Rate Limit Usage**: Avoid subscribing to hundreds of unnecessary pairs
- **Faster Startup**: REST API provides bulk data quickly
- **Cost Efficient**: Fewer WebSocket connections = lower server costs

### Configuration

```bash
# Enable two-phase mode (default: true)
USE_TWO_PHASE=true

# Minimum spread in basis points to consider (default: 5.0 = 0.05%)
MIN_SPREAD_BPS=5.0

# Enabled exchanges
ENABLED_EXCHANGES=binance,bybit,okx
```

### REST Endpoints Used (Binance Example)

| Endpoint | Purpose |
|----------|---------|
| `GET /fapi/v1/exchangeInfo` | Fetch all instruments, filters, fees |
| `GET /fapi/v1/ticker/price` | Fetch current prices for all symbols |
| `GET /fapi/v1/ticker/bookTicker` | Fetch best bid/ask for all symbols |
| `GET /fapi/v1/premiumIndex` | Fetch funding rates |

## MVP Features

- [x] Exchange connectors (Binance, Bybit, OKX, KuCoin)
- [x] Real-time spread discovery
- [x] Two-phase REST-first data loading
- [x] Spread detail with dual orderbook
- [x] Slippage calculator
- [x] Trade ticket with sliced orders
- [x] Emergency exit button
- [x] Admin user management
- [x] Paper trading mode

## Environment Variables

See `.env.example` for required configuration.

## License

Proprietary - All rights reserved

