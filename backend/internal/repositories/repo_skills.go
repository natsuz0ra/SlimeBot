package repositories

import (
	"errors"
	"slimebot/backend/internal/domain"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *Repository) ListSkills() ([]domain.Skill, error) {
	var items []domain.Skill
	err := r.db.Order("uploaded_at desc").Order("created_at desc").Find(&items).Error
	return items, err
}

func (r *Repository) GetSkillByID(id string) (*domain.Skill, error) {
	var item domain.Skill
	err := r.db.First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &item, err
}

func (r *Repository) GetSkillByName(name string) (*domain.Skill, error) {
	var item domain.Skill
	err := r.db.First(&item, "name = ?", name).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &item, err
}

func (r *Repository) CreateSkill(item domain.Skill) (*domain.Skill, error) {
	item.ID = uuid.NewString()
	if item.UploadedAt.IsZero() {
		item.UploadedAt = time.Now()
	}
	if err := r.db.Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) DeleteSkill(id string) error {
	return r.db.Where("id = ?", id).Delete(&domain.Skill{}).Error
}
