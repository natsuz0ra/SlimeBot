// 程序入口：初始化日志、加载环境变量并启动应用
package main

import (
	"log/slog"
	"os"

	"slimebot/internal/app"

	"github.com/joho/godotenv"

	_ "slimebot/internal/tools"
)

func main() {
	// 设置结构化日志输出到标准错误流
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// 尝试从.env文件加载环境变量，失败仅警告不阻断
	if err := godotenv.Load(); err != nil {
		slog.Warn("godotenv_load_failed", "err", err)
	}

	// 从环境变量配置启动应用，失败则记录错误并退出
	if err := app.RunFromEnv(); err != nil {
		slog.Error("server_startup_failed", "err", err)
		os.Exit(1)
	}
}
