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

// AnthropicClient wraps the Anthropic HTTP API and implements llmsvc.Provider.
type AnthropicClient struct {
	Client *http.Client
}

// NewAnthropicClient constructs a client with default HTTP timeouts.
func NewAnthropicClient() *AnthropicClient {
	return &AnthropicClient{
		Client: &http.Client{Timeout: 90 * time.Second},
	}
}

// StreamChatWithTools starts a streaming chat request with tool use; implements llmsvc.Provider.
func (c *AnthropicClient) StreamChatWithTools(
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

	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(c.Client),
	}
	client := anthropic.NewClient(opts...)

	// Extract system messages and build the Anthropic message list.
	systemBlocks, apiMessages := buildAnthropicMessages(messages)

	// Anthropic temperature is in [0.0, 1.0]; clamp to range.
	temperature := modelConfig.Temperature
	if temperature > 1.0 {
		temperature = 1.0
	}

	budget := llmsvc.ThinkingBudgetTokens(modelConfig.ThinkingLevel)
	maxTokens := int64(defaultMaxTokens)
	if budget > 0 && maxTokens < int64(budget)+1 {
		maxTokens = int64(budget) + 1
	}

	params := anthropic.MessageNewParams{
		MaxTokens: maxTokens,
		Model:     anthropic.Model(model),
		Messages:  apiMessages,
		//Temperature: anthropic.Float(temperature),
	}
	if budget > 0 {
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(budget))
	}
	if len(systemBlocks) > 0 {
		params.System = systemBlocks
	}
	if len(toolDefs) > 0 {
		params.Tools = buildAnthropicTools(toolDefs)
	}

	stream := client.Messages.NewStreaming(ctx, params)

	// Streaming accumulation state
	var (
		textBuilder       strings.Builder
		thinkingBlocks    []llmsvc.ThinkingBlockInfo
		thinkingBuilder   strings.Builder
		thinkingSignature strings.Builder
		redactedThinking  string
		toolUseBlocks     []pendingToolUse
		currentToolUseIdx = -1
		inThinkingBlock   = false
	)

	finishThinkingBlock := func() {
		if !inThinkingBlock {
			return
		}
		thinking := thinkingBuilder.String()
		signature := thinkingSignature.String()
		if redactedThinking != "" || thinking != "" || signature != "" {
			thinkingBlocks = append(thinkingBlocks, llmsvc.ThinkingBlockInfo{
				Thinking:     thinking,
				Signature:    signature,
				RedactedData: redactedThinking,
			})
		}
		thinkingBuilder.Reset()
		thinkingSignature.Reset()
		redactedThinking = ""
		inThinkingBlock = false
	}

	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "content_block_start":
			finishThinkingBlock()
			if event.ContentBlock.Type == "thinking" || event.ContentBlock.Type == "redacted_thinking" {
				inThinkingBlock = true
				currentToolUseIdx = -1
				if event.ContentBlock.Type == "thinking" {
					thinkingBuilder.WriteString(firstNonEmpty(
						event.ContentBlock.Thinking,
						extractRawStringField(event.ContentBlock.RawJSON(), "reasoning_content", "reasoning"),
					))
					thinkingSignature.WriteString(event.ContentBlock.Signature)
				} else {
					redactedThinking = event.ContentBlock.Data
				}
			} else if event.ContentBlock.Type == "tool_use" {
				toolUseBlocks = append(toolUseBlocks, pendingToolUse{
					ID:   event.ContentBlock.ID,
					Name: event.ContentBlock.Name,
				})
				currentToolUseIdx = len(toolUseBlocks) - 1
				inThinkingBlock = false
			} else {
				currentToolUseIdx = -1
				inThinkingBlock = false
			}

		case "content_block_delta":
			if inThinkingBlock && event.Delta.Type == "thinking_delta" {
				thinkingDelta := firstNonEmpty(
					event.Delta.Thinking,
					extractNestedRawStringField(event.RawJSON(), "delta", "reasoning_content", "reasoning"),
				)
				if thinkingDelta != "" {
					thinkingBuilder.WriteString(thinkingDelta)
					if callbacks.OnThinkingChunk != nil {
						if err := callbacks.OnThinkingChunk(thinkingDelta); err != nil {
							return nil, err
						}
					}
				}
			}
			if inThinkingBlock && event.Delta.Type == "signature_delta" && event.Delta.Signature != "" {
				thinkingSignature.WriteString(event.Delta.Signature)
			}
			if !inThinkingBlock && event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				textBuilder.WriteString(event.Delta.Text)
				if err := callbacks.OnChunk(event.Delta.Text); err != nil {
					return nil, err
				}
			}
			if event.Delta.Type == "input_json_delta" && currentToolUseIdx >= 0 {
				toolUseBlocks[currentToolUseIdx].InputJSON += event.Delta.PartialJSON
			}

		case "content_block_stop":
			finishThinkingBlock()
			currentToolUseIdx = -1
		}
	}
	finishThinkingBlock()
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("Model request failed: %w", err)
	}

	// If there are tool_use blocks, return tool call results
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
		// Build assistant message for downstream context
		assistantMsg := llmsvc.ChatMessage{
			Role:           "assistant",
			Content:        text,
			ThinkingBlocks: thinkingBlocks,
			ToolCalls:      calls,
		}
		return &llmsvc.StreamResult{
			Type:             llmsvc.StreamResultToolCalls,
			ToolCalls:        calls,
			AssistantMessage: assistantMsg,
		}, nil
	}

	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
}

// pendingToolUse accumulates parameters from streaming tool_use events.
type pendingToolUse struct {
	ID        string
	Name      string
	InputJSON string
}

// normalizeInputJSON ensures accumulated JSON fragments form valid JSON.
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func extractRawStringField(raw string, keys ...string) string {
	if raw == "" {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return ""
	}
	for _, key := range keys {
		if value, ok := data[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func extractNestedRawStringField(raw string, objectKey string, keys ...string) string {
	if raw == "" {
		return ""
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return ""
	}
	nested, ok := data[objectKey]
	if !ok {
		return ""
	}
	return extractRawStringField(string(nested), keys...)
}
