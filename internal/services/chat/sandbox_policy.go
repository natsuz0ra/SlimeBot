package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"slimebot/internal/constants"
	sandboxpolicy "slimebot/internal/sandbox"
)

func (s *ChatService) resolveSandboxPolicy(ctx context.Context, workingDirectory ...string) (*sandboxpolicy.Policy, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve sandbox cwd: %w", err)
	}
	if len(workingDirectory) > 0 && strings.TrimSpace(workingDirectory[0]) != "" && constants.ClientSurfaceFromContext(ctx) != constants.ClientSurfaceCLI {
		cwd = workingDirectory[0]
	}
	mode := string(sandboxpolicy.ModeWorkspaceWrite)
	modeKey := constants.SettingSandboxMode
	writableRootsKey := constants.SettingSandboxWritableRoots
	networkEnabledKey := constants.SettingSandboxNetworkEnabled
	allowedDomainsKey := constants.SettingSandboxNetworkAllowedDomains
	if constants.ClientSurfaceFromContext(ctx) == constants.ClientSurfaceCLI {
		modeKey = constants.SettingCLISandboxMode
		writableRootsKey = constants.SettingCLISandboxWritableRoots
		networkEnabledKey = constants.SettingCLISandboxNetworkEnabled
		allowedDomainsKey = constants.SettingCLISandboxNetworkAllowedDomains
	}
	writableRoots := []string{}
	networkEnabled := true
	allowedDomains := []string{}
	if s.settingsStore != nil {
		if raw, err := s.settingsStore.GetSetting(ctx, modeKey); err == nil && strings.TrimSpace(raw) != "" {
			mode = strings.TrimSpace(raw)
		}
		if raw, err := s.settingsStore.GetSetting(ctx, writableRootsKey); err == nil {
			writableRoots = parseStringSliceSetting(raw)
		}
		if raw, err := s.settingsStore.GetSetting(ctx, networkEnabledKey); err == nil && strings.TrimSpace(raw) != "" {
			switch strings.ToLower(strings.TrimSpace(raw)) {
			case "true", "1", "yes", "on":
				networkEnabled = true
			case "false", "0", "no", "off":
				networkEnabled = false
			default:
				return nil, fmt.Errorf("invalid sandbox network setting: %s", raw)
			}
		}
		if raw, err := s.settingsStore.GetSetting(ctx, allowedDomainsKey); err == nil {
			allowedDomains = parseStringSliceSetting(raw)
		}
	}
	return sandboxpolicy.NewPolicy(sandboxpolicy.Config{
		Mode:          sandboxpolicy.Mode(mode),
		CWD:           cwd,
		WritableRoots: writableRoots,
		Network: sandboxpolicy.NetworkPolicy{
			Enabled:        networkEnabled,
			AllowedDomains: allowedDomains,
		},
	})
}

func parseStringSliceSetting(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	return clean
}
