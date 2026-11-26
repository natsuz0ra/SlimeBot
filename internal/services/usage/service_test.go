package usage

import (
	"context"
	"errors"
	"slimebot/internal/domain"
	"slimebot/internal/services/llm"
	"testing"
)

type memoryStore struct {
	records []domain.UsageRecord
}

func (s *memoryStore) RecordUsage(_ context.Context, record *domain.UsageRecord) error {
	s.records = append(s.records, *record)
	return nil
}
func (s *memoryStore) SummarizeUsage(context.Context, domain.UsageQuery) ([]domain.UsageSummary, error) {
	return nil, nil
}
func (s *memoryStore) SessionOutputTokens(_ context.Context, root string) (int64, error) {
	var output int64
	for _, record := range s.records {
		if record.RootSessionID == root {
			output += record.OutputTokens
		}
	}
	return output, nil
}

type settingsStore map[string]string

func (s settingsStore) GetSetting(_ context.Context, key string) (string, error) { return s[key], nil }
func (s settingsStore) SetSetting(_ context.Context, key, value string) error {
	s[key] = value
	return nil
}

type provider struct {
	calls  int
	config llm.ModelRuntimeConfig
	result *llm.StreamResult
	err    error
}

func (p *provider) StreamChatWithTools(_ context.Context, config llm.ModelRuntimeConfig, _ []llm.ChatMessage, _ []llm.ToolDef, _ llm.StreamCallbacks) (*llm.StreamResult, error) {
	p.calls++
	p.config = config
	return p.result, p.err
}

func TestObserverRecordsSuccessfulNonZeroUsageAndSnapshot(t *testing.T) {
	store := &memoryStore{}
	provider := &provider{result: &llm.StreamResult{TokenUsage: &llm.TokenUsage{InputTokens: 100, OutputTokens: 12, CacheCreationInputTokens: 7, CacheReadInputTokens: 40, TotalTokens: 152}}}
	service := NewService(store, settingsStore{})
	ctx := WithScope(context.Background(), Scope{RootSessionID: "root", SessionID: "child", RequestID: "turn", WorkingDirectory: "/work/project"})
	ctx = llm.WithRequestObserver(ctx, service.ObserveRequest)
	_, err := llm.NewFactory(provider).GetProvider(llm.ProviderAnthropic).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{Provider: llm.ProviderAnthropic, ConfigID: "model-config", Model: "claude", ContextSize: 10000, MaxOutputTokens: 1000, Purpose: "chat"}, nil, nil, llm.StreamCallbacks{})
	if err != nil || len(store.records) != 1 {
		t.Fatalf("records = %+v %v", store.records, err)
	}
	record := store.records[0]
	if record.RootSessionID != "root" || record.SessionID != "child" || record.WorkingDirectory != "/work/project" || record.ModelConfigID != "model-config" || record.TotalTokens != 119 || record.CacheReadInputTokens != 40 {
		t.Fatalf("snapshot = %+v", record)
	}
}

func TestObserverSkipsFailuresAndZeroUsage(t *testing.T) {
	store := &memoryStore{}
	provider := &provider{result: &llm.StreamResult{TokenUsage: &llm.TokenUsage{OutputTokens: 1}}, err: errors.New("failed")}
	ctx := llm.WithRequestObserver(WithScope(context.Background(), Scope{RootSessionID: "root"}), NewService(store, settingsStore{}).ObserveRequest)
	_, _ = llm.NewFactory(provider).GetProvider(llm.ProviderOpenAI).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{Purpose: "compaction"}, nil, nil, llm.StreamCallbacks{})
	provider.err = nil
	provider.result.TokenUsage = &llm.TokenUsage{}
	_, _ = llm.NewFactory(provider).GetProvider(llm.ProviderOpenAI).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{Purpose: "chat"}, nil, nil, llm.StreamCallbacks{})
	if len(store.records) != 0 {
		t.Fatalf("unexpected records: %+v", store.records)
	}
}

func TestObserverClampsRemainingOutputBudget(t *testing.T) {
	store := &memoryStore{records: []domain.UsageRecord{{RootSessionID: "root", OutputTokens: 30}}}
	provider := &provider{result: &llm.StreamResult{}}
	service := NewService(store, settingsStore{"sessionOutputTokenBudget": "100"})
	ctx := llm.WithRequestObserver(WithScope(context.Background(), Scope{RootSessionID: "root"}), service.ObserveRequest)
	_, err := llm.NewFactory(provider).GetProvider(llm.ProviderOpenAI).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{ContextSize: 10000, MaxOutputTokens: 500}, nil, nil, llm.StreamCallbacks{})
	if err != nil || provider.config.MaxOutputTokens != 70 {
		t.Fatalf("max output = %d %v", provider.config.MaxOutputTokens, err)
	}
}

