package repositories

import (
	"context"
	"fmt"
	"slimebot/internal/domain"

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
	items := []domain.UsageSummary{}
	err := db.Select(selectColumns + `, COUNT(*) AS request_count,
		SUM(input_tokens) AS input_tokens, SUM(output_tokens) AS output_tokens,
		SUM(cache_creation_input_tokens) AS cache_creation_input_tokens,
		SUM(cache_read_input_tokens) AS cache_read_input_tokens,
		SUM(total_tokens) AS total_tokens`).
		Group(groupColumns).Order("total_tokens DESC, group_key ASC").Scan(&items).Error
	return items, err
}
