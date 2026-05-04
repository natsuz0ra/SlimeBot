package config

import (
	"path/filepath"
	"testing"

	"slimebot/internal/runtime"
)

func TestLoadDefaultRuntimePaths(t *testing.T) {
	t.Setenv("DB_PATH", "")
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
