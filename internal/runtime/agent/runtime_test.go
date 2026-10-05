package agent_test

import (
	"context"
	"errors"
	"slimebot/internal/domain"
	"slimebot/internal/repositories"
	agent "slimebot/internal/runtime/agent"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T, run agent.Runner) (*repositories.Repository, *agent.Runtime) {
	t.Helper()
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "agent_runtime"))
	r := agent.New(repo, run)
	t.Cleanup(func() {
		ctx, c := context.WithTimeout(context.Background(), time.Second)
		defer c()
		_ = r.Close(ctx)
		_ = repo.Close()
	})
	return repo, r
}
func awaitIdle(t *testing.T, r *agent.Runtime, id string) {
	t.Helper()
	ctx, c := context.WithTimeout(context.Background(), 2*time.Second)
	defer c()
	for r.IsLive(id) {
		if e := r.Wait(ctx, "root", nil, time.Millisecond*10); e != nil {
			t.Fatal(e)
		}
	}
}
func TestPersistentTurnsIdempotencyAndRestart(t *testing.T) {
	var calls atomic.Int32
	repo, r := fixture(t, func(ctx context.Context, d domain.AgentDescriptor, t domain.AgentTurn, m domain.AgentInbox) (agent.Result, error) {
		calls.Add(1)
		return agent.Result{Answer: m.Content, Reason: "completed"}, nil
	})
	ctx := context.Background()
	_, err := r.BeginRoot(ctx, "root", "request")
	if err != nil {
		t.Fatal(err)
	}
	d, err := r.Start(ctx, domain.AgentDescriptor{RootID: "root", ParentID: "root", RequestID: "request", Depth: 1, Mode: "continuable"}, domain.AgentInbox{RequestID: "request", Content: "first", Source: "delegated_task"})
	if err != nil {
		t.Fatal(err)
	}
	awaitIdle(t, r, d.SessionID)
	input := domain.AgentInbox{ClientID: "retry-safe", TargetID: d.SessionID, RequestID: "request", Source: "human_input", Content: "second"}
	if err = repo.AcceptAgentMessage(ctx, &input); err != nil {
		t.Fatal(err)
	}
	duplicate := input
	duplicate.ID = ""
	if err = repo.AcceptAgentMessage(ctx, &duplicate); err != nil {
		t.Fatal(err)
	}
	if duplicate.ID != input.ID {
		t.Fatal("acceptance was not idempotent")
	}
	if err = r.Wake(ctx, *d, "request"); err != nil {
		t.Fatal(err)
	}
	awaitIdle(t, r, d.SessionID)
	turns, err := repo.ListAgentTurns(ctx, d.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || calls.Load() != 2 {
		t.Fatalf("turns=%d calls=%d", len(turns), calls.Load())
	}
	if turns[0].Answer != "second" || turns[0].Status != "succeeded" {
		t.Fatal(turns[0])
	}
	if err = r.FinishRoot(ctx, "request", false); err != nil {
		t.Fatal(err)
	}
	_, err = r.BeginRoot(ctx, "root", "follow-up")
	if err != nil {
		t.Fatal(err)
	}
	input = domain.AgentInbox{ClientID: "third", TargetID: d.SessionID, RequestID: "follow-up", Source: "human_input", Content: "third"}
	if err = repo.AcceptAgentMessage(ctx, &input); err != nil {
		t.Fatal(err)
	}
	if err = r.Wake(ctx, *d, "follow-up"); err != nil {
		t.Fatal(err)
	}
	awaitIdle(t, r, d.SessionID)
	turns, _ = repo.ListAgentTurns(ctx, d.SessionID)
	if len(turns) != 3 || turns[0].RequestID != "follow-up" {
		t.Fatal(turns)
	}
}
func TestCancellationAndPanicBecomeTerminalFacts(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "panic"}[panics], func(t *testing.T) {
			started := make(chan struct{})
			repo, r := fixture(t, func(ctx context.Context, d domain.AgentDescriptor, t domain.AgentTurn, m domain.AgentInbox) (agent.Result, error) {
				close(started)
				if panics {
					panic("executor bug")
				}
				<-ctx.Done()
				return agent.Result{Answer: "partial"}, ctx.Err()
			})
			ctx := context.Background()
			_, _ = r.BeginRoot(ctx, "root", "request")
			d, err := r.Start(ctx, domain.AgentDescriptor{RootID: "root", ParentID: "root", RequestID: "request", Depth: 1}, domain.AgentInbox{RequestID: "request"})
			if err != nil {
				t.Fatal(err)
			}
			<-started
			if !panics {
				if err = r.Interrupt(ctx, d.SessionID, true); err != nil {
					t.Fatal(err)
				}
			}
			awaitIdle(t, r, d.SessionID)
			ts, _ := repo.ListAgentTurns(ctx, d.SessionID)
			if len(ts) != 1 || ts[0].FinishedAt == nil || ts[0].Status == "running" {
				t.Fatal(ts)
			}
		})
	}
}
func TestRootBudgetAndCAS(t *testing.T) {
	repo, r := fixture(t, nil)
	ctx := context.Background()
	_, _ = r.BeginRoot(ctx, "root", "request")
	if e := repo.ReserveAgentBudget(ctx, "request", 1000001); !errors.Is(e, domain.ErrAgentBudget) {
		t.Fatal(e)
	}
	if e := repo.ReserveAgentBudget(ctx, "request", 100); e != nil {
		t.Fatal(e)
	}
	if e := repo.SettleAgentBudget(ctx, "request", 100, 75); e != nil {
		t.Fatal(e)
	}
	root, _ := repo.GetAgentRoot(ctx, "request")
	if root.Consumed != 75 || root.Reserved != 0 {
		t.Fatal(root)
	}
	if e := repo.SettleAgentBudget(ctx, "request", 100, 75); !errors.Is(e, domain.ErrAgentConflict) {
		t.Fatal(e)
	}
}

