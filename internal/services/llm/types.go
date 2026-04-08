package llm

// Provider 常量：标识 LLM 提供商类型。
const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
)

// ModelRuntimeConfig 描述一次 LLM 请求的运行时配置。
type ModelRuntimeConfig struct {
	// Provider 标识使用哪个 LLM 提供商。
	Provider string
	// 兼容 OpenAI 协议的服务地址（Anthropic 也可自定义 base URL）。
	BaseURL string
	// 鉴权密钥。
	APIKey string
	// 目标模型名称。
	Model string
	// 采样温度。
	Temperature float64
}

// ChatMessage 统一的消息结构，跨提供商通用。
type ChatMessage struct {
	Role         string                   `json:"role"`
	Content      string                   `json:"content"`
	ContentParts []ChatMessageContentPart `json:"contentParts,omitempty"`
	ToolCallID   string                   `json:"toolCallId,omitempty"`
	// ToolCalls 仅在 role=assistant 且模型返回了工具调用时使用。
	ToolCalls []ToolCallInfo `json:"toolCalls,omitempty"`
}

// ChatMessageContentPartType 内容块类型枚举。
type ChatMessageContentPartType string

const (
	ChatMessageContentPartTypeText  ChatMessageContentPartType = "text"
	ChatMessageContentPartTypeImage ChatMessageContentPartType = "image"
	ChatMessageContentPartTypeAudio ChatMessageContentPartType = "audio"
	ChatMessageContentPartTypeFile  ChatMessageContentPartType = "file"
)

// ChatMessageContentPart 描述消息中的多模态内容块。
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

// ToolCallInfo 描述一次工具调用请求。
type ToolCallInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDef 用于传入 LLM API 的工具定义。
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// StreamResultType 区分流式结果类型。
type StreamResultType int

const (
	// StreamResultText 表示模型返回了纯文本回答。
	StreamResultText StreamResultType = 0
	// StreamResultToolCalls 表示模型返回了工具调用请求。
	StreamResultToolCalls StreamResultType = 1
)

// StreamResult 包含一次流式调用的结果。
type StreamResult struct {
	// 本次流式结果类型：文本或工具调用。
	Type StreamResultType
	// 模型返回的工具调用列表（仅 Type=StreamResultToolCalls 时有值）。
	ToolCalls []ToolCallInfo
	// AssistantMessage 用于将 assistant 消息（含 tool_calls）追加回上下文。
	AssistantMessage ChatMessage
}
