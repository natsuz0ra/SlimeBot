package config

import (
	"strings"

	"slimebot/internal/domain"
)

type MessagePlatformConfigInput struct {
	Platform       string
	DisplayName    string
	AuthConfigJSON string
	IsEnabled      bool
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
