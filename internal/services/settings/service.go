package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slimebot/internal/apperrors"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/runtime"
	sandboxpolicy "slimebot/internal/sandbox"
	"strconv"
	"strings"
)

// AppSettings is the settings DTO exposed to the frontend.
type AppSettings struct {
	Language                        string
	DefaultModel                    string
	MessagePlatformDefaultModel     string
	MessagePlatformThinkingLevel    string
	MessagePlatformApprovalMode     string
	WebSearchAPIKey                 string
	ApprovalMode                    string
	ThinkingLevel                   string
	SandboxMode                     string
	SandboxWritableRoots            []string
	SandboxNetworkEnabled           bool
	SandboxNetworkAllowedDomains    []string
	CLISandboxMode                  string
	CLISandboxWritableRoots         []string
	CLISandboxNetworkEnabled        bool
	CLISandboxNetworkAllowedDomains []string
	MemoryEnabled                   bool
	MemoryUserProfileEnabled        bool
	MemoryCharLimit                 int
	MemoryUserCharLimit             int
	MemoryNudgeInterval             int
}

// UpdateSettingsInput is the domain input for partial settings updates.
type UpdateSettingsInput struct {
	Language                        *string
	DefaultModel                    *string
	MessagePlatformDefaultModel     *string
	MessagePlatformThinkingLevel    *string
	MessagePlatformApprovalMode     *string
	WebSearchAPIKey                 *string
	ApprovalMode                    *string
	ThinkingLevel                   *string
	SandboxMode                     *string
	SandboxWritableRoots            *[]string
	SandboxNetworkEnabled           *bool
	SandboxNetworkAllowedDomains    *[]string
	CLISandboxMode                  *string
	CLISandboxWritableRoots         *[]string
	CLISandboxNetworkEnabled        *bool
	CLISandboxNetworkAllowedDomains *[]string
	MemoryEnabled                   *bool
	MemoryUserProfileEnabled        *bool
	MemoryCharLimit                 *int
	MemoryUserCharLimit             *int
	MemoryNudgeInterval             *int
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
	if messagePlatformDefaultModel != "" {
		if models, ok := s.store.(interface {
			GetLLMConfigByID(context.Context, string) (*domain.LLMConfig, error)
		}); ok {
			_, lookupErr := models.GetLLMConfigByID(ctx, messagePlatformDefaultModel)
			if errors.Is(lookupErr, apperrors.ErrNotFound) {
				messagePlatformDefaultModel = ""
				if err := s.store.SetSetting(ctx, constants.SettingMessagePlatformDefaultModel, ""); err != nil {
					return nil, err
				}
			} else if lookupErr != nil {
				return nil, lookupErr
			}
		}
	}
	messagePlatformThinkingLevel, err := s.store.GetSetting(ctx, constants.SettingMessagePlatformThinkingLevel)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(messagePlatformThinkingLevel) == "" {
		messagePlatformThinkingLevel = "off"
	}
	messagePlatformApprovalMode, err := s.store.GetSetting(ctx, constants.SettingMessagePlatformApprovalMode)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(messagePlatformApprovalMode) == "" {
		messagePlatformApprovalMode = constants.ApprovalModeStandard
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
	sandboxMode, err := s.store.GetSetting(ctx, constants.SettingSandboxMode)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(sandboxMode) == "" {
		sandboxMode = string(sandboxpolicy.ModeWorkspaceWrite)
	}
	sandboxWritableRoots, err := s.getStringSliceSetting(ctx, constants.SettingSandboxWritableRoots)
	if err != nil {
		return nil, err
	}
	sandboxNetworkEnabled, err := s.getBoolStringSetting(ctx, constants.SettingSandboxNetworkEnabled, true)
	if err != nil {
		return nil, err
	}
	sandboxNetworkAllowedDomains, err := s.getStringSliceSetting(ctx, constants.SettingSandboxNetworkAllowedDomains)
	if err != nil {
		return nil, err
	}
	cliSandboxMode, err := s.store.GetSetting(ctx, constants.SettingCLISandboxMode)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cliSandboxMode) == "" {
		cliSandboxMode = string(sandboxpolicy.ModeWorkspaceWrite)
	}
	cliSandboxWritableRoots, err := s.getStringSliceSetting(ctx, constants.SettingCLISandboxWritableRoots)
	if err != nil {
		return nil, err
	}
	cliSandboxNetworkEnabled, err := s.getBoolStringSetting(ctx, constants.SettingCLISandboxNetworkEnabled, true)
	if err != nil {
		return nil, err
	}
	cliSandboxNetworkAllowedDomains, err := s.getStringSliceSetting(ctx, constants.SettingCLISandboxNetworkAllowedDomains)
	if err != nil {
		return nil, err
	}
	memoryEnabled, err := s.getBoolStringSetting(ctx, constants.SettingMemoryEnabled, true)
	if err != nil {
		return nil, err
	}
	memoryUserProfileEnabled, err := s.getBoolStringSetting(ctx, constants.SettingMemoryUserProfileEnabled, true)
	if err != nil {
		return nil, err
	}
	memoryCharLimit, err := s.getIntStringSetting(ctx, constants.SettingMemoryCharLimit, 2200, 200, 20000)
	if err != nil {
		return nil, err
	}
	memoryUserCharLimit, err := s.getIntStringSetting(ctx, constants.SettingMemoryUserCharLimit, 1375, 200, 20000)
	if err != nil {
		return nil, err
	}
	memoryNudgeInterval, err := s.getIntStringSetting(ctx, constants.SettingMemoryNudgeInterval, 10, 1, 100)
	if err != nil {
		return nil, err
	}
	return &AppSettings{
		Language:                        language,
		DefaultModel:                    defaultModel,
		MessagePlatformDefaultModel:     messagePlatformDefaultModel,
		MessagePlatformThinkingLevel:    messagePlatformThinkingLevel,
		MessagePlatformApprovalMode:     messagePlatformApprovalMode,
		WebSearchAPIKey:                 webSearchAPIKey,
		ApprovalMode:                    approvalMode,
		ThinkingLevel:                   thinkingLevel,
		SandboxMode:                     sandboxMode,
		SandboxWritableRoots:            sandboxWritableRoots,
		SandboxNetworkEnabled:           sandboxNetworkEnabled,
		SandboxNetworkAllowedDomains:    sandboxNetworkAllowedDomains,
		CLISandboxMode:                  cliSandboxMode,
		CLISandboxWritableRoots:         cliSandboxWritableRoots,
		CLISandboxNetworkEnabled:        cliSandboxNetworkEnabled,
		CLISandboxNetworkAllowedDomains: cliSandboxNetworkAllowedDomains,
		MemoryEnabled:                   memoryEnabled,
		MemoryUserProfileEnabled:        memoryUserProfileEnabled,
		MemoryCharLimit:                 memoryCharLimit,
		MemoryUserCharLimit:             memoryUserCharLimit,
		MemoryNudgeInterval:             memoryNudgeInterval,
	}, nil
}