func TestObserverPreservesAnthropicThinkingMinimum(t *testing.T) {
	store := &memoryStore{records: []domain.UsageRecord{{RootSessionID: "root", OutputTokens: 50}}}
	provider := &provider{result: &llm.StreamResult{}}
	service := NewService(store, settingsStore{"sessionOutputTokenBudget": "8000"})
	ctx := llm.WithRequestObserver(WithScope(context.Background(), Scope{RootSessionID: "root"}), service.ObserveRequest)
	_, err := llm.NewFactory(provider).GetProvider(llm.ProviderAnthropic).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{Provider: llm.ProviderAnthropic, ContextSize: 100000, MaxOutputTokens: 20000, ThinkingLevel: "low"}, nil, nil, llm.StreamCallbacks{})
	if !errors.Is(err, ErrSessionOutputTokenBudgetExceeded) || provider.calls != 0 {
		t.Fatalf("thinking minimum bypassed: calls=%d %v", provider.calls, err)
	}
	service.settings = settingsStore{"sessionOutputTokenBudget": "10000"}
	store.records[0].OutputTokens = 1
	_, err = llm.NewFactory(provider).GetProvider(llm.ProviderAnthropic).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{Provider: llm.ProviderAnthropic, ContextSize: 100000, MaxOutputTokens: 20000, ThinkingLevel: "low"}, nil, nil, llm.StreamCallbacks{})
	if err != nil || provider.config.MaxOutputTokens != 9999 {
		t.Fatalf("max output = %d %v", provider.config.MaxOutputTokens, err)
	}
}

func TestObserverBlocksExhaustedBudgetAndZeroDisablesIt(t *testing.T) {
	store := &memoryStore{records: []domain.UsageRecord{{RootSessionID: "root", OutputTokens: 10}}}
	provider := &provider{result: &llm.StreamResult{}}
	ctx := WithScope(context.Background(), Scope{RootSessionID: "root"})
	ctx = llm.WithRequestObserver(ctx, NewService(store, settingsStore{"sessionOutputTokenBudget": "10"}).ObserveRequest)
	_, err := llm.NewFactory(provider).GetProvider(llm.ProviderOpenAI).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{}, nil, nil, llm.StreamCallbacks{})
	if !errors.Is(err, ErrSessionOutputTokenBudgetExceeded) || provider.calls != 0 {
		t.Fatalf("exhausted budget: %d %v", provider.calls, err)
	}
	ctx = llm.WithRequestObserver(context.Background(), NewService(store, settingsStore{"sessionOutputTokenBudget": "0"}).ObserveRequest)
	_, err = llm.NewFactory(provider).GetProvider(llm.ProviderOpenAI).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{}, nil, nil, llm.StreamCallbacks{})
	if err != nil || provider.calls != 1 {
		t.Fatalf("unlimited budget: %d %v", provider.calls, err)
	}
}

func TestTitleIsRecordedWithoutBudgetCheck(t *testing.T) {
	store := &memoryStore{records: []domain.UsageRecord{{RootSessionID: "root", OutputTokens: 10}}}
	provider := &provider{result: &llm.StreamResult{TokenUsage: &llm.TokenUsage{InputTokens: 5, OutputTokens: 2}}}
	ctx := WithScope(context.Background(), Scope{RootSessionID: "root"})
	ctx = llm.WithRequestObserver(ctx, NewService(store, settingsStore{"sessionOutputTokenBudget": "1"}).ObserveRequest)
	_, err := llm.NewFactory(provider).GetProvider(llm.ProviderOpenAI).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{Purpose: "title"}, nil, nil, llm.StreamCallbacks{})
	if err != nil || provider.calls != 1 || len(store.records) != 2 || store.records[1].TotalTokens != 7 {
		t.Fatalf("title accounting: calls=%d records=%+v err=%v", provider.calls, store.records, err)
	}
}

func TestScopeFallsBackToPrivacySession(t *testing.T) {
	store := &memoryStore{}
	provider := &provider{result: &llm.StreamResult{TokenUsage: &llm.TokenUsage{OutputTokens: 1}}}
	ctx := llm.WithPrivacyScope(context.Background(), "privacy-root", "", "title")
	ctx = llm.WithRequestObserver(ctx, NewService(store, settingsStore{}).ObserveRequest)
	_, err := llm.NewFactory(provider).GetProvider(llm.ProviderOpenAI).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{Purpose: "title"}, nil, nil, llm.StreamCallbacks{})
	if err != nil || len(store.records) != 1 || store.records[0].RootSessionID != "privacy-root" {
		t.Fatalf("fallback: %+v %v", store.records, err)
	}
}

func TestSessionBudgetStatus(t *testing.T) {
	store := &memoryStore{
		records: []domain.UsageRecord{
			{RootSessionID: "root-1", SessionID: "root-1", OutputTokens: 1200},
			{RootSessionID: "root-1", SessionID: "child-1", OutputTokens: 350},
			{RootSessionID: "root-2", SessionID: "root-2", OutputTokens: 999},
		},
	}
	svc := NewService(store, settingsStore{"sessionOutputTokenBudget": "2000"})

	status, err := svc.SessionBudgetStatus(context.Background(), "root-1")
	if err != nil || status.BudgetTokens != 2000 || status.UsedTokens != 1550 {
		t.Fatalf("status = %+v %v", status, err)
	}

	emptyStatus, err := svc.SessionBudgetStatus(context.Background(), "")
	if err != nil || emptyStatus.BudgetTokens != 2000 || emptyStatus.UsedTokens != 0 {
		t.Fatalf("empty session status = %+v %v", emptyStatus, err)
	}

	svc.settings = settingsStore{"sessionOutputTokenBudget": "0"}
	unlimited, err := svc.SessionBudgetStatus(context.Background(), "root-1")
	if err != nil || unlimited.BudgetTokens != 0 || unlimited.UsedTokens != 1550 {
		t.Fatalf("unlimited status = %+v %v", unlimited, err)
	}
}
