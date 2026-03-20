package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"slimebot/backend/internal/auth"
	"slimebot/backend/internal/config"
	"slimebot/backend/internal/mcp"
	"slimebot/backend/internal/platforms"
	"slimebot/backend/internal/platforms/telegram"
	"slimebot/backend/internal/repositories"
	"slimebot/backend/internal/server/controller"
	"slimebot/backend/internal/server/router"
	"slimebot/backend/internal/server/ws"
	authsvc "slimebot/backend/internal/services/auth"
	chatsvc "slimebot/backend/internal/services/chat"
	configsvc "slimebot/backend/internal/services/config"
	embsvc "slimebot/backend/internal/services/embedding"
	memsvc "slimebot/backend/internal/services/memory"
	oaisvc "slimebot/backend/internal/services/openai"
	sessionsvc "slimebot/backend/internal/services/session"
	settingssvc "slimebot/backend/internal/services/settings"
	skillsvc "slimebot/backend/internal/services/skill"

	"github.com/joho/godotenv"
)

type App struct {
	httpServer     *http.Server
	telegramWorker *telegram.Worker
}

// RunFromEnv 加载 .env + 配置并启动整个应用（HTTP + telegram worker）。
func RunFromEnv() error {
	_ = godotenv.Load()
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
	configureMemoryVectorization(cfg, memoryService)
	chatUploadService := chatsvc.NewChatUploadService(cfg.ChatUploadRoot)
	chatService := chatsvc.NewChatService(repo, openaiClient, mcpManager, skillRuntimeService, memoryService, cfg.SystemPromptPath)
	// 将附件服务注入 chatService，使 WS chat 链路可消费 attachmentIds。
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
	engine := router.New(cfg, tokenManager, httpController, wsController)

	addr := ":" + cfg.ServerPort
	log.Printf("server listening on %s", addr)

	return &App{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: engine,
		},
		telegramWorker: telegramWorker,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	a.telegramWorker.Start(ctx)
	return runServerWithGracefulShutdown(ctx, a.httpServer)
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

func configureMemoryVectorization(cfg config.Config, memoryService *memsvc.MemoryService) {
	if !strings.EqualFold(strings.TrimSpace(cfg.EmbeddingProvider), "onnx") {
		log.Printf("memory_vectorization_disabled reason=embedding_provider provider=%q", cfg.EmbeddingProvider)
		return
	}
	if strings.TrimSpace(cfg.EmbeddingModelPath) == "" || strings.TrimSpace(cfg.EmbeddingTokenizerPath) == "" {
		log.Printf("memory_vectorization_disabled reason=missing_embedding_paths")
		return
	}
	if strings.TrimSpace(cfg.QdrantURL) == "" || strings.TrimSpace(cfg.QdrantCollection) == "" {
		log.Printf("memory_vectorization_disabled reason=missing_qdrant_config")
		return
	}
	embedding := embsvc.NewONNXRuntimeEmbeddingService(embsvc.ONNXRuntimeEmbeddingConfig{
		ModelPath:     cfg.EmbeddingModelPath,
		TokenizerPath: cfg.EmbeddingTokenizerPath,
		PythonBin:     cfg.EmbeddingPythonBin,
		ScriptPath:    cfg.EmbeddingScriptPath,
		Timeout:       time.Duration(cfg.EmbeddingTimeoutMS) * time.Millisecond,
	})
	memoryService.SetEmbeddingService(embedding)

	vectorStore, err := repositories.NewMemoryVectorRepository(cfg.QdrantURL, cfg.QdrantCollection)
	if err != nil {
		log.Printf("memory_vectorization_disabled reason=qdrant_init_failed err=%v", err)
		return
	}
	memoryService.SetVectorStore(vectorStore)
	memoryService.SetVectorSearchTopK(cfg.MemoryVectorTopK)
	log.Printf(
		"memory_vectorization_enabled provider=onnx qdrant_url=%s collection=%s topk=%d",
		cfg.QdrantURL,
		cfg.QdrantCollection,
		cfg.MemoryVectorTopK,
	)
}

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
