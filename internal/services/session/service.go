package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

// SessionService orchestrates session use cases; controllers stay thin.
type SessionService struct {
	store domain.SessionStore
}

var ErrInvalidWorkingDirectory = errors.New("invalid working directory")

func ValidateWorkingDirectory(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: absolute path is required", ErrInvalidWorkingDirectory)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidWorkingDirectory, err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: path must be an existing directory", ErrInvalidWorkingDirectory)
	}
	return resolved, nil
}

type DirectoryEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type DirectoryListing struct {
	Path        string           `json:"path"`
	Parent      string           `json:"parent"`
	Directories []DirectoryEntry `json:"directories"`
	Places      []DirectoryPlace `json:"places"`
}

type DirectoryPlace struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

func BrowseWorkingDirectory(path string) (DirectoryListing, error) {
	if strings.TrimSpace(path) == "" {
		var err error
		path, err = os.UserHomeDir()
		if err != nil {
			return DirectoryListing{}, err
		}
	}
	resolved, err := ValidateWorkingDirectory(path)
	if err != nil {
		return DirectoryListing{}, err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return DirectoryListing{}, fmt.Errorf("%w: %v", ErrInvalidWorkingDirectory, err)
	}
	home, _ := os.UserHomeDir()
	listing := DirectoryListing{Path: resolved, Parent: filepath.Dir(resolved), Directories: []DirectoryEntry{}, Places: commonDirectoryPlaces(home)}
	for _, entry := range entries {
		isDirectory := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			info, statErr := os.Stat(filepath.Join(resolved, entry.Name()))
			isDirectory = statErr == nil && info.IsDir()
		}
		if !isDirectory {
			continue
		}
		listing.Directories = append(listing.Directories, DirectoryEntry{Name: entry.Name(), Path: filepath.Join(resolved, entry.Name())})
	}
	sort.Slice(listing.Directories, func(i, j int) bool {
		return strings.ToLower(listing.Directories[i].Name) < strings.ToLower(listing.Directories[j].Name)
	})
	return listing, nil
}

func commonDirectoryPlaces(home string) []DirectoryPlace {
	places := []DirectoryPlace{}
	seen := map[string]bool{}
	xdgDirs := xdgUserDirectories(home)
	add := func(kind, path string) {
		if path == "" || !filepath.IsAbs(path) {
			return
		}
		path = filepath.Clean(path)
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if info, err := os.Stat(path); err != nil || !info.IsDir() || seen[key] {
			return
		}
		seen[key] = true
		places = append(places, DirectoryPlace{Kind: kind, Path: path})
	}
	add("home", home)
	for _, item := range []struct{ kind, folder, env string }{
		{"desktop", "Desktop", "XDG_DESKTOP_DIR"},
		{"documents", "Documents", "XDG_DOCUMENTS_DIR"},
		{"downloads", "Downloads", "XDG_DOWNLOAD_DIR"},
	} {
		location := strings.Trim(os.Getenv(item.env), `"`)
		if location == "" {
			location = xdgDirs[item.env]
		}
		if location == "" {
			location = filepath.Join(home, item.folder)
		} else {
			location = strings.ReplaceAll(location, "$HOME", home)
		}
		add(item.kind, location)
	}
	add("projects", filepath.Join(home, "Projects"))
	add("onedrive", os.Getenv("OneDrive"))
	add("computer", filepath.VolumeName(home)+string(os.PathSeparator))
	return places
}

func xdgUserDirectories(home string) map[string]string {
	paths := map[string]string{}
	if runtime.GOOS != "linux" {
		return paths
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "user-dirs.dirs"))
	if err != nil {
		return paths
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || !strings.HasPrefix(key, "XDG_") {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		value = strings.ReplaceAll(value, "$HOME", home)
		if filepath.IsAbs(value) {
			paths[key] = value
		}
	}
	return paths
}

