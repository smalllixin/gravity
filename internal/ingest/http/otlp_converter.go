package http

import (
	"encoding/hex"
	"fmt"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/youware/gravity/internal/ingest/pipeline"
)

// convertOTLPToEnvelopes converts OTLP trace data to Gravity envelopes
func (h *Handler) convertOTLPToEnvelopes(orgID string, req *collectortracepb.ExportTraceServiceRequest) []pipeline.Envelope {
	var envelopes []pipeline.Envelope

	// Iterate through resource spans
	for _, resourceSpan := range req.GetResourceSpans() {
		// Extract resource attributes (if needed for global context)
		resourceAttrs := attributesToMap(resourceSpan.GetResource().GetAttributes())

		// Iterate through scope spans
		for _, scopeSpan := range resourceSpan.GetScopeSpans() {
			// Iterate through individual spans
			for _, span := range scopeSpan.GetSpans() {
				// Filter: only process configured span names
				if !h.spanFilter.ShouldProcess(span.GetName()) {
					continue
				}

				// Extract span attributes to check OpenInference span kind
				attrs := attributesToMap(span.GetAttributes())
				spanKind := getStringAttr(attrs, "openinference.span.kind", "")

				// Filter: only process spans with valid OpenInference span kinds
				// This filters out vendor-specific spans (e.g., llm.azure.*) that don't have span_kind
				if !h.spanFilter.ShouldProcessKind(spanKind) {
					continue
				}

				envelope := h.spanToEnvelope(orgID, span, resourceAttrs)
				envelopes = append(envelopes, envelope)
			}
		}
	}

	return envelopes
}

