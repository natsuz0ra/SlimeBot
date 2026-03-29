package app

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"slimebot/internal/config"
	"slimebot/internal/repositories"
	embsvc "slimebot/internal/services/embedding"
	memsvc "slimebot/internal/services/memory"
)

func configureMemoryVectorization(cfg config.Config, memoryService *memsvc.MemoryService) (*embsvc.ONNXRuntimeEmbeddingService, *repositories.MemoryVectorRepository) {
	if !strings.EqualFold(strings.TrimSpace(cfg.EmbeddingProvider), "onnx") {
		slog.Info("memory_vectorization_disabled", "reason", "embedding_provider", "provider", cfg.EmbeddingProvider)
		return nil, nil
	}
	if strings.TrimSpace(cfg.EmbeddingModelPath) == "" || strings.TrimSpace(cfg.EmbeddingTokenizerPath) == "" {
		slog.Info("memory_vectorization_disabled", "reason", "missing_embedding_paths")
		return nil, nil
	}
	if strings.TrimSpace(cfg.QdrantURL) == "" || strings.TrimSpace(cfg.QdrantCollection) == "" {
		slog.Info("memory_vectorization_disabled", "reason", "missing_qdrant_config")
		return nil, nil
	}
	embedding := embsvc.NewONNXRuntimeEmbeddingService(embsvc.ONNXRuntimeEmbeddingConfig{
		ModelPath:     cfg.EmbeddingModelPath,
		TokenizerPath: cfg.EmbeddingTokenizerPath,
		PythonBin:     cfg.EmbeddingPythonBin,
		ScriptPath:    cfg.EmbeddingScriptPath,
		Timeout:       time.Duration(cfg.EmbeddingTimeoutMS) * time.Millisecond,
	})
	if err := embedding.StartPipe(context.Background()); err != nil {
		slog.Warn("embedding_pipe_start_failed", "err", err)
	}
	memoryService.SetEmbeddingService(embedding)

	vectorStore, err := repositories.NewMemoryVectorRepository(cfg.QdrantURL, cfg.QdrantCollection)
	if err != nil {
		slog.Warn("memory_vectorization_disabled", "reason", "qdrant_init_failed", "err", err)
		return embedding, nil
	}
	memoryService.SetVectorStore(vectorStore)
	memoryService.SetVectorSearchTopK(cfg.MemoryVectorTopK)
	slog.Info("memory_vectorization_enabled",
		"provider", "onnx",
		"qdrant_url", cfg.QdrantURL,
		"collection", cfg.QdrantCollection,
		"topk", cfg.MemoryVectorTopK,
	)
	return embedding, vectorStore
}
