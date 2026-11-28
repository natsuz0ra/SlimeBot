package chat

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	agentruntime "slimebot/internal/runtime/agent"
	llmsvc "slimebot/internal/services/llm"
	"slimebot/internal/services/subagent"
	teamsvc "slimebot/internal/services/team"
)

type subagentContractProvider struct {
	run func(context.Context) (agentruntime.Result, error)
}

func (p subagentContractProvider) StreamChatWithTools(ctx context.Context, _ llmsvc.ModelRuntimeConfig, _ []llmsvc.ChatMessage, _ []llmsvc.ToolDef, callbacks llmsvc.StreamCallbacks) (*llmsvc.StreamResult, error) {
	result, err := p.run(ctx)
	if err != nil {
		return nil, err
	}
	if err := callbacks.OnThinkingChunk(result.Thinking); err != nil {
		return nil, err
	}
	if err := callbacks.OnChunk(result.Answer); err != nil {
		return nil, err
	}
	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
}

func TestSubagentExecutorContract(t *testing.T) {
	for _, engine := range []string{"direct", "cooperative"} {
		t.Run(engine, func(t *testing.T) {
			testSubagentExecutorContract(t, engine == "cooperative")
		})
	}
}

func testSubagentExecutorContract(t *testing.T, cooperative bool) {
	t.Helper()
	for _, tc := range []struct {
		name        string
		atLimit     bool
		missingTask bool
		runErr      error
		cancel      bool
		wantStatus  string
	}{
		{name: "success", wantStatus: domain.TeamMemberRunStatusSucceeded},
		{name: "depth", atLimit: true},
		{name: "missing task", missingTask: true},
		{name: "failure", runErr: errors.New("provider failed"), wantStatus: domain.TeamMemberRunStatusFailed},
		{name: "cancellation", cancel: true, wantStatus: domain.TeamMemberRunStatusCanceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newTestRepo(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			session, err := repo.CreateSession(ctx, "executor contract")
			if err != nil {
				t.Fatal(err)
			}
			const requestID = "executor-contract"
			model := llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI, ConfigID: "contract-model"}
			runStarted := make(chan struct{})
			startEvent := make(chan struct{})
			run := func(childCtx context.Context) (agentruntime.Result, error) {
				close(runStarted)
				if tc.cancel {
					<-childCtx.Done()
					return agentruntime.Result{}, childCtx.Err()
				}
				if tc.runErr != nil {
					return agentruntime.Result{}, tc.runErr
				}
				return agentruntime.Result{Answer: "answer", Thinking: "thinking"}, nil
			}
			agent := NewAgentService(llmsvc.NewFactory(subagentContractProvider{run: run}), nil, nil)
			agent.SetSubagentHost(&stubSubagentHost{})
			opts := AgentLoopOptions{teamRuntime: newTeamRuntime(teamsvc.NewService(repo, teamsvc.Options{}), session.ID, requestID)}
			if tc.atLimit {
				opts.Depth = constants.MaxSubagentDepth
			}
			if cooperative {
				runtime := agentruntime.New(repo, func(ctx context.Context, _ domain.AgentDescriptor, _ domain.AgentTurn, _ domain.AgentInbox) (agentruntime.Result, error) {
					return run(ctx)
				})
				t.Cleanup(func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := runtime.Close(ctx); err != nil {
						t.Error(err)
					}
				})
				if _, err := runtime.BeginRoot(context.Background(), session.ID, requestID); err != nil {
					t.Fatal(err)
				}
				caller := subagentCaller(session.ID, requestID, constants.ApprovalModeStandard, false)
				if tc.atLimit {
					caller.Depth = caller.MaxDepth
					opts.Depth = caller.Depth
				}
				opts.Cooperative = &cooperativeLoop{service: subagent.New(repo, runtime), caller: caller, model: model}
			}
			if tc.cancel {
				go func() {
					for _, ready := range []chan struct{}{startEvent, runStarted} {
						select {
						case <-ready:
						case <-ctx.Done():
							return
						}
					}
					cancel()
				}()
			}

			var events []string
			var meta AgentEventMeta
			var doneErr error
			const callID = "contract-call"
			callbacks := AgentCallbacks{
				OnTeamStart: func(run domain.TeamRun) error {
					events = append(events, "team")
					return nil
				},
				OnTeamMemberQueued: func(member domain.TeamMemberRun) error {
					events = append(events, "queued")
					if member.Status != domain.TeamMemberRunStatusQueued || member.ToolCallID != callID {
						return fmt.Errorf("unexpected queued member: %+v", member)
					}
					return nil
				},
				OnSubagentStart: func(event AgentEventMeta, title, task string) error {
					events = append(events, "start")
					meta = event
					member, err := repo.GetTeamMemberRunByID(ctx, meta.MemberRunID)
					if err != nil {
						return err
					}
					if meta.ParentToolCallID != callID || meta.SubagentRunID == "" || title != "contract" || task != "task" ||
						member.Status != domain.TeamMemberRunStatusRunning || member.SubagentRunID != meta.SubagentRunID || member.TeamRunID != meta.TeamRunID {
						return fmt.Errorf("unexpected start event: %+v, member: %+v", meta, member)
					}
					close(startEvent)
					return nil
				},
				OnThinkingStart: func(ThinkingEventMeta) error {
					events = append(events, "thinking_start")
					return nil
				},
				OnThinkingChunk: func(text string, meta ThinkingEventMeta) error {
					events = append(events, text)
					return nil
				},
				OnThinkingDone: func(ThinkingEventMeta) error {
					events = append(events, "thinking_done")
					return nil
				},
				OnSubagentChunk: func(event AgentEventMeta, chunk string) error {
					events = append(events, chunk)
					if event != meta {
						t.Errorf("chunk metadata = %+v, want %+v", event, meta)
					}
					return nil
				},
				OnSubagentDone: func(event AgentEventMeta, err error) error {
					events = append(events, "done")
					doneErr = err
					if event != meta {
						t.Errorf("done metadata = %+v, want %+v", event, meta)
					}
					return nil
				},
			}
			task := "task"
			if tc.missingTask {
				task = " "
			}
			request := subagentExecutionRequest{
				parentModel: model, sessionID: session.ID, callbacks: callbacks, opts: opts,
				toolCall:   llmsvc.ToolCallInfo{ID: callID, Name: constants.RunSubagentTool},
				invocation: resolvedToolInvocation{toolName: constants.RunSubagentTool, command: "run"},
				params:     map[string]any{"title": "contract", "task": task},
			}
			request.reserveParallelMember(ctx, agent.subagentHost)
			result, err := agent.subagentExecutor(opts.Cooperative).ExecuteSubagent(ctx, request)
			if err != nil || result == nil {
				t.Fatalf("execute = %+v, %v", result, err)
			}
			if tc.wantStatus == "" {
				want := "task"
				if tc.atLimit {
					want = "nested"
					if cooperative {
						want = "depth limit"
					}
				}
				if !strings.Contains(result.Output+result.Error, want) || len(events) != 0 || opts.teamRuntime.run != nil {
					t.Fatalf("unexpected admission outcome %+v, events %v", result, events)
				}
				select {
				case <-runStarted:
					t.Fatal("inadmissible task reached the execution engine")
				default:
				}
				return
			}
			wantEvents := []string{"team", "queued", "start"}
			switch {
			case tc.runErr != nil:
				if doneErr == nil || !strings.Contains(doneErr.Error(), tc.runErr.Error()) || result.Error != doneErr.Error() {
					t.Fatalf("execution error = %+v, done error %v", result, doneErr)
				}
			case tc.cancel:
				if !errors.Is(doneErr, context.Canceled) || result.Error != doneErr.Error() {
					t.Fatalf("cancellation result = %+v, done error %v", result, doneErr)
				}
			default:
				wantEvents = append(wantEvents, "thinking_start", "thinking", "thinking_done", "answer")
				if doneErr != nil || result.Error != "" || result.Output != "answer" {
					t.Fatalf("successful result = %+v, done error %v", result, doneErr)
				}
			}
			wantEvents = append(wantEvents, "done")
			if !slices.Equal(events, wantEvents) {
				t.Fatalf("events = %v, want %v", events, wantEvents)
			}
			member, err := repo.GetTeamMemberRunByID(context.WithoutCancel(ctx), meta.MemberRunID)
			if err != nil {
				t.Fatal(err)
			}
			if member.Status != tc.wantStatus || member.FinishedAt == nil || member.Answer != result.Output {
				t.Fatalf("member = %+v, want %s with answer %q", member, tc.wantStatus, result.Output)
			}
		})
	}
}
