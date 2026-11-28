package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"slimebot/internal/runtime"
)

const (
	defaultJWTExpireMinutes     = 15 * 24 * 60
	defaultContextHistoryRounds = 20
	defaultContextSize          = 1_000_000
)

type Config struct {
	ServerPort        string
	DBPath            string
	Frontend          string
	SkillsRoot        string
	HermesSkillsRoots []string

	ChatUploadRoot   string
	JWTSecret        string
	JWTExpireMinutes int

	ContextHistoryRounds int
	DefaultContextSize   int
}

// Load preserves the legacy forgiving parser for direct library callers.
// Process entrypoints use LoadValidated so explicit invalid values fail early.
func Load() Config {
	cfg, _ := load(false)
	return cfg
}

// LoadValidated parses and normalizes environment configuration, then applies
// one validation contract. Defaults are used only for missing or empty values.
func LoadValidated() (Config, error) {
	cfg, err := load(true)
	if err != nil {
		return Config{}, err
	}
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func load(strict bool) (Config, error) {
	home := runtime.SlimeBotHomeDir()
	serverPort := strings.TrimSpace(getEnv("SERVER_PORT", "6247"))
	if serverPort == "" {
		serverPort = "6247"
	}
	cfg := Config{
		ServerPort:        serverPort,
		DBPath:            getPathEnv("DB_PATH", filepath.Join(home, "storage", "data.db")),
		Frontend:          getEnv("FRONTEND_ORIGIN", ""),
		SkillsRoot:        getPathEnv("SKILLS_ROOT", filepath.Join(home, "skills")),
		HermesSkillsRoots: getPathListEnv("HERMES_SKILLS_ROOTS"),
		ChatUploadRoot:    getPathEnv("CHAT_UPLOAD_ROOT", filepath.Join(home, "storage", "chat_uploads")),
		JWTSecret:         getEnv("JWT_SECRET", ""),
	}
	for _, item := range []struct {
		key      string
		fallback int
		target   *int
	}{
		{key: "JWT_EXPIRE", fallback: defaultJWTExpireMinutes, target: &cfg.JWTExpireMinutes},
		{key: "CONTEXT_HISTORY_ROUNDS", fallback: defaultContextHistoryRounds, target: &cfg.ContextHistoryRounds},
		{key: "DEFAULT_CONTEXT_SIZE", fallback: defaultContextSize, target: &cfg.DefaultContextSize},
	} {
		value := GetIntEnv(item.key, item.fallback)
		if strict {
			parsed, err := parseIntEnv(item.key, item.fallback)
			if err != nil {
				return Config{}, err
			}
			value = parsed
		}
		*item.target = value
	}
	return cfg, nil
}

func validate(cfg Config) error {
	port, err := strconv.Atoi(cfg.ServerPort)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("SERVER_PORT must be an integer between 1 and 65535, got %q", cfg.ServerPort)
	}
	for _, item := range []struct {
		key   string
		value int
	}{
		{key: "JWT_EXPIRE", value: cfg.JWTExpireMinutes},
		{key: "CONTEXT_HISTORY_ROUNDS", value: cfg.ContextHistoryRounds},
		{key: "DEFAULT_CONTEXT_SIZE", value: cfg.DefaultContextSize},
	} {
		if item.value <= 0 {
			return fmt.Errorf("%s must be greater than 0, got %d", item.key, item.value)
		}
	}
	return nil
}

func parseIntEnv(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(getEnv(key, ""))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q: %w", key, raw, err)
	}
	return parsed, nil
}

func getPathListEnv(key string) []string {
	raw := getEnv(key, "")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, string(os.PathListSeparator))
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			paths = append(paths, runtime.ExpandHome(part))
		}
	}
	return paths
}

func getEnv(key, fallback string) string {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		return fallback
	}
	return value
}

func getPathEnv(key, fallback string) string {
	return runtime.ExpandHome(getEnv(key, fallback))
}

// GetIntEnv retains the legacy fallback behavior for compatibility.
func GetIntEnv(key string, fallback int) int {
	value := strings.TrimSpace(getEnv(key, ""))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
