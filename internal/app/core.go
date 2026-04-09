package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"slimebot/internal/config"
	"slimebot/internal/logging"
	"slimebot/internal/mcp"
	"slimebot/internal/repositories"
	sbruntime "slimebot/internal/runtime"
	antsvc "slimebot/internal/services/anthropic"
	authsvc "slimebot/internal/services/auth"
	chatsvc "slimebot/internal/services/chat"
	configsvc "slimebot/internal/services/config"
	llmsvc "slimebot/internal/services/llm"
	memsvc "slimebot/internal/services/memory"
	oaisvc "slimebot/internal/services/openai"
	sessionsvc "slimebot/internal/services/session"
	settingssvc "slimebot/internal/services/settings"
	skillsvc "slimebot/internal/services/skill"
)

// Core 聚合 server/cli 共用的基础依赖与服务。
type Core struct {
	Config config.Config
	Repo   *repositories.Repository

	AuthService      *authsvc.AuthService
	ChatService      *chatsvc.ChatService
	SessionService   *sessionsvc.SessionService
	SettingsService  *settingssvc.SettingsService
	LLMConfigService *configsvc.LLMConfigService
	MCPConfigService *configsvc.MCPConfigService
	PlatformService  *configsvc.MessagePlatformConfigService
	SkillStore       *skillsvc.FileSystemSkillStore
	SkillPackage     *skillsvc.SkillPackageService
	SkillRuntime     *skillsvc.SkillRuntimeService
	ChatUpload       *chatsvc.ChatUploadService
	MCPManager       *mcp.Manager
	MemoryService    *memsvc.MemoryService

	warmupOnce    sync.Once
	warmupDone    chan struct{}
	warmupStarted atomic.Bool
}

// NewCore 初始化可复用核心依赖，不包含 server/telegram/鉴权路由等入口级组件。
func NewCore(cfg config.Config) (*Core, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), os.ModePerm); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.SkillsRoot, os.ModePerm); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.ChatUploadRoot, os.ModePerm); err != nil {
		return nil, err
	}
	db, err := repositories.NewSQLite(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	repo := repositories.New(db)
	authService := authsvc.NewAuthService(repo)
	if err := authService.EnsureDefaultAdmin(); err != nil {
		return nil, err
	}

	openaiClient := oaisvc.NewOpenAIClient()
	anthropicClient := antsvc.NewAnthropicClient()
	providerFactory := llmsvc.NewFactory(openaiClient)
	providerFactory.Register(llmsvc.ProviderAnthropic, anthropicClient)
	mcpManager := mcp.NewManager()
	settingsService := settingssvc.NewSettingsService(repo)
	sessionService := sessionsvc.NewSessionService(repo)
	llmConfigService := configsvc.NewLLMConfigService(repo)
	mcpConfigService := configsvc.NewMCPConfigService(repo)
	platformService := configsvc.NewMessagePlatformConfigService(repo)

	skillStore := skillsvc.NewFileSystemSkillStore(cfg.SkillsRoot)
	skillPackage := skillsvc.NewSkillPackageService(skillStore, cfg.SkillsRoot)
	skillRuntime := skillsvc.NewSkillRuntimeService(skillStore, cfg.SkillsRoot)

	memoryService, err := memsvc.NewMemoryService(cfg.MemoryDir)
	if err != nil {
		return nil, err
	}

	chatUpload := chatsvc.NewChatUploadService(cfg.ChatUploadRoot)
	chatService := chatsvc.NewChatService(repo, providerFactory, mcpManager, skillRuntime, memoryService)
	chatService.SetUploadService(chatUpload)

	return &Core{
		Config:           cfg,
		Repo:             repo,
		AuthService:      authService,
		ChatService:      chatService,
		SessionService:   sessionService,
		SettingsService:  settingsService,
		LLMConfigService: llmConfigService,
		MCPConfigService: mcpConfigService,
		PlatformService:  platformService,
		SkillStore:       skillStore,
		SkillPackage:     skillPackage,
		SkillRuntime:     skillRuntime,
		ChatUpload:       chatUpload,
		MCPManager:       mcpManager,
		MemoryService:    memoryService,
		warmupDone:       make(chan struct{}),
	}, nil
}

// WarmupInBackground 启动后台 goroutine 预热记忆索引。
func (c *Core) WarmupInBackground(ctx context.Context) {
	if c == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.warmupStarted.Store(true)
	c.warmupOnce.Do(func() {
		go func() {
			defer close(c.warmupDone)

			// 重建记忆索引（如果需要）
			if c.MemoryService != nil && c.MemoryService.Store() != nil {
				if err := c.MemoryService.Store().RebuildIndex(); err != nil {
					logging.Warn("memory_index_warmup_failed", "err", err)
				} else {
					logging.Info("memory_index_warmup_complete", "dir", c.Config.MemoryDir)
				}
			}
		}()
	})
}

// Close 关闭核心依赖持有的后台资源。
func (c *Core) Close(ctx context.Context) {
	if c == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if c.warmupStarted.Load() {
		waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		select {
		case <-c.warmupDone:
		case <-waitCtx.Done():
			logging.Warn("async_warmup_wait_timeout", "err", waitCtx.Err())
		}
	}

	if c.MemoryService != nil {
		if err := c.MemoryService.Shutdown(ctx); err != nil {
			logging.Warn("memory_shutdown", "err", err)
		}
	}
	if c.MCPManager != nil {
		c.MCPManager.CloseAll()
	}
}

// buildRunContext 构建 ChatService 所需的运行上下文。
func buildRunContext(isCLI bool) chatsvc.RunContext {
	cwd := ""
	if isCLI {
		cwd, _ = os.Getwd()
	}
	return chatsvc.RunContext{
		ConfigHomeDir:        sbruntime.SlimeBotHomeDir(),
		ConfigDirDescription: sbruntime.DescribeConfigHome(),
		WorkingDir:           cwd,
		IsCLI:                isCLI,
	}
}
