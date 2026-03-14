package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"slimebot/backend/internal/auth"
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
	if strings.TrimSpace(cfg.JWTSecret) == "" {
		log.Fatal("JWT_SECRET 未配置，服务启动失败")
	}
	if cfg.JWTExpireMinutes <= 0 {
		log.Fatal("JWT_EXPIRE 必须大于 0（单位：分钟）")
	}
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
	if err := ensureDefaultAdmin(repo); err != nil {
		log.Fatalf("初始化默认账号失败: %v", err)
	}
	tokenManager, err := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTExpireMinutes)
	if err != nil {
		log.Fatalf("鉴权初始化失败: %v", err)
	}
	openaiClient := services.NewOpenAIClient()
	mcpManager := mcp.NewManager()
	skillPackageService := services.NewSkillPackageService(repo, cfg.SkillsRoot)
	skillRuntimeService := services.NewSkillRuntimeService(repo, cfg.SkillsRoot)
	chatService := services.NewChatService(repo, openaiClient, mcpManager, skillRuntimeService)

	httpController := controllers.NewHTTPController(repo, skillPackageService, skillRuntimeService, tokenManager)
	wsController := controllers.NewWSController(chatService)
	engine := router.New(cfg, tokenManager, httpController, wsController)

	addr := ":" + cfg.ServerPort
	log.Printf("server listening on %s", addr)
	if err := engine.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

func ensureDefaultAdmin(repo *repositories.Repository) error {
	username, err := repo.GetSetting(auth.SettingAuthUsername)
	if err != nil {
		return err
	}
	passwordHash, err := repo.GetSetting(auth.SettingAuthPasswordHash)
	if err != nil {
		return err
	}

	if strings.TrimSpace(username) == "" || strings.TrimSpace(passwordHash) == "" {
		defaultHash, hashErr := auth.HashPassword("admin")
		if hashErr != nil {
			return hashErr
		}
		if err := repo.SetSetting(auth.SettingAuthUsername, "admin"); err != nil {
			return err
		}
		if err := repo.SetSetting(auth.SettingAuthPasswordHash, defaultHash); err != nil {
			return err
		}
		if err := repo.SetSetting(auth.SettingAuthForcePasswordChange, "true"); err != nil {
			return err
		}
		return nil
	}

	forceFlag, err := repo.GetSetting(auth.SettingAuthForcePasswordChange)
	if err != nil {
		return err
	}
	if strings.TrimSpace(forceFlag) == "" {
		if err := repo.SetSetting(auth.SettingAuthForcePasswordChange, "false"); err != nil {
			return err
		}
	}
	return nil
}
