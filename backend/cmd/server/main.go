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

	"github.com/joho/godotenv"

	// 导入 tools 包触发各工具的 init() 自注册
	_ "slimebot/backend/internal/tools"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf(".env not loaded (falling back to system environment variables): %v", err)
	}

	cfg := config.Load()
	if err := validateConfig(cfg); err != nil {
		log.Fatalf("config validation failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), os.ModePerm); err != nil {
		log.Fatalf("failed to create database directory: %v", err)
	}
	if err := os.MkdirAll(cfg.SkillsRoot, os.ModePerm); err != nil {
		log.Fatalf("failed to create skills directory: %v", err)
	}
	if err := os.MkdirAll(cfg.ChatUploadRoot, os.ModePerm); err != nil {
		log.Fatalf("failed to create chat upload directory: %v", err)
	}

	db, err := database.NewSQLite(cfg.DBPath)
	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}

	repo := repositories.New(db)
	if err := ensureDefaultAdmin(repo); err != nil {
		log.Fatalf("failed to initialize default admin account: %v", err)
	}
	tokenManager, err := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTExpireMinutes)
	if err != nil {
		log.Fatalf("authentication initialization failed: %v", err)
	}
	openaiClient := services.NewOpenAIClient()
	mcpManager := mcp.NewManager()
	skillPackageService := services.NewSkillPackageService(repo, cfg.SkillsRoot)
	skillRuntimeService := services.NewSkillRuntimeService(repo, cfg.SkillsRoot)
	memoryService := services.NewMemoryService(repo, openaiClient)
	configureMemoryVectorization(cfg, memoryService)
	// chatUploadService 负责“上传临时存储 -> 回合消费 -> 结束清理”的附件生命周期管理。
	chatUploadService := services.NewChatUploadService(cfg.ChatUploadRoot)
	chatService := services.NewChatService(repo, openaiClient, mcpManager, skillRuntimeService, memoryService)
	// 将附件服务注入 chatService，使 WS chat 链路可消费 attachmentIds。
	chatService.SetUploadService(chatUploadService)
	approvalBroker := telegram.NewApprovalBroker()
	platformDispatcher := platforms.NewDispatcher(chatService, approvalBroker)
	telegramWorker := telegram.NewWorker(repo, platformDispatcher, chatUploadService)
	appCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	telegramWorker.Start(appCtx)

	// HTTP 控制器注入上传服务，提供 /sessions/:id/attachments 临时上传入口。
	httpController := controller.NewHTTPController(repo, skillPackageService, skillRuntimeService, chatUploadService, tokenManager)
	wsController := ws.NewController(chatService)
	engine := router.New(cfg, tokenManager, httpController, wsController)

	addr := ":" + cfg.ServerPort
	log.Printf("server listening on %s", addr)
	server := &http.Server{
		Addr:    addr,
		Handler: engine,
	}
	if err := runServerWithGracefulShutdown(appCtx, server); err != nil {
		log.Fatalf("server startup failed: %v", err)
	}
}

func validateConfig(cfg config.Config) error {
	if strings.TrimSpace(cfg.JWTSecret) == "" {
		return errors.New("JWT_SECRET is not configured")
	}
	if cfg.JWTExpireMinutes <= 0 {
		return errors.New("JWT_EXPIRE must be greater than 0 (minutes)")
	}
	return nil
}

func configureMemoryVectorization(cfg config.Config, memoryService *services.MemoryService) {
	if !strings.EqualFold(strings.TrimSpace(cfg.EmbeddingProvider), "onnx") {
		log.Printf("memory_vectorization_disabled reason=embedding_provider provider=%q", cfg.EmbeddingProvider)
		return
	}
	if strings.TrimSpace(cfg.EmbeddingModelPath) == "" || strings.TrimSpace(cfg.EmbeddingTokenizerPath) == "" {
		log.Printf("memory_vectorization_disabled reason=missing_embedding_paths")
		return
	}
	if strings.TrimSpace(cfg.QdrantURL) == "" || strings.TrimSpace(cfg.QdrantCollection) == "" {
		log.Printf("memory_vectorization_disabled reason=missing_qdrant_config")
		return
	}
	embedding := services.NewONNXRuntimeEmbeddingService(services.ONNXRuntimeEmbeddingConfig{
		ModelPath:     cfg.EmbeddingModelPath,
		TokenizerPath: cfg.EmbeddingTokenizerPath,
		PythonBin:     cfg.EmbeddingPythonBin,
		ScriptPath:    cfg.EmbeddingScriptPath,
		Timeout:       time.Duration(cfg.EmbeddingTimeoutMS) * time.Millisecond,
	})
	memoryService.SetEmbeddingService(embedding)

	vectorStore, err := repositories.NewMemoryVectorRepository(cfg.QdrantURL, cfg.QdrantCollection)
	if err != nil {
		log.Printf("memory_vectorization_disabled reason=qdrant_init_failed err=%v", err)
		return
	}
	memoryService.SetVectorStore(vectorStore)
	memoryService.SetVectorSearchTopK(cfg.MemoryVectorTopK)
	log.Printf(
		"memory_vectorization_enabled provider=onnx qdrant_url=%s collection=%s topk=%d",
		cfg.QdrantURL,
		cfg.QdrantCollection,
		cfg.MemoryVectorTopK,
	)
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
