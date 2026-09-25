package config

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

type LLMConfigInput struct {
	Name              string
	ProviderID        string
	Provider          string
	BaseURL           string
	APIKey            string
	Model             string
	ContextSize       int
	ContextSizeSource string
}

type LLMConfigCreateInput = LLMConfigInput

type LLMConfigService struct {
	store              domain.LLMConfigStore
	providers          domain.LLMProviderStore
	defaultContextSize int
}

func NewLLMConfigService(store domain.LLMConfigStore, defaultContextSize ...int) *LLMConfigService {
	size := constants.DefaultContextSize
	if len(defaultContextSize) > 0 && defaultContextSize[0] > 0 {
		size = defaultContextSize[0]
	}
	providers, _ := store.(domain.LLMProviderStore)
	return &LLMConfigService{store: store, providers: providers, defaultContextSize: size}
}

func (s *LLMConfigService) List(ctx context.Context) ([]domain.LLMConfig, error) {
	return s.store.ListLLMConfigs(ctx)
}

func (s *LLMConfigService) Create(ctx context.Context, input LLMConfigCreateInput) (*domain.LLMConfig, error) {
	config, err := s.buildConfig(ctx, input)
	if err != nil {
		return nil, err
	}
	return s.store.CreateLLMConfig(ctx, config)
}

func (s *LLMConfigService) Update(ctx context.Context, id string, input LLMConfigInput) error {
	config, err := s.buildConfig(ctx, input)
	if err != nil {
		return err
	}
	return s.store.UpdateLLMConfig(ctx, id, config)
}

func (s *LLMConfigService) buildConfig(ctx context.Context, input LLMConfigInput) (domain.LLMConfig, error) {
	if input.ProviderID != "" {
		if s.providers == nil {
			return domain.LLMConfig{}, errors.New("provider store is unavailable")
		}
		provider, err := s.providers.GetLLMProvider(ctx, input.ProviderID)
		if err != nil {
			return domain.LLMConfig{}, err
		}
		input.Provider = provider.Protocol
		input.BaseURL = provider.BaseURL
		input.APIKey = providerRuntimeKey(*provider)
	}
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = "openai"
	}
	contextSize, contextSizeSource := s.contextSizeForInput(ctx, input)
	return domain.LLMConfig{
		Name:              strings.TrimSpace(input.Name),
		ProviderID:        strings.TrimSpace(input.ProviderID),
		Provider:          provider,
		BaseURL:           strings.TrimSpace(input.BaseURL),
		APIKey:            strings.TrimSpace(input.APIKey),
		Model:             strings.TrimSpace(input.Model),
		ContextSize:       contextSize,
		ContextSizeSource: contextSizeSource,
	}, nil
}

type LLMProviderInput struct {
	Name        string
	Protocol    string
	BaseURL     string
	APIKey      string
	ClearAPIKey bool
}

func (s *LLMConfigService) ListProviders(ctx context.Context) ([]domain.LLMProvider, error) {
	return s.providers.ListLLMProviders(ctx)
}

func (s *LLMConfigService) CreateProvider(ctx context.Context, input LLMProviderInput) (*domain.LLMProvider, error) {
	provider, err := buildProvider(input)
	if err != nil {
		return nil, err
	}
	return s.providers.CreateLLMProvider(ctx, provider)
}

func (s *LLMConfigService) UpdateProvider(ctx context.Context, id string, input LLMProviderInput) error {
	if input.ClearAPIKey {
		input.APIKey = ""
	}
	if strings.TrimSpace(input.APIKey) == "" && !input.ClearAPIKey {
		current, err := s.providers.GetLLMProvider(ctx, id)
		if err != nil {
			return err
		}
		input.APIKey = current.APIKey
	}
	provider, err := buildProvider(input)
	if err != nil {
		return err
	}
	return s.providers.UpdateLLMProvider(ctx, id, provider)
}

func (s *LLMConfigService) DeleteProvider(ctx context.Context, id string) error {
	return s.providers.DeleteLLMProvider(ctx, id)
}

func buildProvider(input LLMProviderInput) (domain.LLMProvider, error) {
	item := domain.LLMProvider{
		Name: strings.TrimSpace(input.Name), Protocol: strings.TrimSpace(input.Protocol),
		BaseURL: strings.TrimRight(strings.TrimSpace(input.BaseURL), "/"), APIKey: strings.TrimSpace(input.APIKey),
	}
	if item.Name == "" || item.BaseURL == "" {
		return item, errors.New("provider name and baseUrl are required")
	}
	if item.Protocol != "openai" && item.Protocol != "anthropic" && item.Protocol != "deepseek" {
		return item, errors.New("unsupported provider protocol")
	}
	parsed, err := url.Parse(item.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return item, fmt.Errorf("invalid provider baseUrl")
	}
	if item.Protocol == "anthropic" && item.APIKey == "" {
		return item, errors.New("Anthropic API key is required")
	}
	return item, nil
}

func providerRuntimeKey(provider domain.LLMProvider) string {
	if provider.APIKey != "" {
		return provider.APIKey
	}
	return "no-key-required"
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

func (s *LLMConfigService) contextSizeForInput(ctx context.Context, input LLMConfigInput) (int, string) {
	switch input.ContextSizeSource {
	case "detected":
		if input.ContextSize > 0 {
			return input.ContextSize, "detected"
		}
	case "fallback":
		return s.resolveContextSize(0), "fallback"
	case "auto":
		if input.ProviderID != "" && strings.TrimSpace(input.Model) != "" {
			models, err := s.DiscoverModels(ctx, input.ProviderID)
			if err == nil {
				for _, model := range models {
					if model.ID == strings.TrimSpace(input.Model) && model.ContextSize > 0 {
						return model.ContextSize, "detected"
					}
				}
			}
		}
		return s.resolveContextSize(0), "fallback"
	}
	if input.ContextSize > 0 {
		return input.ContextSize, "manual"
	}
	return s.resolveContextSize(0), "fallback"
}
