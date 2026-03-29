package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"slimebot/internal/config"
)

// RunFromEnv 从环境变量加载配置、完成校验并启动应用主循环。
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

// ValidateConfig 校验启动所需的关键配置，避免服务带着明显错误启动。
func ValidateConfig(cfg config.Config) error {
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
