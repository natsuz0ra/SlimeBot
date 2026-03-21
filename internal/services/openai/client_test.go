package openai

import (
	"encoding/json"
	"strings"
	"testing"
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
	source := []ChatMessage{
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

func TestBuildRequestMessages_UserContentParts(t *testing.T) {
	source := []ChatMessage{
		{
			Role: "user",
			ContentParts: []ChatMessageContentPart{
				{Type: ChatMessageContentPartTypeText, Text: "请分析附件"},
				{Type: ChatMessageContentPartTypeImage, ImageURL: "data:image/png;base64,aW1hZ2UtYnl0ZXM="},
				{Type: ChatMessageContentPartTypeAudio, InputAudioData: "YXVkaW8tYnl0ZXM=", InputAudioFormat: "mp3"},
				{Type: ChatMessageContentPartTypeFile, FileDataBase64: "ZmlsZS1ieXRlcw==", Filename: "notes.bin"},
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
