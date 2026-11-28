package llm

import "slimebot/internal/domain"

// Provider name constants.
const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
	ProviderDeepSeek  = "deepseek"
)

// Provider-neutral value types live in domain so persistence does not depend
// on the LLM service layer. These aliases preserve the existing llm API.
type ModelRuntimeConfig = domain.ModelRuntimeConfig
type ChatMessage = domain.ChatMessage
type ThinkingBlockInfo = domain.ThinkingBlockInfo
type ChatMessageContentPartType = domain.ChatMessageContentPartType
type ChatMessageContentPart = domain.ChatMessageContentPart
type TokenUsage = domain.TokenUsage
type ToolCallInfo = domain.ToolCallInfo

const (
	ChatMessageContentPartTypeText  = domain.ChatMessageContentPartTypeText
	ChatMessageContentPartTypeImage = domain.ChatMessageContentPartTypeImage
	ChatMessageContentPartTypeAudio = domain.ChatMessageContentPartTypeAudio
	ChatMessageContentPartTypeFile  = domain.ChatMessageContentPartTypeFile
)

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
	OnPrivacy       func(PrivacySummary) error
	OnChunk         func(string) error // text content chunks
	OnThinkingChunk func(string) error // thinking content chunks (nil = skip)
}

type ToolDef = domain.ToolDef

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
	// FinishReason distinguishes complete summaries from truncated/refused output.
	FinishReason string
	// TokenUsage is provider-reported usage for this API response when available.
	TokenUsage *TokenUsage
	// Tool calls from the model (only when Type is StreamResultToolCalls).
	ToolCalls []ToolCallInfo
	// AssistantMessage carries assistant role content including tool_calls for context replay.
	AssistantMessage ChatMessage
}
