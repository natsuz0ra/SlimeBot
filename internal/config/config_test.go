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
	if !cfg.MemoryAsyncWriteEnabled {
		t.Fatal("expected MEMORY_ASYNC_WRITE_ENABLED default true")
	}
	if cfg.MemoryAsyncWorkerIntervalSec != 2 {
		t.Fatalf("unexpected MEMORY_ASYNC_WORKER_INTERVAL default: %d", cfg.MemoryAsyncWorkerIntervalSec)
	}
	if cfg.MemoryWriteMaxRetries != 5 {
		t.Fatalf("unexpected MEMORY_WRITE_MAX_RETRIES default: %d", cfg.MemoryWriteMaxRetries)
	}
}

func TestLoadExpandsTildePath(t *testing.T) {
	t.Setenv("DB_PATH", "~/.slimebot/storage/custom.db")
	t.Setenv("CONTEXT_HISTORY_ROUNDS", "30")
	t.Setenv("MEMORY_ASYNC_WRITE_ENABLED", "false")
	t.Setenv("MEMORY_ASYNC_WORKER_INTERVAL", "7")
	t.Setenv("MEMORY_WRITE_MAX_RETRIES", "9")
	cfg := Load()
	if cfg.DBPath == "~/.slimebot/storage/custom.db" {
		t.Fatalf("expected DB_PATH to expand home directory, got=%s", cfg.DBPath)
	}
	if cfg.ContextHistoryRounds != 30 {
		t.Fatalf("expected CONTEXT_HISTORY_ROUNDS=30, got=%d", cfg.ContextHistoryRounds)
	}
	if cfg.MemoryAsyncWriteEnabled {
		t.Fatal("expected MEMORY_ASYNC_WRITE_ENABLED=false")
	}
	if cfg.MemoryAsyncWorkerIntervalSec != 7 {
		t.Fatalf("expected MEMORY_ASYNC_WORKER_INTERVAL=7, got=%d", cfg.MemoryAsyncWorkerIntervalSec)
	}
	if cfg.MemoryWriteMaxRetries != 9 {
		t.Fatalf("expected MEMORY_WRITE_MAX_RETRIES=9, got=%d", cfg.MemoryWriteMaxRetries)
	}
}
