package repositories

import (
	"context"
	"slimebot/internal/domain"
)

func (r *Repository) ListTaskCenterAgents(ctx context.Context, limit int) ([]domain.TaskCenterAgent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	descriptors := []domain.AgentDescriptor{}
	ids := []string{}
	if err := r.dbWithContext(ctx).Model(&domain.AgentDescriptor{}).Where("archived = ?", false).Order("created_at DESC").Limit(limit).Pluck("session_id", &ids).Error; err != nil {
		return nil, err
	}
	if err := r.dbWithContext(ctx).Where("session_id IN ?", ids).Find(&descriptors).Error; err != nil {
		return nil, err
	}
	byID := make(map[string]domain.AgentDescriptor, len(descriptors))
	for _, item := range descriptors {
		byID[item.SessionID] = item
	}
	items := make([]domain.TaskCenterAgent, 0, len(ids))
	for _, id := range ids {
		descriptor := byID[id]
		item := domain.TaskCenterAgent{AgentDescriptor: descriptor}
		var turn domain.AgentTurn
		if err := r.dbWithContext(ctx).Where("session_id = ?", descriptor.SessionID).Order("created_at DESC").Limit(1).Take(&turn).Error; err == nil {
			item.LatestTurn = &turn
		}
		items = append(items, item)
	}
	return items, nil
}
