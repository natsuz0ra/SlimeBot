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

// App holds process-level dependencies and manages the HTTP server, platform workers, and background resource lifecycle.
type App struct {
	httpServer     *http.Server
	listener       net.Listener
	telegramWorker *telegram.Worker
	core           *Core
	cliToken       string
	startCancelMu  sync.Mutex
	startCancel    context.CancelFunc
}

// New builds repositories, services, controller, and background components needed to run the app.
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
		core.PlanService,
		core.SkillPackage,
		core.SkillRuntime,
		core.ChatUpload,
		tokenManager,
	)
	httpController.SetChatContextUsageService(core.ChatService)
	wsController := ws.NewController(core.ChatService, core.PlanService)
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

// NewHeadless builds a CLI headless app: no Telegram, no SPA; uses CLI token bypass auth.
// Returns the app instance and the generated CLI token.
func NewHeadless(cfg config.Config) (*App, error) {
	core, err := NewCore(cfg)
	if err != nil {
		return nil, err
	}

	// CLI headless mode auto-generates JWT secret (no user configuration required).
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
		core.PlanService,
		core.SkillPackage,
		core.SkillRuntime,
		core.ChatUpload,
		tokenManager,
	)
	httpController.SetChatContextUsageService(core.ChatService)
	wsController := ws.NewController(core.ChatService, core.PlanService)
	subDist, _ := fs.Sub(web.DistFS, "dist") // fs required by router; unused in headless UI
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

// CLIToken returns the CLI auth token (headless mode only).
func (a *App) CLIToken() string {
	return a.cliToken
}

// Addr returns the HTTP server's actual listen address. Call after Start.
func (a *App) Addr() string {
	if a.listener == nil {
		return ""
	}
	return a.listener.Addr().String()
}

// Run starts platform workers and the HTTP server, then cleans up resources on exit.
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

// Start starts the HTTP server without blocking; use Addr() for the listen address after return.
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

// Close shuts down resources owned by the app.
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

// startHTTPServer runs the HTTP server until the context is cancelled or the server errors.
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

// cleanup closes memory, MCP, and other background resources to avoid lingering handles on exit.
func (a *App) cleanup(ctx context.Context) {
	if a == nil || a.core == nil {
		return
	}
	a.core.Close(ctx)
}
