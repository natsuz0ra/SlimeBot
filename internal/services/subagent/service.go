package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"slimebot/internal/domain"
	agentruntime "slimebot/internal/runtime/agent"
	workspace "slimebot/internal/runtime/workspace"
	"strings"
	"sync"
	"time"
)

type Caller struct {
	AgentID, RootID, RequestID string
	Depth, MaxDepth            int
	Profile                    string
	ThinkingLevel              string
	ApprovalMode               string
	PlanMode                   bool
}
type Spawn struct {
	Title, Task, Context, Profile, ContextMode, ModelID, Workspace, WorkspaceMode, TaskID string
	OneShot                                                                               bool
}
type Snapshot struct {
	Roots     []domain.AgentRootRequest `json:"roots"`
	Approvals []domain.AgentApproval    `json:"approvals"`
	Agents    []domain.AgentDescriptor  `json:"agents"`
	Turns     []domain.AgentTurn        `json:"turns"`
	Tasks     []domain.AgentTask        `json:"tasks"`
	Artifacts []domain.AgentArtifact    `json:"artifacts"`
	Events    []domain.AgentEvent       `json:"events"`
}
type Service struct {
	Store             domain.AgentStore
	Runtime           *agentruntime.Runtime
	taskMu            sync.Mutex
	ValidateSpawn     func(context.Context, Caller, Spawn) error
	IntegrateArtifact func(context.Context, *domain.AgentArtifact) error
	DrainRoot         func(context.Context, string) error
	ValidateArtifact  func(context.Context, *domain.AgentArtifact, string) error
}

