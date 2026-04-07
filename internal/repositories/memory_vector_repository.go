package repositories

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slimebot/internal/domain"
	"slimebot/internal/logging"
	"strings"
	"sync"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	cemb "github.com/amikos-tech/chroma-go/pkg/embeddings"
)

type MemoryVectorRepository struct {
	chromaPath string
	collection string
	client     chromaClient

	ensureCollectionMu sync.Mutex
	col                chromaCollection
}

type chromaClient interface {
	GetOrCreateCollection(ctx context.Context, name string, opts ...chroma.CreateCollectionOption) (chromaCollection, error)
	Close() error
}

type chromaCollection interface {
	Upsert(ctx context.Context, opts ...chroma.CollectionAddOption) error
	Query(ctx context.Context, opts ...chroma.CollectionQueryOption) (chroma.QueryResult, error)
}

type chromaClientAdapter struct {
	client chroma.Client
}

func (c *chromaClientAdapter) GetOrCreateCollection(ctx context.Context, name string, opts ...chroma.CreateCollectionOption) (chromaCollection, error) {
	col, err := c.client.GetOrCreateCollection(ctx, name, opts...)
	if err != nil {
		return nil, err
	}
	return &chromaCollectionAdapter{col: col}, nil
}

func (c *chromaClientAdapter) Close() error {
	return c.client.Close()
}

type chromaCollectionAdapter struct {
	col chroma.Collection
}

func (c *chromaCollectionAdapter) Upsert(ctx context.Context, opts ...chroma.CollectionAddOption) error {
	return c.col.Upsert(ctx, opts...)
}

func (c *chromaCollectionAdapter) Query(ctx context.Context, opts ...chroma.CollectionQueryOption) (chroma.QueryResult, error) {
	return c.col.Query(ctx, opts...)
}

func NewMemoryVectorRepository(chromaPath string, collection string) (*MemoryVectorRepository, error) {
	persistPath := strings.TrimSpace(chromaPath)
	if persistPath == "" {
		return nil, fmt.Errorf("chroma path cannot be empty")
	}
	logging.Info("resource_prepare_start",
		"resource", "chroma_runtime",
		"persist_path", persistPath,
	)
	if err := os.MkdirAll(persistPath, os.ModePerm); err != nil {
		logging.Warn("resource_prepare_failed",
			"resource", "chroma_runtime",
			"persist_path", persistPath,
			"stage", "mkdir_persist_path",
			"err", err,
		)
		return nil, err
	}
	runtimeCachePath := filepath.Join(persistPath, ".local_runtime_cache")
	if err := os.MkdirAll(runtimeCachePath, os.ModePerm); err != nil {
		logging.Warn("resource_prepare_failed",
			"resource", "chroma_runtime",
			"persist_path", persistPath,
			"runtime_cache_path", runtimeCachePath,
			"stage", "mkdir_runtime_cache",
			"err", err,
		)
		return nil, err
	}
	client, err := chroma.NewPersistentClient(
		chroma.WithPersistentPath(persistPath),
		chroma.WithPersistentLibraryCacheDir(runtimeCachePath),
	)
	if err != nil {
		logging.Warn("resource_prepare_failed",
			"resource", "chroma_runtime",
			"persist_path", persistPath,
			"runtime_cache_path", runtimeCachePath,
			"stage", "new_persistent_client",
			"err", err,
		)
		return nil, err
	}
	logging.Info("resource_prepare_done",
		"resource", "chroma_runtime",
		"persist_path", persistPath,
		"runtime_cache_path", runtimeCachePath,
	)
	return &MemoryVectorRepository{
		chromaPath: persistPath,
		collection: strings.TrimSpace(collection),
		client:     &chromaClientAdapter{client: client},
	}, nil
}

func NewMemoryVectorRepositoryWithClient(client chromaClient, chromaPath string, collection string) *MemoryVectorRepository {
	return &MemoryVectorRepository{
		chromaPath: strings.TrimSpace(chromaPath),
		collection: strings.TrimSpace(collection),
		client:     client,
	}
}

