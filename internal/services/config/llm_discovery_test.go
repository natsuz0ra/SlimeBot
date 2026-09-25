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

func TestDiscoverModelsReadsCompatibleContextMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"a","context_length":131072},{"id":"b","max_model_len":"262144"},{"id":"c","metadata":{"context_window":32768}},{"id":"d"},{"id":"e","metadata":"unexpected"}]}`)
	}))
	defer server.Close()
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "discover_context_metadata"))
	svc := NewLLMConfigService(repo)
	provider, err := svc.CreateProvider(context.Background(), LLMProviderInput{Name: "Gateway", Protocol: "openai", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	models, err := svc.DiscoverModels(context.Background(), provider.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{131072, 262144, 32768, 0, 0} {
		if models[i].ContextSize != want {
			t.Fatalf("%s: expected %d, got %d", models[i].ID, want, models[i].ContextSize)
		}
	}
}

func TestCreateModelAutomaticallyDetectsContextAndMarksFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"known","context_length":128000},{"id":"unknown"}]}`)
	}))
	defer server.Close()
	ctx := context.Background()
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "auto_context_save"))
	svc := NewLLMConfigService(repo)
	provider, err := svc.CreateProvider(ctx, LLMProviderInput{Name: "Gateway", Protocol: "openai", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	known, err := svc.Create(ctx, LLMConfigInput{ProviderID: provider.ID, Name: "Known", Model: "known", ContextSizeSource: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := svc.Create(ctx, LLMConfigInput{ProviderID: provider.ID, Name: "Unknown", Model: "unknown", ContextSizeSource: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if known.ContextSize != 128000 || known.ContextSizeSource != "detected" {
		t.Fatalf("unexpected detected context: %+v", known)
	}
	if unknown.ContextSize != 1_000_000 || unknown.ContextSizeSource != "fallback" {
		t.Fatalf("unexpected fallback context: %+v", unknown)
	}
	stored, err := repo.GetLLMConfigByID(ctx, known.ID)
	if err != nil || stored.ContextSizeSource != "detected" {
		t.Fatalf("detected source was not persisted: %+v, %v", stored, err)
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
