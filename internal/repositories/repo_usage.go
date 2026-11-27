package repositories

import (
	"context"
	"fmt"
	"slimebot/internal/domain"
	"time"

	"gorm.io/gorm"
)

var _ domain.UsageStore = (*Repository)(nil)

func MigrateUsage(db *gorm.DB) error {
	return db.AutoMigrate(&domain.UsageRecord{})
}

func (r *Repository) RecordUsage(ctx context.Context, record *domain.UsageRecord) error {
	return r.dbWithContext(ctx).Create(record).Error
}

func (r *Repository) SessionOutputTokens(ctx context.Context, rootSessionID string) (int64, error) {
	var total int64
	err := r.dbWithContext(ctx).Model(&domain.UsageRecord{}).
		Where("root_session_id = ?", rootSessionID).
		Select("COALESCE(SUM(output_tokens), 0)").Scan(&total).Error
	return total, err
}

func (r *Repository) SummarizeUsage(ctx context.Context, query domain.UsageQuery) ([]domain.UsageSummary, error) {
	groupColumns := "root_session_id"
	selectColumns := "root_session_id, root_session_id AS group_key"
	switch query.GroupBy {
	case "", domain.UsageGroupBySession:
	case domain.UsageGroupByModel:
		groupColumns = "model_config_id, model"
		selectColumns = "model_config_id, model, model_config_id || ':' || model AS group_key"
	case domain.UsageGroupByProject:
		groupColumns = "working_directory"
		selectColumns = "working_directory, working_directory AS group_key"
	default:
		return nil, fmt.Errorf("invalid usage groupBy %q", query.GroupBy)
	}
	db := r.dbWithContext(ctx).Model(&domain.UsageRecord{})
	if query.RootSessionID != "" {
		db = db.Where("root_session_id = ?", query.RootSessionID)
	}
	if query.Since != nil {
		db = db.Where("created_at >= ?", *query.Since)
	}
	if query.Until != nil {
		db = db.Where("created_at < ?", *query.Until)
	}
	items := []domain.UsageSummary{}
	err := db.Select(selectColumns + `, COUNT(*) AS request_count,
		SUM(input_tokens) AS input_tokens, SUM(output_tokens) AS output_tokens,
		SUM(cache_creation_input_tokens) AS cache_creation_input_tokens,
		SUM(cache_read_input_tokens) AS cache_read_input_tokens,
		SUM(total_tokens) AS total_tokens`).
		Group(groupColumns).Order("total_tokens DESC, group_key ASC").Scan(&items).Error
	if err != nil {
		return nil, err
	}
	if (query.GroupBy == "" || query.GroupBy == domain.UsageGroupBySession) && len(items) > 0 {
		if err := r.attachSessionMetadata(ctx, items); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (r *Repository) attachSessionMetadata(ctx context.Context, items []domain.UsageSummary) error {
	ids := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.RootSessionID == "" {
			continue
		}
		if _, ok := seen[item.RootSessionID]; ok {
			continue
		}
		seen[item.RootSessionID] = struct{}{}
		ids = append(ids, item.RootSessionID)
	}
	if len(ids) == 0 {
		return nil
	}
	type sessionMeta struct {
		ID        string
		Name      string
		Kind      string
		DeletedAt *time.Time
	}
	var sessions []sessionMeta
	if err := r.dbWithContext(ctx).Model(&domain.Session{}).
		Select("id, name, kind, deleted_at").
		Where("id IN ?", ids).
		Scan(&sessions).Error; err != nil {
		return err
	}
	byID := make(map[string]sessionMeta, len(sessions))
	for _, session := range sessions {
		byID[session.ID] = session
	}
	for i := range items {
		session, ok := byID[items[i].RootSessionID]
		if !ok {
			continue
		}
		items[i].SessionName = session.Name
		items[i].SessionKind = session.Kind
		items[i].SessionExists = session.DeletedAt == nil
	}
	return nil
}
