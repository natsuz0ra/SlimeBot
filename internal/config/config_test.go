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
	if cfg.EmbeddingModelPath != filepath.Join(home, "onnx", "model.onnx") {
		t.Fatalf("unexpected EMBEDDING_MODEL_PATH default: %s", cfg.EmbeddingModelPath)
	}
	if cfg.EmbeddingTokenizerPath != filepath.Join(home, "onnx", "tokenizer.json") {
		t.Fatalf("unexpected EMBEDDING_TOKENIZER_PATH default: %s", cfg.EmbeddingTokenizerPath)
	}
	if cfg.EmbeddingORTCacheDir != filepath.Join(home, "onnx", "runtime") {
		t.Fatalf("unexpected EMBEDDING_ORT_CACHE_DIR default: %s", cfg.EmbeddingORTCacheDir)
	}
	if cfg.ChromaPath != filepath.Join(home, "storage", "chroma") {
		t.Fatalf("unexpected CHROMA_PATH default: %s", cfg.ChromaPath)
	}
	if cfg.ChromaCollection != "session_memories" {
		t.Fatalf("unexpected CHROMA_COLLECTION default: %s", cfg.ChromaCollection)
	}
}

func TestLoadEmbeddingModelDownloadBaseURL(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_DOWNLOAD_BASE_URL", "")
	cfg := Load()
	if cfg.EmbeddingModelDownloadBaseURL != "https://huggingface.co/BAAI/bge-m3/resolve/main/onnx" {
		t.Fatalf("unexpected default download base url: %s", cfg.EmbeddingModelDownloadBaseURL)
	}
}

func TestLoadEmbeddingModelDownloadBaseURLOverride(t *testing.T) {
	const want = "https://example.com/model"
	t.Setenv("EMBEDDING_MODEL_DOWNLOAD_BASE_URL", want)
	cfg := Load()
	if cfg.EmbeddingModelDownloadBaseURL != want {
		t.Fatalf("unexpected override download base url: got=%s want=%s", cfg.EmbeddingModelDownloadBaseURL, want)
	}
}

func TestLoadExpandsTildePath(t *testing.T) {
	t.Setenv("DB_PATH", "~/.slimebot/storage/custom.db")
	cfg := Load()
	if cfg.DBPath == "~/.slimebot/storage/custom.db" {
		t.Fatalf("expected DB_PATH to expand home directory, got=%s", cfg.DBPath)
	}
}

func TestLoadChromaCollectionOverride(t *testing.T) {
	const want = "custom_collection"
	t.Setenv("CHROMA_COLLECTION", want)
	cfg := Load()
	if cfg.ChromaCollection != want {
		t.Fatalf("unexpected CHROMA_COLLECTION override: got=%s want=%s", cfg.ChromaCollection, want)
	}
}
