package repositories

import (
	"context"
	"slimebot/internal/domain"
	llmsvc "slimebot/internal/services/llm"
	"testing"
	"time"
)

func TestAddMessageWithInput_PersistsFlagsAndAttachments(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_messages_test"))
	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	_, err = repo.AddMessageWithInput(context.Background(), domain.AddMessageInput{
		SessionID:         session.ID,
		Role:              "assistant",
		Content:           "",
		IsInterrupted:     true,
		IsStopPlaceholder: true,
		Attachments: []domain.MessageAttachment{
			{
				ID:        "att1",
				Name:      "a.txt",
				Ext:       "TXT",
				SizeBytes: 12,
				MimeType:  "text/plain",
				IconType:  "text",
			},
		},
	})
	if err != nil {
		t.Fatalf("add message failed: %v", err)
	}

	items, _, listErr := repo.ListSessionMessagesPage(context.Background(), session.ID, 100, nil, nil, nil, nil)
	if listErr != nil {
		t.Fatalf("list messages failed: %v", listErr)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 message, got %d", len(items))
	}
	got := items[0]
	if !got.IsInterrupted || !got.IsStopPlaceholder {
		t.Fatalf("expected interrupted stop-placeholder flags true, got interrupted=%v stop=%v", got.IsInterrupted, got.IsStopPlaceholder)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Name != "a.txt" {
		t.Fatalf("unexpected attachments payload: %+v", got.Attachments)
	}
}

func TestAddMessageWithInput_PersistsTokenUsage(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_messages_usage_test"))
	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	_, err = repo.AddMessageWithInput(context.Background(), domain.AddMessageInput{
		SessionID: session.ID,
		Role:      "assistant",
		Content:   "answer",
		TokenUsage: &llmsvc.TokenUsage{
			InputTokens:              100,
			OutputTokens:             20,
			CacheCreationInputTokens: 3,
			CacheReadInputTokens:     4,
		},
	})
	if err != nil {
		t.Fatalf("add message failed: %v", err)
	}

	items, _, listErr := repo.ListSessionMessagesPage(context.Background(), session.ID, 100, nil, nil, nil, nil)
	if listErr != nil {
		t.Fatalf("list messages failed: %v", listErr)
	}
	if len(items) != 1 || items[0].TokenUsage == nil {
		t.Fatalf("expected token usage to round-trip, got %+v", items)
	}
	if got := items[0].TokenUsage; got.InputTokens != 100 || got.OutputTokens != 20 || got.CacheCreationInputTokens != 3 || got.CacheReadInputTokens != 4 {
		t.Fatalf("unexpected token usage: %+v", got)
	}
}

func TestListSessionMessagesPage_HasMoreWithLimitPlusOne(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_messages_pagination_test"))
	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, err := repo.AddMessageWithInput(context.Background(), domain.AddMessageInput{
			SessionID: session.ID,
			Role:      "user",
			Content:   "m",
		}); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
		time.Sleep(1 * time.Millisecond)
	}

	page, hasMore, err := repo.ListSessionMessagesPage(context.Background(), session.ID, 2, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("list page failed: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(page))
	}
	if !hasMore {
		t.Fatal("expected hasMore=true when there are remaining messages")
	}
}

func TestListSessionMessagesPage_DefaultReturnsNewest(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_messages_newest_test"))
	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	// Insert 3 messages with distinct content and small delay to ensure ordering.
	contents := []string{"oldest", "middle", "newest"}
	for _, c := range contents {
		if _, err := repo.AddMessageWithInput(context.Background(), domain.AddMessageInput{
			SessionID: session.ID,
			Role:      "user",
			Content:   c,
		}); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Request limit=2 (default case, no cursor) — should return the 2 newest messages.
	page, hasMore, err := repo.ListSessionMessagesPage(context.Background(), session.ID, 2, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("list page failed: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(page))
	}
	if !hasMore {
		t.Fatal("expected hasMore=true")
	}
	// Must return the 2 newest messages in chronological order.
	if page[0].Content != "middle" || page[1].Content != "newest" {
		t.Fatalf("expected [middle, newest], got [%s, %s]", page[0].Content, page[1].Content)
	}
}

func TestListSessionMessagesPage_BeforeCursorReturnsNewestBeforeCursor(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_messages_before_test"))
	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	// Insert 5 messages with distinct content.
	for i, c := range []string{"m0", "m1", "m2", "m3", "m4"} {
		if _, err := repo.AddMessageWithInput(context.Background(), domain.AddMessageInput{
			SessionID: session.ID,
			Role:      "user",
			Content:   c,
		}); err != nil {
			t.Fatalf("add message %d failed: %v", i, err)
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Get m3's cursor (4th message, 0-indexed).
	all, _, err := repo.ListSessionMessagesPage(context.Background(), session.ID, 100, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("list all failed: %v", err)
	}
	m3 := all[3]

	// Request 2 messages before m3 — should return [m1, m2] (the 2 newest before the cursor).
	before := m3.CreatedAt
	beforeSeq := m3.Seq
	page, hasMore, err := repo.ListSessionMessagesPage(context.Background(), session.ID, 2, &before, &beforeSeq, nil, nil)
	if err != nil {
		t.Fatalf("list page failed: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(page))
	}
	if !hasMore {
		t.Fatal("expected hasMore=true (m0 exists before m1)")
	}
	if page[0].Content != "m1" || page[1].Content != "m2" {
		t.Fatalf("expected [m1, m2], got [%s, %s]", page[0].Content, page[1].Content)
	}
}

func TestAddMessageWithInput_AssignsIncreasingSeq(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_messages_seq_test"))
	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	first, err := repo.AddMessageWithInput(context.Background(), domain.AddMessageInput{
		SessionID: session.ID,
		Role:      "user",
		Content:   "a",
	})
	if err != nil {
		t.Fatalf("add first message failed: %v", err)
	}
	second, err := repo.AddMessageWithInput(context.Background(), domain.AddMessageInput{
		SessionID: session.ID,
		Role:      "assistant",
		Content:   "b",
	})
	if err != nil {
		t.Fatalf("add second message failed: %v", err)
	}
	if first.Seq != 1 || second.Seq != 2 {
		t.Fatalf("unexpected seq values: first=%d second=%d", first.Seq, second.Seq)
	}
}

func TestAddMessageWithInput_UsesProvidedCreatedAt(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_messages_created_at_test"))
	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	createdAt := time.Date(2026, 4, 29, 1, 2, 3, 456000000, time.UTC)

	message, err := repo.AddMessageWithInput(context.Background(), domain.AddMessageInput{
		SessionID: session.ID,
		Role:      "user",
		Content:   "hello",
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("add message failed: %v", err)
	}

	if !message.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected createdAt %s, got %s", createdAt, message.CreatedAt)
	}
}
