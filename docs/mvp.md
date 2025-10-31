# MVP Implementation Guide

This document describes the **current working implementation** of Gravity's compression worker. For the future architecture vision, see [data_flow.md](./data_flow.md).

## Current MVP Status

The MVP implements a simple but effective compression pipeline:

- **Architecture**: LiteLLM → OTel Collector → S3 (raw spans) → Worker (polling) → S3 (blobs + indexes)
- **Chunking**: Newline-based splitting (simple, fast)
- **Hashing**: BLAKE3 (64-character hex)
- **Compression**: gzip per chunk
- **Deduplication**: Content-addressed storage (same content = same blob)
- **Storage paths**:
  - Raw spans: `s3://traces/raw-spans/dt={date}/hour={hour}/batch-*.json.gz`
  - Blobs: `s3://traces/blobs/{hash[0:2]}/{hash}.gz`
  - Indexes: `s3://traces/indexes/{trace_id}.json`
- **Trigger**: Simple polling every 10s (no SQS)
- **Results**: ~7-8x compression ratio with deduplication
- **Reconstruction**: CLI tool to recover original content

---

## Architecture Diagram

```
┌─────────────┐
│  LiteLLM    │ (LLM proxy with OTLP instrumentation)
│   Proxy     │
└──────┬──────┘
       │ OTLP spans
       ↓
┌─────────────────┐
│ OTel Collector  │ (batch + tail_sampling)
│  with S3 exp.   │
└────────┬────────┘
         │ gzipped OTLP JSON (every 10s)
         ↓
┌────────────────────────┐
│  S3: raw-spans/        │
│  dt=2025-10-31/        │
│    hour=11/            │
│      batch-*.json.gz   │
└──────┬─────────────────┘
       │ poll every 10s
       ↓
┌─────────────────────┐
│ Compression Worker  │
│ (Go binary)         │
│                     │
│ 1. Parse OTLP JSON  │
│ 2. Extract content  │
│ 3. Chunk by \n      │
│ 4. BLAKE3 hash      │
│ 5. gzip compress    │
│ 6. Dedupe & store   │
└──────┬──────────────┘
       │
       ├─→ S3: blobs/{hash[0:2]}/{hash}.gz
       └─→ S3: indexes/{trace_id}.json

┌──────────────────┐
│ Reconstruction   │ (lossless recovery)
│      Tool        │
└──────────────────┘
```

---

## Quick Start

```bash
# Start all services
./scripts/start-services.sh

# Run worker (in another terminal)
./scripts/run-worker.sh

# Or use all-in-one
make dev

# Inspect compression results
make inspect

# Reconstruct a trace
make reconstruct TRACE_ID=<trace_id>
```

---

## Component Details

### 1. OTel Collector Configuration

See `dev/collector-config.yaml`:

**Key settings:**
- **Batch processor**: 10s timeout, 1000 spans per batch (for S3 pipeline)
- **Tail sampling**: Keeps only `litellm_request` and `raw_gen_ai_request` spans
- **S3 exporter**:
  - Marshaler: `otlp_json`
  - Compression: `gzip`
  - Partition: `dt=%Y-%m-%d/hour=%H`
  - File prefix: `batch-`

**Pipelines:**
- `traces/realtime`: No batching → Jaeger + Debug (for instant viewing)
- `traces/s3`: With batching → S3 (for compression worker)

### 2. Worker Implementation

**Core logic** (`internal/worker/processor.go`):

```go
// For each raw span file:
1. Download from S3 and decompress (gzip)
2. Parse OTLP JSON → extract span events with "body" attributes
3. For each content string:
   a. Split by newlines into chunks
   b. For each chunk:
      - Hash with BLAKE3 → 64-char hex
      - Compress with gzip
      - Check if blob exists (HEAD request)
      - Upload if missing (idempotent)
4. Create index JSON with ordered list of hashes
5. Upload index to S3
```

**Idempotency:**
- Uses `HEAD` before `PUT` to check blob existence
- Multiple workers can process same file safely
- Deduplication happens automatically via content addressing

**Configuration** (`internal/worker/config.go`):

