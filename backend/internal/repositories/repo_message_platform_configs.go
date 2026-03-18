package repositories

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"slimebot/backend/internal/models"
)

func (r *Repository) ListMessagePlatformConfigs() ([]models.MessagePlatformConfig, error) {
	var items []models.MessagePlatformConfig
	err := r.db.Order("created_at asc").Find(&items).Error
	return items, err
}

func (r *Repository) GetMessagePlatformConfigByPlatform(platform string) (*models.MessagePlatformConfig, error) {
	var item models.MessagePlatformConfig
	err := r.db.First(&item, "platform = ?", platform).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) CreateMessagePlatformConfig(item models.MessagePlatformConfig) (*models.MessagePlatformConfig, error) {
	item.ID = uuid.NewString()
	if err := r.db.Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) UpdateMessagePlatformConfig(id string, item models.MessagePlatformConfig) error {
	return r.db.Model(&models.MessagePlatformConfig{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"display_name":     item.DisplayName,
			"auth_config_json": item.AuthConfigJSON,
			"is_enabled":       item.IsEnabled,
			"updated_at":       time.Now(),
		}).Error
}

func (r *Repository) DeleteMessagePlatformConfig(id string) error {
	return r.db.Where("id = ?", id).Delete(&models.MessagePlatformConfig{}).Error
}
