package domain

import (
	"context"
	"time"
)

const (
	UsageGroupBySession = "session"
	UsageGroupByModel   = "model"
	UsageGroupByProject = "project"
)

type UsageRecord struct {
	ID                       string    `gorm:"primaryKey;size:36" json:"id"`
	RootSessionID            string    `gorm:"size:36;index:idx_usage_root_created,priority:1;index:idx_usage_root_model;index:idx_usage_root_project" json:"rootSessionId"`
	SessionID                string    `gorm:"size:36;index" json:"sessionId"`
	RequestID                string    `gorm:"size:36;index" json:"requestId,omitempty"`
	Purpose                  string    `gorm:"size:32" json:"purpose,omitempty"`
	Provider                 string    `gorm:"size:32" json:"provider"`
	ModelConfigID            string    `gorm:"size:36;index:idx_usage_root_model,priority:2" json:"modelConfigId,omitempty"`
	Model                    string    `gorm:"size:255;index:idx_usage_root_model,priority:3" json:"model"`
	WorkingDirectory         string    `gorm:"size:2048;index:idx_usage_root_project,priority:2" json:"workingDirectory,omitempty"`
	InputTokens              int64     `json:"inputTokens"`
	OutputTokens             int64     `json:"outputTokens"`
	CacheCreationInputTokens int64     `json:"cacheCreationInputTokens"`
	CacheReadInputTokens     int64     `json:"cacheReadInputTokens"`
	TotalTokens              int64     `json:"totalTokens"`
	CreatedAt                time.Time `gorm:"index:idx_usage_root_created,priority:2" json:"createdAt"`
}

type UsageQuery struct {
	GroupBy       string
	RootSessionID string
}

type UsageSummary struct {
	GroupKey                 string `json:"groupKey"`
	RootSessionID            string `json:"rootSessionId,omitempty"`
	ModelConfigID            string `json:"modelConfigId,omitempty"`
	Model                    string `json:"model,omitempty"`
	WorkingDirectory         string `json:"workingDirectory,omitempty"`
	RequestCount             int64  `json:"requestCount"`
	InputTokens              int64  `json:"inputTokens"`
	OutputTokens             int64  `json:"outputTokens"`
	CacheCreationInputTokens int64  `json:"cacheCreationInputTokens"`
	CacheReadInputTokens     int64  `json:"cacheReadInputTokens"`
	TotalTokens              int64  `json:"totalTokens"`
}

type UsageStore interface {
	RecordUsage(ctx context.Context, record *UsageRecord) error
	SummarizeUsage(ctx context.Context, query UsageQuery) ([]UsageSummary, error)
	SessionOutputTokens(ctx context.Context, rootSessionID string) (int64, error)
}
