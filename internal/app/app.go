package app

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
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

type App struct {
	httpServer     *http.Server
	telegramWorker *telegram.Worker
	memoryService  *memsvc.MemoryService
	embedding      *embsvc.ONNXRuntimeEmbeddingService
	vectorRepo     *repositories.MemoryVectorRepository
	mcpManager     *mcp.Manager
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
