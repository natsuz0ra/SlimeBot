package repositories

import (
	"context"
	"os"
	"testing"
	"time"

	"slimebot/internal/domain"
)

func TestMemoryVectorRepository_Integration_UpsertAndSearch(t *testing.T) {
	if os.Getenv("QDRANT_INTEGRATION") != "1" {
		t.Skip("set QDRANT_INTEGRATION=1 to run integration test")
	}

	collection := "session_memories_integration_" + time.Now().Format("20060102150405")
	repo, err := NewMemoryVectorRepository("127.0.0.1:6334", collection)
	if err != nil {
		t.Fatalf("create repository failed: %v", err)
	}
	defer func() { _ = repo.Close() }()

	if err := repo.UpsertSessionMemoryVector(context.Background(), domain.MemoryVectorUpsertInput{
		MemoryID:  "00000000-0000-0000-0000-000000000001",
		SessionID: "integration-s1",
		Vector:    []float32{0.12, 0.23, 0.34},
		Payload: map[string]any{
			"summary": "integration memory",
		},
	}); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	hits, err := repo.SearchSimilarSessionIDs(context.Background(), []float32{0.12, 0.23, 0.34}, 3, "")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one hit")
	}
	if hits[0].SessionID != "integration-s1" {
		t.Fatalf("expected integration-s1 top hit, got %q", hits[0].SessionID)
	}
}
