package repositories

import (
	"errors"
	"slimebot/backend/internal/domain"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *Repository) ListMessagePlatformConfigs() ([]domain.MessagePlatformConfig, error) {
	var items []domain.MessagePlatformConfig
	err := r.db.Order("created_at asc").Find(&items).Error
	return items, err
}

func (r *Repository) GetMessagePlatformConfigByPlatform(platform string) (*domain.MessagePlatformConfig, error) {
	var item domain.MessagePlatformConfig
	err := r.db.First(&item, "platform = ?", platform).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) CreateMessagePlatformConfig(item domain.MessagePlatformConfig) (*domain.MessagePlatformConfig, error) {
	item.ID = uuid.NewString()
	if err := r.db.Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) UpdateMessagePlatformConfig(id string, item domain.MessagePlatformConfig) error {
	return r.db.Model(&domain.MessagePlatformConfig{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"display_name":     item.DisplayName,
			"auth_config_json": item.AuthConfigJSON,
			"is_enabled":       item.IsEnabled,
			"updated_at":       time.Now(),
		}).Error
}

func (r *Repository) DeleteMessagePlatformConfig(id string) error {
	return r.db.Where("id = ?", id).Delete(&domain.MessagePlatformConfig{}).Error
}
