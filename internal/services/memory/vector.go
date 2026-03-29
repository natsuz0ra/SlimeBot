package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

func upsertEpisodeVector(m *MemoryService, ctx context.Context, item *domain.EpisodeMemory) error {
	if item == nil {
		return nil
	}
	vector, err := m.embedding.Embed(ctx, strings.TrimSpace(item.Title+"\n"+item.Summary+"\n"+strings.Join(decodeKeywordsJSON(item.KeywordsJSON), " ")))
	if err != nil {
		return err
	}
	return m.vectorStore.UpsertSessionMemoryVector(ctx, domain.MemoryVectorUpsertInput{
		MemoryID:  item.ID,
		SessionID: item.SessionID,
		Vector:    vector,
		Payload: map[string]any{
			"title":     item.Title,
			"topic_key": item.TopicKey,
			"kind":      "episode",
		},
	})
}

func (m *MemoryService) RetrieveRelevantEpisodes(ctx context.Context, sessionID, query string, excludeStartSeq, excludeEndSeq int64, limit int) ([]domain.EpisodeMemorySearchHit, error) {
	if limit <= 0 {
		limit = constants.MemorySearchTopK
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return []domain.EpisodeMemorySearchHit{}, nil
	}
	queryText := q
	if keywords := m.TokenizeKeywords(q); len(keywords) > 0 {
		queryText = strings.Join(keywords, " ")
	}

	if m.embedding != nil && m.vectorStore != nil {
		hits, err := m.retrieveEpisodesByVector(ctx, sessionID, queryText, excludeStartSeq, excludeEndSeq, limit)
		if err == nil && len(hits) > 0 {
			return hits, nil
		}
		if err != nil {
			slog.Warn("memory_vector_retrieve_fallback", "session", sessionID, "err", err)
		}
	}

	return m.store.SearchEpisodeMemories(ctx, domain.EpisodeMemorySearchInput{
		SessionID:       sessionID,
		Query:           queryText,
		Limit:           limit,
		ExcludeStartSeq: excludeStartSeq,
		ExcludeEndSeq:   excludeEndSeq,
		Now:             time.Now(),
	})
}

func (m *MemoryService) retrieveEpisodesByVector(ctx context.Context, sessionID, query string, excludeStartSeq, excludeEndSeq int64, limit int) ([]domain.EpisodeMemorySearchHit, error) {
	vector, err := m.embedding.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	searchLimit := limit
	if m.vectorTopK > searchLimit {
		searchLimit = m.vectorTopK
	}
	vectorHits, err := m.vectorStore.SearchMemoriesInSession(ctx, vector, sessionID, searchLimit)
	if err != nil {
		return nil, err
	}
	if len(vectorHits) == 0 {
		return []domain.EpisodeMemorySearchHit{}, nil
	}
	ids := make([]string, 0, len(vectorHits))
	for _, hit := range vectorHits {
		if strings.TrimSpace(hit.MemoryID) != "" {
			ids = append(ids, hit.MemoryID)
		}
	}
	episodes, err := m.store.GetEpisodeMemoriesByIDs(ids)
	if err != nil {
		return nil, err
	}
	episodeByID := make(map[string]domain.EpisodeMemory, len(episodes))
	for _, item := range episodes {
		episodeByID[item.ID] = item
	}
	queryKeywords := m.TokenizeKeywords(query)
	result := make([]domain.EpisodeMemorySearchHit, 0, len(vectorHits))
	for _, hit := range vectorHits {
		item, ok := episodeByID[hit.MemoryID]
		if !ok || item.State == domain.EpisodeMemoryStateArchived {
			continue
		}
		if excludeStartSeq > 0 && excludeEndSeq > 0 && item.SourceEndSeq >= excludeStartSeq && item.SourceStartSeq <= excludeEndSeq {
			continue
		}
		score := float64(hit.Score) + similarityFromKeywords(decodeKeywordsJSON(item.KeywordsJSON), queryKeywords)*100
		result = append(result, domain.EpisodeMemorySearchHit{
			Episode:         item,
			MatchedKeywords: matchEpisodeKeywordsLocal(queryKeywords, item),
			Score:           score,
		})
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (m *MemoryService) buildMemoryContext(ctx context.Context, sessionID string, history []domain.Message) string {
	sticky, err := m.store.ListStickyMemoriesForPrompt(ctx, sessionID, 10, time.Now())
	if err != nil {
		return ""
	}
	query, startSeq, endSeq := buildEpisodeQueryFromHistory(history)
	episodes, err := m.RetrieveRelevantEpisodes(ctx, sessionID, query, startSeq, endSeq, constants.MemoryContextTopK)
	if err != nil {
		return formatMemoryContext(sticky, nil)
	}
	return formatMemoryContext(sticky, episodes)
}

func buildEpisodeQueryFromHistory(history []domain.Message) (string, int64, int64) {
	var parts []string
	var startSeq int64
	var endSeq int64
	for _, item := range history {
		if startSeq == 0 || (item.Seq > 0 && item.Seq < startSeq) {
			startSeq = item.Seq
		}
		if item.Seq > endSeq {
			endSeq = item.Seq
		}
		if item.Role == "user" && strings.TrimSpace(item.Content) != "" {
			parts = append(parts, strings.TrimSpace(item.Content))
		}
	}
	if len(parts) == 0 {
		for _, item := range history {
			if strings.TrimSpace(item.Content) != "" {
				parts = append(parts, strings.TrimSpace(item.Content))
			}
		}
	}
	if len(parts) > 2 {
		parts = parts[len(parts)-2:]
	}
	return strings.Join(parts, "\n"), startSeq, endSeq
}

func formatMemoryContext(sticky []domain.StickyMemory, episodes []domain.EpisodeMemorySearchHit) string {
	var b strings.Builder
	if len(sticky) > 0 {
		b.WriteString("<sticky_memories>\n")
		for _, item := range sticky {
			b.WriteString(fmt.Sprintf("  <memory kind=\"%s\" key=\"%s\" confidence=\"%.2f\">%s</memory>\n", item.Kind, item.Key, item.Confidence, strings.TrimSpace(item.Summary)))
		}
		b.WriteString("</sticky_memories>\n")
	}
	if len(episodes) > 0 {
		b.WriteString("<episode_memories>\n")
		for _, item := range episodes {
			keywords := decodeKeywordsJSON(item.Episode.KeywordsJSON)
			b.WriteString(fmt.Sprintf("  <episode id=\"%s\" topic_key=\"%s\" title=\"%s\" state=\"%s\" range=\"%d-%d\">%s | keywords: %s</episode>\n",
				item.Episode.ID,
				item.Episode.TopicKey,
				item.Episode.Title,
				item.Episode.State,
				item.Episode.SourceStartSeq,
				item.Episode.SourceEndSeq,
				strings.TrimSpace(item.Episode.Summary),
				strings.Join(keywords, ", "),
			))
		}
		b.WriteString("</episode_memories>")
	}
	return strings.TrimSpace(b.String())
}

func matchEpisodeKeywordsLocal(queries []string, item domain.EpisodeMemory) []string {
	text := strings.ToLower(strings.Join(append([]string{item.TopicKey, item.Title, item.Summary}, decodeKeywordsJSON(item.KeywordsJSON)...), " "))
	result := make([]string, 0, len(queries))
	for _, query := range queries {
		if strings.Contains(text, strings.ToLower(strings.TrimSpace(query))) {
			result = append(result, query)
		}
	}
	return result
}
