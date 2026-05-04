package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	llmsvc "slimebot/internal/services/llm"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// OpenAIClient wraps an OpenAI-compatible HTTP API client and implements llmsvc.Provider.
type OpenAIClient struct {
	Client *http.Client
}

func NewOpenAIClient() *OpenAIClient {
	return &OpenAIClient{
		Client: &http.Client{Timeout: 90 * time.Second},
	}
}

// StreamChat starts a streaming chat request without tools (legacy compatibility).
func (c *OpenAIClient) StreamChat(ctx context.Context, modelConfig llmsvc.ModelRuntimeConfig, messages []llmsvc.ChatMessage, onChunk func(string) error) error {
	result, err := c.StreamChatWithTools(ctx, modelConfig, messages, nil, llmsvc.StreamCallbacks{OnChunk: onChunk})
	if err != nil {
		return err
	}
	if result.Type == llmsvc.StreamResultToolCalls {
		return fmt.Errorf("The model unexpectedly returned tool calls while tools are disabled.")
	}
	return nil
}

// StreamChatWithTools starts a streaming chat with function calling; implements llmsvc.Provider.
func (c *OpenAIClient) StreamChatWithTools(
	ctx context.Context,
	modelConfig llmsvc.ModelRuntimeConfig,
	messages []llmsvc.ChatMessage,
	toolDefs []llmsvc.ToolDef,
	callbacks llmsvc.StreamCallbacks,
) (*llmsvc.StreamResult, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(modelConfig.BaseURL), "/")
	apiKey := strings.TrimSpace(modelConfig.APIKey)
	model := strings.TrimSpace(modelConfig.Model)
	if baseURL == "" || apiKey == "" || model == "" {
		return nil, fmt.Errorf("Model config is missing baseUrl, apiKey, or model.")
	}

	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(c.Client),
	)

	supportDeveloperRole := supportsDeveloperRole(baseURL)
	requestMessages := buildRequestMessages(messages, supportDeveloperRole)
	if len(requestMessages) == 0 {
		return nil, fmt.Errorf("Request messages are empty.")
	}

	params := openai.ChatCompletionNewParams{
		Messages: requestMessages,
		Model:    openai.ChatModel(model),
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		},
		//Temperature: openai.Float(modelConfig.Temperature),
	}

	applyThinkingParams(&params, modelConfig)

	if len(toolDefs) > 0 {
		params.Tools = buildToolParams(toolDefs)
	}

	result, sawStreamEvent, err := streamChatCompletion(ctx, client, params, callbacks)
	if err != nil && !sawStreamEvent && isStreamUsageUnsupported(err) {
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{}
		result, _, err = streamChatCompletion(ctx, client, params, callbacks)
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func streamChatCompletion(ctx context.Context, client openai.Client, params openai.ChatCompletionNewParams, callbacks llmsvc.StreamCallbacks) (*llmsvc.StreamResult, bool, error) {
	stream := client.Chat.Completions.NewStreaming(ctx, params)
	acc := openai.ChatCompletionAccumulator{}
	var reasoningBuf strings.Builder
	var tokenUsage llmsvc.TokenUsage
	sawStreamEvent := false

	for stream.Next() {
		sawStreamEvent = true
		chunk := stream.Current()
		acc.AddChunk(chunk)
		if chunk.Usage.TotalTokens > 0 || chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			tokenUsage = tokenUsageFromOpenAIChunkUsage(
				chunk.Usage.PromptTokens,
				chunk.Usage.PromptTokensDetails.CachedTokens,
				chunk.Usage.CompletionTokens,
				chunk.Usage.TotalTokens,
			)
		}

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta

			// Handle reasoning_content from Volcengine/DeepSeek/Zhipu compatible APIs.
			if reasoning := extractReasoningContent(delta); reasoning != "" {
				reasoningBuf.WriteString(reasoning)
				if callbacks.OnThinkingChunk != nil {
					if err := callbacks.OnThinkingChunk(reasoning); err != nil {
						return nil, sawStreamEvent, err
					}
				}
			}

			if delta.Content != "" {
				if err := callbacks.OnChunk(delta.Content); err != nil {
					return nil, sawStreamEvent, err
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, sawStreamEvent, fmt.Errorf("Model request failed: %w", err)
	}

	if len(acc.Choices) == 0 {
		return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, sawStreamEvent, nil
	}

	choice := acc.Choices[0]
	if len(choice.Message.ToolCalls) > 0 {
		var calls []llmsvc.ToolCallInfo
		for _, tc := range choice.Message.ToolCalls {
			calls = append(calls, llmsvc.ToolCallInfo{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		return &llmsvc.StreamResult{
			Type:       llmsvc.StreamResultToolCalls,
			TokenUsage: nonZeroUsage(tokenUsage),
			ToolCalls:  calls,
			AssistantMessage: llmsvc.ChatMessage{
				Role:             "assistant",
				Content:          choice.Message.Content,
				ToolCalls:        calls,
				ReasoningContent: reasoningBuf.String(),
			},
		}, sawStreamEvent, nil
	}

	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText, TokenUsage: nonZeroUsage(tokenUsage)}, sawStreamEvent, nil
}

func tokenUsageFromOpenAIChunkUsage(promptTokens, cachedTokens, completionTokens, totalTokens int64) llmsvc.TokenUsage {
	return llmsvc.TokenUsage{
		InputTokens:          int(promptTokens),
		OutputTokens:         int(completionTokens),
		CacheReadInputTokens: int(cachedTokens),
		TotalTokens:          int(totalTokens),
	}
}

func nonZeroUsage(usage llmsvc.TokenUsage) *llmsvc.TokenUsage {
	if usage.IsZero() {
		return nil
	}
	return &usage
}

func isStreamUsageUnsupported(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "stream_options") || strings.Contains(msg, "include_usage")
}

func applyThinkingParams(params *openai.ChatCompletionNewParams, modelConfig llmsvc.ModelRuntimeConfig) {
	if params == nil {
		return
	}
	if strings.EqualFold(strings.TrimSpace(modelConfig.Provider), llmsvc.ProviderDeepSeek) {
		thinking := map[string]any{"type": "disabled"}
		if effort := llmsvc.DeepSeekReasoningEffort(modelConfig.ThinkingLevel); effort != "" {
			thinking["type"] = "enabled"
			params.ReasoningEffort = shared.ReasoningEffort(effort)
		}
		params.SetExtraFields(map[string]any{"thinking": thinking})
		return
	}
	if effort := llmsvc.ThinkingReasoningEffort(modelConfig.ThinkingLevel); effort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(effort)
	}
}

// extractReasoningContent extracts reasoning_content from a streaming delta.
// Volcengine, DeepSeek, and Zhipu return thinking content via this non-standard field.
// The openai-go SDK marks ExtraFields as status=invalid for unknown fields, so
// we cannot use f.Valid() — we check f.Raw() instead.
func extractReasoningContent(delta openai.ChatCompletionChunkChoiceDelta) string {
	f, ok := delta.JSON.ExtraFields["reasoning_content"]
	if !ok {
		return ""
	}
	raw := f.Raw()
	if raw == "" || raw == "null" {
		return ""
	}
	var result string
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return ""
	}
	return result
}

// supportsDeveloperRole: some compatible endpoints (e.g. Alibaba Cloud) omit developer role; fall back to system.
func supportsDeveloperRole(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return true
	}

	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(strings.TrimSpace(parsed.Path))
	if strings.Contains(host, "dashscope.aliyuncs.com") {
		return false
	}
	if strings.Contains(host, "aliyuncs.com") && strings.Contains(path, "/compatible-mode/") {
		return false
	}
	return true
}

