package platforms

import (
	"context"
	"time"

	chatsvc "slimebot/internal/services/chat"
)

// ApprovalBroker 抽象审批注册、等待与回调解析能力。
type ApprovalBroker interface {
	Register(toolCallID string, chatID string, ttl time.Duration) (string, string, error)
	Wait(ctx context.Context, toolCallID string) (*chatsvc.ApprovalResponse, error)
	ResolveByCallback(chatID string, callbackData string) (bool, error)
	Remove(toolCallID string)
}
