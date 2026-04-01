package app

import (
	"io"
	"log/slog"
	"strings"
	"time"

	"slimebot/internal/config"
	"slimebot/internal/repositories"
	embsvc "slimebot/internal/services/embedding"
	memsvc "slimebot/internal/services/memory"
)

func configureMemoryVectorization(cfg config.Config, memoryService *memsvc.MemoryService) (io.Closer, *repositories.MemoryVectorRepository, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.EmbeddingProvider))
	if provider != "onnx_go" && provider != "onnx" {
		slog.Info("memory_vectorization_disabled", "reason", "embedding_provider", "provider", cfg.EmbeddingProvider)
		return nil, nil, nil
	}
	if strings.TrimSpace(cfg.EmbeddingModelPath) == "" || strings.TrimSpace(cfg.EmbeddingTokenizerPath) == "" {
		slog.Info("memory_vectorization_disabled", "reason", "missing_embedding_paths")
		return nil, nil, nil
	}
	if strings.TrimSpace(cfg.QdrantURL) == "" || strings.TrimSpace(cfg.QdrantCollection) == "" {
		slog.Info("memory_vectorization_disabled", "reason", "missing_qdrant_config")
		return nil, nil, nil
	}

	embedding, err := embsvc.NewONNXRuntimeGoEmbeddingService(embsvc.ONNXRuntimeGoEmbeddingConfig{
		ModelPath:        cfg.EmbeddingModelPath,
		TokenizerPath:    cfg.EmbeddingTokenizerPath,
		ORTSharedLibPath: cfg.EmbeddingORTLibPath,
		Timeout:          time.Duration(cfg.EmbeddingTimeoutMS) * time.Millisecond,
	})
	if err != nil {
		return nil, nil, err
	}
	memoryService.SetEmbeddingService(embedding)

	vectorStore, err := repositories.NewMemoryVectorRepository(cfg.QdrantURL, cfg.QdrantCollection)
	if err != nil {
		slog.Warn("memory_vectorization_disabled", "reason", "qdrant_init_failed", "err", err)
		return embedding, nil, nil
	}
	memoryService.SetVectorStore(vectorStore)
	memoryService.SetVectorSearchTopK(cfg.MemoryVectorTopK)
	slog.Info("memory_vectorization_enabled",
		"provider", "onnx_go",
		"qdrant_url", cfg.QdrantURL,
		"collection", cfg.QdrantCollection,
		"topk", cfg.MemoryVectorTopK,
	)
	return embedding, vectorStore, nil
}
