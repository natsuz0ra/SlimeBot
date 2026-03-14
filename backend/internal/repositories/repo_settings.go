package repositories

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"slimebot/backend/internal/models"
)

func (r *Repository) GetSetting(key string) (string, error) {
	var item models.AppSetting
	err := r.db.First(&item, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return item.Value, err
}

func (r *Repository) SetSetting(key, value string) error {
	setting := models.AppSetting{Key: key, Value: value}
	return r.db.
		Where(models.AppSetting{Key: key}).
		Assign(models.AppSetting{Value: value, UpdatedAt: time.Now()}).
		FirstOrCreate(&setting).Error
}
