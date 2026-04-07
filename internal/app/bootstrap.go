package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"slimebot/internal/config"
)

type RunMode string

const (
	RunModeServer RunMode = "server"
	RunModeCLI    RunMode = "cli"
)

// RunFromEnv 从环境变量加载配置、完成校验并启动应用主循环。
func RunFromEnv() error {
	return RunFromEnvWithMode(RunModeServer, nil)
}

// RunFromEnvWithMode 从环境变量加载配置，并按运行模式启动对应入口。
func RunFromEnvWithMode(mode RunMode, runCLI func(context.Context, *Core) error) error {
	cfg := config.Load()

	if err := ValidateConfigForMode(cfg, mode); err != nil {
		return err
	}

	if mode == RunModeCLI {
		core, err := NewCore(cfg)
		if err != nil {
			return err
		}
		core.WarmupInBackground(context.Background())
		appCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stopSignals()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			core.Close(shutdownCtx)
		}()
		if runCLI == nil {
			return fmt.Errorf("cli runner is not configured")
		}
		return runCLI(appCtx, core)
	}

	app, err := New(cfg)
	if err != nil {
		return err
	}

	appCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	return app.Run(appCtx)
}

// RunCLIHeadless 启动 headless HTTP 服务器，返回应用实例（调用方负责关闭）。
// CLI 模式不需要 JWT_SECRET，会自动生成；返回的应用已开始监听。
func RunCLIHeadless() (*App, error) {
	cfg := config.Load()

	// CLI 模式不需要 JWT_SECRET，自动生成一个
	if strings.TrimSpace(cfg.JWTSecret) == "" {
		cfg.JWTSecret = fmt.Sprintf("cli-auto-%d", time.Now().UnixNano())
	}

	app, err := NewHeadless(cfg)
	if err != nil {
		return nil, err
	}

	// Keep headless server context alive for the whole CLI process lifetime.
	// Cancellation is delegated to App.Close() by storing this cancel func in App.
	appCtx, cancel := context.WithCancel(context.Background())
	app.setStartCancel(cancel)

	if err := app.Start(appCtx); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start headless server: %w", err)
	}

	return app, nil
}

// ValidateConfig 校验启动所需的关键配置，避免服务带着明显错误启动。
func ValidateConfig(cfg config.Config) error {
	return ValidateConfigForMode(cfg, RunModeServer)
}

// ValidateConfigForMode 根据运行模式校验关键配置。
func ValidateConfigForMode(cfg config.Config, mode RunMode) error {
	if mode == RunModeCLI {
		return nil
	}
	if strings.TrimSpace(cfg.JWTSecret) == "" {
		return errors.New("JWT_SECRET is not configured")
	}
	if cfg.JWTExpireMinutes <= 0 {
		return errors.New("JWT_EXPIRE must be greater than 0 (minutes)")
	}
	return nil
}

// runServerWithGracefulShutdown 在监听错误与外部退出信号之间协调服务关闭流程。
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
