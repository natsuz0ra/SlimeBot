package domain

import "context"

// SettingsReaderWriter 配置读写接口，含布尔便捷方法。
type SettingsReaderWriter interface {
	GetSetting(ctx context.Context, key string) (string, error)
	GetSettingBool(key string, defaultVal bool) (bool, error)
	SetSetting(ctx context.Context, key, value string) error
}

// SettingsStore 键值配置持久化接口。
type SettingsStore interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}
