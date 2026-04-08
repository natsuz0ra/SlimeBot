package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"

	llmsvc "slimebot/internal/services/llm"

	"github.com/anthropics/anthropic-sdk-go"
)

// buildAnthropicMessages 将内部 ChatMessage 列表转为 Anthropic SDK 参数。
// 返回 system blocks（顶层参数）和 messages 数组。
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

// buildAssistantBlocks 构建 assistant 消息的内容块（文本 + tool_use）。
func buildAssistantBlocks(msg llmsvc.ChatMessage) []anthropic.ContentBlockParamUnion {
	var blocks []anthropic.ContentBlockParamUnion

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

// buildContentParts 将多模态 ContentParts 转为 Anthropic 内容块。
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

// buildFallbackTextForPart 为不支持的 content type 生成文本回退。
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

// buildAnthropicTools 将内部 ToolDef 转为 Anthropic ToolUnionParam。
func buildAnthropicTools(defs []llmsvc.ToolDef) []anthropic.ToolUnionParam {
	var tools []anthropic.ToolUnionParam
	for _, def := range defs {
		tools = append(tools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        def.Name,
				Description: anthropic.String(def.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type:        "object",
					Properties:  def.Parameters,
					ExtraFields: map[string]any{},
				},
			},
		})
	}
	return tools
}