// Update applies only fields that are explicitly set in the request.
func (s *SettingsService) Update(ctx context.Context, input UpdateSettingsInput) error {
	if input.Language != nil && strings.TrimSpace(*input.Language) != "" {
		if err := s.store.SetSetting(ctx, constants.SettingLanguage, *input.Language); err != nil {
			return err
		}
	}
	if input.DefaultModel != nil && strings.TrimSpace(*input.DefaultModel) != "" {
		if err := s.store.SetSetting(ctx, constants.SettingDefaultModel, *input.DefaultModel); err != nil {
			return err
		}
	}
	if input.MessagePlatformDefaultModel != nil {
		if err := s.store.SetSetting(ctx, constants.SettingMessagePlatformDefaultModel, *input.MessagePlatformDefaultModel); err != nil {
			return err
		}
	}
	if input.MessagePlatformThinkingLevel != nil {
		thinkingLevel := strings.TrimSpace(*input.MessagePlatformThinkingLevel)
		if thinkingLevel == "" {
			thinkingLevel = "off"
		}
		if err := s.store.SetSetting(ctx, constants.SettingMessagePlatformThinkingLevel, thinkingLevel); err != nil {
			return err
		}
	}
	if input.MessagePlatformApprovalMode != nil {
		approvalMode := strings.TrimSpace(*input.MessagePlatformApprovalMode)
		if !isValidApprovalMode(approvalMode) {
			return fmt.Errorf("invalid message platform approval mode: %s", approvalMode)
		}
		if err := s.store.SetSetting(ctx, constants.SettingMessagePlatformApprovalMode, approvalMode); err != nil {
			return err
		}
	}
	if input.WebSearchAPIKey != nil && strings.TrimSpace(*input.WebSearchAPIKey) != "" {
		if err := runtime.UpsertEnvValue(constants.SettingWebSearchAPIKey, *input.WebSearchAPIKey); err != nil {
			return err
		}
		if err := os.Setenv(constants.SettingWebSearchAPIKey, *input.WebSearchAPIKey); err != nil {
			return err
		}
	}
	if input.ApprovalMode != nil && strings.TrimSpace(*input.ApprovalMode) != "" {
		approvalMode := strings.TrimSpace(*input.ApprovalMode)
		if !isValidApprovalMode(approvalMode) {
			return fmt.Errorf("invalid approval mode: %s", approvalMode)
		}
		if err := s.store.SetSetting(ctx, constants.SettingApprovalMode, approvalMode); err != nil {
			return err
		}
	}
	if input.ThinkingLevel != nil && strings.TrimSpace(*input.ThinkingLevel) != "" {
		if err := s.store.SetSetting(ctx, constants.SettingThinkingLevel, *input.ThinkingLevel); err != nil {
			return err
		}
	}
	if input.SandboxMode != nil && strings.TrimSpace(*input.SandboxMode) != "" {
		mode := strings.TrimSpace(*input.SandboxMode)
		if !isValidSandboxMode(mode) {
			return fmt.Errorf("invalid sandbox mode: %s", mode)
		}
		if err := s.store.SetSetting(ctx, constants.SettingSandboxMode, mode); err != nil {
			return err
		}
	}
	if input.SandboxWritableRoots != nil {
		if err := s.setStringSliceSetting(ctx, constants.SettingSandboxWritableRoots, *input.SandboxWritableRoots); err != nil {
			return err
		}
	}
	if input.SandboxNetworkEnabled != nil {
		if err := s.store.SetSetting(ctx, constants.SettingSandboxNetworkEnabled, fmt.Sprintf("%t", *input.SandboxNetworkEnabled)); err != nil {
			return err
		}
	}
	if input.SandboxNetworkAllowedDomains != nil {
		if err := s.setStringSliceSetting(ctx, constants.SettingSandboxNetworkAllowedDomains, *input.SandboxNetworkAllowedDomains); err != nil {
			return err
		}
	}
	if input.CLISandboxMode != nil && strings.TrimSpace(*input.CLISandboxMode) != "" {
		mode := strings.TrimSpace(*input.CLISandboxMode)
		if !isValidSandboxMode(mode) {
			return fmt.Errorf("invalid CLI sandbox mode: %s", mode)
		}
		if err := s.store.SetSetting(ctx, constants.SettingCLISandboxMode, mode); err != nil {
			return err
		}
	}
	if input.CLISandboxWritableRoots != nil {
		if err := s.setStringSliceSetting(ctx, constants.SettingCLISandboxWritableRoots, *input.CLISandboxWritableRoots); err != nil {
			return err
		}
	}
	if input.CLISandboxNetworkEnabled != nil {
		if err := s.store.SetSetting(ctx, constants.SettingCLISandboxNetworkEnabled, fmt.Sprintf("%t", *input.CLISandboxNetworkEnabled)); err != nil {
			return err
		}
	}
	if input.CLISandboxNetworkAllowedDomains != nil {
		if err := s.setStringSliceSetting(ctx, constants.SettingCLISandboxNetworkAllowedDomains, *input.CLISandboxNetworkAllowedDomains); err != nil {
			return err
		}
	}
	if input.MemoryEnabled != nil {
		if err := s.store.SetSetting(ctx, constants.SettingMemoryEnabled, fmt.Sprintf("%t", *input.MemoryEnabled)); err != nil {
			return err
		}
	}
	if input.MemoryUserProfileEnabled != nil {
		if err := s.store.SetSetting(ctx, constants.SettingMemoryUserProfileEnabled, fmt.Sprintf("%t", *input.MemoryUserProfileEnabled)); err != nil {
			return err
		}
	}
	if input.MemoryCharLimit != nil {
		if err := validateRange("memory char limit", *input.MemoryCharLimit, 200, 20000); err != nil {
			return err
		}
		if err := s.store.SetSetting(ctx, constants.SettingMemoryCharLimit, strconv.Itoa(*input.MemoryCharLimit)); err != nil {
			return err
		}
	}
	if input.MemoryUserCharLimit != nil {
		if err := validateRange("memory user char limit", *input.MemoryUserCharLimit, 200, 20000); err != nil {
			return err
		}
		if err := s.store.SetSetting(ctx, constants.SettingMemoryUserCharLimit, strconv.Itoa(*input.MemoryUserCharLimit)); err != nil {
			return err
		}
	}
	if input.MemoryNudgeInterval != nil {
		if err := validateRange("memory nudge interval", *input.MemoryNudgeInterval, 1, 100); err != nil {
			return err
		}
		if err := s.store.SetSetting(ctx, constants.SettingMemoryNudgeInterval, strconv.Itoa(*input.MemoryNudgeInterval)); err != nil {
			return err
		}
	}
	return nil
}

func isValidApprovalMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case constants.ApprovalModeStandard, constants.ApprovalModeAutoReview, constants.ApprovalModeAuto:
		return true
	default:
		return false
	}
}

func isValidSandboxMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case string(sandboxpolicy.ModeReadOnly), string(sandboxpolicy.ModeWorkspaceWrite), string(sandboxpolicy.ModeDangerFullAccess):
		return true
	default:
		return false
	}
}

func (s *SettingsService) getStringSliceSetting(ctx context.Context, key string) ([]string, error) {
	raw, err := s.store.GetSetting(ctx, key)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("invalid %s setting: %w", key, err)
	}
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	return clean, nil
}

func (s *SettingsService) setStringSliceSetting(ctx context.Context, key string, values []string) error {
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return err
	}
	return s.store.SetSetting(ctx, key, string(raw))
}

func (s *SettingsService) getBoolStringSetting(ctx context.Context, key string, fallback bool) (bool, error) {
	raw, err := s.store.GetSetting(ctx, key)
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
		return false, fmt.Errorf("invalid boolean setting %s: %s", key, raw)
	}
}

func (s *SettingsService) getIntStringSetting(ctx context.Context, key string, fallback int, min int, max int) (int, error) {
	raw, err := s.store.GetSetting(ctx, key)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid integer setting %s: %s", key, raw)
	}
	if err := validateRange(key, value, min, max); err != nil {
		return 0, err
	}
	return value, nil
}

func validateRange(name string, value int, min int, max int) error {
	if value < min || value > max {
		return fmt.Errorf("%s must be between %d and %d", name, min, max)
	}
	return nil
}