| Variable | Default | Description |
|----------|---------|-------------|
| `S3_BUCKET` | *required* | S3 bucket name |
| `S3_REGION` | `us-east-1` | AWS region |
| `AWS_ENDPOINT_URL` | - | For MinIO |
| `POLL_INTERVAL` | `30s` | How often to check for new files |
| `MAX_CONCURRENT` | `5` | Concurrent file processing |
| `CHUNK_SEPARATOR` | `\n` | Splitting delimiter |

**Paths:**
- Raw spans: `{S3_BUCKET}/raw-spans/`
- Blobs: `{S3_BUCKET}/blobs/`
- Indexes: `{S3_BUCKET}/indexes/`

### 3. Storage Format

**Blob key format:**
```
blobs/{hash[0:2]}/{hash}.gz

Example:
blobs/3d/3d566b5e4b7739fa00c75389569bf47691fecc86e81081fc3503f1c446817f1e.gz
```

**Index format** (`indexes/{trace_id}.json`):
```json
{
  "trace_id": "44e0c73c00b2914b0b08945fd2665935",
  "span_id": "1ac2b947224f6e3b",
  "hashes": [
    "3486759456e006b120e16268f7bd885f5c860a9d0d49ea1e0e64622cd022420b",
    "3d566b5e4b7739fa00c75389569bf47691fecc86e81081fc3503f1c446817f1e",
    "d74981efa70a0c880b8d8c1985d075dbcbf679b99a5f9914e5aaf96b831a9e24"
  ]
}
```

### 4. Reconstruction Tool

**Usage:**
```bash
./scripts/reconstruct.sh <trace_id>
# or: make reconstruct TRACE_ID=<trace_id>
```

**Algorithm** (`cmd/reconstruct/main.go`):
```go
1. Download index from S3: indexes/{trace_id}.json
2. Parse JSON to get ordered list of hashes
3. For each hash:
   a. Download blob: blobs/{hash[0:2]}/{hash}.gz
   b. Decompress with gzip
   c. Append to result (with newline separator)
4. Output reconstructed content
```

**Example output:**
```
Reconstructed content for trace 44e0c73c00b2914b0b08945fd2665935 (span 1ac2b947224f6e3b):
================================================================================
good day
You are a helpful travel assistant. You are concise...
hello world
Hello there! 👋 How can I help you today?
...
================================================================================
✓ Successfully reconstructed 644 bytes from 9 chunks
```

---

## Compression Metrics

Typical results from MVP:

```
Original Data (raw-spans):
  Files:  3
  Size:   517,192 bytes (505 KB)

Compressed Storage:
  Blobs:   66 files → 31,919 bytes (31 KB)
  Indexes: 10 files → 33,934 bytes (33 KB)
  Total:   76 files → 65,853 bytes (64 KB)

Compression Ratio: 87.3%
Storage Efficiency: 12.7% of original size
Space Saved: 451,339 bytes (440 KB)
```

**Key observations:**
- **~7.8x compression** overall
- Deduplication is working (repeated content stored once)
- Small chunks have negative compression (gzip overhead), this is expected and will improve with zstd + larger chunks in future

---

## Development Workflow

### Local Setup

```bash
# Start MinIO + LiteLLM + OTel + Jaeger + Prometheus
./scripts/start-services.sh

# Build worker
make build

# Run worker
./scripts/run-worker.sh

# Stop services
./scripts/stop-services.sh
```

### Monitoring

**Service URLs:**
- MinIO Console: http://localhost:9001 (minioadmin/minioadmin)
- LiteLLM Proxy: http://localhost:4000
- Jaeger UI: http://localhost:16686
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3001 (admin/admin)

**Check worker logs:**
```bash
# Watch worker process traces
./scripts/run-worker.sh

# Check S3 contents
AWS_ACCESS_KEY_ID=minioadmin AWS_SECRET_ACCESS_KEY=minioadmin \
AWS_ENDPOINT_URL=http://localhost:9000 \
aws s3 ls s3://traces/ --recursive --region us-east-1
```

### Testing

**Generate test traces:**
```bash
# Use LiteLLM to make LLM requests
curl http://localhost:4000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Check OTel Collector received spans
docker logs litellm_otel_collector | grep "ExportTraceServiceRequest"

# Wait 10s for worker to process
# Check compression results
make inspect
```

---

## Limitations & Future Work

