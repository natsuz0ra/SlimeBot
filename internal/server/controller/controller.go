package controller

import (
	"context"
	"mime/multipart"
	"slimebot/internal/domain"
	"time"

	"slimebot/internal/auth"
	chatsvc "slimebot/internal/services/chat"
	configsvc "slimebot/internal/services/config"
	sessionsvc "slimebot/internal/services/session"
	settingssvc "slimebot/internal/services/settings"
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

type llmConfigService interface {
	List(ctx context.Context) ([]domain.LLMConfig, error)
	Create(ctx context.Context, input configsvc.LLMConfigCreateInput) (*domain.LLMConfig, error)
	Update(ctx context.Context, id string, input configsvc.LLMConfigInput) error
	Delete(ctx context.Context, id string) error
}

type mcpConfigService interface {
	List(ctx context.Context) ([]domain.MCPConfig, error)
	ValidateConfig(raw string) error
	Create(ctx context.Context, input configsvc.MCPConfigInput) (*domain.MCPConfig, error)
	Update(ctx context.Context, id string, input configsvc.MCPConfigInput) error
	Delete(ctx context.Context, id string) error
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
}

type chatUploadService interface {
	SaveFiles(sessionID string, files []*multipart.FileHeader) ([]chatsvc.UploadedAttachment, error)
}

type chatContextUsageService interface {
	GetContextUsage(ctx context.Context, sessionID string, modelID string) (chatsvc.ContextUsage, error)
}

// HTTPController wires REST handlers and request/response shaping.
type HTTPController struct {
	skillPackage skillPackageService
	skillRuntime skillRuntimeService
	chatUploads  chatUploadService
	settings     settingsService
	auth         authService
	sessions     sessionService
	llmConfigs   llmConfigService
	mcpConfigs   mcpConfigService
	platforms    messagePlatformConfigService
	plans        planService
	chatUsage    chatContextUsageService
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
