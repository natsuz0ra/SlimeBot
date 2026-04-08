package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	llmsvc "slimebot/internal/services/llm"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// OpenAIClient 封装 OpenAI 兼容 API 的 HTTP 客户端，实现 llmsvc.Provider 接口。
type OpenAIClient struct {
	Client *http.Client
}

func NewOpenAIClient() *OpenAIClient {
	return &OpenAIClient{
		Client: &http.Client{Timeout: 90 * time.Second},
	}
}

// StreamChat 发起流式聊天请求（不带工具），保持原有调用兼容。
func (c *OpenAIClient) StreamChat(ctx context.Context, modelConfig llmsvc.ModelRuntimeConfig, messages []llmsvc.ChatMessage, onChunk func(string) error) error {
	result, err := c.StreamChatWithTools(ctx, modelConfig, messages, nil, onChunk)
	if err != nil {
		return err
	}
	if result.Type == llmsvc.StreamResultToolCalls {
		return fmt.Errorf("The model unexpectedly returned tool calls while tools are disabled.")
	}
	return nil
}

// StreamChatWithTools 发起支持 function call 的流式聊天请求，实现 llmsvc.Provider 接口。
func (c *OpenAIClient) StreamChatWithTools(
	ctx context.Context,
	modelConfig llmsvc.ModelRuntimeConfig,
	messages []llmsvc.ChatMessage,
	toolDefs []llmsvc.ToolDef,
	onChunk func(string) error,
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
		Messages:    requestMessages,
		Model:       openai.ChatModel(model),
		Temperature: openai.Float(modelConfig.Temperature),
	}

	if len(toolDefs) > 0 {
		params.Tools = buildToolParams(toolDefs)
	}

	stream := client.Chat.Completions.NewStreaming(ctx, params)
	acc := openai.ChatCompletionAccumulator{}

	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		if len(chunk.Choices) > 0 {
			content := chunk.Choices[0].Delta.Content
			if content != "" {
				if err := onChunk(content); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("Model request failed: %w", err)
	}

	if len(acc.Choices) == 0 {
		return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
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
			Type:      llmsvc.StreamResultToolCalls,
			ToolCalls: calls,
			AssistantMessage: llmsvc.ChatMessage{
				Role:      "assistant",
				Content:   choice.Message.Content,
				ToolCalls: calls,
			},
		}, nil
	}

	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
}

// supportsDeveloperRole 部分兼容端（如阿里云）不支持 developer role，需降级为 system。
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

// buildRequestMessages 将内部 ChatMessage 转为 SDK 消息。
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
				result = append(result, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						Content:   openai.ChatCompletionAssistantMessageParamContentUnion{OfString: openai.String(content)},
						ToolCalls: toolCalls,
					},
				})
			} else {
				if content == "" {
					continue
				}
				result = append(result, openai.AssistantMessage(content))
			}
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

// buildRequestUserContentParts 将多模态 ContentParts 转为 OpenAI 图文/音频/文件等内容块列表。
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