// spanToEnvelope converts a single OTLP span to a Gravity envelope
// Note: resourceAttrs parameter is currently unused but reserved for future use
func (h *Handler) spanToEnvelope(orgID string, span *tracepb.Span, _ map[string]string) pipeline.Envelope {
	// Convert trace and span IDs from bytes to hex strings
	traceID := hex.EncodeToString(span.GetTraceId())
	spanID := hex.EncodeToString(span.GetSpanId())

	// Extract span attributes
	attrs := attributesToMap(span.GetAttributes())

	// Calculate latency from start and end times (nanoseconds to milliseconds)
	startTime := span.GetStartTimeUnixNano()
	endTime := span.GetEndTimeUnixNano()
	latencyMs := int64(0)
	if endTime > startTime {
		latencyMs = int64((endTime - startTime) / 1_000_000) // nano to milli
	}

	// REQUIRED: Extract openinference.span.kind
	spanKind := getStringAttr(attrs, "openinference.span.kind", "")

	// Extract llm.provider (hosting provider)
	provider := getStringAttr(attrs, "llm.provider", "")

	// Try multiple model attribute names (LiteLLM uses different ones)
	model := getStringAttr(attrs, "llm.model_name",
		getStringAttr(attrs, "llm.response.model",
			getStringAttr(attrs, "llm.model",
				getStringAttr(attrs, "gen_ai.request.model", "unknown"))))

	// Extract llm.system (AI product identifier)
	// This is different from provider (hosting) - e.g., system=openai, provider=azure
	// If not present, infer from provider and model name
	system := getStringAttr(attrs, "llm.system",
		getStringAttr(attrs, "gen_ai.system", ""))
	if system == "" {
		system = inferSystem(provider, model)
	}

	route := getStringAttr(attrs, "llm.route", getStringAttr(attrs, "http.route", span.GetName()))

	// Extract invocation parameters (JSON string)
	invocationParameters := getStringAttr(attrs, "llm.invocation_parameters", "")

	// Extract token counts (OpenInference spec)
	promptTokens := getIntAttr(attrs, "llm.token_count.prompt",
		getIntAttr(attrs, "gen_ai.usage.input_tokens", 0))
	completionTokens := getIntAttr(attrs, "llm.token_count.completion",
		getIntAttr(attrs, "gen_ai.usage.output_tokens", 0))
	totalTokens := getIntAttr(attrs, "llm.token_count.total", 0)

	// Extended token counts - prompt details
	cachedTokens := getIntAttr(attrs, "llm.token_count.prompt_details.cache_read", 0)
	cacheWriteTokens := getIntAttr(attrs, "llm.token_count.prompt_details.cache_write", 0)
	promptAudioTokens := getIntAttr(attrs, "llm.token_count.prompt_details.audio", 0)

	// Extended token counts - completion details
	reasoningTokens := getIntAttr(attrs, "llm.token_count.completion_details.reasoning", 0)
	completionAudioTokens := getIntAttr(attrs, "llm.token_count.completion_details.audio", 0)

	// Extract latency details (from seconds to milliseconds)
	// OpenTelemetry/OpenInference use seconds as the standard unit for timing
	timeToFirstTokenSec := getFloatAttr(attrs, "llm.latency.time_to_first_token", 0.0)
	tokenGenerationSec := getFloatAttr(attrs, "llm.latency.token_generation", 0.0)
	totalLatencySec := getFloatAttr(attrs, "llm.latency.total", 0.0)

	// Convert to milliseconds for storage
	timeToFirstTokenMs := int64(timeToFirstTokenSec * 1000)
	tokenGenerationMs := int64(tokenGenerationSec * 1000)
	totalLatencyMs := int64(totalLatencySec * 1000)

	// Extract cost attributes (USD)
	costPrompt := getFloatAttr(attrs, "llm.cost.prompt", 0.0)
	costCompletion := getFloatAttr(attrs, "llm.cost.completion", 0.0)
	costTotal := getFloatAttr(attrs, "llm.cost.total", 0.0)

	// Cost details - prompt
	costPromptInput := getFloatAttr(attrs, "llm.cost.prompt_details.input", 0.0)
	costPromptCacheWrite := getFloatAttr(attrs, "llm.cost.prompt_details.cache_write", 0.0)
	costPromptCacheRead := getFloatAttr(attrs, "llm.cost.prompt_details.cache_read", 0.0)
	costPromptAudio := getFloatAttr(attrs, "llm.cost.prompt_details.audio", 0.0)

	// Cost details - completion
	costCompletionOutput := getFloatAttr(attrs, "llm.cost.completion_details.output", 0.0)
	costCompletionReasoning := getFloatAttr(attrs, "llm.cost.completion_details.reasoning", 0.0)
	costCompletionAudio := getFloatAttr(attrs, "llm.cost.completion_details.audio", 0.0)

	// Extract structured messages from flattened attributes
	inputMessages := extractMessages(attrs, "llm.input_messages.")
	outputMessages := extractMessages(attrs, "llm.output_messages.")

	// Extract tool definitions
	tools := extractTools(attrs)

	// Extract additional attributes
	isStreaming := getBoolAttr(attrs, "llm.is_streaming", false)
	responseID := getStringAttr(attrs, "llm.response.id", "")
	requestType := getStringAttr(attrs, "llm.request.type", "")
	inputValue := getStringAttr(attrs, "input.value", "")
	outputValue := getStringAttr(attrs, "output.value", "")

	// Extract preview from input.value or first input message
	preview := inputValue
	if preview == "" && len(inputMessages) > 0 {
		preview = inputMessages[0].Content
	}

	// Timestamp in milliseconds
	timestamp := int64(startTime / 1_000_000)

	return pipeline.Envelope{
		OrgID:                orgID,
		TraceID:              traceID,
		SpanID:               spanID,
		SpanKind:             spanKind,
		System:               system,
		Provider:             provider,
		Model:                model,
		Route:                route,
		InvocationParameters: invocationParameters,
		InputMessages:        inputMessages,
		OutputMessages:       outputMessages,
		Tools:                tools,
		Metrics: pipeline.Metrics{
			LatencyMs:             latencyMs,
			PromptTokens:          promptTokens,
			CompletionTokens:      completionTokens,
			TotalTokens:           totalTokens,
			TimeToFirstTokenMs:    timeToFirstTokenMs,
			TokenGenerationMs:     tokenGenerationMs,
			TotalLatencyMs:        totalLatencyMs,
			CachedTokens:          cachedTokens,
			CacheWriteTokens:      cacheWriteTokens,
			PromptAudioTokens:     promptAudioTokens,
			ReasoningTokens:       reasoningTokens,
			CompletionAudioTokens: completionAudioTokens,
		},
		Costs: pipeline.Costs{
			Prompt:              costPrompt,
			Completion:          costCompletion,
			Total:               costTotal,
			PromptInput:         costPromptInput,
			PromptCacheWrite:    costPromptCacheWrite,
			PromptCacheRead:     costPromptCacheRead,
			PromptAudio:         costPromptAudio,
			CompletionOutput:    costCompletionOutput,
			CompletionReasoning: costCompletionReasoning,
			CompletionAudio:     costCompletionAudio,
		},
		Pointers: pipeline.Pointers{
			PrefixPtr:     nil, // Will be populated by worker after processing
			PrefixPreview: preview,
		},
		Timestamp:   timestamp,
		IsStreaming: isStreaming,
		ResponseID:  responseID,
		RequestType: requestType,
		InputValue:  inputValue,
		OutputValue: outputValue,
		Attributes:  attrs, // Store all attributes for potential later use
	}
}

