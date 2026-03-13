package controllers

import "corner/backend/internal/repositories"
import "corner/backend/internal/services"

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
