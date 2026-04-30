package repositories

import (
	"context"
	"slimebot/internal/domain"
	"time"

	"github.com/google/uuid"
)

func (r *Repository) ListMCPConfigs(ctx context.Context) ([]domain.MCPConfig, error) {
	var items []domain.MCPConfig
	err := r.dbWithContext(ctx).Order("created_at asc").Find(&items).Error
	return items, err
}

func (r *Repository) ListEnabledMCPConfigs(ctx context.Context) ([]domain.MCPConfig, error) {
	var items []domain.MCPConfig
	err := r.dbWithContext(ctx).Where("is_enabled = ?", true).Order("created_at asc").Find(&items).Error
	return items, err
}

func (r *Repository) CreateMCPConfig(ctx context.Context, item domain.MCPConfig) (*domain.MCPConfig, error) {
	item.ID = uuid.NewString()
	if err := r.dbWithContext(ctx).Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) UpdateMCPConfig(ctx context.Context, id string, item domain.MCPConfig) error {
	return r.dbWithContext(ctx).Model(&domain.MCPConfig{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"name":       item.Name,
			"config":     item.Config,
			"is_enabled": item.IsEnabled,
			"updated_at": time.Now(),
		}).Error
}

func (r *Repository) DeleteMCPConfig(ctx context.Context, id string) error {
	return r.dbWithContext(ctx).Where("id = ?", id).Delete(&domain.MCPConfig{}).Error
}