func New(store domain.AgentStore, runtime *agentruntime.Runtime) *Service {
	return &Service{Store: store, Runtime: runtime}
}
func (s *Service) Authorize(ctx context.Context, c Caller, target string, control bool) (*domain.AgentDescriptor, error) {
	d, err := s.Store.GetAgent(ctx, target)
	if err != nil {
		return nil, err
	}
	if d.Archived || d.RootID != c.RootID {
		return nil, errors.New("UNAUTHORIZED")
	}
	if c.AgentID == c.RootID || d.ParentID == c.AgentID {
		return d, nil
	}
	caller, err := s.Store.GetAgent(ctx, c.AgentID)
	if err != nil {
		return nil, errors.New("UNAUTHORIZED")
	}
	if !control && caller.ParentID == target {
		return d, nil
	}
	if !control && caller.PeerMessages && d.PeerMessages {
		return d, nil
	}
	if control {
		p := d.ParentID
		for depth := 0; depth < 4 && p != c.RootID; depth++ {
			if p == c.AgentID {
				return d, nil
			}
			a, e := s.Store.GetAgent(ctx, p)
			if e != nil {
				break
			}
			p = a.ParentID
		}
	}
	return nil, errors.New("UNAUTHORIZED")
}
func (s *Service) Spawn(ctx context.Context, c Caller, p Spawn) (*domain.AgentDescriptor, error) {
	if strings.TrimSpace(p.Task) == "" || len([]byte(p.Task+p.Context)) > 16384 {
		return nil, errors.New("task must be nonempty and at most 16 KiB")
	}
	maxDepth := c.MaxDepth
	if maxDepth == 0 && c.Depth == 0 {
		return nil, errors.New("delegation is disabled")
	}
	if c.Depth >= maxDepth {
		return nil, errors.New("delegation depth limit exceeded")
	}
	if p.Profile == "" {
		p.Profile = "researcher"
	}
	if p.Profile != "researcher" && p.Profile != "reviewer" && p.Profile != "worker" {
		return nil, errors.New("unknown agent profile")
	}
	if (c.PlanMode || (c.Depth > 0 && c.Profile != "worker")) && p.Profile == "worker" {
		return nil, errors.New("worker unavailable in plan mode")
	}
	if p.ContextMode == "" {
		p.ContextMode = "isolated"
	}
	if p.ContextMode != "isolated" && p.ContextMode != "fork" {
		return nil, errors.New("unknown context mode")
	}
	if s.ValidateSpawn != nil {
		if err := s.ValidateSpawn(ctx, c, p); err != nil {
			return nil, err
		}
	}
	createdTask := false
	if p.TaskID == "" && !p.OneShot {
		t, err := s.Task(ctx, c, TaskMutation{Action: "create", Title: func() string {
			if p.Title != "" {
				return p.Title
			}
			return p.Task
		}(), Description: p.Task})
		if err != nil {
			return nil, err
		}
		p.TaskID = t.ID
		createdTask = true
	}
	if p.TaskID != "" {
		ts, err := s.Store.ListAgentTasks(ctx, c.RootID)
		if err != nil {
			return nil, err
		}
		found := false
		for _, t := range ts {
			if t.ID == p.TaskID {
				found = true
				if t.Status != "pending" || t.OwnerID != "" {
					return nil, errors.New("task unavailable")
				}
				var deps []string
				_ = json.Unmarshal([]byte(t.Dependencies), &deps)
				for _, id := range deps {
					ready := false
					for _, dep := range ts {
						if dep.ID == id && dep.Status == "completed" {
							ready = true
						}
					}
					if !ready {
						return nil, errors.New("TASK_BLOCKED")
					}
				}
			}
		}
		if !found {
			return nil, errors.New("TASK_NOT_FOUND")
		}
	}
	mode := "continuable"
	if p.OneShot {
		mode = "one-shot"
	}
	d := domain.AgentDescriptor{ThinkingLevel: c.ThinkingLevel, MaxDepth: maxDepth, RootID: c.RootID, ParentID: c.AgentID, RequestID: c.RequestID, Title: p.Title, Task: p.Task, Profile: p.Profile, Mode: mode, ContextMode: p.ContextMode, ModelID: p.ModelID, Workspace: p.Workspace, Depth: c.Depth + 1, PeerMessages: !p.OneShot, ApprovalMode: c.ApprovalMode, PlanMode: c.PlanMode, TaskID: p.TaskID}
	if d.Title == "" {
		d.Title = p.Task
	}
	r := []rune(d.Title)
	if len(r) > 80 {
		d.Title = string(r[:80])
	}
	body := p.Task
	if p.Context != "" {
		body += "\n\nBackground from parent (not new human authorization):\n" + p.Context
	}
	agent, err := s.Runtime.Start(ctx, d, domain.AgentInbox{SenderID: c.AgentID, Source: "delegated_task", RequestID: c.RequestID, Content: body, Delivery: "queue"})
	if err != nil && createdTask {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if rollback, ok := s.Store.(interface {
			RemovePendingAgentTask(context.Context, string, string) error
		}); ok {
			if cleanupErr := rollback.RemovePendingAgentTask(cleanup, p.TaskID, c.RootID); cleanupErr != nil {
				return nil, errors.Join(err, cleanupErr)
			}
		}
	}
	return agent, err
}
func (s *Service) Send(ctx context.Context, c Caller, target, text, clientID, delivery string, human bool) (*domain.AgentInbox, error) {
	d, err := s.Authorize(ctx, c, target, false)
	if err != nil {
		return nil, err
	}
	if d.Mode != "continuable" {
		return nil, errors.New("one-shot agent cannot continue")
	}
	if len([]byte(text)) > 16384 || strings.TrimSpace(text) == "" {
		return nil, errors.New("message must be nonempty and at most 16 KiB")
	}
	if delivery == "" {
		delivery = "steer"
	}
	if delivery != "steer" && delivery != "queue" {
		return nil, errors.New("invalid delivery")
	}
	if !human {
		delivery = "steer"
	}
	if clientID == "" {
		clientID = uuid.NewString()
	}
	source := "agent_message"
	if human {
		source = "human_input"
	}
	m := &domain.AgentInbox{ID: uuid.NewString(), ClientID: clientID, TargetID: target, SenderID: c.AgentID, Source: source, RequestID: c.RequestID, Content: text, Delivery: delivery}
	if err = s.Store.AcceptAgentMessage(ctx, m); err != nil {
		return nil, err
	}
	if err = s.Runtime.Wake(ctx, *d, c.RequestID); err != nil && !errors.Is(err, domain.ErrAgentCapacity) {
		return m, err
	}
	s.Runtime.Emit(ctx, c.RootID, target, "", "inbox_accepted", m)
	return m, nil
}
func (s *Service) Snapshot(ctx context.Context, root string, after int64) (Snapshot, error) {
	a, err := s.Store.ListAgents(ctx, root)
	if err != nil {
		return Snapshot{}, err
	}
	x := Snapshot{Agents: a, Turns: []domain.AgentTurn{}}
	x.Roots, err = s.Store.ListAgentRoots(ctx, root)
	if err != nil {
		return x, err
	}
	for index, d := range a {
		if messages, e := s.Store.ListAgentInbox(ctx, d.SessionID, "pending"); e == nil {
			x.Agents[index].Queued = len(messages)
		}
		t, e := s.Store.ListAgentTurns(ctx, d.SessionID)
		if e != nil {
			return x, e
		}
		if len(t) > 3 {
			t = t[:3]
		}
		for i := range t {
			rs := []rune(t[i].Answer)
			if len(rs) > 4096 {
				t[i].Answer = string(rs[:4096]) + "\n[report truncated; read_agent_result]"
			}
		}
		x.Turns = append(x.Turns, t...)
	}
	x.Tasks, err = s.Store.ListAgentTasks(ctx, root)
	if err != nil {
		return x, err
	}
	x.Artifacts, err = s.Store.ListAgentArtifacts(ctx, root)
	if err != nil {
		return x, err
	}
	x.Events, err = s.Store.ListAgentEvents(ctx, root, after)
	if err == nil {
		x.Approvals, err = s.Store.ListAgentApprovals(ctx, root)
	}
	return x, err
}

