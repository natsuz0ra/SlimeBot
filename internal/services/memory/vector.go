package memory

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

// retrieveMemoriesByVectorImpl 通过向量检索命中会话 ID，再回查并拼装记忆结果。
func retrieveMemoriesByVectorImpl(m *MemoryService, ctx context.Context, query string, excludeSessionID string, limit int) ([]domain.SessionMemorySearchHit, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return []domain.SessionMemorySearchHit{}, nil
	}
	queryKeywords := m.TokenizeKeywords(q)
	// 先生成查询向量，失败直接返回错误。
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

	// 搜索上限取请求 limit 与配置 topK 的较大值。
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
		// 无命中直接返回空结果，便于上层回退关键词检索。
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

	sessionLatest := make(map[string]string)
	sessionIDsNeedingLookup := make([]string, 0, len(vectorHits))
	sessionSeen := make(map[string]struct{}, len(vectorHits))
	for _, hit := range vectorHits {
		sid := strings.TrimSpace(hit.SessionID)
		if sid == "" || strings.TrimSpace(hit.MemoryID) != "" {
			continue
		}
		if _, ok := sessionSeen[sid]; ok {
			continue
		}
		sessionSeen[sid] = struct{}{}
		sessionIDsNeedingLookup = append(sessionIDsNeedingLookup, sid)
	}
	if len(sessionIDsNeedingLookup) > 0 {
		rows, rerr := m.store.GetSessionMemoriesBySessionIDs(sessionIDsNeedingLookup)
		if rerr != nil {
			return nil, rerr
		}
		latestBySession := make(map[string]domain.SessionMemory, len(rows))
		for _, row := range rows {
			sid := strings.TrimSpace(row.SessionID)
			if sid == "" {
				continue
			}
			existing, ok := latestBySession[sid]
			if !ok || row.UpdatedAt.After(existing.UpdatedAt) {
				latestBySession[sid] = row
			}
		}
		for sid, row := range latestBySession {
			sessionLatest[sid] = row.ID
		}
	}
	memoryIDs := make([]string, 0, len(vectorHits))
	for _, hit := range vectorHits {
		if id := strings.TrimSpace(hit.MemoryID); id != "" {
			memoryIDs = append(memoryIDs, id)
			continue
		}
		sid := strings.TrimSpace(hit.SessionID)
		if id, ok := sessionLatest[sid]; ok {
			memoryIDs = append(memoryIDs, id)
		}
	}
	uniqIDs := make([]string, 0, len(memoryIDs))
	seen := make(map[string]struct{}, len(memoryIDs))
	for _, id := range memoryIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqIDs = append(uniqIDs, id)
	}
	memories, err := m.store.GetSessionMemoriesByIDs(uniqIDs)
	if err != nil {
		return nil, err
	}
	memoryByID := make(map[string]domain.SessionMemory, len(memories))
	for _, item := range memories {
		memoryByID[item.ID] = item
	}

	// 按向量命中顺序拼装结果，并补充关键词匹配信息。
	results := make([]domain.SessionMemorySearchHit, 0, len(vectorHits))
	for _, hit := range vectorHits {
		lookupID := strings.TrimSpace(hit.MemoryID)
		if lookupID == "" {
			sid := strings.TrimSpace(hit.SessionID)
			if sid == "" {
				continue
			}
			var ok bool
			lookupID, ok = sessionLatest[sid]
			if !ok || lookupID == "" {
				continue
			}
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

// upsertMemoryVector 将摘要向量写入向量库并附带关键词等 payload。
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

// buildSearchQuery 将近期消息拼成检索 query，并限制 rune 数量。
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

// filterVectorHitsByScore 过滤低分命中并按分值降序去重。
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

// orderedMemoriesFromVectorHits 按向量命中顺序重排记忆。
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

// mergeMemoriesByID 以 primary 为先去重合并两组记忆。
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

// buildSessionMemoryContextForPrompt 生成会话记忆上下文：小量全量注入，超限时走向量筛选。
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
		// 记忆数量较少时全量注入。
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
	// 过滤后按向量排序获取正文，并合并最近记忆作补充。
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
