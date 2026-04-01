package app

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// App 聚合进程级依赖，负责 HTTP 服务、平台 worker 与后台资源的生命周期。
type App struct {
	httpServer     *http.Server
	telegramWorker *telegram.Worker
	memoryService  *memsvc.MemoryService
	embedding      io.Closer
	vectorRepo     *repositories.MemoryVectorRepository
	mcpManager     *mcp.Manager
}

// New 构建应用运行所需的仓储、服务、控制器与后台组件。
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
	if strings.EqualFold(strings.TrimSpace(cfg.EmbeddingProvider), "onnx_go") ||
		strings.EqualFold(strings.TrimSpace(cfg.EmbeddingProvider), "onnx") {
		libPath, err := embsvc.EnsureORTSharedLibrary(context.Background(), embsvc.ORTRuntimeConfig{
			Version:         cfg.EmbeddingORTVersion,
			CacheDir:        cfg.EmbeddingORTCacheDir,
			LibPath:         cfg.EmbeddingORTLibPath,
			DownloadBaseURL: cfg.EmbeddingORTDownloadBaseURL,
		})
		if err != nil {
			return nil, err
		}
		cfg.EmbeddingORTLibPath = libPath
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
	embedding, vectorRepo, err := configureMemoryVectorization(cfg, memoryService)
	if err != nil {
		return nil, err
	}
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

// Run 启动平台 worker 与 HTTP 服务，并在退出时统一触发资源清理。
func (a *App) Run(ctx context.Context) error {
	a.telegramWorker.Start(ctx)
	err := runServerWithGracefulShutdown(ctx, a.httpServer)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	a.cleanup(shutdownCtx)
	return err
}

// cleanup 关闭内存、向量化与 MCP 等后台资源，避免进程退出时遗留句柄。
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
