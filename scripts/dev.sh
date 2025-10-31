#!/bin/bash
# Start development environment and run worker

set -e
cd "$(dirname "$0")/.."

echo "=== Gravity Development Environment ==="
echo ""

# Start dev stack
echo "Starting dev stack (MinIO, LiteLLM, OTel Collector)..."
cd dev
docker-compose up -d
cd ..

echo ""
echo "Waiting for services to be ready..."
sleep 3

# Configure AWS for MinIO
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export AWS_ENDPOINT_URL=http://localhost:9000
export AWS_REGION=us-east-1

# Worker config
export S3_BUCKET=traces
export S3_REGION=us-east-1
export POLL_INTERVAL=10s

echo ""
echo "✓ Services running:"
echo "  MinIO Console:  http://localhost:9001 (minioadmin/minioadmin)"
echo "  LiteLLM:        http://localhost:4000"
echo "  Jaeger UI:      http://localhost:16686"
echo "  Prometheus:     http://localhost:9090"
echo "  Grafana:        http://localhost:3001 (admin/admin)"
echo ""
echo "Starting worker..."
echo "Press Ctrl+C to stop"
echo ""

./bin/worker
