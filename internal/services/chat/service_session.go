package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"slimebot/internal/apperrors"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

// EnsureSession ensures a normal chat session exists; reuses valid sessionID or creates one.
func (s *ChatService) EnsureSession(ctx context.Context, sessionID string) (*domain.Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if sessionID != "" {
		existing, err := s.store.GetSessionByID(ctx, sessionID)
		if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}
	return s.store.CreateSession(ctx, "New Chat", s.runContext.WorkingDir)
}

// EnsureMessagePlatformSession ensures the bridged platform session exists with a fixed ID.
func (s *ChatService) EnsureMessagePlatformSession(ctx context.Context) (*domain.Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	session, err := s.store.GetSessionByID(ctx, constants.MessagePlatformSessionID)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		return nil, err
	}
	if session != nil {
		return session, nil
	}
	return s.store.CreateSessionWithID(ctx, constants.MessagePlatformSessionID, constants.MessagePlatformSessionName)
}

// ResolvePlatformModel resolves the default model for platform ingress (platform setting, then global, then first).
func (s *ChatService) ResolvePlatformModel(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Helper to verify a model ID exists without duplicating store lookups.
	resolveModel := func(modelID string) (string, bool, error) {
		trimmed := strings.TrimSpace(modelID)
		if trimmed == "" {
			return "", false, nil
		}
		item, err := s.store.GetLLMConfigByID(ctx, trimmed)
		if err != nil {
			if errors.Is(err, apperrors.ErrNotFound) {
				return "", false, nil
			}
			return "", false, err
		}
		return item.ID, true, nil
	}

	platformDefault, err := s.store.GetSetting(ctx, constants.SettingMessagePlatformDefaultModel)
	if err != nil {
		return "", err
	}
	if id, ok, err := resolveModel(platformDefault); err != nil {
		return "", err
	} else if ok {
		return id, nil
	}

	globalDefault, err := s.store.GetSetting(ctx, constants.SettingDefaultModel)
	if err != nil {
		return "", err
	}
	if id, ok, err := resolveModel(globalDefault); err != nil {
		return "", err
	} else if ok {
		return id, nil
	}

	allModels, err := s.store.ListLLMConfigs(ctx)
	if err != nil {
		return "", err
	}
	if len(allModels) == 0 {
		return "", fmt.Errorf("No available model is configured.")
	}
	fallbackID := strings.TrimSpace(allModels[0].ID)
	if fallbackID == "" {
		return "", fmt.Errorf("No available model is configured.")
	}
	return fallbackID, nil
}

// ResolvePlatformRuntimeSettings loads Telegram/message-platform runtime options.
func (s *ChatService) ResolvePlatformRuntimeSettings(ctx context.Context) (string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	thinkingLevel, err := s.store.GetSetting(ctx, constants.SettingMessagePlatformThinkingLevel)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(thinkingLevel) == "" {
		thinkingLevel = "off"
	}
	approvalMode, err := s.store.GetSetting(ctx, constants.SettingMessagePlatformApprovalMode)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(approvalMode) == "" {
		approvalMode = constants.ApprovalModeStandard
	}
	return strings.TrimSpace(thinkingLevel), strings.TrimSpace(approvalMode), nil
}

// ResolveLLMConfig loads and validates model config before requests.
func (s *ChatService) ResolveLLMConfig(ctx context.Context, modelID string) (*domain.LLMConfig, error) {
	configID := strings.TrimSpace(modelID)
	if configID == "" {
		return nil, fmt.Errorf("modelId is required.")
	}

	config, err := s.store.GetLLMConfigByID(ctx, configID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil, fmt.Errorf("Model config not found: %s.", configID)
		}
		return nil, err
	}
	if strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("Model config is incomplete: %s.", config.Name)
	}
	if config.ContextSize <= 0 {
		config.ContextSize = constants.DefaultContextSize
	}
	return config, nil
}
