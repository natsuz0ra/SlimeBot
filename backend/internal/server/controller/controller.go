package controller

import (
	"mime/multipart"
	"slimebot/backend/internal/domain"
	"time"

	"slimebot/backend/internal/auth"
	chatsvc "slimebot/backend/internal/services/chat"
	configsvc "slimebot/backend/internal/services/config"
	settingssvc "slimebot/backend/internal/services/settings"
)

type authService interface {
	VerifyLogin(username, password string) (bool, error)
	MustChangePassword() (bool, error)
	UpdateAccount(username, oldPassword, newPassword string) error
}

type sessionService interface {
	List() ([]domain.Session, error)
	Create(name string) (*domain.Session, error)
	RenameByUser(id, name string) error
	Delete(id string) error
	ListMessagesPage(sessionID string, limit int, before *time.Time, after *time.Time) ([]domain.Message, bool, error)
	ListToolCallRecords(sessionID string) ([]domain.ToolCallRecord, error)
	SetModel(sessionID, modelConfigID string) error
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

// HTTPController 聚合 REST 接口依赖，负责参数/响应层处理。
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
	tokenManager *auth.TokenManager
}

// NewHTTPController 组装 HTTP 控制器并注入所需服务。
func NewHTTPController(
	authService authService,
	sessionsService sessionService,
	settingsService settingsService,
	llmConfigsService llmConfigService,
	mcpConfigsService mcpConfigService,
	platformsService messagePlatformConfigService,
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
		tokenManager: tokenManager,
	}
}
