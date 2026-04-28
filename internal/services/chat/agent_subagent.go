package chat

// wrapSubagentCallbacks routes child stream chunks to OnSubagentChunk and tags nested tool events.
func wrapSubagentCallbacks(base AgentCallbacks, parentToolCallID, subagentRunID string) AgentCallbacks {
	thinkingMeta := ThinkingEventMeta{ParentToolCallID: parentToolCallID, SubagentRunID: subagentRunID}
	return AgentCallbacks{
		OnChunk: func(chunk string) error {
			if chunk == "" {
				return nil
			}
			if base.OnSubagentChunk != nil {
				return base.OnSubagentChunk(parentToolCallID, subagentRunID, chunk)
			}
			return nil
		},
		OnToolCallStart: func(req ApprovalRequest) error {
			if base.OnToolCallStart == nil {
				return nil
			}
			req.ParentToolCallID = parentToolCallID
			req.SubagentRunID = subagentRunID
			return base.OnToolCallStart(req)
		},
		WaitApproval: base.WaitApproval,
		OnToolCallResult: func(result ToolCallResult) error {
			if base.OnToolCallResult == nil {
				return nil
			}
			result.ParentToolCallID = parentToolCallID
			result.SubagentRunID = subagentRunID
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
