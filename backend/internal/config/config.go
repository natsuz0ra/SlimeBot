package config

import (
	"os"
	"strconv"
)

type Config struct {
	ServerPort       string
	DBPath           string
	Frontend         string
	SkillsRoot       string
	JWTSecret        string
	JWTExpireMinutes int
}

func Load() Config {
	return Config{
		ServerPort:       getEnv("SERVER_PORT", "8080"),
		DBPath:           getEnv("DB_PATH", "./storage/data.db"),
		Frontend:         getEnv("FRONTEND_ORIGIN", "http://localhost:5173"),
		SkillsRoot:       getEnv("SKILLS_ROOT", "./skills"),
		JWTSecret:        getEnv("JWT_SECRET", ""),
		JWTExpireMinutes: GetIntEnv("JWT_EXPIRE", 15*24*60),
	}
}

func getEnv(key, fallback string) string {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		return fallback
	}
	return value
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
