package controller

import (
	"context"
	"mime/multipart"
	"slimebot/internal/domain"
	"time"

	"slimebot/internal/auth"
	agentssvc "slimebot/internal/services/agents"
	chatsvc "slimebot/internal/services/chat"
	configsvc "slimebot/internal/services/config"
	memorysvc "slimebot/internal/services/memory"
	sessionsvc "slimebot/internal/services/session"
	settingssvc "slimebot/internal/services/settings"
	"slimebot/internal/updater"
)

type authService interface {
	VerifyLogin(ctx context.Context, username, password string) (bool, error)
	MustChangePassword(ctx context.Context) (bool, error)
	UpdateAccount(ctx context.Context, username, oldPassword, newPassword string) error
}

type sessionService interface {
	List(ctx context.Context, limit int, offset int, query string) (sessionsvc.ListResult, error)
	Create(ctx context.Context, name string) (*domain.Session, error)
	RenameByUser(ctx context.Context, id, name string) error
	Delete(ctx context.Context, id string) error
	GetMessageHistory(ctx context.Context, sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) (sessionsvc.MessageHistoryPage, error)
}

type settingsService interface {
	Get(ctx context.Context) (*settingssvc.AppSettings, error)
	Update(ctx context.Context, input settingssvc.UpdateSettingsInput) error
}

type memoryService interface {
	Snapshot(ctx context.Context) (memorysvc.Snapshot, error)
	Clear(ctx context.Context, target memorysvc.Target) (memorysvc.TargetState, error)
	RemoveIndex(ctx context.Context, target memorysvc.Target, index int) (memorysvc.TargetState, error)
}

type agentsInstructionsService interface {
	ReadGlobal(ctx context.Context) (agentssvc.File, error)
	UpdateGlobal(ctx context.Context, content string) error
}

type llmConfigService interface {
	List(ctx context.Context) ([]domain.LLMConfig, error)
	Create(ctx context.Context, input configsvc.LLMConfigCreateInput) (*domain.LLMConfig, error)
	Update(ctx context.Context, id string, input configsvc.LLMConfigInput) error
	Delete(ctx context.Context, id string) error
	ListProviders(ctx context.Context) ([]domain.LLMProvider, error)
	CreateProvider(ctx context.Context, input configsvc.LLMProviderInput) (*domain.LLMProvider, error)
	UpdateProvider(ctx context.Context, id string, input configsvc.LLMProviderInput) error
	DeleteProvider(ctx context.Context, id string) error
	DiscoverModels(ctx context.Context, providerID string) ([]configsvc.DiscoveredModel, error)
}

type mcpConfigService interface {
	List(ctx context.Context) ([]domain.MCPConfig, error)
	ValidateConfig(raw string) error
	Create(ctx context.Context, input configsvc.MCPConfigInput) (*domain.MCPConfig, error)
	Update(ctx context.Context, id string, input configsvc.MCPConfigInput) error
	Delete(ctx context.Context, id string) error
	GetTools(ctx context.Context, id string) (*configsvc.MCPToolListResponse, error)
}

type messagePlatformConfigService interface {
	List(ctx context.Context) ([]domain.MessagePlatformConfig, error)
	Create(ctx context.Context, input configsvc.MessagePlatformConfigInput) (*domain.MessagePlatformConfig, error)
	Update(ctx context.Context, id string, input configsvc.MessagePlatformConfigInput) error
	Delete(ctx context.Context, id string) error
}

type skillPackageService interface {
	InstallFromZip(filename string, data []byte) (*domain.Skill, error)
}

type skillRuntimeService interface {
	ListSkills() ([]domain.Skill, error)
	DeleteSkillByID(id string) error
	SetSkillEnabled(id string, enabled bool) error
}

type chatUploadService interface {
	SaveFiles(sessionID string, files []*multipart.FileHeader) ([]chatsvc.UploadedAttachment, error)
}

type chatContextUsageService interface {
	GetContextUsage(ctx context.Context, sessionID string, modelID string) (chatsvc.ContextUsage, error)
}

type updateService interface {
	Check(ctx context.Context, force bool) (updater.CheckResult, error)
	Status(ctx context.Context) (updater.JobStatus, error)
	Apply(ctx context.Context, req updater.ApplyRequest) (updater.JobStatus, error)
}

// HTTPController wires REST handlers and request/response shaping.
type HTTPController struct {
	skillPackage skillPackageService
	skillRuntime skillRuntimeService
	chatUploads  chatUploadService
	settings     settingsService
	memory       memoryService
	agents       agentsInstructionsService
	auth         authService
	sessions     sessionService
	llmConfigs   llmConfigService
	mcpConfigs   mcpConfigService
	platforms    messagePlatformConfigService
	plans        planService
	chatUsage    chatContextUsageService
	update       updateService
	tokenManager *auth.TokenManager
}

// NewHTTPController constructs the HTTP controller with injected services.
func NewHTTPController(
	authService authService,
	sessionsService sessionService,
	settingsService settingsService,
	llmConfigsService llmConfigService,
	mcpConfigsService mcpConfigService,
	platformsService messagePlatformConfigService,
	plansService planService,
	skillPackage skillPackageService,
	skillRuntime skillRuntimeService,
	chatUploads chatUploadService,
	tokenManager *auth.TokenManager,
) *HTTPController {
	return &HTTPController{
		skillPackage: skillPackage,
		skillRuntime: skillRuntime,
		chatUploads:  chatUploads,
		settings:     settingsService,
		auth:         authService,
		sessions:     sessionsService,
		llmConfigs:   llmConfigsService,
		mcpConfigs:   mcpConfigsService,
		platforms:    platformsService,
		plans:        plansService,
		tokenManager: tokenManager,
	}
}

func (h *HTTPController) SetChatContextUsageService(service chatContextUsageService) {
	h.chatUsage = service
}

func (h *HTTPController) SetAgentsInstructionsService(service agentsInstructionsService) {
	h.agents = service
}

func (h *HTTPController) SetUpdateService(service updateService) {
	h.update = service
}

func (h *HTTPController) SetMemoryService(service memoryService) {
	h.memory = service
}
