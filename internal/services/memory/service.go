package memory

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"slimebot/internal/constants"
	"slimebot/internal/runtime"
)

type ConfigProvider interface {
	MemoryConfig(ctx context.Context) (Config, error)
}

type StaticSettings struct {
	config Config
}

func NewStaticSettings(config Config) StaticSettings {
	return StaticSettings{config: normalizeConfig(config)}
}

func (s StaticSettings) MemoryConfig(_ context.Context) (Config, error) {
	return normalizeConfig(s.config), nil
}

type settingsStore interface {
	GetSetting(ctx context.Context, key string) (string, error)
}

type SettingsStoreConfigProvider struct {
	store settingsStore
}

func NewSettingsStoreConfigProvider(store settingsStore) SettingsStoreConfigProvider {
	return SettingsStoreConfigProvider{store: store}
}

func (p SettingsStoreConfigProvider) MemoryConfig(ctx context.Context) (Config, error) {
	cfg := DefaultConfig()
	if p.store == nil {
		return cfg, nil
	}
	var err error
	if cfg.Enabled, err = getBoolSetting(ctx, p.store, constants.SettingMemoryEnabled, cfg.Enabled); err != nil {
		return Config{}, err
	}
	if cfg.UserProfileEnabled, err = getBoolSetting(ctx, p.store, constants.SettingMemoryUserProfileEnabled, cfg.UserProfileEnabled); err != nil {
		return Config{}, err
	}
	if cfg.MemoryCharLimit, err = getIntSetting(ctx, p.store, constants.SettingMemoryCharLimit, cfg.MemoryCharLimit); err != nil {
		return Config{}, err
	}
	if cfg.UserCharLimit, err = getIntSetting(ctx, p.store, constants.SettingMemoryUserCharLimit, cfg.UserCharLimit); err != nil {
		return Config{}, err
	}
	if cfg.NudgeInterval, err = getIntSetting(ctx, p.store, constants.SettingMemoryNudgeInterval, cfg.NudgeInterval); err != nil {
		return Config{}, err
	}
	return normalizeConfig(cfg), nil
}

type Service struct {
	store    *Store
	provider ConfigProvider
}

func DefaultDir() string {
	return filepath.Join(runtime.SlimeBotHomeDir(), "memories")
}

func NewService(store *Store, provider ConfigProvider) *Service {
	if store == nil {
		store = NewStore(DefaultDir())
	}
	if provider == nil {
		provider = NewStaticSettings(DefaultConfig())
	}
	return &Service{store: store, provider: provider}
}

func NewServiceFromSettings(store settingsStore) *Service {
	return NewService(NewStore(DefaultDir()), NewSettingsStoreConfigProvider(store))
}

func (s *Service) Config(ctx context.Context) (Config, error) {
	cfg, err := s.provider.MemoryConfig(ctx)
	if err != nil {
		return Config{}, err
	}
	return normalizeConfig(cfg), nil
}

func (s *Service) Add(ctx context.Context, target Target, content string) (TargetState, error) {
	cfg, err := s.configForWrite(ctx, target)
	if err != nil {
		return TargetState{}, err
	}
	return s.store.Add(ctx, target, content, targetLimit(cfg, target))
}

func (s *Service) Replace(ctx context.Context, target Target, oldText string, content string) (TargetState, error) {
	cfg, err := s.configForWrite(ctx, target)
	if err != nil {
		return TargetState{}, err
	}
	return s.store.Replace(ctx, target, oldText, content, targetLimit(cfg, target))
}

func (s *Service) Remove(ctx context.Context, target Target, oldText string) (TargetState, error) {
	cfg, err := s.configForWrite(ctx, target)
	if err != nil {
		return TargetState{}, err
	}
	state, err := s.store.Remove(ctx, target, oldText)
	if err != nil {
		return TargetState{}, err
	}
	state.CharLimit = targetLimit(cfg, target)
	state.Enabled = targetEnabled(cfg, target)
	return state, nil
}

func (s *Service) RemoveIndex(ctx context.Context, target Target, index int) (TargetState, error) {
	cfg, err := s.Config(ctx)
	if err != nil {
		return TargetState{}, err
	}
	state, err := s.store.RemoveIndex(ctx, target, index)
	if err != nil {
		return TargetState{}, err
	}
	state.CharLimit = targetLimit(cfg, target)
	state.Enabled = targetEnabled(cfg, target)
	return state, nil
}

