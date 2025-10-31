#!/bin/bash
# Start development services (MinIO, LiteLLM, OTel Collector)

set -e
cd "$(dirname "$0")/.."

echo "=== Gravity Development Services ==="
echo ""

# Stop any existing containers first to avoid conflicts
echo "Stopping any existing containers..."
cd dev
docker-compose down 2>/dev/null || true
cd ..

echo ""
# Start dev stack
echo "Starting dev stack (MinIO, LiteLLM, OTel Collector)..."
cd dev
docker-compose up -d
cd ..

echo ""
echo "Waiting for services to be ready..."
sleep 3

echo ""
echo "✓ Services running:"
echo "  MinIO Console:  http://localhost:9001 (minioadmin/minioadmin)"
echo "  LiteLLM:        http://localhost:4000"
echo "  Jaeger UI:      http://localhost:16686"
echo "  Prometheus:     http://localhost:9090"
echo "  Grafana:        http://localhost:3001 (admin/admin)"
echo ""
echo "Services are ready. Run './scripts/run-worker.sh' to start the worker."
echo ""