// attributesToMap converts OTLP KeyValue attributes to a string map
func attributesToMap(attrs []*commonpb.KeyValue) map[string]string {
	result := make(map[string]string)
	for _, attr := range attrs {
		key := attr.GetKey()
		value := attributeValueToString(attr.GetValue())
		if value != "" {
			result[key] = value
		}
	}
	return result
}

// attributeValueToString converts an OTLP attribute value to string
func attributeValueToString(val *commonpb.AnyValue) string {
	if val == nil {
		return ""
	}

	switch v := val.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return v.StringValue
	case *commonpb.AnyValue_IntValue:
		return fmt.Sprintf("%d", v.IntValue)
	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprintf("%f", v.DoubleValue)
	case *commonpb.AnyValue_BoolValue:
		return fmt.Sprintf("%t", v.BoolValue)
	default:
		return ""
	}
}

// getStringAttr gets a string attribute with a fallback default
func getStringAttr(attrs map[string]string, key, defaultVal string) string {
	if val, ok := attrs[key]; ok {
		return val
	}
	return defaultVal
}

// getIntAttr gets an integer attribute with a fallback default
func getIntAttr(attrs map[string]string, key string, defaultVal int) int {
	if val, ok := attrs[key]; ok {
		var intVal int
		if _, err := fmt.Sscanf(val, "%d", &intVal); err == nil {
			return intVal
		}
	}
	return defaultVal
}

// getBoolAttr gets a boolean attribute with a fallback default
func getBoolAttr(attrs map[string]string, key string, defaultVal bool) bool {
	if val, ok := attrs[key]; ok {
		return val == "true" || val == "True" || val == "1"
	}
	return defaultVal
}

// getFloatAttr gets a float attribute with a fallback default
func getFloatAttr(attrs map[string]string, key string, defaultVal float64) float64 {
	if val, ok := attrs[key]; ok {
		var floatVal float64
		if _, err := fmt.Sscanf(val, "%f", &floatVal); err == nil {
			return floatVal
		}
	}
	return defaultVal
}

// inferSystem infers llm.system from llm.provider and model name when llm.system is not present
// This handles cases where instrumentation doesn't set llm.system explicitly
func inferSystem(provider, modelName string) string {
	// Direct provider-to-system mappings
	// Azure typically hosts OpenAI models
	if provider == "azure" {
		return "openai"
	}

	// For other providers, try to infer from model name patterns
	// OpenAI models
	if hasPrefix(modelName, "gpt-", "text-davinci", "text-curie", "text-babbage", "text-ada",
		"davinci", "curie", "babbage", "ada", "o1-", "o3-") {
		return "openai"
	}

	// Anthropic models
	if hasPrefix(modelName, "claude-") {
		return "anthropic"
	}

	// Cohere models
	if hasPrefix(modelName, "command-", "embed-") {
		return "cohere"
	}

	// Mistral AI models
	if hasPrefix(modelName, "mistral-", "mixtral-") {
		return "mistralai"
	}

	// Google models
	if hasPrefix(modelName, "gemini-", "palm-", "bison-") {
		if provider == "google" || provider == "" {
			return "vertexai"
		}
	}

	// If provider is set but not recognized, use it as system
	if provider != "" && provider != "unknown" {
		return provider
	}

	return "unknown"
}

