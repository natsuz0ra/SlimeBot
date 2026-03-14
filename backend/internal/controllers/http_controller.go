package controllers

import "slimebot/backend/internal/repositories"
import "slimebot/backend/internal/services"
import "slimebot/backend/internal/auth"

type HTTPController struct {
	repo         *repositories.Repository
	skillPackage *services.SkillPackageService
	skillRuntime *services.SkillRuntimeService
	tokenManager *auth.TokenManager
}

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
		tokenManager: tokenManager,
	}
}
