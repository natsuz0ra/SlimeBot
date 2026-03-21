export type ToolCallStatus = 'pending' | 'rejected' | 'executing' | 'completed' | 'error'

export type ReplyTimelineEntry =
  | { id: string; kind: 'text'; content: string }
  | { id: string; kind: 'tool_start'; toolCallId: string }
  | { id: string; kind: 'tool_result'; toolCallId: string }

export type ToolTimelineEntry = {
  id: string
  kind: 'tool_start' | 'tool_result'
  toolCallId: string
}
