package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	llmsvc "slimebot/internal/services/llm"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const defaultMaxTokens = 4096

// AnthropicClient 封装 Anthropic API 的 HTTP 客户端，实现 llmsvc.Provider 接口。
type AnthropicClient struct {
	Client *http.Client
}

// NewAnthropicClient 创建 Anthropic 客户端实例。
func NewAnthropicClient() *AnthropicClient {
	return &AnthropicClient{
		Client: &http.Client{Timeout: 90 * time.Second},
	}
}

// StreamChatWithTools 发起支持 tool use 的流式聊天请求，实现 llmsvc.Provider 接口。
func (c *AnthropicClient) StreamChatWithTools(
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

	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(c.Client),
	}
	client := anthropic.NewClient(opts...)

	// 提取 system 消息并构建 Anthropic 消息列表。
	systemBlocks, apiMessages := buildAnthropicMessages(messages)

	// Anthropic Temperature 范围 [0.0, 1.0]，需要钳位。
	temperature := modelConfig.Temperature
	if temperature > 1.0 {
		temperature = 1.0
	}

	params := anthropic.MessageNewParams{
		MaxTokens:   defaultMaxTokens,
		Model:       anthropic.Model(model),
		Messages:    apiMessages,
		Temperature: anthropic.Float(temperature),
	}
	if len(systemBlocks) > 0 {
		params.System = systemBlocks
	}
	if len(toolDefs) > 0 {
		params.Tools = buildAnthropicTools(toolDefs)
	}

	stream := client.Messages.NewStreaming(ctx, params)

	// 流式累积状态
	var (
		textBuilder       strings.Builder
		toolUseBlocks     []pendingToolUse
		currentToolUseIdx = -1
	)

	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "content_block_start":
			if event.ContentBlock.Type == "tool_use" {
				toolUseBlocks = append(toolUseBlocks, pendingToolUse{
					ID:   event.ContentBlock.ID,
					Name: event.ContentBlock.Name,
				})
				currentToolUseIdx = len(toolUseBlocks) - 1
			} else {
				currentToolUseIdx = -1
			}

		case "content_block_delta":
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				textBuilder.WriteString(event.Delta.Text)
				if err := onChunk(event.Delta.Text); err != nil {
					return nil, err
				}
			}
			if event.Delta.Type == "input_json_delta" && currentToolUseIdx >= 0 {
				toolUseBlocks[currentToolUseIdx].InputJSON += event.Delta.PartialJSON
			}

		case "content_block_stop":
			currentToolUseIdx = -1
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("Model request failed: %w", err)
	}

	// 如果有 tool_use 块，返回工具调用结果
	if len(toolUseBlocks) > 0 {
		var calls []llmsvc.ToolCallInfo
		contentBlocks := make([]anthropic.ContentBlockParamUnion, 0, len(toolUseBlocks)+1)
		text := strings.TrimSpace(textBuilder.String())
		if text != "" {
			contentBlocks = append(contentBlocks, anthropic.NewTextBlock(text))
		}
		for _, tu := range toolUseBlocks {
			inputJSON := normalizeInputJSON(tu.InputJSON)
			calls = append(calls, llmsvc.ToolCallInfo{
				ID:        tu.ID,
				Name:      tu.Name,
				Arguments: inputJSON,
			})
			contentBlocks = append(contentBlocks, anthropic.ContentBlockParamUnion{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    tu.ID,
					Name:  tu.Name,
					Input: json.RawMessage(inputJSON),
				},
			})
		}
		// 构造 assistant 消息供上下文追加
		assistantMsg := llmsvc.ChatMessage{
			Role:      "assistant",
			Content:   text,
			ToolCalls: calls,
		}
		return &llmsvc.StreamResult{
			Type:             llmsvc.StreamResultToolCalls,
			ToolCalls:        calls,
			AssistantMessage: assistantMsg,
		}, nil
	}

	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
}

// pendingToolUse 累积流式 tool_use 事件中的参数。
type pendingToolUse struct {
	ID        string
	Name      string
	InputJSON string
}

// normalizeInputJSON 确保累积的 JSON 片段是合法 JSON。
func normalizeInputJSON(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "{}"
	}
	if !json.Valid([]byte(s)) {
		return "{}"
	}
	return s
}
