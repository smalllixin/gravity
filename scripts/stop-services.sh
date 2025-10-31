#!/bin/bash
# Stop and remove development services

set -e
cd "$(dirname "$0")/.."

echo "=== Stopping Gravity Development Services ==="
echo ""

cd dev
docker-compose down
cd ..

echo ""
echo "✓ Services stopped and removed"
echo ""
