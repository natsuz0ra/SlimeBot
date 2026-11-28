package chat

import (
	"sync"
	"time"
)

// EventSink adapts external notifications into best-effort callbacks for one turn.
type EventSink struct {
	mu      sync.Mutex
	pushErr error
}

func (s *EventSink) capture(err error) error {
	if err != nil {
		s.mu.Lock()
		if s.pushErr == nil {
			s.pushErr = err
		}
		s.mu.Unlock()
	}
	return nil
}

func (s *EventSink) failure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pushErr
}

func notifyEvent[T any](s *EventSink, send func(T) error) func(T) error {
	if send == nil {
		return nil
	}
	return func(event T) error { return s.capture(send(event)) }
}

func (s *EventSink) adapt(cb AgentCallbacks) AgentCallbacks {
	cb.OnTurnStarted = notifyEvent(s, cb.OnTurnStarted)
	cb.OnPrivacy = notifyEvent(s, cb.OnPrivacy)
	cb.OnChunk = notifyEvent(s, cb.OnChunk)
	cb.OnContextUsage = notifyEvent(s, cb.OnContextUsage)
	cb.OnContextCompacted = notifyEvent(s, cb.OnContextCompacted)
	cb.OnToolCallStart = notifyEvent(s, cb.OnToolCallStart)
	cb.OnToolApprovalReview = notifyEvent(s, cb.OnToolApprovalReview)
	cb.OnToolApprovalRequired = notifyEvent(s, cb.OnToolApprovalRequired)
	cb.OnToolCallResult = notifyEvent(s, cb.OnToolCallResult)
	cb.OnTeamStart = notifyEvent(s, cb.OnTeamStart)
	cb.OnTeamMemberQueued = notifyEvent(s, cb.OnTeamMemberQueued)
	cb.OnTeamDone = notifyEvent(s, cb.OnTeamDone)
	cb.OnThinkingStart = notifyEvent(s, cb.OnThinkingStart)
	cb.OnThinkingDone = notifyEvent(s, cb.OnThinkingDone)
	cb.OnTodoUpdate = notifyEvent(s, cb.OnTodoUpdate)
	cb.OnPlanChunk = notifyEvent(s, cb.OnPlanChunk)
	cb.OnPlanBody = notifyEvent(s, cb.OnPlanBody)
	if send := cb.OnMessageEdited; send != nil {
		cb.OnMessageEdited = func(id, content string, at time.Time) error { return s.capture(send(id, content, at)) }
	}
	if send := cb.OnThinkingChunk; send != nil {
		cb.OnThinkingChunk = func(chunk string, meta ThinkingEventMeta) error { return s.capture(send(chunk, meta)) }
	}
	if send := cb.OnSubagentStart; send != nil {
		cb.OnSubagentStart = func(meta AgentEventMeta, title, task string) error { return s.capture(send(meta, title, task)) }
	}
	if send := cb.OnSubagentChunk; send != nil {
		cb.OnSubagentChunk = func(meta AgentEventMeta, chunk string) error { return s.capture(send(meta, chunk)) }
	}
	if send := cb.OnSubagentDone; send != nil {
		cb.OnSubagentDone = func(meta AgentEventMeta, err error) error { return s.capture(send(meta, err)) }
	}
	// WaitApproval supplies an authorization decision and retains its error contract.
	return cb
}
