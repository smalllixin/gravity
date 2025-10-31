# Jaeger Span Query Tool

Query spans from Jaeger by trace ID and span name, including all child spans.

## Installation

```bash
pip install requests
```

## Usage

### Basic Query (JSON output)

```bash
python query_span.py --trace-id <TRACE_ID> --span-name "<SPAN_NAME>"
```

### Pretty Print Output

```bash
python query_span.py --trace-id <TRACE_ID> --span-name "<SPAN_NAME>" --format pretty
```

### Save to File

```bash
python query_span.py --trace-id <TRACE_ID> --span-name "<SPAN_NAME>" --output output.json
```

### Custom Jaeger URL

```bash
python query_span.py --trace-id <TRACE_ID> --span-name "<SPAN_NAME>" --url http://jaeger:16686
```

## Examples

### Example 1: Query a specific span

```bash
python query_span.py \
  --trace-id "abc123def456" \
  --span-name "POST /chat/completions"
```

### Example 2: Pretty print with all child spans

```bash
python query_span.py \
  --trace-id "abc123def456" \
  --span-name "POST /chat/completions" \
  --format pretty
```

### Example 3: Save output to file

```bash
python query_span.py \
  --trace-id "abc123def456" \
  --span-name "chat_completion" \
  --output span_output.json
```

## Features

- Query spans by trace ID and span name
- Automatically retrieves all child spans (sub-spans)
- Two output formats:
  - `json`: Raw JSON output (default)
  - `pretty`: Human-readable formatted output
- Save results to file
- Shows span details: operation name, duration, tags, logs
- Displays span hierarchy with parent-child relationships

## Output Structure

The JSON output includes:
- All original span fields
- `childSpans`: Array of child spans (if any)
- `childCount`: Number of direct child spans

Example output structure:
```json
{
  "traceID": "abc123",
  "spanID": "span123",
  "operationName": "POST /chat/completions",
  "startTime": 1698000000000000,
  "duration": 150000,
  "tags": [...],
  "logs": [...],
  "childSpans": [
    {
      "spanID": "child1",
      "operationName": "database_query",
      "duration": 50000,
      ...
    }
  ],
  "childCount": 1
}
```

## Help

```bash
python query_span.py --help
```
