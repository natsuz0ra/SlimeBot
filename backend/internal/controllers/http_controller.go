package controllers

import "slimebot/backend/internal/repositories"
import "slimebot/backend/internal/services"
import "slimebot/backend/internal/auth"

// HTTPController 聚合 REST 接口依赖，负责参数/响应层处理。
type HTTPController struct {
	repo         *repositories.Repository
	skillPackage *services.SkillPackageService
	skillRuntime *services.SkillRuntimeService
	settings     *services.SettingsService
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
		tokenManager: tokenManager,
	}
}