// buildRequestMessages converts internal ChatMessages to SDK message params.
func buildRequestMessages(messages []llmsvc.ChatMessage, supportDeveloperRole bool) []openai.ChatCompletionMessageParamUnion {
	var result []openai.ChatCompletionMessageParamUnion
	for _, msg := range messages {
		content := strings.TrimSpace(msg.Content)

		switch strings.ToLower(strings.TrimSpace(msg.Role)) {
		case "system":
			if content == "" {
				continue
			}
			result = append(result, openai.SystemMessage(content))
		case "assistant":
			ap := buildAssistantMessageParam(msg, content)
			if ap == nil {
				continue
			}
			result = append(result, openai.ChatCompletionMessageParamUnion{OfAssistant: ap})
		case "tool":
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		case "developer":
			if content == "" {
				continue
			}
			if supportDeveloperRole {
				result = append(result, openai.DeveloperMessage(content))
			} else {
				result = append(result, openai.SystemMessage(content))
			}
		case "user":
			if len(msg.ContentParts) > 0 {
				userParts := buildRequestUserContentParts(msg.ContentParts)
				if len(userParts) > 0 {
					result = append(result, openai.UserMessage(userParts))
					continue
				}
			}
			if content == "" {
				continue
			}
			result = append(result, openai.UserMessage(content))
		default:
			if content == "" {
				continue
			}
			result = append(result, openai.UserMessage(content))
		}
	}
	return result
}

