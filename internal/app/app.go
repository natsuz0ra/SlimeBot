package app

import (
	"context"
	"io/fs"
	"net"
	"net/http"
	"slimebot/internal/logging"
	"sync"
	"time"

	"slimebot/internal/auth"
	"slimebot/internal/config"
	"slimebot/internal/platforms"
	"slimebot/internal/platforms/telegram"
	"slimebot/internal/server/controller"
	"slimebot/internal/server/router"
	"slimebot/internal/server/ws"
	"slimebot/web"
)

// App 聚合进程级依赖，负责 HTTP 服务、平台 worker 与后台资源的生命周期。
type App struct {
	httpServer     *http.Server
	listener       net.Listener
	telegramWorker *telegram.Worker
	core           *Core
	cliToken       string
	startCancelMu  sync.Mutex
	startCancel    context.CancelFunc
}

// New 构建应用运行所需的仓储、服务、控制器与后台组件。
func New(cfg config.Config) (*App, error) {
	core, err := NewCore(cfg)
	if err != nil {
		return nil, err
	}
	tokenManager, err := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTExpireMinutes)
	if err != nil {
		return nil, err
	}

	approvalBroker := telegram.NewApprovalBroker()
	platformDispatcher := platforms.NewDispatcher(core.ChatService, approvalBroker)
	telegramWorker := telegram.NewWorker(core.Repo, platformDispatcher, core.ChatUpload)

	httpController := controller.NewHTTPController(
		core.AuthService,
		core.SessionService,
		core.SettingsService,
		core.LLMConfigService,
		core.MCPConfigService,
		core.PlatformService,
		core.SkillPackage,
		core.SkillRuntime,
		core.ChatUpload,
		tokenManager,
	)
	wsController := ws.NewController(core.ChatService)
	subDist, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		return nil, err
	}
	engine := router.New(cfg, tokenManager, httpController, wsController, subDist)

	addr := ":" + cfg.ServerPort
	logging.Info("server_listening", "addr", addr)

	app := &App{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           engine,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		telegramWorker: telegramWorker,
		core:           core,
	}
	core.WarmupInBackground(context.Background())

	core.ChatService.SetRunContext(buildRunContext(false))

	return app, nil
}

// NewHeadless 构建 CLI headless 模式的应用：无 Telegram、无 SPA，使用 CLI token 旁路认证。
// 返回应用实例和生成的 CLI token。
func NewHeadless(cfg config.Config) (*App, error) {
	core, err := NewCore(cfg)
	if err != nil {
		return nil, err
	}

	// CLI headless 模式自动生成 JWT secret（用户不需要配置）
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = auth.GenerateCLIToken()
	}
	tokenManager, err := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTExpireMinutes)
	if err != nil {
		return nil, err
	}

	cliToken := auth.GenerateCLIToken()

	httpController := controller.NewHTTPController(
		core.AuthService,
		core.SessionService,
		core.SettingsService,
		core.LLMConfigService,
		core.MCPConfigService,
		core.PlatformService,
		core.SkillPackage,
		core.SkillRuntime,
		core.ChatUpload,
		tokenManager,
	)
	wsController := ws.NewController(core.ChatService)
	subDist, _ := fs.Sub(web.DistFS, "dist") // 需要 fs 参数但不实际使用
	engine := router.New(cfg, tokenManager, httpController, wsController, subDist,
		router.RouterConfig{
			CLIToken: cliToken,
			Headless: true,
		},
	)

	app := &App{
		httpServer: &http.Server{
			Addr:              "127.0.0.1:0", // localhost only, OS assigns port
			Handler:           engine,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		core:     core,
		cliToken: cliToken,
	}
	core.WarmupInBackground(context.Background())

	core.ChatService.SetRunContext(buildRunContext(true))

	return app, nil
}

// CLIToken 返回 CLI 认证 token（仅 headless 模式有效）。
func (a *App) CLIToken() string {
	return a.cliToken
}

// Addr 返回 HTTP 服务器实际监听地址。必须在 Start 之后调用。
func (a *App) Addr() string {
	if a.listener == nil {
		return ""
	}
	return a.listener.Addr().String()
}

// Run 启动平台 worker 与 HTTP 服务，并在退出时统一触发资源清理。
func (a *App) Run(ctx context.Context) error {
	if a.telegramWorker != nil {
		a.telegramWorker.Start(ctx)
	}
	err := a.startHTTPServer(ctx)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	a.cleanup(shutdownCtx)
	return err
}

// Start 启动 HTTP 服务器（不阻塞），返回后可通过 Addr() 获取监听地址。
func (a *App) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.httpServer.Addr)
	if err != nil {
		return err
	}
	a.listener = ln

	go func() {
		if err := a.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			logging.Error("http_server_error", "err", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.httpServer.Shutdown(shutdownCtx)
	}()

	return nil
}

// Close 主动关闭应用资源。
func (a *App) Close(ctx context.Context) {
	a.startCancelMu.Lock()
	cancel := a.startCancel
	a.startCancel = nil
	a.startCancelMu.Unlock()
	if cancel != nil {
		cancel()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = a.httpServer.Shutdown(shutdownCtx)
	a.cleanup(ctx)
}

func (a *App) setStartCancel(cancel context.CancelFunc) {
	a.startCancelMu.Lock()
	defer a.startCancelMu.Unlock()
	a.startCancel = cancel
}

// startHTTPServer 启动 HTTP 服务并阻塞，直到上下文取消或服务出错。
func (a *App) startHTTPServer(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.httpServer.Addr)
	if err != nil {
		return err
	}
	a.listener = ln
	logging.Info("server_listening", "addr", ln.Addr().String())

	errCh := make(chan error, 1)
	go func() {
		if err := a.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	case err := <-errCh:
		return err
	}
}

// cleanup 关闭内存、向量化与 MCP 等后台资源，避免进程退出时遗留句柄。
func (a *App) cleanup(ctx context.Context) {
	if a == nil || a.core == nil {
		return
	}
	a.core.Close(ctx)
}
