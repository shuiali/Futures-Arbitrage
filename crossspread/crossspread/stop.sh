#!/bin/bash
cd "$(dirname "$0")"

echo "=== CrossSpread Stop ==="

# Stop by PID files
for f in .pids/*.pid; do
    if [ -f "$f" ]; then
        pid=$(cat "$f")
        kill -9 $pid 2>/dev/null && echo "Stopped PID $pid"
        rm -f "$f"
    fi
done

# Kill by port
lsof -ti :8000 | xargs kill -9 2>/dev/null || true
lsof -ti :3000 | xargs kill -9 2>/dev/null || true

# Kill md-ingest and node processes
pkill -f "./ingest" 2>/dev/null || true
pkill -f "node dist/main.js" 2>/dev/null || true
pkill -f "vite" 2>/dev/null || true

# Docker - keep running by default (data preserved)
# Uncomment next line to also stop Docker:
# docker-compose -f infra/docker-compose.yml down 2>/dev/null || true

echo "All services stopped."
echo "(Docker containers still running - run 'docker-compose -f infra/docker-compose.yml down' to stop)"
