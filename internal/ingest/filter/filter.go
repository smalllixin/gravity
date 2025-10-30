package filter

// Config defines which spans to process based on name and OpenInference span kind.
//
// Filtering Behavior:
// - Spans are first filtered by name (SpanNames)
// - If a span passes name filter but has no openinference.span.kind attribute,
//   it will NOT be processed (e.g., vendor-specific instrumentation spans like llm.azure.*)
// - Only spans with valid OpenInference span kinds (SpanKinds) are processed
//
// This ensures we only process standardized OpenInference traces and ignore
// vendor-specific metadata spans that don't conform to the spec.
type Config struct {
	SpanNames []string
	SpanKinds []string // OpenInference span kinds to process
}

// Default returns the default filter configuration
func Default() *Config {
	return &Config{
		SpanNames: []string{
			"litellm_request",
			"raw_gen_ai_request",
		},
		// OpenInference span kinds we're interested in
		// Note: Spans without openinference.span.kind will be filtered out
		SpanKinds: []string{
			"LLM",
			"EMBEDDING",
			"CHAIN",
			"RETRIEVER",
			"RERANKER",
			"TOOL",
			"AGENT",
		},
	}
}

// ShouldProcess checks if a span should be processed based on its name
func (c *Config) ShouldProcess(spanName string) bool {
	for _, name := range c.SpanNames {
		if name == spanName {
			return true
		}
	}
	return false
}

// ShouldProcessKind checks if a span should be processed based on its OpenInference span kind
func (c *Config) ShouldProcessKind(spanKind string) bool {
	// If no span kinds configured, allow all
	if len(c.SpanKinds) == 0 {
		return true
	}

	for _, kind := range c.SpanKinds {
		if kind == spanKind {
			return true
		}
	}
	return false
}
