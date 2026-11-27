package llm

import (
	"context"
	"errors"
	"testing"
)

type observerTestProvider struct {
	calls  int
	err    error
	config ModelRuntimeConfig
	result *StreamResult
}

func (p *observerTestProvider) StreamChatWithTools(ctx context.Context, config ModelRuntimeConfig, messages []ChatMessage, tools []ToolDef, callbacks StreamCallbacks) (*StreamResult, error) {
	p.calls++
	p.config = config
	if p.result != nil {
		return p.result, p.err
	}
	return &StreamResult{Type: StreamResultText}, p.err
}

func TestRequestObserverCanClampProviderConfig(t *testing.T) {
	provider := &observerTestProvider{result: &StreamResult{TokenUsage: &TokenUsage{OutputTokens: 4}}}
	ctx := WithRequestObserver(context.Background(), func(ctx context.Context, config ModelRuntimeConfig, messages []ChatMessage, tools []ToolDef) (func(*StreamResult, error) error, error) {
		config.MaxOutputTokens = 64
		ReplaceRequestConfig(ctx, config)
		return func(*StreamResult, error) error { return nil }, nil
	})
	_, err := NewFactory(provider).GetProvider(ProviderOpenAI).StreamChatWithTools(ctx, ModelRuntimeConfig{MaxOutputTokens: 1024}, nil, nil, StreamCallbacks{})
	if err != nil || provider.config.MaxOutputTokens != 64 {
		t.Fatalf("provider config was not clamped: %+v %v", provider.config, err)
	}
}

func TestAppendRequestObserverPreservesExistingObserver(t *testing.T) {
	ctx := WithRequestObserver(context.Background(), func(context.Context, ModelRuntimeConfig, []ChatMessage, []ToolDef) (func(*StreamResult, error) error, error) {
		return nil, errors.New("stop")
	})
	called := false
	ctx = AppendRequestObserver(ctx, func(context.Context, ModelRuntimeConfig, []ChatMessage, []ToolDef) (func(*StreamResult, error) error, error) {
		called = true
		return nil, nil
	})
	_, err := NewFactory(&observerTestProvider{}).GetProvider(ProviderOpenAI).StreamChatWithTools(ctx, ModelRuntimeConfig{}, nil, nil, StreamCallbacks{})
	if err == nil || called {
		t.Fatalf("existing observer was replaced: %v %t", err, called)
	}
}

func TestFactoryAndContextObserversBothFinishOnce(t *testing.T) {
	provider := &observerTestProvider{result: &StreamResult{TokenUsage: &TokenUsage{OutputTokens: 4}}}
	factory := NewFactory(provider)
	factoryFinished := 0
	factory.SetRequestObserver(func(ctx context.Context, config ModelRuntimeConfig, _ []ChatMessage, _ []ToolDef) (func(*StreamResult, error) error, error) {
		config.MaxOutputTokens = 32
		ReplaceRequestConfig(ctx, config)
		return func(*StreamResult, error) error { factoryFinished++; return nil }, nil
	})
	localFinished := 0
	ctx := WithRequestObserver(context.Background(), func(context.Context, ModelRuntimeConfig, []ChatMessage, []ToolDef) (func(*StreamResult, error) error, error) {
		return func(*StreamResult, error) error { localFinished++; return nil }, nil
	})
	_, err := factory.GetProvider(ProviderOpenAI).StreamChatWithTools(ctx, ModelRuntimeConfig{MaxOutputTokens: 100}, nil, nil, StreamCallbacks{})
	if err != nil || provider.config.MaxOutputTokens != 32 || factoryFinished != 1 || localFinished != 1 {
		t.Fatalf("observers: config=%+v factory=%d local=%d err=%v", provider.config, factoryFinished, localFinished, err)
	}
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
