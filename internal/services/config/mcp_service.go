package config

import (
	"context"
	"strings"

	"slimebot/internal/domain"
	"slimebot/internal/mcp"
)

type MCPConfigInput struct {
	Name      string
	Config    string
	IsEnabled bool
}

type MCPConfigService struct {
	store domain.MCPConfigStore
}

func NewMCPConfigService(store domain.MCPConfigStore) *MCPConfigService {
	return &MCPConfigService{store: store}
}

func (s *MCPConfigService) List(ctx context.Context) ([]domain.MCPConfig, error) {
	return s.store.ListMCPConfigs(ctx)
}

func (s *MCPConfigService) ValidateConfig(raw string) error {
	_, err := mcp.ParseAndValidateConfig(strings.TrimSpace(raw))
	return err
}

func (s *MCPConfigService) Create(ctx context.Context, input MCPConfigInput) (*domain.MCPConfig, error) {
	return s.store.CreateMCPConfig(ctx, domain.MCPConfig{
		Name:      strings.TrimSpace(input.Name),
		Config:    strings.TrimSpace(input.Config),
		IsEnabled: input.IsEnabled,
	})
}

func (s *MCPConfigService) Update(ctx context.Context, id string, input MCPConfigInput) error {
	return s.store.UpdateMCPConfig(ctx, id, domain.MCPConfig{
		Name:      strings.TrimSpace(input.Name),
		Config:    strings.TrimSpace(input.Config),
		IsEnabled: input.IsEnabled,
	})
}

func (s *MCPConfigService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteMCPConfig(ctx, id)
}
