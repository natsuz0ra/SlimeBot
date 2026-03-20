package repositories

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"slimebot/backend/internal/constants"
	"slimebot/backend/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrStaleSessionMemoryWrite = errors.New("stale session memory write")

func (r *Repository) GetSessionMemory(sessionID string) (*domain.SessionMemory, error) {
	var item domain.SessionMemory
	err := r.db.Where("session_id = ?", strings.TrimSpace(sessionID)).First(&item).Error
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
	if err := r.db.Where("session_id IN ?", normalized).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) UpsertSessionMemory(input domain.SessionMemoryUpsertInput) error {
	_, err := r.UpsertSessionMemoryIfNewer(input)
	return err
}

// UpsertSessionMemoryIfNewer 仅在消息计数不倒退时更新记忆，防止旧摘要覆盖新摘要。
func (r *Repository) UpsertSessionMemoryIfNewer(input domain.SessionMemoryUpsertInput) (bool, error) {
	now := time.Now()
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return false, fmt.Errorf("session_id cannot be empty")
	}
	keywords := normalizeKeywords(input.Keywords)
	keywordsJSONBytes, err := json.Marshal(keywords)
	if err != nil {
		return false, err
	}
	keywordsJSON := string(keywordsJSONBytes)
	keywordsText := strings.Join(keywords, " ")

	updated := false
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var existing domain.SessionMemory
		query := tx.Where("session_id = ?", sessionID).First(&existing)
		if query.Error == nil {
			if input.SourceMessageCount < existing.SourceMessageCount {
				updated = false
				return nil
			}
			if err := tx.Model(&domain.SessionMemory{}).
				Where("id = ?", existing.ID).
				Updates(map[string]any{
					"summary":              input.Summary,
					"keywords_json":        keywordsJSON,
					"keywords_text":        keywordsText,
					"source_message_count": input.SourceMessageCount,
					"updated_at":           now,
				}).Error; err != nil {
				return err
			}
			updated = true
			return nil
		}
		if query.Error != nil && !isRecordNotFound(query.Error) {
			return query.Error
		}

		item := domain.SessionMemory{
			ID:                 uuid.NewString(),
			SessionID:          sessionID,
			Summary:            input.Summary,
			KeywordsJSON:       keywordsJSON,
			KeywordsText:       keywordsText,
			SourceMessageCount: input.SourceMessageCount,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		updated = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return updated, nil
}

// SearchMemoriesByKeywords 基于关键词匹配和时间衰减评分返回 TopN 记忆。
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
	query := r.db.Order("updated_at desc").Limit(candidateLimit)
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

// normalizeKeywords 归一化、去空和去重关键词。
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

// parseStoredKeywords 优先解析 JSON 关键词，失败时回退到文本字段。
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

// intersectKeywords 计算查询词与候选词集合的交集。
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

// scoreMemoryHit 按匹配数量与更新时间计算检索排序分值。
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