// hasPrefix checks if a string starts with any of the given prefixes
func hasPrefix(s string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// extractMessages extracts messages from flattened OpenInference attributes
// Pattern: llm.input_messages.0.message.role, llm.input_messages.0.message.content, etc.
func extractMessages(attrs map[string]string, prefix string) []pipeline.Message {
	// Map to hold messages by index
	messageMap := make(map[int]*pipeline.Message)

	// Scan all attributes for message fields
	for key, value := range attrs {
		// Check if this key starts with our prefix (e.g., "llm.input_messages.")
		if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
			continue
		}

		// Parse the index and field name
		// Format: llm.input_messages.INDEX.message.FIELD
		remaining := key[len(prefix):] // e.g., "0.message.role"

		var index int
		var field string
		n, err := fmt.Sscanf(remaining, "%d.message.%s", &index, &field)
		if err != nil || n != 2 {
			continue
		}

		// Get or create message for this index
		if messageMap[index] == nil {
			messageMap[index] = &pipeline.Message{}
		}

		// Set the appropriate field
		switch field {
		case "role":
			messageMap[index].Role = value
		case "content":
			messageMap[index].Content = value
		case "name":
			messageMap[index].Name = value
		case "tool_call_id":
			messageMap[index].ToolCallID = value
		}
	}

	// Convert map to sorted slice
	if len(messageMap) == 0 {
		return nil
	}

	// Find max index
	maxIndex := 0
	for index := range messageMap {
		if index > maxIndex {
			maxIndex = index
		}
	}

	// Build ordered slice
	messages := make([]pipeline.Message, 0, maxIndex+1)
	for i := 0; i <= maxIndex; i++ {
		if msg, ok := messageMap[i]; ok {
			messages = append(messages, *msg)
		}
	}

	return messages
}

// extractTools extracts tool definitions from flattened OpenInference attributes
// Pattern: llm.tools.0.tool.name, llm.tools.0.tool.description, etc.
func extractTools(attrs map[string]string) []pipeline.Tool {
	// Map to hold tools by index
	toolMap := make(map[int]*pipeline.Tool)

	// Scan all attributes for tool fields
	for key, value := range attrs {
		// Check if this key starts with "llm.tools."
		if len(key) <= 10 || key[:10] != "llm.tools." {
			continue
		}

		// Parse the index and field name
		// Format: llm.tools.INDEX.tool.FIELD
		remaining := key[10:] // e.g., "0.tool.name"

		var index int
		var field string
		if _, err := fmt.Sscanf(remaining, "%d.tool.%s", &index, &field); err != nil {
			continue
		}

		// Get or create tool for this index
		if toolMap[index] == nil {
			toolMap[index] = &pipeline.Tool{}
		}

		// Set the appropriate field
		switch field {
		case "name":
			toolMap[index].Name = value
		case "description":
			toolMap[index].Description = value
		case "parameters":
			toolMap[index].Parameters = value
		case "json_schema":
			toolMap[index].JSONSchema = value
		}
	}

	// Convert map to sorted slice
	if len(toolMap) == 0 {
		return nil
	}

	// Find max index
	maxIndex := 0
	for index := range toolMap {
		if index > maxIndex {
			maxIndex = index
		}
	}

	// Build ordered slice
	tools := make([]pipeline.Tool, 0, maxIndex+1)
	for i := 0; i <= maxIndex; i++ {
		if tool, ok := toolMap[i]; ok {
			tools = append(tools, *tool)
		}
	}

	return tools
}
