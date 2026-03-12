package controllers

import "corner/backend/internal/repositories"

type HTTPController struct {
	repo *repositories.Repository
}

func NewHTTPController(repo *repositories.Repository) *HTTPController {
	return &HTTPController{repo: repo}
}
