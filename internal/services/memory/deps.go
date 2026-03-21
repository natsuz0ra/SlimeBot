package memory

import (
	embsvc "slimebot/internal/services/embedding"
	oaisvc "slimebot/internal/services/openai"
)

type OpenAIClient = oaisvc.OpenAIClient
type ModelRuntimeConfig = oaisvc.ModelRuntimeConfig
type ChatMessage = oaisvc.ChatMessage
type EmbeddingService = embsvc.EmbeddingService
