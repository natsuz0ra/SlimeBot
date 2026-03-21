package memory

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

func retrieveMemoriesByVectorImpl(m *MemoryService, ctx context.Context, query string, excludeSessionID string, limit int) ([]domain.SessionMemorySearchHit, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return []domain.SessionMemorySearchHit{}, nil
	}
	queryKeywords := m.TokenizeKeywords(q)
	queryVector, err := m.embedding.Embed(ctx, q)
	if err != nil {
		slog.Warn("memory_vector_query_embedding_failed",
			"keyword_count", len(queryKeywords),
			"exclude_session", strings.TrimSpace(excludeSessionID),
			"err", err,
		)
		return nil, err
	}
	slog.Info("memory_vector_query_embedding_succeeded",
		"keyword_count", len(queryKeywords),
		"vector_dim", len(queryVector),
		"exclude_session", strings.TrimSpace(excludeSessionID),
	)

	searchLimit := limit
	if m.vectorTopK > searchLimit {
		searchLimit = m.vectorTopK
	}
	vectorHits, err := m.vectorStore.SearchSimilarSessionIDs(ctx, queryVector, searchLimit, excludeSessionID)
	if err != nil {
		slog.Warn("memory_vector_query_failed",
			"keyword_count", len(queryKeywords),
			"search_limit", searchLimit,
			"exclude_session", strings.TrimSpace(excludeSessionID),
			"err", err,
		)
		return nil, err
	}
	if len(vectorHits) == 0 {
		slog.Info("memory_vector_query_no_hit",
			"keyword_count", len(queryKeywords),
			"search_limit", searchLimit,
			"exclude_session", strings.TrimSpace(excludeSessionID),
		)
		return []domain.SessionMemorySearchHit{}, nil
	}
	slog.Info("memory_vector_query_hit",
		"keyword_count", len(queryKeywords),
		"search_limit", searchLimit,
		"raw_hit_count", len(vectorHits),
		"exclude_session", strings.TrimSpace(excludeSessionID),
	)

	memoryIDs := make([]string, 0, len(vectorHits))
	for _, hit := range vectorHits {
		if strings.TrimSpace(hit.MemoryID) != "" {
			memoryIDs = append(memoryIDs, hit.MemoryID)
			continue
		}
		if strings.TrimSpace(hit.SessionID) != "" {
			rows, rerr := m.store.ListRecentActiveSessionMemories(hit.SessionID, 1)
			if rerr == nil && len(rows) > 0 {
				memoryIDs = append(memoryIDs, rows[0].ID)
			}
		}
	}
	memories, err := m.store.GetSessionMemoriesByIDs(memoryIDs)
	if err != nil {
		return nil, err
	}
	memoryByID := make(map[string]domain.SessionMemory, len(memories))
	for _, item := range memories {
		memoryByID[item.ID] = item
	}

	results := make([]domain.SessionMemorySearchHit, 0, len(vectorHits))
	for _, hit := range vectorHits {
		lookupID := strings.TrimSpace(hit.MemoryID)
		if lookupID == "" {
			rows, rerr := m.store.ListRecentActiveSessionMemories(hit.SessionID, 1)
			if rerr != nil || len(rows) == 0 {
				continue
			}
			lookupID = rows[0].ID
		}
		memory, ok := memoryByID[lookupID]
		if !ok {
			continue
		}
		matched := intersectKeywordSlicesImpl(queryKeywords, m.TokenizeKeywords(memory.KeywordsText+" "+memory.Summary))
		results = append(results, domain.SessionMemorySearchHit{
			Memory:          memory,
			MatchedKeywords: matched,
			Score:           hit.Score,
		})
		if len(results) >= limit {
			break
		}
	}
	slog.Info("memory_vector_query_resolved",
		"keyword_count", len(queryKeywords),
		"resolved_hit_count", len(results),
		"limit", limit,
	)
	return results, nil
}

func upsertMemoryVector(m *MemoryService, ctx context.Context, memoryID, sessionID, summary string, keywords []string, messageCount int) error {
	vector, err := m.embedding.Embed(ctx, summary)
	if err != nil {
		slog.Warn("memory_vector_generate_failed",
			"memory_id", strings.TrimSpace(memoryID),
			"session", strings.TrimSpace(sessionID),
			"summary_len", len(strings.TrimSpace(summary)),
			"keyword_count", len(keywords),
			"err", err,
		)
		return err
	}
	slog.Info("memory_vector_generate_succeeded",
		"memory_id", strings.TrimSpace(memoryID),
		"session", strings.TrimSpace(sessionID),
		"summary_len", len(strings.TrimSpace(summary)),
		"keyword_count", len(keywords),
		"vector_dim", len(vector),
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
		MemoryID:  memoryID,
		SessionID: sessionID,
		Vector:    vector,
		Payload:   payload,
	}); err != nil {
		slog.Warn("memory_vector_upsert_failed",
			"memory_id", strings.TrimSpace(memoryID),
			"session", strings.TrimSpace(sessionID),
			"vector_dim", len(vector),
			"err", err,
		)
		return err
	}
	slog.Info("memory_vector_upsert_succeeded",
		"memory_id", strings.TrimSpace(memoryID),
		"session", strings.TrimSpace(sessionID),
		"vector_dim", len(vector),
	)
	return nil
}

