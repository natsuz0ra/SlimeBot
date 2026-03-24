package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/qdrant/go-client/qdrant"
	"slimebot/internal/domain"
)

func TestMemoryVectorRepository_UpsertAndSearch(t *testing.T) {
	t.Helper()

	client := &mockQdrantClient{
		searchResults: []*qdrant.ScoredPoint{
			{
				Id:    qdrant.NewID("s1"),
				Score: 0.9,
			},
		},
	}
	repo := NewMemoryVectorRepositoryWithClient(client, "session_memories")
	err := repo.UpsertSessionMemoryVector(context.Background(), domain.MemoryVectorUpsertInput{
		MemoryID:  "m1",
		SessionID: "s1",
		Vector:    []float32{0.1, 0.2, 0.3},
		Payload: map[string]any{
			"summary": "hello",
		},
	})
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	hits, err := repo.SearchMemoriesInSession(context.Background(), []float32{0.1, 0.2, 0.3}, "s1", 3)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].SessionID != "s1" {
		t.Fatalf("expected session id s1, got %q", hits[0].SessionID)
	}
	if hits[0].MemoryID != "s1" && hits[0].MemoryID != "m1" {
		t.Fatalf("unexpected memory id %q", hits[0].MemoryID)
	}
	if client.collectionCreated != 1 {
		t.Fatalf("expected collection created once, got=%d", client.collectionCreated)
	}
}

func TestMemoryVectorRepository_CollectionExistsError(t *testing.T) {
	client := &mockQdrantClient{
		existsErr: errors.New("exists failed"),
	}
	repo := NewMemoryVectorRepositoryWithClient(client, "session_memories")
	err := repo.UpsertSessionMemoryVector(context.Background(), domain.MemoryVectorUpsertInput{
		MemoryID:  "m1",
		SessionID: "s1",
		Vector:    []float32{1, 2, 3},
	})
	if err == nil {
		t.Fatal("expected error when collection exists check fails")
	}
}

type mockQdrantClient struct {
	collectionExists  bool
	collectionCreated int
	existsErr         error
	createErr         error
	upsertErr         error
	queryErr          error
	searchResults     []*qdrant.ScoredPoint
}

func (m *mockQdrantClient) CollectionExists(_ context.Context, _ string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	return m.collectionExists, nil
}

func (m *mockQdrantClient) CreateCollection(_ context.Context, _ *qdrant.CreateCollection) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.collectionExists = true
	m.collectionCreated++
	return nil
}

func (m *mockQdrantClient) CreateFieldIndex(_ context.Context, _ *qdrant.CreateFieldIndexCollection) (*qdrant.UpdateResult, error) {
	return &qdrant.UpdateResult{}, nil
}

func (m *mockQdrantClient) Upsert(_ context.Context, _ *qdrant.UpsertPoints) (*qdrant.UpdateResult, error) {
	if m.upsertErr != nil {
		return nil, m.upsertErr
	}
	return &qdrant.UpdateResult{}, nil
}

func (m *mockQdrantClient) Query(_ context.Context, _ *qdrant.QueryPoints) ([]*qdrant.ScoredPoint, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return m.searchResults, nil
}

func (m *mockQdrantClient) Close() error {
	return nil
}
