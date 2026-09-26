package app

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"slimebot/internal/config"
	"slimebot/internal/runtime"
)

// RunDesktopHost serves the desktop-owned UI until the parent closes stdin or the OS stops us.
func RunDesktopHost(input io.Reader, output io.Writer) error {
	reader := bufio.NewReader(input)
	line, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read desktop startup: %w", err)
	}
	var startup struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(line), &startup); err != nil {
		return fmt.Errorf("parse desktop startup: %w", err)
	}
	startup.Token = strings.TrimSpace(startup.Token)
	if decoded, err := hex.DecodeString(startup.Token); err != nil || len(decoded) != 32 {
		return fmt.Errorf("desktop startup token must be 32 random bytes")
	}

	if err := runtime.EnsureAndLoadEnv(); err != nil {
		return err
	}
	cfg := desktopConfig(startup.Token)
	instance, err := NewDesktop(cfg, startup.Token)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		instance.Close(shutdownCtx)
	}()
	if err := instance.Start(ctx); err != nil {
		return err
	}
	if err := json.NewEncoder(output).Encode(map[string]string{"address": instance.Addr(), "version": "1"}); err != nil {
		return fmt.Errorf("send desktop ready: %w", err)
	}
	go func() {
		_, _ = io.Copy(io.Discard, reader)
		stop()
	}()
	<-ctx.Done()
	return nil
}

func desktopConfig(token string) config.Config {
	cfg := config.Load()
	cfg.Frontend = ""
	cfg.JWTSecret = token
	return cfg
}
