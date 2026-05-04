package llm

// Provider name constants.
const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
	ProviderDeepSeek  = "deepseek"
)

// ModelRuntimeConfig is per-request LLM configuration.
type ModelRuntimeConfig struct {
	// ConfigID is the saved LLM config id when the model came from persisted settings.
	ConfigID string
	// Provider selects which backend implementation to use.
	Provider string
	// Base URL for OpenAI-compatible APIs (Anthropic may also use a custom base URL).
	BaseURL string
	// API key for authentication.
	APIKey string
	// Model identifier.
	Model string
	// ContextSize is the approximate token threshold for session context compaction.
	ContextSize int
	// Sampling temperature.
	Temperature float64
	// Thinking level: off, low, medium, high, max. Empty or "off" = no extended thinking.
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
	// ThinkingBlocks carries Anthropic thinking blocks that must be passed back
	// when a thinking-enabled assistant turn requests tools.
	ThinkingBlocks []ThinkingBlockInfo `json:"thinkingBlocks,omitempty"`
	// ReasoningContent carries thinking/reasoning output from providers like DeepSeek.
	// Must be passed back in multi-turn agent loops to avoid API errors.
	ReasoningContent string `json:"reasoningContent,omitempty"`
}

// ThinkingBlockInfo describes one Anthropic thinking content block.
type ThinkingBlockInfo struct {
	Thinking     string `json:"thinking,omitempty"`
	Signature    string `json:"signature,omitempty"`
	RedactedData string `json:"redactedData,omitempty"`
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

// TokenUsage is provider-reported token usage for one model response.
type TokenUsage struct {
	InputTokens              int `json:"inputTokens"`
	OutputTokens             int `json:"outputTokens"`
	CacheCreationInputTokens int `json:"cacheCreationInputTokens"`
	CacheReadInputTokens     int `json:"cacheReadInputTokens"`
	// TotalTokens is the provider-reported total for the request when available.
	// It is the best available snapshot of the context window used by that call.
	TotalTokens int `json:"totalTokens,omitempty"`
}

func (u TokenUsage) TotalContextTokens() int {
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return u.InputTokens + u.OutputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

func (u TokenUsage) ContextWindowTokens() int {
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

func (u TokenUsage) IsZero() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.CacheCreationInputTokens == 0 && u.CacheReadInputTokens == 0 && u.TotalTokens == 0
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
	case "max":
		return 65536
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
	case "max":
		return "xhigh"
	default:
		return ""
	}
}

// DeepSeekReasoningEffort maps the shared thinking level to DeepSeek's
// OpenAI-compatible reasoning_effort values.
func DeepSeekReasoningEffort(level string) string {
	switch level {
	case "low", "medium", "high":
		return "high"
	case "max":
		return "max"
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
	// TokenUsage is provider-reported usage for this API response when available.
	TokenUsage *TokenUsage
	// Tool calls from the model (only when Type is StreamResultToolCalls).
	ToolCalls []ToolCallInfo
	// AssistantMessage carries assistant role content including tool_calls for context replay.
	AssistantMessage ChatMessage
}
