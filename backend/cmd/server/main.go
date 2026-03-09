package main

import (
	"log"
	"os"
	"path/filepath"

	"corner/backend/internal/config"
	"corner/backend/internal/controllers"
	"corner/backend/internal/database"
	"corner/backend/internal/repositories"
	"corner/backend/internal/router"
	"corner/backend/internal/services"
)

func main() {
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
	chatService := services.NewChatService(repo, openaiClient)

	httpController := controllers.NewHTTPController(repo)
	wsController := controllers.NewWSController(chatService)
	engine := router.New(cfg, httpController, wsController)

	addr := ":" + cfg.ServerPort
	log.Printf("server listening on %s", addr)
	if err := engine.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
