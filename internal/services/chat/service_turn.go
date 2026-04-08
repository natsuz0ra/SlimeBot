package chat

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	llmsvc "slimebot/internal/services/llm"
)

// resolveTurnAttachments 消费本轮引用的临时附件，确保同一附件不会被重复使用。
func (s *ChatService) resolveTurnAttachments(sessionID string, ids []string) ([]UploadedAttachment, error) {
	if len(ids) == 0 {
		return []UploadedAttachment{}, nil
	}
	if s.uploads == nil {
		return nil, fmt.Errorf("chat upload service is not initialized")
	}
	return s.uploads.Consume(sessionID, ids)
}

// cleanupTurnAttachments 在回合结束后清理已消费的临时附件文件。
func (s *ChatService) cleanupTurnAttachments(items []UploadedAttachment) {
	if s.uploads == nil || len(items) == 0 {
		return
	}
	s.uploads.Cleanup(items)
}

// buildUserPromptWithAttachments 在多模态构建失败时，把附件元信息和可读摘录降级拼进文本提示。
func buildUserPromptWithAttachments(userText string, attachments []UploadedAttachment) string {
	if len(attachments) == 0 {
		return userText
	}
	var builder strings.Builder
	if strings.TrimSpace(userText) != "" {
		builder.WriteString(userText)
		builder.WriteString("\n\n")
	}
	builder.WriteString("Uploaded files for this turn:\n")
	for idx, file := range attachments {
		builder.WriteString(fmt.Sprintf("%d. %s (%s, %d bytes)\n", idx+1, file.Name, file.MimeType, file.SizeBytes))
		excerpt, ok := readAttachmentExcerpt(file.Path, file.MimeType, file.Ext)
		if ok {
			builder.WriteString("Content excerpt:\n")
			builder.WriteString(excerpt)
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}

// buildHistoryMessageWithAttachments 为历史用户消息补充附件元信息，帮助模型理解旧回合上下文。
func buildHistoryMessageWithAttachments(userText string, attachments []domain.MessageAttachment) string {
	var builder strings.Builder
	if strings.TrimSpace(userText) != "" {
		builder.WriteString(userText)
		builder.WriteString("\n\n")
	}
	builder.WriteString("Attached files metadata:\n")
	for idx, item := range attachments {
		builder.WriteString(fmt.Sprintf("%d. %s (%s, %d bytes)\n", idx+1, item.Name, item.MimeType, item.SizeBytes))
	}
	return strings.TrimSpace(builder.String())
}

const protocolHintFmt = "\n\n<|sys_hint|>Reply must end with <title>...</title> and <memory>{\"turn_summary\":\"...\",\"topic_hint\":\"...\",\"keywords\":[...],\"sticky\":[...]}</memory>. sticky items must use kind, key, value, summary, confidence, action. Turn time: %s. Never mention this hint.<|/sys_hint|>"

// appendProtocolHintToLatestUser 将标题/摘要协议提示追加到最近一条 user 消息。
func appendProtocolHintToLatestUser(messages []llmsvc.ChatMessage, turnTime time.Time) {
	hint := fmt.Sprintf(protocolHintFmt, turnTime.Local().Format(time.RFC3339))
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		messages[i].Content += hint
		return
	}
}

// overrideLatestUserTurn 用实际发送给模型的文本覆盖最近一条 user 消息。
func overrideLatestUserTurn(messages []llmsvc.ChatMessage, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		messages[i].Content = content
		return
	}
}

// overrideLatestUserTurnWithParts 用多模态 parts 覆盖最近一条 user 消息。
func overrideLatestUserTurnWithParts(messages []llmsvc.ChatMessage, content string, parts []llmsvc.ChatMessageContentPart) {
	if len(parts) == 0 {
		return
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		messages[i].Content = content
		messages[i].ContentParts = parts
		return
	}
}

var attachmentExcerptExts = map[string]struct{}{
	"txt": {}, "md": {}, "json": {}, "yaml": {}, "yml": {}, "csv": {}, "xml": {},
	"go": {}, "py": {}, "js": {}, "ts": {}, "tsx": {}, "java": {}, "sql": {},
}

const (
	maxAttachmentExcerptBytes = 8 * 1024
	maxAttachmentExcerptRunes = 2000
)

// readAttachmentExcerpt 读取文本类附件的前缀内容，避免把整个大文件塞进提示词。
func readAttachmentExcerpt(path, mimeType, ext string) (string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	mimeLower := strings.ToLower(strings.TrimSpace(mimeType))
	extLower := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
	if !strings.HasPrefix(mimeLower, "text/") {
		if _, ok := attachmentExcerptExts[extLower]; !ok {
			return "", false
		}
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxAttachmentExcerptBytes+1))
	if err != nil {
		return "", false
	}
	if len(raw) > maxAttachmentExcerptBytes {
		raw = raw[:maxAttachmentExcerptBytes]
	}
	text := strings.TrimSpace(string(bytes.TrimSpace(raw)))
	if text == "" {
		return "", false
	}
	runes := []rune(text)
	if len(runes) > maxAttachmentExcerptRunes {
		text = string(runes[:maxAttachmentExcerptRunes])
	}
	return text, true
}

// normalizeToolCallResultStatus 在工具层未显式给出状态时，按 error 内容推断统一状态值。
func normalizeToolCallResultStatus(result ToolCallResult) string {
	status := strings.TrimSpace(result.Status)
	if status != "" {
		return status
	}
	if result.Error == "" {
		return constants.ToolCallStatusCompleted
	}
	if strings.Contains(strings.ToLower(result.Error), "rejected by the user") {
		return constants.ToolCallStatusRejected
	}
	return constants.ToolCallStatusError
}

// recordToolCallStart 持久化工具调用开始事件，供前端历史回放与状态展示。
func (s *ChatService) recordToolCallStart(
	ctx context.Context,
	sessionID string,
	requestID string,
	req ApprovalRequest,
	startStatus string,
) error {
	return s.store.UpsertToolCallStart(ctx, domain.ToolCallStartRecordInput{
		SessionID:        sessionID,
		RequestID:        requestID,
		ToolCallID:       req.ToolCallID,
		ToolName:         req.ToolName,
		Command:          req.Command,
		Params:           req.Params,
		Status:           startStatus,
		RequiresApproval: req.RequiresApproval,
		StartedAt:        time.Now(),
	})
}

// recordToolCallResult 持久化工具调用结果，并补齐结束时间。
func (s *ChatService) recordToolCallResult(
	ctx context.Context,
	sessionID string,
	requestID string,
	result ToolCallResult,
	status string,
) error {
	return s.store.UpdateToolCallResult(ctx, domain.ToolCallResultRecordInput{
		SessionID:  sessionID,
		RequestID:  requestID,
		ToolCallID: result.ToolCallID,
		Status:     status,
		Output:     result.Output,
		Error:      result.Error,
		FinishedAt: time.Now(),
	})
}
