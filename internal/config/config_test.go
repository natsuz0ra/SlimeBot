package config

import "testing"

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
