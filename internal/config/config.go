package config

import (
	"os"
	"strconv"
)

type Config struct {
	ServerPort string
	DBPath     string
	Frontend   string
	SkillsRoot string
	// ChatUploadRoot 用于存放聊天附件临时文件（回合结束后会清理）。
	ChatUploadRoot   string
	SystemPromptPath string
	JWTSecret        string
	JWTExpireMinutes int

	EmbeddingProvider      string
	EmbeddingModelPath     string
	EmbeddingTokenizerPath string
	EmbeddingPythonBin     string
	EmbeddingScriptPath    string
	EmbeddingTimeoutMS     int
	QdrantURL              string
	QdrantCollection       string
	MemoryVectorTopK       int
}

func Load() Config {
	return Config{
		ServerPort:             getEnv("SERVER_PORT", "8080"),
		DBPath:                 getEnv("DB_PATH", "./storage/data.db"),
		Frontend:               getEnv("FRONTEND_ORIGIN", ""),
		SkillsRoot:             getEnv("SKILLS_ROOT", "./skills"),
		ChatUploadRoot:         getEnv("CHAT_UPLOAD_ROOT", "./storage/chat_uploads"),
		SystemPromptPath:       getEnv("SYSTEM_PROMPT_PATH", "./prompts/system_prompt.md"),
		JWTSecret:              getEnv("JWT_SECRET", ""),
		JWTExpireMinutes:       GetIntEnv("JWT_EXPIRE", 15*24*60),
		EmbeddingProvider:      getEnv("EMBEDDING_PROVIDER", "onnx"),
		EmbeddingModelPath:     getEnv("EMBEDDING_MODEL_PATH", "./onnx/model.onnx"),
		EmbeddingTokenizerPath: getEnv("EMBEDDING_TOKENIZER_PATH", "./onnx"),
		EmbeddingPythonBin:     getEnv("EMBEDDING_PYTHON_BIN", "python"),
		EmbeddingScriptPath:    getEnv("EMBEDDING_SCRIPT_PATH", "./scripts/onnx_embed_server.py"),
		EmbeddingTimeoutMS:     GetIntEnv("EMBEDDING_TIMEOUT_MS", 30000),
		QdrantURL:              getEnv("QDRANT_URL", "127.0.0.1:6334"),
		QdrantCollection:       getEnv("QDRANT_COLLECTION", "session_memories"),
		MemoryVectorTopK:       GetIntEnv("MEMORY_VECTOR_TOPK", 5),
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
