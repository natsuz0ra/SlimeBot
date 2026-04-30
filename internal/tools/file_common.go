package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

const (
	fileReadDefaultMaxLines = 2000
	fileReadMaxSizeBytes    = 256 * 1024
	fileWriteMaxSizeBytes   = 1024 * 1024
)

func resolveFilePath(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", fmt.Errorf("file_path is required")
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				path = home
			} else if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
				path = filepath.Join(home, path[2:])
			}
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve file_path: %w", err)
	}
	return filepath.Clean(abs), nil
}

func isBlockedDevicePath(path string) bool {
	if runtime.GOOS == "windows" {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	blocked := map[string]struct{}{
		"/dev/zero":    {},
		"/dev/random":  {},
		"/dev/urandom": {},
		"/dev/full":    {},
		"/dev/stdin":   {},
		"/dev/tty":     {},
		"/dev/console": {},
		"/dev/stdout":  {},
		"/dev/stderr":  {},
		"/dev/fd/0":    {},
		"/dev/fd/1":    {},
		"/dev/fd/2":    {},
	}
	if _, ok := blocked[clean]; ok {
		return true
	}
	return strings.HasPrefix(clean, "/proc/") &&
		(strings.HasSuffix(clean, "/fd/0") || strings.HasSuffix(clean, "/fd/1") || strings.HasSuffix(clean, "/fd/2"))
}

func validateTextBytes(data []byte) (string, error) {
	if strings.IndexByte(string(data), 0) >= 0 {
		return "", fmt.Errorf("file appears to be binary; refusing to process as text")
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("file is not valid UTF-8 text; refusing to process it")
	}
	return string(data), nil
}

func fileMTimeUnix(info os.FileInfo) int64 {
	if info == nil {
		return 0
	}
	return info.ModTime().UnixNano()
}

func splitTextLines(content string) []string {
	if content == "" {
		return []string{}
	}
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") && len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func normalizeReplacementLineEndings(existingContent, replacement string) string {
	if strings.Contains(existingContent, "\r\n") && !strings.Contains(replacement, "\r\n") {
		return strings.ReplaceAll(replacement, "\n", "\r\n")
	}
	return replacement
}
