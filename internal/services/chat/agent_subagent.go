package chat

// wrapSubagentCallbacks routes child stream chunks to OnSubagentChunk and tags nested tool events.
func wrapSubagentCallbacks(base AgentCallbacks, meta AgentEventMeta) AgentCallbacks {
	thinkingMeta := ThinkingEventMeta{
		ParentToolCallID: meta.ParentToolCallID,
		SubagentRunID:    meta.SubagentRunID,
		TeamRunID:        meta.TeamRunID,
		MemberRunID:      meta.MemberRunID,
	}
	return AgentCallbacks{
		OnPrivacy: base.OnPrivacy,
		OnChunk: func(chunk string) error {
			if chunk == "" {
				return nil
			}
			if base.OnSubagentChunk != nil {
				return base.OnSubagentChunk(meta, chunk)
			}
			return nil
		},
		OnToolCallStart: func(req ApprovalRequest) error {
			if base.OnToolCallStart == nil {
				return nil
			}
			req.ParentToolCallID = meta.ParentToolCallID
			req.SubagentRunID = meta.SubagentRunID
			req.TeamRunID = meta.TeamRunID
			req.MemberRunID = meta.MemberRunID
			return base.OnToolCallStart(req)
		},
		OnToolApprovalReview: func(event ApprovalReviewEvent) error {
			if base.OnToolApprovalReview == nil {
				return nil
			}
			event.ParentToolCallID = meta.ParentToolCallID
			event.SubagentRunID = meta.SubagentRunID
			event.TeamRunID = meta.TeamRunID
			event.MemberRunID = meta.MemberRunID
			return base.OnToolApprovalReview(event)
		},
		OnToolApprovalRequired: func(req ApprovalRequest) error {
			if base.OnToolApprovalRequired == nil {
				return nil
			}
			req.ParentToolCallID = meta.ParentToolCallID
			req.SubagentRunID = meta.SubagentRunID
			req.TeamRunID = meta.TeamRunID
			req.MemberRunID = meta.MemberRunID
			return base.OnToolApprovalRequired(req)
		},
		WaitApproval: base.WaitApproval,
		OnToolCallResult: func(result ToolCallResult) error {
			if base.OnToolCallResult == nil {
				return nil
			}
			result.ParentToolCallID = meta.ParentToolCallID
			result.SubagentRunID = meta.SubagentRunID
			result.TeamRunID = meta.TeamRunID
			result.MemberRunID = meta.MemberRunID
			return base.OnToolCallResult(result)
		},
		OnThinkingStart: func(_ ThinkingEventMeta) error {
			if base.OnThinkingStart == nil {
				return nil
			}
			return base.OnThinkingStart(thinkingMeta)
		},
		OnThinkingChunk: func(chunk string, _ ThinkingEventMeta) error {
			if base.OnThinkingChunk == nil {
				return nil
			}
			return base.OnThinkingChunk(chunk, thinkingMeta)
		},
		OnThinkingDone: func(_ ThinkingEventMeta) error {
			if base.OnThinkingDone == nil {
				return nil
			}
			return base.OnThinkingDone(thinkingMeta)
		},
	}
}
