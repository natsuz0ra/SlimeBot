package tools

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagedCompletionDeliveredOnceAcrossDetachRace(t *testing.T) {
	for _, late := range []bool{false, true} {
		m := NewProcessManager()
		release := make(chan struct{})
		events := make(chan ProcessCompletion, 2)
		id := m.Start("synthetic", func(context.Context) processRunResult {
			<-release
			return processRunResult{Stdout: "result", ExitCode: 7}
		})
		callback := func(event ProcessCompletion) {
			if _, ok := m.Get(id); !ok {
				t.Error("callback cannot inspect process")
			}
			events <- event
		}
		if late {
			close(release)
			_, finished, err := m.Wait(context.Background(), id, time.Second)
			if err != nil || !finished {
				t.Fatal(err)
			}
		}
		m.Detach(id, callback)
		if !late {
			close(release)
		}
		select {
		case event := <-events:
			if event.ExitCode != 7 || !strings.Contains(event.Output, "result") {
				t.Fatal(event)
			}
		case <-time.After(time.Second):
			t.Fatal("missing completion")
		}
		m.Detach(id, callback)
		select {
		case <-events:
			t.Fatal("completion delivered twice")
		default:
		}
	}
}

func TestManagedCapacityPreservesRunningTaskAndStop(t *testing.T) {
	m := NewProcessManager()
	m.maxItems = 1
	release := make(chan struct{})
	id := m.Start("running", func(ctx context.Context) processRunResult {
		<-ctx.Done()
		<-release
		return processRunResult{Error: "cancelled"}
	})
	if other := m.Start("overflow", func(context.Context) processRunResult { t.Error("overflow runner executed"); return processRunResult{} }); other != "" {
		t.Fatal("running task was evicted")
	}
	events := make(chan ProcessCompletion, 1)
	m.Detach(id, func(event ProcessCompletion) { events <- event })
	m.Stop(id)
	trackedWhileStopping := m.HasRunning() && len(m.RunningSpecs()) == 1
	close(release)
	if !trackedWhileStopping {
		t.Error("a cancelled runner must remain active until it exits")
	}
	select {
	case event := <-events:
		if event.Status != "stopped" {
			t.Fatal(event)
		}
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel runner")
	}
	if m.HasRunning() {
		t.Fatal("stopped runner remains active")
	}
}

func TestExecYieldsOnceAndFastCommandDoesNotWake(t *testing.T) {
	for _, slow := range []bool{false, true} {
		m := NewProcessManager()
		completed := make(chan ProcessCompletion, 1)
		var starts atomic.Int32
		ctx := WithBackgroundObserver(WithProcessManager(context.Background(), m), BackgroundObserver{Started: func(ProcessCompletion) { starts.Add(1) }, Completed: func(event ProcessCompletion) { completed <- event }})
		command := "printf 'done'"
		if slow {
			command = "sleep 0.15; printf 'done'"
		}
		if runtime.GOOS == "windows" {
			command = "Write-Output done"
			if slow {
				command = "Start-Sleep -Milliseconds 150; Write-Output done"
			}
		}
		yield := "30000"
		if slow {
			yield = "1"
		}
		result, err := (&execTool{}).run(ctx, map[string]any{"command": command, "description": "synthetic background test", "yield_time_ms": yield, "timeout_ms": "5000"})
		if err != nil || result.Error != "" {
			t.Fatalf("exec: %v %v", result, err)
		}
		var payload execOutputPayload
		if err = json.Unmarshal([]byte(result.Output), &payload); err != nil {
			t.Fatal(err)
		}
		if slow {
			if !payload.Background || payload.ProcessID == "" {
				t.Fatal("long command did not yield")
			}
			select {
			case event := <-completed:
				if !strings.Contains(event.Output, "done") || event.Status != "completed" {
					t.Fatal(event)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("background command did not finish")
			}
			if starts.Load() != 1 || len(m.List()) != 1 {
				t.Fatal("command was started more than once")
			}
		} else {
			if payload.Background || !strings.Contains(payload.Stdout, "done") || starts.Load() != 1 {
				t.Fatal("fast command was detached")
			}
			select {
			case event := <-completed:
				if !strings.Contains(event.Output, "done") || event.Command != command || event.Shell == "" || event.WorkingDirectory == "" {
					t.Fatal(event)
				}
			case <-time.After(time.Second):
				t.Fatal("fast command completion was not persisted")
			}
		}
	}
}
