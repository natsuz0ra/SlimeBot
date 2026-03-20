package memory

import (
	"context"
	"log"
	"strings"

	"slimebot/backend/internal/domain"
)

func retrieveMemoriesByVectorImpl(m *MemoryService, keywords []string, excludeSessionID string, limit int) ([]domain.SessionMemorySearchHit, error) {
	if len(keywords) == 0 {
		return []domain.SessionMemorySearchHit{}, nil
	}
	query := strings.Join(keywords, " ")
	queryVector, err := m.embedding.Embed(context.Background(), query)
	if err != nil {
		log.Printf(
			"memory_vector_query_embedding_failed keyword_count=%d exclude_session=%s err=%v",
			len(keywords),
			strings.TrimSpace(excludeSessionID),
			err,
		)
		return nil, err
	}
	log.Printf(
		"memory_vector_query_embedding_succeeded keyword_count=%d vector_dim=%d exclude_session=%s",
		len(keywords),
		len(queryVector),
		strings.TrimSpace(excludeSessionID),
	)

	searchLimit := limit
	if m.vectorTopK > searchLimit {
		searchLimit = m.vectorTopK
	}
	vectorHits, err := m.vectorStore.SearchSimilarSessionIDs(context.Background(), queryVector, searchLimit, excludeSessionID)
	if err != nil {
		log.Printf(
			"memory_vector_query_failed keyword_count=%d search_limit=%d exclude_session=%s err=%v",
			len(keywords),
			searchLimit,
			strings.TrimSpace(excludeSessionID),
			err,
		)
		return nil, err
	}
	if len(vectorHits) == 0 {
		log.Printf(
			"memory_vector_query_no_hit keyword_count=%d search_limit=%d exclude_session=%s",
			len(keywords),
			searchLimit,
			strings.TrimSpace(excludeSessionID),
		)
		return []domain.SessionMemorySearchHit{}, nil
	}
	log.Printf(
		"memory_vector_query_hit keyword_count=%d search_limit=%d raw_hit_count=%d exclude_session=%s",
		len(keywords),
		searchLimit,
		len(vectorHits),
		strings.TrimSpace(excludeSessionID),
	)

	sessionIDs := make([]string, 0, len(vectorHits))
	for _, hit := range vectorHits {
		sessionIDs = append(sessionIDs, hit.SessionID)
	}
	memories, err := m.store.GetSessionMemoriesBySessionIDs(sessionIDs)
	if err != nil {
		return nil, err
	}
	memoryBySessionID := make(map[string]domain.SessionMemory, len(memories))
	for _, item := range memories {
		memoryBySessionID[item.SessionID] = item
	}

	results := make([]domain.SessionMemorySearchHit, 0, len(vectorHits))
	for _, hit := range vectorHits {
		memory, ok := memoryBySessionID[hit.SessionID]
		if !ok {
			continue
		}
		matched := intersectKeywordSlicesImpl(keywords, m.TokenizeKeywords(memory.KeywordsText+" "+memory.Summary))
		results = append(results, domain.SessionMemorySearchHit{
			Memory:          memory,
			MatchedKeywords: matched,
			Score:           hit.Score,
		})
		if len(results) >= limit {
			break
		}
	}
	log.Printf(
		"memory_vector_query_resolved keyword_count=%d resolved_hit_count=%d limit=%d",
		len(keywords),
		len(results),
		limit,
	)
	return results, nil
}

func upsertSessionMemoryVectorImpl(m *MemoryService, ctx context.Context, sessionID string, summary string, keywords []string, messageCount int) error {
	vector, err := m.embedding.Embed(ctx, summary)
	if err != nil {
		log.Printf(
			"memory_vector_generate_failed session=%s summary_len=%d keyword_count=%d err=%v",
			strings.TrimSpace(sessionID),
			len(strings.TrimSpace(summary)),
			len(keywords),
			err,
		)
		return err
	}
	log.Printf(
		"memory_vector_generate_succeeded session=%s summary_len=%d keyword_count=%d vector_dim=%d",
		strings.TrimSpace(sessionID),
		len(strings.TrimSpace(summary)),
		len(keywords),
		len(vector),
	)
	payload := map[string]any{
		"summary":              summary,
		"source_message_count": messageCount,
	}
	if len(keywords) > 0 {
		keywordValues := make([]any, 0, len(keywords))
		for _, item := range keywords {
			keywordValues = append(keywordValues, item)
		}
		payload["keywords"] = keywordValues
	}
	if err := m.vectorStore.UpsertSessionMemoryVector(ctx, domain.MemoryVectorUpsertInput{
		SessionID: sessionID,
		Vector:    vector,
		Payload:   payload,
	}); err != nil {
		log.Printf(
			"memory_vector_upsert_failed session=%s vector_dim=%d err=%v",
			strings.TrimSpace(sessionID),
			len(vector),
			err,
		)
		return err
	}
	log.Printf(
		"memory_vector_upsert_succeeded session=%s vector_dim=%d",
		strings.TrimSpace(sessionID),
		len(vector),
	)
	return nil
}

func intersectKeywordSlicesImpl(left []string, right []string) []string {
	if len(left) == 0 || len(right) == 0 {
		return []string{}
	}
	rightSet := make(map[string]struct{}, len(right))
	for _, item := range right {
		rightSet[item] = struct{}{}
	}
	result := make([]string, 0, len(left))
	for _, item := range left {
		if _, ok := rightSet[item]; ok {
			result = append(result, item)
		}
	}
	return result
}
