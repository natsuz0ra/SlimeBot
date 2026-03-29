package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

const platformModelCacheTTL = 30 * time.Second

// EnsureSession 确保普通聊天会话存在；传入有效 sessionID 时优先复用，否则创建新会话。
func (s *ChatService) EnsureSession(ctx context.Context, sessionID string) (*domain.Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if sessionID != "" {
		existing, err := s.store.GetSessionByID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}
	return s.store.CreateSession(ctx, "New Chat")
}

// EnsureMessagePlatformSession 确保消息平台桥接会话存在，并固定使用预定义 ID。
func (s *ChatService) EnsureMessagePlatformSession(ctx context.Context) (*domain.Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	session, err := s.store.GetSessionByID(ctx, constants.MessagePlatformSessionID)
	if err != nil {
		return nil, err
	}
	if session != nil {
		return session, nil
	}
	return s.store.CreateSessionWithID(ctx, constants.MessagePlatformSessionID, constants.MessagePlatformSessionName)
}

// ResolvePlatformModel 为消息平台入口解析默认模型，优先平台级设置，再回退全局设置和首个可用模型。
func (s *ChatService) ResolvePlatformModel(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.platformModelMu.Lock()
	cacheID := s.platformModelID
	cacheAt := s.platformModelAt
	s.platformModelMu.Unlock()
	if cacheID != "" && time.Since(cacheAt) < platformModelCacheTTL {
		item, err := s.store.GetLLMConfigByID(ctx, cacheID)
		if err != nil {
			return "", err
		}
		if item != nil {
			return cacheID, nil
		}
	}

	// 统一校验候选模型 ID 是否存在，避免重复写 store 查询逻辑。
	resolveModel := func(modelID string) (string, bool, error) {
		trimmed := strings.TrimSpace(modelID)
		if trimmed == "" {
			return "", false, nil
		}
		item, err := s.store.GetLLMConfigByID(ctx, trimmed)
		if err != nil {
			return "", false, err
		}
		if item == nil {
			return "", false, nil
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
		s.platformModelMu.Lock()
		s.platformModelID = id
		s.platformModelAt = time.Now()
		s.platformModelMu.Unlock()
		return id, nil
	}

	globalDefault, err := s.store.GetSetting(ctx, constants.SettingDefaultModel)
	if err != nil {
		return "", err
	}
	if id, ok, err := resolveModel(globalDefault); err != nil {
		return "", err
	} else if ok {
		_ = s.store.SetSetting(ctx, constants.SettingMessagePlatformDefaultModel, id)
		s.platformModelMu.Lock()
		s.platformModelID = id
		s.platformModelAt = time.Now()
		s.platformModelMu.Unlock()
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
	_ = s.store.SetSetting(ctx, constants.SettingMessagePlatformDefaultModel, fallbackID)
	s.platformModelMu.Lock()
	s.platformModelID = fallbackID
	s.platformModelAt = time.Now()
	s.platformModelMu.Unlock()
	return fallbackID, nil
}

// ResolveLLMConfig 读取并校验模型配置，保证发起请求前关键字段完整。
func (s *ChatService) ResolveLLMConfig(ctx context.Context, modelID string) (*domain.LLMConfig, error) {
	configID := strings.TrimSpace(modelID)
	if configID == "" {
		return nil, fmt.Errorf("modelId is required.")
	}

	config, err := s.store.GetLLMConfigByID(ctx, configID)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, fmt.Errorf("Model config not found: %s.", configID)
	}
	if strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("Model config is incomplete: %s.", config.Name)
	}
	return config, nil
}
