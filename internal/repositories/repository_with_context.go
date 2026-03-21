package repositories

import (
	"context"

	"slimebot/internal/domain"
)

func (r *Repository) withContext(ctx context.Context) *Repository {
	if ctx == nil {
		return r
	}
	return &Repository{db: r.db.WithContext(ctx)}
}

func (r *Repository) GetSessionByIDWithContext(ctx context.Context, id string) (*domain.Session, error) {
	return r.withContext(ctx).GetSessionByID(id)
}

func (r *Repository) CreateSessionWithContext(ctx context.Context, name string) (*domain.Session, error) {
	return r.withContext(ctx).CreateSession(name)
}

func (r *Repository) CreateSessionWithIDWithContext(ctx context.Context, id, name string) (*domain.Session, error) {
	return r.withContext(ctx).CreateSessionWithID(id, name)
}

func (r *Repository) GetLLMConfigByIDWithContext(ctx context.Context, id string) (*domain.LLMConfig, error) {
	return r.withContext(ctx).GetLLMConfigByID(id)
}

func (r *Repository) ListLLMConfigsWithContext(ctx context.Context) ([]domain.LLMConfig, error) {
	return r.withContext(ctx).ListLLMConfigs()
}

func (r *Repository) ListRecentSessionMessagesWithContext(ctx context.Context, sessionID string, limit int) ([]domain.Message, error) {
	return r.withContext(ctx).ListRecentSessionMessages(sessionID, limit)
}

func (r *Repository) GetSessionMemoryWithContext(ctx context.Context, sessionID string) (*domain.SessionMemory, error) {
	return r.withContext(ctx).GetSessionMemory(sessionID)
}

func (r *Repository) GetSettingWithContext(ctx context.Context, key string) (string, error) {
	return r.withContext(ctx).GetSetting(key)
}

func (r *Repository) SetSettingWithContext(ctx context.Context, key, value string) error {
	return r.withContext(ctx).SetSetting(key, value)
}

func (r *Repository) AddMessageWithInputWithContext(ctx context.Context, input domain.AddMessageInput) (*domain.Message, error) {
	return r.withContext(ctx).AddMessageWithInput(input)
}

func (r *Repository) ListEnabledMCPConfigsWithContext(ctx context.Context) ([]domain.MCPConfig, error) {
	return r.withContext(ctx).ListEnabledMCPConfigs()
}

func (r *Repository) BindToolCallsToAssistantMessageWithContext(ctx context.Context, sessionID, requestID, assistantMessageID string) error {
	return r.withContext(ctx).BindToolCallsToAssistantMessage(sessionID, requestID, assistantMessageID)
}

func (r *Repository) UpdateSessionTitleWithContext(ctx context.Context, id, name string) error {
	return r.withContext(ctx).UpdateSessionTitle(id, name)
}

func (r *Repository) UpsertToolCallStartWithContext(ctx context.Context, input domain.ToolCallStartRecordInput) error {
	return r.withContext(ctx).UpsertToolCallStart(input)
}

func (r *Repository) UpdateToolCallResultWithContext(ctx context.Context, input domain.ToolCallResultRecordInput) error {
	return r.withContext(ctx).UpdateToolCallResult(input)
}
