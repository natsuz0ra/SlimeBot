package repositories

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"slimebot/internal/domain"
	llm "slimebot/internal/services/llm"
)

func TestAgentForkIncludesOnlyClosedHistoryAndReclassifiesHumanSources(t *testing.T) {
	db := NewSQLiteDBTest(t, "agent_fork")
	repo := New(db)
	ctx := context.Background()
	if _, err := repo.CreateSessionWithID(ctx, "parent", "parent"); err != nil {
		t.Fatal(err)
	}
	for _, m := range []domain.Message{{ID: "user", SessionID: "parent", Role: "user", Seq: 1}, {ID: "assistant", SessionID: "parent", Role: "assistant", Seq: 2}} {
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	messages := []llm.ChatMessage{
		{Role: "user", SourceKind: "direct_user", Content: "original task"},
		{Role: "assistant", ToolCalls: []llm.ToolCallInfo{{ID: "closed", Name: "file_read"}}},
		{Role: "tool", ToolCallID: "closed", Content: "result"},
		{Role: "assistant", Content: "complete report"},
		{Role: "assistant", ToolCalls: []llm.ToolCallInfo{{ID: "inflight", Name: "file_read"}}},
	}
	for _, m := range messages {
		payload, _ := json.Marshal(m)
		kind := m.Role
		if m.SourceKind != "" {
			kind = m.SourceKind
		}
		entry := domain.ContextEntry{ScopeID: "parent", MessageSeq: 2, SourceMessageID: "assistant", SourceKind: kind, Payload: string(payload), Fidelity: "exact"}
		if err := db.Create(&entry).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.PutAgentRoot(ctx, &domain.AgentRootRequest{ID: "request", SessionID: "parent", Status: "running", Budget: 1000000}); err != nil {
		t.Fatal(err)
	}
	d := domain.AgentDescriptor{SessionID: "child", RootID: "parent", ParentID: "parent", RequestID: "request", ContextMode: "fork", Depth: 1}
	input := domain.AgentInbox{ID: "input", TargetID: "child", SenderID: "parent", ClientID: "input", RequestID: "request", Content: "child task"}
	if err := repo.CreateAgent(ctx, &d, &input); err != nil {
		t.Fatal(err)
	}
	var fork []domain.ContextEntry
	if err := db.Where("scope_id = ?", "child").Order("id").Find(&fork).Error; err != nil {
		t.Fatal(err)
	}
	if len(fork) != 4 {
		t.Fatalf("fork copied an open tool batch: %d", len(fork))
	}
	for _, entry := range fork {
		if entry.SourceMessageID != "" || entry.RequestID != "" || entry.MessageSeq != 0 {
			t.Fatal("parent binding reused")
		}
	}
	m, _ := fork[0].Message()
	if fork[0].SourceKind != "fork_background" || m.SourceKind != "fork_background" {
		t.Fatal("parent human source became a child authorization")
	}
}

func TestDeletingRootRemovesChildStateAndPreservesOtherRoots(t *testing.T) {
	db := NewSQLiteDBTest(t, "agent_delete")
	repo := New(db)
	ctx := context.Background()
	for _, id := range []string{"root", "other"} {
		if _, err := repo.CreateSessionWithID(ctx, id, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.PutAgentRoot(ctx, &domain.AgentRootRequest{ID: "request", SessionID: "root", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	d := domain.AgentDescriptor{SessionID: "child", RootID: "root", ParentID: "root", RequestID: "request", Depth: 1}
	m := domain.AgentInbox{ID: "input", TargetID: "child", ClientID: "input", RequestID: "request", Content: "task"}
	if err := repo.CreateAgent(ctx, &d, &m); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{
		&domain.AgentTurn{ID: "turn", RootID: "root", SessionID: "child", Status: "succeeded"},
		&domain.AgentTask{ID: "task", RootID: "root"},
		&domain.AgentArtifact{ID: "artifact", RootID: "root", AgentID: "child"},
		&domain.AgentApproval{ID: "approval", RootID: "root"},
		&domain.AgentEvent{RootID: "root", AgentID: "child", Kind: "finished"},
		&domain.ContextHead{ScopeID: "child"},
		&domain.ContextEntry{ScopeID: "child", Fidelity: "exact", Payload: `{}`},
		&domain.Message{ID: "child-message", SessionID: "child", Role: "assistant", CreatedAt: time.Now()},
	} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.DeleteSession(ctx, "root"); err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{&domain.AgentDescriptor{}, &domain.AgentInbox{}, &domain.AgentTurn{}, &domain.AgentRootRequest{}, &domain.AgentTask{}, &domain.AgentArtifact{}, &domain.AgentApproval{}, &domain.AgentEvent{}, &domain.ContextHead{}, &domain.ContextEntry{}, &domain.Message{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("left %T rows: %d %v", model, count, err)
		}
	}
	if _, err := repo.GetSessionByID(ctx, "child"); err == nil {
		t.Fatal("child session retained")
	}
	if _, err := repo.GetSessionByID(ctx, "other"); err != nil {
		t.Fatal("another root deleted")
	}
}

func TestRelayReceiptAndModelInputCommitTogether(t *testing.T) {
	db := NewSQLiteDBTest(t, "relay_atomic")
	repo := New(db)
	ctx := context.Background()
	if err := db.Create(&domain.ContextHead{ScopeID: "parent"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.PutAgentRoot(ctx, &domain.AgentRootRequest{ID: "request", SessionID: "parent", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	inbox := domain.AgentInbox{ID: "relay", ClientID: "relay", TargetID: "parent", RequestID: "request", Content: "steering", Source: "human_input", Delivery: "steer"}
	if err := repo.AcceptAgentMessage(ctx, &inbox); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimAgentRelays(ctx, "parent", "request", false)
	if err != nil || len(claimed) != 1 || claimed[0].Status != "claimed" {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	message := llm.ChatMessage{Role: "user", SourceInboxID: inbox.ID, SourceKind: "human_input", Content: inbox.Content}
	// A failed context CAS must not acknowledge the accepted inbox message.
	if _, _, err := repo.AppendContextEntries(ctx, "parent", "request", "", 0, 99, []llm.ChatMessage{message}); err == nil {
		t.Fatal("stale context accepted")
	}
	pending, _ := repo.ListAgentInbox(ctx, "parent", "claimed")
	if len(pending) != 1 {
		t.Fatal("failed context append consumed the message")
	}
	if _, _, err := repo.AppendContextEntries(ctx, "parent", "request", "", 0, 0, []llm.ChatMessage{message}); err != nil {
		t.Fatal(err)
	}
	completed, _ := repo.ListAgentInbox(ctx, "parent", "completed")
	if len(completed) != 1 {
		t.Fatal("durable context did not acknowledge relay")
	}
	var entries []domain.ContextEntry
	if err := db.Where("scope_id = ?", "parent").Find(&entries).Error; err != nil || len(entries) != 1 {
		t.Fatalf("model input absent: %d %v", len(entries), err)
	}
	if err := db.Create(&domain.ContextHead{ScopeID: "other"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.AppendContextEntries(ctx, "other", "request", "", 0, 0, []llm.ChatMessage{message}); err == nil {
		t.Fatal("cross-scope relay acknowledgement accepted")
	}
}
