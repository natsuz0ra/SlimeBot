package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var defaultProcessManager = NewProcessManager()

type processRunResult struct {
	Stdout     string
	Stderr     string
	ExitCode   int
	TimedOut   bool
	Error      string
	DurationMs int64
}

type managedProcess struct {
	ID        string
	Command   string
	StartedAt time.Time
	EndedAt   time.Time
	Status    string
	Result    processRunResult
	done      chan struct{}
	cancel    context.CancelFunc
}

type ProcessManager struct {
	mu       sync.Mutex
	nextID   int
	items    map[string]*managedProcess
	maxItems int
}

func NewProcessManager() *ProcessManager {
	return &ProcessManager{items: make(map[string]*managedProcess), maxItems: 128}
}

func (m *ProcessManager) Start(command string, runner func(context.Context) processRunResult) string {
	if m == nil {
		m = defaultProcessManager
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.nextID++
	id := fmt.Sprintf("proc-%d", m.nextID)
	item := &managedProcess{
		ID:        id,
		Command:   strings.TrimSpace(command),
		StartedAt: time.Now(),
		Status:    "running",
		cancel:    cancel,
		done:      make(chan struct{}),
	}
	m.items[id] = item
	m.trimLocked()
	m.mu.Unlock()

	go func() {
		defer close(item.done)
		result := runner(ctx)
		m.mu.Lock()
		defer m.mu.Unlock()
		if current := m.items[id]; current != nil {
			current.Result = result
			current.EndedAt = time.Now()
			if current.Status == "stopping" {
				current.Status = "stopped"
			} else {
				current.Status = "completed"
			}
		}
	}()
	return id
}

func (m *ProcessManager) List() []managedProcess {
	if m == nil {
		m = defaultProcessManager
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]managedProcess, 0, len(m.items))
	for _, item := range m.items {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].StartedAt.Before(items[j].StartedAt)
	})
	return items
}

func (m *ProcessManager) Get(id string) (managedProcess, bool) {
	if m == nil {
		m = defaultProcessManager
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[strings.TrimSpace(id)]
	if item == nil {
		return managedProcess{}, false
	}
	return *item, true
}

func (m *ProcessManager) Stop(id string) (managedProcess, bool) {
	if m == nil {
		m = defaultProcessManager
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[strings.TrimSpace(id)]
	if item == nil {
		return managedProcess{}, false
	}
	if item.Status == "running" {
		item.Status = "stopped"
		item.cancel()
	}
	return *item, true
}

func (m *ProcessManager) trimLocked() {
	for len(m.items) > m.maxItems {
		var oldestID string
		var oldest time.Time
		for id, item := range m.items {
			if oldestID == "" || item.StartedAt.Before(oldest) {
				oldestID = id
				oldest = item.StartedAt
			}
		}
		delete(m.items, oldestID)
	}
}

type processTool struct{}

func init() {
	Register(&processTool{})
}

func (p *processTool) Name() string { return "process" }

func (p *processTool) Description() string {
	return "List, inspect, and stop background processes started by SlimeBot tools."
}

func (p *processTool) Commands() []Command {
	return []Command{
		{Name: "list", Description: "List managed background processes for this runtime."},
		{
			Name:        "status",
			Description: "Show status and captured output summary for one managed background process.",
			Params:      []CommandParam{{Name: "process_id", Required: true, Description: "Managed process id.", Example: "proc-1"}},
		},
		{
			Name:        "stop",
			Description: "Request cancellation for one running managed background process.",
			Params:      []CommandParam{{Name: "process_id", Required: true, Description: "Managed process id.", Example: "proc-1"}},
		},
	}
}

func (p *processTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	switch command {
	case "list":
		return p.list(ctx, params)
	case "status":
		return p.status(ctx, params)
	case "stop":
		return p.stop(ctx, params)
	default:
		return nil, fmt.Errorf("process tool does not support command: %s", command)
	}
}

func (p *processTool) list(ctx context.Context, _ map[string]any) (*ExecuteResult, error) {
	items := processManagerFromContext(ctx).List()
	if len(items) == 0 {
		return &ExecuteResult{Output: "No managed processes."}, nil
	}
	var out strings.Builder
	for _, item := range items {
		out.WriteString(formatManagedProcess(item, false) + "\n")
	}
	return &ExecuteResult{Output: strings.TrimSpace(out.String())}, nil
}

func (p *processTool) status(ctx context.Context, params map[string]any) (*ExecuteResult, error) {
	id := paramStringTrim(params, "process_id")
	if id == "" {
		return nil, fmt.Errorf("process_id is required")
	}
	item, ok := processManagerFromContext(ctx).Get(id)
	if !ok {
		return nil, fmt.Errorf("managed process not found: %s", id)
	}
	return &ExecuteResult{Output: formatManagedProcess(item, true)}, nil
}

func (p *processTool) stop(ctx context.Context, params map[string]any) (*ExecuteResult, error) {
	id := paramStringTrim(params, "process_id")
	if id == "" {
		return nil, fmt.Errorf("process_id is required")
	}
	item, ok := processManagerFromContext(ctx).Stop(id)
	if !ok {
		return nil, fmt.Errorf("managed process not found: %s", id)
	}
	return &ExecuteResult{Output: "Stop requested.\n" + formatManagedProcess(item, true)}, nil
}

func formatManagedProcess(item managedProcess, includeOutput bool) string {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("%s [%s] %s", item.ID, item.Status, item.Command))
	if !item.StartedAt.IsZero() {
		out.WriteString(fmt.Sprintf("\nStarted: %s", item.StartedAt.Format(time.RFC3339)))
	}
	if !item.EndedAt.IsZero() {
		out.WriteString(fmt.Sprintf("\nEnded: %s", item.EndedAt.Format(time.RFC3339)))
	}
	if includeOutput && item.Status != "running" && item.Status != "stopping" {
		out.WriteString(fmt.Sprintf("\nExitCode: %d TimedOut: %t DurationMs: %d", item.Result.ExitCode, item.Result.TimedOut, item.Result.DurationMs))
		if item.Result.Stdout != "" {
			out.WriteString("\nStdout:\n" + item.Result.Stdout)
		}
		if item.Result.Stderr != "" {
			out.WriteString("\nStderr:\n" + item.Result.Stderr)
		}
		if item.Result.Error != "" {
			out.WriteString("\nError: " + item.Result.Error)
		}
	}
	return out.String()
}

// StopAndWait drains owned commands before a worktree is captured or removed.
func (m *ProcessManager) StopAndWait(ctx context.Context) error {
	m.mu.Lock()
	var done []chan struct{}
	for _, p := range m.items {
		if p.Status == "running" || p.Status == "stopping" {
			p.Status = "stopping"
			p.cancel()
			done = append(done, p.done)
		}
	}
	m.mu.Unlock()
	for _, ch := range done {
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// HasRunning keeps an eviction from detaching processes that still need draining.
func (m *ProcessManager) HasRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.items {
		if item.Status == "running" || item.Status == "stopping" {
			return true
		}
	}
	return false
}
