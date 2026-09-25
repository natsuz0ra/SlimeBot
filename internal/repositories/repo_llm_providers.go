package repositories

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *Repository) ListLLMProviders(ctx context.Context) ([]domain.LLMProvider, error) {
	items := make([]domain.LLMProvider, 0)
	err := r.dbWithContext(ctx).Order("name asc").Order("created_at asc").Find(&items).Error
	for i := range items {
		items[i].HasAPIKey = items[i].APIKey != ""
	}
	return items, err
}

func (r *Repository) GetLLMProvider(ctx context.Context, id string) (*domain.LLMProvider, error) {
	var item domain.LLMProvider
	err := r.dbWithContext(ctx).First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("LLM provider %s: %w", id, apperrors.ErrNotFound)
	}
	item.HasAPIKey = item.APIKey != ""
	return &item, err
}

func (r *Repository) CreateLLMProvider(ctx context.Context, item domain.LLMProvider) (*domain.LLMProvider, error) {
	item.ID = uuid.NewString()
	if err := r.dbWithContext(ctx).Create(&item).Error; err != nil {
		return nil, err
	}
	item.HasAPIKey = item.APIKey != ""
	return &item, nil
}

func (r *Repository) UpdateLLMProvider(ctx context.Context, id string, item domain.LLMProvider) error {
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current domain.LLMProvider
		if err := tx.First(&current, "id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Model(&current).Updates(map[string]any{
			"name": item.Name, "protocol": item.Protocol, "base_url": item.BaseURL, "api_key": item.APIKey,
		}).Error; err != nil {
			return err
		}
		// Legacy flattened fields remain available to existing chat and CLI paths.
		return tx.Model(&domain.LLMConfig{}).Where("provider_id = ?", id).Updates(map[string]any{
			"provider": item.Protocol, "base_url": item.BaseURL, "api_key": providerRuntimeKey(item),
		}).Error
	})
}

func providerRuntimeKey(item domain.LLMProvider) string {
	if item.APIKey != "" {
		return item.APIKey
	}
	return "no-key-required"
}

func (r *Repository) DeleteLLMProvider(ctx context.Context, id string) error {
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("provider_id = ?", id).Delete(&domain.LLMConfig{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&domain.LLMProvider{}).Error
	})
}

// migrateLegacyLLMProviders keeps every existing model ID, grouping only identical connections.
func migrateLegacyLLMProviders(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var models []domain.LLMConfig
		if err := tx.Where("provider_id = '' OR provider_id IS NULL").Order("created_at asc").Find(&models).Error; err != nil {
			return err
		}
		providers := map[string]string{}
		for _, model := range models {
			protocol := strings.TrimSpace(model.Provider)
			if protocol == "" {
				protocol = "openai"
			}
			key := protocol + "\x00" + strings.TrimRight(strings.TrimSpace(model.BaseURL), "/") + "\x00" + model.APIKey
			id := providers[key]
			if id == "" {
				name := protocol
				if parsed, err := url.Parse(model.BaseURL); err == nil && parsed.Hostname() != "" {
					name = parsed.Hostname()
				}
				provider := domain.LLMProvider{ID: uuid.NewString(), Name: name, Protocol: protocol, BaseURL: model.BaseURL, APIKey: model.APIKey}
				if err := tx.Create(&provider).Error; err != nil {
					return err
				}
				id = provider.ID
				providers[key] = id
			}
			if err := tx.Model(&domain.LLMConfig{}).Where("id = ?", model.ID).Update("provider_id", id).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
