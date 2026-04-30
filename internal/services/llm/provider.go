package llm

import "context"

// Provider abstracts streaming chat with optional tools.
type Provider interface {
	StreamChatWithTools(
		ctx context.Context,
		modelConfig ModelRuntimeConfig,
		messages []ChatMessage,
		toolDefs []ToolDef,
		callbacks StreamCallbacks,
	) (*StreamResult, error)
}

// Factory resolves Provider implementations by name.
// Implementations are registered to avoid import cycles.
type Factory struct {
	providers map[string]Provider
}

// NewFactory builds a factory with OpenAI as the default provider.
func NewFactory(openai Provider) *Factory {
	return &Factory{
		providers: map[string]Provider{
			ProviderOpenAI:   openai,
			ProviderDeepSeek: openai,
		},
	}
}

// Register adds a named provider implementation.
func (f *Factory) Register(name string, p Provider) {
	f.providers[name] = p
}

// GetProvider returns the implementation for a provider string.
// Unknown names fall back to OpenAI.
func (f *Factory) GetProvider(provider string) Provider {
	if p, ok := f.providers[provider]; ok {
		return p
	}
	return f.providers[ProviderOpenAI]
}