func TestExpiredDrainDeadlineEventuallyReleasesRoot(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	repo, runtime := fixture(t, func(ctx context.Context, d domain.AgentDescriptor, turn domain.AgentTurn, input domain.AgentInbox) (agent.Result, error) {
		close(started)
		<-ctx.Done()
		<-finish
		return agent.Result{Answer: "partial"}, ctx.Err()
	})
	ctx := context.Background()
	if _, err := runtime.BeginRoot(ctx, "root", "request"); err != nil {
		t.Fatal(err)
	}
	child, err := runtime.Start(ctx, domain.AgentDescriptor{RootID: "root", ParentID: "root", RequestID: "request", Depth: 1, Mode: "continuable"}, domain.AgentInbox{RequestID: "request", Content: "work"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	drain, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if err := runtime.FinishRoot(drain, "request", true); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if _, _, live := runtime.CurrentRoot("root"); !live {
		t.Fatal("capacity released before work drained")
	}
	close(finish)
	awaitIdle(t, runtime, child.SessionID)
	deadline, cancelWait := context.WithTimeout(ctx, time.Second)
	defer cancelWait()
	for {
		if _, _, live := runtime.CurrentRoot("root"); !live {
			break
		}
		if err := runtime.Wait(deadline, "root", nil, time.Millisecond*10); err != nil {
			t.Fatal(err)
		}
	}
	root, err := repo.GetAgentRoot(ctx, "request")
	if err != nil || root.Status != "canceled" {
		t.Fatalf("root terminal fact missing: %+v %v", root, err)
	}
	if _, err := runtime.BeginRoot(ctx, "root", "next"); err != nil {
		t.Fatal("root blocked after drain:", err)
	}
}

func TestWaitSignalRetainsUpdateBetweenQueryAndWait(t *testing.T) {
	_, runtime := fixture(t, nil)
	ctx := context.Background()
	changed := runtime.Changes()
	runtime.Emit(ctx, "root", "", "", "state_changed", nil)
	deadline, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if err := runtime.WaitSignal(deadline, changed, time.Second); err != nil {
		t.Fatalf("lost update before wait: %v", err)
	}
}
