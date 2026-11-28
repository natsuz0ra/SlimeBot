package llm

import (
	"context"
	"fmt"
	"strings"

	"slimebot/internal/apperrors"
)

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

// NewFactory builds a factory with OpenAI as the legacy default provider.
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
	f.providers[NormalizeProviderName(name)] = p
}

// NormalizeProviderName canonicalizes protocol names for configuration and
// runtime lookup. An empty name retains the historical OpenAI default.
func NormalizeProviderName(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized == "" {
		return ProviderOpenAI
	}
	return normalized
}

// ValidateProviderName rejects unsupported wire protocols explicitly.
func ValidateProviderName(provider string) error {
	switch NormalizeProviderName(provider) {
	case ProviderOpenAI, ProviderDeepSeek, ProviderAnthropic:
		return nil
	default:
		return fmt.Errorf("%w: unsupported LLM provider %q (supported: %s, %s, %s)", apperrors.ErrInvalidInput, strings.TrimSpace(provider), ProviderOpenAI, ProviderDeepSeek, ProviderAnthropic)
	}
}

// ResolveProvider returns the implementation for a supported provider string.
// Unknown protocols and known protocols without an implementation are errors;
// neither case silently falls back to OpenAI.
func (f *Factory) ResolveProvider(provider string) (Provider, error) {
	name := NormalizeProviderName(provider)
	if err := ValidateProviderName(name); err != nil {
		return nil, err
	}
	p, ok := f.providers[name]
	if !ok || p == nil {
		return nil, fmt.Errorf("%w: LLM provider %q is not initialized", apperrors.ErrInvalidInput, name)
	}
	return observedProvider{Provider: p, factory: f}, nil
}

// GetProvider retains the legacy no-error API for tests and compatibility
// callers. Known protocols may still use the historical OpenAI implementation
// fallback when their own implementation was not registered. Unknown protocol
// names return nil; production code must use ResolveProvider.
func (f *Factory) GetProvider(provider string) Provider {
	p, err := f.ResolveProvider(provider)
	if err == nil {
		return p
	}
	if ValidateProviderName(provider) != nil {
		return nil
	}
	fallback := f.providers[ProviderOpenAI]
	if fallback == nil {
		return nil
	}
	return observedProvider{Provider: fallback, factory: f}
}
