package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slimebot/internal/app"
	"slimebot/internal/logging"
	sbruntime "slimebot/internal/runtime"

	_ "slimebot/internal/tools"
)

func main() {
	_, cleanupLogs, _ := logging.Init(logging.Options{Mode: logging.ModeCLI})
	defer cleanupLogs()

	if err := sbruntime.EnsureAndLoadEnv(); err != nil {
		logging.Error("env_bootstrap_failed", "err", err)
		os.Exit(1)
	}

	// 启动 headless HTTP 服务器
	slimeApp, err := app.RunCLIHeadless()
	if err != nil {
		logging.Error("cli_headless_start_failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		_ = shutdownCtx // just call Close directly
		slimeApp.Close(shutdownCtx)
	}()

	apiURL := fmt.Sprintf("http://%s", slimeApp.Addr())
	cliToken := slimeApp.CLIToken()

	logging.Info("cli_headless_ready", "api_url", apiURL)

	// 查找 CLI JS 入口
	cliEntry := findCLIEntry()
	if cliEntry == "" {
		logging.Error("cli_entry_not_found", "message", "cli/cli.cjs not found. Run 'npm run build:cli' first.")
		os.Exit(1)
	}

	// 启动 Node.js CLI 子进程
	nodeCmd := findNode()
	args := []string{cliEntry,
		"--api-url", apiURL,
		"--cli-token", cliToken,
	}

	cmd := exec.Command(nodeCmd, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"SLIMEBOT_API_URL="+apiURL,
		"SLIMEBOT_CLI_TOKEN="+cliToken,
	)

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		logging.Error("cli_process_error", "err", err)
		os.Exit(1)
	}
}

// findCLIEntry 查找 CLI JS 打包产物。
func findCLIEntry() string {
	// 相对于可执行文件所在目录查找
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	base := filepath.Dir(exe)

	candidates := []string{
		filepath.Join(base, "cli", "cli.cjs"),
		filepath.Join(base, "..", "cli", "cli.cjs"),
	}

	// 开发模式：相对于工作目录
	wd, _ := os.Getwd()
	candidates = append(candidates,
		filepath.Join(wd, "cli", "cli.cjs"),
	)

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// findNode 查找 Node.js 可执行文件。
func findNode() string {
	// 优先使用 NODE_PATH 环境变量
	if nodePath := os.Getenv("NODE_PATH"); nodePath != "" {
		return nodePath
	}

	// 查找系统 Node.js
	name := "node"
	if runtime.GOOS == "windows" {
		name = "node.exe"
	}

	if p, err := exec.LookPath(name); err == nil {
		return p
	}

	// 常见安装路径
	if runtime.GOOS == "windows" {
		paths := []string{
			`C:\Program Files\nodejs\node.exe`,
			`C:\Program Files (x86)\nodejs\node.exe`,
		}
		// nvm for Windows
		nvmHome := os.Getenv("NVM_HOME")
		if nvmHome != "" {
			paths = append([]string{filepath.Join(nvmHome, "node.exe")}, paths...)
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	return "node"
}
