package agent

import (
	"context"
	"io"
	"log/slog"
	"sync"

	"slimebot/internal/domain"
	chatsvc "slimebot/internal/services/chat"

	sdk "github.com/coder/acp-go-sdk"
)

// ChatRunner drives one SlimeBot chat turn on behalf of the ACP agent.
type ChatRunner interface {
	HandleChatStream(ctx context.Context, sessionID, requestID, content, displayContent, modelID string,
		attachmentIDs []string, thinkingLevel string, planMode bool, subagentModelID, approvalModeOverride string,
		callbacks chatsvc.AgentCallbacks) (*chatsvc.ChatStreamResult, error)
}

// SessionStore creates ACP-backed sessions and replays stored history.
type SessionStore interface {
	Create(ctx context.Context, name string, workingDirectory ...string) (*domain.Session, error)
	ReplaySession(ctx context.Context, id string) (domain.SessionExportSnapshot, error)
}

// ModelLister exposes the configured SlimeBot models.
type ModelLister interface {
	List(ctx context.Context) ([]domain.LLMConfig, error)
}

// UploadRegistrar stores prompt attachments as chat uploads.
type UploadRegistrar interface {
	RegisterLocalFiles(sessionID string, files []chatsvc.LocalAttachmentFile) ([]chatsvc.UploadedAttachment, error)
	Consume(sessionID string, ids []string) ([]chatsvc.UploadedAttachment, error)
	Cleanup(items []chatsvc.UploadedAttachment)
}

type Options struct {
	Chat           ChatRunner
	Sessions       SessionStore
	Models         ModelLister
	Uploads        UploadRegistrar
	PreferredModel string
	ApprovalMode   string
	Version        string
}

type sessionState struct {
	cwd, model, mode, thinking, approval string
}

type bridge struct {
	Options
	ctx      context.Context
	ready    chan struct{}
	conn     *sdk.AgentSideConnection
	mu       sync.Mutex
	states   map[sdk.SessionId]sessionState
	closed   bool
	requests sync.WaitGroup
}

// readyReader holds back peer input until the connection has finished
// wiring itself. The SDK starts its receive loop inside
// NewAgentSideConnection, before that constructor has stored the connection,
// so an eager client message could otherwise be handled against a nil
// connection.
type readyReader struct {
	reader io.Reader
	ready  <-chan struct{}
}

func (r *readyReader) Read(p []byte) (int, error) {
	<-r.ready
	return r.reader.Read(p)
}

func Serve(ctx context.Context, input io.Reader, output io.Writer, options Options, logger *slog.Logger) {
	lifecycle, cancel := context.WithCancel(ctx)
	defer cancel()
	b := &bridge{Options: options, ctx: lifecycle, ready: make(chan struct{}), states: make(map[sdk.SessionId]sessionState)}
	gate := make(chan struct{})
	b.conn = sdk.NewAgentSideConnection(b, output, &readyReader{reader: input, ready: gate})
	close(gate)
	if logger != nil {
		b.conn.SetLogger(logger)
	}
	close(b.ready)
	select {
	case <-ctx.Done():
	case <-b.conn.Done():
	}
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	cancel()
	b.requests.Wait()
}

func (b *bridge) connection(ctx context.Context) (*sdk.AgentSideConnection, error) {
	select {
	case <-b.ready:
		return b.conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *bridge) request(ctx context.Context) (context.Context, func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, nil, context.Canceled
	}
	b.requests.Add(1)
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(b.ctx, cancel)
	return ctx, func() { stop(); cancel(); b.requests.Done() }, nil
}
