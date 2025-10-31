#!/usr/bin/env python3
"""
Jaeger Span Query Tool
Query spans from Jaeger by trace ID and span name
"""

import requests
import json
import argparse
from typing import Dict, List, Optional


class JaegerQueryTool:
    def __init__(self, jaeger_url: str = "http://localhost:16686"):
        self.jaeger_url = jaeger_url.rstrip('/')

    def get_trace(self, trace_id: str) -> Optional[Dict]:
        """Get a complete trace by trace ID"""
        url = f"{self.jaeger_url}/api/traces/{trace_id}"

        try:
            response = requests.get(url)
            response.raise_for_status()
            data = response.json()

            if 'data' in data and len(data['data']) > 0:
                return data['data'][0]
            return None
        except requests.exceptions.RequestException as e:
            print(f"Error fetching trace: {e}")
            return None

    def find_span_by_name(self, spans: List[Dict], span_name: str) -> Optional[Dict]:
        """Find a span by operation name"""
        for span in spans:
            if span.get('operationName') == span_name:
                return span
        return None

    def get_child_spans(self, parent_span_id: str, all_spans: List[Dict]) -> List[Dict]:
        """Get all child spans of a parent span"""
        children = []

        for span in all_spans:
            if 'references' in span:
                for ref in span['references']:
                    if ref.get('refType') == 'CHILD_OF' and ref.get('spanID') == parent_span_id:
                        children.append(span)
                        # Recursively get children of this span
                        children.extend(self.get_child_spans(span['spanID'], all_spans))

        return children

    def build_span_tree(self, root_span: Dict, all_spans: List[Dict]) -> Dict:
        """Build a span tree with the root span and all its descendants"""
        span_tree = root_span.copy()

        # Get all child spans
        children = self.get_child_spans(root_span['spanID'], all_spans)

        if children:
            span_tree['childSpans'] = children
            span_tree['childCount'] = len(children)

        return span_tree

    def format_span_output(self, span: Dict, indent: int = 0) -> str:
        """Format span for readable output"""
        prefix = "  " * indent
        output = []

        # Basic info
        output.append(f"{prefix}Operation: {span.get('operationName')}")
        output.append(f"{prefix}Span ID: {span.get('spanID')}")
        output.append(f"{prefix}Duration: {span.get('duration', 0) / 1000:.2f}ms")
        output.append(f"{prefix}Start Time: {span.get('startTime')}")

        # Tags
        if 'tags' in span and span['tags']:
            output.append(f"{prefix}Tags:")
            for tag in span['tags']:
                key = tag.get('key')
                value = tag.get('value', '')
                output.append(f"{prefix}  {key}: {value}")

        # Logs
        if 'logs' in span and span['logs']:
            output.append(f"{prefix}Logs: {len(span['logs'])} entries")

        # Child spans
        if 'childSpans' in span:
            output.append(f"{prefix}Child Spans: {len(span['childSpans'])}")
            for child in span['childSpans']:
                output.append("")
                output.append(self.format_span_output(child, indent + 1))

        return "\n".join(output)

    def query_span(self, trace_id: str, span_name: str, output_format: str = 'json') -> Optional[Dict]:
        """
        Query a span and its children by trace ID and span name

        Args:
            trace_id: The trace ID
            span_name: The operation name of the span
            output_format: Output format ('json' or 'pretty')

        Returns:
            Span tree as dictionary or None if not found
        """
        # Get the trace
        trace = self.get_trace(trace_id)

        if not trace:
            print(f"Trace not found: {trace_id}")
            return None

        if 'spans' not in trace:
            print("No spans found in trace")
            return None

        all_spans = trace['spans']

        # Find the target span
        target_span = self.find_span_by_name(all_spans, span_name)

        if not target_span:
            print(f"Span not found: {span_name}")
            print(f"\nAvailable spans in this trace:")
            for span in all_spans:
                print(f"  - {span.get('operationName')}")
            return None

        # Build span tree with children
        span_tree = self.build_span_tree(target_span, all_spans)

        # Output based on format
        if output_format == 'pretty':
            print("\n=== Span Tree ===\n")
            print(self.format_span_output(span_tree))
            print("\n")
        else:
            print(json.dumps(span_tree, indent=2))

        return span_tree


def main():
    parser = argparse.ArgumentParser(
        description='Query Jaeger spans by trace ID and span name',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Query span and output JSON
  python query_span.py --trace-id abc123 --span-name "POST /chat/completions"

  # Query span with pretty output
  python query_span.py --trace-id abc123 --span-name "POST /chat/completions" --format pretty

  # Query from custom Jaeger URL
  python query_span.py --trace-id abc123 --span-name "my-operation" --url http://jaeger:16686

  # Save output to file
  python query_span.py --trace-id abc123 --span-name "my-operation" --output output.json
        """
    )

    parser.add_argument(
        '--trace-id',
        required=True,
        help='Trace ID to query'
    )

    parser.add_argument(
        '--span-name',
        required=True,
        help='Span operation name to find'
    )

    parser.add_argument(
        '--url',
        default='http://localhost:16686',
        help='Jaeger UI URL (default: http://localhost:16686)'
    )

    parser.add_argument(
        '--format',
        choices=['json', 'pretty'],
        default='json',
        help='Output format (default: json)'
    )

    parser.add_argument(
        '--output',
        help='Output file path (optional, prints to stdout if not specified)'
    )

    args = parser.parse_args()

    # Create query tool
    tool = JaegerQueryTool(jaeger_url=args.url)

    # Query the span
    span_tree = tool.query_span(args.trace_id, args.span_name, args.format)

    # Save to file if specified
    if args.output and span_tree:
        with open(args.output, 'w') as f:
            json.dump(span_tree, f, indent=2)
        print(f"\nOutput saved to: {args.output}")


if __name__ == "__main__":
    main()
