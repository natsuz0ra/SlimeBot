package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"slimebot/internal/apperrors"
	"slimebot/internal/domain"

	"gorm.io/gorm"
)

func (r *Repository) ListACPConfigs(ctx context.Context) ([]domain.ACPConfig, error) {
	items := make([]domain.ACPConfig, 0)
	err := r.dbWithContext(ctx).Order("name asc, id asc").Find(&items).Error
	return items, err
}

func (r *Repository) GetACPConfig(ctx context.Context, id string) (domain.ACPConfig, error) {
	var item domain.ACPConfig
	err := r.dbWithContext(ctx).Take(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = apperrors.ErrNotFound
	}
	return item, err
}

func (r *Repository) CreateACPConfig(ctx context.Context, value *domain.ACPConfig) error {
	return r.dbWithContext(ctx).Create(value).Error
}

func (r *Repository) UpdateACPConfig(ctx context.Context, value *domain.ACPConfig) error {
	now := time.Now().UTC()
	result := r.dbWithContext(ctx).Model(&domain.ACPConfig{}).
		Where("id = ? AND updated_at = ?", value.ID, value.UpdatedAt).
		UpdateColumns(map[string]any{
			"name": value.Name, "command": value.Command, "args_json": value.ArgsJSON, "env_json": value.EnvJSON,
			"default_cwd": value.DefaultCwd, "timeout_sec": value.TimeoutSec, "is_enabled": value.IsEnabled, "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("ACP configuration changed: %w", apperrors.ErrInvalidInput)
	}
	value.UpdatedAt = now
	return nil
}

func (r *Repository) DeleteACPConfig(ctx context.Context, id string) error {
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ?", id).Delete(&domain.ACPConfig{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return apperrors.ErrNotFound
		}
		return tx.Model(&domain.Session{}).Where("acp_agent_id = ?", id).
			UpdateColumns(map[string]any{"acp_agent_id": nil, "acp_remote_session_id": ""}).Error
	})
}

func (r *Repository) SetSessionACP(ctx context.Context, id string, agentID *string, remote string) error {
	query := r.dbWithContext(ctx).Model(&domain.Session{}).Where("id = ? AND deleted_at IS NULL", id)
	if agentID != nil {
		query = query.Where("EXISTS (SELECT 1 FROM acp_configs WHERE id = ? AND is_enabled = ?)", *agentID, true)
	}
	result := query.UpdateColumns(map[string]any{"acp_agent_id": agentID, "acp_remote_session_id": remote})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}
