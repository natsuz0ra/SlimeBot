package settings

import (
	"context"
	"os"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/runtime"
	"strings"
)

// AppSettings is the settings DTO exposed to the frontend.
type AppSettings struct {
	Language                    string
	DefaultModel                string
	MessagePlatformDefaultModel string
	WebSearchAPIKey             string
	ApprovalMode                string
	ThinkingLevel               string
}

// UpdateSettingsInput is the domain input for partial settings updates.
type UpdateSettingsInput struct {
	Language                    string
	DefaultModel                string
	MessagePlatformDefaultModel string
	WebSearchAPIKey             string
	ApprovalMode                string
	ThinkingLevel               string
}

type SettingsService struct {
	store domain.SettingsStore
}

func NewSettingsService(store domain.SettingsStore) *SettingsService {
	return &SettingsService{store: store}
}

// Get loads settings and fills defaults for a stable API surface.
func (s *SettingsService) Get(ctx context.Context) (*AppSettings, error) {
	language, err := s.store.GetSetting(ctx, constants.SettingLanguage)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(language) == "" {
		language = "zh-CN"
	}
	defaultModel, err := s.store.GetSetting(ctx, constants.SettingDefaultModel)
	if err != nil {
		return nil, err
	}
	messagePlatformDefaultModel, err := s.store.GetSetting(ctx, constants.SettingMessagePlatformDefaultModel)
	if err != nil {
		return nil, err
	}
	webSearchAPIKey, err := runtime.ReadEnvValue(constants.SettingWebSearchAPIKey)
	if err != nil {
		return nil, err
	}
	approvalMode, err := s.store.GetSetting(ctx, constants.SettingApprovalMode)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(approvalMode) == "" {
		approvalMode = constants.ApprovalModeStandard
	}
	thinkingLevel, err := s.store.GetSetting(ctx, constants.SettingThinkingLevel)
	if err != nil {
		return nil, err
	}
	return &AppSettings{
		Language:                    language,
		DefaultModel:                defaultModel,
		MessagePlatformDefaultModel: messagePlatformDefaultModel,
		WebSearchAPIKey:             webSearchAPIKey,
		ApprovalMode:                approvalMode,
		ThinkingLevel:               thinkingLevel,
	}, nil
}

// Update applies only fields that are explicitly set in the request.
func (s *SettingsService) Update(ctx context.Context, input UpdateSettingsInput) error {
	if strings.TrimSpace(input.Language) != "" {
		if err := s.store.SetSetting(ctx, constants.SettingLanguage, input.Language); err != nil {
			return err
		}
	}
	if strings.TrimSpace(input.DefaultModel) != "" {
		if err := s.store.SetSetting(ctx, constants.SettingDefaultModel, input.DefaultModel); err != nil {
			return err
		}
	}
	if strings.TrimSpace(input.MessagePlatformDefaultModel) != "" {
		if err := s.store.SetSetting(ctx, constants.SettingMessagePlatformDefaultModel, input.MessagePlatformDefaultModel); err != nil {
			return err
		}
	}
	if strings.TrimSpace(input.WebSearchAPIKey) != "" {
		if err := runtime.UpsertEnvValue(constants.SettingWebSearchAPIKey, input.WebSearchAPIKey); err != nil {
			return err
		}
		if err := os.Setenv(constants.SettingWebSearchAPIKey, input.WebSearchAPIKey); err != nil {
			return err
		}
	}
	if strings.TrimSpace(input.ApprovalMode) != "" {
		if err := s.store.SetSetting(ctx, constants.SettingApprovalMode, input.ApprovalMode); err != nil {
			return err
		}
	}
	if strings.TrimSpace(input.ThinkingLevel) != "" {
		if err := s.store.SetSetting(ctx, constants.SettingThinkingLevel, input.ThinkingLevel); err != nil {
			return err
		}
	}
	return nil
}
