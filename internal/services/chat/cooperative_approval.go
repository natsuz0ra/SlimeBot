package chat

import (
	"context"
	"encoding/json"
	"slimebot/internal/domain"
	subagent "slimebot/internal/services/subagent"
	"time"
)

// The transport is one subscriber; durable approval decisions remain available
// after it disconnects. Both paths resolve the same pending approval once.
func (s *ChatService) cooperativeApprovals(ctx context.Context, c subagent.Caller, turn string, cb AgentCallbacks) AgentCallbacks {
	id := func(tool string) string { return turn + ":" + tool }
	record := func(req ApprovalRequest) error {
		b, err := json.Marshal(req)
		if err != nil {
			return err
		}
		a := &domain.AgentApproval{ID: id(req.ToolCallID), RootID: c.RootID, RequestID: c.RequestID, AgentID: c.AgentID, TurnID: turn, ToolCallID: req.ToolCallID, Payload: string(b), Status: "pending", CreatedAt: time.Now()}
		if err = s.cooperative.Store.PutAgentApproval(ctx, a); err != nil {
			return err
		}
		s.cooperative.Runtime.Emit(ctx, c.RootID, c.AgentID, turn, "approval_required", a)
		return nil
	}
	start, required, wait, result := cb.OnToolCallStart, cb.OnToolApprovalRequired, cb.WaitApproval, cb.OnToolCallResult
	cb.OnToolCallStart = func(req ApprovalRequest) error {
		if req.RequiresApproval && req.ReviewStatus != "reviewing" {
			if err := record(req); err != nil {
				return err
			}
		}
		if start != nil {
			return start(req)
		}
		return nil
	}
	cb.OnToolApprovalRequired = func(req ApprovalRequest) error {
		if err := record(req); err != nil {
			return err
		}
		if required != nil {
			return required(req)
		}
		return nil
	}
	cb.WaitApproval = func(waitCtx context.Context, tool string) (*ApprovalResponse, error) {
		listen, cancel := context.WithCancel(waitCtx)
		defer cancel()
		answers := make(chan *ApprovalResponse, 1)
		if wait != nil {
			go func() {
				resp, err := wait(listen, tool)
				if err == nil && resp != nil {
					select {
					case answers <- resp:
					case <-listen.Done():
					}
				}
			}()
		}
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			as, err := s.cooperative.Store.ListAgentApprovals(waitCtx, c.RootID)
			if err != nil {
				return nil, err
			}
			for _, a := range as {
				if a.ID == id(tool) && a.Status != "pending" {
					return &ApprovalResponse{ToolCallID: tool, Approved: a.Status == "approved"}, nil
				}
			}
			select {
			case <-waitCtx.Done():
				_ = s.cooperative.Store.ResolveAgentApproval(context.WithoutCancel(waitCtx), id(tool), "expired")
				return nil, waitCtx.Err()
			case resp := <-answers:
				status := "rejected"
				if resp.Approved {
					status = "approved"
				}
				if err := s.cooperative.Store.ResolveAgentApproval(waitCtx, id(tool), status); err == nil {
					return resp, nil
				}
			case <-ticker.C:
			}
		}
	}
	cb.OnToolCallResult = func(x ToolCallResult) error {
		_ = s.cooperative.Store.ResolveAgentApproval(context.WithoutCancel(ctx), id(x.ToolCallID), "expired")
		if result != nil {
			return result(x)
		}
		return nil
	}
	return cb
}
