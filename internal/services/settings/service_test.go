package settings

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slimebot/internal/runtime"
	"testing"
)

type memorySettingsStore struct {
	values map[string]string
}

func (m *memorySettingsStore) GetSetting(_ context.Context, key string) (string, error) {
	return m.values[key], nil
}

func (m *memorySettingsStore) SetSetting(_ context.Context, key, value string) error {
	m.values[key] = value
	return nil
}

func TestSettingsService_GetIncludesWebSearchAPIKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	envPath := filepath.Join(runtime.SlimeBotHomeDir(), ".env")
	if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
		t.Fatalf("mkdir env dir failed: %v", err)
	}
	if err := os.WriteFile(envPath, []byte("WEB_SEARCH_API_KEY=test-key\n"), 0o644); err != nil {
		t.Fatalf("write env failed: %v", err)
	}
	store := &memorySettingsStore{values: map[string]string{"language": "en-US", "defaultModel": "gpt", "messagePlatformDefaultModel": "mp"}}
	svc := NewSettingsService(store)

	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.WebSearchAPIKey != "test-key" {
		t.Fatalf("expected web search key, got %q", got.WebSearchAPIKey)
	}
}

func TestSettingsService_UpdatePreservesOtherSettingsStoreWrites(t *testing.T) {
	store := &memorySettingsStore{values: map[string]string{}}
	svc := NewSettingsService(store)

	if err := svc.Update(context.Background(), UpdateSettingsInput{Language: "en-US", DefaultModel: "gpt-4.1", MessagePlatformDefaultModel: "gpt-4.1-mini"}); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if store.values["language"] != "en-US" || store.values["defaultModel"] != "gpt-4.1" || store.values["messagePlatformDefaultModel"] != "gpt-4.1-mini" {
		t.Fatalf("unexpected settings store values: %#v", store.values)
	}
}

func TestSettingsService_GetReturnsEnvErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	store := &memorySettingsStore{values: map[string]string{}}
	svc := NewSettingsService(store)

	_, err := svc.Get(context.Background())
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}
