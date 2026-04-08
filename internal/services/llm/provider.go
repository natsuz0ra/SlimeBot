package llm

import "context"

// Provider 抽象 LLM 提供商的流式聊天能力（含可选工具支持）。
type Provider interface {
	StreamChatWithTools(
		ctx context.Context,
		modelConfig ModelRuntimeConfig,
		messages []ChatMessage,
		toolDefs []ToolDef,
		onChunk func(string) error,
	) (*StreamResult, error)
}

// Factory 根据 provider 标识返回对应的 LLM 提供商实现。
// 通过 Register 注入具体实现，避免循环依赖。
type Factory struct {
	providers map[string]Provider
}

// NewFactory 创建 provider 工厂，openai 为默认 provider。
func NewFactory(openai Provider) *Factory {
	return &Factory{
		providers: map[string]Provider{
			ProviderOpenAI: openai,
		},
	}
}

// Register 注册一个 provider 实现。
func (f *Factory) Register(name string, p Provider) {
	f.providers[name] = p
}

// GetProvider 根据 provider 字符串返回对应的 LLM Provider。
// 未知 provider 回退到 OpenAI。
func (f *Factory) GetProvider(provider string) Provider {
	if p, ok := f.providers[provider]; ok {
		return p
	}
	return f.providers[ProviderOpenAI]
}
