package config

import (
	"os"
	"path/filepath"
	"strconv"

	"slimebot/internal/runtime"
)

type Config struct {
	ServerPort string
	DBPath     string
	Frontend   string
	SkillsRoot string

	ChatUploadRoot   string
	JWTSecret        string
	JWTExpireMinutes int

	MemoryDir string

	ContextHistoryRounds         int
	MemoryAsyncWriteEnabled      bool
	MemoryAsyncWorkerIntervalSec int
	MemoryWriteMaxRetries        int
}

func Load() Config {
	home := runtime.SlimeBotHomeDir()

	return Config{
		ServerPort:                   getEnv("SERVER_PORT", "8080"),
		DBPath:                       getPathEnv("DB_PATH", filepath.Join(home, "storage", "data.db")),
		Frontend:                     getEnv("FRONTEND_ORIGIN", ""),
		SkillsRoot:                   getPathEnv("SKILLS_ROOT", filepath.Join(home, "skills")),
		ChatUploadRoot:               getPathEnv("CHAT_UPLOAD_ROOT", filepath.Join(home, "storage", "chat_uploads")),
		JWTSecret:                    getEnv("JWT_SECRET", ""),
		JWTExpireMinutes:             GetIntEnv("JWT_EXPIRE", 15*24*60),
		MemoryDir:                    getPathEnv("MEMORY_DIR", filepath.Join(home, "memory")),
		ContextHistoryRounds:         GetIntEnv("CONTEXT_HISTORY_ROUNDS", 20),
		MemoryAsyncWriteEnabled:      getBoolEnv("MEMORY_ASYNC_WRITE_ENABLED", true),
		MemoryAsyncWorkerIntervalSec: GetIntEnv("MEMORY_ASYNC_WORKER_INTERVAL", 2),
		MemoryWriteMaxRetries:        GetIntEnv("MEMORY_WRITE_MAX_RETRIES", 5),
	}
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

func GetIntEnv(key string, fallback int) int {
	value := getEnv(key, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getBoolEnv(key string, fallback bool) bool {
	value := getEnv(key, "")
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "TRUE", "True", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "False", "no", "NO", "off", "OFF":
		return false
	default:
		return fallback
	}
}
