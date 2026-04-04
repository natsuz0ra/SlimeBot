package runtime

import (
	"os"
	"path/filepath"
	"strings"
)

const SlimeBotDirName = ".slimebot"

func SlimeBotHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return SlimeBotDirName
	}
	return filepath.Join(home, SlimeBotDirName)
}

func ExpandHome(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if trimmed == "~" {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return trimmed
		}
		return home
	}
	if strings.HasPrefix(trimmed, "~/") || strings.HasPrefix(trimmed, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return trimmed
		}
		suffix := strings.TrimPrefix(strings.TrimPrefix(trimmed, "~/"), "~\\")
		return filepath.Join(home, suffix)
	}
	return trimmed
}
