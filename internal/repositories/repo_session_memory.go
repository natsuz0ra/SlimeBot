package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *Repository) GetSessionMemory(ctx context.Context, sessionID string) (*domain.SessionMemory, error) {
	var item domain.SessionMemory
	err := r.dbWithContext(ctx).Where("session_id = ? AND is_active = ?", strings.TrimSpace(sessionID), true).
		Order("updated_at DESC").
		First(&item).Error
	if err != nil {
		if isRecordNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) GetSessionMemoriesBySessionIDs(sessionIDs []string) ([]domain.SessionMemory, error) {
	if len(sessionIDs) == 0 {
		return []domain.SessionMemory{}, nil
	}
	normalized := make([]string, 0, len(sessionIDs))
	for _, item := range sessionIDs {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		normalized = append(normalized, v)
	}
	if len(normalized) == 0 {
		return []domain.SessionMemory{}, nil
	}
	var rows []domain.SessionMemory
	if err := r.db.Where("session_id IN ? AND is_active = ?", normalized, true).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) GetSessionMemoriesByIDs(ids []string) ([]domain.SessionMemory, error) {
	if len(ids) == 0 {
		return []domain.SessionMemory{}, nil
	}
	normalized := make([]string, 0, len(ids))
	for _, id := range ids {
		if v := strings.TrimSpace(id); v != "" {
			normalized = append(normalized, v)
		}
	}
	if len(normalized) == 0 {
		return []domain.SessionMemory{}, nil
	}
	var rows []domain.SessionMemory
	if err := r.db.Where("id IN ? AND is_active = ?", normalized, true).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) CountActiveSessionMemories(sessionID string) (int64, error) {
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return 0, fmt.Errorf("session_id cannot be empty")
	}
	var count int64
	if err := r.db.Model(&domain.SessionMemory{}).Where("session_id = ? AND is_active = ?", sid, true).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) ListActiveSessionMemories(ctx context.Context, sessionID string) ([]domain.SessionMemory, error) {
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}
	var rows []domain.SessionMemory
	if err := r.dbWithContext(ctx).Where("session_id = ? AND is_active = ?", sid, true).
		Order("updated_at ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) ListRecentActiveSessionMemories(sessionID string, limit int) ([]domain.SessionMemory, error) {
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}
	if limit <= 0 {
		return []domain.SessionMemory{}, nil
	}
	var rows []domain.SessionMemory
	if err := r.db.Where("session_id = ? AND is_active = ?", sid, true).
		Order("updated_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) CreateSessionMemory(input domain.SessionMemoryCreateInput) (*domain.SessionMemory, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		return nil, fmt.Errorf("summary cannot be empty")
	}
	keywords := normalizeKeywords(input.Keywords)
	keywordsJSONBytes, err := json.Marshal(keywords)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	item := domain.SessionMemory{
		ID:                 uuid.NewString(),
		SessionID:          sessionID,
		Summary:            summary,
		KeywordsJSON:       string(keywordsJSONBytes),
		KeywordsText:       strings.Join(keywords, " "),
		SourceMessageCount: input.SourceMessageCount,
		IsActive:           true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := r.db.Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) UpdateSessionMemoryContent(id, sessionID, summary string, keywords []string, sourceMessageCount int) error {
	id = strings.TrimSpace(id)
	sid := strings.TrimSpace(sessionID)
	if id == "" || sid == "" {
		return fmt.Errorf("id and session_id required")
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return fmt.Errorf("summary cannot be empty")
	}
	kw := normalizeKeywords(keywords)
	kwJSON, err := json.Marshal(kw)
	if err != nil {
		return err
	}
	now := time.Now()
	res := r.db.Model(&domain.SessionMemory{}).
		Where("id = ? AND session_id = ? AND is_active = ?", id, sid, true).
		Updates(map[string]any{
			"summary":              summary,
			"keywords_json":        string(kwJSON),
			"keywords_text":        strings.Join(kw, " "),
			"source_message_count": sourceMessageCount,
			"updated_at":           now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) SoftDeleteSessionMemory(id, sessionID string) error {
	id = strings.TrimSpace(id)
	sid := strings.TrimSpace(sessionID)
	if id == "" || sid == "" {
		return fmt.Errorf("id and session_id required")
	}
	now := time.Now()
	res := r.db.Model(&domain.SessionMemory{}).
		Where("id = ? AND session_id = ? AND is_active = ?", id, sid, true).
		Updates(map[string]any{
			"is_active":  false,
			"updated_at": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) UpsertSessionMemoryIfNewer(input domain.SessionMemoryUpsertInput) (bool, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return false, fmt.Errorf("session_id cannot be empty")
	}
	var existing domain.SessionMemory
	err := r.db.Where("session_id = ? AND is_active = ?", sessionID, true).
		Order("source_message_count DESC, updated_at DESC").
		First(&existing).Error
	if err != nil && !isRecordNotFound(err) {
		return false, err
	}
	if err == nil {
		if input.SourceMessageCount < existing.SourceMessageCount {
			return false, nil
		}
		keywords := normalizeKeywords(input.Keywords)
		keywordsJSONBytes, err := json.Marshal(keywords)
		if err != nil {
			return false, err
		}
		now := time.Now()
		if err := r.db.Model(&domain.SessionMemory{}).
			Where("id = ?", existing.ID).
			Updates(map[string]any{
				"summary":              input.Summary,
				"keywords_json":        string(keywordsJSONBytes),
				"keywords_text":        strings.Join(keywords, " "),
				"source_message_count": input.SourceMessageCount,
				"updated_at":           now,
			}).Error; err != nil {
			return false, err
		}
		return true, nil
	}

	keywords := normalizeKeywords(input.Keywords)
	keywordsJSONBytes, err := json.Marshal(keywords)
	if err != nil {
		return false, err
	}
	now := time.Now()
	item := domain.SessionMemory{
		ID:                 uuid.NewString(),
		SessionID:          sessionID,
		Summary:            input.Summary,
		KeywordsJSON:       string(keywordsJSONBytes),
		KeywordsText:       strings.Join(keywords, " "),
		SourceMessageCount: input.SourceMessageCount,
		IsActive:           true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := r.db.Create(&item).Error; err != nil {
		return false, err
	}
	return true, nil
}

func ftsMatchPhrase(keyword string) string {
	k := strings.TrimSpace(keyword)
	if k == "" {
		return ""
	}
	k = strings.ReplaceAll(k, `"`, `""`)
	return `"` + k + `"`
}

func buildFTSMatchQuery(keywords []string) string {
	parts := make([]string, 0, len(keywords))
	for _, kw := range keywords {
		p := ftsMatchPhrase(kw)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " OR ")
}

func (r *Repository) ftsSessionMemoriesTableExists() bool {
	r.ftsOnce.Do(func() {
		var n int64
		_ = r.db.Raw(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='session_memories_fts'`).Scan(&n)
		r.ftsOK = n > 0
	})
	return r.ftsOK
}

func (r *Repository) SearchMemoriesByKeywords(keywords []string, limit int, excludeSessionID string) ([]domain.SessionMemorySearchHit, error) {
	normalizedKeywords := normalizeKeywords(keywords)
	if len(normalizedKeywords) == 0 || limit <= 0 {
		return []domain.SessionMemorySearchHit{}, nil
	}

	candidateLimit := limit * 20
	if candidateLimit < constants.DefaultMemoryCandidateLimit {
		candidateLimit = constants.DefaultMemoryCandidateLimit
	}
	if candidateLimit > constants.MaxMemoryCandidateLimit {
		candidateLimit = constants.MaxMemoryCandidateLimit
	}

	var candidates []domain.SessionMemory
	match := buildFTSMatchQuery(normalizedKeywords)
	if r.ftsSessionMemoriesTableExists() && match != "" {
		raw := `SELECT sm.id FROM session_memories sm
INNER JOIN session_memories_fts ON session_memories_fts.rowid = sm.rowid
WHERE session_memories_fts MATCH ? AND sm.is_active = 1`
		args := []any{match}
		if sid := strings.TrimSpace(excludeSessionID); sid != "" {
			raw += ` AND sm.session_id <> ?`
			args = append(args, sid)
		}
		raw += ` LIMIT ?`
		args = append(args, candidateLimit)
		var idRows []struct {
			ID string `gorm:"column:id"`
		}
		if err := r.db.Raw(raw, args...).Scan(&idRows).Error; err == nil && len(idRows) > 0 {
			ids := make([]string, 0, len(idRows))
			for _, row := range idRows {
				if strings.TrimSpace(row.ID) != "" {
					ids = append(ids, row.ID)
				}
			}
			if len(ids) > 0 {
				if err := r.db.Where("id IN ? AND is_active = ?", ids, true).Find(&candidates).Error; err != nil {
					return nil, err
				}
				byID := make(map[string]domain.SessionMemory, len(candidates))
				for _, c := range candidates {
					byID[c.ID] = c
				}
				candidates = candidates[:0]
				for _, id := range ids {
					if row, ok := byID[id]; ok {
						candidates = append(candidates, row)
					}
				}
			}
		}
	}
	if len(candidates) == 0 {
		query := r.db.Where("is_active = ?", true).Order("updated_at desc").Limit(candidateLimit)
		if sessionID := strings.TrimSpace(excludeSessionID); sessionID != "" {
			query = query.Where("session_id <> ?", sessionID)
		}
		orLikeParts := make([]string, 0, len(normalizedKeywords)*2)
		orLikeArgs := make([]any, 0, len(normalizedKeywords)*2)
		for _, keyword := range normalizedKeywords {
			like := "%" + keyword + "%"
			orLikeParts = append(orLikeParts, "keywords_text LIKE ?")
			orLikeArgs = append(orLikeArgs, like)
			orLikeParts = append(orLikeParts, "summary LIKE ?")
			orLikeArgs = append(orLikeArgs, like)
		}
		if len(orLikeParts) > 0 {
			query = query.Where("("+strings.Join(orLikeParts, " OR ")+")", orLikeArgs...)
		}
		if err := query.Find(&candidates).Error; err != nil {
			return nil, err
		}
	}

	hits := make([]domain.SessionMemorySearchHit, 0, len(candidates))
	for _, candidate := range candidates {
		parsedKeywords := parseStoredKeywords(candidate)
		matched := intersectKeywords(normalizedKeywords, parsedKeywords)
		if len(matched) == 0 {
			continue
		}
		hits = append(hits, domain.SessionMemorySearchHit{
			Memory:          candidate,
			MatchedKeywords: matched,
			Score:           scoreMemoryHit(len(matched), candidate.UpdatedAt),
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

func (r *Repository) CountSessionMessages(sessionID string) (int64, error) {
	var total int64
	err := r.db.Model(&domain.Message{}).
		Where("session_id = ?", strings.TrimSpace(sessionID)).
		Count(&total).
		Error
	return total, err
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

func parseStoredKeywords(memory domain.SessionMemory) []string {
	if strings.TrimSpace(memory.KeywordsJSON) != "" {
		var parsed []string
		if err := json.Unmarshal([]byte(memory.KeywordsJSON), &parsed); err == nil {
			return normalizeKeywords(parsed)
		}
	}
	if strings.TrimSpace(memory.KeywordsText) == "" {
		return []string{}
	}
	return normalizeKeywords(strings.Fields(memory.KeywordsText))
}

func intersectKeywords(queries []string, candidate []string) []string {
	if len(queries) == 0 || len(candidate) == 0 {
		return []string{}
	}
	candidateSet := make(map[string]struct{}, len(candidate))
	for _, item := range candidate {
		candidateSet[item] = struct{}{}
	}

	result := make([]string, 0, len(queries))
	for _, query := range queries {
		if _, ok := candidateSet[query]; ok {
			result = append(result, query)
		}
	}
	return result
}

func scoreMemoryHit(matchedCount int, updatedAt time.Time) float64 {
	score := float64(matchedCount) * 100
	ageHours := time.Since(updatedAt).Hours()
	switch {
	case ageHours <= 24:
		score += 30
	case ageHours <= 24*7:
		score += 20
	case ageHours <= 24*30:
		score += 10
	}
	return score
}
