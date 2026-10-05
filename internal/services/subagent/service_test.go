package subagent

import (
	"context"
	"errors"
	"slimebot/internal/domain"
	"slimebot/internal/repositories"
	agent "slimebot/internal/runtime/agent"
	"testing"
)

func fixture(t *testing.T) (*Service, Caller) {
	t.Helper()
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "tasks"))
	t.Cleanup(func() { _ = repo.Close() })
	r := agent.New(repo, nil)
	return New(repo, r), Caller{AgentID: "root", RootID: "root", MaxDepth: 2}
}
func TestTaskDependenciesCASAndReopenInvalidation(t *testing.T) {
	s, c := fixture(t)
	ctx := context.Background()
	a, e := s.Task(ctx, c, TaskMutation{Action: "create", Title: "first"})
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.Task(ctx, c, TaskMutation{Action: "create", Title: "second", Dependencies: []string{a.ID}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Task(ctx, c, TaskMutation{Action: "claim", ID: b.ID, ExpectedRevision: b.Revision}); e == nil {
		t.Fatal("blocked task claimed")
	}
	if _, e = s.Task(ctx, c, TaskMutation{Action: "update", ID: a.ID, ExpectedRevision: a.Revision, Dependencies: []string{b.ID}}); e == nil {
		t.Fatal("cycle accepted")
	}
	claimed, e := s.Task(ctx, c, TaskMutation{Action: "claim", ID: a.ID, ExpectedRevision: a.Revision})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Task(ctx, c, TaskMutation{Action: "claim", ID: a.ID, ExpectedRevision: a.Revision}); !errors.Is(e, domain.ErrAgentConflict) {
		t.Fatal(e)
	}
	completed, e := s.Task(ctx, c, TaskMutation{Action: "complete", ID: a.ID, ExpectedRevision: claimed.Revision})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Task(ctx, c, TaskMutation{Action: "claim", ID: b.ID, ExpectedRevision: b.Revision}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Task(ctx, c, TaskMutation{Action: "reopen", ID: a.ID, ExpectedRevision: completed.Revision}); e != nil {
		t.Fatal(e)
	}
	ts, _ := s.Store.ListAgentTasks(ctx, c.RootID)
	for _, t2 := range ts {
		if t2.ID == b.ID && t2.Status != "needs_attention" {
			t.Fatal("dependent claim survived reopened prerequisite")
		}
	}
}
func TestDepthAndRoleCannotEscalate(t *testing.T) {
	s, c := fixture(t)
	ctx := context.Background()
	c.MaxDepth = 0
	if _, e := s.Spawn(ctx, c, Spawn{Task: "x"}); e == nil {
		t.Fatal("disabled delegation accepted")
	}
	c.MaxDepth = 4
	c.Depth = 1
	c.Profile = "reviewer"
	if _, e := s.Spawn(ctx, c, Spawn{Task: "x", Profile: "worker"}); e == nil {
		t.Fatal("reviewer escalated to worker")
	}
}

func TestFailedSpawnDoesNotLeaveUnclaimedTask(t *testing.T) {
	s, caller := fixture(t)
	_, err := s.Spawn(context.Background(), caller, Spawn{Title: "not admitted", Task: "research", ModelID: "model"})
	if err == nil {
		t.Fatal("inactive root accepted spawn")
	}
	tasks, err := s.Store.ListAgentTasks(context.Background(), caller.RootID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("failed admission left tasks: %+v", tasks)
	}
}

func TestNestedArtifactChangeInvalidatesParentAcceptance(t *testing.T) {
	s, c := fixture(t)
	ctx := context.Background()
	parent := domain.AgentTask{ID: "parent", RootID: c.RootID, Status: "completed", OwnerID: c.RootID, ArtifactID: "artifact", Dependencies: "[]", Revision: 1}
	dependent := domain.AgentTask{ID: "dependent", RootID: c.RootID, Status: "completed", OwnerID: c.RootID, Dependencies: `["parent"]`, Revision: 1}
	for _, task := range []*domain.AgentTask{&parent, &dependent} {
		if err := s.Store.PutAgentTask(ctx, task, 0); err != nil {
			t.Fatal(err)
		}
	}
	a := domain.AgentArtifact{ID: "artifact", RootID: c.RootID, AgentID: "worker", TaskID: "parent", ResultCommit: "new", Status: "ready"}
	if err := s.RefreshArtifact(ctx, &a, true); err != nil {
		t.Fatal(err)
	}
	tasks, err := s.Store.ListAgentTasks(ctx, c.RootID)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.ID == "parent" && (task.Status != "awaiting_integration" || task.ArtifactID != a.ID) {
			t.Fatalf("parent acceptance survived changed artifact: %+v", task)
		}
		if task.ID == "dependent" && task.Status != "needs_attention" {
			t.Fatalf("dependent remained completed: %+v", task)
		}
	}
}
