package embedding

import (
	"context"
	"errors"
	"testing"
)

func TestONNXRuntimeEmbeddingService_Embed(t *testing.T) {
	svc := NewONNXRuntimeEmbeddingService(ONNXRuntimeEmbeddingConfig{
		ModelPath:     "./models/bge-m3/model.onnx",
		TokenizerPath: "./models/bge-m3/tokenizer.json",
		PythonBin:     "python",
		ScriptPath:    "./scripts/onnx_embed_server.py",
		Runner: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte(`{"vectors":[[0.11,0.22,0.33]]}`), nil
		},
	})

	vector, err := svc.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if len(vector) != 3 {
		t.Fatalf("expected dim=3, got %d", len(vector))
	}
}

func TestONNXRuntimeEmbeddingService_EmbedBatch(t *testing.T) {
	svc := NewONNXRuntimeEmbeddingService(ONNXRuntimeEmbeddingConfig{
		ModelPath:     "./models/bge-m3/model.onnx",
		TokenizerPath: "./models/bge-m3/tokenizer.json",
		PythonBin:     "python",
		ScriptPath:    "./scripts/onnx_embed_server.py",
		Runner: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte(`{"vectors":[[0.1,0.2],[0.3,0.4]]}`), nil
		},
	})

	vectors, err := svc.EmbedBatch(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("embed batch failed: %v", err)
	}
	if len(vectors) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vectors))
	}
}

func TestONNXRuntimeEmbeddingService_EmbedBatchPropagatesRunnerError(t *testing.T) {
	svc := NewONNXRuntimeEmbeddingService(ONNXRuntimeEmbeddingConfig{
		ModelPath:     "./models/bge-m3/model.onnx",
		TokenizerPath: "./models/bge-m3/tokenizer.json",
		PythonBin:     "python",
		ScriptPath:    "./scripts/onnx_embed_server.py",
		Runner: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, errors.New("runner failed")
		},
	})

	_, err := svc.EmbedBatch(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("expected error from runner")
	}
}
