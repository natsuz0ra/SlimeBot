package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"slimebot/internal/runtime"
)

func TestLoadDefaultRuntimePaths(t *testing.T) {
	t.Setenv("DB_PATH", "")
	t.Setenv("SERVER_PORT", "")
	t.Setenv("SKILLS_ROOT", "")
	t.Setenv("CHAT_UPLOAD_ROOT", "")
	t.Setenv("EMBEDDING_MODEL_PATH", "")
	t.Setenv("EMBEDDING_TOKENIZER_PATH", "")
	t.Setenv("EMBEDDING_ORT_CACHE_DIR", "")
	t.Setenv("CHROMA_PATH", "")
	t.Setenv("CHROMA_COLLECTION", "")

	home := runtime.SlimeBotHomeDir()
	cfg := Load()

	if cfg.DBPath != filepath.Join(home, "storage", "data.db") {
		t.Fatalf("unexpected DB_PATH default: %s", cfg.DBPath)
	}
	if cfg.SkillsRoot != filepath.Join(home, "skills") {
		t.Fatalf("unexpected SKILLS_ROOT default: %s", cfg.SkillsRoot)
	}
	if cfg.ChatUploadRoot != filepath.Join(home, "storage", "chat_uploads") {
		t.Fatalf("unexpected CHAT_UPLOAD_ROOT default: %s", cfg.ChatUploadRoot)
	}
	if cfg.ServerPort != "6247" {
		t.Fatalf("unexpected SERVER_PORT default: %s", cfg.ServerPort)
	}
	if cfg.ContextHistoryRounds != 20 {
		t.Fatalf("unexpected CONTEXT_HISTORY_ROUNDS default: %d", cfg.ContextHistoryRounds)
	}
	if cfg.DefaultContextSize != 1_000_000 {
		t.Fatalf("unexpected DEFAULT_CONTEXT_SIZE default: %d", cfg.DefaultContextSize)
	}
}

func TestLoadExpandsTildePath(t *testing.T) {
	t.Setenv("DB_PATH", "~/.slimebot/storage/custom.db")
	t.Setenv("CONTEXT_HISTORY_ROUNDS", "30")
	t.Setenv("DEFAULT_CONTEXT_SIZE", "2048")
	cfg := Load()
	if cfg.DBPath == "~/.slimebot/storage/custom.db" {
		t.Fatalf("expected DB_PATH to expand home directory, got=%s", cfg.DBPath)
	}
	if cfg.ContextHistoryRounds != 30 {
		t.Fatalf("expected CONTEXT_HISTORY_ROUNDS=30, got=%d", cfg.ContextHistoryRounds)
	}
	if cfg.DefaultContextSize != 2048 {
		t.Fatalf("expected DEFAULT_CONTEXT_SIZE=2048, got=%d", cfg.DefaultContextSize)
	}
}

func TestLoadParsesHermesSkillRoots(t *testing.T) {
	t.Setenv("HERMES_SKILLS_ROOTS", "~/one"+string(os.PathListSeparator)+"/tmp/two")
	cfg := Load()
	if len(cfg.HermesSkillsRoots) != 2 {
		t.Fatalf("expected 2 Hermes roots, got %#v", cfg.HermesSkillsRoots)
	}
	if cfg.HermesSkillsRoots[0] == "~/one" {
		t.Fatalf("expected Hermes root to expand home directory")
	}
	if cfg.HermesSkillsRoots[1] != "/tmp/two" {
		t.Fatalf("unexpected second Hermes root: %s", cfg.HermesSkillsRoots[1])
	}
}

func TestLoadValidatedUsesDefaultsForMissingAndEmptyValues(t *testing.T) {
	clearValidatedEnv(t)
	t.Setenv("JWT_EXPIRE", "")
	t.Setenv("CONTEXT_HISTORY_ROUNDS", "")
	t.Setenv("DEFAULT_CONTEXT_SIZE", "")
	t.Setenv("SERVER_PORT", "  ")
	cfg, err := LoadValidated()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerPort != "6247" || cfg.JWTExpireMinutes != 15*24*60 || cfg.ContextHistoryRounds != 20 || cfg.DefaultContextSize != 1_000_000 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadValidatedAcceptsAndNormalizesExplicitValues(t *testing.T) {
	clearValidatedEnv(t)
	t.Setenv("SERVER_PORT", " 6400 ")
	t.Setenv("JWT_EXPIRE", "60")
	t.Setenv("CONTEXT_HISTORY_ROUNDS", "30")
	t.Setenv("DEFAULT_CONTEXT_SIZE", "2048")
	cfg, err := LoadValidated()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerPort != "6400" || cfg.JWTExpireMinutes != 60 || cfg.ContextHistoryRounds != 30 || cfg.DefaultContextSize != 2048 {
		t.Fatalf("explicit values were not preserved: %+v", cfg)
	}
}

func TestLoadValidatedRejectsExplicitInvalidValues(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{key: "JWT_EXPIRE", value: "abc"},
		{key: "JWT_EXPIRE", value: "0"},
		{key: "JWT_EXPIRE", value: "-1"},
		{key: "CONTEXT_HISTORY_ROUNDS", value: "abc"},
		{key: "CONTEXT_HISTORY_ROUNDS", value: "0"},
		{key: "DEFAULT_CONTEXT_SIZE", value: "abc"},
		{key: "DEFAULT_CONTEXT_SIZE", value: "0"},
		{key: "SERVER_PORT", value: "abc"},
		{key: "SERVER_PORT", value: "0"},
		{key: "SERVER_PORT", value: "65536"},
		{key: "SERVER_PORT", value: "-1"},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			clearValidatedEnv(t)
			t.Setenv(tt.key, tt.value)
			if _, err := LoadValidated(); err == nil || !strings.Contains(err.Error(), tt.key) {
				t.Fatalf("LoadValidated() error = %v, want error naming %s", err, tt.key)
			}
		})
	}
}

func TestLoadRetainsLegacyIntegerFallback(t *testing.T) {
	clearValidatedEnv(t)
	t.Setenv("JWT_EXPIRE", "abc")
	t.Setenv("CONTEXT_HISTORY_ROUNDS", "abc")
	t.Setenv("DEFAULT_CONTEXT_SIZE", "abc")
	cfg := Load()
	if cfg.JWTExpireMinutes != 15*24*60 || cfg.ContextHistoryRounds != 20 || cfg.DefaultContextSize != 1_000_000 {
		t.Fatalf("legacy defaults changed: %+v", cfg)
	}
}

func clearValidatedEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"SERVER_PORT", "JWT_EXPIRE", "CONTEXT_HISTORY_ROUNDS", "DEFAULT_CONTEXT_SIZE"} {
		t.Setenv(key, "")
	}
}
