package embedding

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureBgeM3ModelFilesDownloadsMissingFiles(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	modelPath := filepath.Join(dir, "onnx", "model.onnx")
	tokenizerPath := filepath.Join(dir, "onnx")
	err := EnsureBgeM3ModelFiles(context.Background(), BgeM3ModelConfig{
		ModelPath:       modelPath,
		TokenizerPath:   tokenizerPath,
		DownloadBaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mustExist(t, modelPath)
	mustExist(t, filepath.Join(filepath.Dir(modelPath), "model.onnx_data"))
	mustExist(t, filepath.Join(tokenizerPath, "tokenizer.json"))

	if len(hits) != 3 {
		t.Fatalf("unexpected request count: %d", len(hits))
	}
}

func TestEnsureBgeM3ModelFilesSkipWhenFilesExist(t *testing.T) {
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	modelPath := filepath.Join(dir, "onnx", "model.onnx")
	tokenizerJSONPath := filepath.Join(dir, "onnx", "tokenizer.json")
	modelDataPath := filepath.Join(filepath.Dir(modelPath), "model.onnx_data")
	mustWrite(t, modelPath, []byte("model"))
	mustWrite(t, modelDataPath, []byte("data"))
	mustWrite(t, tokenizerJSONPath, []byte("tok"))

	err := EnsureBgeM3ModelFiles(context.Background(), BgeM3ModelConfig{
		ModelPath:       modelPath,
		TokenizerPath:   tokenizerJSONPath,
		DownloadBaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hitCount != 0 {
		t.Fatalf("unexpected download hit count: %d", hitCount)
	}
}

func TestEnsureBgeM3ModelFilesTokenizerPathAsFile(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	modelPath := filepath.Join(dir, "onnx", "model.onnx")
	tokenizerFile := filepath.Join(dir, "custom", "my_tokenizer.json")
	err := EnsureBgeM3ModelFiles(context.Background(), BgeM3ModelConfig{
		ModelPath:       modelPath,
		TokenizerPath:   tokenizerFile,
		DownloadBaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mustExist(t, tokenizerFile)

	if len(paths) != 3 {
		t.Fatalf("unexpected request count: %d", len(paths))
	}
	if !containsPath(paths, "/tokenizer.json") {
		t.Fatalf("tokenizer download path missing: %#v", paths)
	}
}

func TestEnsureBgeM3ModelFilesDownloadFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	err := EnsureBgeM3ModelFiles(context.Background(), BgeM3ModelConfig{
		ModelPath:       filepath.Join(dir, "onnx", "model.onnx"),
		TokenizerPath:   filepath.Join(dir, "onnx"),
		DownloadBaseURL: srv.URL,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status=404") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func mustWrite(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if !isFile(path) {
		t.Fatalf("expected file to exist: %s", path)
	}
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
