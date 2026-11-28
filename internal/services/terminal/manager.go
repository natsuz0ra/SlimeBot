package terminal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const MaxSessions = 8
const replaySize = 64 << 10
const idleTimeout = 30 * time.Minute

var (
	ErrNotFound = errors.New("terminal session not found")
	ErrQuota    = errors.New("terminal session quota reached")
	ErrClosed   = errors.New("terminal is closed")
	ErrSize     = errors.New("invalid terminal dimensions")
)

type Options struct {
	ChatSessionID string `json:"chatSessionId"`
	Cwd           string `json:"cwd"`
	Shell         string `json:"shell"`
	Cols          int    `json:"cols"`
	Rows          int    `json:"rows"`
}

type SessionInfo struct {
	ID string `json:"id"`
	Options
	Title        string    `json:"title"`
	Status       string    `json:"status"`
	ExitCode     *int      `json:"exitCode,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	LastActiveAt time.Time `json:"lastActiveAt"`
}

type ringBuffer struct {
	data       [replaySize]byte
	next, size int
}

func (r *ringBuffer) append(data []byte) {
	if len(data) > len(r.data) {
		data = data[len(data)-len(r.data):]
	}
	first := copy(r.data[r.next:], data)
	copy(r.data[:], data[first:])
	r.next = (r.next + len(data)) % len(r.data)
	r.size = min(r.size+len(data), len(r.data))
}

func (r *ringBuffer) snapshot() []byte {
	start := (r.next - r.size + len(r.data)) % len(r.data)
	out := make([]byte, r.size)
	first := copy(out, r.data[start:])
	copy(out[first:], r.data[:r.next])
	return out
}

type Manager struct {
	mu        sync.Mutex
	sessions  map[string]*Session
	stop      chan struct{}
	done      chan struct{}
	processes sync.WaitGroup
	running   int
	start     func(shell, cwd string, cols, rows int) (PTY, error)
}

func NewManager() *Manager {
	m := &Manager{sessions: make(map[string]*Session), stop: make(chan struct{}), done: make(chan struct{}), start: startPTY}
	go m.reap()
	return m
}

func (m *Manager) Create(owner string, opts Options, configuredShell string) (SessionInfo, error) {
	if opts.Cols == 0 {
		opts.Cols = 80
	}
	if opts.Rows == 0 {
		opts.Rows = 24
	}
	if !validSize(opts.Cols, opts.Rows) {
		return SessionInfo{}, ErrSize
	}
	opts.Shell = DetectShell(strings.TrimSpace(opts.Shell), configuredShell)
	if opts.Shell == "" {
		return SessionInfo{}, errors.New("no executable shell found")
	}
	if opts.Cwd == "" {
		var err error
		opts.Cwd, err = os.UserHomeDir()
		if err != nil {
			return SessionInfo{}, err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-m.stop:
		return SessionInfo{}, ErrClosed
	default:
	}
	if m.running >= MaxSessions || len(m.sessions) >= MaxSessions*4 {
		return SessionInfo{}, ErrQuota
	}
	p, err := m.start(opts.Shell, opts.Cwd, opts.Cols, opts.Rows)
	if err != nil {
		return SessionInfo{}, err
	}
	now := time.Now()
	info := SessionInfo{ID: uuid.NewString(), Options: opts, Status: "running", CreatedAt: now, LastActiveAt: now,
		Title: trimExeSuffix(filepath.Base(opts.Shell)) + " · " + filepath.Base(opts.Cwd)}
	s := &Session{owner: owner, pty: p, info: info, subscribers: make(map[chan []byte]struct{}), closed: make(chan struct{}), done: make(chan struct{})}
	m.sessions[info.ID] = s
	m.running++
	m.processes.Go(func() {
		s.run()
		m.mu.Lock()
		m.running--
		m.mu.Unlock()
		close(s.done)
	})
	return info, nil
}

func (m *Manager) List(owner string) []SessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]SessionInfo, 0)
	for _, s := range m.sessions {
		if s.owner == owner {
			result = append(result, s.Info())
		}
	}
	slices.SortFunc(result, func(a, b SessionInfo) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return result
}

func (m *Manager) Get(owner, id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s == nil || s.owner != owner {
		return nil, ErrNotFound
	}
	return s, nil
}

func (m *Manager) Delete(ctx context.Context, owner, id string) error {
	m.mu.Lock()
	s := m.sessions[id]
	if s == nil || s.owner != owner {
		m.mu.Unlock()
		return ErrNotFound
	}
	delete(m.sessions, id)
	m.mu.Unlock()
	s.close()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) reap() {
	defer close(m.done)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case now := <-ticker.C:
			m.reapIdle(now)
		}
	}
}

func (m *Manager) reapIdle(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sessions {
		s.mu.Lock()
		if len(s.subscribers) == 0 && now.Sub(s.info.LastActiveAt) >= idleTimeout {
			delete(m.sessions, id)
			s.close()
		}
		s.mu.Unlock()
	}
}

func (m *Manager) CloseAll(ctx context.Context) {
	m.mu.Lock()
	select {
	case <-m.stop:
	default:
		close(m.stop)
		for _, s := range m.sessions {
			s.close()
		}
		clear(m.sessions)
	}
	m.mu.Unlock()
	finished := make(chan struct{})
	go func() {
		<-m.done
		m.processes.Wait()
		close(finished)
	}()
	select {
	case <-ctx.Done():
	case <-finished:
	}
}

func (m *Manager) StopSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		s.close()
	}
	clear(m.sessions)
}

type Session struct {
	owner       string
	pty         PTY
	mu          sync.Mutex
	info        SessionInfo
	replay      ringBuffer
	subscribers map[chan []byte]struct{}
	closed      chan struct{}
	done        chan struct{}
	closeOnce   sync.Once
}

func validSize(cols, rows int) bool { return cols >= 1 && cols <= 500 && rows >= 1 && rows <= 500 }

func trimExeSuffix(name string) string {
	if len(name) >= 4 && strings.EqualFold(name[len(name)-4:], ".exe") {
		return name[:len(name)-4]
	}
	return name
}

func (s *Session) Info() SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info
}

func (s *Session) Subscribe() (SessionInfo, []byte, <-chan []byte, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stream := make(chan []byte, 32)
	if s.info.Status == "exited" {
		close(stream)
	} else {
		s.subscribers[stream] = struct{}{}
	}
	s.info.LastActiveAt = time.Now()
	return s.info, s.replay.snapshot(), stream, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.subscribers[stream]; ok {
			delete(s.subscribers, stream)
			close(stream)
		}
		s.info.LastActiveAt = time.Now()
	}
}

func (s *Session) Write(data []byte) error {
	s.mu.Lock()
	s.info.LastActiveAt = time.Now()
	s.mu.Unlock()
	_, err := s.pty.Write(data)
	return err
}

func (s *Session) Resize(cols, rows int) error {
	if !validSize(cols, rows) {
		return ErrSize
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.info.Status != "running" {
		return ErrClosed
	}
	select {
	case <-s.closed:
		return ErrClosed
	default:
	}
	if err := s.pty.Resize(cols, rows); err != nil {
		return err
	}
	s.info.Cols, s.info.Rows = cols, rows
	s.info.LastActiveAt = time.Now()
	return nil
}

func (s *Session) close() {
	s.closeOnce.Do(func() { close(s.closed) })
}

func (s *Session) run() {
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		buf := make([]byte, 4096)
		for {
			n, err := s.pty.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.replay.append(buf[:n])
				s.info.LastActiveAt = time.Now()
				data := slices.Clone(buf[:n])
				for stream := range s.subscribers {
					select {
					case stream <- data:
					default:
						delete(s.subscribers, stream)
						close(stream)
					}
				}
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	waited := make(chan int, 1)
	go func() { waited <- s.pty.Wait() }()
	var code int
	select {
	case code = <-waited:
		// Allow the reader to consume the Unix PTY's final buffered bytes before closing it.
		select {
		case <-drained:
		case <-s.closed:
		case <-time.After(100 * time.Millisecond):
		}
		_ = s.pty.Close()
	case <-s.closed:
		_ = s.pty.Close()
		code = <-waited
	}
	<-drained
	s.mu.Lock()
	defer s.mu.Unlock()
	s.info.Status = "exited"
	s.info.ExitCode = &code
	s.info.LastActiveAt = time.Now()
	for stream := range s.subscribers {
		close(stream)
	}
	clear(s.subscribers)
}
