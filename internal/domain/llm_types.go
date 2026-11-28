package domain

// ModelRuntimeConfig is per-request LLM configuration.
type ModelRuntimeConfig struct {
	// ConfigID is the saved LLM config id when the model came from persisted settings.
	ConfigID string
	// ProviderID is the persisted provider used by request privacy policy.
	ProviderID string
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
	// MaxOutputTokens is resolved once and shared with context budgeting.
	MaxOutputTokens int
	Purpose         string
}

// orchestration, and provider adapters.
// ToolDef is passed to the LLM tools API.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ChatMessage is the provider-agnostic message shape shared by persistence,
// orchestration, and provider adapters.
type ChatMessage struct {
	// SourceInboxID ties host-delivered messages to their atomic durable receipt.
	SourceInboxID string `json:"sourceInboxId,omitempty"`
	// SourceKind is host metadata. Provider adapters serialize explicit wire fields.
	SourceKind string `json:"sourceKind,omitempty"`
	// ContextLayer identifies prompt accounting metadata; it is never sent to providers.
	ContextLayer string                   `json:"-"`
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

// InputContextTokens excludes output and counts provider caches exactly once.
func (u TokenUsage) InputContextTokens(provider string) int {
	if provider == "anthropic" {
		return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
	}
	return u.InputTokens
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
