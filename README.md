# Gravity - Prompt-DAG Observability System

## Project Structure

```
gravity/
├── cmd/
│   ├── ingest-http/           # OTLP/HTTP ingestion service (collector target, planned)
│   ├── prompt-worker/         # Compression/deduplication worker
│   ├── reconstructor/         # Prompt rehydration CLI (node_ids → S3 → decode)
│   └── admin-cli/             # Admin tooling for backfills, repairs
│
├── internal/
│   ├── ingest/                # OTLP/HTTP decoding → envelope batching
│   │   ├── http/              # `/v1/traces` + `/v1/metrics` handlers
│   │   └── pipeline/          # Batching, queue sinks, retries
│   │
│   ├── worker/                # Worker components
│   │   ├── processor/         # Normalize → tokenize → chunk → hash
│   │   ├── storage/           # Blob uploader (S3/MinIO) + idempotent logic
│   │   ├── indexer/           # Postgres/ClickHouse PromptNode graph updates
│   │   ├── metrics/           # Trace metric derivation + OTLP feedback
│   │   └── wal/               # Optional short-ttl raw envelope WAL (planned)
│   │
│   ├── dag/                   # Core Prompt-DAG domain logic (isolated)
│   │   ├── node.go            # PromptNode model
│   │   ├── edge.go            # DAG edges
│   │   └── builder.go         # Build/merge DAG
│   │
│   ├── queue/                 # Queue abstraction layer (explicit interfaces)
│   │   ├── queue.go           # Consumer/Producer interfaces
│   │   ├── kafka/             # Kafka implementation
│   │   └── sqs/               # AWS SQS implementation
│   │
│   ├── shared/                # Cross-cutting concerns
│   │   ├── config/            # Config loading/validation
│   │   ├── logging/           # Structured logging
│   │   └── observability/     # Tracing, metrics instrumentation
│   │
│   └── backfill/              # Backfill routines (CLI/cron)
│
├── pkg/
│   └── promptdag/             # Stable public library (schemas, client)
│
├── db/migrations/             # Database migrations
├── analytics/                 # Offline analytics queries, notebooks (planned)
├── scripts/                   # Build, dev, and test scripts
│   ├── localstack.sh
│   └── bench.sh
├── deploy/                    # Deployment configurations
│   ├── docker/
│   ├── k8s/
│   └── terraform/
├── test/
│   ├── integration/           # End-to-end tests
│   └── load/                  # Performance tests
├── docs/contracts/            # Data contracts for envelopes, frames, indexes
└── docs/runbooks/             # Operational playbooks
```

## Otel

- Configure the OTel Collector with OTLP/HTTP exporters pointing to the Gravity ingest service (`POST /v1/traces` / `/v1/metrics`); the service converts spans to envelopes and pushes them onto the queue.

- span spec /Users/lixin/code/temp/gateway/openinference/spec
