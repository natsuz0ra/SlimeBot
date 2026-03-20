package repositories

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/qdrant/go-client/qdrant"
	"slimebot/backend/internal/domain"
)

type MemoryVectorRepository struct {
	qdrantURL  string
	collection string
	client     qdrantVectorClient

	ensureCollectionMu sync.Mutex
	collectionEnsured  bool
}

type qdrantVectorClient interface {
	CollectionExists(ctx context.Context, collectionName string) (bool, error)
	CreateCollection(ctx context.Context, request *qdrant.CreateCollection) error
	Upsert(ctx context.Context, request *qdrant.UpsertPoints) (*qdrant.UpdateResult, error)
	Query(ctx context.Context, request *qdrant.QueryPoints) ([]*qdrant.ScoredPoint, error)
	Close() error
}

func NewMemoryVectorRepository(qdrantURL string, collection string) (*MemoryVectorRepository, error) {
	address := strings.TrimSpace(qdrantURL)
	host, portText, splitErr := net.SplitHostPort(address)
	if splitErr != nil {
		return nil, fmt.Errorf("invalid qdrant url %q: expected host:port", address)
	}
	port, portErr := strconv.Atoi(strings.TrimSpace(portText))
	if portErr != nil || port <= 0 {
		return nil, fmt.Errorf("invalid qdrant url %q: invalid port", address)
	}
	client, err := qdrant.NewClient(&qdrant.Config{
		Host:                   strings.TrimSpace(host),
		Port:                   port,
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		return nil, err
	}
	return &MemoryVectorRepository{
		qdrantURL:  address,
		collection: strings.TrimSpace(collection),
		client:     client,
	}, nil
}

func NewMemoryVectorRepositoryWithClient(client qdrantVectorClient, collection string) *MemoryVectorRepository {
	return &MemoryVectorRepository{
		collection: strings.TrimSpace(collection),
		client:     client,
	}
}

func (r *MemoryVectorRepository) UpsertSessionMemoryVector(ctx context.Context, input domain.MemoryVectorUpsertInput) error {
	if err := r.validateConfig(); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}
	if len(input.Vector) == 0 {
		return fmt.Errorf("vector cannot be empty")
	}
	if err := r.ensureCollection(ctx, len(input.Vector)); err != nil {
		return err
	}

	payload := map[string]any{
		"session_id": sessionID,
	}
	for k, v := range input.Payload {
		payload[k] = v
	}

	payloadValue, err := qdrant.TryValueMap(payload)
	if err != nil {
		return err
	}

	_, err = r.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: r.collection,
		Wait:           qdrant.PtrOf(true),
		Points: []*qdrant.PointStruct{
			{
				Id:      buildPointID(sessionID),
				Vectors: qdrant.NewVectorsDense(input.Vector),
				Payload: payloadValue,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("qdrant upsert failed: %w", err)
	}
	return nil
}

func (r *MemoryVectorRepository) SearchSimilarSessionIDs(ctx context.Context, queryVector []float32, limit int, excludeSessionID string) ([]domain.MemoryVectorSearchHit, error) {
	if err := r.validateConfig(); err != nil {
		return nil, err
	}
	if len(queryVector) == 0 || limit <= 0 {
		return []domain.MemoryVectorSearchHit{}, nil
	}
	if err := r.ensureCollection(ctx, len(queryVector)); err != nil {
		return nil, err
	}

	request := &qdrant.QueryPoints{
		CollectionName: r.collection,
		Query:          qdrant.NewQueryDense(queryVector),
		Limit:          qdrant.PtrOf(uint64(limit)),
		WithPayload:    qdrant.NewWithPayload(true),
	}
	if sessionID := strings.TrimSpace(excludeSessionID); sessionID != "" {
		request.Filter = &qdrant.Filter{
			MustNot: []*qdrant.Condition{
				qdrant.NewMatch("session_id", sessionID),
			},
		}
	}
	results, err := r.client.Query(ctx, request)
	if err != nil {
		return nil, err
	}
	hits := make([]domain.MemoryVectorSearchHit, 0, len(results))
	for _, item := range results {
		sessionID := extractSessionID(item.GetPayload(), item.GetId())
		if strings.TrimSpace(sessionID) == "" {
			continue
		}
		hits = append(hits, domain.MemoryVectorSearchHit{
			SessionID: sessionID,
			Score:     float64(item.GetScore()),
		})
	}
	return hits, nil
}

func (r *MemoryVectorRepository) ensureCollection(ctx context.Context, vectorDim int) error {
	r.ensureCollectionMu.Lock()
	defer r.ensureCollectionMu.Unlock()

	if r.collectionEnsured {
		return nil
	}

	exists, err := r.client.CollectionExists(ctx, r.collection)
	if err != nil {
		return err
	}
	if exists {
		r.collectionEnsured = true
		return nil
	}

	err = r.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: r.collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(vectorDim),
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("qdrant ensure collection failed: %w", err)
	}

	r.collectionEnsured = true
	return nil
}

func (r *MemoryVectorRepository) Close() error {
	if r.client == nil {
		return nil
	}
	return r.client.Close()
}

func (r *MemoryVectorRepository) validateConfig() error {
	if strings.TrimSpace(r.qdrantURL) == "" && r.client == nil {
		return fmt.Errorf("qdrant url cannot be empty")
	}
	if strings.TrimSpace(r.collection) == "" {
		return fmt.Errorf("qdrant collection cannot be empty")
	}
	return nil
}

func extractSessionID(payload map[string]*qdrant.Value, id *qdrant.PointId) string {
	if payload != nil {
		if value, ok := payload["session_id"]; ok && value != nil {
			if text := strings.TrimSpace(value.GetStringValue()); text != "" {
				return text
			}
		}
	}
	if id == nil {
		return ""
	}
	if uuid := strings.TrimSpace(id.GetUuid()); uuid != "" {
		return uuid
	}
	if num := id.GetNum(); num > 0 {
		return strconv.FormatUint(num, 10)
	}
	return ""
}

func buildPointID(sessionID string) *qdrant.PointId {
	trimmed := strings.TrimSpace(sessionID)
	if isUUIDLike(trimmed) {
		return qdrant.NewID(trimmed)
	}
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(trimmed))
	return qdrant.NewIDNum(hasher.Sum64())
}

func isUUIDLike(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
				return false
			}
		}
	}
	return true
}
