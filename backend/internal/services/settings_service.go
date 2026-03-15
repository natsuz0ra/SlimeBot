package services

import (
	"strings"
)

const (
	settingLanguage     = "language"
	settingDefaultModel = "defaultModel"
)

// SettingsStore 抽象设置读写能力，便于 controller 与 repo 解耦。
type SettingsStore interface {
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
}

// AppSettings 是后端对前端暴露的设置视图模型。
type AppSettings struct {
	Language     string
	DefaultModel string
}

// UpdateSettingsInput 是设置更新请求的领域输入。
type UpdateSettingsInput struct {
	Language     string
	DefaultModel string
}

type SettingsService struct {
	store SettingsStore
}

func NewSettingsService(store SettingsStore) *SettingsService {
	return &SettingsService{store: store}
}

// Get 读取设置并补齐默认值，保证接口返回稳定字段。
func (s *SettingsService) Get() (*AppSettings, error) {
	language, err := s.store.GetSetting(settingLanguage)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(language) == "" {
		language = "zh-CN"
	}
	defaultModel, err := s.store.GetSetting(settingDefaultModel)
	if err != nil {
		return nil, err
	}
	return &AppSettings{
		Language:     language,
		DefaultModel: defaultModel,
	}, nil
}

// Update 仅更新请求中显式提供的字段，避免覆盖未传值配置。
func (s *SettingsService) Update(input UpdateSettingsInput) error {
	if strings.TrimSpace(input.Language) != "" {
		if err := s.store.SetSetting(settingLanguage, input.Language); err != nil {
			return err
		}
	}
	if strings.TrimSpace(input.DefaultModel) != "" {
		if err := s.store.SetSetting(settingDefaultModel, input.DefaultModel); err != nil {
			return err
		}
	}
	return nil
}
