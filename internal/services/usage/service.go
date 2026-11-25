package usage

import (
	"context"
	"errors"
	"fmt"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/services/llm"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrSessionOutputTokenBudgetExceeded = errors.New("session output token budget exceeded")

type BudgetExceededError struct {
	RootSessionID string
	Budget        int64
	Used          int64
	Minimum       int64
}

func (e *BudgetExceededError) Error() string {
	return fmt.Sprintf("session output token budget exceeded: root=%s budget=%d used=%d minimum=%d", e.RootSessionID, e.Budget, e.Used, e.Minimum)
}
func (e *BudgetExceededError) Unwrap() error { return ErrSessionOutputTokenBudgetExceeded }

type Scope struct {
	RootSessionID    string
	SessionID        string
	RequestID        string
	WorkingDirectory string
}

type scopeKey struct{}

func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

func ScopeFromContext(ctx context.Context) Scope {
	scope, _ := ctx.Value(scopeKey{}).(Scope)
	return scope
}

type Service struct {
	store    domain.UsageStore
	settings domain.SettingsStore
}

func NewService(store domain.UsageStore, settings domain.SettingsStore) *Service {
	return &Service{store: store, settings: settings}
}

func (s *Service) Summary(ctx context.Context, query domain.UsageQuery) ([]domain.UsageSummary, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("usage store is not initialized")
	}
	return s.store.SummarizeUsage(ctx, query)
}

func (s *Service) SessionOutputTokenBudget(ctx context.Context) (int64, error) {
	if s == nil || s.settings == nil {
		return 0, nil
	}
	raw, err := s.settings.GetSetting(ctx, constants.SettingSessionOutputTokenBudget)
	if err != nil || strings.TrimSpace(raw) == "" {
		return 0, err
	}
	budget, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || budget < 0 {
		return 0, fmt.Errorf("invalid %s setting: %q", constants.SettingSessionOutputTokenBudget, raw)
	}
	return budget, nil
}

func (s *Service) ObserveRequest(ctx context.Context, model llm.ModelRuntimeConfig, _ []llm.ChatMessage, _ []llm.ToolDef) (func(*llm.StreamResult, error) error, error) {
	if s == nil || s.store == nil {
		return func(*llm.StreamResult, error) error { return nil }, nil
	}
	scope := ScopeFromContext(ctx)
	if scope.RootSessionID == "" {
		scope.RootSessionID = llm.PrivacyScopeFromContext(ctx).SessionID
	}
	if scope.SessionID == "" {
		scope.SessionID = scope.RootSessionID
	}
	if model.Purpose != "title" {
		resolved, err := llm.ResolveRequestConfig(model)
		if err != nil {
			return nil, err
		}
		model = resolved
		budget, err := s.SessionOutputTokenBudget(ctx)
		if err != nil {
			return nil, err
		}
		if budget > 0 {
			used, err := s.store.SessionOutputTokens(ctx, scope.RootSessionID)
			if err != nil {
				return nil, err
			}
			remaining := budget - used
			minimum := int64(1)
			if model.Provider == llm.ProviderAnthropic {
				minimum = int64(llm.ThinkingBudgetTokens(model.ThinkingLevel) + 1)
			}
			if remaining < minimum {
				return nil, &BudgetExceededError{RootSessionID: scope.RootSessionID, Budget: budget, Used: used, Minimum: minimum}
			}
			if int64(model.MaxOutputTokens) > remaining {
				model.MaxOutputTokens = int(remaining)
			}
		}
	}
	llm.ReplaceRequestConfig(ctx, model)
	return func(result *llm.StreamResult, requestErr error) error {
		if requestErr != nil || result == nil || result.TokenUsage == nil || result.TokenUsage.IsZero() || scope.RootSessionID == "" {
			return nil
		}
		tokens := result.TokenUsage
		record := &domain.UsageRecord{
			ID: uuid.NewString(), RootSessionID: scope.RootSessionID, SessionID: scope.SessionID,
			RequestID: scope.RequestID, Purpose: model.Purpose, Provider: model.Provider,
			ModelConfigID: model.ConfigID, Model: model.Model, WorkingDirectory: scope.WorkingDirectory,
			InputTokens: int64(tokens.InputTokens), OutputTokens: int64(tokens.OutputTokens),
			CacheCreationInputTokens: int64(tokens.CacheCreationInputTokens),
			CacheReadInputTokens:     int64(tokens.CacheReadInputTokens),
			TotalTokens:              int64(tokens.InputTokens + tokens.OutputTokens + tokens.CacheCreationInputTokens),
			CreatedAt:                time.Now(),
		}
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return s.store.RecordUsage(writeCtx, record)
	}, nil
}
