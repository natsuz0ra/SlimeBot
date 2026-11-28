package settings

import (
	"context"
	"os"
	"strconv"
	"strings"
)

type TerminalSettings struct {
	Enabled      bool   `json:"enabled"`
	LockedByEnv  bool   `json:"lockedByEnv"`
	DefaultShell string `json:"defaultShell"`
}

func (s *SettingsService) Terminal(ctx context.Context) (TerminalSettings, error) {
	shell, err := s.store.GetSetting(ctx, "terminal_default_shell")
	if err != nil {
		return TerminalSettings{}, err
	}
	enabled, err := s.getBoolStringSetting(ctx, "terminal_enabled", true)
	if err != nil {
		return TerminalSettings{}, err
	}
	locked := strings.EqualFold(strings.TrimSpace(os.Getenv("SLIMEBOT_TERMINAL_ENABLED")), "false")
	return TerminalSettings{Enabled: enabled && !locked, LockedByEnv: locked, DefaultShell: shell}, nil
}

func (s *SettingsService) UpdateTerminal(ctx context.Context, enabled *bool, shell *string) error {
	if shell != nil {
		if err := s.store.SetSetting(ctx, "terminal_default_shell", strings.TrimSpace(*shell)); err != nil {
			return err
		}
	}
	if enabled != nil {
		return s.store.SetSetting(ctx, "terminal_enabled", strconv.FormatBool(*enabled))
	}
	return nil
}
