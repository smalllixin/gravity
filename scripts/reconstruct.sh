#!/bin/bash
# Reconstruct original content from compressed blobs

set -e
cd "$(dirname "$0")/.."

if [ -z "$1" ]; then
    echo "Usage: $0 <trace_id>"
    echo ""
    echo "Example:"
    echo "  $0 44e0c73c00b2914b0b08945fd2665935"
    echo ""
    echo "To list available traces:"
    echo "  AWS_ACCESS_KEY_ID=minioadmin AWS_SECRET_ACCESS_KEY=minioadmin AWS_ENDPOINT_URL=http://localhost:9000 aws s3 ls s3://traces/indexes/ --region us-east-1"
    exit 1
fi

TRACE_ID="$1"

# Configure AWS for MinIO
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export AWS_ENDPOINT_URL=http://localhost:9000
export AWS_REGION=us-east-1

# Reconstruction config
export S3_BUCKET=traces
export S3_REGION=us-east-1

echo "=== Gravity Content Reconstruction ==="
echo ""

./bin/reconstruct "$TRACE_ID"