func (s *Service) Clear(ctx context.Context, target Target) (TargetState, error) {
	cfg, err := s.Config(ctx)
	if err != nil {
		return TargetState{}, err
	}
	state, err := s.store.Clear(ctx, target)
	if err != nil {
		return TargetState{}, err
	}
	state.CharLimit = targetLimit(cfg, target)
	state.Enabled = targetEnabled(cfg, target)
	return state, nil
}

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	cfg, err := s.Config(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	memoryState, err := s.store.Load(ctx, TargetMemory, cfg.MemoryCharLimit)
	if err != nil {
		return Snapshot{}, err
	}
	userState, err := s.store.Load(ctx, TargetUser, cfg.UserCharLimit)
	if err != nil {
		return Snapshot{}, err
	}
	memoryState.Enabled = targetEnabled(cfg, TargetMemory)
	userState.Enabled = targetEnabled(cfg, TargetUser)
	return Snapshot{
		Memory:                   memoryState,
		User:                     userState,
		MemoryEnabled:            cfg.Enabled,
		MemoryUserProfileEnabled: cfg.UserProfileEnabled,
		MemoryNudgeInterval:      cfg.NudgeInterval,
		MemoryDirectory:          s.store.Dir(),
	}, nil
}

func (s *Service) FormatForSystemPrompt(ctx context.Context) (string, error) {
	snapshot, err := s.Snapshot(ctx)
	if err != nil {
		return "", err
	}
	var sections []string
	if snapshot.Memory.Enabled && len(snapshot.Memory.Entries) > 0 {
		sections = append(sections, formatTargetBlock("MEMORY (your personal notes)", snapshot.Memory))
	}
	if snapshot.User.Enabled && len(snapshot.User.Entries) > 0 {
		sections = append(sections, formatTargetBlock("USER PROFILE (who the user is)", snapshot.User))
	}
	return strings.Join(sections, "\n\n"), nil
}

func (s *Service) ToolCacheKey(ctx context.Context) string {
	cfg, err := s.Config(ctx)
	if err != nil {
		return "memory:error"
	}
	return fmt.Sprintf("memory:%t:%t:%d:%d:%d", cfg.Enabled, cfg.UserProfileEnabled, cfg.MemoryCharLimit, cfg.UserCharLimit, cfg.NudgeInterval)
}

func (s *Service) ToolsEnabled(ctx context.Context) bool {
	cfg, err := s.Config(ctx)
	return err == nil && cfg.Enabled
}

func (s *Service) configForWrite(ctx context.Context, target Target) (Config, error) {
	cfg, err := s.Config(ctx)
	if err != nil {
		return Config{}, err
	}
	if !cfg.Enabled {
		return Config{}, ErrMemoryDisabled
	}
	if target == TargetUser && !cfg.UserProfileEnabled {
		return Config{}, ErrUserProfileDisabled
	}
	return cfg, nil
}

func formatTargetBlock(title string, state TargetState) string {
	percent := 0
	if state.CharLimit > 0 {
		percent = state.UsageChars * 100 / state.CharLimit
	}
	header := fmt.Sprintf("========================================\n%s [%d%% - %d/%d chars]\n========================================", title, percent, state.UsageChars, state.CharLimit)
	return header + "\n" + strings.Join(state.Entries, "\n\n")
}

func normalizeConfig(cfg Config) Config {
	defaults := DefaultConfig()
	if cfg.MemoryCharLimit <= 0 {
		cfg.MemoryCharLimit = defaults.MemoryCharLimit
	}
	if cfg.UserCharLimit <= 0 {
		cfg.UserCharLimit = defaults.UserCharLimit
	}
	if cfg.NudgeInterval <= 0 {
		cfg.NudgeInterval = defaults.NudgeInterval
	}
	return cfg
}

func getBoolSetting(ctx context.Context, store settingsStore, key string, fallback bool) (bool, error) {
	raw, err := store.GetSetting(ctx, key)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return fallback, nil
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%w %s: %s", ErrInvalidMemorySetting, key, raw)
	}
}

func getIntSetting(ctx context.Context, store settingsStore, key string, fallback int) (int, error) {
	raw, err := store.GetSetting(ctx, key)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%w %s: %s", ErrInvalidMemorySetting, key, raw)
	}
	return value, nil
}
