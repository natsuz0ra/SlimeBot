package repositories

import (
	"slimebot/backend/internal/domain"
	"testing"
)

func TestMessagePlatformConfigCRUD(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "platform_repo"))

	created, err := repo.CreateMessagePlatformConfig(domain.MessagePlatformConfig{
		Platform:       "telegram",
		DisplayName:    "Telegram",
		AuthConfigJSON: `{"botToken":"test-token"}`,
		IsEnabled:      true,
	})
	if err != nil {
		t.Fatalf("create config failed: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty config id")
	}

	got, err := repo.GetMessagePlatformConfigByPlatform("telegram")
	if err != nil {
		t.Fatalf("get by platform failed: %v", err)
	}
	if got == nil || got.DisplayName != "Telegram" {
		t.Fatalf("unexpected config: %+v", got)
	}

	err = repo.UpdateMessagePlatformConfig(created.ID, domain.MessagePlatformConfig{
		DisplayName:    "Telegram Bot",
		AuthConfigJSON: `{"botToken":"new-token"}`,
		IsEnabled:      false,
	})
	if err != nil {
		t.Fatalf("update config failed: %v", err)
	}
	got, err = repo.GetMessagePlatformConfigByPlatform("telegram")
	if err != nil {
		t.Fatalf("reload by platform failed: %v", err)
	}
	if got == nil || got.DisplayName != "Telegram Bot" || got.IsEnabled {
		t.Fatalf("unexpected updated config: %+v", got)
	}

	if err := repo.DeleteMessagePlatformConfig(created.ID); err != nil {
		t.Fatalf("delete config failed: %v", err)
	}
	got, err = repo.GetMessagePlatformConfigByPlatform("telegram")
	if err != nil {
		t.Fatalf("get after delete failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil after delete, got=%+v", got)
	}
}
