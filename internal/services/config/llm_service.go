package config

import (
	"context"
	"strings"

	"slimebot/internal/domain"
)

type LLMConfigCreateInput struct {
	Name     string
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
}

type LLMConfigService struct {
	store domain.LLMConfigStore
}

func NewLLMConfigService(store domain.LLMConfigStore) *LLMConfigService {
	return &LLMConfigService{store: store}
}

func (s *LLMConfigService) List() ([]domain.LLMConfig, error) {
	return s.store.ListLLMConfigs(context.Background())
}

func (s *LLMConfigService) Create(input LLMConfigCreateInput) (*domain.LLMConfig, error) {
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = "openai"
	}
	return s.store.CreateLLMConfig(domain.LLMConfig{
		Name:     strings.TrimSpace(input.Name),
		Provider: provider,
		BaseURL:  strings.TrimSpace(input.BaseURL),
		APIKey:   strings.TrimSpace(input.APIKey),
		Model:    strings.TrimSpace(input.Model),
	})
}

func (s *LLMConfigService) Delete(id string) error {
	return s.store.DeleteLLMConfig(id)
}
