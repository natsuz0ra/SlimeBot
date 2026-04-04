package settings

import (
	"context"
	"os"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/runtime"
	"strings"
)

// AppSettings 是后端对前端暴露的设置视图模型。
type AppSettings struct {
	Language                    string
	DefaultModel                string
	MessagePlatformDefaultModel string
	WebSearchAPIKey             string
}

// UpdateSettingsInput 是设置更新请求的领域输入。
type UpdateSettingsInput struct {
	Language                    string
	DefaultModel                string
	MessagePlatformDefaultModel string
	WebSearchAPIKey             string
}

type SettingsService struct {
	store domain.SettingsStore
}

func NewSettingsService(store domain.SettingsStore) *SettingsService {
	return &SettingsService{store: store}
}

// Get 读取设置并补齐默认值，保证接口返回稳定字段。
func (s *SettingsService) Get() (*AppSettings, error) {
	language, err := s.store.GetSetting(context.Background(), constants.SettingLanguage)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(language) == "" {
		language = "zh-CN"
	}
	defaultModel, err := s.store.GetSetting(context.Background(), constants.SettingDefaultModel)
	if err != nil {
		return nil, err
	}
	messagePlatformDefaultModel, err := s.store.GetSetting(context.Background(), constants.SettingMessagePlatformDefaultModel)
	if err != nil {
		return nil, err
	}
	webSearchAPIKey, err := runtime.ReadEnvValue(constants.SettingWebSearchAPIKey)
	if err != nil {
		return nil, err
	}
	return &AppSettings{
		Language:                    language,
		DefaultModel:                defaultModel,
		MessagePlatformDefaultModel: messagePlatformDefaultModel,
		WebSearchAPIKey:             webSearchAPIKey,
	}, nil
}

// Update 仅更新请求中显式提供的字段，避免覆盖未传值配置。
func (s *SettingsService) Update(input UpdateSettingsInput) error {
	if strings.TrimSpace(input.Language) != "" {
		if err := s.store.SetSetting(context.Background(), constants.SettingLanguage, input.Language); err != nil {
			return err
		}
	}
	if strings.TrimSpace(input.DefaultModel) != "" {
		if err := s.store.SetSetting(context.Background(), constants.SettingDefaultModel, input.DefaultModel); err != nil {
			return err
		}
	}
	if strings.TrimSpace(input.MessagePlatformDefaultModel) != "" {
		if err := s.store.SetSetting(context.Background(), constants.SettingMessagePlatformDefaultModel, input.MessagePlatformDefaultModel); err != nil {
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
	return nil
}
