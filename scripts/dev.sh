#!/bin/bash
# Start development environment and run worker
# This is a convenience script that calls start-services.sh and run-worker.sh

set -e
cd "$(dirname "$0")/.."

# Start services
./scripts/start-services.sh

# Run worker
./scripts/run-worker.sh