func WorkingDirectoryGitBranch(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "git", "-C", path, "symbolic-ref", "--quiet", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

type teamHistoryStore interface {
	ListSessionTeamRunsByAssistantMessageIDs(ctx context.Context, sessionID string, messageIDs []string) ([]domain.TeamRun, error)
	ListSessionTeamMemberRunsByAssistantMessageIDs(ctx context.Context, sessionID string, messageIDs []string) ([]domain.TeamMemberRun, error)
}

func NewSessionService(store domain.SessionStore) *SessionService {
	return &SessionService{store: store}
}

type ListResult struct {
	Sessions []domain.Session
	HasMore  bool
}

type MessageHistoryPage struct {
	Messages                        []domain.Message
	ToolCallsByAssistantMessageID   map[string][]ToolCallHistory
	ThinkingByAssistantMessageID    map[string][]ThinkingHistory
	ReplyTimingByAssistantMessageID map[string]ReplyTiming
	TeamRuns                        []domain.TeamRun
	TeamMemberRuns                  []domain.TeamMemberRun
	HasMore                         bool
}

type ToolCallHistory struct {
	ToolCallID       string         `json:"toolCallId"`
	ToolName         string         `json:"toolName"`
	Command          string         `json:"command"`
	Params           map[string]any `json:"params"`
	Status           string         `json:"status"`
	RequiresApproval bool           `json:"requiresApproval"`
	ParentToolCallID string         `json:"parentToolCallId,omitempty"`
	SubagentRunID    string         `json:"subagentRunId,omitempty"`
	Output           string         `json:"output,omitempty"`
	Error            string         `json:"error,omitempty"`
	Metadata         any            `json:"metadata,omitempty"`
	StartedAt        string         `json:"startedAt"`
	FinishedAt       string         `json:"finishedAt,omitempty"`
}

type ThinkingHistory struct {
	ThinkingID       string `json:"thinkingId"`
	ParentToolCallID string `json:"parentToolCallId,omitempty"`
	SubagentRunID    string `json:"subagentRunId,omitempty"`
	Content          string `json:"content"`
	Status           string `json:"status"`
	StartedAt        string `json:"startedAt"`
	FinishedAt       string `json:"finishedAt,omitempty"`
	DurationMs       int64  `json:"durationMs"`
}

type ReplyTiming struct {
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
	DurationMs int64  `json:"durationMs"`
}

func (s *SessionService) List(ctx context.Context, limit, offset int, query string) (ListResult, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	sessions, err := s.store.ListSessions(ctx, limit+1, offset, strings.TrimSpace(query))
	if err != nil {
		return ListResult{}, err
	}
	sessions, hasMore := fetchWindow(sessions, limit)
	return ListResult{Sessions: sessions, HasMore: hasMore}, nil
}

func (s *SessionService) Create(ctx context.Context, name string, workingDirectory ...string) (*domain.Session, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = "New Chat"
	}
	path := ""
	if len(workingDirectory) > 0 && strings.TrimSpace(workingDirectory[0]) != "" {
		var err error
		path, err = ValidateWorkingDirectory(workingDirectory[0])
		if err != nil {
			return nil, err
		}
	}
	return s.store.CreateSession(ctx, trimmed, path)
}

func (s *SessionService) RenameByUser(ctx context.Context, id string, name string) error {
	return s.store.RenameSessionByUser(ctx, id, strings.TrimSpace(name))
}

func (s *SessionService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteSession(ctx, id)
}

func (s *SessionService) GetMessageHistory(ctx context.Context, sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) (MessageHistoryPage, error) {
	messages, hasMore, err := s.store.ListSessionMessagesPage(ctx, sessionID, limit, before, beforeSeq, after, afterSeq)
	if err != nil {
		return MessageHistoryPage{}, err
	}
	messageIDSet := make(map[string]struct{}, len(messages))
	interruptedAssistantIDs := make(map[string]struct{}, len(messages))
	messageIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		messageIDSet[message.ID] = struct{}{}
		if message.Role == "assistant" && message.IsInterrupted {
			interruptedAssistantIDs[message.ID] = struct{}{}
		}
		messageIDs = append(messageIDs, message.ID)
	}

	records, err := s.store.ListSessionToolCallRecordsByAssistantMessageIDs(ctx, sessionID, messageIDs)
	if err != nil {
		return MessageHistoryPage{}, err
	}
	thinkingRecords, err := s.store.ListSessionThinkingRecordsByAssistantMessageIDs(ctx, sessionID, messageIDs)
	if err != nil {
		return MessageHistoryPage{}, err
	}
	teamRuns := []domain.TeamRun{}
	teamMemberRuns := []domain.TeamMemberRun{}
	if teamStore, ok := s.store.(teamHistoryStore); ok {
		teamRuns, err = teamStore.ListSessionTeamRunsByAssistantMessageIDs(ctx, sessionID, messageIDs)
		if err != nil {
			return MessageHistoryPage{}, err
		}
		teamMemberRuns, err = teamStore.ListSessionTeamMemberRunsByAssistantMessageIDs(ctx, sessionID, messageIDs)
		if err != nil {
			return MessageHistoryPage{}, err
		}
	}

	return MessageHistoryPage{
		Messages:                        messages,
		ToolCallsByAssistantMessageID:   buildToolCallHistory(records, messageIDSet, interruptedAssistantIDs),
		ThinkingByAssistantMessageID:    buildThinkingHistory(thinkingRecords, messageIDSet, interruptedAssistantIDs),
		ReplyTimingByAssistantMessageID: buildReplyTiming(messages),
		TeamRuns:                        teamRuns,
		TeamMemberRuns:                  teamMemberRuns,
		HasMore:                         hasMore,
	}, nil
}

