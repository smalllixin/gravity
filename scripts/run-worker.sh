#!/bin/bash
# Run the Gravity worker with development configuration

set -e
cd "$(dirname "$0")/.."

# Configure AWS for MinIO
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export AWS_ENDPOINT_URL=http://localhost:9000
export AWS_REGION=us-east-1
export AWS_S3_FORCE_PATH_STYLE=true

# Worker config
export S3_BUCKET=traces
export S3_REGION=us-east-1
export POLL_INTERVAL=10s

echo "=== Gravity Worker ==="
echo ""
echo "Starting worker..."
echo "Press Ctrl+C to stop"
echo ""

./bin/worker
