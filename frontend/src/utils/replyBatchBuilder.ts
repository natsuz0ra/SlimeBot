import type { SessionHistoryPayload, SessionHistoryToolCallItem, ToolCallItem } from '@/api/chat'
import type { ToolCallStatus } from '@/types/chat'

export type AssistantReplyTimelineItem =
  | {
      id: string
      kind: 'tool_start'
      toolCallId: string
    }
  | {
      id: string
      kind: 'tool_result'
      toolCallId: string
    }
  | {
      id: string
      kind: 'text'
      content: string
    }

export interface AssistantReplyBatch {
  id: string
  sessionId: string
  assistantMessageId: string
  toolCalls: ToolCallItem[]
  timeline: AssistantReplyTimelineItem[]
  collapsed: boolean
}

export function normalizeToolStatus(status?: string, fallbackError?: string): ToolCallStatus {
  if (status === 'pending' || status === 'rejected' || status === 'executing' || status === 'completed' || status === 'error') {
    return status
  }
  return fallbackError ? 'error' : 'completed'
}

export function buildReplyBatchesFromHistory(sessionId: string, history: SessionHistoryPayload): AssistantReplyBatch[] {
  const nextBatches: AssistantReplyBatch[] = []
  for (const message of history.messages) {
    if (message.role !== 'assistant') continue
    const historyToolCalls = history.toolCallsByAssistantMessageId[message.id] || []
    if (historyToolCalls.length === 0) continue

    const sortedCalls = [...historyToolCalls].sort((left, right) => {
      const leftAt = new Date(left.startedAt || 0).getTime()
      const rightAt = new Date(right.startedAt || 0).getTime()
      return leftAt - rightAt
    })

    const toolCalls: ToolCallItem[] = sortedCalls.map((item: SessionHistoryToolCallItem) => ({
      toolCallId: item.toolCallId,
      toolName: item.toolName,
      command: item.command,
      params: item.params || {},
      requiresApproval: !!item.requiresApproval,
      status: normalizeToolStatus(item.status, item.error),
      output: item.output,
      error: item.error,
    }))

    const timeline: AssistantReplyTimelineItem[] = []
    for (const item of toolCalls) {
      timeline.push({
        id: crypto.randomUUID(),
        kind: 'tool_start',
        toolCallId: item.toolCallId,
      })
      if (item.status !== 'pending' && item.status !== 'executing') {
        timeline.push({
          id: crypto.randomUUID(),
          kind: 'tool_result',
          toolCallId: item.toolCallId,
        })
      }
    }
    if (message.content.trim() !== '') {
      timeline.push({
        id: crypto.randomUUID(),
        kind: 'text',
        content: message.content,
      })
    }

    nextBatches.push({
      id: crypto.randomUUID(),
      sessionId: sessionId,
      assistantMessageId: message.id,
      toolCalls,
      timeline,
      collapsed: true,
    })
  }
  return nextBatches
}
