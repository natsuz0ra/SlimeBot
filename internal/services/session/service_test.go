package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"slimebot/internal/domain"
)

type storeStub struct {
	seenCtx         context.Context
	sessions        []domain.Session
	messages        []domain.Message
	toolRecords     []domain.ToolCallRecord
	thinkingRecords []domain.ThinkingRecord
	teamRuns        []domain.TeamRun
	teamMembers     []domain.TeamMemberRun
}

func (s *storeStub) ListSessions(ctx context.Context, limit int, offset int, query string) ([]domain.Session, error) {
	s.seenCtx = ctx
	return s.sessions, nil
}

func (s *storeStub) CreateSession(ctx context.Context, name string, workingDirectory ...string) (*domain.Session, error) {
	s.seenCtx = ctx
	item := &domain.Session{ID: "created", Name: name}
	if len(workingDirectory) > 0 {
		item.WorkingDirectory = workingDirectory[0]
	}
	return item, nil
}

func (s *storeStub) RenameSessionByUser(ctx context.Context, id, name string) error {
	s.seenCtx = ctx
	return nil
}

func (s *storeStub) DeleteSession(ctx context.Context, id string) error {
	s.seenCtx = ctx
	return nil
}

func (s *storeStub) ListSessionMessagesPage(ctx context.Context, sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) ([]domain.Message, bool, error) {
	s.seenCtx = ctx
	return s.messages, false, nil
}

func (s *storeStub) ListSessionToolCallRecordsByAssistantMessageIDs(ctx context.Context, sessionID string, messageIDs []string) ([]domain.ToolCallRecord, error) {
	s.seenCtx = ctx
	return s.toolRecords, nil
}

func (s *storeStub) ListSessionThinkingRecordsByAssistantMessageIDs(ctx context.Context, sessionID string, messageIDs []string) ([]domain.ThinkingRecord, error) {
	s.seenCtx = ctx
	return s.thinkingRecords, nil
}

func (s *storeStub) ListSessionTeamRunsByAssistantMessageIDs(ctx context.Context, sessionID string, messageIDs []string) ([]domain.TeamRun, error) {
	s.seenCtx = ctx
	return s.teamRuns, nil
}

func (s *storeStub) ListSessionTeamMemberRunsByAssistantMessageIDs(ctx context.Context, sessionID string, messageIDs []string) ([]domain.TeamMemberRun, error) {
	s.seenCtx = ctx
	return s.teamMembers, nil
}

func TestCreateStoresValidatedWorkingDirectory(t *testing.T) {
	store := &storeStub{}
	service := NewSessionService(store)
	dir := t.TempDir()
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	item, err := service.Create(context.Background(), "project", dir)
	if err != nil || item.WorkingDirectory != canonical {
		t.Fatalf("working directory not stored: item=%+v err=%v", item, err)
	}
	if _, err := service.Create(context.Background(), "invalid", "relative/path"); !errors.Is(err, ErrInvalidWorkingDirectory) {
		t.Fatalf("expected invalid working directory, got %v", err)
	}
}

func TestBrowseWorkingDirectoryListsFoldersOnly(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"beta", "Alpha"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	listing, err := BrowseWorkingDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Directories) != 2 || listing.Directories[0].Name != "Alpha" || listing.Directories[1].Name != "beta" {
		t.Fatalf("unexpected directories: %+v", listing.Directories)
	}
	if listing.Parent != filepath.Dir(listing.Path) {
		t.Fatalf("unexpected parent: %s", listing.Parent)
	}
}

