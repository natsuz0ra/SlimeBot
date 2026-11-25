package chat

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"slimebot/internal/domain"
)

var ErrTurnBusy = errors.New("a turn is already running for this session")
var ErrStaleTurn = errors.New("the target turn has ended or is no longer accepting changes")

type SteeringMessage struct {
	SubmissionID string    `json:"submissionId"`
	MessageID    string    `json:"messageId"`
	TurnID       string    `json:"turnId"`
	Content      string    `json:"content"`
	CreatedAt    time.Time `json:"createdAt"`
}

type turnInbox struct {
	mu        sync.Mutex
	id        string
	closed    bool
	ready     bool
	pending   []SteeringMessage
	accepted  map[string]SteeringMessage
	changed   chan struct{}
	finished  bool
	delivered <-chan struct{}
	done      chan struct{}
}
type turnInboxKey struct{}

func (s *ChatService) beginTurn(ctx context.Context, sessionID, requestID string) (context.Context, func(), error) {
	release, err := s.acquireTurnWithContext(ctx, sessionID)
	if err != nil {
		return ctx, nil, err
	}
	delivered, _ := ctx.Value(deliveryKey{}).(<-chan struct{})
	inbox := &turnInbox{delivered: delivered, id: requestID, accepted: map[string]SteeringMessage{}, changed: make(chan struct{}), done: make(chan struct{})}
	for {
		previous, loaded := s.turnInboxes.LoadOrStore(sessionID, inbox)
		if !loaded {
			break
		}
		old := previous.(*turnInbox)
		select {
		case <-old.done:
			if old.delivered != nil {
				select {
				case <-old.delivered:
				default:
					release()
					return ctx, nil, ErrTurnBusy
				}
			}
			s.turnInboxes.CompareAndDelete(sessionID, old)
		default:
			release()
			return ctx, nil, ErrTurnBusy
		}
	}
	return context.WithValue(ctx, turnInboxKey{}, inbox), func() {
		inbox.mu.Lock()
		if inbox.finished {
			inbox.mu.Unlock()
			return
		}
		inbox.finished = true
		inbox.closed = true
		release()
		close(inbox.done)
		inbox.mu.Unlock()
		remove := func() { s.turnInboxes.CompareAndDelete(sessionID, inbox) }
		if inbox.delivered == nil {
			remove()
		} else {
			go func() { <-inbox.delivered; remove() }()
		}

	}, nil
}

// Steer durably accepts a text message for exactly one active turn. Submission
// IDs make retries idempotent; an old turn can never steer a replacement turn.
func (s *ChatService) Steer(ctx context.Context, sessionID, turnID, submissionID, content string) (SteeringMessage, error) {
	content = strings.TrimSpace(content)
	if sessionID == "" || turnID == "" || submissionID == "" || len(submissionID) > 128 || content == "" || len(content) > 64<<10 || !utf8.ValidString(content) {
		return SteeringMessage{}, errors.New("a session, active turn, submission ID and text (up to 64 KiB) are required")
	}
	value, ok := s.turnInboxes.Load(sessionID)
	if !ok {
		return SteeringMessage{}, ErrStaleTurn
	}
	inbox := value.(*turnInbox)
	inbox.mu.Lock()
	defer inbox.mu.Unlock()
	if inbox.id != turnID {
		return SteeringMessage{}, ErrStaleTurn
	}
	if previous, ok := inbox.accepted[submissionID]; ok {
		if previous.Content != content {
			return SteeringMessage{}, errors.New("submission ID was already used for different text")
		}
		return previous, nil
	}
	if inbox.closed || !inbox.ready {
		return SteeringMessage{}, ErrStaleTurn
	}
	if len(inbox.accepted) >= 64 {
		return SteeringMessage{}, errors.New("too many changes for one turn")
	}
	now := time.Now()
	message, err := s.store.AddMessageWithInput(ctx, domain.AddMessageInput{SessionID: sessionID, Role: "user", Content: content, CreatedAt: now})
	if err != nil {
		return SteeringMessage{}, err
	}
	accepted := SteeringMessage{submissionID, message.ID, turnID, content, now}
	if len(inbox.pending) == 0 {
		close(inbox.changed)
	}
	inbox.pending = append(inbox.pending, accepted)
	inbox.accepted[submissionID] = accepted
	return accepted, nil
}

func (i *turnInbox) open() { i.mu.Lock(); i.ready = true; i.mu.Unlock() }

func (i *turnInbox) hasPending() bool {
	if i == nil {
		return false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return len(i.pending) > 0
}

// seal and acceptance share the same lock: every acknowledged message is either
// consumed by the loop or remains persisted if the user cancels the turn.
func (i *turnInbox) drain(seal bool) []SteeringMessage {
	if i == nil {
		return nil
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	items := i.pending
	i.pending = nil
	if len(items) > 0 {
		i.changed = make(chan struct{})
	} else if seal {
		i.closed = true
	}
	return items
}

func (i *turnInbox) approvalContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	i.mu.Lock()
	changed := i.changed
	i.mu.Unlock()
	go func() {
		select {
		case <-changed:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}
