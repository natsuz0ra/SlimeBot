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
	oaisvc "slimebot/internal/services/openai"
	plansvc "slimebot/internal/services/plan"
	sessionsvc "slimebot/internal/services/session"
	settingssvc "slimebot/internal/services/settings"
	skillsvc "slimebot/internal/services/skill"
)

// Core holds shared dependencies for server and CLI entrypoints.
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
	PlanService      *plansvc.PlanService

	warmupOnce    sync.Once
	warmupDone    chan struct{}
	warmupStarted atomic.Bool
}

// NewCore wires reusable services; it does not include HTTP routes, Telegram, or auth wiring.
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
	llmConfigService := configsvc.NewLLMConfigService(repo, cfg.DefaultContextSize)
	mcpConfigService := configsvc.NewMCPConfigService(repo)
	platformService := configsvc.NewMessagePlatformConfigService(repo)

	skillStore := skillsvc.NewFileSystemSkillStore(cfg.SkillsRoot)
	skillPackage := skillsvc.NewSkillPackageService(skillStore, cfg.SkillsRoot)
	skillRuntime := skillsvc.NewSkillRuntimeService(skillStore, cfg.SkillsRoot)

	chatUpload := chatsvc.NewChatUploadService(cfg.ChatUploadRoot)
	chatService := chatsvc.NewChatService(repo, repo, providerFactory, mcpManager, skillRuntime)
	chatService.SetUploadService(chatUpload)
	chatService.SetContextHistoryRounds(cfg.ContextHistoryRounds)

	planService, err := plansvc.NewPlanService()
	if err != nil {
		return nil, err
	}
	chatService.SetPlanService(planService)

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
		PlanService:      planService,
		warmupDone:       make(chan struct{}),
	}, nil
}

// WarmupInBackground starts lightweight background warmup work.
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

			logging.Info("warmup_complete")
		}()
	})
}

// Close releases background resources held by Core.
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

	if c.MCPManager != nil {
		c.MCPManager.CloseAll()
	}
	if c.Repo != nil {
		if err := c.Repo.Close(); err != nil {
			logging.Warn("db_close", "err", err)
		}
	}
}

// buildRunContext builds ChatService RunContext for CLI vs server.
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
