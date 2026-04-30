package openai

import (
	"encoding/json"
	"strings"
	"testing"

	llmsvc "slimebot/internal/services/llm"

	"github.com/openai/openai-go/v3"
)

func TestSupportsDeveloperRole(t *testing.T) {
	t.Run("dashscope compatible mode should disable developer role", func(t *testing.T) {
		baseURL := "https://dashscope.aliyuncs.com/compatible-mode/v1"
		if supportsDeveloperRole(baseURL) {
			t.Fatalf("expected developer role unsupported for %q", baseURL)
		}
	})

	t.Run("openai endpoint should keep developer role", func(t *testing.T) {
		baseURL := "https://api.openai.com/v1"
		if !supportsDeveloperRole(baseURL) {
			t.Fatalf("expected developer role supported for %q", baseURL)
		}
	})
}

func TestBuildRequestMessages_DeveloperRoleFallback(t *testing.T) {
	source := []llmsvc.ChatMessage{
		{Role: "developer", Content: "memory context"},
	}

	unsupportedMsgs := buildRequestMessages(source, false)
	rawUnsupported, err := json.Marshal(unsupportedMsgs)
	if err != nil {
		t.Fatalf("marshal unsupported messages failed: %v", err)
	}
	unsupportedJSON := string(rawUnsupported)
	if !strings.Contains(unsupportedJSON, `"role":"system"`) {
		t.Fatalf("expected fallback role system, got: %s", unsupportedJSON)
	}
	if strings.Contains(unsupportedJSON, `"role":"developer"`) {
		t.Fatalf("unexpected developer role in unsupported provider payload: %s", unsupportedJSON)
	}

	supportedMsgs := buildRequestMessages(source, true)
	rawSupported, err := json.Marshal(supportedMsgs)
	if err != nil {
		t.Fatalf("marshal supported messages failed: %v", err)
	}
	supportedJSON := string(rawSupported)
	if !strings.Contains(supportedJSON, `"role":"developer"`) {
		t.Fatalf("expected developer role preserved, got: %s", supportedJSON)
	}
}

