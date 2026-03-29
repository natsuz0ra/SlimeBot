package repositories

import (
	"context"
	"slimebot/internal/domain"
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

	items, _, listErr := repo.ListSessionMessagesPage(session.ID, 100, nil, nil, nil, nil)
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

	page, hasMore, err := repo.ListSessionMessagesPage(session.ID, 2, nil, nil, nil, nil)
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