func (m *MemoryService) retrieveMemoriesByVector(ctx context.Context, query string, excludeSessionID string, limit int) ([]domain.SessionMemorySearchHit, error) {
	return retrieveMemoriesByVectorImpl(m, ctx, query, excludeSessionID, limit)
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

func buildSearchQuery(history []domain.Message, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	var parts []string
	runeCount := 0
	for i := len(history) - 1; i >= 0 && runeCount < maxRunes; i-- {
		text := strings.TrimSpace(history[i].Content)
		if text == "" {
			continue
		}
		textRunes := []rune(text)
		if runeCount+len(textRunes) > maxRunes {
			textRunes = textRunes[:maxRunes-runeCount]
			text = string(textRunes)
		}
		parts = append([]string{text}, parts...)
		runeCount += len(textRunes)
	}
	return strings.Join(parts, "\n")
}

func filterVectorHitsByScore(hits []domain.MemoryVectorSearchHit, minScore float64) []domain.MemoryVectorSearchHit {
	if len(hits) == 0 {
		return nil
	}
	out := make([]domain.MemoryVectorSearchHit, 0, len(hits))
	seen := make(map[string]struct{}, len(hits))
	for _, h := range hits {
		if h.Score < minScore {
			continue
		}
		id := strings.TrimSpace(h.MemoryID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	return out
}

func orderedMemoriesFromVectorHits(hits []domain.MemoryVectorSearchHit, mems []domain.SessionMemory) []domain.SessionMemory {
	byID := make(map[string]domain.SessionMemory, len(mems))
	for _, m := range mems {
		byID[m.ID] = m
	}
	out := make([]domain.SessionMemory, 0, len(hits))
	for _, h := range hits {
		id := strings.TrimSpace(h.MemoryID)
		if id == "" {
			continue
		}
		m, ok := byID[id]
		if !ok {
			continue
		}
		out = append(out, m)
	}
	return out
}

func mergeMemoriesByID(primary []domain.SessionMemory, extra []domain.SessionMemory) []domain.SessionMemory {
	seen := make(map[string]struct{}, len(primary)+len(extra))
	out := make([]domain.SessionMemory, 0, len(primary)+len(extra))
	for _, m := range primary {
		if _, ok := seen[m.ID]; ok {
			continue
		}
		seen[m.ID] = struct{}{}
		out = append(out, m)
	}
	for _, m := range extra {
		if _, ok := seen[m.ID]; ok {
			continue
		}
		seen[m.ID] = struct{}{}
		out = append(out, m)
	}
	return out
}

func (m *MemoryService) buildSessionMemoryContextForPrompt(ctx context.Context, sessionID string, history []domain.Message) string {
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return ""
	}
	n, err := m.store.CountActiveSessionMemories(sid)
	if err != nil || n == 0 {
		return ""
	}
	fallback := func() string {
		recent, _ := m.store.ListRecentActiveSessionMemories(sid, constants.MemoryRecentFallback)
		return FormatMemoriesListXMLWithBudget(recent, constants.MemoryContextMaxRunes)
	}
	if n <= int64(constants.MemoryFullInjectThreshold) {
		all, lerr := m.store.ListActiveSessionMemories(sid)
		if lerr != nil || len(all) == 0 {
			return ""
		}
		return FormatMemoriesListXMLWithBudget(all, constants.MemoryContextMaxRunes)
	}
	if m.embedding == nil || m.vectorStore == nil {
		return fallback()
	}
	q := buildSearchQuery(history, constants.MemorySearchQueryMaxRunes)
	if q == "" {
		return fallback()
	}
	vec, err := m.embedding.Embed(ctx, q)
	if err != nil {
		return fallback()
	}
	hits, err := m.vectorStore.SearchMemoriesInSession(ctx, vec, sid, constants.MemoryContextTopK)
	if err != nil || len(hits) == 0 {
		return fallback()
	}
	filtered := filterVectorHitsByScore(hits, constants.MemoryVectorScoreThreshold)
	if len(filtered) == 0 {
		return fallback()
	}
	ids := make([]string, 0, len(filtered))
	for _, h := range filtered {
		if id := strings.TrimSpace(h.MemoryID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return fallback()
	}
	vecMem, err := m.store.GetSessionMemoriesByIDs(ids)
	if err != nil {
		return fallback()
	}
	ordered := orderedMemoriesFromVectorHits(filtered, vecMem)
	recent, _ := m.store.ListRecentActiveSessionMemories(sid, constants.MemoryRecentFallback)
	merged := mergeMemoriesByID(ordered, recent)
	return FormatMemoriesListXMLWithBudget(merged, constants.MemoryContextMaxRunes)
}
