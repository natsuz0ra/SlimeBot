package config

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"slimebot/internal/repositories"
)

func TestDiscoverModelsUsesSavedProviderCredentials(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("unexpected request: %s %#v", r.URL, r.Header)
		}
		fmt.Fprint(w, `{"data":[{"id":"z-model"},{"id":"a-model"}]}`)
	}))
	defer server.Close()
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "discover_openai"))
	svc := NewLLMConfigService(repo)
	provider, err := svc.CreateProvider(context.Background(), LLMProviderInput{Name: "Gateway", Protocol: "openai", BaseURL: server.URL + "/v1", APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	found, err := svc.DiscoverModels(context.Background(), provider.ID)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(found) != 2 || found[0].ID != "a-model" {
		t.Fatalf("unexpected discovery result: %+v", found)
	}
}

func TestDiscoverAnthropicModelsPaginatesAndKeepsContextSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("x-api-key") != "secret" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("unexpected request: %s %#v", r.URL, r.Header)
		}
		if r.URL.Query().Get("after_id") == "" {
			fmt.Fprint(w, `{"data":[{"id":"claude-a","display_name":"Claude A","max_input_tokens":200000}],"has_more":true,"last_id":"claude-a"}`)
			return
		}
		fmt.Fprint(w, `{"data":[{"id":"claude-b","max_input_tokens":100000}],"has_more":false}`)
	}))
	defer server.Close()
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "discover_anthropic"))
	svc := NewLLMConfigService(repo)
	provider, err := svc.CreateProvider(context.Background(), LLMProviderInput{Name: "Anthropic", Protocol: "anthropic", BaseURL: server.URL, APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	found, err := svc.DiscoverModels(context.Background(), provider.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 || found[0].ContextSize != 200000 || found[1].Name != "claude-b" {
		t.Fatalf("unexpected discovery result: %+v", found)
	}
}

func TestProviderKeyCanBeRetainedOrExplicitlyCleared(t *testing.T) {
	ctx := context.Background()
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "provider_key_update"))
	svc := NewLLMConfigService(repo)
	provider, err := svc.CreateProvider(ctx, LLMProviderInput{Name: "Gateway", Protocol: "openai", BaseURL: "http://localhost:11434/v1", APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := svc.Create(ctx, LLMConfigInput{ProviderID: provider.ID, Name: "Test", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateProvider(ctx, provider.ID, LLMProviderInput{Name: "Gateway", Protocol: "openai", BaseURL: "http://localhost:11434/v1"}); err != nil {
		t.Fatal(err)
	}
	retained, err := repo.GetLLMConfigByID(ctx, model.ID)
	if err != nil || retained.APIKey != "secret" {
		t.Fatalf("key was not retained: %+v, %v", retained, err)
	}
	if err := svc.UpdateProvider(ctx, provider.ID, LLMProviderInput{Name: "Gateway", Protocol: "openai", BaseURL: "http://localhost:11434/v1", ClearAPIKey: true}); err != nil {
		t.Fatal(err)
	}
	cleared, err := repo.GetLLMConfigByID(ctx, model.ID)
	if err != nil || cleared.APIKey != "no-key-required" {
		t.Fatalf("key was not cleared for keyless endpoint: %+v, %v", cleared, err)
	}
}
