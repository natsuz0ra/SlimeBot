package main

import (
	"log/slog"
	"os"

	"slimebot/internal/app"

	"github.com/joho/godotenv"

	_ "slimebot/internal/tools"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := godotenv.Load(); err != nil {
		slog.Warn("godotenv_load_failed", "err", err)
	}

	if err := app.RunFromEnv(); err != nil {
		slog.Error("server_startup_failed", "err", err)
		os.Exit(1)
	}
}
