package controllers

import "slimebot/backend/internal/repositories"
import "slimebot/backend/internal/services"

type HTTPController struct {
	repo         *repositories.Repository
	skillPackage *services.SkillPackageService
	skillRuntime *services.SkillRuntimeService
}

func NewHTTPController(repo *repositories.Repository, skillPackage *services.SkillPackageService, skillRuntime *services.SkillRuntimeService) *HTTPController {
	return &HTTPController{
		repo:         repo,
		skillPackage: skillPackage,
		skillRuntime: skillRuntime,
	}
}
