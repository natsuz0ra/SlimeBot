export type ToolCallStatus = 'pending' | 'rejected' | 'executing' | 'completed' | 'error'

export type ReplyTimelineEntry =
  | { id: string; kind: 'text'; content: string }
  | { id: string; kind: 'notice'; content: string }
  | { id: string; kind: 'plan'; content: string; generating?: boolean }
  | { id: string; kind: 'tool_start'; toolCallId: string }
  | { id: string; kind: 'tool_result'; toolCallId: string }
  | { id: string; kind: 'thinking'; content: string; done: boolean; durationMs?: number; startedAt?: number }

export type ToolTimelineEntry =
  | { id: string; kind: 'tool_start'; toolCallId: string }
  | { id: string; kind: 'tool_result'; toolCallId: string }
  | { id: string; kind: 'thinking'; content: string; done: boolean; durationMs?: number; startedAt?: number }
