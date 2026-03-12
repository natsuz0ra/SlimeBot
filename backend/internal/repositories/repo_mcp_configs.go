package repositories

import (
	"time"

	"corner/backend/internal/models"
	"github.com/google/uuid"
)

func (r *Repository) ListMCPConfigs() ([]models.MCPConfig, error) {
	var items []models.MCPConfig
	err := r.db.Order("created_at asc").Find(&items).Error
	return items, err
}

func (r *Repository) ListEnabledMCPConfigs() ([]models.MCPConfig, error) {
	var items []models.MCPConfig
	err := r.db.Where("is_enabled = ?", true).Order("created_at asc").Find(&items).Error
	return items, err
}

func (r *Repository) CreateMCPConfig(item models.MCPConfig) (*models.MCPConfig, error) {
	item.ID = uuid.NewString()
	if err := r.db.Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) UpdateMCPConfig(id string, item models.MCPConfig) error {
	return r.db.Model(&models.MCPConfig{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"name":       item.Name,
			"config":     item.Config,
			"is_enabled": item.IsEnabled,
			"updated_at": time.Now(),
		}).Error
}

func (r *Repository) DeleteMCPConfig(id string) error {
	return r.db.Where("id = ?", id).Delete(&models.MCPConfig{}).Error
}
