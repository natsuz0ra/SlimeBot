package repositories

import (
	"errors"
	"slimebot/internal/domain"
	"strconv"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) GetSetting(key string) (string, error) {
	var item domain.AppSetting
	err := r.db.First(&item, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return item.Value, err
}

func (r *Repository) SetSetting(key, value string) error {
	setting := domain.AppSetting{Key: key, Value: value}
	return r.db.
		Where(domain.AppSetting{Key: key}).
		Assign(domain.AppSetting{Value: value, UpdatedAt: time.Now()}).
		FirstOrCreate(&setting).Error
}

func (r *Repository) GetSettingBool(key string, fallback bool) (bool, error) {
	raw, err := r.GetSetting(key)
	if err != nil {
		return fallback, err
	}
	if raw == "" {
		return fallback, nil
	}
	parsed, parseErr := strconv.ParseBool(raw)
	if parseErr != nil {
		return fallback, nil
	}
	return parsed, nil
}
