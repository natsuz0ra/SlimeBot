package memory

import (
	embsvc "slimebot/backend/internal/services/embedding"
	oaisvc "slimebot/backend/internal/services/openai"
)

type OpenAIClient = oaisvc.OpenAIClient
type ModelRuntimeConfig = oaisvc.ModelRuntimeConfig
type ChatMessage = oaisvc.ChatMessage
type EmbeddingService = embsvc.EmbeddingService
