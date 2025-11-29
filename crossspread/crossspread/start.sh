#!/bin/bash
set -e
cd "$(dirname "$0")"

mkdir -p .pids logs

# Environment variables
export DATABASE_URL="postgresql://crossspread:changeme@localhost:5432/crossspread"
export REDIS_URL="redis://localhost:6379"
export REDIS_HOST="localhost"
export REDIS_PORT="6379"
export JWT_SECRET="supersecretjwtkey_change_in_production"
export SERVICE_SECRET="service_secret_key"
export API_URL="http://localhost:8000"

echo "=== CrossSpread Start ==="

# 1. Docker
echo "[1/5] Starting Docker..."
docker-compose -f infra/docker-compose.yml up -d postgres redis

# Wait for PostgreSQL
echo "  Waiting for PostgreSQL..."
for i in {1..30}; do
    if docker exec crossspread-postgres pg_isready -U crossspread > /dev/null 2>&1; then
        echo "  PostgreSQL ready"
        break
    fi
    sleep 1
done

# Wait for Redis
echo "  Waiting for Redis..."
for i in {1..30}; do
    if docker exec crossspread-redis redis-cli ping 2>/dev/null | grep -q PONG; then
        echo "  Redis ready"
        break
    fi
    sleep 1
done

# 2. Backend
echo "[2/5] Starting Backend..."
cd services/backend-api
npm install --silent 2>/dev/null || true
npx prisma generate 2>/dev/null || true
npx prisma db push --accept-data-loss 2>/dev/null || true
npm run build 2>/dev/null || true
lsof -ti :8000 | xargs kill -9 2>/dev/null || true
node dist/main.js > ../../logs/backend.log 2>&1 &
echo $! > ../../.pids/backend.pid
cd ../..

# Wait for backend
echo "  Waiting for Backend..."
for i in {1..30}; do
    if curl -s http://localhost:8000/api/v1/exchanges > /dev/null 2>&1; then
        echo "  Backend ready"
        break
    fi
    sleep 1
done

# 3. md-ingest
echo "[3/5] Starting md-ingest..."
cd services/md-ingest
go build -o ingest ./cmd/ingest 2>/dev/null
pkill -f "./ingest" 2>/dev/null || true
./ingest > ../../logs/md-ingest.log 2>&1 &
echo $! > ../../.pids/md-ingest.pid
cd ../..

# 4. Frontend
echo "[4/5] Starting Frontend..."
cd web/web-frontend
npm install --silent 2>/dev/null || true
lsof -ti :3000 | xargs kill -9 2>/dev/null || true
npm run dev > ../../logs/frontend.log 2>&1 &
echo $! > ../../.pids/frontend.pid
cd ../..

# 5. Done
echo "[5/5] Done!"
echo ""
echo "Frontend: http://localhost:3000"
echo "Backend:  http://localhost:8000"
echo ""
echo "Logs in ./logs/"
echo "Stop with: ./stop.sh"
