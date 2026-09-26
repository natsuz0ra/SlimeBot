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
	agentssvc "slimebot/internal/services/agents"
	antsvc "slimebot/internal/services/anthropic"
	authsvc "slimebot/internal/services/auth"
	chatsvc "slimebot/internal/services/chat"
	configsvc "slimebot/internal/services/config"
	llmsvc "slimebot/internal/services/llm"
	memorysvc "slimebot/internal/services/memory"
	oaisvc "slimebot/internal/services/openai"
	plansvc "slimebot/internal/services/plan"
	schedulesvc "slimebot/internal/services/schedule"
	sessionsvc "slimebot/internal/services/session"
	settingssvc "slimebot/internal/services/settings"
	skillsvc "slimebot/internal/services/skill"
	teamsvc "slimebot/internal/services/team"
	"slimebot/internal/updater"
	buildversion "slimebot/internal/version"
)

// Core holds shared dependencies for server and CLI entrypoints.
type Core struct {
	Config config.Config
	Repo   *repositories.Repository

	AuthService      *authsvc.AuthService
	ChatService      *chatsvc.ChatService
	MemoryService    *memorysvc.Service
	ScheduleService  *schedulesvc.Service
	SessionService   *sessionsvc.SessionService
	SettingsService  *settingssvc.SettingsService
	AgentsService    *agentssvc.Service
	LLMConfigService *configsvc.LLMConfigService
	MCPConfigService *configsvc.MCPConfigService
	PlatformService  *configsvc.MessagePlatformConfigService
	SkillStore       *skillsvc.FileSystemSkillStore
	SkillPackage     *skillsvc.SkillPackageService
	SkillRuntime     *skillsvc.SkillRuntimeService
	ChatUpload       *chatsvc.ChatUploadService
	MCPManager       *mcp.Manager
	PlanService      *plansvc.PlanService
	UpdateService    *updater.Service
	TeamService      *teamsvc.Service

	warmupOnce    sync.Once
	warmupDone    chan struct{}
	warmupStarted atomic.Bool
}

// NewCore wires reusable services; it does not include HTTP routes, Telegram, or auth wiring.
func NewCore(cfg config.Config) (*Core, error) {
	return newCore(cfg, false)
}

func newCore(cfg config.Config, desktop bool) (*Core, error) {
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
	teamService := teamsvc.NewService(repo, teamsvc.Options{})
	if err := teamService.RecoverInterrupted(context.Background()); err != nil {
		return nil, err
	}
	authService := authsvc.NewAuthService(repo)
	if !desktop {
		if err := authService.EnsureDefaultAdmin(); err != nil {
			return nil, err
		}
	}

	openaiClient := oaisvc.NewOpenAIClient()
	anthropicClient := antsvc.NewAnthropicClient()
	providerFactory := llmsvc.NewFactory(openaiClient)
	providerFactory.Register(llmsvc.ProviderAnthropic, anthropicClient)
	mcpManager := mcp.NewManager()
	settingsService := settingssvc.NewSettingsService(repo)
	agentsService := agentssvc.NewService(sbruntime.GlobalAgentsPath())
	sessionService := sessionsvc.NewSessionService(repo)
	llmConfigService := configsvc.NewLLMConfigService(repo, cfg.DefaultContextSize)
	mcpConfigService := configsvc.NewMCPConfigServiceWithTools(repo, mcpManager)
	platformService := configsvc.NewMessagePlatformConfigService(repo)

	skillStore := skillsvc.NewFileSystemSkillStore(cfg.SkillsRoot)
	skillPackage := skillsvc.NewSkillPackageService(skillStore, cfg.SkillsRoot)
	skillRuntime := skillsvc.NewSkillRuntimeServiceWithOptions(skillStore, cfg.SkillsRoot, skillsvc.SkillRuntimeOptions{
		StateDir: cfg.SkillsRoot,
		Sources:  skillsvc.DefaultGlobalSkillSources(cfg.HermesSkillsRoots),
	})

	chatUpload := chatsvc.NewChatUploadService(cfg.ChatUploadRoot)
	memoryService := memorysvc.NewServiceFromSettings(repo)
	scheduleService := schedulesvc.NewService(repo, nil, schedulesvc.Options{})
	chatService := chatsvc.NewChatService(repo, repo, providerFactory, mcpManager, skillRuntime)
	chatService.SetTeamService(teamService)
	chatService.SetMemoryService(memoryService)
	chatService.SetScheduleService(scheduleService)
	chatService.SetAgentsInstructions(agentsService)
	chatService.SetUploadService(chatUpload)
	chatService.SetContextHistoryRounds(cfg.ContextHistoryRounds)

	planService, err := plansvc.NewPlanService()
	if err != nil {
		return nil, err
	}
	chatService.SetPlanService(planService)
	info := buildversion.Info()
	updateService := updater.NewService(updater.ServiceOptions{CurrentVersion: info.Version})

	return &Core{
		Config:           cfg,
		Repo:             repo,
		AuthService:      authService,
		ChatService:      chatService,
		MemoryService:    memoryService,
		ScheduleService:  scheduleService,
		SessionService:   sessionService,
		SettingsService:  settingsService,
		AgentsService:    agentsService,
		LLMConfigService: llmConfigService,
		MCPConfigService: mcpConfigService,
		PlatformService:  platformService,
		SkillStore:       skillStore,
		SkillPackage:     skillPackage,
		SkillRuntime:     skillRuntime,
		ChatUpload:       chatUpload,
		MCPManager:       mcpManager,
		PlanService:      planService,
		UpdateService:    updateService,
		TeamService:      teamService,
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

func configureSkillSources(core *Core, isCLI bool, workingDir string) {
	if core == nil || core.SkillRuntime == nil {
		return
	}
	sources := skillsvc.DefaultGlobalSkillSources(core.Config.HermesSkillsRoots)
	if isCLI {
		sources = append(sources, skillsvc.ProjectSkillSources(workingDir)...)
	}
	core.SkillRuntime.SetSources(sources)
}
