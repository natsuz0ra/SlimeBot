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

// RunFromEnv loads config from the environment, validates, and runs the app main loop.
func RunFromEnv() error {
	return RunFromEnvWithMode(RunModeServer, nil)
}

// RunFromEnvWithMode loads config from the environment and starts the entry for the given mode.
func RunFromEnvWithMode(mode RunMode, runCLI func(context.Context, *Core) error) error {
	cfg, err := config.LoadValidated()
	if err != nil {
		return err
	}

	if err := ValidateConfigForMode(cfg, mode); err != nil {
		return err
	}

	if mode == RunModeCLI {
		core, err := NewCore(cfg)
		if err != nil {
			return err
		}
		core.WarmupInBackground(context.Background())
		rc := buildRunContext(true)
		core.ChatService.SetRunContext(rc)
		configureSkillSources(core, rc.IsCLI, rc.WorkingDir)
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

// RunCLIHeadless starts the headless HTTP server and returns the App (caller must close).
// CLI mode does not require JWT_SECRET; one is auto-generated. The returned App is already listening.
func RunCLIHeadless() (*App, error) {
	cfg, err := config.LoadValidated()
	if err != nil {
		return nil, err
	}

	// CLI mode does not require JWT_SECRET; generate one if missing.
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

// ValidateConfig checks required settings so the server does not start misconfigured.
func ValidateConfig(cfg config.Config) error {
	return ValidateConfigForMode(cfg, RunModeServer)
}

// ValidateConfigForMode validates config for the given run mode.
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

// runServerWithGracefulShutdown coordinates shutdown between listen errors and context cancel.
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
