package repositories

import (
	"errors"
	"time"

	"corner/backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListSessions() ([]models.Session, error) {
	var sessions []models.Session
	err := r.db.Order("updated_at desc").Find(&sessions).Error
	return sessions, err
}

func (r *Repository) GetSessionByID(id string) (*models.Session, error) {
	var session models.Session
	err := r.db.First(&session, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &session, err
}

func (r *Repository) CreateSession(name string) (*models.Session, error) {
	session := &models.Session{
		ID:   uuid.NewString(),
		Name: name,
	}
	err := r.db.Create(session).Error
	return session, err
}

func (r *Repository) RenameSessionByUser(id, name string) error {
	return r.db.Model(&models.Session{}).
		Where("id = ?", id).
		Updates(map[string]any{"name": name, "is_title_locked": true, "updated_at": time.Now()}).
		Error
}

func (r *Repository) UpdateSessionTitle(id, name string) error {
	return r.db.Model(&models.Session{}).
		Where("id = ?", id).
		Updates(map[string]any{"name": name, "updated_at": time.Now()}).
		Error
}

func (r *Repository) DeleteSession(id string) error {
	tx := r.db.Begin()
	if err := tx.Where("session_id = ?", id).Delete(&models.Message{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Where("id = ?", id).Delete(&models.Session{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

func (r *Repository) ListSessionMessages(sessionID string) ([]models.Message, error) {
	var messages []models.Message
	err := r.db.Where("session_id = ?", sessionID).Order("created_at asc").Find(&messages).Error
	return messages, err
}

func (r *Repository) ListRecentSessionMessages(sessionID string, limit int) ([]models.Message, error) {
	if limit <= 0 {
		return []models.Message{}, nil
	}

	var messages []models.Message
	err := r.db.
		Where("session_id = ?", sessionID).
		Order("created_at desc").
		Limit(limit).
		Find(&messages).
		Error
	if err != nil {
		return nil, err
	}

	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	return messages, nil
}

func (r *Repository) AddMessage(sessionID, role, content string) (*models.Message, error) {
	message := &models.Message{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		Role:      role,
		Content:   content,
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		return tx.Model(&models.Session{}).
			Where("id = ?", sessionID).
			Update("updated_at", time.Now()).
			Error
	})
	return message, err
}

func (r *Repository) SetSessionModel(sessionID, modelConfigID string) error {
	return r.db.Model(&models.Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{"model_config_id": modelConfigID, "updated_at": time.Now()}).
		Error
}

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

func (r *Repository) ListMCPConfigs() ([]models.MCPConfig, error) {
	var items []models.MCPConfig
	err := r.db.Order("created_at asc").Find(&items).Error
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
			"server_url": item.ServerURL,
			"auth_type":  item.AuthType,
			"auth_value": item.AuthValue,
			"is_enabled": item.IsEnabled,
			"updated_at": time.Now(),
		}).Error
}

func (r *Repository) DeleteMCPConfig(id string) error {
	return r.db.Where("id = ?", id).Delete(&models.MCPConfig{}).Error
}