**Current MVP limitations:**
1. **Newline chunking** - Naive splitting, doesn't respect token boundaries
2. **Gzip compression** - Not optimal for small chunks (overhead)
3. **Polling** - 10s delay before processing (vs. instant with SQS)
4. **No structured queries** - Must download index JSON files to search
5. **Single-tenant** - No org_id isolation
6. **No metrics** - No Prometheus metrics for monitoring

**Planned improvements** (see [data_flow.md](./data_flow.md)):
- Token-level chunking with tiktoken
- Zstd compression with per-model dictionaries
- S3 event notifications → SQS
- Parquet/ClickHouse indexes for fast queries
- Multi-tenancy with org_id partitioning
- Prometheus metrics dashboard
- Web UI for trace exploration

---

## Troubleshooting

### Worker not processing files

```bash
# Check S3 connectivity
AWS_ACCESS_KEY_ID=minioadmin AWS_SECRET_ACCESS_KEY=minioadmin \
AWS_ENDPOINT_URL=http://localhost:9000 \
aws s3 ls s3://traces/raw-spans/ --region us-east-1

# Check worker logs for errors
./scripts/run-worker.sh

# Verify OTel Collector is writing to S3
docker logs litellm_otel_collector | grep "awss3"
```

### Blobs not deduplicating

- Check that identical content produces same hash
- Verify `HEAD` requests work (check S3 permissions)
- Review worker logs for "already exists, skipping" messages

### Reconstruction fails

```bash
# Verify index exists
AWS_ACCESS_KEY_ID=minioadmin AWS_SECRET_ACCESS_KEY=minioadmin \
AWS_ENDPOINT_URL=http://localhost:9000 \
aws s3 ls s3://traces/indexes/<trace_id>.json --region us-east-1

# Check blobs exist
aws s3 ls s3://traces/blobs/ --recursive | grep <hash>

# Verify gzip decompression works
aws s3 cp s3://traces/blobs/<prefix>/<hash>.gz - | gunzip
```

### MinIO bucket not created

```bash
# Check minio-setup container logs
docker logs litellm_minio_setup

# Manually create bucket
docker exec litellm_minio mc alias set local http://localhost:9000 minioadmin minioadmin
docker exec litellm_minio mc mb -p local/traces
docker exec litellm_minio mc ls local/
```

---

## Files Reference

**Core implementation:**
- `cmd/worker/main.go` - Worker entry point
- `cmd/reconstruct/main.go` - Reconstruction tool
- `internal/worker/worker.go` - Polling loop
- `internal/worker/processor.go` - Compression pipeline
- `internal/worker/config.go` - Configuration
- `internal/worker/parser.go` - OTLP JSON parsing

**Scripts:**
- `scripts/start-services.sh` - Start docker-compose stack
- `scripts/stop-services.sh` - Stop and remove containers
- `scripts/run-worker.sh` - Run worker with MinIO config
- `scripts/reconstruct.sh` - Reconstruct content from trace
- `scripts/inspect.sh` - Show compression statistics
- `scripts/dev.sh` - All-in-one: start services + worker

**Configuration:**
- `dev/docker-compose.yml` - Dev stack definition
- `dev/collector-config.yaml` - OTel Collector pipelines
- `dev/config.yaml` - LiteLLM proxy config
- `Makefile` - Build and run targets

---

## Performance Characteristics

**Throughput (local MinIO):**
- ~50-100 files/minute per worker
- Parallelizable (multiple workers can run concurrently)
- Bottleneck: S3 API latency (HEAD + PUT requests)

**Memory usage:**
- Worker: ~20-50 MB
- Processing overhead: ~2-3x file size during decompression/parsing

**Latency:**
- Poll interval: 10s (configurable)
- Processing time: ~100-500ms per file (depends on size)
- End-to-end: 10-15s from trace creation to blobs stored

---

## Next Steps

Once you're comfortable with the MVP:

1. **Read [data_flow.md](./data_flow.md)** - Understand the future architecture
2. **Explore tokenization** - Try chunking by tokens instead of newlines
3. **Test zstd** - Compare compression ratios with zstd dictionaries
4. **Add metrics** - Instrument worker with Prometheus metrics
5. **Try SQS** - Replace polling with event-driven processing

The MVP proves the core concept works. The future improvements will make it production-ready! 🚀
