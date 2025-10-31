#!/bin/bash
# Inspect compression worker results

export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export AWS_ENDPOINT_URL=http://localhost:9000

echo "=== Compression Worker Results ==="
echo ""

RAW=$(aws s3 ls s3://traces/raw-spans/ --recursive 2>/dev/null | wc -l)
BLOBS=$(aws s3 ls s3://traces/blobs/ --recursive 2>/dev/null | wc -l)
INDEXES=$(aws s3 ls s3://traces/indexes/ --recursive 2>/dev/null | wc -l)

echo "Statistics:"
echo "  Raw spans:  $RAW"
echo "  Blobs:      $BLOBS (compressed chunks)"
echo "  Indexes:    $INDEXES (trace mappings)"
echo ""

if [ "$INDEXES" -gt 0 ]; then
    LATEST=$(aws s3 ls s3://traces/indexes/ --recursive 2>/dev/null | tail -1 | awk '{print $4}')
    echo "Latest index ($LATEST):"
    aws s3 cp s3://traces/$LATEST - 2>/dev/null | jq
    echo ""

    # Show first chunk
    HASH=$(aws s3 cp s3://traces/$LATEST - 2>/dev/null | jq -r '.hashes[0]')
    if [ -n "$HASH" ] && [ "$HASH" != "null" ]; then
        PREFIX="${HASH:0:2}"
        echo "First chunk content:"
        aws s3 cp s3://traces/blobs/$PREFIX/$HASH.gz - 2>/dev/null | gunzip | head -c 300
        echo "..."
        echo ""
    fi
fi

echo "MinIO Console: http://localhost:9001"