func fetchWindow[T any](items []T, limit int) (trimmed []T, hasMore bool) {
	if len(items) > limit {
		return items[:limit], true
	}
	return items, false
}

func formatHistoryTime(value time.Time) string {
	return value.Format("2006-01-02T15:04:05.000Z07:00")
}

func parseToolCallParams(raw string) map[string]any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]any{}
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(trimmed), &params); err != nil {
		return map[string]any{}
	}
	return params
}

func parseToolCallMetadata(raw string) any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var metadata any
	if err := json.Unmarshal([]byte(trimmed), &metadata); err != nil {
		return nil
	}
	return metadata
}

func buildToolCallHistory(records []domain.ToolCallRecord, messageIDSet, interruptedAssistantIDs map[string]struct{}) map[string][]ToolCallHistory {
	byAssistantID := make(map[string][]ToolCallHistory)
	for _, record := range records {
		if record.AssistantMessageID == nil || strings.TrimSpace(*record.AssistantMessageID) == "" {
			continue
		}
		key := strings.TrimSpace(*record.AssistantMessageID)
		if _, ok := messageIDSet[key]; !ok {
			continue
		}
		status := record.Status
		errText := record.Error
		if _, interrupted := interruptedAssistantIDs[key]; interrupted && (status == constants.ToolCallStatusPending || status == constants.ToolCallStatusExecuting) {
			status = constants.ToolCallStatusError
			if strings.TrimSpace(errText) == "" {
				errText = "Execution cancelled."
			}
		}
		item := ToolCallHistory{
			ToolCallID:       record.ToolCallID,
			ToolName:         record.ToolName,
			Command:          record.Command,
			Params:           parseToolCallParams(record.ParamsJSON),
			Status:           status,
			RequiresApproval: record.RequiresApproval,
			ParentToolCallID: record.ParentToolCallID,
			SubagentRunID:    record.SubagentRunID,
			Output:           record.Output,
			Error:            errText,
			Metadata:         parseToolCallMetadata(record.MetadataJSON),
			StartedAt:        formatHistoryTime(record.StartedAt),
		}
		if record.FinishedAt != nil {
			item.FinishedAt = formatHistoryTime(*record.FinishedAt)
		}
		byAssistantID[key] = append(byAssistantID[key], item)
	}
	return byAssistantID
}

func buildThinkingHistory(records []domain.ThinkingRecord, messageIDSet, interruptedAssistantIDs map[string]struct{}) map[string][]ThinkingHistory {
	byAssistantID := make(map[string][]ThinkingHistory)
	for _, record := range records {
		if record.AssistantMessageID == nil || strings.TrimSpace(*record.AssistantMessageID) == "" {
			continue
		}
		key := strings.TrimSpace(*record.AssistantMessageID)
		if _, ok := messageIDSet[key]; !ok {
			continue
		}
		status := record.Status
		if _, interrupted := interruptedAssistantIDs[key]; interrupted && status == "streaming" {
			status = "completed"
		}
		item := ThinkingHistory{
			ThinkingID:       record.ThinkingID,
			ParentToolCallID: record.ParentToolCallID,
			SubagentRunID:    record.SubagentRunID,
			Content:          record.Content,
			Status:           status,
			StartedAt:        formatHistoryTime(record.StartedAt),
			DurationMs:       record.DurationMs,
		}
		if record.FinishedAt != nil {
			item.FinishedAt = formatHistoryTime(*record.FinishedAt)
		}
		byAssistantID[key] = append(byAssistantID[key], item)
	}
	return byAssistantID
}

func buildReplyTiming(messages []domain.Message) map[string]ReplyTiming {
	byAssistantID := make(map[string]ReplyTiming)
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
			durationMs := message.CreatedAt.Sub(previousUser.CreatedAt).Milliseconds()
			if durationMs < 0 {
				durationMs = 0
			}
			byAssistantID[message.ID] = ReplyTiming{
				StartedAt:  formatHistoryTime(previousUser.CreatedAt),
				FinishedAt: formatHistoryTime(message.CreatedAt),
				DurationMs: durationMs,
			}
			previousUser = nil
		}
	}
	return byAssistantID
}
