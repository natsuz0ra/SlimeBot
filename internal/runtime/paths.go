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

// DescribeConfigHome 生成 SlimeBot 配置目录的内容描述文本。
// 仅扫描顶层条目，由调用方在启动时调用一次并缓存结果。
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
