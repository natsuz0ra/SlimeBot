package app

import (
	"context"
	"errors"
	"io/fs"
	"log"
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

	"github.com/joho/godotenv"
)

// App ????????????
type App struct {
	httpServer     *http.Server
	telegramWorker *telegram.Worker
}

// RunFromEnv ?? .env + ??????????HTTP + telegram worker??
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

// New ????????????????????????????
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
	// ??????? chatService?? WS chat ????? attachmentIds?
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
	log.Printf("server listening on %s", addr)

	return &App{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           engine,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		telegramWorker: telegramWorker,
	}, nil
}

// Run ?? Telegram worker ??? HTTP ???????
func (a *App) Run(ctx context.Context) error {
	a.telegramWorker.Start(ctx)
	return runServerWithGracefulShutdown(ctx, a.httpServer)
}

// ValidateConfig ?????????????
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

// runServerWithGracefulShutdown ????????????????? HTTP ???
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
