package main

import (
	"os"

	"slimebot/internal/app"
	"slimebot/internal/logging"
	"slimebot/internal/runtime"

	_ "slimebot/internal/tools"
)

func main() {
	_, cleanupLogs, _ := logging.Init(logging.Options{Mode: logging.ModeServer})
	defer cleanupLogs()

	if err := runtime.EnsureAndLoadEnv(); err != nil {
		logging.Error("env_bootstrap_failed", "err", err)
		os.Exit(1)
	}

	if err := app.RunFromEnv(); err != nil {
		logging.Error("server_startup_failed", "err", err)
		os.Exit(1)
	}
}
