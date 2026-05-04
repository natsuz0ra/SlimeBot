package config

import (
	"context"
	"testing"

	"slimebot/internal/domain"
)

type llmConfigStoreStub struct {
	created  domain.LLMConfig
	updated  domain.LLMConfig
	updateID string
}

func (s *llmConfigStoreStub) ListLLMConfigs(context.Context) ([]domain.LLMConfig, error) {
	return []domain.LLMConfig{s.created}, nil
}

func (s *llmConfigStoreStub) CreateLLMConfig(_ context.Context, item domain.LLMConfig) (*domain.LLMConfig, error) {
	s.created = item
	return &item, nil
}

func (s *llmConfigStoreStub) UpdateLLMConfig(_ context.Context, id string, item domain.LLMConfig) error {
	s.updateID = id
	s.updated = item
	return nil
}

func (s *llmConfigStoreStub) DeleteLLMConfig(context.Context, string) error {
	return nil
}

func TestLLMConfigService_DefaultsContextSize(t *testing.T) {
	store := &llmConfigStoreStub{}
	svc := NewLLMConfigService(store, 1_000_000)

	item, err := svc.Create(context.Background(), LLMConfigCreateInput{
		Name:    "test",
		BaseURL: "http://fake",
		APIKey:  "key",
		Model:   "fake-model",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if item.ContextSize != 1_000_000 {
		t.Fatalf("expected default context size, got %d", item.ContextSize)
	}
}

func TestLLMConfigService_UsesPositiveContextSize(t *testing.T) {
	store := &llmConfigStoreStub{}
	svc := NewLLMConfigService(store, 1_000_000)

	item, err := svc.Create(context.Background(), LLMConfigCreateInput{
		Name:        "test",
		BaseURL:     "http://fake",
		APIKey:      "key",
		Model:       "fake-model",
		ContextSize: 2048,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if item.ContextSize != 2048 {
		t.Fatalf("expected explicit context size, got %d", item.ContextSize)
	}
}

func TestLLMConfigService_UpdateTrimsFieldsAndDefaultsContextSize(t *testing.T) {
	store := &llmConfigStoreStub{}
	svc := NewLLMConfigService(store, 1_000_000)

	err := svc.Update(context.Background(), "model-1", LLMConfigInput{
		Name:        "  test  ",
		Provider:    "  anthropic  ",
		BaseURL:     "  http://fake  ",
		APIKey:      "  key  ",
		Model:       "  fake-model  ",
		ContextSize: -1,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if store.updateID != "model-1" {
		t.Fatalf("expected update id model-1, got %q", store.updateID)
	}
	if store.updated.Name != "test" || store.updated.Provider != "anthropic" || store.updated.BaseURL != "http://fake" || store.updated.APIKey != "key" || store.updated.Model != "fake-model" {
		t.Fatalf("expected trimmed update payload, got %+v", store.updated)
	}
	if store.updated.ContextSize != 1_000_000 {
		t.Fatalf("expected default context size, got %d", store.updated.ContextSize)
	}
}
