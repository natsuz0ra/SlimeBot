package llm

// Provider name constants.
const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
)

// ModelRuntimeConfig is per-request LLM configuration.
type ModelRuntimeConfig struct {
	// Provider selects which backend implementation to use.
	Provider string
	// Base URL for OpenAI-compatible APIs (Anthropic may also use a custom base URL).
	BaseURL string
	// API key for authentication.
	APIKey string
	// Model identifier.
	Model string
	// Sampling temperature.
	Temperature float64
	// Thinking level: off, low, medium, high. Empty or "off" = no extended thinking.
	ThinkingLevel string
}

// ChatMessage is the provider-agnostic message shape.
type ChatMessage struct {
	Role         string                   `json:"role"`
	Content      string                   `json:"content"`
	ContentParts []ChatMessageContentPart `json:"contentParts,omitempty"`
	ToolCallID   string                   `json:"toolCallId,omitempty"`
	// ToolCalls is set when role=assistant and the model requested tools.
	ToolCalls []ToolCallInfo `json:"toolCalls,omitempty"`
	// ReasoningContent carries thinking/reasoning output from providers like DeepSeek.
	// Must be passed back in multi-turn agent loops to avoid API errors.
	ReasoningContent string `json:"reasoningContent,omitempty"`
}

// ChatMessageContentPartType enumerates multimodal part kinds.
type ChatMessageContentPartType string

const (
	ChatMessageContentPartTypeText  ChatMessageContentPartType = "text"
	ChatMessageContentPartTypeImage ChatMessageContentPartType = "image"
	ChatMessageContentPartTypeAudio ChatMessageContentPartType = "audio"
	ChatMessageContentPartTypeFile  ChatMessageContentPartType = "file"
)

// ChatMessageContentPart is one multimodal segment in a message.
type ChatMessageContentPart struct {
	Type             ChatMessageContentPartType `json:"type"`
	Text             string                     `json:"text,omitempty"`
	ImageURL         string                     `json:"imageUrl,omitempty"`
	ImageDetail      string                     `json:"imageDetail,omitempty"`
	InputAudioData   string                     `json:"inputAudioData,omitempty"`
	InputAudioFormat string                     `json:"inputAudioFormat,omitempty"`
	FileDataBase64   string                     `json:"fileDataBase64,omitempty"`
	Filename         string                     `json:"filename,omitempty"`
}

// ToolCallInfo describes one tool invocation from the model.
type ToolCallInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDef is passed to the LLM tools API.

// ThinkingBudgetTokens maps a thinking level to Anthropic budget_tokens.
// Returns 0 for off/empty, meaning extended thinking is disabled.
func ThinkingBudgetTokens(level string) int {
	switch level {
	case "low":
		return 8192
	case "medium":
		return 16384
	case "high":
		return 32768
	default:
		return 0
	}
}

// ThinkingReasoningEffort maps a thinking level to OpenAI reasoning_effort.
// Returns empty string for off/empty, meaning reasoning is disabled.
func ThinkingReasoningEffort(level string) string {
	switch level {
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	default:
		return ""
	}
}

// StreamCallbacks groups streaming output callbacks for Provider implementations.
type StreamCallbacks struct {
	OnChunk         func(string) error // text content chunks
	OnThinkingChunk func(string) error // thinking content chunks (nil = skip)
}

type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// StreamResultType classifies streaming outcomes.
type StreamResultType int

const (
	// StreamResultText means the model returned plain text.
	StreamResultText StreamResultType = 0
	// StreamResultToolCalls means the model requested tool calls.
	StreamResultToolCalls StreamResultType = 1
)

// StreamResult is one streaming completion outcome.
type StreamResult struct {
	// Result kind: text or tool calls.
	Type StreamResultType
	// Tool calls from the model (only when Type is StreamResultToolCalls).
	ToolCalls []ToolCallInfo
	// AssistantMessage carries assistant role content including tool_calls for context replay.
	AssistantMessage ChatMessage
}
