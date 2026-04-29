package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"

	llmsvc "slimebot/internal/services/llm"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

// buildAnthropicMessages converts ChatMessage slices to Anthropic system blocks and messages.
func buildAnthropicMessages(messages []llmsvc.ChatMessage) ([]anthropic.TextBlockParam, []anthropic.MessageParam) {
	var systemBlocks []anthropic.TextBlockParam
	var apiMessages []anthropic.MessageParam

	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		content := strings.TrimSpace(msg.Content)

		switch role {
		case "system", "developer":
			if content != "" {
				systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: content})
			}

		case "user":
			if len(msg.ContentParts) > 0 {
				parts := buildContentParts(msg.ContentParts)
				if len(parts) > 0 {
					apiMessages = append(apiMessages, anthropic.NewUserMessage(parts...))
					continue
				}
			}
			if content == "" {
				continue
			}
			apiMessages = append(apiMessages, anthropic.NewUserMessage(
				anthropic.NewTextBlock(content),
			))

		case "assistant":
			blocks := buildAssistantBlocks(msg)
			if len(blocks) > 0 {
				apiMessages = append(apiMessages, anthropic.NewAssistantMessage(blocks...))
			} else if content != "" {
				apiMessages = append(apiMessages, anthropic.NewAssistantMessage(
					anthropic.NewTextBlock(content),
				))
			}

		case "tool":
			toolResult := anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content, false)
			if len(apiMessages) > 0 && apiMessages[len(apiMessages)-1].Role == "user" {
				lastMsg := &apiMessages[len(apiMessages)-1]
				lastMsg.Content = append(lastMsg.Content, toolResult)
			} else {
				apiMessages = append(apiMessages, anthropic.NewUserMessage(toolResult))
			}
		}
	}

	return systemBlocks, apiMessages
}

// buildAssistantBlocks builds assistant message content blocks (text + tool_use).
func buildAssistantBlocks(msg llmsvc.ChatMessage) []anthropic.ContentBlockParamUnion {
	var blocks []anthropic.ContentBlockParamUnion

	for _, tb := range msg.ThinkingBlocks {
		if strings.TrimSpace(tb.RedactedData) != "" {
			blocks = append(blocks, anthropic.NewRedactedThinkingBlock(tb.RedactedData))
			continue
		}
		if strings.TrimSpace(tb.Thinking) != "" {
			blocks = append(blocks, newThinkingBlock(tb.Signature, tb.Thinking))
		}
	}

	if strings.TrimSpace(msg.Content) != "" {
		blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
	}

	for _, tc := range msg.ToolCalls {
		var input any
		raw := strings.TrimSpace(tc.Arguments)
		if raw != "" && json.Valid([]byte(raw)) {
			input = json.RawMessage(raw)
		} else {
			input = map[string]any{}
		}
		blocks = append(blocks, anthropic.ContentBlockParamUnion{
			OfToolUse: &anthropic.ToolUseBlockParam{
				ID:    tc.ID,
				Name:  tc.Name,
				Input: input,
			},
		})
	}

	return blocks
}

func newThinkingBlock(signature string, thinking string) anthropic.ContentBlockParamUnion {
	block := anthropic.ThinkingBlockParam{
		Signature: strings.TrimSpace(signature),
		Thinking:  thinking,
	}
	if block.Signature == "" {
		block.SetExtraFields(map[string]any{"signature": param.Omit})
	}
	return anthropic.ContentBlockParamUnion{OfThinking: &block}
}

// buildContentParts converts multimodal ContentParts to Anthropic content blocks.
func buildContentParts(parts []llmsvc.ChatMessageContentPart) []anthropic.ContentBlockParamUnion {
	var result []anthropic.ContentBlockParamUnion
	for _, part := range parts {
		switch part.Type {
		case llmsvc.ChatMessageContentPartTypeText:
			text := strings.TrimSpace(part.Text)
			if text != "" {
				result = append(result, anthropic.NewTextBlock(text))
			}

		case llmsvc.ChatMessageContentPartTypeImage:
			imageURL := strings.TrimSpace(part.ImageURL)
			if imageURL == "" {
				continue
			}
			if strings.HasPrefix(imageURL, "data:") {
				mimeEnd := strings.Index(imageURL, ";")
				if mimeEnd < 0 {
					continue
				}
				mimeType := imageURL[5:mimeEnd]
				b64Start := strings.Index(imageURL, ",")
				if b64Start < 0 {
					continue
				}
				b64Data := imageURL[b64Start+1:]
				result = append(result, anthropic.ContentBlockParamUnion{
					OfImage: &anthropic.ImageBlockParam{
						Source: anthropic.ImageBlockParamSourceUnion{
							OfBase64: &anthropic.Base64ImageSourceParam{
								Data:      b64Data,
								MediaType: anthropic.Base64ImageSourceMediaType(mimeType),
							},
						},
					},
				})
			} else {
				result = append(result, anthropic.ContentBlockParamUnion{
					OfImage: &anthropic.ImageBlockParam{
						Source: anthropic.ImageBlockParamSourceUnion{
							OfURL: &anthropic.URLImageSourceParam{
								URL: imageURL,
							},
						},
					},
				})
			}

		case llmsvc.ChatMessageContentPartTypeAudio, llmsvc.ChatMessageContentPartTypeFile:
			text := buildFallbackTextForPart(part)
			if text != "" {
				result = append(result, anthropic.NewTextBlock(text))
			}
		}
	}
	return result
}

// buildFallbackTextForPart produces a text fallback for unsupported content types.
func buildFallbackTextForPart(part llmsvc.ChatMessageContentPart) string {
	switch part.Type {
	case llmsvc.ChatMessageContentPartTypeAudio:
		return fmt.Sprintf("[Audio attachment: format=%s]", part.InputAudioFormat)
	case llmsvc.ChatMessageContentPartTypeFile:
		name := part.Filename
		if name == "" {
			name = "attachment"
		}
		return fmt.Sprintf("[File attachment: %s]", name)
	default:
		return ""
	}
}

// buildAnthropicTools converts internal ToolDefs to Anthropic ToolUnionParam values.
func buildAnthropicTools(defs []llmsvc.ToolDef) []anthropic.ToolUnionParam {
	var tools []anthropic.ToolUnionParam
	for _, def := range defs {
		tools = append(tools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        def.Name,
				Description: anthropic.String(def.Description),
				InputSchema: buildAnthropicInputSchema(def.Parameters),
			},
		})
	}
	return tools
}

func buildAnthropicInputSchema(parameters map[string]any) anthropic.ToolInputSchemaParam {
	schema := anthropic.ToolInputSchemaParam{
		Properties:  map[string]any{},
		ExtraFields: map[string]any{},
	}
	for key, value := range parameters {
		switch key {
		case "type":
			continue
		case "properties":
			if properties, ok := value.(map[string]any); ok {
				schema.Properties = properties
			}
		case "required":
			schema.Required = toStringSlice(value)
		default:
			schema.ExtraFields[key] = value
		}
	}
	return schema
}

func toStringSlice(value any) []string {
	switch items := value.(type) {
	case []string:
		return items
	case []any:
		result := make([]string, 0, len(items))
		for _, item := range items {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}
