# Gravity Ingest HTTP Server

## Build & Run

```bash
cd /Users/lixin/code/youware/gravity
go build -o bin/ingest-http ./cmd/ingest-http
./bin/ingest-http
```

Server listens on `http://localhost:8080`

## Integration with LiteLLM

Edit `/Users/lixin/code/temp/gateway/lite_proxy/collector-config.yaml`:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

exporters:
  debug:
    verbosity: normal
    sampling_initial: 5
    sampling_thereafter: 200

  otlp/jaeger:
    endpoint: jaeger:4317
    tls:
      insecure: true

  prometheus:
    endpoint: "0.0.0.0:8889"
    namespace: litellm

  otlphttp/loki:
    endpoint: http://loki:3100/otlp
    tls:
      insecure: true

  otlphttp/gravity:
    endpoint: http://host.docker.internal:8080  # Mac/Windows
    # endpoint: http://172.17.0.1:8080          # Linux
    compression: gzip
    headers:
      x-org-id: default
    tls:
      insecure: true
    retry_on_failure:
      enabled: true
      initial_interval: 1s
      max_interval: 30s

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug, otlp/jaeger, otlphttp/gravity]
    metrics:
      receivers: [otlp]
      exporters: [debug, prometheus]
    logs:
      receivers: [otlp]
      exporters: [debug, otlphttp/loki]
```

Restart collector:

```bash
cd /Users/lixin/code/temp/gateway/lite_proxy
docker-compose restart otel-collector
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `HTTP_ADDRESS` | `:8080` | Server listen address |
| `HTTP_READ_TIMEOUT` | `10s` | HTTP read timeout |
| `HTTP_WRITE_TIMEOUT` | `10s` | HTTP write timeout |
| `HTTP_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown timeout |

## Endpoints

- `GET /health` - Health check
- `GET /ready` - Readiness check
- `POST /v1/traces` - OTLP traces endpoint
- `POST /v1/metrics` - OTLP metrics endpoint

## OTLP Attribute Mapping

| OTLP Attribute | Envelope Field |
|----------------|----------------|
| `gen_ai.system` | `provider` |
| `gen_ai.request.model` | `model` |
| `gen_ai.usage.input_tokens` | `metrics.prompt_tokens` |
| `gen_ai.usage.output_tokens` | `metrics.completion_tokens` |
| span duration | `metrics.latency_ms` |
| span start time | `timestamp` |

Fallback attributes: `llm.provider`, `llm.model`, `llm.usage.*`

## Troubleshooting

Test connectivity from collector:
```bash
docker exec litellm_otel_collector curl http://host.docker.internal:8080/health
```

On Linux, if `host.docker.internal` fails, use Docker bridge IP:
```bash
ip addr show docker0 | grep inet
# Use http://172.17.0.1:8080 in collector config
```

Check collector logs:
```bash
docker logs litellm_otel_collector -f
```
