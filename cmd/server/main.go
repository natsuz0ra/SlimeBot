package main

import (
	"log"
	"slimebot/internal/app"

	"github.com/joho/godotenv"

	// 导入 tools 包触发各工具的 init() 自注册
	_ "slimebot/internal/tools"
)

func main() {
	// 保险起见：让 App.RunFromEnv 之前已经加载 .env（如果外部启动脚本提供了不同工作目录）。
	if err := godotenv.Load(); err != nil {
		log.Printf(".env not loaded (falling back to system environment variables): %v", err)
	}

	if err := app.RunFromEnv(); err != nil {
		log.Fatalf("server startup failed: %v", err)
	}
}
