package repositories

import (
	"context"
	"errors"
	"testing"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	cemb "github.com/amikos-tech/chroma-go/pkg/embeddings"
	"slimebot/internal/domain"
)

func TestMemoryVectorRepository_UpsertAndSearch(t *testing.T) {
	t.Helper()

	col := &mockChromaCollection{}
	client := &mockChromaClient{col: col}
	repo := NewMemoryVectorRepositoryWithClient(client, "/tmp/chroma", "session_memories")

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
	if hits[0].MemoryID != "m1" {
		t.Fatalf("unexpected memory id %q", hits[0].MemoryID)
	}
	if client.getOrCreateCount != 1 {
		t.Fatalf("expected collection created once, got=%d", client.getOrCreateCount)
	}
}

func TestMemoryVectorRepository_GetCollectionError(t *testing.T) {
	client := &mockChromaClient{
		getOrCreateErr: errors.New("create failed"),
		col:            &mockChromaCollection{},
	}
	repo := NewMemoryVectorRepositoryWithClient(client, "/tmp/chroma", "session_memories")
	err := repo.UpsertSessionMemoryVector(context.Background(), domain.MemoryVectorUpsertInput{
		MemoryID:  "m1",
		SessionID: "s1",
		Vector:    []float32{1, 2, 3},
	})
	if err == nil {
		t.Fatal("expected error when collection creation fails")
	}
}

func TestMemoryVectorRepository_SearchEmptyInput(t *testing.T) {
	repo := NewMemoryVectorRepositoryWithClient(&mockChromaClient{col: &mockChromaCollection{}}, "/tmp/chroma", "session_memories")
	hits, err := repo.SearchMemoriesInSession(context.Background(), nil, "s1", 3)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected empty hits, got %d", len(hits))
	}
}

type mockChromaClient struct {
	col              chromaCollection
	getOrCreateErr   error
	getOrCreateCount int
}

func (m *mockChromaClient) GetOrCreateCollection(_ context.Context, _ string, _ ...chroma.CreateCollectionOption) (chromaCollection, error) {
	if m.getOrCreateErr != nil {
		return nil, m.getOrCreateErr
	}
	m.getOrCreateCount++
	return m.col, nil
}

func (m *mockChromaClient) Close() error {
	return nil
}

type mockRecord struct {
	id       string
	metadata chroma.DocumentMetadata
}

type mockChromaCollection struct {
	upsertErr error
	queryErr  error
	records   []mockRecord
}

func (m *mockChromaCollection) Upsert(_ context.Context, opts ...chroma.CollectionAddOption) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	op, err := chroma.NewCollectionAddOp(opts...)
	if err != nil {
		return err
	}
	for idx, id := range op.Ids {
		meta := chroma.NewDocumentMetadata()
		if len(op.Metadatas) > idx && op.Metadatas[idx] != nil {
			meta = op.Metadatas[idx]
		}
		m.records = append(m.records, mockRecord{
			id:       string(id),
			metadata: meta,
		})
	}
	return nil
}

func (m *mockChromaCollection) Query(_ context.Context, opts ...chroma.CollectionQueryOption) (chroma.QueryResult, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	op, err := chroma.NewCollectionQueryOp(opts...)
	if err != nil {
		return nil, err
	}
	sessionID := ""
	if clause, ok := op.Where.(interface {
		Key() string
		Operand() interface{}
	}); ok && clause.Key() == "session_id" {
		if text, ok := clause.Operand().(string); ok {
			sessionID = text
		}
	}

	ids := make(chroma.DocumentIDs, 0)
	metas := make(chroma.DocumentMetadatas, 0)
	dists := make(cemb.Distances, 0)
	for _, item := range m.records {
		if sessionID != "" {
			if sid, ok := item.metadata.GetString("session_id"); !ok || sid != sessionID {
				continue
			}
		}
		ids = append(ids, chroma.DocumentID(item.id))
		metas = append(metas, item.metadata)
		dists = append(dists, cemb.Distance(0.01))
		if len(ids) >= op.NResults {
			break
		}
	}
	return &chroma.QueryResultImpl{
		IDLists:        []chroma.DocumentIDs{ids},
		MetadatasLists: []chroma.DocumentMetadatas{metas},
		DistancesLists: []cemb.Distances{dists},
	}, nil
}
