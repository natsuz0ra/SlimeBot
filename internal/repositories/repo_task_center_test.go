package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"slimebot/internal/domain"
)

func TestTaskCenterDiscoversAgentsAcrossRootsWithLatestTurn(t *testing.T) {
	db := NewSQLiteDBTest(t, "task_center_agents")
	repo := New(db)
	for _, root := range []string{"root-a", "root-b"} {
		if _, err := repo.CreateSessionWithID(context.Background(), root, root); err != nil {
			t.Fatal(err)
		}
		id := "agent-" + root
		if err := repo.CreateAgent(context.Background(), &domain.AgentDescriptor{SessionID: id, RootID: root, ParentID: root, RequestID: uuid.NewString(), Title: id}, &domain.AgentInbox{ClientID: uuid.NewString(), TargetID: id, Content: "work", Delivery: "queue"}); err != nil {
			t.Fatal(err)
		}
		for _, status := range []string{"running", "completed"} {
			turn := &domain.AgentTurn{ID: uuid.NewString(), SessionID: id, RootID: root, RequestID: uuid.NewString(), MessageID: uuid.NewString(), Status: status, Revision: 0, Epoch: uuid.NewString(), CreatedAt: time.Now()}
			if err := db.Create(turn).Error; err != nil {
				t.Fatal(err)
			}
			time.Sleep(time.Millisecond)
		}
	}
	items, err := repo.ListTaskCenterAgents(context.Background(), 10)
	if err != nil || len(items) != 2 {
		t.Fatalf("agents=%d err=%v", len(items), err)
	}
	for _, item := range items {
		if item.LatestTurn == nil || item.LatestTurn.Status != "completed" {
			t.Fatalf("latest turn missing: %+v", item)
		}
	}
}
