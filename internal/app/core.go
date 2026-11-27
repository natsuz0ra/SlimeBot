package app

import (
	"context"
	"fmt"
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
	agentruntime "slimebot/internal/runtime/agent"
	"slimebot/internal/runtime/workspace"
	agentssvc "slimebot/internal/services/agents"
	antsvc "slimebot/internal/services/anthropic"
	authsvc "slimebot/internal/services/auth"
	chatsvc "slimebot/internal/services/chat"
	configsvc "slimebot/internal/services/config"
	llmsvc "slimebot/internal/services/llm"
	memorysvc "slimebot/internal/services/memory"
	oaisvc "slimebot/internal/services/openai"
	plansvc "slimebot/internal/services/plan"
	privacysvc "slimebot/internal/services/privacy"
	schedulesvc "slimebot/internal/services/schedule"
	sessionsvc "slimebot/internal/services/session"
	settingssvc "slimebot/internal/services/settings"
	skillsvc "slimebot/internal/services/skill"
	teamsvc "slimebot/internal/services/team"
	usagesvc "slimebot/internal/services/usage"
	"slimebot/internal/updater"
	buildversion "slimebot/internal/version"
)

// Core holds shared dependencies for server and CLI entrypoints.
type Core struct {
	runtimeOwner *agentruntime.Owner
	Config       config.Config
	Repo         *repositories.Repository

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
	SkillLearning    *skillsvc.LearningService
	ChatUpload       *chatsvc.ChatUploadService
	MCPManager       *mcp.Manager
	PlanService      *plansvc.PlanService
	UpdateService    *updater.Service
	TeamService      *teamsvc.Service
	PrivacyService   *privacysvc.Service
	UsageService     *usagesvc.Service

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
	owner, err := agentruntime.AcquireOwner(cfg.DBPath + ".runtime.lock")
	if err != nil {
		return nil, err
	}
	owned := false
	defer func() {
		if !owned {
			_ = owner.Close()
		}
	}()
	db, err := repositories.NewSQLite(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	repo := repositories.New(db)
	defer func() {
		if !owned {
			_ = repo.Close()
		}
	}()
	if err := repo.RecoverAgentRuntime(context.Background()); err != nil {
		return nil, err
	}
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
	privacyService := privacysvc.NewService(repo, filepath.Join(filepath.Dir(cfg.DBPath), "privacy"))
	// Keep the deployment key out of environments inherited by tools and MCP servers.
	if err := os.Unsetenv("SLIMEBOT_PRIVACY_MAPPING_KEY"); err != nil {
		return nil, fmt.Errorf("clear privacy mapping key from tool environment: %w", err)
	}
	providerFactory := llmsvc.NewFactory(privacyService.Wrap(openaiClient))
	providerFactory.Register(llmsvc.ProviderAnthropic, privacyService.Wrap(anthropicClient))
	providerFactory.SetTextProtector(privacyService.ProtectText)
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
	skillLearning := skillsvc.NewLearningService(providerFactory, skillRuntime)
	chatService.SetSkillLearningService(skillLearning)
	chatService.SetFileCheckpoints(workspace.NewCheckpoints(filepath.Join(filepath.Dir(cfg.DBPath), "file-checkpoints")))
	usageService := usagesvc.NewService(repo, repo)
	chatService.SetUsageService(usageService)
	chatService.SetTeamService(teamService)
	chatService.SetMemoryService(memoryService)
	chatService.SetScheduleService(scheduleService)
	chatService.SetAgentsInstructions(agentsService)
	if err := chatService.RecoverBackgroundTasks(context.Background()); err != nil {
		return nil, fmt.Errorf("recover background tasks: %w", err)
	}
	if err := chatService.RecoverCooperativeWorkspaces(context.Background()); err != nil {
		logging.Warn("agent_workspace_recovery", "err", err)
	}
	chatService.SetUploadService(chatUpload)
	chatService.SetContextHistoryRounds(cfg.ContextHistoryRounds)

	planService, err := plansvc.NewPlanService()
	if err != nil {
		return nil, err
	}
	chatService.SetPlanService(planService)
	info := buildversion.Info()
	updateService := updater.NewService(updater.ServiceOptions{CurrentVersion: info.Version})

	privacyService.StartMappingCleanup(context.Background())
	owned = true
	return &Core{
		Config:           cfg,
		Repo:             repo,
		runtimeOwner:     owner,
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
		SkillLearning:    skillLearning,
		ChatUpload:       chatUpload,
		MCPManager:       mcpManager,
		PlanService:      planService,
		UpdateService:    updateService,
		TeamService:      teamService,
		PrivacyService:   privacyService,
		UsageService:     usageService,
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

	if c.ChatService != nil {
		c.ChatService.CloseBackground(ctx)
		if err := c.ChatService.CloseCooperative(ctx); err != nil {
			logging.Warn("agent_runtime_close", "err", err)
		}
	}
	if c.MCPManager != nil {
		c.MCPManager.CloseAll()
	}
	if c.PrivacyService != nil {
		c.PrivacyService.Close()
	}
	if c.Repo != nil {
		if err := c.Repo.Close(); err != nil {
			logging.Warn("db_close", "err", err)
		}
	}
	_ = c.runtimeOwner.Close()
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
