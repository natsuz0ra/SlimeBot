package config

import (
	"context"
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

func (s *MessagePlatformConfigService) List(ctx context.Context) ([]domain.MessagePlatformConfig, error) {
	return s.store.ListMessagePlatformConfigs(ctx)
}

func (s *MessagePlatformConfigService) Create(ctx context.Context, input MessagePlatformConfigInput) (*domain.MessagePlatformConfig, error) {
	return s.store.CreateMessagePlatformConfig(ctx, domain.MessagePlatformConfig{
		Platform:       strings.ToLower(strings.TrimSpace(input.Platform)),
		DisplayName:    strings.TrimSpace(input.DisplayName),
		AuthConfigJSON: strings.TrimSpace(input.AuthConfigJSON),
		IsEnabled:      input.IsEnabled,
	})
}

func (s *MessagePlatformConfigService) Update(ctx context.Context, id string, input MessagePlatformConfigInput) error {
	return s.store.UpdateMessagePlatformConfig(ctx, id, domain.MessagePlatformConfig{
		Platform:       strings.ToLower(strings.TrimSpace(input.Platform)),
		DisplayName:    strings.TrimSpace(input.DisplayName),
		AuthConfigJSON: strings.TrimSpace(input.AuthConfigJSON),
		IsEnabled:      input.IsEnabled,
	})
}

func (s *MessagePlatformConfigService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteMessagePlatformConfig(ctx, id)
}
