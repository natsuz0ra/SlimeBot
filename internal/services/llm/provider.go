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
	providers     map[string]Provider
	textProtector func(context.Context, ModelRuntimeConfig, string) (string, error)
	observer      RequestObserver
}

func (f *Factory) SetRequestObserver(observer RequestObserver) { f.observer = observer }

// SetTextProtector protects complete auxiliary input before callers shorten it.
// Configure it during application wiring, before the factory is shared.
func (f *Factory) SetTextProtector(protect func(context.Context, ModelRuntimeConfig, string) (string, error)) {
	f.textProtector = protect
}
func (f *Factory) ProtectText(ctx context.Context, cfg ModelRuntimeConfig, text string) (string, error) {
	if f.textProtector != nil {
		return f.textProtector(ctx, cfg, text)
	}
	return text, nil
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
		if p == nil {
			return nil
		}
		return observedProvider{Provider: p, factory: f}
	}
	p := f.providers[ProviderOpenAI]
	if p == nil {
		return nil
	}
	return observedProvider{Provider: p, factory: f}
}
