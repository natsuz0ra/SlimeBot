package repositories

import (
	"context"
	"testing"

	"slimebot/internal/domain"
)

func TestSessionContextSummaryLifecycle(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "context_summary"))
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "summary")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if _, err := repo.GetSessionContextSummary(ctx, session.ID, ""); err == nil {
		t.Fatal("expected missing summary to return an error")
	}

	first := &domain.SessionContextSummary{
		SessionID:               session.ID,
		ModelConfigID:           "",
		Summary:                 "first summary",
		SummarizedUntilSeq:      4,
		PreCompactTokenEstimate: 100,
	}
	if err := repo.UpsertSessionContextSummary(ctx, first); err != nil {
		t.Fatalf("UpsertSessionContextSummary first failed: %v", err)
	}
	got, err := repo.GetSessionContextSummary(ctx, session.ID, "")
	if err != nil {
		t.Fatalf("GetSessionContextSummary failed: %v", err)
	}
	if got.Summary != "first summary" || got.SummarizedUntilSeq != 4 {
		t.Fatalf("unexpected first summary: %+v", got)
	}

	second := &domain.SessionContextSummary{
		SessionID:               session.ID,
		ModelConfigID:           "",
		Summary:                 "second summary",
		SummarizedUntilSeq:      8,
		PreCompactTokenEstimate: 200,
	}
	if err := repo.UpsertSessionContextSummary(ctx, second); err != nil {
		t.Fatalf("UpsertSessionContextSummary second failed: %v", err)
	}
	got, err = repo.GetSessionContextSummary(ctx, session.ID, "")
	if err != nil {
		t.Fatalf("GetSessionContextSummary after update failed: %v", err)
	}
	if got.Summary != "second summary" || got.SummarizedUntilSeq != 8 {
		t.Fatalf("unexpected updated summary: %+v", got)
	}
}

func TestLLMConfigUpdatePersistsContextSize(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "llm_config_update"))
	ctx := context.Background()

	item, err := repo.CreateLLMConfig(ctx, domain.LLMConfig{
		Name:        "Old",
		Provider:    "openai",
		BaseURL:     "http://old",
		APIKey:      "old-key",
		Model:       "old-model",
		ContextSize: 1_000_000,
	})
	if err != nil {
		t.Fatalf("CreateLLMConfig failed: %v", err)
	}

	err = repo.UpdateLLMConfig(ctx, item.ID, domain.LLMConfig{
		Name:        "New",
		Provider:    "anthropic",
		BaseURL:     "http://new",
		APIKey:      "new-key",
		Model:       "new-model",
		ContextSize: 128_000,
	})
	if err != nil {
		t.Fatalf("UpdateLLMConfig failed: %v", err)
	}

	got, err := repo.GetLLMConfigByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetLLMConfigByID failed: %v", err)
	}
	if got.Name != "New" || got.Provider != "anthropic" || got.BaseURL != "http://new" || got.APIKey != "new-key" || got.Model != "new-model" {
		t.Fatalf("unexpected updated config: %+v", got)
	}
	if got.ContextSize != 128_000 {
		t.Fatalf("expected context size 128000, got %d", got.ContextSize)
	}
}
