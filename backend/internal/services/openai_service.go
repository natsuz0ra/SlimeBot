package services

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"slimebot/backend/internal/consts"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type OpenAIClient struct {
	// 复用的 HTTP 客户端，统一控制超时与连接行为。
	Client *http.Client
}

type ModelRuntimeConfig struct {
	// 兼容 OpenAI 协议的服务地址。
	BaseURL string
	// 鉴权密钥。
	APIKey string
	// 目标模型名称。
	Model string
}

type ChatMessage struct {
	Role         string                   `json:"role"`
	Content      string                   `json:"content"`
	ContentParts []ChatMessageContentPart `json:"contentParts,omitempty"`
	ToolCallID   string                   `json:"toolCallId,omitempty"`
	// ToolCalls 仅在 role=assistant 且模型返回了工具调用时使用
	ToolCalls []ToolCallInfo `json:"toolCalls,omitempty"`
}

type ChatMessageContentPartType string

const (
	ChatMessageContentPartTypeText  ChatMessageContentPartType = "text"
	ChatMessageContentPartTypeImage ChatMessageContentPartType = "image"
	ChatMessageContentPartTypeAudio ChatMessageContentPartType = "audio"
	ChatMessageContentPartTypeFile  ChatMessageContentPartType = "file"
)

type ChatMessageContentPart struct {
	Type             ChatMessageContentPartType `json:"type"`
	Text             string                     `json:"text,omitempty"`
	ImageURL         string                     `json:"imageUrl,omitempty"`
	ImageDetail      string                     `json:"imageDetail,omitempty"`
	InputAudioData   string                     `json:"inputAudioData,omitempty"`
	InputAudioFormat string                     `json:"inputAudioFormat,omitempty"`
	FileDataBase64   string                     `json:"fileDataBase64,omitempty"`
	Filename         string                     `json:"filename,omitempty"`
}

// ToolCallInfo 描述一次工具调用请求
type ToolCallInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDef 用于传入 OpenAI API 的工具定义
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// StreamResultType 区分流式结果类型
type StreamResultType int

// StreamResult 包含一次流式调用的结果
type StreamResult struct {
	// 本次流式结果类型：文本或工具调用。
	Type StreamResultType
	// 模型返回的工具调用列表（仅 Type=StreamResultToolCalls 时有值）。
	ToolCalls []ToolCallInfo
	// AssistantMessage 用于将 assistant 消息（含 tool_calls）追加回上下文
	AssistantMessage ChatMessage
}

func NewOpenAIClient() *OpenAIClient {
	return &OpenAIClient{
		Client: &http.Client{Timeout: 90 * time.Second},
	}
}

// StreamChat 发起流式聊天请求（不带工具），保持原有调用兼容
func (c *OpenAIClient) StreamChat(ctx context.Context, modelConfig ModelRuntimeConfig, messages []ChatMessage, onChunk func(string) error) error {
	// 复用带工具版本，传入空工具定义以保持单一实现入口。
	result, err := c.StreamChatWithTools(ctx, modelConfig, messages, nil, onChunk)
	if err != nil {
		return err
	}
	if result.Type == StreamResultType(consts.StreamResultToolCalls) {
		return fmt.Errorf("The model unexpectedly returned tool calls while tools are disabled.")
	}
	return nil
}

