// Package agent owns process-local agent activations. Persistence owns accepted
// input and terminal facts; channels only wake workers and never hold messages.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"slimebot/internal/domain"
	"sync"
	"time"
)

type Result struct {
	Answer, Thinking, Reason, Error string
	Input, Output                   int
}
type Runner func(context.Context, domain.AgentDescriptor, domain.AgentTurn, domain.AgentInbox) (Result, error)
type activation struct {
	cancel  context.CancelFunc
	done    chan struct{}
	paused  bool
	turn    string
	request string
}
type root struct {
	ctx     context.Context
	cancel  context.CancelFunc
	session string
	signal  chan struct{}
}
type Runtime struct {
	Store   domain.AgentStore
	runner  Runner
	mu      sync.Mutex
	live    map[string]*activation
	roots   map[string]*root
	changed chan struct{}
	models  chan struct{}
	perRoot map[string]chan struct{}
	closed  bool
}

func New(store domain.AgentStore, runner Runner) *Runtime {
	return &Runtime{Store: store, runner: runner, live: map[string]*activation{}, roots: map[string]*root{}, changed: make(chan struct{}), models: make(chan struct{}, 8), perRoot: map[string]chan struct{}{}}
}
func (r *Runtime) BeginRoot(ctx context.Context, session, request string) (context.Context, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, errors.New("agent runtime closed")
	}
	if len(r.roots)+len(r.live) >= 32 {
		return nil, domain.ErrAgentCapacity
	}
	for _, x := range r.roots {
		if x.session == session {
			return nil, errors.New("session already has an active request")
		}
	}
	deadline := time.Now().Add(30 * time.Minute)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	rootCtx, cancel := context.WithDeadline(ctx, deadline)
	if err := r.Store.PutAgentRoot(ctx, &domain.AgentRootRequest{ID: request, SessionID: session, Status: "running", Budget: 1000000, Deadline: deadline}); err != nil {
		cancel()
		return nil, err
	}
	r.roots[request] = &root{ctx: rootCtx, cancel: cancel, session: session, signal: make(chan struct{}, 1)}
	r.perRoot[request] = make(chan struct{}, 4)
	return rootCtx, nil
}
func (r *Runtime) RootContext(request string) (context.Context, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	x := r.roots[request]
	if x == nil {
		return nil, false
	}
	return x.ctx, true
}
func (r *Runtime) FinishRoot(ctx context.Context, request string, canceled bool) error {
	status := "completed"
	if canceled {
		status = "canceled"
	}
	return r.FinishRootStatus(ctx, request, status)
}
func (r *Runtime) FinishRootStatus(ctx context.Context, request, status string) error {
	r.mu.Lock()
	x := r.roots[request]
	r.mu.Unlock()
	if x == nil {
		return nil
	}
	if status == "completed" && x.ctx.Err() != nil {
		status = "canceled"
	}
	err := r.Store.UpdateAgentRoot(ctx, request, status)
	x.cancel()
	r.mu.Lock()
	var waiting []chan struct{}
	for _, a := range r.live {
		if a.request == request {
			a.cancel()
			waiting = append(waiting, a.done)
		}
	}
	r.mu.Unlock()
	for _, done := range waiting {
		select {
		case <-done:
		case <-ctx.Done():
			// Retain capacity while canceled work drains, then release it even if
			// the caller's shutdown/HTTP deadline has already elapsed.
			go func() {
				for _, done := range waiting {
					<-done
				}
				finish, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = r.Store.UpdateAgentRoot(finish, request, status)
				r.releaseRoot(request, x)
				r.Emit(finish, x.session, x.session, "", "root_finished", map[string]string{"requestId": request, "status": status})
			}()
			return ctx.Err()
		}
	}
	r.releaseRoot(request, x)
	if err == nil {
		r.Emit(ctx, x.session, x.session, "", "root_finished", map[string]string{"requestId": request, "status": status})
	}
	return err
}
func (r *Runtime) releaseRoot(request string, owner *root) {
	r.mu.Lock()
	if r.roots[request] == owner {
		delete(r.roots, request)
		delete(r.perRoot, request)
		r.notifyLocked()
	}
	r.mu.Unlock()
	go r.wakePending()
}
func (r *Runtime) notifyLocked() { close(r.changed); r.changed = make(chan struct{}) }
func (r *Runtime) Emit(ctx context.Context, rootID, agentID, turnID, kind string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil || len(data) > 16384 {
		data, _ = json.Marshal(map[string]any{"truncated": true, "bytes": len(data)})
	}
	_ = r.Store.AppendAgentEvent(ctx, &domain.AgentEvent{RootID: rootID, AgentID: agentID, TurnID: turnID, Kind: kind, Payload: string(data)})
	r.mu.Lock()
	r.notifyLocked()
	r.mu.Unlock()
}
func (r *Runtime) Start(ctx context.Context, d domain.AgentDescriptor, input domain.AgentInbox) (*domain.AgentDescriptor, error) {
	if d.Depth < 1 || d.Depth > 4 {
		return nil, errors.New("delegation depth limit exceeded")
	}
	rootCtx, ok := r.RootContext(input.RequestID)
	if !ok {
		return nil, errors.New("root request is not active")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.SessionID = uuid.NewString()
	d.Version = 1
	d.CreatedAt = time.Now()
	input.ID = uuid.NewString()
	input.TargetID = d.SessionID
	input.ClientID = input.ID
	r.mu.Lock()
	if r.closed || len(r.live)+len(r.roots) >= 32 {
		r.mu.Unlock()
		return nil, domain.ErrAgentCapacity
	}
	count := 0
	for id := range r.live {
		a, e := r.Store.GetAgent(ctx, id)
		if e == nil && a.RootID == d.RootID {
			count++
		}
	}
	if count >= 16 {
		r.mu.Unlock()
		return nil, domain.ErrAgentCapacity
	}
	aCtx, cancel := context.WithCancel(rootCtx)
	a := &activation{cancel: cancel, done: make(chan struct{}), request: input.RequestID}
	r.live[d.SessionID] = a
	r.mu.Unlock()
	if err := r.Store.CreateAgent(ctx, &d, &input); err != nil {
		cancel()
		r.mu.Lock()
		delete(r.live, d.SessionID)
		r.notifyLocked()
		r.mu.Unlock()
		close(a.done)
		return nil, err
	}
	r.Emit(context.WithoutCancel(ctx), d.RootID, d.SessionID, "", "agent_created", d)
	go r.drive(aCtx, d, a)
	return &d, nil
}
func (r *Runtime) Wake(ctx context.Context, d domain.AgentDescriptor, request string) error {
	rootCtx, ok := r.RootContext(request)
	if !ok {
		return errors.New("root request is not active")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.live[d.SessionID]; a != nil {
		if a.paused {
			return errors.New("agent is still stopping")
		}
		r.notifyLocked()
		return nil
	}
	if r.closed || len(r.live)+len(r.roots) >= 32 {
		return domain.ErrAgentCapacity
	}
	count := 0
	for id := range r.live {
		x, e := r.Store.GetAgent(ctx, id)
		if e == nil && x.RootID == d.RootID {
			count++
		}
	}
	if count >= 16 {
		return domain.ErrAgentCapacity
	}
	aCtx, cancel := context.WithCancel(rootCtx)
	a := &activation{cancel: cancel, done: make(chan struct{}), request: request}
	r.live[d.SessionID] = a
	go r.drive(aCtx, d, a)
	return nil
}
func (r *Runtime) drive(ctx context.Context, d domain.AgentDescriptor, a *activation) {
	defer func() {
		a.cancel()
		r.mu.Lock()
		if r.live[d.SessionID] == a {
			delete(r.live, d.SessionID)
		}
		r.notifyLocked()
		r.mu.Unlock()
		close(a.done)
		go r.wakePending()
	}()
	for ctx.Err() == nil {
		r.mu.Lock()
		input, turn, err := r.Store.ClaimAgentMessage(ctx, d.SessionID, a.request, uuid.NewString())
		if err != nil {
			if r.live[d.SessionID] == a {
				delete(r.live, d.SessionID)
			}
			r.mu.Unlock()
			return
		}
		r.mu.Unlock()
		r.mu.Lock()
		a.turn = turn.ID
		r.notifyLocked()
		r.mu.Unlock()
		r.Emit(ctx, d.RootID, d.SessionID, turn.ID, "turn_started", turn)
		runCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		result, runErr := r.runSafely(runCtx, d, *turn, *input)
		cancel()
		turn.Answer = result.Answer
		turn.Thinking = result.Thinking
		turn.InputTokens = result.Input
		turn.OutputTokens = result.Output
		turn.Status = "succeeded"
		turn.StopReason = "completed"
		turn.Error = result.Error
		if runErr != nil {
			turn.Status = "failed"
			turn.StopReason = "provider_error"
			turn.Error = runErr.Error()
		}
		if errors.Is(runErr, context.Canceled) || ctx.Err() != nil {
			turn.Status = "canceled"
			turn.StopReason = "canceled"
		}
		if errors.Is(runErr, context.DeadlineExceeded) {
			turn.Status = "failed"
			turn.StopReason = "timeout"
		}
		if result.Reason != "" {
			turn.StopReason = result.Reason
			if result.Reason != "completed" {
				turn.Status = "failed"
			}
		}
		now := time.Now()
		turn.FinishedAt = &now
		body := fmt.Sprintf("Agent %s turn %s ended: %s. Report:\n%s", d.SessionID, turn.ID, turn.StopReason, boundText(turn.Answer, 3072))
		notice := &domain.AgentInbox{ID: uuid.NewString(), ClientID: "result:" + turn.ID, TargetID: d.ParentID, SenderID: d.SessionID, Source: "agent_result", RequestID: turn.RequestID, Content: body, Delivery: "steer"}
		if d.Mode == "one-shot" {
			notice = nil
		}
		finishCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		finishErr := r.Store.CompleteAgentTurn(finishCtx, turn, notice)
		if errors.Is(finishErr, domain.ErrAgentInboxFull) {
			r.Emit(finishCtx, d.RootID, d.SessionID, turn.ID, "result_delivery_failed", map[string]string{"turnId": turn.ID})
			finishErr = r.Store.CompleteAgentTurn(finishCtx, turn, nil)
		}
		if err := finishErr; err != nil {
			r.Emit(finishCtx, d.RootID, d.SessionID, turn.ID, "persistence_failed", map[string]string{"error": err.Error()})
			done()
			return
		}
		r.Emit(finishCtx, d.RootID, d.SessionID, turn.ID, "turn_finished", turn)
		done()
		if ctx.Err() != nil {
			return
		}
	}
}
func boundText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "\n[truncated; use read_agent_result]"
}
func (r *Runtime) IsLive(id string) bool { r.mu.Lock(); defer r.mu.Unlock(); return r.live[id] != nil }

// Changes captures the wakeup signal before checking persistent state, so an
// update arriving between that check and the wait cannot be missed.
func (r *Runtime) Changes() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.changed
}
func (r *Runtime) Wait(ctx context.Context, rootID string, ids []string, timeout time.Duration) error {
	return r.WaitSignal(ctx, r.Changes(), timeout)
}
func (r *Runtime) WaitSignal(ctx context.Context, changed <-chan struct{}, timeout time.Duration) error {
	if timeout <= 0 {
		return nil
	}
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	case <-changed:
		return nil
	}
}
func (r *Runtime) Interrupt(ctx context.Context, id string, subtree bool) error {
	var selected []string
	selected = append(selected, id)
	if subtree {
		d, err := r.Store.GetAgent(ctx, id)
		if err != nil {
			return err
		}
		agents, err := r.Store.ListAgents(ctx, d.RootID)
		if err != nil {
			return err
		}
		for changed := true; changed; {
			changed = false
			for _, x := range agents {
				if contains(selected, x.ParentID) && !contains(selected, x.SessionID) {
					selected = append(selected, x.SessionID)
					changed = true
				}
			}
		}
	}
	if store, ok := r.Store.(interface {
		PauseAgentInbox(context.Context, []string) error
	}); ok {
		if err := store.PauseAgentInbox(ctx, selected); err != nil {
			return err
		}
	}
	r.mu.Lock()
	for i := len(selected) - 1; i >= 0; i-- {
		target := selected[i]
		if a := r.live[target]; a != nil {
			a.paused = true
			a.cancel()
		}
	}
	r.notifyLocked()
	r.mu.Unlock()
	return nil
}
func contains(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
func (r *Runtime) ModelPermit(ctx context.Context, request string) (func(), error) {
	r.mu.Lock()
	local := r.perRoot[request]
	r.mu.Unlock()
	if local == nil {
		return nil, errors.New("root request unavailable")
	}
	select {
	case local <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case r.models <- struct{}{}:
	case <-ctx.Done():
		<-local
		return nil, ctx.Err()
	}
	return func() { <-r.models; <-local }, nil
}
func (r *Runtime) Close(ctx context.Context) error {
	r.mu.Lock()
	r.closed = true
	var done []chan struct{}
	for _, x := range r.roots {
		x.cancel()
	}
	for _, a := range r.live {
		a.cancel()
		done = append(done, a.done)
	}
	r.mu.Unlock()
	for _, d := range done {
		select {
		case <-d:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (r *Runtime) runSafely(ctx context.Context, d domain.AgentDescriptor, t domain.AgentTurn, m domain.AgentInbox) (result Result, err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("agent executor panicked; inspect partial records before retry")
		}
	}()
	return r.runner(ctx, d, t, m)
}
func (r *Runtime) CurrentRoot(session string) (string, context.Context, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, x := range r.roots {
		if x.session == session {
			return id, x.ctx, true
		}
	}
	return "", nil, false
}
func (r *Runtime) StopRoot(session string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.roots {
		if x.session == session {
			x.cancel()
			return true
		}
	}
	return false
}
func (r *Runtime) HasWork(ctx context.Context, session, request string) bool {
	agents, err := r.Store.ListAgents(ctx, session)
	if err != nil {
		return true
	}
	for _, d := range agents {
		if r.IsLive(d.SessionID) {
			return true
		}
		ms, err := r.Store.ListAgentInbox(ctx, d.SessionID, "pending")
		if err != nil {
			return true
		}
		for _, m := range ms {
			if m.RequestID == request {
				return true
			}
		}
	}
	return false
}

func (r *Runtime) wakePending() {
	r.mu.Lock()
	requests := map[string]string{}
	for id, x := range r.roots {
		if x.ctx.Err() == nil {
			requests[id] = x.session
		}
	}
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for request, session := range requests {
		agents, err := r.Store.ListAgents(ctx, session)
		if err != nil {
			continue
		}
		for _, d := range agents {
			if r.IsLive(d.SessionID) {
				continue
			}
			ms, err := r.Store.ListAgentInbox(ctx, d.SessionID, "pending")
			if err != nil {
				continue
			}
			for _, m := range ms {
				if m.RequestID == request {
					_ = r.Wake(ctx, d, request)
					break
				}
			}
		}
	}
}
func (r *Runtime) DrainSession(ctx context.Context, session string) error {
	request, _, ok := r.CurrentRoot(session)
	if !ok {
		return nil
	}
	r.StopRoot(session)
	return r.FinishRoot(ctx, request, true)
}
