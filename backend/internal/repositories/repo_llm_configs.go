package repositories

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"slimebot/backend/internal/models"
)

func (r *Repository) ListLLMConfigs() ([]models.LLMConfig, error) {
	var items []models.LLMConfig
	err := r.db.Order("name asc").Order("created_at asc").Find(&items).Error
	return items, err
}

func (r *Repository) GetLLMConfigByID(id string) (*models.LLMConfig, error) {
	var item models.LLMConfig
	err := r.db.First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &item, err
}

func (r *Repository) CreateLLMConfig(item models.LLMConfig) (*models.LLMConfig, error) {
	item.ID = uuid.NewString()
	if err := r.db.Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) DeleteLLMConfig(id string) error {
	return r.db.Where("id = ?", id).Delete(&models.LLMConfig{}).Error
}
