package config

import (
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

func (s *MCPConfigService) List() ([]domain.MCPConfig, error) {
	return s.store.ListMCPConfigs()
}

func (s *MCPConfigService) ValidateConfig(raw string) error {
	_, err := mcp.ParseAndValidateConfig(strings.TrimSpace(raw))
	return err
}

func (s *MCPConfigService) Create(input MCPConfigInput) (*domain.MCPConfig, error) {
	return s.store.CreateMCPConfig(domain.MCPConfig{
		Name:      strings.TrimSpace(input.Name),
		Config:    strings.TrimSpace(input.Config),
		IsEnabled: input.IsEnabled,
	})
}

func (s *MCPConfigService) Update(id string, input MCPConfigInput) error {
	return s.store.UpdateMCPConfig(id, domain.MCPConfig{
		Name:      strings.TrimSpace(input.Name),
		Config:    strings.TrimSpace(input.Config),
		IsEnabled: input.IsEnabled,
	})
}

func (s *MCPConfigService) Delete(id string) error {
	return s.store.DeleteMCPConfig(id)
}
