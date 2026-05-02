package repositories

import (
	"context"
	"errors"
	"fmt"
	"slimebot/internal/apperrors"
	"slimebot/internal/constants"
	"slimebot/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *Repository) ListLLMConfigs(ctx context.Context) ([]domain.LLMConfig, error) {
	var items []domain.LLMConfig
	err := r.dbWithContext(ctx).Order("name asc").Order("created_at asc").Find(&items).Error
	normalizeLLMConfigs(items)
	return items, err
}

func (r *Repository) GetLLMConfigByID(ctx context.Context, id string) (*domain.LLMConfig, error) {
	var item domain.LLMConfig
	err := r.dbWithContext(ctx).First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("llm config %s: %w", id, apperrors.ErrNotFound)
	}
	normalizeLLMConfig(&item)
	return &item, err
}

func (r *Repository) CreateLLMConfig(ctx context.Context, item domain.LLMConfig) (*domain.LLMConfig, error) {
	item.ID = uuid.NewString()
	normalizeLLMConfig(&item)
	if err := r.dbWithContext(ctx).Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) UpdateLLMConfig(ctx context.Context, id string, item domain.LLMConfig) error {
	normalizeLLMConfig(&item)
	return r.dbWithContext(ctx).Model(&domain.LLMConfig{}).Where("id = ?", id).Updates(domain.LLMConfig{
		Name:        item.Name,
		Provider:    item.Provider,
		BaseURL:     item.BaseURL,
		APIKey:      item.APIKey,
		Model:       item.Model,
		ContextSize: item.ContextSize,
	}).Error
}

func (r *Repository) DeleteLLMConfig(ctx context.Context, id string) error {
	return r.dbWithContext(ctx).Where("id = ?", id).Delete(&domain.LLMConfig{}).Error
}

func normalizeLLMConfigs(items []domain.LLMConfig) {
	for idx := range items {
		normalizeLLMConfig(&items[idx])
	}
}

func normalizeLLMConfig(item *domain.LLMConfig) {
	if item == nil {
		return
	}
	if item.ContextSize <= 0 {
		item.ContextSize = constants.DefaultContextSize
	}
}
