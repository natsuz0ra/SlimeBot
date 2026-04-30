package repositories

import (
	"context"
	"errors"
	"slimebot/internal/domain"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *Repository) ListMessagePlatformConfigs(ctx context.Context) ([]domain.MessagePlatformConfig, error) {
	var items []domain.MessagePlatformConfig
	err := r.dbWithContext(ctx).Order("created_at asc").Find(&items).Error
	return items, err
}

func (r *Repository) GetMessagePlatformConfigByPlatform(ctx context.Context, platform string) (*domain.MessagePlatformConfig, error) {
	var item domain.MessagePlatformConfig
	err := r.dbWithContext(ctx).First(&item, "platform = ?", platform).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) CreateMessagePlatformConfig(ctx context.Context, item domain.MessagePlatformConfig) (*domain.MessagePlatformConfig, error) {
	item.ID = uuid.NewString()
	if err := r.dbWithContext(ctx).Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) UpdateMessagePlatformConfig(ctx context.Context, id string, item domain.MessagePlatformConfig) error {
	return r.dbWithContext(ctx).Model(&domain.MessagePlatformConfig{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"display_name":     item.DisplayName,
			"auth_config_json": item.AuthConfigJSON,
			"is_enabled":       item.IsEnabled,
			"updated_at":       time.Now(),
		}).Error
}

func (r *Repository) DeleteMessagePlatformConfig(ctx context.Context, id string) error {
	return r.dbWithContext(ctx).Where("id = ?", id).Delete(&domain.MessagePlatformConfig{}).Error
}
