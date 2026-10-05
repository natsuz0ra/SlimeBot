package app

import (
	"context"
	"strings"
	"testing"

	"slimebot/internal/domain"
	"slimebot/internal/repositories"
	chatsvc "slimebot/internal/services/chat"
	llmsvc "slimebot/internal/services/llm"
)

type scheduledRunnerProvider struct{}

func (p *scheduledRunnerProvider) StreamChatWithTools(
	_ context.Context,
	_ llmsvc.ModelRuntimeConfig,
	messages []llmsvc.ChatMessage,
	_ []llmsvc.ToolDef,
	callbacks llmsvc.StreamCallbacks,
) (*llmsvc.StreamResult, error) {
	if len(messages) > 0 && strings.Contains(messages[0].Content, "concise title generator") {
		if callbacks.OnChunk != nil {
			if err := callbacks.OnChunk(`{"title":"任务摘要"}`); err != nil {
				return nil, err
			}
		}
		return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
	}
	if callbacks.OnChunk != nil {
		if err := callbacks.OnChunk("scheduled answer"); err != nil {
			return nil, err
		}
	}
	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
}

func TestScheduleChatRunnerCreatesIsolatedTaskExecution(t *testing.T) {
	db := repositories.NewSQLiteDBTest(t, "app_schedule_runner_new_session")
	repo := repositories.New(db)
	ctx := context.Background()
	sourceSession, err := repo.CreateSession(ctx, "来源会话", "/tmp")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	model, err := repo.CreateLLMConfig(ctx, domain.LLMConfig{
		Name:     "fake",
		Provider: llmsvc.ProviderOpenAI,
		BaseURL:  "http://fake",
		APIKey:   "key",
		Model:    "fake-model",
	})
	if err != nil {
		t.Fatalf("CreateLLMConfig failed: %v", err)
	}
	chatService := chatsvc.NewChatService(repo, repo, llmsvc.NewFactory(&scheduledRunnerProvider{}), nil, nil)
	core := &Core{Repo: repo, ChatService: chatService}
	task := domain.ScheduledTask{
		ID:            "task-1",
		Name:          "每日摘要",
		Prompt:        "总结今天的项目状态",
		SessionID:     sourceSession.ID,
		ModelConfigID: model.ID,
		ThinkingLevel: "off",
	}

	result := scheduleChatRunner{core: core}.RunScheduledTask(ctx, task)
	if !result.Success {
		t.Fatalf("RunScheduledTask failed: %s", result.Error)
	}
	if result.SessionID == "" {
		t.Fatal("RunScheduledTask should return the new session ID")
	}
	if result.SessionID == sourceSession.ID {
		t.Fatalf("scheduled run used source session %q; want a new session", sourceSession.ID)
	}

	sourceMessages, _, err := repo.ListSessionMessagesPage(ctx, sourceSession.ID, 10, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("ListSessionMessagesPage(source) failed: %v", err)
	}
	if len(sourceMessages) != 0 {
		t.Fatalf("source session messages = %d, want 0", len(sourceMessages))
	}

	runMessages, _, err := repo.ListSessionMessagesPage(ctx, result.SessionID, 10, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("ListSessionMessagesPage(run) failed: %v", err)
	}
	if len(runMessages) != 2 {
		t.Fatalf("new session messages = %d, want 2", len(runMessages))
	}
	if runMessages[0].Role != "user" || runMessages[0].Content != task.Prompt {
		t.Fatalf("stored scheduled user message = (%s, %q), want display content", runMessages[0].Role, runMessages[0].Content)
	}
	if runMessages[1].Role != "assistant" || runMessages[1].Content != "scheduled answer" {
		t.Fatalf("stored scheduled assistant message = (%s, %q), want scheduled answer", runMessages[1].Role, runMessages[1].Content)
	}

	execution, err := repo.GetSessionByID(ctx, result.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Kind != domain.SessionKindTaskRun || execution.Name != task.Name || !execution.IsTitleLocked || execution.WorkingDirectory != "/tmp" {
		t.Fatalf("execution context: %+v", execution)
	}
	chats, err := repo.ListSessions(ctx, 100, 0, "")
	if err != nil || len(chats) != 1 || chats[0].ID != sourceSession.ID {
		t.Fatalf("ordinary chats polluted: %+v, %v", chats, err)
	}
	hits, err := repo.SearchChats(ctx, "scheduled answer", "all", 30, 0)
	if err != nil || len(hits) != 0 {
		t.Fatalf("task execution appears in chat search: %+v, %v", hits, err)
	}
	task.SessionID = ""
	task.WorkingDirectory = t.TempDir()
	next := scheduleChatRunner{core: core}.RunScheduledTask(ctx, task)
	if !next.Success || next.SessionID == result.SessionID {
		t.Fatalf("executions share a context: %+v", next)
	}
	independent, err := repo.GetSessionByID(ctx, next.SessionID)
	if err != nil || independent.WorkingDirectory != task.WorkingDirectory || independent.Kind != domain.SessionKindTaskRun {
		t.Fatalf("independent task context: %+v, %v", independent, err)
	}
}
