package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"slimebot/internal/domain"
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

func (s sessionServiceStub) List(limit int, offset int, query string) ([]domain.Session, error) {
	return nil, nil
}

func (s sessionServiceStub) Create(name string) (*domain.Session, error) {
	return nil, nil
}

func (s sessionServiceStub) RenameByUser(id, name string) error {
	return nil
}

func (s sessionServiceStub) Delete(id string) error {
	return nil
}

func (s sessionServiceStub) ListMessagesPage(sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) ([]domain.Message, bool, error) {
	return s.messages, false, nil
}

func (s sessionServiceStub) ListToolCallRecordsByAssistantMessageIDs(sessionID string, messageIDs []string) ([]domain.ToolCallRecord, error) {
	return s.toolRecords, nil
}

func (s sessionServiceStub) ListThinkingRecordsByAssistantMessageIDs(sessionID string, messageIDs []string) ([]domain.ThinkingRecord, error) {
	return s.thinkingRecords, nil
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
