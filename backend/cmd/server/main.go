package main

import (
	"log"
	"os"
	"path/filepath"

	"corner/backend/internal/config"
	"corner/backend/internal/controllers"
	"corner/backend/internal/database"
	"corner/backend/internal/mcp"
	"corner/backend/internal/repositories"
	"corner/backend/internal/router"
	"corner/backend/internal/services"
	"github.com/joho/godotenv"

	// 导入 tools 包触发各工具的 init() 自注册
	_ "corner/backend/internal/tools"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf(".env 未加载（将继续使用系统环境变量）: %v", err)
	}

	cfg := config.Load()
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), os.ModePerm); err != nil {
		log.Fatalf("创建数据库目录失败: %v", err)
	}

	db, err := database.NewSQLite(cfg.DBPath)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}

	repo := repositories.New(db)
	openaiClient := services.NewOpenAIClient()
	mcpManager := mcp.NewManager()
	chatService := services.NewChatService(repo, openaiClient, mcpManager)

	httpController := controllers.NewHTTPController(repo)
	wsController := controllers.NewWSController(chatService)
	engine := router.New(cfg, httpController, wsController)

	addr := ":" + cfg.ServerPort
	log.Printf("server listening on %s", addr)
	if err := engine.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
