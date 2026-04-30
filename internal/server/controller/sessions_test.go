package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
	sessionsvc "slimebot/internal/services/session"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestCreateSession_ReturnsBadRequestForMalformedJSON(t *testing.T) {
	controller := NewHTTPController(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewBufferString(`{"name":`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	controller.CreateSession(NewChiContext(resp, req))

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", resp.Code, resp.Body.String())
	}
}

type sessionServiceStub struct {
	messages        []domain.Message
	toolRecords     []domain.ToolCallRecord
	thinkingRecords []domain.ThinkingRecord
}

func (s sessionServiceStub) List(ctx context.Context, limit int, offset int, query string) (sessionsvc.ListResult, error) {
	return sessionsvc.ListResult{}, nil
}

func (s sessionServiceStub) Create(ctx context.Context, name string) (*domain.Session, error) {
	return nil, nil
}

func (s sessionServiceStub) RenameByUser(ctx context.Context, id, name string) error {
	return nil
}

func (s sessionServiceStub) Delete(ctx context.Context, id string) error {
	return nil
}

func (s sessionServiceStub) GetMessageHistory(ctx context.Context, sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) (sessionsvc.MessageHistoryPage, error) {
	messageIDSet := make(map[string]struct{}, len(s.messages))
	interruptedAssistantIDs := make(map[string]struct{}, len(s.messages))
	for _, message := range s.messages {
		messageIDSet[message.ID] = struct{}{}
		if message.Role == "assistant" && message.IsInterrupted {
			interruptedAssistantIDs[message.ID] = struct{}{}
		}
	}
	return sessionsvc.MessageHistoryPage{
		Messages:                        s.messages,
		ToolCallsByAssistantMessageID:   testBuildToolCallHistory(s.toolRecords, messageIDSet, interruptedAssistantIDs),
		ThinkingByAssistantMessageID:    testBuildThinkingHistory(s.thinkingRecords, messageIDSet, interruptedAssistantIDs),
		ReplyTimingByAssistantMessageID: testBuildReplyTiming(s.messages),
	}, nil
}

func testFormatHistoryTime(value time.Time) string {
	return value.Format("2006-01-02T15:04:05.000Z07:00")
}

func testBuildToolCallHistory(records []domain.ToolCallRecord, messageIDSet, interruptedAssistantIDs map[string]struct{}) map[string][]sessionsvc.ToolCallHistory {
	byAssistantID := make(map[string][]sessionsvc.ToolCallHistory)
	for _, record := range records {
		if record.AssistantMessageID == nil {
			continue
		}
		key := *record.AssistantMessageID
		if _, ok := messageIDSet[key]; !ok {
			continue
		}
		status := record.Status
		errText := record.Error
		if _, interrupted := interruptedAssistantIDs[key]; interrupted && (status == constants.ToolCallStatusPending || status == constants.ToolCallStatusExecuting) {
			status = constants.ToolCallStatusError
			if errText == "" {
				errText = "Execution cancelled."
			}
		}
		byAssistantID[key] = append(byAssistantID[key], sessionsvc.ToolCallHistory{
			ToolCallID:       record.ToolCallID,
			ToolName:         record.ToolName,
			Command:          record.Command,
			Params:           map[string]string{},
			Status:           status,
			RequiresApproval: record.RequiresApproval,
			ParentToolCallID: record.ParentToolCallID,
			SubagentRunID:    record.SubagentRunID,
			Output:           record.Output,
			Error:            errText,
			StartedAt:        testFormatHistoryTime(record.StartedAt),
		})
	}
	return byAssistantID
}

func testBuildThinkingHistory(records []domain.ThinkingRecord, messageIDSet, interruptedAssistantIDs map[string]struct{}) map[string][]sessionsvc.ThinkingHistory {
	byAssistantID := make(map[string][]sessionsvc.ThinkingHistory)
	for _, record := range records {
		if record.AssistantMessageID == nil {
			continue
		}
		key := *record.AssistantMessageID
		if _, ok := messageIDSet[key]; !ok {
			continue
		}
		status := record.Status
		if _, interrupted := interruptedAssistantIDs[key]; interrupted && status == "streaming" {
			status = "completed"
		}
		byAssistantID[key] = append(byAssistantID[key], sessionsvc.ThinkingHistory{
			ThinkingID:       record.ThinkingID,
			ParentToolCallID: record.ParentToolCallID,
			SubagentRunID:    record.SubagentRunID,
			Content:          record.Content,
			Status:           status,
			StartedAt:        testFormatHistoryTime(record.StartedAt),
			DurationMs:       record.DurationMs,
		})
	}
	return byAssistantID
}

func testBuildReplyTiming(messages []domain.Message) map[string]sessionsvc.ReplyTiming {
	byAssistantID := make(map[string]sessionsvc.ReplyTiming)
	var previousUser *domain.Message
	for idx := range messages {
		message := messages[idx]
		switch message.Role {
		case "user":
			previousUser = &messages[idx]
		case "assistant":
			if previousUser == nil {
				continue
			}
			byAssistantID[message.ID] = sessionsvc.ReplyTiming{
				StartedAt:  testFormatHistoryTime(previousUser.CreatedAt),
				FinishedAt: testFormatHistoryTime(message.CreatedAt),
				DurationMs: message.CreatedAt.Sub(previousUser.CreatedAt).Milliseconds(),
			}
			previousUser = nil
		}
	}
	return byAssistantID
}

func TestListMessages_ReturnsReplyTimingForAssistantMessages(t *testing.T) {
	sessionID := "session-1"
	userAt := time.Date(2026, 4, 29, 1, 2, 3, 0, time.UTC)
	assistantAt := userAt.Add(2500 * time.Millisecond)
	controller := NewHTTPController(nil, sessionServiceStub{messages: []domain.Message{
		{ID: "user-1", SessionID: sessionID, Role: "user", Content: "hello", CreatedAt: userAt, Seq: 1},
		{ID: "assistant-1", SessionID: sessionID, Role: "assistant", Content: "hi", CreatedAt: assistantAt, Seq: 2},
	}}, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/sessions/"+sessionID+"/messages", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", sessionID)
	req = req.WithContext(contextWithRoute(req.Context(), routeCtx))
	resp := httptest.NewRecorder()
	ctx := NewChiContext(resp, req)

	controller.ListMessages(ctx)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !bytes.Contains([]byte(body), []byte(`"replyTimingByAssistantMessageId"`)) {
		t.Fatalf("expected reply timing map in response body: %s", body)
	}
	if !bytes.Contains([]byte(body), []byte(`"assistant-1":{"startedAt":"2026-04-29T01:02:03.000Z","finishedAt":"2026-04-29T01:02:05.500Z","durationMs":2500}`)) {
		t.Fatalf("unexpected reply timing body: %s", body)
	}
}

func TestListMessages_NormalizesInterruptedOpenToolAndThinkingHistory(t *testing.T) {
	sessionID := "session-1"
	assistantID := "assistant-1"
	assistantIDPtr := assistantID
	assistantAt := time.Date(2026, 4, 29, 1, 2, 3, 0, time.UTC)
	controller := NewHTTPController(nil, sessionServiceStub{
		messages: []domain.Message{{
			ID:            assistantID,
			SessionID:     sessionID,
			Role:          "assistant",
			Content:       "<!-- TOOL_CALL:parent-tool -->",
			IsInterrupted: true,
			CreatedAt:     assistantAt,
			Seq:           1,
		}},
		toolRecords: []domain.ToolCallRecord{{
			ToolCallID:         "parent-tool",
			ToolName:           "run_subagent",
			Command:            "delegate",
			ParamsJSON:         `{}`,
			Status:             "executing",
			RequiresApproval:   false,
			AssistantMessageID: &assistantIDPtr,
			StartedAt:          assistantAt,
		}},
		thinkingRecords: []domain.ThinkingRecord{{
			ThinkingID:         "think-child",
			ParentToolCallID:   "parent-tool",
			SubagentRunID:      "sub-run",
			Content:            "child reasoning",
			Status:             "streaming",
			AssistantMessageID: &assistantIDPtr,
			StartedAt:          assistantAt,
		}},
	}, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/sessions/"+sessionID+"/messages", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", sessionID)
	req = req.WithContext(contextWithRoute(req.Context(), routeCtx))
	resp := httptest.NewRecorder()

	controller.ListMessages(NewChiContext(resp, req))

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !bytes.Contains([]byte(body), []byte(`"status":"error"`)) || !bytes.Contains([]byte(body), []byte(`"error":"Execution cancelled."`)) {
		t.Fatalf("expected interrupted open tool to be returned as error: %s", body)
	}
	if !bytes.Contains([]byte(body), []byte(`"thinkingId":"think-child"`)) || !bytes.Contains([]byte(body), []byte(`"status":"completed"`)) {
		t.Fatalf("expected interrupted streaming thinking to be returned as completed: %s", body)
	}
}

func contextWithRoute(parent context.Context, routeCtx *chi.Context) context.Context {
	return context.WithValue(parent, chi.RouteCtxKey, routeCtx)
}