// buildAssistantMessageParam builds the SDK assistant message param, including
// reasoning_content via SetExtraFields when present (required by DeepSeek).
func buildAssistantMessageParam(msg llmsvc.ChatMessage, content string) *openai.ChatCompletionAssistantMessageParam {
	if len(msg.ToolCalls) > 0 {
		var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
		for _, tc := range msg.ToolCalls {
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: tc.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				},
			})
		}
		ap := &openai.ChatCompletionAssistantMessageParam{
			Content:   openai.ChatCompletionAssistantMessageParamContentUnion{OfString: openai.String(content)},
			ToolCalls: toolCalls,
		}
		if msg.ReasoningContent != "" {
			ap.SetExtraFields(map[string]any{"reasoning_content": msg.ReasoningContent})
		}
		return ap
	}

	if content == "" && msg.ReasoningContent == "" {
		return nil
	}
	ap := &openai.ChatCompletionAssistantMessageParam{
		Content: openai.ChatCompletionAssistantMessageParamContentUnion{OfString: openai.String(content)},
	}
	if msg.ReasoningContent != "" {
		ap.SetExtraFields(map[string]any{"reasoning_content": msg.ReasoningContent})
	}
	return ap
}

// buildRequestUserContentParts converts multimodal ContentParts to OpenAI user content parts (image/audio/file blocks).
func buildRequestUserContentParts(parts []llmsvc.ChatMessageContentPart) []openai.ChatCompletionContentPartUnionParam {
	result := make([]openai.ChatCompletionContentPartUnionParam, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case llmsvc.ChatMessageContentPartTypeText:
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			result = append(result, openai.TextContentPart(text))
		case llmsvc.ChatMessageContentPartTypeImage:
			imageURL := strings.TrimSpace(part.ImageURL)
			if imageURL == "" {
				continue
			}
			image := openai.ChatCompletionContentPartImageImageURLParam{URL: imageURL}
			if detail := strings.TrimSpace(part.ImageDetail); detail != "" {
				image.Detail = detail
			}
			result = append(result, openai.ImageContentPart(image))
		case llmsvc.ChatMessageContentPartTypeAudio:
			data := strings.TrimSpace(part.InputAudioData)
			format := strings.TrimSpace(part.InputAudioFormat)
			if data == "" || format == "" {
				continue
			}
			result = append(result, openai.InputAudioContentPart(openai.ChatCompletionContentPartInputAudioInputAudioParam{
				Data:   data,
				Format: format,
			}))
		case llmsvc.ChatMessageContentPartTypeFile:
			fileData := strings.TrimSpace(part.FileDataBase64)
			if fileData == "" {
				continue
			}
			file := openai.ChatCompletionContentPartFileFileParam{
				FileData: openai.String(fileData),
			}
			if filename := strings.TrimSpace(part.Filename); filename != "" {
				file.Filename = openai.String(filename)
			}
			result = append(result, openai.FileContentPart(file))
		}
	}
	return result
}

func buildToolParams(defs []llmsvc.ToolDef) []openai.ChatCompletionToolUnionParam {
	var tools []openai.ChatCompletionToolUnionParam
	for _, def := range defs {
		tools = append(tools, openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        def.Name,
			Description: openai.String(def.Description),
			Parameters:  openai.FunctionParameters(def.Parameters),
		}))
	}
	return tools
}
