package platforms

import (
	"context"
	"fmt"
	"slimebot/internal/domain"
	"strings"
	"time"

	"slimebot/internal/constants"
	chatsvc "slimebot/internal/services/chat"

	"github.com/google/uuid"
)

// Dispatcher 消息平台入站统一入口：解析会话与模型，组装 AgentCallbacks 并调用 HandleChatStream。
type Dispatcher struct {
	chat      platformChatService
	approvals ApprovalBroker
}

type platformChatService interface {
	EnsureMessagePlatformSession(ctx context.Context) (*domain.Session, error)
	ResolvePlatformModel(ctx context.Context) (string, error)
	HandleChatStream(
		ctx context.Context,
		sessionID string,
		requestID string,
		content string,
		modelID string,
		attachmentIDs []string,
		callbacks chatsvc.AgentCallbacks,
	) (*chatsvc.ChatStreamResult, error)
}

func NewDispatcher(chat platformChatService, approvals ApprovalBroker) *Dispatcher {
	return &Dispatcher{
		chat:      chat,
		approvals: approvals,
	}
}

// HandleInbound 是消息平台的主入口：
// 1) 校验入站消息并解析 chat/session/model；
// 2) 组装 AgentCallbacks，串联工具开始、审批等待、工具结果回传；
// 3) 将最终模型回复发送回平台。
func (d *Dispatcher) HandleInbound(ctx context.Context, message InboundMessage, sender OutboundSender) error {
	if d == nil || d.chat == nil {
		return fmt.Errorf("dispatcher is not initialized")
	}
	content := strings.TrimSpace(message.Text)
	attachmentIDs := make([]string, 0, len(message.AttachmentIDs))
	for _, id := range message.AttachmentIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		attachmentIDs = append(attachmentIDs, trimmed)
	}
	if content == "" && len(attachmentIDs) == 0 {
		return nil
	}
	chatID := strings.TrimSpace(message.ChatID)
	if chatID == "" {
		return fmt.Errorf("chat id is required")
	}

	session, err := d.chat.EnsureMessagePlatformSession(ctx)
	if err != nil {
		return err
	}
	modelID, err := d.chat.ResolvePlatformModel(ctx)
	if err != nil {
		return err
	}

	callbacks := chatsvc.AgentCallbacks{
		OnChunk: func(_ string) error {
			return nil
		},
		OnToolCallStart: func(req chatsvc.ApprovalRequest) error {
			if req.RequiresApproval {
				if d.approvals == nil {
					return fmt.Errorf("dispatcher approvals is not initialized")
				}
				approveData, rejectData, err := d.approvals.Register(req.ToolCallID, chatID, constants.AgentApprovalTimeout+10*time.Second)
				if err != nil {
					return err
				}
				return sender.SendApprovalKeyboard(chatID, formatToolApprovalPrompt(req), approveData, rejectData)
			}
			return sender.SendText(chatID, formatToolStartSummary(req))
		},
		WaitApproval: func(waitCtx context.Context, toolCallID string) (*chatsvc.ApprovalResponse, error) {
			if d.approvals == nil {
				return nil, fmt.Errorf("dispatcher approvals is not initialized")
			}
			return d.approvals.Wait(waitCtx, toolCallID)
		},
		OnToolCallResult: func(result chatsvc.ToolCallResult) error {
			return sender.SendText(chatID, formatToolResultSummary(result))
		},
	}

	streamResult, err := d.chat.HandleChatStream(
		ctx,
		session.ID,
		uuid.NewString(),
		content,
		modelID,
		attachmentIDs,
		callbacks,
	)
	if err != nil {
		return err
	}
	if streamResult == nil {
		return nil
	}
	answer := strings.TrimSpace(streamResult.Answer)
	if answer == "" {
		answer = "The model returned no content."
	}
	return sender.SendText(chatID, answer)
}

// HandleTelegramApprovalCallback 只负责把 Telegram 回调转给审批 broker 解析并落地结果。
func (d *Dispatcher) HandleTelegramApprovalCallback(chatID string, callbackData string) (bool, error) {
	if d == nil || d.approvals == nil {
		return false, fmt.Errorf("dispatcher approvals is not initialized")
	}
	return d.approvals.ResolveByCallback(chatID, callbackData)
}

func formatToolStartSummary(req chatsvc.ApprovalRequest) string {
	details := make([]string, 0, len(req.Params)+1)
	if command := strings.TrimSpace(req.Command); command != "" {
		details = append(details, command)
	}
	params := make([]string, 0, len(req.Params))
	for key, value := range req.Params {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		if len(v) > 80 {
			v = v[:80] + "..."
		}
		params = append(params, fmt.Sprintf("%s=%s", key, v))
		if len(params) >= 3 {
			break
		}
	}
	details = append(details, params...)
	if len(details) == 0 {
		return fmt.Sprintf("Tool execution started: %s", req.ToolName)
	}
	return fmt.Sprintf("Tool execution started: %s (%s)", req.ToolName, strings.Join(details, ", "))
}

func formatToolResultSummary(result chatsvc.ToolCallResult) string {
	statusText := "failed"
	if strings.EqualFold(strings.TrimSpace(result.Status), "completed") {
		statusText = "succeeded"
	}
	if strings.TrimSpace(result.Error) == "" {
		return fmt.Sprintf("Tool execution %s: %s", statusText, result.ToolName)
	}
	return fmt.Sprintf("Tool execution %s: %s (%s)", statusText, result.ToolName, strings.TrimSpace(result.Error))
}

func formatToolApprovalPrompt(req chatsvc.ApprovalRequest) string {
	base := fmt.Sprintf("Tool execution requires approval: %s", req.ToolName)
	if strings.TrimSpace(req.Command) != "" {
		base = fmt.Sprintf("%s (%s)", base, strings.TrimSpace(req.Command))
	}
	return base + "\nPlease choose Approve or Reject."
}
