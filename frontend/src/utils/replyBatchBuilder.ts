import type { SessionHistoryPayload, SessionHistoryToolCallItem, ToolCallItem } from '@/api/chat'
import type { ToolCallStatus } from '@/types/chat'
import { hasContentMarkers, parseContentMarkers } from './contentMarkers'

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
  | { id: string; kind: 'thinking'; content: string; done: boolean; durationMs?: number; startedAt?: number }

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

function buildLegacyTimeline(toolCalls: ToolCallItem[], content: string): AssistantReplyTimelineItem[] {
  const timeline: AssistantReplyTimelineItem[] = []
  for (const item of toolCalls) {
    if (item.parentToolCallId) continue
    timeline.push({ id: crypto.randomUUID(), kind: 'tool_start', toolCallId: item.toolCallId })
    if (item.status !== 'pending' && item.status !== 'executing') {
      timeline.push({ id: crypto.randomUUID(), kind: 'tool_result', toolCallId: item.toolCallId })
    }
  }
  if (content.trim() !== '') {
    timeline.push({ id: crypto.randomUUID(), kind: 'text', content })
  }
  return timeline
}

function buildInterleavedTimeline(toolCalls: ToolCallItem[], content: string): AssistantReplyTimelineItem[] {
  const toolCallMap = new Map(toolCalls.map(tc => [tc.toolCallId, tc]))
  const segments = parseContentMarkers(content)
  const timeline: AssistantReplyTimelineItem[] = []

  for (const seg of segments) {
    if (seg.type === 'text') {
      timeline.push({ id: crypto.randomUUID(), kind: 'text', content: seg.content })
    } else if (seg.type === 'tool_call_marker' && seg.toolCallId) {
      const tc = toolCallMap.get(seg.toolCallId)
      if (tc && !tc.parentToolCallId) {
        timeline.push({ id: crypto.randomUUID(), kind: 'tool_start', toolCallId: tc.toolCallId })
        if (tc.status !== 'pending' && tc.status !== 'executing') {
          timeline.push({ id: crypto.randomUUID(), kind: 'tool_result', toolCallId: tc.toolCallId })
        }
      }
    }
  }

  // Fallback: append tool calls whose markers are missing
  const markerIds = new Set(segments.filter(s => s.toolCallId).map(s => s.toolCallId))
  for (const tc of toolCalls) {
    if (!tc.parentToolCallId && !markerIds.has(tc.toolCallId)) {
      timeline.push({ id: crypto.randomUUID(), kind: 'tool_start', toolCallId: tc.toolCallId })
      if (tc.status !== 'pending' && tc.status !== 'executing') {
        timeline.push({ id: crypto.randomUUID(), kind: 'tool_result', toolCallId: tc.toolCallId })
      }
    }
  }

  return timeline
}

export function buildReplyBatchesFromHistory(sessionId: string, history: SessionHistoryPayload): AssistantReplyBatch[] {
  const nextBatches: AssistantReplyBatch[] = []
  for (const message of history.messages) {
    if (message.role !== 'assistant') continue
    const historyToolCalls = history.toolCallsByAssistantMessageId[message.id] || []

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
      parentToolCallId: item.parentToolCallId,
      subagentRunId: item.subagentRunId,
    }))

    const timeline = hasContentMarkers(message.content)
      ? buildInterleavedTimeline(toolCalls, message.content)
      : buildLegacyTimeline(toolCalls, message.content)

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
