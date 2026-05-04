package config

import (
	"context"
	"strings"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

type LLMConfigInput struct {
	Name        string
	Provider    string
	BaseURL     string
	APIKey      string
	Model       string
	ContextSize int
}

type LLMConfigCreateInput = LLMConfigInput

type LLMConfigService struct {
	store              domain.LLMConfigStore
	defaultContextSize int
}

func NewLLMConfigService(store domain.LLMConfigStore, defaultContextSize ...int) *LLMConfigService {
	size := constants.DefaultContextSize
	if len(defaultContextSize) > 0 && defaultContextSize[0] > 0 {
		size = defaultContextSize[0]
	}
	return &LLMConfigService{store: store, defaultContextSize: size}
}

func (s *LLMConfigService) List(ctx context.Context) ([]domain.LLMConfig, error) {
	return s.store.ListLLMConfigs(ctx)
}

func (s *LLMConfigService) Create(ctx context.Context, input LLMConfigCreateInput) (*domain.LLMConfig, error) {
	return s.store.CreateLLMConfig(ctx, s.buildConfig(input))
}

func (s *LLMConfigService) Update(ctx context.Context, id string, input LLMConfigInput) error {
	return s.store.UpdateLLMConfig(ctx, id, s.buildConfig(input))
}

func (s *LLMConfigService) buildConfig(input LLMConfigInput) domain.LLMConfig {
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = "openai"
	}
	return domain.LLMConfig{
		Name:        strings.TrimSpace(input.Name),
		Provider:    provider,
		BaseURL:     strings.TrimSpace(input.BaseURL),
		APIKey:      strings.TrimSpace(input.APIKey),
		Model:       strings.TrimSpace(input.Model),
		ContextSize: s.resolveContextSize(input.ContextSize),
	}
}

func (s *LLMConfigService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteLLMConfig(ctx, id)
}

func (s *LLMConfigService) resolveContextSize(value int) int {
	if value > 0 {
		return value
	}
	if s.defaultContextSize > 0 {
		return s.defaultContextSize
	}
	return constants.DefaultContextSize
}
