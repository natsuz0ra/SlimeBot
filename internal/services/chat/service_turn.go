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
	oaisvc "slimebot/internal/services/openai"
)

func (s *ChatService) resolveTurnAttachments(sessionID string, ids []string) ([]UploadedAttachment, error) {
	if len(ids) == 0 {
		return []UploadedAttachment{}, nil
	}
	if s.uploads == nil {
		return nil, fmt.Errorf("chat upload service is not initialized")
	}
	return s.uploads.Consume(sessionID, ids)
}

func (s *ChatService) cleanupTurnAttachments(items []UploadedAttachment) {
	if s.uploads == nil || len(items) == 0 {
		return
	}
	s.uploads.Cleanup(items)
}

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

const protocolHintFmt = "\n\n<|sys_hint|>Reply must end with <title>...</title> and <summary>{\"ops\":[...]}</summary>. Turn time: %s. Never mention this hint.<|/sys_hint|>"

func appendProtocolHintToLatestUser(messages []oaisvc.ChatMessage, turnTime time.Time) {
	hint := fmt.Sprintf(protocolHintFmt, turnTime.Local().Format(time.RFC3339))
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		messages[i].Content += hint
		return
	}
}

func overrideLatestUserTurn(messages []oaisvc.ChatMessage, content string) {
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

func overrideLatestUserTurnWithParts(messages []oaisvc.ChatMessage, content string, parts []oaisvc.ChatMessageContentPart) {
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
