package domain

import "context"

// SettingsReaderWriter reads settings with typed helpers for booleans.
type SettingsReaderWriter interface {
	GetSetting(ctx context.Context, key string) (string, error)
	GetSettingBool(ctx context.Context, key string, defaultVal bool) (bool, error)
	SetSetting(ctx context.Context, key, value string) error
}

// SettingsStore persists key/value settings.
type SettingsStore interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}
