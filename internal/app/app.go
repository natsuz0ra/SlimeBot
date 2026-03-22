package app

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"slimebot/internal/auth"
	"slimebot/internal/config"
	"slimebot/internal/mcp"
	"slimebot/internal/platforms"
	"slimebot/internal/platforms/telegram"
	"slimebot/internal/repositories"
	"slimebot/internal/server/controller"
	"slimebot/internal/server/router"
	"slimebot/internal/server/ws"
	authsvc "slimebot/internal/services/auth"
	chatsvc "slimebot/internal/services/chat"
	configsvc "slimebot/internal/services/config"
	embsvc "slimebot/internal/services/embedding"
	memsvc "slimebot/internal/services/memory"
	oaisvc "slimebot/internal/services/openai"
	sessionsvc "slimebot/internal/services/session"
	settingssvc "slimebot/internal/services/settings"
	skillsvc "slimebot/internal/services/skill"
	"slimebot/web"
)

// App 应用根：HTTP 服务、Telegram、记忆/嵌入/向量与 MCP 等长生命周期资源。
type App struct {
	httpServer     *http.Server
	telegramWorker *telegram.Worker
	memoryService  *memsvc.MemoryService
	embedding      *embsvc.ONNXRuntimeEmbeddingService
	vectorRepo     *repositories.MemoryVectorRepository
	mcpManager     *mcp.Manager
}

func RunFromEnv() error {
	cfg := config.Load()

	if err := ValidateConfig(cfg); err != nil {
		return err
	}

	app, err := New(cfg)
	if err != nil {
		return err
	}

	appCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	return app.Run(appCtx)
}

// New 初始化目录、SQLite、各业务服务、路由与 http.Server。
func New(cfg config.Config) (*App, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), os.ModePerm); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.SkillsRoot, os.ModePerm); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.ChatUploadRoot, os.ModePerm); err != nil {
		return nil, err
	}

	db, err := repositories.NewSQLite(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	repo := repositories.New(db)
	authService := authsvc.NewAuthService(repo)
	if err := authService.EnsureDefaultAdmin(); err != nil {
		return nil, err
	}

	tokenManager, err := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTExpireMinutes)
	if err != nil {
		return nil, err
	}
	openaiClient := oaisvc.NewOpenAIClient()
	mcpManager := mcp.NewManager()
	settingsService := settingssvc.NewSettingsService(repo)
	sessionsService := sessionsvc.NewSessionService(repo)
	llmConfigsService := configsvc.NewLLMConfigService(repo)
	mcpConfigsService := configsvc.NewMCPConfigService(repo)
	platformsService := configsvc.NewMessagePlatformConfigService(repo)
	skillPackageService := skillsvc.NewSkillPackageService(repo, cfg.SkillsRoot)
	skillRuntimeService := skillsvc.NewSkillRuntimeService(repo, cfg.SkillsRoot)
	memoryService := memsvc.NewMemoryService(repo, openaiClient)
	// 记忆向量化依赖可选，未满足条件时保持 nil 并降级为关键词检索。
	embedding, vectorRepo := configureMemoryVectorization(cfg, memoryService)
	memoryService.WarmupTokenizer()
	chatUploadService := chatsvc.NewChatUploadService(cfg.ChatUploadRoot)
	chatService := chatsvc.NewChatService(repo, openaiClient, mcpManager, skillRuntimeService, memoryService, cfg.SystemPromptPath)
	chatService.SetUploadService(chatUploadService)

	approvalBroker := telegram.NewApprovalBroker()
	platformDispatcher := platforms.NewDispatcher(chatService, approvalBroker)
	telegramWorker := telegram.NewWorker(repo, platformDispatcher, chatUploadService)

	httpController := controller.NewHTTPController(
		authService,
		sessionsService,
		settingsService,
		llmConfigsService,
		mcpConfigsService,
		platformsService,
		skillPackageService,
		skillRuntimeService,
		chatUploadService,
		tokenManager,
	)
	wsController := ws.NewController(chatService)
	subDist, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		return nil, err
	}
	engine := router.New(cfg, tokenManager, httpController, wsController, subDist)

	addr := ":" + cfg.ServerPort
	slog.Info("server_listening", "addr", addr)

	return &App{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           engine,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		telegramWorker: telegramWorker,
		memoryService:  memoryService,
		embedding:      embedding,
		vectorRepo:     vectorRepo,
		mcpManager:     mcpManager,
	}, nil
}

// Run 启动 Telegram Worker 与 HTTP；ctx 取消时优雅关闭服务并 cleanup。
func (a *App) Run(ctx context.Context) error {
	a.telegramWorker.Start(ctx)
	err := runServerWithGracefulShutdown(ctx, a.httpServer)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	a.cleanup(shutdownCtx)
	return err
}

func (a *App) cleanup(ctx context.Context) {
	if a == nil {
		return
	}
	if a.memoryService != nil {
		// 先关闭 memory worker，避免写入过程中资源被释放。
		if err := a.memoryService.Shutdown(ctx); err != nil {
			slog.Warn("memory_shutdown", "err", err)
		}
	}
	if a.embedding != nil {
		if err := a.embedding.Close(); err != nil {
			slog.Warn("embedding_close", "err", err)
		}
	}
	if a.vectorRepo != nil {
		if err := a.vectorRepo.Close(); err != nil {
			slog.Warn("vector_repo_close", "err", err)
		}
	}
	if a.mcpManager != nil {
		a.mcpManager.CloseAll()
	}
}

func ValidateConfig(cfg config.Config) error {
	if strings.TrimSpace(cfg.JWTSecret) == "" {
		return errors.New("JWT_SECRET is not configured")
	}
	if cfg.JWTExpireMinutes <= 0 {
		return errors.New("JWT_EXPIRE must be greater than 0 (minutes)")
	}
	return nil
}

// configureMemoryVectorization 当 provider=onnx 且路径与 Qdrant 齐全时注入嵌入与向量库；否则返回 nil 并打日志。
// 该步骤只影响记忆检索能力，不阻塞主流程启动。
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

// runServerWithGracefulShutdown 监听 errCh；ctx 取消时在 5s 内 Shutdown。
func runServerWithGracefulShutdown(ctx context.Context, server *http.Server) error {
	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	case err := <-errCh:
		return err
	}
}
