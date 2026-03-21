package chat

import (
	memsvc "slimebot/internal/services/memory"
	oaisvc "slimebot/internal/services/openai"
	skillsvc "slimebot/internal/services/skill"
)

type OpenAIClient = oaisvc.OpenAIClient
type ModelRuntimeConfig = oaisvc.ModelRuntimeConfig
type ChatMessage = oaisvc.ChatMessage
type ChatMessageContentPart = oaisvc.ChatMessageContentPart
type ChatMessageContentPartType = oaisvc.ChatMessageContentPartType
type ToolCallInfo = oaisvc.ToolCallInfo
type ToolDef = oaisvc.ToolDef
type StreamResultType = oaisvc.StreamResultType

const (
	ChatMessageContentPartTypeText  = oaisvc.ChatMessageContentPartTypeText
	ChatMessageContentPartTypeImage = oaisvc.ChatMessageContentPartTypeImage
	ChatMessageContentPartTypeAudio = oaisvc.ChatMessageContentPartTypeAudio
	ChatMessageContentPartTypeFile  = oaisvc.ChatMessageContentPartTypeFile
)

type MemoryService = memsvc.MemoryService
type SkillRuntimeService = skillsvc.SkillRuntimeService

var NewMemoryService = memsvc.NewMemoryService
