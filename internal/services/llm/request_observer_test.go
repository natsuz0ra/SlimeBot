package llm

import (
	"context"
	"errors"
	"testing"
)

type observerTestProvider struct {
	calls int
	err   error
}

func (p *observerTestProvider) StreamChatWithTools(ctx context.Context, config ModelRuntimeConfig, messages []ChatMessage, tools []ToolDef, callbacks StreamCallbacks) (*StreamResult, error) {
	p.calls++
	return &StreamResult{Type: StreamResultText}, p.err
}

func TestRequestObserverRejectsBeforeProviderCall(t *testing.T) {
	provider := &observerTestProvider{}
	denied := errors.New("root budget exceeded")
	ctx := WithRequestObserver(context.Background(), func(context.Context, ModelRuntimeConfig, []ChatMessage, []ToolDef) (func(*StreamResult, error) error, error) {
		return nil, denied
	})
	_, err := NewFactory(provider).GetProvider(ProviderOpenAI).StreamChatWithTools(ctx, ModelRuntimeConfig{Purpose: "compaction"}, nil, nil, StreamCallbacks{})
	if !errors.Is(err, denied) || provider.calls != 0 {
		t.Fatalf("budget rejection reached provider: calls=%d error=%v", provider.calls, err)
	}
}

func TestRequestObserverSettlesEveryPurposeIncludingProviderFailure(t *testing.T) {
	for _, purpose := range []string{"chat", "compaction", "approval_review"} {
		t.Run(purpose, func(t *testing.T) {
			failure := errors.New("provider failed")
			provider := &observerTestProvider{err: failure}
			finished := 0
			ctx := WithRequestObserver(context.Background(), func(ctx context.Context, config ModelRuntimeConfig, messages []ChatMessage, tools []ToolDef) (func(*StreamResult, error) error, error) {
				if config.Purpose != purpose {
					t.Fatal("purpose lost")
				}
				return func(result *StreamResult, err error) error {
					finished++
					if !errors.Is(err, failure) {
						t.Fatal("provider failure lost")
					}
					return nil
				}, nil
			})
			_, err := NewFactory(provider).GetProvider(ProviderOpenAI).StreamChatWithTools(ctx, ModelRuntimeConfig{Purpose: purpose}, nil, nil, StreamCallbacks{})
			if !errors.Is(err, failure) || finished != 1 {
				t.Fatalf("reservation not settled: %d %v", finished, err)
			}
		})
	}
}