func TestCommonDirectoryPlacesOnlyIncludesExistingFolders(t *testing.T) {
	home := t.TempDir()
	for _, name := range []string{"Documents", "Downloads"} {
		if err := os.Mkdir(filepath.Join(home, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_DESKTOP_DIR", "")
	t.Setenv("XDG_DOCUMENTS_DIR", "")
	t.Setenv("XDG_DOWNLOAD_DIR", "")
	t.Setenv("OneDrive", "")
	places := commonDirectoryPlaces(home)
	got := map[string]bool{}
	for _, place := range places {
		got[place.Kind] = true
	}
	if !got["home"] || !got["documents"] || !got["downloads"] || got["desktop"] {
		t.Fatalf("unexpected common places: %+v", places)
	}
}

func TestCommonDirectoryPlacesUsesXDGLocations(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG user directories apply on Linux")
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, "My Downloads"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := []byte(`XDG_DOWNLOAD_DIR="$HOME/My Downloads"`)
	if err := os.WriteFile(filepath.Join(home, ".config", "user-dirs.dirs"), config, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DOWNLOAD_DIR", "")
	for _, place := range commonDirectoryPlaces(home) {
		if place.Kind == "downloads" {
			if place.Path != filepath.Join(home, "My Downloads") {
				t.Fatalf("unexpected XDG downloads path: %s", place.Path)
			}
			return
		}
	}
	t.Fatal("XDG downloads directory was not listed")
}

func TestGetMessageHistoryBuildsToolThinkingAndReplyTiming(t *testing.T) {
	sessionID := "session-1"
	assistantID := "assistant-1"
	assistantIDPtr := assistantID
	userAt := time.Date(2026, 4, 29, 1, 2, 3, 0, time.UTC)
	assistantAt := userAt.Add(2500 * time.Millisecond)
	store := &storeStub{
		messages: []domain.Message{
			{ID: "user-1", SessionID: sessionID, Role: "user", Content: "hello", CreatedAt: userAt, Seq: 1},
			{ID: assistantID, SessionID: sessionID, Role: "assistant", Content: "hi", IsInterrupted: true, CreatedAt: assistantAt, Seq: 2},
		},
		toolRecords: []domain.ToolCallRecord{{
			ToolCallID:         "tool-1",
			ToolName:           "exec",
			Command:            "run",
			ParamsJSON:         `{"cmd":"pwd"}`,
			Status:             "executing",
			MetadataJSON:       `{"filePath":"a.txt","operation":"Update"}`,
			AssistantMessageID: &assistantIDPtr,
			StartedAt:          assistantAt,
		}},
		thinkingRecords: []domain.ThinkingRecord{{
			ThinkingID:         "think-1",
			Content:            "reasoning",
			Status:             "streaming",
			AssistantMessageID: &assistantIDPtr,
			StartedAt:          assistantAt,
		}},
	}

	got, err := NewSessionService(store).GetMessageHistory(context.Background(), sessionID, 10, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("GetMessageHistory failed: %v", err)
	}

	if got.ReplyTimingByAssistantMessageID[assistantID].DurationMs != 2500 {
		t.Fatalf("unexpected reply timing: %+v", got.ReplyTimingByAssistantMessageID)
	}
	tool := got.ToolCallsByAssistantMessageID[assistantID][0]
	if tool.Status != "error" || tool.Error != "Execution cancelled." || tool.Params["cmd"] != "pwd" {
		t.Fatalf("unexpected tool history: %+v", tool)
	}
	metadata, ok := tool.Metadata.(map[string]any)
	if !ok || metadata["filePath"] != "a.txt" {
		t.Fatalf("unexpected tool metadata: %#v", tool.Metadata)
	}
	thinking := got.ThinkingByAssistantMessageID[assistantID][0]
	if thinking.Status != "completed" || thinking.Content != "reasoning" {
		t.Fatalf("unexpected thinking history: %+v", thinking)
	}
}

func TestGetMessageHistoryIncludesTeamRunsAndMembers(t *testing.T) {
	assistantID := "assistant-1"
	assistantIDPtr := assistantID
	store := &storeStub{
		messages: []domain.Message{{ID: assistantID, SessionID: "session-1", Role: "assistant", Content: "answer"}},
		teamRuns: []domain.TeamRun{{
			ID: "team-1", SessionID: "session-1", RequestID: "request-1", AssistantMessageID: &assistantIDPtr, Status: domain.TeamRunStatusSucceeded,
		}},
		teamMembers: []domain.TeamMemberRun{{
			ID: "member-1", TeamRunID: "team-1", ToolCallID: "tool-1", Status: domain.TeamMemberRunStatusSucceeded,
		}},
	}

	got, err := NewSessionService(store).GetMessageHistory(context.Background(), "session-1", 10, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("GetMessageHistory failed: %v", err)
	}
	if len(got.TeamRuns) != 1 || got.TeamRuns[0].ID != "team-1" {
		t.Fatalf("team runs = %#v", got.TeamRuns)
	}
	if len(got.TeamMemberRuns) != 1 || got.TeamMemberRuns[0].ID != "member-1" {
		t.Fatalf("team members = %#v", got.TeamMemberRuns)
	}
}

func TestGetMessageHistoryReplyTimingUsesEditedUserCreatedAt(t *testing.T) {
	sessionID := "session-1"
	originalUserAt := time.Date(2026, 4, 29, 1, 2, 3, 0, time.UTC)
	editedUserAt := originalUserAt.Add(2 * time.Hour)
	assistantAt := editedUserAt.Add(3200 * time.Millisecond)
	store := &storeStub{
		messages: []domain.Message{
			{ID: "user-1", SessionID: sessionID, Role: "user", Content: "edited", CreatedAt: editedUserAt, Seq: 1},
			{ID: "assistant-1", SessionID: sessionID, Role: "assistant", Content: "fresh answer", CreatedAt: assistantAt, Seq: 2},
		},
	}

	got, err := NewSessionService(store).GetMessageHistory(context.Background(), sessionID, 10, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("GetMessageHistory failed: %v", err)
	}

	timing := got.ReplyTimingByAssistantMessageID["assistant-1"]
	if timing.StartedAt != formatHistoryTime(editedUserAt) {
		t.Fatalf("expected edited user createdAt as reply start, got %+v", timing)
	}
	if timing.DurationMs != 3200 {
		t.Fatalf("expected duration from edited user time, got %+v", timing)
	}
}

func TestSessionServicePassesContextToStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &storeStub{}

	_, err := NewSessionService(store).Create(ctx, "demo")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if !errors.Is(store.seenCtx.Err(), context.Canceled) {
		t.Fatalf("expected canceled context to reach store, got %v", store.seenCtx.Err())
	}
}
