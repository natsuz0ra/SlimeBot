package services

import (
	"strings"

	"slimebot/backend/internal/mcp"
	"slimebot/backend/internal/models"
	"slimebot/backend/internal/repositories"
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
	store repositories.LLMConfigStore
}

func NewLLMConfigService(store repositories.LLMConfigStore) *LLMConfigService {
	return &LLMConfigService{store: store}
}

func (s *LLMConfigService) List() ([]models.LLMConfig, error) {
	return s.store.ListLLMConfigs()
}

func (s *LLMConfigService) Create(input LLMConfigCreateInput) (*models.LLMConfig, error) {
	return s.store.CreateLLMConfig(models.LLMConfig{
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
	store repositories.MCPConfigStore
}

func NewMCPConfigService(store repositories.MCPConfigStore) *MCPConfigService {
	return &MCPConfigService{store: store}
}

func (s *MCPConfigService) List() ([]models.MCPConfig, error) {
	return s.store.ListMCPConfigs()
}

func (s *MCPConfigService) ValidateConfig(raw string) error {
	_, err := mcp.ParseAndValidateConfig(strings.TrimSpace(raw))
	return err
}

func (s *MCPConfigService) Create(input MCPConfigInput) (*models.MCPConfig, error) {
	return s.store.CreateMCPConfig(models.MCPConfig{
		Name:      strings.TrimSpace(input.Name),
		Config:    strings.TrimSpace(input.Config),
		IsEnabled: input.IsEnabled,
	})
}

func (s *MCPConfigService) Update(id string, input MCPConfigInput) error {
	return s.store.UpdateMCPConfig(id, models.MCPConfig{
		Name:      strings.TrimSpace(input.Name),
		Config:    strings.TrimSpace(input.Config),
		IsEnabled: input.IsEnabled,
	})
}

func (s *MCPConfigService) Delete(id string) error {
	return s.store.DeleteMCPConfig(id)
}

type MessagePlatformConfigService struct {
	store repositories.MessagePlatformConfigStore
}

func NewMessagePlatformConfigService(store repositories.MessagePlatformConfigStore) *MessagePlatformConfigService {
	return &MessagePlatformConfigService{store: store}
}

func (s *MessagePlatformConfigService) List() ([]models.MessagePlatformConfig, error) {
	return s.store.ListMessagePlatformConfigs()
}

func (s *MessagePlatformConfigService) Create(input MessagePlatformConfigInput) (*models.MessagePlatformConfig, error) {
	return s.store.CreateMessagePlatformConfig(models.MessagePlatformConfig{
		Platform:       strings.ToLower(strings.TrimSpace(input.Platform)),
		DisplayName:    strings.TrimSpace(input.DisplayName),
		AuthConfigJSON: strings.TrimSpace(input.AuthConfigJSON),
		IsEnabled:      input.IsEnabled,
	})
}

func (s *MessagePlatformConfigService) Update(id string, input MessagePlatformConfigInput) error {
	return s.store.UpdateMessagePlatformConfig(id, models.MessagePlatformConfig{
		Platform:       strings.ToLower(strings.TrimSpace(input.Platform)),
		DisplayName:    strings.TrimSpace(input.DisplayName),
		AuthConfigJSON: strings.TrimSpace(input.AuthConfigJSON),
		IsEnabled:      input.IsEnabled,
	})
}

func (s *MessagePlatformConfigService) Delete(id string) error {
	return s.store.DeleteMessagePlatformConfig(id)
}
