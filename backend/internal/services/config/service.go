package config

import (
	"strings"

	"slimebot/backend/internal/domain"
	"slimebot/backend/internal/mcp"
)

type LLMConfigCreateInput struct {
	Name    string
	BaseURL string
	APIKey  string
	Model   string
}

type MCPConfigInput struct {
	Name      string
	Config    string
	IsEnabled bool
}

type MessagePlatformConfigInput struct {
	Platform       string
	DisplayName    string
	AuthConfigJSON string
	IsEnabled      bool
}

type LLMConfigService struct {
	store domain.LLMConfigStore
}

func NewLLMConfigService(store domain.LLMConfigStore) *LLMConfigService {
	return &LLMConfigService{store: store}
}

func (s *LLMConfigService) List() ([]domain.LLMConfig, error) {
	return s.store.ListLLMConfigs()
}

func (s *LLMConfigService) Create(input LLMConfigCreateInput) (*domain.LLMConfig, error) {
	return s.store.CreateLLMConfig(domain.LLMConfig{
		Name:    strings.TrimSpace(input.Name),
		BaseURL: strings.TrimSpace(input.BaseURL),
		APIKey:  strings.TrimSpace(input.APIKey),
		Model:   strings.TrimSpace(input.Model),
	})
}

func (s *LLMConfigService) Delete(id string) error {
	return s.store.DeleteLLMConfig(id)
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

type MessagePlatformConfigService struct {
	store domain.MessagePlatformConfigStore
}

func NewMessagePlatformConfigService(store domain.MessagePlatformConfigStore) *MessagePlatformConfigService {
	return &MessagePlatformConfigService{store: store}
}

func (s *MessagePlatformConfigService) List() ([]domain.MessagePlatformConfig, error) {
	return s.store.ListMessagePlatformConfigs()
}

func (s *MessagePlatformConfigService) Create(input MessagePlatformConfigInput) (*domain.MessagePlatformConfig, error) {
	return s.store.CreateMessagePlatformConfig(domain.MessagePlatformConfig{
		Platform:       strings.ToLower(strings.TrimSpace(input.Platform)),
		DisplayName:    strings.TrimSpace(input.DisplayName),
		AuthConfigJSON: strings.TrimSpace(input.AuthConfigJSON),
		IsEnabled:      input.IsEnabled,
	})
}

func (s *MessagePlatformConfigService) Update(id string, input MessagePlatformConfigInput) error {
	return s.store.UpdateMessagePlatformConfig(id, domain.MessagePlatformConfig{
		Platform:       strings.ToLower(strings.TrimSpace(input.Platform)),
		DisplayName:    strings.TrimSpace(input.DisplayName),
		AuthConfigJSON: strings.TrimSpace(input.AuthConfigJSON),
		IsEnabled:      input.IsEnabled,
	})
}

func (s *MessagePlatformConfigService) Delete(id string) error {
	return s.store.DeleteMessagePlatformConfig(id)
}
