package repositories

import (
	"context"
	"testing"

	"slimebot/internal/domain"
)

func TestLegacyLLMProviderMigrationPreservesModelIDs(t *testing.T) {
	db := NewSQLiteDBTest(t, "legacy_llm_provider")
	original := []domain.LLMConfig{
		{ID: "model-one", Name: "Model One", Provider: "openai", BaseURL: "https://api.example.com/v1", APIKey: "secret", Model: "one"},
		{ID: "model-two", Name: "Model Two", Provider: "openai", BaseURL: "https://api.example.com/v1", APIKey: "secret", Model: "two"},
	}
	for _, item := range original {
		if err := db.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateLegacyLLMProviders(db); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyLLMProviders(db); err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	providers, err := repo.ListLLMProviders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || !providers[0].HasAPIKey {
		t.Fatalf("unexpected providers: %+v", providers)
	}
	models, err := repo.ListLLMConfigs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ProviderID != providers[0].ID || models[1].ProviderID != providers[0].ID {
		t.Fatalf("unexpected models: %+v", models)
	}
	if models[0].ProviderName != providers[0].Name {
		t.Fatalf("provider name missing from selectable model: %+v", models[0])
	}
	if models[0].ID != "model-one" || models[1].ID != "model-two" {
		t.Fatalf("model IDs changed: %+v", models)
	}

	if err := repo.UpdateLLMProvider(context.Background(), providers[0].ID, domain.LLMProvider{Name: "Renamed", Protocol: "anthropic", BaseURL: "https://new.example.com", APIKey: "new-secret"}); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.GetLLMConfigByID(context.Background(), "model-one")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Provider != "anthropic" || updated.BaseURL != "https://new.example.com" || updated.APIKey != "new-secret" {
		t.Fatalf("model connection not updated: %+v", updated)
	}
}
