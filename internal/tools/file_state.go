package tools

import (
	"context"
	"sync"
)

type fileToolContextKey struct{}

// ReadFileState tracks files the model has read during a chat session.
type ReadFileState struct {
	mu    sync.Mutex
	files map[string]ReadFileEntry
}

// ReadFileEntry is the last observed full or partial view of a text file.
type ReadFileEntry struct {
	Content   string
	MTimeUnix int64
	Offset    int
	Limit     int
	Partial   bool
}

// NewReadFileState constructs an empty file state cache.
func NewReadFileState() *ReadFileState {
	return &ReadFileState{files: make(map[string]ReadFileEntry)}
}

// WithReadFileState attaches a file state cache to a tool execution context.
func WithReadFileState(ctx context.Context, state *ReadFileState) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if state == nil {
		return ctx
	}
	return context.WithValue(ctx, fileToolContextKey{}, state)
}

func readFileStateFromContext(ctx context.Context) *ReadFileState {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(fileToolContextKey{}).(*ReadFileState)
	return state
}

func (s *ReadFileState) get(path string) (ReadFileEntry, bool) {
	if s == nil {
		return ReadFileEntry{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.files[path]
	return entry, ok
}

func (s *ReadFileState) set(path string, entry ReadFileEntry) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.files == nil {
		s.files = make(map[string]ReadFileEntry)
	}
	s.files[path] = entry
}