func TestApplyThinkingParamsDeepSeek(t *testing.T) {
	tests := []struct {
		name         string
		level        string
		wantThinking string
		wantEffort   string
		forbidEffort bool
	}{
		{name: "off disables thinking", level: "off", wantThinking: `"thinking":{"type":"disabled"}`, forbidEffort: true},
		{name: "empty disables thinking", level: "", wantThinking: `"thinking":{"type":"disabled"}`, forbidEffort: true},
		{name: "low maps to high", level: "low", wantThinking: `"thinking":{"type":"enabled"}`, wantEffort: `"reasoning_effort":"high"`},
		{name: "medium maps to high", level: "medium", wantThinking: `"thinking":{"type":"enabled"}`, wantEffort: `"reasoning_effort":"high"`},
		{name: "high maps to high", level: "high", wantThinking: `"thinking":{"type":"enabled"}`, wantEffort: `"reasoning_effort":"high"`},
		{name: "max maps to max", level: "max", wantThinking: `"thinking":{"type":"enabled"}`, wantEffort: `"reasoning_effort":"max"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := openai.ChatCompletionNewParams{}
			applyThinkingParams(&params, llmsvc.ModelRuntimeConfig{
				Provider:      llmsvc.ProviderDeepSeek,
				ThinkingLevel: tt.level,
			})
			raw, err := json.Marshal(params)
			if err != nil {
				t.Fatalf("marshal params failed: %v", err)
			}
			got := string(raw)
			if !strings.Contains(got, tt.wantThinking) {
				t.Fatalf("expected %s in payload, got: %s", tt.wantThinking, got)
			}
			if tt.wantEffort != "" && !strings.Contains(got, tt.wantEffort) {
				t.Fatalf("expected %s in payload, got: %s", tt.wantEffort, got)
			}
			if tt.forbidEffort && strings.Contains(got, `"reasoning_effort"`) {
				t.Fatalf("did not expect reasoning_effort in payload, got: %s", got)
			}
		})
	}
}

func TestApplyThinkingParamsOpenAIDoesNotSendDeepSeekThinking(t *testing.T) {
	params := openai.ChatCompletionNewParams{}
	applyThinkingParams(&params, llmsvc.ModelRuntimeConfig{
		Provider:      llmsvc.ProviderOpenAI,
		ThinkingLevel: "high",
	})
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params failed: %v", err)
	}
	got := string(raw)
	if strings.Contains(got, `"thinking"`) {
		t.Fatalf("openai provider should not receive DeepSeek thinking field: %s", got)
	}
	if !strings.Contains(got, `"reasoning_effort":"high"`) {
		t.Fatalf("expected generic reasoning_effort in payload, got: %s", got)
	}
}

func TestBuildRequestMessages_UserContentParts(t *testing.T) {
	source := []llmsvc.ChatMessage{
		{
			Role: "user",
			ContentParts: []llmsvc.ChatMessageContentPart{
				{Type: llmsvc.ChatMessageContentPartTypeText, Text: "请分析附件"},
				{Type: llmsvc.ChatMessageContentPartTypeImage, ImageURL: "data:image/png;base64,aW1hZ2UtYnl0ZXM="},
				{Type: llmsvc.ChatMessageContentPartTypeAudio, InputAudioData: "YXVkaW8tYnl0ZXM=", InputAudioFormat: "mp3"},
				{Type: llmsvc.ChatMessageContentPartTypeFile, FileDataBase64: "ZmlsZS1ieXRlcw==", Filename: "notes.bin"},
			},
		},
	}

	msgs := buildRequestMessages(source, true)
	raw, err := json.Marshal(msgs)
	if err != nil {
		t.Fatalf("marshal messages failed: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"type":"image_url"`) || !strings.Contains(got, `"url":"data:image/png;base64,aW1hZ2UtYnl0ZXM="`) {
		t.Fatalf("expected image content part in payload, got: %s", got)
	}
	if !strings.Contains(got, `"type":"input_audio"`) || !strings.Contains(got, `"format":"mp3"`) {
		t.Fatalf("expected audio content part in payload, got: %s", got)
	}
	if !strings.Contains(got, `"type":"file"`) || !strings.Contains(got, `"file_data":"ZmlsZS1ieXRlcw=="`) || !strings.Contains(got, `"filename":"notes.bin"`) {
		t.Fatalf("expected file content part in payload, got: %s", got)
	}
}

func TestBuildAssistantMessageParamPreservesReasoningContentWithToolCalls(t *testing.T) {
	msgs := buildRequestMessages([]llmsvc.ChatMessage{{
		Role:             "assistant",
		Content:          "I need a tool.",
		ReasoningContent: "Need to inspect before answering.",
		ToolCalls: []llmsvc.ToolCallInfo{{
			ID:        "call-1",
			Name:      "exec__run",
			Arguments: `{"command":"pwd"}`,
		}},
	}}, true)

	raw, err := json.Marshal(msgs)
	if err != nil {
		t.Fatalf("marshal messages failed: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"reasoning_content":"Need to inspect before answering."`) {
		t.Fatalf("expected reasoning_content extra field in tool-call assistant message, got: %s", got)
	}
	if !strings.Contains(got, `"tool_calls"`) {
		t.Fatalf("expected tool_calls to be preserved, got: %s", got)
	}
}

func TestBuildAssistantMessageParamPreservesReasoningOnlyAssistantMessage(t *testing.T) {
	msgs := buildRequestMessages([]llmsvc.ChatMessage{{
		Role:             "assistant",
		ReasoningContent: "Reasoning-only compatible response.",
	}}, true)

	raw, err := json.Marshal(msgs)
	if err != nil {
		t.Fatalf("marshal messages failed: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"role":"assistant"`) {
		t.Fatalf("expected assistant message to be retained, got: %s", got)
	}
	if !strings.Contains(got, `"reasoning_content":"Reasoning-only compatible response."`) {
		t.Fatalf("expected reasoning_content extra field, got: %s", got)
	}
}
