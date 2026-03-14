package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"slimebot/backend/internal/config"
	"slimebot/backend/internal/controllers"
	"slimebot/backend/internal/database"
	"slimebot/backend/internal/mcp"
	"slimebot/backend/internal/repositories"
	"slimebot/backend/internal/router"
	"slimebot/backend/internal/services"

	// 导入 tools 包触发各工具的 init() 自注册
	_ "slimebot/backend/internal/tools"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf(".env 未加载（将继续使用系统环境变量）: %v", err)
	}

	cfg := config.Load()
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), os.ModePerm); err != nil {
		log.Fatalf("创建数据库目录失败: %v", err)
	}
	if err := os.MkdirAll(cfg.SkillsRoot, os.ModePerm); err != nil {
		log.Fatalf("创建 skills 目录失败: %v", err)
	}

	db, err := database.NewSQLite(cfg.DBPath)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}

	repo := repositories.New(db)
	openaiClient := services.NewOpenAIClient()
	mcpManager := mcp.NewManager()
	skillPackageService := services.NewSkillPackageService(repo, cfg.SkillsRoot)
	skillRuntimeService := services.NewSkillRuntimeService(repo, cfg.SkillsRoot)
	chatService := services.NewChatService(repo, openaiClient, mcpManager, skillRuntimeService)

	httpController := controllers.NewHTTPController(repo, skillPackageService, skillRuntimeService)
	wsController := controllers.NewWSController(chatService)
	engine := router.New(cfg, httpController, wsController)

	addr := ":" + cfg.ServerPort
	log.Printf("server listening on %s", addr)
	if err := engine.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
