package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"slimebot/backend/internal/models"
	"slimebot/backend/internal/repositories"
)

func TestParseMemoryDecision_WithCodeFence(t *testing.T) {
	raw := "```json\n{\"need_memory\":true,\"keywords\":[\"golang\",\"rag\"],\"reason\":\"需要历史信息\"}\n```"
	decision, err := parseMemoryDecision(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !decision.NeedMemory {
		t.Fatal("expected need_memory=true")
	}
	if len(decision.Keywords) != 2 {
		t.Fatalf("unexpected keywords length: %d", len(decision.Keywords))
	}
}

func TestMemoryServiceTokenizeKeywords_Multilingual(t *testing.T) {
	svc := NewMemoryService(nil, nil)
	words := svc.TokenizeKeywords("请帮我优化 Golang RAG memory retrieval，在 Windows 上运行")
	if len(words) == 0 {
		t.Fatal("expected non-empty keywords")
	}
	joined := strings.Join(words, " ")
	if !strings.Contains(joined, "golang") {
		t.Fatalf("expected token list to contain golang, got %v", words)
	}
}

func TestMemoryServiceShouldCompressContext(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession("s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	for i := 0; i < compressHistoryThreshold; i++ {
		if _, err := repo.AddMessage(sessionID, "user", "hello"); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
	}

	ok, count, err := svc.ShouldCompressContext(sessionID)
	if err != nil {
		t.Fatalf("should compress failed: %v", err)
	}
	if !ok {
		t.Fatal("expected compress=true when threshold reached")
	}
	if count != compressHistoryThreshold {
		t.Fatalf("unexpected count: %d", count)
	}
}

func TestMemoryServiceRetrieveMemoriesRanking(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	if err := repo.UpsertSessionMemory(repositories.SessionMemoryUpsertInput{
		SessionID:          "s1",
		Summary:            "用户喜欢 golang 与 rag，关注 token 成本",
		Keywords:           []string{"golang", "rag", "token"},
		SourceMessageCount: 20,
	}); err != nil {
		t.Fatalf("upsert memory failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := repo.UpsertSessionMemory(repositories.SessionMemoryUpsertInput{
		SessionID:          "s2",
		Summary:            "用户只提到了 golang",
		Keywords:           []string{"golang"},
		SourceMessageCount: 10,
	}); err != nil {
		t.Fatalf("upsert memory failed: %v", err)
	}

	hits, err := svc.RetrieveMemories([]string{"golang", "rag"}, "", 5)
	if err != nil {
		t.Fatalf("retrieve memories failed: %v", err)
	}
	if len(hits) < 2 {
		t.Fatalf("expected at least 2 hits, got %d", len(hits))
	}
	if hits[0].Memory.SessionID != "s1" {
		t.Fatalf("expected s1 ranked first, got %s", hits[0].Memory.SessionID)
	}
}

func TestMemoryServiceUpdateSummaryAsync_NonBlocking(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession("s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	for i := 0; i < 3; i++ {
		if _, err := repo.AddMessage(sessionID, "user", "hello"); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
	}

	start := time.Now()
	svc.UpdateSummaryAsync(ModelRuntimeConfig{BaseURL: "http://invalid", APIKey: "x", Model: "y"}, sessionID)
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("expected async call to return quickly, elapsed=%s", elapsed)
	}
}

func TestChatServiceBuildContextMessages_NoDuplicateUserMessage(t *testing.T) {
	repo := newTestRepo(t)
	session, err := repo.CreateSession("s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	if _, err := repo.AddMessage(sessionID, "user", "only-once"); err != nil {
		t.Fatalf("add message failed: %v", err)
	}

	svc := &ChatService{repo: repo}
	msgs, err := svc.BuildContextMessages(context.Background(), sessionID, ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("build context failed: %v", err)
	}

	userCount := 0
	for _, msg := range msgs {
		if msg.Role == "user" && strings.TrimSpace(msg.Content) == "only-once" {
			userCount++
		}
	}
	if userCount != 1 {
		t.Fatalf("expected single user message, got %d", userCount)
	}
}

func newTestRepo(t *testing.T) *repositories.Repository {
	t.Helper()

	dsn := fmt.Sprintf("file:test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Session{},
		&models.Message{},
		&models.SessionMemory{},
		&models.ToolCallRecord{},
		&models.AppSetting{},
		&models.LLMConfig{},
		&models.MCPConfig{},
		&models.Skill{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return repositories.New(db)
}
