package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"slimebot/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func encodeKeywords(keywords []string) string {
	items := normalizeKeywords(keywords)
	if len(items) == 0 {
		return "[]"
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func decodeKeywords(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []string{}
	}
	return normalizeKeywords(items)
}

func (r *Repository) CreateEpisodeMemory(input domain.EpisodeMemoryCreateInput) (*domain.EpisodeMemory, error) {
	item, err := buildEpisodeMemory(input)
	if err != nil {
		return nil, err
	}
	if err := r.db.Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (r *Repository) UpdateEpisodeMemory(input domain.EpisodeMemoryUpdateInput) error {
	id := strings.TrimSpace(input.ID)
	sessionID := strings.TrimSpace(input.SessionID)
	if id == "" || sessionID == "" {
		return fmt.Errorf("id and session_id required")
	}
	updates := map[string]any{
		"topic_key":        strings.TrimSpace(input.TopicKey),
		"title":            strings.TrimSpace(input.Title),
		"summary":          strings.TrimSpace(input.Summary),
		"keywords_json":    encodeKeywords(input.Keywords),
		"state":            normalizeEpisodeState(input.State),
		"source_start_seq": input.SourceStartSeq,
		"source_end_seq":   input.SourceEndSeq,
		"turn_count":       input.TurnCount,
		"last_active_at":   normalizeEpisodeTime(input.LastActiveAt),
		"updated_at":       time.Now(),
	}
	return r.db.Model(&domain.EpisodeMemory{}).
		Where("id = ? AND session_id = ?", id, sessionID).
		Updates(updates).Error
}

func (r *Repository) GetOpenEpisodeMemory(ctx context.Context, sessionID string) (*domain.EpisodeMemory, error) {
	var row domain.EpisodeMemory
	err := r.dbWithContext(ctx).
		Where("session_id = ? AND state = ?", strings.TrimSpace(sessionID), domain.EpisodeMemoryStateOpen).
		Order("updated_at DESC").
		First(&row).Error
	if err != nil {
		if isRecordNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repository) GetLatestClosedEpisodeByTopicKey(ctx context.Context, sessionID, topicKey string) (*domain.EpisodeMemory, error) {
	var row domain.EpisodeMemory
	err := r.dbWithContext(ctx).
		Where("session_id = ? AND topic_key = ? AND state = ?", strings.TrimSpace(sessionID), strings.TrimSpace(topicKey), domain.EpisodeMemoryStateClosed).
		Order("last_active_at DESC, updated_at DESC").
		First(&row).Error
	if err != nil {
		if isRecordNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repository) GetEpisodeMemoriesByIDs(ids []string) ([]domain.EpisodeMemory, error) {
	if len(ids) == 0 {
		return []domain.EpisodeMemory{}, nil
	}
	var rows []domain.EpisodeMemory
	if err := r.db.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) SearchEpisodeMemories(ctx context.Context, input domain.EpisodeMemorySearchInput) ([]domain.EpisodeMemorySearchHit, error) {
	terms := normalizeKeywords(splitQueryTerms(input.Query))
	if len(terms) == 0 || input.Limit <= 0 {
		return []domain.EpisodeMemorySearchHit{}, nil
	}
	now := normalizeNow(input.Now)
	candidateLimit := input.Limit * 10
	if candidateLimit < 50 {
		candidateLimit = 50
	}

	var rows []domain.EpisodeMemory
	query := r.dbWithContext(ctx).
		Where("state <> ?", domain.EpisodeMemoryStateArchived).
		Order("last_active_at DESC").
		Limit(candidateLimit)
	if sessionID := strings.TrimSpace(input.SessionID); sessionID != "" {
		query = query.Where("session_id = ?", sessionID)
	}
	if input.ExcludeStartSeq > 0 && input.ExcludeEndSeq > 0 && input.ExcludeStartSeq <= input.ExcludeEndSeq {
		query = query.Where("NOT (source_end_seq >= ? AND source_start_seq <= ?)", input.ExcludeStartSeq, input.ExcludeEndSeq)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}

	hits := make([]domain.EpisodeMemorySearchHit, 0, len(rows))
	for _, row := range rows {
		matched := matchEpisodeKeywords(terms, row)
		if len(matched) == 0 {
			continue
		}
		hits = append(hits, domain.EpisodeMemorySearchHit{
			Episode:         row,
			MatchedKeywords: matched,
			Score:           scoreEpisodeHit(row, matched, now),
		})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Episode.LastActiveAt.After(hits[j].Episode.LastActiveAt)
		}
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > input.Limit {
		hits = hits[:input.Limit]
	}
	return hits, nil
}

func (r *Repository) UpsertStickyMemory(input domain.StickyMemoryUpsertInput) (*domain.StickyMemory, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	kind := strings.TrimSpace(strings.ToLower(input.Kind))
	key := strings.TrimSpace(strings.ToLower(input.Key))
	if sessionID == "" || kind == "" || key == "" {
		return nil, fmt.Errorf("session_id, kind and key required")
	}
	var existing domain.StickyMemory
	err := r.db.Where("session_id = ? AND kind = ? AND key = ?", sessionID, kind, key).First(&existing).Error
	if err != nil && !isRecordNotFound(err) {
		return nil, err
	}
	now := time.Now()
	if isRecordNotFound(err) {
		row := &domain.StickyMemory{
			ID:             uuid.NewString(),
			SessionID:      sessionID,
			Kind:           kind,
			Key:            key,
			Value:          strings.TrimSpace(input.Value),
			Summary:        strings.TrimSpace(input.Summary),
			Confidence:     clampConfidence(input.Confidence),
			Status:         domain.StickyMemoryStatusActive,
			SourceStartSeq: input.SourceStartSeq,
			SourceEndSeq:   input.SourceEndSeq,
			LastSeenAt:     normalizeFactTime(input.LastSeenAt),
			ExpiresAt:      input.ExpiresAt,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := r.db.Create(row).Error; err != nil {
			return nil, err
		}
		return row, nil
	}

	updates := map[string]any{
		"value":            strings.TrimSpace(input.Value),
		"summary":          strings.TrimSpace(input.Summary),
		"confidence":       clampConfidence(input.Confidence),
		"status":           domain.StickyMemoryStatusActive,
		"source_start_seq": input.SourceStartSeq,
		"source_end_seq":   input.SourceEndSeq,
		"last_seen_at":     normalizeFactTime(input.LastSeenAt),
		"expires_at":       input.ExpiresAt,
		"updated_at":       now,
	}
	if err := r.db.Model(&domain.StickyMemory{}).
		Where("id = ?", existing.ID).
		Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := r.db.Where("id = ?", existing.ID).First(&existing).Error; err != nil {
		return nil, err
	}
	return &existing, nil
}

func (r *Repository) DeleteStickyMemory(ctx context.Context, sessionID, kind, key string) error {
	return r.dbWithContext(ctx).
		Model(&domain.StickyMemory{}).
		Where("session_id = ? AND kind = ? AND key = ?", strings.TrimSpace(sessionID), strings.TrimSpace(strings.ToLower(kind)), strings.TrimSpace(strings.ToLower(key))).
		Updates(map[string]any{"status": domain.StickyMemoryStatusDeleted, "updated_at": time.Now()}).Error
}

func (r *Repository) ListStickyMemoriesForPrompt(ctx context.Context, sessionID string, limit int, now time.Time) ([]domain.StickyMemory, error) {
	if limit <= 0 {
		return []domain.StickyMemory{}, nil
	}
	var rows []domain.StickyMemory
	err := r.dbWithContext(ctx).
		Where("session_id = ? AND status = ?", strings.TrimSpace(sessionID), domain.StickyMemoryStatusActive).
		Where("(expires_at IS NULL OR expires_at > ?)", normalizeNow(now)).
		Order("updated_at DESC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *Repository) SearchStickyMemories(ctx context.Context, sessionID, query string, limit int, now time.Time) ([]domain.StickyMemorySearchHit, error) {
	terms := normalizeKeywords(splitQueryTerms(query))
	if len(terms) == 0 || limit <= 0 {
		return []domain.StickyMemorySearchHit{}, nil
	}
	rows, err := r.listStickyMemories(ctx, sessionID, limit*3, now)
	if err != nil {
		return nil, err
	}
	hits := make([]domain.StickyMemorySearchHit, 0, len(rows))
	for _, row := range rows {
		matched := matchStickyKeywords(terms, row)
		if len(matched) == 0 {
			continue
		}
		hits = append(hits, domain.StickyMemorySearchHit{
			Memory:          row,
			MatchedKeywords: matched,
			Score:           float64(len(matched))*100 + row.Confidence*100,
		})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Memory.UpdatedAt.After(hits[j].Memory.UpdatedAt)
		}
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func buildEpisodeMemory(input domain.EpisodeMemoryCreateInput) (*domain.EpisodeMemory, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	topicKey := strings.TrimSpace(input.TopicKey)
	title := strings.TrimSpace(input.Title)
	summary := strings.TrimSpace(input.Summary)
	if sessionID == "" || topicKey == "" || title == "" || summary == "" {
		return nil, fmt.Errorf("episode memory requires session_id, topic_key, title and summary")
	}
	now := time.Now()
	return &domain.EpisodeMemory{
		ID:             uuid.NewString(),
		SessionID:      sessionID,
		TopicKey:       topicKey,
		Title:          title,
		Summary:        summary,
		KeywordsJSON:   encodeKeywords(input.Keywords),
		State:          normalizeEpisodeState(input.State),
		SourceStartSeq: input.SourceStartSeq,
		SourceEndSeq:   input.SourceEndSeq,
		TurnCount:      input.TurnCount,
		LastActiveAt:   normalizeEpisodeTime(input.LastActiveAt),
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func normalizeEpisodeState(state string) string {
	switch strings.TrimSpace(state) {
	case domain.EpisodeMemoryStateOpen, domain.EpisodeMemoryStateClosed, domain.EpisodeMemoryStateArchived:
		return strings.TrimSpace(state)
	default:
		return domain.EpisodeMemoryStateOpen
	}
}

func normalizeEpisodeTime(v time.Time) time.Time {
	if v.IsZero() {
		return time.Now()
	}
	return v
}

func episodeTerms(item domain.EpisodeMemory) []string {
	return normalizeKeywords(append(
		[]string{item.TopicKey, item.Title, item.Summary},
		decodeKeywords(item.KeywordsJSON)...,
	))
}

func matchEpisodeKeywords(queries []string, item domain.EpisodeMemory) []string {
	text := strings.ToLower(strings.Join(episodeTerms(item), " "))
	result := make([]string, 0, len(queries))
	for _, query := range queries {
		if strings.Contains(text, query) {
			result = append(result, query)
		}
	}
	return result
}

func scoreEpisodeHit(item domain.EpisodeMemory, matched []string, now time.Time) float64 {
	score := float64(len(matched)) * 100
	if item.State == domain.EpisodeMemoryStateOpen {
		score += 30
	}
	ageHours := normalizeNow(now).Sub(item.LastActiveAt).Hours()
	switch {
	case ageHours <= 24:
		score += 25
	case ageHours <= 24*7:
		score += 10
	}
	return score
}

func matchStickyKeywords(queries []string, item domain.StickyMemory) []string {
	text := strings.ToLower(strings.Join([]string{item.Kind, item.Key, item.Value, item.Summary}, " "))
	result := make([]string, 0, len(queries))
	for _, query := range queries {
		if strings.Contains(text, query) {
			result = append(result, query)
		}
	}
	return result
}

func (r *Repository) GetMessageByID(ctx context.Context, id string) (*domain.Message, error) {
	var item domain.Message
	err := r.dbWithContext(ctx).Where("id = ?", strings.TrimSpace(id)).First(&item).Error
	if err != nil {
		if isRecordNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	item.Attachments = decodeMessageAttachments(item.AttachmentsJSON)
	return &item, nil
}

func isRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func normalizeKeywords(keywords []string) []string {
	seen := make(map[string]struct{}, len(keywords))
	result := make([]string, 0, len(keywords))
	for _, item := range keywords {
		normalized := strings.ToLower(strings.TrimSpace(item))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func splitQueryTerms(query string) []string {
	return strings.Fields(strings.NewReplacer("\n", " ", "\t", " ", ",", " ", "，", " ").Replace(query))
}

func normalizeNow(v time.Time) time.Time {
	if v.IsZero() {
		return time.Now()
	}
	return v
}

func clampConfidence(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func normalizeFactTime(v time.Time) time.Time {
	if v.IsZero() {
		return time.Now()
	}
	return v
}

func (r *Repository) listStickyMemories(ctx context.Context, sessionID string, limit int, now time.Time) ([]domain.StickyMemory, error) {
	if limit <= 0 {
		return []domain.StickyMemory{}, nil
	}
	query := r.dbWithContext(ctx).
		Where("status = ?", domain.StickyMemoryStatusActive).
		Where("(expires_at IS NULL OR expires_at > ?)", normalizeNow(now)).
		Order("updated_at DESC").
		Limit(limit)
	if trimmed := strings.TrimSpace(sessionID); trimmed != "" {
		query = query.Where("session_id = ?", trimmed)
	}
	var rows []domain.StickyMemory
	err := query.Find(&rows).Error
	return rows, err
}
