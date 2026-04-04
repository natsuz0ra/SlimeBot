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

	EmbeddingProvider             string
	EmbeddingModelPath            string
	EmbeddingTokenizerPath        string
	EmbeddingModelDownloadBaseURL string
	EmbeddingTimeoutMS            int
	EmbeddingORTVersion           string
	EmbeddingORTCacheDir          string
	EmbeddingORTLibPath           string
	EmbeddingORTDownloadBaseURL   string
	ChromaPath                    string
	ChromaCollection              string
	MemoryVectorTopK              int
}

func Load() Config {
	home := runtime.SlimeBotHomeDir()

	return Config{
		ServerPort:                    getEnv("SERVER_PORT", "8080"),
		DBPath:                        getPathEnv("DB_PATH", filepath.Join(home, "storage", "data.db")),
		Frontend:                      getEnv("FRONTEND_ORIGIN", ""),
		SkillsRoot:                    getPathEnv("SKILLS_ROOT", filepath.Join(home, "skills")),
		ChatUploadRoot:                getPathEnv("CHAT_UPLOAD_ROOT", filepath.Join(home, "storage", "chat_uploads")),
		JWTSecret:                     getEnv("JWT_SECRET", ""),
		JWTExpireMinutes:              GetIntEnv("JWT_EXPIRE", 15*24*60),
		EmbeddingProvider:             getEnv("EMBEDDING_PROVIDER", "onnx_go"),
		EmbeddingModelPath:            getPathEnv("EMBEDDING_MODEL_PATH", filepath.Join(home, "onnx", "model.onnx")),
		EmbeddingTokenizerPath:        getPathEnv("EMBEDDING_TOKENIZER_PATH", filepath.Join(home, "onnx", "tokenizer.json")),
		EmbeddingModelDownloadBaseURL: getEnv("EMBEDDING_MODEL_DOWNLOAD_BASE_URL", "https://huggingface.co/BAAI/bge-m3/resolve/main/onnx"),
		EmbeddingTimeoutMS:            GetIntEnv("EMBEDDING_TIMEOUT_MS", 30000),
		EmbeddingORTVersion:           getEnv("EMBEDDING_ORT_VERSION", "1.24.1"),
		EmbeddingORTCacheDir:          getPathEnv("EMBEDDING_ORT_CACHE_DIR", filepath.Join(home, "onnx", "runtime")),
		EmbeddingORTLibPath:           getPathEnv("EMBEDDING_ORT_LIB_PATH", ""),
		EmbeddingORTDownloadBaseURL:   getEnv("EMBEDDING_ORT_DOWNLOAD_BASE_URL", "https://github.com/microsoft/onnxruntime/releases/download"),
		ChromaPath:                    getPathEnv("CHROMA_PATH", filepath.Join(home, "storage", "chroma")),
		ChromaCollection:              getEnv("CHROMA_COLLECTION", "session_memories"),
		MemoryVectorTopK:              GetIntEnv("MEMORY_VECTOR_TOPK", 5),
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
