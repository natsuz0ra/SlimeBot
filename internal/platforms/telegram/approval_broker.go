package telegram

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	chatsvc "slimebot/internal/services/chat"
)

const (
	approvalApprovePrefix = "ap"
	approvalRejectPrefix  = "rj"
)

type pendingApproval struct {
	toolCallID string
	chatID     string
	token      string
	expireAt   time.Time
	ch         chan chatsvc.ApprovalResponse
}

type approvalBroker struct {
	mu         sync.Mutex
	byToolCall map[string]*pendingApproval
	byToken    map[string]*pendingApproval
	now        func() time.Time
}

func NewApprovalBroker() *approvalBroker {
	return &approvalBroker{
		byToolCall: make(map[string]*pendingApproval),
		byToken:    make(map[string]*pendingApproval),
		now:        time.Now,
	}
}

// Register 为一次待审批工具调用创建上下文，并返回批准/拒绝 callback_data。
// 内部维护 toolCallID 与 token 双索引，分别服务等待方与回调解析方。
func (b *approvalBroker) Register(toolCallID string, chatID string, ttl time.Duration) (string, string, error) {
	if b == nil {
		return "", "", fmt.Errorf("approval broker is nil")
	}
	toolCallID = strings.TrimSpace(toolCallID)
	chatID = strings.TrimSpace(chatID)
	if toolCallID == "" || chatID == "" {
		return "", "", fmt.Errorf("toolCallID/chatID is required")
	}
	expireAt := b.now().Add(ttl)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.cleanupExpiredLocked(b.now())
	token, err := b.newTokenLocked()
	if err != nil {
		return "", "", err
	}
	if old, ok := b.byToolCall[toolCallID]; ok {
		delete(b.byToken, old.token)
		delete(b.byToolCall, toolCallID)
	}
	entry := &pendingApproval{
		toolCallID: toolCallID,
		chatID:     chatID,
		token:      token,
		expireAt:   expireAt,
		ch:         make(chan chatsvc.ApprovalResponse, 1),
	}
	b.byToolCall[toolCallID] = entry
	b.byToken[token] = entry
	return approvalApprovePrefix + ":" + token, approvalRejectPrefix + ":" + token, nil
}

// Wait 阻塞等待某个 toolCallID 的审批结果，直到收到回调或 ctx 取消/超时。
func (b *approvalBroker) Wait(ctx context.Context, toolCallID string) (*chatsvc.ApprovalResponse, error) {
	if b == nil {
		return nil, fmt.Errorf("approval broker is nil")
	}
	toolCallID = strings.TrimSpace(toolCallID)
	b.mu.Lock()
	entry, ok := b.byToolCall[toolCallID]
	if !ok {
		b.mu.Unlock()
		return nil, fmt.Errorf("approval context not found")
	}
	ch := entry.ch
	b.mu.Unlock()

	select {
	case <-ctx.Done():
		b.Remove(toolCallID)
		return nil, ctx.Err()
	case resp := <-ch:
		return &resp, nil
	}
}

// ResolveByCallback 解析按钮回调并投递审批结果；
// 仅允许原 chatID 消费该 token，防止跨会话误操作。
func (b *approvalBroker) ResolveByCallback(chatID string, callbackData string) (bool, error) {
	if b == nil {
		return false, fmt.Errorf("approval broker is nil")
	}
	chatID = strings.TrimSpace(chatID)
	action, token, err := parseApprovalCallbackData(callbackData)
	if err != nil {
		return false, err
	}

	now := b.now()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cleanupExpiredLocked(now)

	entry, ok := b.byToken[token]
	if !ok {
		return false, fmt.Errorf("approval token is invalid or expired")
	}
	if entry.chatID != chatID {
		return false, fmt.Errorf("approval token does not belong to this chat")
	}
	delete(b.byToken, entry.token)
	delete(b.byToolCall, entry.toolCallID)

	approved := action == approvalApprovePrefix
	select {
	case entry.ch <- chatsvc.ApprovalResponse{ToolCallID: entry.toolCallID, Approved: approved}:
	default:
	}
	return approved, nil
}

func (b *approvalBroker) Remove(toolCallID string) {
	if b == nil {
		return
	}
	toolCallID = strings.TrimSpace(toolCallID)
	b.mu.Lock()
	defer b.mu.Unlock()
	entry, ok := b.byToolCall[toolCallID]
	if !ok {
		return
	}
	delete(b.byToolCall, toolCallID)
	delete(b.byToken, entry.token)
}

// parseApprovalCallbackData 解析 callback_data 协议：<action>:<token>，
// 其中 action 只能是 ap(批准) 或 rj(拒绝)。
func parseApprovalCallbackData(raw string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid callback data")
	}
	action := strings.TrimSpace(parts[0])
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", "", fmt.Errorf("approval token is required")
	}
	if action != approvalApprovePrefix && action != approvalRejectPrefix {
		return "", "", fmt.Errorf("unsupported callback action")
	}
	return action, token, nil
}

func (b *approvalBroker) newTokenLocked() (string, error) {
	for i := 0; i < 3; i++ {
		buf := make([]byte, 6)
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		token := hex.EncodeToString(buf)
		_, exists := b.byToken[token]
		if !exists {
			return token, nil
		}
	}
	return "", fmt.Errorf("failed to generate unique approval token")
}

func (b *approvalBroker) cleanupExpiredLocked(now time.Time) {
	for toolCallID, entry := range b.byToolCall {
		if now.After(entry.expireAt) {
			delete(b.byToolCall, toolCallID)
			delete(b.byToken, entry.token)
		}
	}
}
