package app

import (
	"context"
	"io"
	"strings"
	"time"

	"slimebot/internal/config"
	"slimebot/internal/logging"
	"slimebot/internal/repositories"
	embsvc "slimebot/internal/services/embedding"
)

func initEmbeddingService(ctx context.Context, cfg config.Config) (embsvc.EmbeddingService, io.Closer, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.EmbeddingProvider))
	if provider != "onnx_go" && provider != "onnx" {
		logging.Info("memory_vectorization_disabled", "reason", "embedding_provider", "provider", cfg.EmbeddingProvider)
		return nil, nil, nil
	}
	if strings.TrimSpace(cfg.EmbeddingModelPath) == "" || strings.TrimSpace(cfg.EmbeddingTokenizerPath) == "" {
		logging.Info("memory_vectorization_disabled", "reason", "missing_embedding_paths")
		return nil, nil, nil
	}

	libPath, err := embsvc.EnsureORTSharedLibrary(ctx, embsvc.ORTRuntimeConfig{
		Version:         cfg.EmbeddingORTVersion,
		CacheDir:        cfg.EmbeddingORTCacheDir,
		LibPath:         cfg.EmbeddingORTLibPath,
		DownloadBaseURL: cfg.EmbeddingORTDownloadBaseURL,
	})
	if err != nil {
		return nil, nil, err
	}
	cfg.EmbeddingORTLibPath = libPath

	if err := embsvc.EnsureBgeM3ModelFiles(ctx, embsvc.BgeM3ModelConfig{
		ModelPath:       cfg.EmbeddingModelPath,
		TokenizerPath:   cfg.EmbeddingTokenizerPath,
		DownloadBaseURL: cfg.EmbeddingModelDownloadBaseURL,
	}); err != nil {
		return nil, nil, err
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
	return embedding, embedding, nil
}

func initVectorStore(_ context.Context, cfg config.Config) (*repositories.MemoryVectorRepository, error) {
	if strings.TrimSpace(cfg.ChromaPath) == "" || strings.TrimSpace(cfg.ChromaCollection) == "" {
		logging.Info("memory_vectorization_disabled", "reason", "missing_chroma_config")
		return nil, nil
	}

	vectorStore, err := repositories.NewMemoryVectorRepository(cfg.ChromaPath, cfg.ChromaCollection)
	if err != nil {
		return nil, err
	}
	return vectorStore, nil
}
