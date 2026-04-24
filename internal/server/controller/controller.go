package controller

import (
	"mime/multipart"
	"slimebot/internal/domain"
	"time"

	"slimebot/internal/auth"
	chatsvc "slimebot/internal/services/chat"
	configsvc "slimebot/internal/services/config"
	settingssvc "slimebot/internal/services/settings"
)

type authService interface {
	VerifyLogin(username, password string) (bool, error)
	MustChangePassword() (bool, error)
	UpdateAccount(username, oldPassword, newPassword string) error
}

type sessionService interface {
	List(limit int, offset int, query string) ([]domain.Session, error)
	Create(name string) (*domain.Session, error)
	RenameByUser(id, name string) error
	Delete(id string) error
	ListMessagesPage(sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) ([]domain.Message, bool, error)
	ListToolCallRecordsByAssistantMessageIDs(sessionID string, messageIDs []string) ([]domain.ToolCallRecord, error)
	ListThinkingRecordsByAssistantMessageIDs(sessionID string, messageIDs []string) ([]domain.ThinkingRecord, error)
}

type settingsService interface {
	Get() (*settingssvc.AppSettings, error)
	Update(input settingssvc.UpdateSettingsInput) error
}

type llmConfigService interface {
	List() ([]domain.LLMConfig, error)
	Create(input configsvc.LLMConfigCreateInput) (*domain.LLMConfig, error)
	Delete(id string) error
}

type mcpConfigService interface {
	List() ([]domain.MCPConfig, error)
	ValidateConfig(raw string) error
	Create(input configsvc.MCPConfigInput) (*domain.MCPConfig, error)
	Update(id string, input configsvc.MCPConfigInput) error
	Delete(id string) error
}

type messagePlatformConfigService interface {
	List() ([]domain.MessagePlatformConfig, error)
	Create(input configsvc.MessagePlatformConfigInput) (*domain.MessagePlatformConfig, error)
	Update(id string, input configsvc.MessagePlatformConfigInput) error
	Delete(id string) error
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
