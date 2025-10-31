# Gravity - Prompt-DAG Observability System

Offline-first, compression-optimized LLM observability.

**📖 Documentation:**
- [MVP Implementation Guide](docs/mvp.md) - Current working implementation
- [Architecture Vision](docs/data_flow.md) - Future goals and design philosophy

## Quick Start

```bash
# Option 1: All-in-one (start services + worker)
make dev

# Option 2: Separate services and worker
make build                    # Build worker binary
./scripts/start-services.sh   # Start MinIO, LiteLLM, OTel Collector
./scripts/run-worker.sh       # Run worker (in separate terminal)

# Inspect compression results
make inspect

# Stop services
./scripts/stop-services.sh
# or: make stop
```

## Architecture (MVP)

```
LiteLLM → OTel Collector → MinIO (raw-spans/) → Worker → MinIO (blobs/ + indexes/)
          ↓                 ↓                     ↓
       Jaeger        OTLP JSON (gzip)    Content-addressed (BLAKE3 + gzip)
```

**Compression Worker (Current MVP):**
1. Poll `s3://traces/raw-spans/` every 10s
2. Parse OTLP JSON (gzip) → extract prompt content from span events
3. Chunk by newlines → hash with BLAKE3 → compress with gzip
4. Store deduplicated blobs: `s3://traces/blobs/{hash[0:2]}/{hash}.gz`
5. Store indexes: `s3://traces/indexes/{trace_id}.json`
6. Achieves ~7-8x compression ratio with deduplication

→ **[Full MVP implementation details](docs/mvp.md)**

**Dev Stack** (`dev/`):
- MinIO (S3): http://localhost:9000 (console: 9001)
- LiteLLM Proxy: http://localhost:4000
- Jaeger UI: http://localhost:16686
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3001

## Configuration

Worker environment variables (see `./scripts/run-worker.sh`):

| Variable | Default | Description |
|----------|---------|-------------|
| `S3_BUCKET` | *required* | S3 bucket name (e.g., `traces`) |
| `S3_REGION` | `us-east-1` | AWS region |
| `AWS_ENDPOINT_URL` | - | S3 endpoint (use `http://localhost:9000` for MinIO) |
| `AWS_ACCESS_KEY_ID` | - | AWS credentials (MinIO: `minioadmin`) |
| `AWS_SECRET_ACCESS_KEY` | - | AWS credentials (MinIO: `minioadmin`) |
| `POLL_INTERVAL` | `30s` | Polling frequency |
| `MAX_CONCURRENT` | `5` | Max concurrent file processing |

Full config: `internal/worker/config.go`

## Output

**Blobs**: `s3://traces/blobs/{hash[0:2]}/{hash}.gz`
- Content-addressed chunks (BLAKE3 hash, gzip compressed)
- Deduplicated globally (same content = same hash = stored once)
- Example: `blobs/3d/3d566b5e4b7739fa00c75389569bf47691fecc86e81081fc3503f1c446817f1e.gz`

**Indexes**: `s3://traces/indexes/{trace_id}.json`
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

**To verify compression:**
```bash
make inspect  # Shows compression ratio and storage breakdown
```

**To reconstruct original content from blobs:**
```bash
# List available traces
./scripts/inspect.sh

# Reconstruct a specific trace
make reconstruct TRACE_ID=44e0c73c00b2914b0b08945fd2665935
# or: ./scripts/reconstruct.sh 44e0c73c00b2914b0b08945fd2665935
```

The reconstruction tool:
- Downloads the index for the trace
- Fetches all referenced blobs from S3
- Decompresses each blob (gzip)
- Concatenates them in order to recover the original content
- Demonstrates lossless compression

## Roadmap

**MVP ✅ (Completed)**
- [x] OTLP parsing and content extraction
- [x] Newline-based chunking
- [x] BLAKE3 hashing + gzip compression
- [x] S3 storage with idempotency
- [x] Content deduplication (7-8x compression)
- [x] Simple polling mechanism
- [x] MinIO dev environment
- [x] Reconstruction tool (lossless recovery from blobs)

**Next Phase**
1. **Tokenization**: tiktoken/o200k_base for token-level chunking
2. **Better compression**: Zstd with per-model dictionaries
3. **Event-driven**: S3 event notifications → SQS for lower latency
4. **Structured indexes**: Parquet/ClickHouse for efficient queries
5. **Metrics**: Prometheus metrics for compression ratio, processing lag
6. **Multi-tenancy**: Org-based partitioning and isolation
7. **Web UI**: Simple dashboard for viewing traces and metrics
