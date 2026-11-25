package chat

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"slimebot/internal/domain"
	"slimebot/internal/runtime/workspace"
	"slimebot/internal/services/llm"
)

type checkpointProvider struct{ file string }

func (p checkpointProvider) StreamChatWithTools(_ context.Context, _ llm.ModelRuntimeConfig, _ []llm.ChatMessage, _ []llm.ToolDef, cb llm.StreamCallbacks) (*llm.StreamResult, error) {
	if err := os.WriteFile(p.file, []byte("after turn\n"), 0644); err != nil {
		return nil, err
	}
	if cb.OnChunk != nil {
		if err := cb.OnChunk("Done."); err != nil {
			return nil, err
		}
	}
	return &llm.StreamResult{Type: llm.StreamResultText}, nil
}

func TestMainChatCapturesFileCheckpoint(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	root := t.TempDir()
	file := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(file, []byte("before turn\n"), 0644); err != nil {
		t.Fatal(err)
	}
	session, err := repo.CreateSession(ctx, "Checkpoint integration", root)
	if err != nil {
		t.Fatal(err)
	}
	config, err := repo.CreateLLMConfig(ctx, domain.LLMConfig{Name: "fake", Provider: llm.ProviderOpenAI, BaseURL: "http://fake.invalid", APIKey: "synthetic", Model: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewChatService(repo, repo, llm.NewFactory(checkpointProvider{file}), nil, nil)
	svc.SetFileCheckpoints(workspace.NewCheckpoints(t.TempDir()))
	if _, err := svc.HandleChatStream(ctx, session.ID, "checkpoint-turn", "test", "", config.ID, nil, "off", false, "", "", AgentCallbacks{}); err != nil {
		t.Fatal(err)
	}
	items, err := svc.ListFileCheckpoints(ctx, session.ID)
	if err != nil || len(items) != 1 || items[0].RequestID != "checkpoint-turn" || items[0].FileCount != 1 || items[0].UserMessageID == "" {
		t.Fatalf("checkpoints = %+v, %v", items, err)
	}
	release, err := svc.ReserveSessionMaintenance(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RollbackFileCheckpoint(ctx, session.ID, items[0].ID, "undo"); !errors.Is(err, workspace.ErrCheckpointBusy) {
		t.Errorf("allowing rollback during a turn: %v", err)
	}
	release()
	if err := svc.RollbackFileCheckpoint(ctx, session.ID, items[0].ID, "undo"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "before turn\n" {
		t.Fatalf("rollback = %q, %v", data, err)
	}
}
