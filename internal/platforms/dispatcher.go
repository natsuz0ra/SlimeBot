package platforms

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slimebot/internal/domain"
	"strings"
	"time"

	"slimebot/internal/constants"
	chatsvc "slimebot/internal/services/chat"

	"github.com/google/uuid"
)

const (
	telegramMessageChunkLimit       = 3500
	telegramApprovalCommandMaxRunes = 160
)

var telegramFinalMarkerRegex = regexp.MustCompile(`\n?<!-- (?:TOOL_CALL:.+?|THINKING:.+?|PLAN_START|PLAN_END) -->\n?`)

// Dispatcher is the inbound entry for message platforms: resolves session/model and calls HandleChatStream.
type Dispatcher struct {
	chat      platformChatService
	approvals ApprovalBroker
}

type platformChatService interface {
	EnsureMessagePlatformSession(ctx context.Context, platform string) (*domain.Session, error)
	ResolvePlatformModel(ctx context.Context) (string, error)
	ResolvePlatformRuntimeSettings(ctx context.Context) (string, string, error)
	HandleChatStream(
		ctx context.Context,
		sessionID string,
		requestID string,
		content string,
		displayContent string,
		modelID string,
		attachmentIDs []string,
		thinkingLevel string,
		planMode bool,
		subagentModelID string,
		approvalModeOverride string,
		callbacks chatsvc.AgentCallbacks,
	) (*chatsvc.ChatStreamResult, error)
}

func NewDispatcher(chat platformChatService, approvals ApprovalBroker) *Dispatcher {
	return &Dispatcher{
		chat:      chat,
		approvals: approvals,
	}
}

// HandleInbound is the main platform entry:
// 1) validate inbound payload and resolve chat/session/model;
// 2) wire AgentCallbacks for tool start, approval wait, and tool results;
// 3) send the final model reply back to the platform.
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

	session, err := d.chat.EnsureMessagePlatformSession(ctx, message.Platform)
	if err != nil {
		return err
	}
	modelID, err := d.chat.ResolvePlatformModel(ctx)
	if err != nil {
		return err
	}
	thinkingLevel, approvalMode, err := d.chat.ResolvePlatformRuntimeSettings(ctx)
	if err != nil {
		return err
	}

	// Track ask_questions toolCallIDs for auto-approval on platforms without Q&A UI.
	askQuestionsIDs := make(map[string]string) // toolCallID -> questions JSON

	callbacks := chatsvc.AgentCallbacks{
		OnChunk: func(_ string) error {
			return nil
		},
		OnToolCallStart: func(req chatsvc.ApprovalRequest) error {
			// ask_questions: skip approval UI, auto-approve in WaitApproval.
			if req.ToolName == constants.AskQuestionsTool {
				askQuestionsIDs[req.ToolCallID] = fmt.Sprintf("%v", req.Params["questions"])
				return nil
			}
			if req.ReviewStatus == string(chatsvc.ApprovalReviewStatusReviewing) {
				return sender.SendText(chatID, "Automatic approval review is checking this tool call: "+formatToolStartSummary(req))
			}
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
		OnToolApprovalReview: func(event chatsvc.ApprovalReviewEvent) error {
			if event.ReviewStatus == string(chatsvc.ApprovalReviewStatusReviewing) {
				return nil
			}
			text := fmt.Sprintf("Automatic approval review %s for %s.%s", event.ReviewStatus, event.ToolName, event.Command)
			if strings.TrimSpace(event.ReviewReason) != "" {
				text += "\nReason: " + strings.TrimSpace(event.ReviewReason)
			}
			return sender.SendText(chatID, text)
		},
		OnToolApprovalRequired: func(req chatsvc.ApprovalRequest) error {
			if d.approvals == nil {
				return fmt.Errorf("dispatcher approvals is not initialized")
			}
			approveData, rejectData, err := d.approvals.Register(req.ToolCallID, chatID, constants.AgentApprovalTimeout+10*time.Second)
			if err != nil {
				return err
			}
			return sender.SendApprovalKeyboard(chatID, formatToolApprovalPrompt(req), approveData, rejectData)
		},
		WaitApproval: func(waitCtx context.Context, toolCallID string) (*chatsvc.ApprovalResponse, error) {
			// ask_questions: auto-approve with a default response.
			if questionsJSON, ok := askQuestionsIDs[toolCallID]; ok {
				delete(askQuestionsIDs, toolCallID)
				defaultAnswers := buildDefaultAskQuestionsAnswers(questionsJSON)
				return &chatsvc.ApprovalResponse{ToolCallID: toolCallID, Approved: true, Answers: defaultAnswers}, nil
			}
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
		"",
		modelID,
		attachmentIDs,
		thinkingLevel,
		false,
		"",
		approvalMode,
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
	return sendTelegramFinalAnswer(chatID, answer, sender)
}

// HandleTelegramApprovalCallback forwards Telegram callback_data to the approval broker.
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
		v := strings.TrimSpace(fmt.Sprintf("%v", value))
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
	if command := formatToolCommandForApproval(req); command != "" {
		base += "\nCommand: " + command
	}
	return base + "\nPlease choose Approve or Reject."
}

func formatToolCommandForApproval(req chatsvc.ApprovalRequest) string {
	if strings.EqualFold(strings.TrimSpace(req.ToolName), constants.ExecToolName) {
		if value, ok := req.Params["command"]; ok {
			if command := trimTelegramApprovalCommand(fmt.Sprintf("%v", value)); command != "" {
				return command
			}
		}
		if value, ok := req.Params["cmd"]; ok {
			if command := trimTelegramApprovalCommand(fmt.Sprintf("%v", value)); command != "" {
				return command
			}
		}
		return ""
	}
	return trimTelegramApprovalCommand(req.Command)
}

func trimTelegramApprovalCommand(command string) string {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= telegramApprovalCommandMaxRunes {
		return trimmed
	}
	return string(runes[:telegramApprovalCommandMaxRunes]) + "..."
}

func sendTelegramFinalAnswer(chatID string, answer string, sender OutboundSender) error {
	for _, block := range splitFinalAnswerBlocks(answer) {
		for _, chunk := range splitTelegramTextForSend(block) {
			if err := sender.SendText(chatID, chunk); err != nil {
				return err
			}
		}
	}
	return nil
}

func splitFinalAnswerBlocks(answer string) []string {
	parts := telegramFinalMarkerRegex.Split(answer, -1)
	blocks := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		blocks = append(blocks, trimmed)
	}
	return blocks
}

