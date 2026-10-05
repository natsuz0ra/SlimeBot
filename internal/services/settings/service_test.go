package settings

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slimebot/internal/constants"
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

	envPath := filepath.Join(runtime.SlimeBotHomeDir(), "config.cfg")
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

func TestSettingsService_ProxyRoundTrip(t *testing.T) {
	t.Setenv("SLIMEBOT_HOME", t.TempDir())
	t.Cleanup(func() { _ = runtime.SetProxyURL("") })
	store := &memorySettingsStore{values: map[string]string{}}
	svc := NewSettingsService(store)
	proxy := "http://127.0.0.1:7890"
	if err := svc.Update(context.Background(), UpdateSettingsInput{ProxyURL: &proxy}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(context.Background())
	if err != nil || got.ProxyURL != proxy {
		t.Fatalf("proxy round trip = %v, %v", got, err)
	}
	invalid := "http://127.0.0.1"
	if err := svc.Update(context.Background(), UpdateSettingsInput{ProxyURL: &invalid}); err == nil {
		t.Fatal("expected invalid proxy error")
	}
	got, err = svc.Get(context.Background())
	if err != nil || got.ProxyURL != proxy {
		t.Fatalf("invalid proxy overwrote saved value: %v, %v", got, err)
	}
}

func TestSettingsService_UpdatePreservesOtherSettingsStoreWrites(t *testing.T) {
	store := &memorySettingsStore{values: map[string]string{}}
	svc := NewSettingsService(store)

	if err := svc.Update(context.Background(), UpdateSettingsInput{
		Language:                    stringPtr("en-US"),
		DefaultModel:                stringPtr("gpt-4.1"),
		MessagePlatformDefaultModel: stringPtr("gpt-4.1-mini"),
	}); err != nil {
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

func TestSettingsService_UpdateValidatesApprovalMode(t *testing.T) {
	store := &memorySettingsStore{values: map[string]string{}}
	svc := NewSettingsService(store)

	if err := svc.Update(context.Background(), UpdateSettingsInput{ApprovalMode: stringPtr(constants.ApprovalModeAutoReview)}); err != nil {
		t.Fatalf("Update auto_review failed: %v", err)
	}
	if got := store.values[constants.SettingApprovalMode]; got != constants.ApprovalModeAutoReview {
		t.Fatalf("approvalMode = %q, want %q", got, constants.ApprovalModeAutoReview)
	}

	err := svc.Update(context.Background(), UpdateSettingsInput{ApprovalMode: stringPtr("danger")})
	if err == nil {
		t.Fatal("expected invalid approval mode error")
	}
	if got := store.values[constants.SettingApprovalMode]; got != constants.ApprovalModeAutoReview {
		t.Fatalf("invalid mode should not overwrite existing value, got %q", got)
	}
}

func TestSettingsService_GetIncludesMessagePlatformRuntimeDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	envPath := filepath.Join(runtime.SlimeBotHomeDir(), "config.cfg")
	if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
		t.Fatalf("mkdir env dir failed: %v", err)
	}
	if err := os.WriteFile(envPath, nil, 0o644); err != nil {
		t.Fatalf("write env failed: %v", err)
	}

	store := &memorySettingsStore{values: map[string]string{}}
	svc := NewSettingsService(store)
	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.MessagePlatformThinkingLevel != "off" {
		t.Fatalf("message platform thinking level = %q, want off", got.MessagePlatformThinkingLevel)
	}
	if got.MessagePlatformApprovalMode != constants.ApprovalModeStandard {
		t.Fatalf("message platform approval mode = %q, want %q", got.MessagePlatformApprovalMode, constants.ApprovalModeStandard)
	}
}

func TestSettingsService_GetIncludesSandboxDefaults(t *testing.T) {
	store := &memorySettingsStore{values: map[string]string{}}
	svc := NewSettingsService(store)

	settings, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if settings.SandboxMode != "workspace-write" {
		t.Fatalf("sandbox mode = %q, want workspace-write", settings.SandboxMode)
	}
	if settings.SandboxNetworkEnabled != true {
		t.Fatal("sandbox network should default to enabled")
	}
	if settings.CLISandboxMode != "workspace-write" {
		t.Fatalf("cli sandbox mode = %q, want workspace-write", settings.CLISandboxMode)
	}
	if settings.CLISandboxNetworkEnabled != true {
		t.Fatal("cli sandbox network should default to enabled")
	}
	if len(settings.CLISandboxWritableRoots) != 0 {
		t.Fatalf("cli sandbox writable roots = %#v, want empty", settings.CLISandboxWritableRoots)
	}
	if len(settings.CLISandboxNetworkAllowedDomains) != 0 {
		t.Fatalf("cli sandbox allowed domains = %#v, want empty", settings.CLISandboxNetworkAllowedDomains)
	}
}

func TestSettingsService_GetIncludesMemoryDefaults(t *testing.T) {
	store := &memorySettingsStore{values: map[string]string{}}
	svc := NewSettingsService(store)

	settings, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !settings.MemoryEnabled {
		t.Fatal("memory should default to enabled")
	}
	if !settings.MemoryUserProfileEnabled {
		t.Fatal("user profile memory should default to enabled")
	}
	if settings.MemoryCharLimit != 2200 {
		t.Fatalf("memory char limit = %d, want 2200", settings.MemoryCharLimit)
	}
	if settings.MemoryUserCharLimit != 1375 {
		t.Fatalf("user char limit = %d, want 1375", settings.MemoryUserCharLimit)
	}
	if settings.MemoryNudgeInterval != 10 {
		t.Fatalf("nudge interval = %d, want 10", settings.MemoryNudgeInterval)
	}
}

func TestSettingsService_UpdateValidatesMemorySettings(t *testing.T) {
	store := &memorySettingsStore{values: map[string]string{}}
	svc := NewSettingsService(store)
	enabled := false
	userEnabled := true
	charLimit := 3000
	userLimit := 1500
	interval := 5

	err := svc.Update(context.Background(), UpdateSettingsInput{
		MemoryEnabled:            &enabled,
		MemoryUserProfileEnabled: &userEnabled,
		MemoryCharLimit:          &charLimit,
		MemoryUserCharLimit:      &userLimit,
		MemoryNudgeInterval:      &interval,
	})
	if err != nil {
		t.Fatalf("Update memory settings failed: %v", err)
	}
	if store.values[constants.SettingMemoryEnabled] != "false" {
		t.Fatalf("memory enabled = %q", store.values[constants.SettingMemoryEnabled])
	}
	if store.values[constants.SettingMemoryUserProfileEnabled] != "true" {
		t.Fatalf("user profile enabled = %q", store.values[constants.SettingMemoryUserProfileEnabled])
	}
	if store.values[constants.SettingMemoryCharLimit] != "3000" {
		t.Fatalf("memory char limit = %q", store.values[constants.SettingMemoryCharLimit])
	}
	if store.values[constants.SettingMemoryUserCharLimit] != "1500" {
		t.Fatalf("user char limit = %q", store.values[constants.SettingMemoryUserCharLimit])
	}
	if store.values[constants.SettingMemoryNudgeInterval] != "5" {
		t.Fatalf("nudge interval = %q", store.values[constants.SettingMemoryNudgeInterval])
	}

	badLimit := 199
	if err := svc.Update(context.Background(), UpdateSettingsInput{MemoryCharLimit: &badLimit}); err == nil {
		t.Fatal("expected invalid memory char limit error")
	}
	badInterval := 101
	if err := svc.Update(context.Background(), UpdateSettingsInput{MemoryNudgeInterval: &badInterval}); err == nil {
		t.Fatal("expected invalid nudge interval error")
	}
}

func TestSettingsService_UpdateValidatesSandboxMode(t *testing.T) {
	store := &memorySettingsStore{values: map[string]string{}}
	svc := NewSettingsService(store)

	if err := svc.Update(context.Background(), UpdateSettingsInput{SandboxMode: stringPtr("read-only")}); err != nil {
		t.Fatalf("Update read-only failed: %v", err)
	}
	if got := store.values[constants.SettingSandboxMode]; got != "read-only" {
		t.Fatalf("sandbox mode = %q, want read-only", got)
	}

	err := svc.Update(context.Background(), UpdateSettingsInput{SandboxMode: stringPtr("root")})
	if err == nil {
		t.Fatal("expected invalid sandbox mode error")
	}
	if got := store.values[constants.SettingSandboxMode]; got != "read-only" {
		t.Fatalf("invalid update should preserve sandbox mode, got %q", got)
	}
}

func TestSettingsService_UpdateValidatesCLISandboxMode(t *testing.T) {
	store := &memorySettingsStore{values: map[string]string{}}
	svc := NewSettingsService(store)

	if err := svc.Update(context.Background(), UpdateSettingsInput{CLISandboxMode: stringPtr("danger-full-access")}); err != nil {
		t.Fatalf("Update CLI sandbox mode failed: %v", err)
	}
	if got := store.values[constants.SettingCLISandboxMode]; got != "danger-full-access" {
		t.Fatalf("cli sandbox mode = %q, want danger-full-access", got)
	}

	err := svc.Update(context.Background(), UpdateSettingsInput{CLISandboxMode: stringPtr("root")})
	if err == nil {
		t.Fatal("expected invalid CLI sandbox mode error")
	}
	if got := store.values[constants.SettingCLISandboxMode]; got != "danger-full-access" {
		t.Fatalf("invalid update should preserve CLI sandbox mode, got %q", got)
	}
}

func TestSettingsService_UpdateStoresCLISandboxFieldsSeparately(t *testing.T) {
	store := &memorySettingsStore{values: map[string]string{}}
	svc := NewSettingsService(store)
	roots := []string{"/tmp/cli-work"}
	domains := []string{"api.example.com"}
	network := false

	if err := svc.Update(context.Background(), UpdateSettingsInput{
		CLISandboxWritableRoots:         &roots,
		CLISandboxNetworkEnabled:        &network,
		CLISandboxNetworkAllowedDomains: &domains,
	}); err != nil {
		t.Fatalf("Update CLI sandbox fields failed: %v", err)
	}

	if got := store.values[constants.SettingCLISandboxWritableRoots]; got != `["/tmp/cli-work"]` {
		t.Fatalf("cli sandbox writable roots = %q", got)
	}
	if got := store.values[constants.SettingCLISandboxNetworkEnabled]; got != "false" {
		t.Fatalf("cli sandbox network enabled = %q", got)
	}
	if got := store.values[constants.SettingCLISandboxNetworkAllowedDomains]; got != `["api.example.com"]` {
		t.Fatalf("cli sandbox allowed domains = %q", got)
	}
	if _, exists := store.values[constants.SettingSandboxWritableRoots]; exists {
		t.Fatalf("CLI update should not write web sandbox roots: %#v", store.values)
	}
}

func TestSettingsService_UpdateMessagePlatformRuntimeSettings(t *testing.T) {
	store := &memorySettingsStore{values: map[string]string{}}
	svc := NewSettingsService(store)

	err := svc.Update(context.Background(), UpdateSettingsInput{
		MessagePlatformDefaultModel:  stringPtr(""),
		MessagePlatformThinkingLevel: stringPtr("high"),
		MessagePlatformApprovalMode:  stringPtr(constants.ApprovalModeAutoReview),
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if got := store.values[constants.SettingMessagePlatformDefaultModel]; got != "" {
		t.Fatalf("message platform default model = %q, want empty", got)
	}
	if got := store.values[constants.SettingMessagePlatformThinkingLevel]; got != "high" {
		t.Fatalf("message platform thinking level = %q, want high", got)
	}
	if got := store.values[constants.SettingMessagePlatformApprovalMode]; got != constants.ApprovalModeAutoReview {
		t.Fatalf("message platform approval mode = %q, want %q", got, constants.ApprovalModeAutoReview)
	}
}

func TestSettingsService_UpdateValidatesMessagePlatformApprovalMode(t *testing.T) {
	store := &memorySettingsStore{values: map[string]string{}}
	svc := NewSettingsService(store)

	err := svc.Update(context.Background(), UpdateSettingsInput{MessagePlatformApprovalMode: stringPtr("danger")})
	if err == nil {
		t.Fatal("expected invalid message platform approval mode error")
	}
}

func stringPtr(value string) *string {
	return &value
}
