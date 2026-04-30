package repositories

import (
	"context"
	"errors"
	"fmt"
	"slimebot/internal/apperrors"
	"slimebot/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *Repository) ListLLMConfigs(ctx context.Context) ([]domain.LLMConfig, error) {
	var items []domain.LLMConfig
	err := r.dbWithContext(ctx).Order("name asc").Order("created_at asc").Find(&items).Error
	return items, err
}

func (r *Repository) GetLLMConfigByID(ctx context.Context, id string) (*domain.LLMConfig, error) {
	var item domain.LLMConfig
	err := r.dbWithContext(ctx).First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("llm config %s: %w", id, apperrors.ErrNotFound)
	}
	return &item, err
}

func (r *Repository) CreateLLMConfig(ctx context.Context, item domain.LLMConfig) (*domain.LLMConfig, error) {
	item.ID = uuid.NewString()
	if err := r.dbWithContext(ctx).Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) DeleteLLMConfig(ctx context.Context, id string) error {
	return r.dbWithContext(ctx).Where("id = ?", id).Delete(&domain.LLMConfig{}).Error
}