func splitTelegramTextForSend(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len(text) <= telegramMessageChunkLimit {
		return []string{text}
	}

	chunks := make([]string, 0, len(text)/telegramMessageChunkLimit+1)
	for _, paragraph := range strings.Split(text, "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		chunks = appendWrappedTelegramChunk(chunks, paragraph)
	}
	return chunks
}

func appendWrappedTelegramChunk(chunks []string, text string) []string {
	if len(text) <= telegramMessageChunkLimit {
		return append(chunks, text)
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		runes := []rune(line)
		for len(runes) > telegramMessageChunkLimit {
			chunks = append(chunks, string(runes[:telegramMessageChunkLimit]))
			runes = runes[telegramMessageChunkLimit:]
		}
		if len(runes) > 0 {
			chunks = append(chunks, string(runes))
		}
	}
	return chunks
}

// buildDefaultAskQuestionsAnswers constructs a default answer response for ask_questions
// on platforms that cannot display the Q&A UI.
func buildDefaultAskQuestionsAnswers(questionsJSON string) string {
	type qItem struct {
		ID       string   `json:"id"`
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	var questions []qItem
	if err := json.Unmarshal([]byte(questionsJSON), &questions); err != nil {
		return `[{"questionId":"unknown","selectedOption":0,"customAnswer":"User cannot answer on this platform. Please use your best judgment."}]`
	}
	type answer struct {
		QuestionID     string `json:"questionId"`
		SelectedOption int    `json:"selectedOption"`
		CustomAnswer   string `json:"customAnswer"`
	}
	answers := make([]answer, len(questions))
	for i, q := range questions {
		answers[i] = answer{
			QuestionID:     q.ID,
			SelectedOption: -1,
			CustomAnswer:   "User cannot answer on this platform. Please use your best judgment.",
		}
	}
	b, _ := json.Marshal(answers)
	return string(b)
}