func (r *MemoryVectorRepository) UpsertSessionMemoryVector(ctx context.Context, input domain.MemoryVectorUpsertInput) error {
	if err := r.validateConfig(); err != nil {
		return err
	}
	memoryID := strings.TrimSpace(input.MemoryID)
	sessionID := strings.TrimSpace(input.SessionID)
	if memoryID == "" {
		return fmt.Errorf("memory_id cannot be empty")
	}
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}
	if len(input.Vector) == 0 {
		return fmt.Errorf("vector cannot be empty")
	}
	col, err := r.ensureCollection(ctx)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"session_id": sessionID,
		"memory_id":  memoryID,
	}
	for k, v := range input.Payload {
		payload[k] = v
	}
	metadata, err := chroma.NewDocumentMetadataFromMap(payload)
	if err != nil {
		return err
	}
	err = col.Upsert(ctx,
		chroma.WithIDs(chroma.DocumentID(memoryID)),
		chroma.WithEmbeddings(cemb.NewEmbeddingFromFloat32(input.Vector)),
		chroma.WithMetadatas(metadata),
	)
	if err != nil {
		return fmt.Errorf("chroma upsert failed: %w", err)
	}
	return nil
}

func (r *MemoryVectorRepository) SearchMemoriesInSession(ctx context.Context, queryVector []float32, sessionID string, limit int) ([]domain.MemoryVectorSearchHit, error) {
	if err := r.validateConfig(); err != nil {
		return nil, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if len(queryVector) == 0 || limit <= 0 || sessionID == "" {
		return []domain.MemoryVectorSearchHit{}, nil
	}
	col, err := r.ensureCollection(ctx)
	if err != nil {
		return nil, err
	}
	result, err := col.Query(ctx,
		chroma.WithQueryEmbeddings(cemb.NewEmbeddingFromFloat32(queryVector)),
		chroma.WithNResults(limit),
		chroma.WithWhere(chroma.EqString("session_id", sessionID)),
		chroma.WithInclude(chroma.IncludeMetadatas, chroma.IncludeDistances),
	)
	if err != nil {
		return nil, err
	}
	return queryResultToMemoryHits(result, sessionID), nil
}

func queryResultToMemoryHits(result chroma.QueryResult, fallbackSessionID string) []domain.MemoryVectorSearchHit {
	idsGroups := result.GetIDGroups()
	if len(idsGroups) == 0 || len(idsGroups[0]) == 0 {
		return []domain.MemoryVectorSearchHit{}
	}
	metasGroups := result.GetMetadatasGroups()
	distGroups := result.GetDistancesGroups()

	hits := make([]domain.MemoryVectorSearchHit, 0, len(idsGroups[0]))
	for idx, id := range idsGroups[0] {
		sessionID := strings.TrimSpace(fallbackSessionID)
		memoryID := strings.TrimSpace(string(id))
		if len(metasGroups) > 0 && len(metasGroups[0]) > idx && metasGroups[0][idx] != nil {
			if text, ok := metasGroups[0][idx].GetString("session_id"); ok && strings.TrimSpace(text) != "" {
				sessionID = strings.TrimSpace(text)
			}
			if text, ok := metasGroups[0][idx].GetString("memory_id"); ok && strings.TrimSpace(text) != "" {
				memoryID = strings.TrimSpace(text)
			}
		}
		if sessionID == "" {
			continue
		}
		score := 0.0
		if len(distGroups) > 0 && len(distGroups[0]) > idx {
			score = float64(distGroups[0][idx])
		}
		hits = append(hits, domain.MemoryVectorSearchHit{
			SessionID: sessionID,
			MemoryID:  memoryID,
			Score:     score,
		})
	}
	return hits
}

func (r *MemoryVectorRepository) ensureCollection(ctx context.Context) (chromaCollection, error) {
	r.ensureCollectionMu.Lock()
	defer r.ensureCollectionMu.Unlock()

	if r.col != nil {
		return r.col, nil
	}

	col, err := r.client.GetOrCreateCollection(ctx, r.collection)
	if err != nil {
		return nil, err
	}
	r.col = col
	return col, nil
}

func (r *MemoryVectorRepository) Close() error {
	if r.client == nil {
		return nil
	}
	return r.client.Close()
}

func (r *MemoryVectorRepository) validateConfig() error {
	if strings.TrimSpace(r.chromaPath) == "" {
		return fmt.Errorf("chroma path cannot be empty")
	}
	if strings.TrimSpace(r.collection) == "" {
		return fmt.Errorf("chroma collection cannot be empty")
	}
	if r.client == nil {
		return fmt.Errorf("chroma client cannot be nil")
	}
	return nil
}
