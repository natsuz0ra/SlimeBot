package chat

import (
	"context"
	"sync"
	"testing"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/tools"
)

func TestRunParallelToolJobsExecutesConcurrentlyAndReturnsOriginalOrder(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	var mu sync.Mutex
	var pushed []string

	jobs := []parallelToolJob{
		{
			index:            0,
			toolCallID:       "call_a",
			toolName:         "test",
			command:          "slow_a",
			requiresApproval: false,
			execute: func(ctx context.Context) *tools.ExecuteResult {
				started <- "call_a"
				<-release
				return &tools.ExecuteResult{Output: "A"}
			},
		},
		{
			index:            1,
			toolCallID:       "call_b",
			toolName:         "test",
			command:          "slow_b",
			requiresApproval: false,
			execute: func(ctx context.Context) *tools.ExecuteResult {
				started <- "call_b"
				<-release
				return &tools.ExecuteResult{Output: "B"}
			},
		},
	}

	done := make(chan []parallelToolOutcome, 1)
	go func() {
		done <- runParallelToolJobs(context.Background(), jobs, 2, func(result ToolCallResult) {
			mu.Lock()
			defer mu.Unlock()
			pushed = append(pushed, result.ToolCallID)
		})
	}()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case id := <-started:
			seen[id] = true
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("expected both jobs to start before either is released, saw %v", seen)
		}
	}
	close(release)

	var outcomes []parallelToolOutcome
	select {
	case outcomes = <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("parallel jobs did not finish")
	}

	if len(outcomes) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(outcomes))
	}
	if outcomes[0].toolCallID != "call_a" || outcomes[1].toolCallID != "call_b" {
		t.Fatalf("outcomes were not returned in original order: %+v", outcomes)
	}
	if outcomes[0].messageContent != "Execution result:\nA" || outcomes[1].messageContent != "Execution result:\nB" {
		t.Fatalf("unexpected message content: %+v", outcomes)
	}
	if len(pushed) != 2 {
		t.Fatalf("expected result callback for both jobs, got %v", pushed)
	}
}

func TestRunParallelToolJobsWaitsForApprovalsIndependently(t *testing.T) {
	approveA := make(chan approvalDecision, 1)
	approveB := make(chan approvalDecision, 1)
	executed := make(chan string, 1)

	jobs := []parallelToolJob{
		{
			index:            0,
			toolCallID:       "call_a",
			toolName:         constants.ExecToolName,
			command:          "run",
			requiresApproval: true,
			awaitApproval: func(ctx context.Context) approvalDecision {
				return <-approveA
			},
			execute: func(ctx context.Context) *tools.ExecuteResult {
				executed <- "call_a"
				return &tools.ExecuteResult{Output: "approved"}
			},
		},
		{
			index:            1,
			toolCallID:       "call_b",
			toolName:         constants.ExecToolName,
			command:          "run",
			requiresApproval: true,
			awaitApproval: func(ctx context.Context) approvalDecision {
				return <-approveB
			},
			execute: func(ctx context.Context) *tools.ExecuteResult {
				t.Fatal("rejected job should not execute")
				return &tools.ExecuteResult{}
			},
		},
	}

	done := make(chan []parallelToolOutcome, 1)
	go func() {
		done <- runParallelToolJobs(context.Background(), jobs, 2, nil)
	}()

	approveA <- approvalDecision{approved: true}
	select {
	case id := <-executed:
		if id != "call_a" {
			t.Fatalf("unexpected executed job: %s", id)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("approved job did not execute while another approval was still pending")
	}

	approveB <- approvalDecision{approved: false, rejectionMessage: "rejected"}
	outcomes := <-done
	if len(outcomes) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(outcomes))
	}
	if outcomes[0].status != constants.ToolCallStatusCompleted {
		t.Fatalf("approved outcome status = %s", outcomes[0].status)
	}
	if outcomes[1].status != constants.ToolCallStatusRejected || outcomes[1].messageContent != "rejected" {
		t.Fatalf("unexpected rejected outcome: %+v", outcomes[1])
	}
}
