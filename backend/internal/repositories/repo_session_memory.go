package repositories

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"slimebot/backend/internal/models"
)

const (
	defaultMemoryCandidateLimit = 200
	maxMemoryCandidateLimit     = 1000
)

type SessionMemoryUpsertInput struct {
	SessionID          string
	Summary            string
	Keywords           []string
	SourceMessageCount int
}

type SessionMemorySearchHit struct {
	Memory          models.SessionMemory
	MatchedKeywords []string
	Score           float64
}

func (r *Repository) GetSessionMemory(sessionID string) (*models.SessionMemory, error) {
	var item models.SessionMemory
	err := r.db.Where("session_id = ?", strings.TrimSpace(sessionID)).First(&item).Error
	if err != nil {
		if isRecordNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) UpsertSessionMemory(input SessionMemoryUpsertInput) error {
	now := time.Now()
	sessionID := strings.TrimSpace(input.SessionID)
	keywords := normalizeKeywords(input.Keywords)
	keywordsJSONBytes, err := json.Marshal(keywords)
	if err != nil {
		return err
	}
	keywordsJSON := string(keywordsJSONBytes)
	keywordsText := strings.Join(keywords, " ")

	var existing models.SessionMemory
	query := r.db.Where("session_id = ?", sessionID).First(&existing)
	if query.Error == nil {
		return r.db.Model(&models.SessionMemory{}).
			Where("id = ?", existing.ID).
			Updates(map[string]any{
				"summary":              input.Summary,
				"keywords_json":        keywordsJSON,
				"keywords_text":        keywordsText,
				"source_message_count": input.SourceMessageCount,
				"updated_at":           now,
			}).
			Error
	}
	if query.Error != nil && !isRecordNotFound(query.Error) {
		return query.Error
	}

	item := models.SessionMemory{
		ID:                 uuid.NewString(),
		SessionID:          sessionID,
		Summary:            input.Summary,
		KeywordsJSON:       keywordsJSON,
		KeywordsText:       keywordsText,
		SourceMessageCount: input.SourceMessageCount,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	return r.db.Create(&item).Error
}

func (r *Repository) SearchMemoriesByKeywords(keywords []string, limit int, excludeSessionID string) ([]SessionMemorySearchHit, error) {
	normalizedKeywords := normalizeKeywords(keywords)
	if len(normalizedKeywords) == 0 || limit <= 0 {
		return []SessionMemorySearchHit{}, nil
	}

	candidateLimit := limit * 20
	if candidateLimit < defaultMemoryCandidateLimit {
		candidateLimit = defaultMemoryCandidateLimit
	}
	if candidateLimit > maxMemoryCandidateLimit {
		candidateLimit = maxMemoryCandidateLimit
	}

	var candidates []models.SessionMemory
	query := r.db.Order("updated_at desc").Limit(candidateLimit)
	if sessionID := strings.TrimSpace(excludeSessionID); sessionID != "" {
		query = query.Where("session_id <> ?", sessionID)
	}
	if err := query.Find(&candidates).Error; err != nil {
		return nil, err
	}

	hits := make([]SessionMemorySearchHit, 0, len(candidates))
	for _, candidate := range candidates {
		parsedKeywords := parseStoredKeywords(candidate)
		matched := intersectKeywords(normalizedKeywords, parsedKeywords)
		if len(matched) == 0 {
			continue
		}
		hits = append(hits, SessionMemorySearchHit{
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
	err := r.db.Model(&models.Message{}).
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

func parseStoredKeywords(memory models.SessionMemory) []string {
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
