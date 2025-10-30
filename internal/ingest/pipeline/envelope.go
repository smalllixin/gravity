package pipeline

import (
	"fmt"
	"strings"
	"time"
)

// Envelope represents a processed OTLP span converted into the Gravity format
type Envelope struct {
	OrgID                string            `json:"org_id"`
	TraceID              string            `json:"trace_id"`
	SpanID               string            `json:"span_id"`
	SpanKind             string            `json:"span_kind"` // openinference.span.kind (REQUIRED)
	System               string            `json:"system"`    // llm.system (AI product: openai, anthropic, etc)
	Provider             string            `json:"provider"`  // llm.provider (hosting: azure, aws, etc)
	Model                string            `json:"model"`     // llm.model_name
	Route                string            `json:"route"`
	InvocationParameters string            `json:"invocation_parameters"`     // llm.invocation_parameters (JSON)
	InputMessages        []Message         `json:"input_messages,omitempty"`  // llm.input_messages
	OutputMessages       []Message         `json:"output_messages,omitempty"` // llm.output_messages
	Tools                []Tool            `json:"tools,omitempty"`           // llm.tools
	Metrics              Metrics           `json:"metrics"`
	Costs                Costs             `json:"costs"`
	Pointers             Pointers          `json:"pointers"`
	Timestamp            int64             `json:"timestamp"`

	// Additional attributes
	IsStreaming  bool   `json:"is_streaming"`            // llm.is_streaming
	ResponseID   string `json:"response_id,omitempty"`   // llm.response.id
	RequestType  string `json:"request_type,omitempty"`  // llm.request.type
	InputValue   string `json:"input_value,omitempty"`   // input.value
	OutputValue  string `json:"output_value,omitempty"`  // output.value

	Attributes map[string]string `json:"attributes,omitempty"`
}

// Metrics holds the usage and performance metrics
type Metrics struct {
	LatencyMs        int64 `json:"latency_ms"`
	PromptTokens     int   `json:"prompt_tokens"`     // llm.token_count.prompt
	CompletionTokens int   `json:"completion_tokens"` // llm.token_count.completion
	TotalTokens      int   `json:"total_tokens"`      // llm.token_count.total

	// Latency details
	TimeToFirstTokenMs int64 `json:"time_to_first_token_ms"` // llm.latency.time_to_first_token (ms)
	TokenGenerationMs  int64 `json:"token_generation_ms"`    // llm.latency.token_generation (ms)
	TotalLatencyMs     int64 `json:"total_latency_ms"`       // llm.latency.total (ms)

	// Prompt token details
	CachedTokens      int `json:"cached_tokens"`       // llm.token_count.prompt_details.cache_read
	CacheWriteTokens  int `json:"cache_write_tokens"`  // llm.token_count.prompt_details.cache_write
	PromptAudioTokens int `json:"prompt_audio_tokens"` // llm.token_count.prompt_details.audio

	// Completion token details
	ReasoningTokens       int `json:"reasoning_tokens"`        // llm.token_count.completion_details.reasoning
	CompletionAudioTokens int `json:"completion_audio_tokens"` // llm.token_count.completion_details.audio
}

// Costs holds the cost metrics in USD
type Costs struct {
	Prompt     float64 `json:"prompt"`     // llm.cost.prompt (USD)
	Completion float64 `json:"completion"` // llm.cost.completion (USD)
	Total      float64 `json:"total"`      // llm.cost.total (USD)

	// Detailed cost breakdown
	PromptInput      float64 `json:"prompt_input"`       // llm.cost.prompt_details.input
	PromptCacheWrite float64 `json:"prompt_cache_write"` // llm.cost.prompt_details.cache_write
	PromptCacheRead  float64 `json:"prompt_cache_read"`  // llm.cost.prompt_details.cache_read
	PromptAudio      float64 `json:"prompt_audio"`       // llm.cost.prompt_details.audio

	CompletionOutput    float64 `json:"completion_output"`    // llm.cost.completion_details.output
	CompletionReasoning float64 `json:"completion_reasoning"` // llm.cost.completion_details.reasoning
	CompletionAudio     float64 `json:"completion_audio"`     // llm.cost.completion_details.audio
}

// Pointers holds references to prompt content (will be resolved by worker)
type Pointers struct {
	PrefixPtr     *string `json:"prefix_ptr,omitempty"`     // Content-addressed hash (after processing)
	PrefixPreview string  `json:"prefix_preview,omitempty"` // Short preview for debugging
}

