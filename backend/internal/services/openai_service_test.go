package services

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
