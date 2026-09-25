package runtime

import (
	"os"
	"path/filepath"
	"strings"
)

const SlimeBotDirName = ".slimebot"
const GlobalAgentsFileName = "AGENTS.md"

func SlimeBotHomeDir() string {
	if configured := strings.TrimSpace(os.Getenv("SLIMEBOT_HOME")); configured != "" {
		return filepath.Clean(ExpandHome(configured))
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return SlimeBotDirName
	}
	return filepath.Join(home, SlimeBotDirName)
}

func GlobalAgentsPath() string {
	return filepath.Join(SlimeBotHomeDir(), GlobalAgentsFileName)
}

// DescribeConfigHome returns a human-readable listing of the SlimeBot config directory.
// Only top-level entries are scanned; call once at startup and cache if needed.
func DescribeConfigHome() string {
	home := SlimeBotHomeDir()
	entries, err := os.ReadDir(home)
	if err != nil {
		return home + " (directory not yet created)"
	}
	var b strings.Builder
	b.WriteString(home)
	b.WriteString("/\n")
	for _, e := range entries {
		b.WriteString("  ")
		b.WriteString(e.Name())
		if e.IsDir() {
			b.WriteString("/")
		}
		b.WriteString("\n")
	}
	return b.String()
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
