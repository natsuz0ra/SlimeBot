package repositories

import (
	"slimebot/backend/internal/domain"
	"testing"
)

func TestAddMessageWithInput_PersistsFlagsAndAttachments(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_messages_test"))
	session, err := repo.CreateSession("s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	_, err = repo.AddMessageWithInput(AddMessageInput{
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

	items, listErr := repo.ListSessionMessages(session.ID)
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
