# Gravity - Prompt-DAG Observability System

Offline-first, compression-optimized LLM observability. See `docs/data_flow.md` for architecture.

## Quick Start

```bash
# Start dev environment and run compression worker
make dev

# Or manually:
make build           # Build worker binary
cd dev && docker-compose up -d  # Start MinIO, LiteLLM, OTel Collector
./scripts/dev.sh     # Run worker

# Inspect results
make inspect

# Stop everything
make stop
```

## Architecture

```
LiteLLM → OTel Collector → MinIO (raw-spans/) → Worker → MinIO (blobs/ + indexes/)
          ↓                 ↓                     ↓
       Jaeger        OTLP JSON (gzip)    Content-addressed (BLAKE3 + gzip)
```

**Compression Worker:**
1. Poll `s3://traces/raw-spans/` every 10s
2. Parse OTLP JSON → extract prompt content
3. Chunk (newlines) → hash (BLAKE3) → compress (gzip)
4. Store deduplicated blobs: `s3://traces/blobs/{hash}.gz`
5. Store indexes: `s3://traces/indexes/{trace_id}.json`

**Dev Stack** (`dev/`):
- MinIO (S3): http://localhost:9000 (console: 9001)
- LiteLLM Proxy: http://localhost:4000
- Jaeger UI: http://localhost:16686
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3001

## Configuration

Worker environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `S3_BUCKET` | *required* | S3 bucket name |
| `S3_REGION` | `us-west-2` | AWS region |
| `POLL_INTERVAL` | `30s` | Polling frequency |

Full config: `internal/worker/config.go`

## Output

**Blobs**: `s3://bucket/blobs/{hash[0:2]}/{hash}.gz`
Content-addressed chunks (BLAKE3 hash, gzip compressed)

**Indexes**: `s3://bucket/indexes/{trace_id}.json`
```json
{
  "trace_id": "...",
  "span_id": "...",
  "hashes": ["hash1", "hash2", "hash3"]
}
```

## Roadmap

**MVP ✅**
- [x] OTLP parsing and content extraction
- [x] Newline-based chunking
- [x] BLAKE3 hashing + gzip compression
- [x] S3 storage with idempotency

**Next**
1. Tokenization (tiktoken/o200k_base)
2. Zstd compression with dictionaries
3. S3 event triggers (SQS)
4. Reconstruction tool
5. Prometheus metrics