// Message represents a chat message (input or output)
// Maps to llm.input_messages and llm.output_messages from OpenInference spec
type Message struct {
	Role    string `json:"role"`              // message.role: "system", "user", "assistant", "tool"
	Content string `json:"content,omitempty"` // message.content: text content of the message
	Name    string `json:"name,omitempty"`    // message.name: for tool messages, the tool name

	// Tool call fields (for assistant messages that invoke tools)
	ToolCallID string `json:"tool_call_id,omitempty"` // message.tool_call_id: links tool result to tool call
}

// Tool represents an available tool/function that can be called by the LLM
// Maps to llm.tools from OpenInference spec
type Tool struct {
	Name        string `json:"name"`                  // tool.name
	Description string `json:"description,omitempty"` // tool.description
	Parameters  string `json:"parameters,omitempty"`  // tool.parameters: JSON schema string
	JSONSchema  string `json:"json_schema,omitempty"` // tool.json_schema: complete tool definition (alternative format)
}

// Batch represents a batch of envelopes ready for queue submission
type Batch struct {
	Envelopes []Envelope
	CreatedAt time.Time
}

// NewBatch creates a new batch
func NewBatch() *Batch {
	return &Batch{
		Envelopes: make([]Envelope, 0),
		CreatedAt: time.Now(),
	}
}

// Add adds an envelope to the batch
func (b *Batch) Add(env Envelope) {
	b.Envelopes = append(b.Envelopes, env)
}

// Size returns the number of envelopes in the batch
func (b *Batch) Size() int {
	return len(b.Envelopes)
}

// IsEmpty checks if the batch is empty
func (b *Batch) IsEmpty() bool {
	return len(b.Envelopes) == 0
}

// truncateSmart truncates content showing both start and end with ellipsis in middle
func truncateSmart(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}

	halfLen := (maxLen - 5) / 2 // 5 chars for " ... "
	return content[:halfLen] + " ... " + content[len(content)-halfLen:]
}

// LogSummary returns a formatted string suitable for logging with smart truncation
func (e *Envelope) LogSummary() string {
	var sb strings.Builder

	// Basic info
	sb.WriteString(fmt.Sprintf("Extracted: %d input messages, %d output messages, %d tools\n",
		len(e.InputMessages), len(e.OutputMessages), len(e.Tools)))

	// Key attributes
	var keyAttrs []string
	if e.IsStreaming {
		keyAttrs = append(keyAttrs, "streaming=true")
	}
	if e.ResponseID != "" {
		keyAttrs = append(keyAttrs, fmt.Sprintf("response_id=%s", e.ResponseID))
	}
	if e.RequestType != "" {
		keyAttrs = append(keyAttrs, fmt.Sprintf("type=%s", e.RequestType))
	}
	if e.Metrics.TimeToFirstTokenMs > 0 {
		keyAttrs = append(keyAttrs, fmt.Sprintf("ttft=%dms", e.Metrics.TimeToFirstTokenMs))
	}
	if e.Metrics.TokenGenerationMs > 0 {
		keyAttrs = append(keyAttrs, fmt.Sprintf("token_gen=%dms", e.Metrics.TokenGenerationMs))
	}
	if e.Metrics.TotalLatencyMs > 0 {
		keyAttrs = append(keyAttrs, fmt.Sprintf("total_latency=%dms", e.Metrics.TotalLatencyMs))
	}
	if len(keyAttrs) > 0 {
		sb.WriteString(fmt.Sprintf("  Key attributes: %s\n", strings.Join(keyAttrs, ", ")))
	}

	// Log input messages
	for i, msg := range e.InputMessages {
		contentPreview := truncateSmart(msg.Content, 120)
		sb.WriteString(fmt.Sprintf("  Input[%d]: role=%s content=%q\n", i, msg.Role, contentPreview))
	}

	// Log output messages
	for i, msg := range e.OutputMessages {
		contentPreview := truncateSmart(msg.Content, 120)
		sb.WriteString(fmt.Sprintf("  Output[%d]: role=%s content=%q\n", i, msg.Role, contentPreview))
	}

	// Log tools
	for i, tool := range e.Tools {
		sb.WriteString(fmt.Sprintf("  Tool[%d]: name=%s\n", i, tool.Name))
		// sb.WriteString(fmt.Sprintf("  Tool[%d]: name=%s description=%q\n", i, tool.Name, tool.Description))
	}

	// Dump metadata if present
	if metadata, ok := e.Attributes["metadata"]; ok && metadata != "" {
		sb.WriteString("  Metadata:\n")
		sb.WriteString(fmt.Sprintf("    %s\n", metadata))
	}

	// Log attributes if significant
	if e.System != "unknown" || len(e.Attributes) > 5 {
		sb.WriteString("  Attributes:\n")
		for key := range e.Attributes {
			sb.WriteString(fmt.Sprintf("    %s\n", key))
		}
	}

	return sb.String()
}
