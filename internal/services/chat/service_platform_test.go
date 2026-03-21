package chat

import (
	"context"
	"slimebot/internal/domain"
	"testing"

	"slimebot/internal/constants"
	"slimebot/internal/repositories"
)

func TestEnsureMessagePlatformSession_StableID(t *testing.T) {
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "platform_session"))
	service := &ChatService{store: repo}

	session, err := service.EnsureMessagePlatformSession(context.Background())
	if err != nil {
		t.Fatalf("ensure platform session failed: %v", err)
	}
	if session.ID != constants.MessagePlatformSessionID {
		t.Fatalf("expected fixed session id=%s, got=%s", constants.MessagePlatformSessionID, session.ID)
	}

	second, err := service.EnsureMessagePlatformSession(context.Background())
	if err != nil {
		t.Fatalf("ensure existing platform session failed: %v", err)
	}
	if second.ID != constants.MessagePlatformSessionID {
		t.Fatalf("expected same fixed session id, got=%s", second.ID)
	}
}

func TestResolvePlatformModel_FallbackAndPersist(t *testing.T) {
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "platform_model"))
	service := &ChatService{store: repo}

	first, err := repo.CreateLLMConfig(domain.LLMConfig{
		Name:    "model-a",
		BaseURL: "http://localhost:9999",
		APIKey:  "k1",
		Model:   "m1",
	})
	if err != nil {
		t.Fatalf("create first model failed: %v", err)
	}
	second, err := repo.CreateLLMConfig(domain.LLMConfig{
		Name:    "model-b",
		BaseURL: "http://localhost:9998",
		APIKey:  "k2",
		Model:   "m2",
	})
	if err != nil {
		t.Fatalf("create second model failed: %v", err)
	}

	if err := repo.SetSetting(constants.SettingMessagePlatformDefaultModel, "missing-id"); err != nil {
		t.Fatalf("set platform default failed: %v", err)
	}
	if err := repo.SetSetting(constants.SettingDefaultModel, second.ID); err != nil {
		t.Fatalf("set global default failed: %v", err)
	}

	modelID, err := service.ResolvePlatformModel(context.Background())
	if err != nil {
		t.Fatalf("resolve platform model failed: %v", err)
	}
	if modelID != second.ID {
		t.Fatalf("expected fallback to global default=%s, got=%s", second.ID, modelID)
	}

	persisted, err := repo.GetSetting(constants.SettingMessagePlatformDefaultModel)
	if err != nil {
		t.Fatalf("read persisted platform default failed: %v", err)
	}
	if persisted != second.ID {
		t.Fatalf("expected persisted platform default=%s, got=%s", second.ID, persisted)
	}

	if err := repo.SetSetting(constants.SettingMessagePlatformDefaultModel, ""); err != nil {
		t.Fatalf("clear platform default failed: %v", err)
	}
	if err := repo.SetSetting(constants.SettingDefaultModel, ""); err != nil {
		t.Fatalf("clear global default failed: %v", err)
	}
	// 删除 second 后，应回落到首个可用模型（按 name asc，这里是 model-a）。
	if err := repo.DeleteLLMConfig(second.ID); err != nil {
		t.Fatalf("delete second model failed: %v", err)
	}

	modelID, err = service.ResolvePlatformModel(context.Background())
	if err != nil {
		t.Fatalf("resolve platform model without defaults failed: %v", err)
	}
	if modelID != first.ID {
		t.Fatalf("expected fallback to first available model=%s, got=%s", first.ID, modelID)
	}
}
