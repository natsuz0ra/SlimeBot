package repositories

import (
	"context"
	"errors"
	"fmt"

	"slimebot/internal/apperrors"
	"slimebot/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) GetSessionContextSummary(ctx context.Context, sessionID, modelConfigID string) (*domain.SessionContextSummary, error) {
	var item domain.SessionContextSummary
	err := r.dbWithContext(ctx).
		Where("session_id = ? AND model_config_id = ?", sessionID, modelConfigID).
		First(&item).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("session context summary %s/%s: %w", sessionID, modelConfigID, apperrors.ErrNotFound)
	}
	return &item, err
}

func (r *Repository) UpsertSessionContextSummary(ctx context.Context, item *domain.SessionContextSummary) error {
	if item == nil {
		return nil
	}
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	return r.dbWithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "session_id"},
			{Name: "model_config_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"summary",
			"summarized_until_seq",
			"pre_compact_token_estimate",
			"updated_at",
		}),
	}).Create(item).Error
}
