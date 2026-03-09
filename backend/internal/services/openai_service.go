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
	Role    string `json:"role"`
	Content string `json:"content"`
}

func NewOpenAIClient() *OpenAIClient {
	return &OpenAIClient{
		Client: &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *OpenAIClient) StreamChat(ctx context.Context, modelConfig ModelRuntimeConfig, messages []ChatMessage, onChunk func(string) error) error {
	baseURL := strings.TrimRight(strings.TrimSpace(modelConfig.BaseURL), "/")
	apiKey := strings.TrimSpace(modelConfig.APIKey)
	model := strings.TrimSpace(modelConfig.Model)
	if baseURL == "" || apiKey == "" || model == "" {
		return fmt.Errorf("模型配置缺失 baseUrl/apiKey/model")
	}

	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(c.Client),
	)

	var requestMessages []openai.ChatCompletionMessageParamUnion
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(message.Role)) {
		case "system":
			requestMessages = append(requestMessages, openai.SystemMessage(content))
		case "assistant":
			requestMessages = append(requestMessages, openai.AssistantMessage(content))
		case "developer":
			requestMessages = append(requestMessages, openai.DeveloperMessage(content))
		default:
			requestMessages = append(requestMessages, openai.UserMessage(content))
		}
	}
	if len(requestMessages) == 0 {
		return fmt.Errorf("请求消息为空")
	}

	stream := client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Messages: requestMessages,
		Model:    openai.ChatModel(model),
	})

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}
		content := chunk.Choices[0].Delta.Content
		if content != "" {
			if err := onChunk(content); err != nil {
				return err
			}
		}
	}
	if err := stream.Err(); err != nil {
		return fmt.Errorf("模型请求失败: %w", err)
	}
	return nil
}