// StreamChatWithTools 发起支持 function call 的流式聊天请求。
// 如果模型返回纯文本，通过 onChunk 推送并返回 StreamResultText；
// 如果模型返回 tool_calls，累积完成后返回 StreamResultToolCalls。
func (c *OpenAIClient) StreamChatWithTools(
	ctx context.Context,
	modelConfig ModelRuntimeConfig,
	messages []ChatMessage,
	toolDefs []ToolDef,
	onChunk func(string) error,
) (*StreamResult, error) {
	// 统一净化配置，避免尾部斜杠和空白导致请求失败。
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

	// 组装 OpenAI 聊天参数；仅在有工具定义时启用 tools。
	params := openai.ChatCompletionNewParams{
		Messages: requestMessages,
		Model:    openai.ChatModel(model),
	}

	if len(toolDefs) > 0 {
		params.Tools = buildToolParams(toolDefs)
	}

	stream := client.Chat.Completions.NewStreaming(ctx, params)
	acc := openai.ChatCompletionAccumulator{}

	// 流式消费：边累积完整结果，边把文本增量回调给上层。
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

	// 无选择分支时按空文本处理，保持返回结构稳定。
	if len(acc.Choices) == 0 {
		return &StreamResult{Type: StreamResultType(consts.StreamResultText)}, nil
	}

	choice := acc.Choices[0]
	if len(choice.Message.ToolCalls) > 0 {
		// 将 SDK 的 tool_calls 结构收敛为内部统一格式。
		var calls []ToolCallInfo
		for _, tc := range choice.Message.ToolCalls {
			calls = append(calls, ToolCallInfo{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		return &StreamResult{
			Type:      StreamResultType(consts.StreamResultToolCalls),
			ToolCalls: calls,
			// 保留完整 assistant 消息，供上层追加回上下文继续推理。
			AssistantMessage: ChatMessage{
				Role:      "assistant",
				Content:   choice.Message.Content,
				ToolCalls: calls,
			},
		}, nil
	}

	return &StreamResult{Type: StreamResultType(consts.StreamResultText)}, nil
}

func supportsDeveloperRole(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		// URL 解析失败时保持原行为，避免误伤其他兼容供应商。
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

func buildRequestMessages(messages []ChatMessage, supportDeveloperRole bool) []openai.ChatCompletionMessageParamUnion {
	var result []openai.ChatCompletionMessageParamUnion
	for _, msg := range messages {
		content := strings.TrimSpace(msg.Content)

		// 按内部 role 映射到 OpenAI SDK 对应消息类型。
		switch strings.ToLower(strings.TrimSpace(msg.Role)) {
		case "system":
			if content == "" {
				continue
			}
			result = append(result, openai.SystemMessage(content))
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// assistant 带 tool_calls 时必须使用结构化消息，保留调用 ID 与参数。
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
			// tool 消息通过 toolCallID 与上一个 assistant 的调用请求关联。
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		case "developer":
			if content == "" {
				continue
			}
			if supportDeveloperRole {
				result = append(result, openai.DeveloperMessage(content))
			} else {
				// 对不支持 developer 的兼容端点降级为 system，保留指令语义。
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

func buildRequestUserContentParts(parts []ChatMessageContentPart) []openai.ChatCompletionContentPartUnionParam {
	result := make([]openai.ChatCompletionContentPartUnionParam, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case ChatMessageContentPartTypeText:
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			result = append(result, openai.TextContentPart(text))
		case ChatMessageContentPartTypeImage:
			imageURL := strings.TrimSpace(part.ImageURL)
			if imageURL == "" {
				continue
			}
			image := openai.ChatCompletionContentPartImageImageURLParam{URL: imageURL}
			if detail := strings.TrimSpace(part.ImageDetail); detail != "" {
				image.Detail = detail
			}
			result = append(result, openai.ImageContentPart(image))
		case ChatMessageContentPartTypeAudio:
			data := strings.TrimSpace(part.InputAudioData)
			format := strings.TrimSpace(part.InputAudioFormat)
			if data == "" || format == "" {
				continue
			}
			result = append(result, openai.InputAudioContentPart(openai.ChatCompletionContentPartInputAudioInputAudioParam{
				Data:   data,
				Format: format,
			}))
		case ChatMessageContentPartTypeFile:
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

func buildToolParams(defs []ToolDef) []openai.ChatCompletionToolUnionParam {
	var tools []openai.ChatCompletionToolUnionParam
	for _, def := range defs {
		// 将内部工具定义转换为 OpenAI function tool 参数。
		tools = append(tools, openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        def.Name,
			Description: openai.String(def.Description),
			Parameters:  openai.FunctionParameters(def.Parameters),
		}))
	}
	return tools
}
