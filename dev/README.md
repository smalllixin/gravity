# How to start

rename litellm-config.yaml to config.yaml then fill u keys

## Litellm Examples

Here's the litellm test key
`sk-7lp7bTgEEqoR1_zTSWVu_A`


Here's the example to use key


```bash
curl -X POST "http://localhost:4000/v1/chat/completions" \
  -H "Authorization: Bearer sk-7lp7bTgEEqoR1_zTSWVu_A" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "azure/gpt-5-codex",
    "messages": [
      { "role": "user", "content": "hello world" },
      { "role": "assistant", "content": "Hello there! 👋 How can I help you today?" },
      { "role": "user", "content": "good day" }
    ]
  }'
```

```python
import openai

client = openai.OpenAI(
	api_key="sk-7lp7bTgEEqoR1_zTSWVu_A",
	base_url="http://localhost:4000"
)

import base64

# Helper function to encode images to base64
def encode_image(image_path):
    with open(image_path, "rb") as image_file:
        return base64.b64encode(image_file.read()).decode('utf-8')

# Example with text only
response = client.chat.completions.create(
    model="azure/gpt-5-codex",
    messages=[
    {
        "role": "user",
        "content": "hello world"
    },
    {
        "role": "assistant",
        "content": "Hello there! 👋 How can I help you today?"
    },
    {
        "role": "user",
        "content": "good"
    },
    {
        "role": "assistant",
        "content": "Great! Is there anything specific you’d like to talk about or need help with today?"
    }
]
)

print(response)

# Example with image or PDF (uncomment and provide file path to use)
# base64_file = encode_image("path/to/your/file.jpg")  # or .pdf
# response_with_file = client.chat.completions.create(
#     model="azure/gpt-5-codex",
#     messages=[
#         {
#             "role": "user",
#             "content": [
#                 {
#                     "type": "text",
#                     "text": "Your prompt here"
#                 },
#                 {
#                     "type": "image_url",
#                     "image_url": {
#                         "url": f"data:image/jpeg;base64,{base64_file}"  # or data:application/pdf;base64,{base64_file}
#                     }
#                 }
#             ]
#         }
#     ]
# )
# print(response_with_file)
```

---

## Observability with OpenTelemetry

### Start all services
```bash
docker compose up -d
```

### Access UIs

- **Litellm**: http://localhost:4000
- **Jaeger (Traces)**: http://localhost:16686 - View request traces and spans
- **Prometheus (Metrics)**: http://localhost:9090 - Query raw metrics
- **Loki (Logs)**: http://localhost:3100 - Log aggregation system
- **Grafana (Dashboards)**: http://localhost:3001 - Visualize metrics, traces, and logs
  - Username: `admin`
  - Password: `admin`

### How it works

1. Litellm sends traces, metrics, and logs to OTEL Collector (gRPC port 4317)
2. OTEL Collector exports:
   - **Traces** → Jaeger for visualization
   - **Metrics** → Prometheus (scraped every 15s)
   - **Logs** → Loki for log aggregation
3. Grafana can query Prometheus, Jaeger, and Loki for unified observability

**Setup data sources in Grafana:**

1. Open Grafana at http://localhost:3001
2. Login with `admin` / `admin`
3. Go to **Configuration** (⚙️) → **Data Sources** → **Add data source**
4. Add **Prometheus**:
   - URL: `http://prometheus:9090`
   - Click **Save & Test**
5. Add **Jaeger**:
   - URL: `http://jaeger:16686`
   - Click **Save & Test**
6. Add **Loki**:
   - URL: `http://loki:3100`
   - Click **Save & Test**

**Example dashboard ideas:**
- LLM requests per minute (query: `rate(litellm_gen_ai_client_operation_duration_count[5m])`)
- Average token usage by model
- Cost breakdown over time
- Request latency percentiles (p50, p95, p99)
- Error rates and failed requests

**Quick PromQL examples:**
```promql
# Total token usage rate
rate(litellm_gen_ai_client_token_usage_sum[5m])

# Average request duration
rate(litellm_gen_ai_client_operation_duration_sum[5m]) / rate(litellm_gen_ai_client_operation_duration_count[5m])

# Cost per hour
rate(litellm_gen_ai_client_token_cost_sum[1h])
```

### Viewing Logs in Grafana

After setting up the Loki data source, you can view logs in Grafana:

1. Go to **Explore** (🧭 compass icon in the left sidebar)
2. Select **Loki** from the data source dropdown
3. Use LogQL queries to filter and search logs

**Quick LogQL examples:**
```logql
# All logs from litellm
{job="litellm"}

# Filter by service name
{service_name="litellm"}

# Only error logs
{level="error"}

# Search for specific text in logs
{job="litellm"} |= "error"

# Filter by time range and error level
{service_name="litellm"} | json | level="ERROR"
```

**Tips for viewing logs:**
- Use the **Live** toggle for real-time log streaming
- Combine with metrics: Click on a trace in Jaeger to see related logs
- Use **Labels** browser to discover available log attributes
- Create alerts based on log patterns (e.g., error rate threshold)
