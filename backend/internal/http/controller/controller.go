package controller

import "slimebot/backend/internal/auth"
import "slimebot/backend/internal/repositories"
import "slimebot/backend/internal/services"

// HTTPController 聚合 REST 接口依赖，负责参数/响应层处理。
type HTTPController struct {
	repo         *repositories.Repository
	skillPackage *services.SkillPackageService
	skillRuntime *services.SkillRuntimeService
	settings     *services.SettingsService
	auth         *services.AuthService
	sessions     *services.SessionService
	llmConfigs   *services.LLMConfigService
	mcpConfigs   *services.MCPConfigService
	platforms    *services.MessagePlatformConfigService
	tokenManager *auth.TokenManager
}

// NewHTTPController 组装 HTTP 控制器并注入所需服务。
func NewHTTPController(
	repo *repositories.Repository,
	skillPackage *services.SkillPackageService,
	skillRuntime *services.SkillRuntimeService,
	tokenManager *auth.TokenManager,
) *HTTPController {
	return &HTTPController{
		repo:         repo,
		skillPackage: skillPackage,
		skillRuntime: skillRuntime,
		settings:     services.NewSettingsService(repo),
		auth:         services.NewAuthService(repo),
		sessions:     services.NewSessionService(repo),
		llmConfigs:   services.NewLLMConfigService(repo),
		mcpConfigs:   services.NewMCPConfigService(repo),
		platforms:    services.NewMessagePlatformConfigService(repo),
		tokenManager: tokenManager,
	}
}
