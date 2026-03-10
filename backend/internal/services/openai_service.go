package services

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type OpenAIClient struct {
	Client *http.Client
}

type ModelRuntimeConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

type ChatMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"toolCallId,omitempty"`
	// ToolCalls 仅在 role=assistant 且模型返回了工具调用时使用
	ToolCalls []ToolCallInfo `json:"toolCalls,omitempty"`
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

const (
	StreamResultText      StreamResultType = iota // 纯文本回复
	StreamResultToolCalls                         // 工具调用请求
)

// StreamResult 包含一次流式调用的结果
type StreamResult struct {
	Type      StreamResultType
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
	result, err := c.StreamChatWithTools(ctx, modelConfig, messages, nil, onChunk)
	if err != nil {
		return err
	}
	if result.Type == StreamResultToolCalls {
		return fmt.Errorf("模型意外返回了工具调用，但当前未启用工具")
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
	baseURL := strings.TrimRight(strings.TrimSpace(modelConfig.BaseURL), "/")
	apiKey := strings.TrimSpace(modelConfig.APIKey)
	model := strings.TrimSpace(modelConfig.Model)
	if baseURL == "" || apiKey == "" || model == "" {
		return nil, fmt.Errorf("模型配置缺失 baseUrl/apiKey/model")
	}

	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(c.Client),
	)

	requestMessages := buildRequestMessages(messages)
	if len(requestMessages) == 0 {
		return nil, fmt.Errorf("请求消息为空")
	}

	params := openai.ChatCompletionNewParams{
		Messages: requestMessages,
		Model:    openai.ChatModel(model),
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
		return nil, fmt.Errorf("模型请求失败: %w", err)
	}

	if len(acc.Choices) == 0 {
		return &StreamResult{Type: StreamResultText}, nil
	}

	choice := acc.Choices[0]
	if len(choice.Message.ToolCalls) > 0 {
		var calls []ToolCallInfo
		for _, tc := range choice.Message.ToolCalls {
			calls = append(calls, ToolCallInfo{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		return &StreamResult{
			Type:      StreamResultToolCalls,
			ToolCalls: calls,
			AssistantMessage: ChatMessage{
				Role:      "assistant",
				Content:   choice.Message.Content,
				ToolCalls: calls,
			},
		}, nil
	}

	return &StreamResult{Type: StreamResultText}, nil
}

func buildRequestMessages(messages []ChatMessage) []openai.ChatCompletionMessageParamUnion {
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
			result = append(result, openai.DeveloperMessage(content))
		default:
			if content == "" {
				continue
			}
			result = append(result, openai.UserMessage(content))
		}
	}
	return result
}

func buildToolParams(defs []ToolDef) []openai.ChatCompletionToolUnionParam {
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
