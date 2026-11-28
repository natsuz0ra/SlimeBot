package llm

import (
	"context"
	"errors"
	"testing"

	"slimebot/internal/apperrors"
)

type providerContractStub struct{}

func (*providerContractStub) StreamChatWithTools(context.Context, ModelRuntimeConfig, []ChatMessage, []ToolDef, StreamCallbacks) (*StreamResult, error) {
	return &StreamResult{Type: StreamResultText}, nil
}

func TestResolveProviderNormalizesKnownProtocols(t *testing.T) {
	provider := &providerContractStub{}
	factory := NewFactory(provider)
	factory.Register(" AnThRoPiC ", provider)
	for _, name := range []string{"", " OpenAI ", "DEEPSEEK", "anthropic"} {
		t.Run(name, func(t *testing.T) {
			resolved, err := factory.ResolveProvider(name)
			if err != nil {
				t.Fatalf("ResolveProvider(%q) failed: %v", name, err)
			}
			if resolved == nil {
				t.Fatalf("ResolveProvider(%q) returned nil", name)
			}
		})
	}
}

func TestResolveProviderRejectsUnknownProtocolWithoutFallback(t *testing.T) {
	factory := NewFactory(&providerContractStub{})
	resolved, err := factory.ResolveProvider("mystery")
	if resolved != nil {
		t.Fatal("unknown protocol must not resolve to OpenAI")
	}
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
	if got := factory.GetProvider("mystery"); got != nil {
		t.Fatal("legacy GetProvider must not silently fall back for unknown names")
	}
}

func TestResolveProviderRejectsMissingImplementation(t *testing.T) {
	factory := NewFactory(&providerContractStub{})
	if _, err := factory.ResolveProvider(ProviderAnthropic); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("unregistered Anthropic error = %v, want ErrInvalidInput", err)
	}
	if got := factory.GetProvider(ProviderAnthropic); got == nil {
		t.Fatal("legacy GetProvider should retain OpenAI fallback for a known unregistered protocol")
	}
}
