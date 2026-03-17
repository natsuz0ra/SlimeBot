package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"slimebot/backend/internal/auth"
	"slimebot/backend/internal/config"
	"slimebot/backend/internal/consts"
	"slimebot/backend/internal/database"
	"slimebot/backend/internal/http/controller"
	"slimebot/backend/internal/http/router"
	"slimebot/backend/internal/http/ws"
	"slimebot/backend/internal/mcp"
	"slimebot/backend/internal/platforms"
	"slimebot/backend/internal/platforms/telegram"
	"slimebot/backend/internal/repositories"
	"slimebot/backend/internal/services"

	// 导入 tools 包触发各工具的 init() 自注册
	_ "slimebot/backend/internal/tools"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf(".env 未加载（将继续使用系统环境变量）: %v", err)
	}

	cfg := config.Load()
	if err := validateConfig(cfg); err != nil {
		log.Fatalf("配置校验失败: %v", err)
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
	memoryService := services.NewMemoryService(repo, openaiClient)
	chatService := services.NewChatService(repo, openaiClient, mcpManager, skillRuntimeService, memoryService)
	approvalBroker := telegram.NewApprovalBroker()
	platformDispatcher := platforms.NewDispatcher(chatService, approvalBroker)
	telegramWorker := telegram.NewWorker(repo, platformDispatcher)
	appCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	telegramWorker.Start(appCtx)

	httpController := controller.NewHTTPController(repo, skillPackageService, skillRuntimeService, tokenManager)
	wsController := ws.NewController(chatService)
	engine := router.New(cfg, tokenManager, httpController, wsController)

	addr := ":" + cfg.ServerPort
	log.Printf("server listening on %s", addr)
	server := &http.Server{
		Addr:    addr,
		Handler: engine,
	}
	if err := runServerWithGracefulShutdown(appCtx, server); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

func validateConfig(cfg config.Config) error {
	if strings.TrimSpace(cfg.JWTSecret) == "" {
		return errors.New("JWT_SECRET 未配置")
	}
	if cfg.JWTExpireMinutes <= 0 {
		return errors.New("JWT_EXPIRE 必须大于 0（单位：分钟）")
	}
	return nil
}

func runServerWithGracefulShutdown(ctx context.Context, server *http.Server) error {
	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	case err := <-errCh:
		return err
	}
}

func ensureDefaultAdmin(repo *repositories.Repository) error {
	username, err := repo.GetSetting(consts.SettingAuthUsername)
	if err != nil {
		return err
	}
	passwordHash, err := repo.GetSetting(consts.SettingAuthPasswordHash)
	if err != nil {
		return err
	}

	if strings.TrimSpace(username) == "" || strings.TrimSpace(passwordHash) == "" {
		defaultHash, hashErr := auth.HashPassword("admin")
		if hashErr != nil {
			return hashErr
		}
		if err := repo.SetSetting(consts.SettingAuthUsername, "admin"); err != nil {
			return err
		}
		if err := repo.SetSetting(consts.SettingAuthPasswordHash, defaultHash); err != nil {
			return err
		}
		if err := repo.SetSetting(consts.SettingAuthForcePasswordChange, "true"); err != nil {
			return err
		}
		return nil
	}

	forceFlag, err := repo.GetSetting(consts.SettingAuthForcePasswordChange)
	if err != nil {
		return err
	}
	if strings.TrimSpace(forceFlag) == "" {
		if err := repo.SetSetting(consts.SettingAuthForcePasswordChange, "false"); err != nil {
			return err
		}
	}
	return nil
}
