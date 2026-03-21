package platforms

import "testing"

func TestParseTelegramBotToken(t *testing.T) {
	got := ParseTelegramBotToken(`{"botToken":"  abc123  "}`)
	if got != "abc123" {
		t.Fatalf("expected trimmed token, got %q", got)
	}
}

func TestParseTelegramBotToken_InvalidJSON(t *testing.T) {
	if got := ParseTelegramBotToken("{"); got != "" {
		t.Fatalf("expected empty token for invalid json, got %q", got)
	}
}

func TestValidateAuthConfig_TelegramRequiresToken(t *testing.T) {
	err := ValidateAuthConfig("telegram", `{"botToken":"   "}`)
	if err == nil {
		t.Fatal("expected telegram config validation error")
	}
}

func TestValidateAuthConfig_NonTelegramOnlyChecksJSON(t *testing.T) {
	if err := ValidateAuthConfig("custom", `{"anything":1}`); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
