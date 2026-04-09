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
}

func TestLoadExpandsTildePath(t *testing.T) {
	t.Setenv("DB_PATH", "~/.slimebot/storage/custom.db")
	cfg := Load()
	if cfg.DBPath == "~/.slimebot/storage/custom.db" {
		t.Fatalf("expected DB_PATH to expand home directory, got=%s", cfg.DBPath)
	}
}
