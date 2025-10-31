package worker

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// OTLPSpan represents the structure we care about from OTLP JSON
// This is a minimal structure focusing only on what we need for compression
type OTLPData struct {
	ResourceSpans []ResourceSpan `json:"resourceSpans"`
}

type ResourceSpan struct {
	ScopeSpans []ScopeSpan `json:"scopeSpans"`
}

type ScopeSpan struct {
	Spans []Span `json:"spans"`
}

type Span struct {
	TraceID    string      `json:"traceId"`
	SpanID     string      `json:"spanId"`
	Name       string      `json:"name"`
	Attributes []Attribute `json:"attributes"`
	Events     []Event     `json:"events"`
}

type Attribute struct {
	Key   string     `json:"key"`
	Value ValueUnion `json:"value"`
}

type Event struct {
	Name       string      `json:"name"`
	Attributes []Attribute `json:"attributes"`
}

// ValueUnion represents the possible value types in OTLP
// Note: intValue can be either a number or string in JSON (Python encodes large ints as strings)
type ValueUnion struct {
	StringValue string      `json:"stringValue,omitempty"`
	IntValue    interface{} `json:"intValue,omitempty"` // Can be int64 or string
	DoubleValue float64     `json:"doubleValue,omitempty"`
	BoolValue   bool        `json:"boolValue,omitempty"`
}

// ExtractedContent represents content extracted from a single span
type ExtractedContent struct {
	TraceID string
	SpanID  string
	Content string // Full prompt text to be compressed
}

// ParseOTLPFile parses a gzipped OTLP JSON file and extracts prompt content
func ParseOTLPFile(reader io.Reader) ([]ExtractedContent, error) {
	// Decompress gzip
	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Parse JSON
	var otlpData OTLPData
	decoder := json.NewDecoder(gzReader)
	if err := decoder.Decode(&otlpData); err != nil {
		return nil, fmt.Errorf("failed to decode OTLP JSON: %w", err)
	}

	// Extract content from spans
	var results []ExtractedContent

	for _, resourceSpan := range otlpData.ResourceSpans {
		for _, scopeSpan := range resourceSpan.ScopeSpans {
			for _, span := range scopeSpan.Spans {
				content := extractContentFromSpan(span)
				if content != "" {
					results = append(results, ExtractedContent{
						TraceID: span.TraceID,
						SpanID:  span.SpanID,
						Content: content,
					})
				}
			}
		}
	}

	return results, nil
}

// extractContentFromSpan extracts the prompt/message content from a span
// For MVP, we extract from:
// 1. span.events with name "llm.request" or "llm.response"
// 2. span.attributes like "llm.input_messages" or "input.value"
func extractContentFromSpan(span Span) string {
	var parts []string

	// Extract from span events (llm.request, llm.response)
	for _, event := range span.Events {
		if event.Name == "llm.request" || event.Name == "llm.response" {
			// Look for "body" attribute in event
			for _, attr := range event.Attributes {
				if attr.Key == "body" && attr.Value.StringValue != "" {
					parts = append(parts, attr.Value.StringValue)
				}
			}
		}
	}

	// Extract from span attributes
	// Look for message content patterns
	for _, attr := range span.Attributes {
		// Handle input.value, output.value
		if (attr.Key == "input.value" || attr.Key == "output.value") && attr.Value.StringValue != "" {
			parts = append(parts, attr.Value.StringValue)
		}

		// Handle flattened message attributes (llm.input_messages.N.message.content)
		if strings.Contains(attr.Key, "message.content") && attr.Value.StringValue != "" {
			parts = append(parts, attr.Value.StringValue)
		}
	}

	// Join all parts with newlines
	return strings.Join(parts, "\n")
}