type TaskMutation struct {
	ID               string   `json:"taskId"`
	Action           string   `json:"action"`
	ExpectedRevision int64    `json:"expectedRevision"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Acceptance       string   `json:"acceptance"`
	OwnerID          string   `json:"ownerId"`
	Dependencies     []string `json:"dependencies"`
	WriteScopes      []string `json:"writeScopes"`
	Result           string   `json:"result"`
	ArtifactID       string   `json:"artifactId"`
}

func (s *Service) Task(ctx context.Context, c Caller, m TaskMutation) (*domain.AgentTask, error) {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	if err := workspace.ValidateScopes(m.WriteScopes); err != nil {
		return nil, err
	}
	tasks, err := s.Store.ListAgentTasks(ctx, c.RootID)
	if err != nil {
		return nil, err
	}
	var t domain.AgentTask
	found := false
	for _, x := range tasks {
		if x.ID == m.ID {
			t = x
			found = true
			break
		}
	}
	if m.Action == "create" {
		if len(tasks) >= 256 {
			return nil, domain.ErrAgentCapacity
		}
		if strings.TrimSpace(m.Title) == "" {
			return nil, errors.New("title required")
		}
		t = domain.AgentTask{ID: uuid.NewString(), RootID: c.RootID, RequestID: c.RequestID, Title: m.Title, Description: m.Description, Acceptance: m.Acceptance, Status: "pending", CreatedAt: time.Now(), Revision: 1}
		found = true
	} else if !found {
		return nil, errors.New("TASK_NOT_FOUND")
	}
	if m.Action == "get" {
		return &t, nil
	}
	if m.Action != "create" && t.Revision != m.ExpectedRevision {
		return nil, domain.ErrAgentConflict
	}
	lead := c.AgentID == c.RootID
	owner := t.OwnerID == c.AgentID
	if m.Action != "create" && m.Action != "claim" && !lead && !owner {
		return nil, errors.New("UNAUTHORIZED")
	}
	switch m.Action {
	case "create":
		t.Dependencies = jsonText(m.Dependencies)
		t.WriteScopes = jsonText(m.WriteScopes)
	case "claim":
		if t.Status != "pending" || (t.OwnerID != "" && t.OwnerID != c.AgentID) {
			return nil, errors.New("TASK_ALREADY_CLAIMED")
		}
		var deps []string
		_ = json.Unmarshal([]byte(t.Dependencies), &deps)
		for _, id := range deps {
			ready := false
			for _, other := range tasks {
				if other.ID == id && other.Status == "completed" {
					ready = true
				}
			}
			if !ready {
				return nil, errors.New("TASK_BLOCKED")
			}
		}
		t.OwnerID = c.AgentID
		t.Status = "in_progress"
	case "release":
		if t.Status != "in_progress" {
			return nil, errors.New("invalid task transition")
		}
		t.OwnerID = ""
		t.Status = "pending"
	case "reassign":
		if t.Status == "completed" || t.Status == "deleted" {
			return nil, errors.New("reopen this task before reassigning it")
		}
		if !lead {
			return nil, errors.New("UNAUTHORIZED")
		}
		if m.OwnerID != c.RootID {
			if _, e := s.Authorize(ctx, c, m.OwnerID, true); e != nil {
				return nil, e
			}
		}
		t.OwnerID = m.OwnerID
		t.Status = "pending"
	case "update":
		if m.Title != "" {
			t.Title = m.Title
		}
		if m.Description != "" {
			t.Description = m.Description
		}
		if m.Acceptance != "" {
			t.Acceptance = m.Acceptance
		}
		if m.Dependencies != nil {
			t.Dependencies = jsonText(m.Dependencies)
		}
		if m.WriteScopes != nil {
			t.WriteScopes = jsonText(m.WriteScopes)
		}
	case "complete":
		if t.OwnerID != c.AgentID && !lead {
			return nil, errors.New("UNAUTHORIZED")
		}
		if t.Status != "in_progress" {
			return nil, errors.New("invalid task transition")
		}
		t.Result = m.Result
		t.ArtifactID = m.ArtifactID
		t.Status = "completed"
		if m.ArtifactID != "" {
			as, e := s.Store.ListAgentArtifacts(ctx, c.RootID)
			if e != nil {
				return nil, e
			}
			found := false
			for _, a := range as {
				if a.ID == m.ArtifactID && a.TaskID == t.ID && a.AgentID == t.OwnerID {
					found = true
				}
			}
			if !found {
				return nil, errors.New("invalid task artifact")
			}
			t.Status = "awaiting_integration"
		}
	case "reopen":
		t.Status = "pending"
		t.OwnerID = ""
		t.ArtifactID = ""
		t.Result = ""
	case "cancel":
		t.Status = "canceled"
	case "delete":
		t.Status = "deleted"
	default:
		return nil, errors.New("invalid task action")
	}
	if err = validateGraph(tasks, t); err != nil {
		return nil, err
	}
	old := int64(0)
	if m.Action != "create" {
		old = t.Revision
		t.Revision++
	}
	t.UpdatedAt = time.Now()
	if err = s.Store.PutAgentTask(ctx, &t, old); err != nil {
		return nil, err
	}
	if m.Action == "reassign" || m.Action == "cancel" || m.Action == "reopen" {
		for _, oldTask := range tasks {
			if oldTask.ID == t.ID && oldTask.OwnerID != "" && oldTask.OwnerID != c.RootID && oldTask.OwnerID != c.AgentID {
				_ = s.Runtime.Interrupt(ctx, oldTask.OwnerID, true)
			}
		}
		s.invalidateDependents(ctx, c, t.ID, tasks)
	}
	if m.Action == "reassign" && t.OwnerID != "" && t.OwnerID != c.RootID {
		d, err := s.Authorize(ctx, c, t.OwnerID, true)
		if err != nil {
			return &t, err
		}
		message := domain.AgentInbox{ID: uuid.NewString(), ClientID: fmt.Sprintf("task:%s:%d", t.ID, t.Revision), TargetID: t.OwnerID, SenderID: c.AgentID, Source: "delegated_task", RequestID: c.RequestID, TaskID: t.ID, Content: fmt.Sprintf("Task %s: %s\n%s\nAcceptance: %s", t.ID, t.Title, t.Description, t.Acceptance), Delivery: "queue"}
		if err = s.Store.AcceptAgentMessage(ctx, &message); err != nil {
			return &t, err
		}
		if err = s.Runtime.Wake(ctx, *d, c.RequestID); err != nil && !errors.Is(err, domain.ErrAgentCapacity) && err.Error() != "agent is still stopping" {
			return &t, err
		}
	}
	s.Runtime.Emit(ctx, c.RootID, c.AgentID, "", "task_updated", t)
	return &t, nil
}
func jsonText(x []string) string {
	if x == nil {
		x = []string{}
	}
	b, _ := json.Marshal(x)
	return string(b)
}
func validateGraph(tasks []domain.AgentTask, candidate domain.AgentTask) error {
	graph := map[string]domain.AgentTask{}
	for _, t := range tasks {
		graph[t.ID] = t
	}
	graph[candidate.ID] = candidate
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return errors.New("DEPENDENCY_CYCLE")
		}
		if state[id] == 2 {
			return nil
		}
		t, ok := graph[id]
		if !ok || t.Status == "deleted" {
			return errors.New("TASK_DEPENDENCY_MISSING")
		}
		state[id] = 1
		var deps []string
		if err := json.Unmarshal([]byte(t.Dependencies), &deps); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, d := range deps {
			if seen[d] {
				return errors.New("duplicate dependency")
			}
			seen[d] = true
			if err := visit(d); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id, t := range graph {
		if t.Status != "deleted" {
			if err := visit(id); err != nil {
				return fmt.Errorf("task %s: %w", id, err)
			}
		}
	}
	return nil
}

func (s *Service) invalidateDependents(ctx context.Context, c Caller, id string, tasks []domain.AgentTask) {
	affected := map[string]bool{id: true}
	for again := true; again; {
		again = false
		for _, t := range tasks {
			if affected[t.ID] {
				continue
			}
			var deps []string
			_ = json.Unmarshal([]byte(t.Dependencies), &deps)
			for _, dep := range deps {
				if affected[dep] {
					affected[t.ID] = true
					again = true
					break
				}
			}
		}
	}
	for _, t := range tasks {
		if t.ID == id || !affected[t.ID] || t.Status == "deleted" || t.Status == "canceled" {
			continue
		}
		if t.OwnerID != "" && t.OwnerID != c.RootID {
			_ = s.Runtime.Interrupt(ctx, t.OwnerID, true)
		}
		rev := t.Revision
		t.Revision++
		t.Status = "needs_attention"
		t.UpdatedAt = time.Now()
		_ = s.Store.PutAgentTask(ctx, &t, rev)
	}
}
func (s *Service) Integrate(ctx context.Context, a *domain.AgentArtifact) error {
	if a.Status != "integrated" && (a.ResultCommit == "" || a.ValidatedCommit != a.ResultCommit || a.ValidationCommand == "") {
		return errors.New("artifact must pass validation before integration")
	}
	if s.Runtime.IsLive(a.AgentID) {
		return errors.New("agent is still using this workspace")
	}
	if s.IntegrateArtifact == nil {
		return errors.New("integration unavailable")
	}
	if err := s.IntegrateArtifact(ctx, a); err != nil {
		_ = s.Store.PutAgentArtifact(ctx, a)
		return err
	}
	if err := s.Store.PutAgentArtifact(ctx, a); err != nil {
		return err
	}
	ts, err := s.Store.ListAgentTasks(ctx, a.RootID)
	if err != nil {
		return err
	}
	for _, t := range ts {
		if t.ID == a.TaskID && t.ArtifactID == a.ID && (t.Status == "awaiting_integration" || t.Status == "needs_attention") {
			rev := t.Revision
			t.Revision++
			t.Status = "completed"
			t.UpdatedAt = time.Now()
			if err := s.Store.PutAgentTask(ctx, &t, rev); err != nil {
				return err
			}
		}
	}
	s.Runtime.Emit(ctx, a.RootID, a.AgentID, "", "integration_finished", a)
	return nil
}

// RefreshArtifact records a nested workspace change and reopens acceptance of
// its parent task without discarding the existing result or responsibility.
func (s *Service) RefreshArtifact(ctx context.Context, a *domain.AgentArtifact, changed bool) error {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	if err := s.Store.PutAgentArtifact(ctx, a); err != nil {
		return err
	}
	if !changed {
		return nil
	}
	tasks, err := s.Store.ListAgentTasks(ctx, a.RootID)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.ArtifactID != a.ID || task.Status == "deleted" || task.Status == "canceled" {
			continue
		}
		revision := task.Revision
		task.Revision++
		task.Status = "awaiting_integration"
		task.UpdatedAt = time.Now()
		if err := s.Store.PutAgentTask(ctx, &task, revision); err != nil {
			return err
		}
		s.invalidateDependents(ctx, Caller{AgentID: a.RootID, RootID: a.RootID}, task.ID, tasks)
	}
	s.Runtime.Emit(ctx, a.RootID, a.AgentID, "", "artifact_updated", a)
	return nil
}
