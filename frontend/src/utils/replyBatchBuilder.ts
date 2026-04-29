import type { SessionHistoryPayload, SessionHistoryReplyTimingItem, SessionHistoryThinkingItem, SessionHistoryToolCallItem, ToolCallItem } from '@/api/chat'
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
  | {
      id: string
      kind: 'plan'
      content: string
      generating?: boolean
    }
  | { id: string; kind: 'thinking'; content: string; done: boolean; durationMs?: number; startedAt?: number }

export interface AssistantReplyBatch {
  id: string
  sessionId: string
  assistantMessageId: string
  toolCalls: ToolCallItem[]
  timeline: AssistantReplyTimelineItem[]
  collapsed: boolean
  startedAt?: number
  finishedAt?: number
  durationMs?: number
}

function parseTimestamp(value?: string) {
  if (!value) return undefined
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? parsed : undefined
}

function finishFromDuration(startedAt: number | undefined, durationMs: number | undefined) {
  if (typeof startedAt !== 'number' || typeof durationMs !== 'number') return undefined
  return startedAt + Math.max(0, durationMs)
}

function deriveReplyTiming(
  messageCreatedAt: string | undefined,
  toolCalls: SessionHistoryToolCallItem[],
  thinkingRecords: SessionHistoryThinkingItem[],
  replyTiming?: SessionHistoryReplyTimingItem,
) {
  const replyStartedAt = parseTimestamp(replyTiming?.startedAt)
  const replyFinishedAt = parseTimestamp(replyTiming?.finishedAt)
  if (typeof replyStartedAt === 'number' && typeof replyFinishedAt === 'number') {
    return {
      startedAt: replyStartedAt,
      finishedAt: replyFinishedAt,
      durationMs: typeof replyTiming?.durationMs === 'number'
        ? Math.max(0, replyTiming.durationMs)
        : Math.max(0, replyFinishedAt - replyStartedAt),
    }
  }

  const starts: number[] = []
  const finishes: number[] = []

  const messageStartedAt = parseTimestamp(messageCreatedAt)
  if (typeof messageStartedAt === 'number') starts.push(messageStartedAt)

  for (const item of toolCalls) {
    const startedAt = parseTimestamp(item.startedAt)
    const finishedAt = parseTimestamp(item.finishedAt)
    if (typeof startedAt === 'number') starts.push(startedAt)
    if (typeof finishedAt === 'number') finishes.push(finishedAt)
  }

  for (const item of thinkingRecords) {
    const startedAt = parseTimestamp(item.startedAt)
    const finishedAt = parseTimestamp(item.finishedAt) ?? finishFromDuration(startedAt, item.durationMs)
    if (typeof startedAt === 'number') starts.push(startedAt)
    if (typeof finishedAt === 'number') finishes.push(finishedAt)
  }

  const startedAt = starts.length > 0 ? Math.min(...starts) : undefined
  const finishedAt = finishes.length > 0 ? Math.max(...finishes) : undefined
  const durationMs =
    typeof startedAt === 'number' && typeof finishedAt === 'number'
      ? Math.max(0, finishedAt - startedAt)
      : undefined

  return { startedAt, finishedAt, durationMs }
}

export function getCollapsedReplyTimeline(timeline: AssistantReplyTimelineItem[]) {
  let lastTextIndex = -1
  for (let i = timeline.length - 1; i >= 0; i--) {
    const entry = timeline[i]
    if (entry?.kind === 'text' && entry.content.trim() !== '') {
      lastTextIndex = i
      break
    }
  }

  return timeline.filter((entry, index) => entry.kind === 'plan' || index === lastTextIndex)
}

export function hasCollapsibleReplyContent(timeline: AssistantReplyTimelineItem[], toolCalls: ToolCallItem[]) {
  if (toolCalls.some((item) => item.preamble && item.preamble.trim() !== '')) return true
  const collapsed = getCollapsedReplyTimeline(timeline)
  if (collapsed.length !== timeline.length) return true
  return collapsed.some((entry, index) => entry.id !== timeline[index]?.id)
}

export function normalizeToolStatus(status?: string, fallbackError?: string): ToolCallStatus {
  if (status === 'pending' || status === 'rejected' || status === 'executing' || status === 'completed' || status === 'error') {
    return status
  }
  return fallbackError ? 'error' : 'completed'
}

export function buildLegacyTimeline(toolCalls: ToolCallItem[], content: string, thinkingRecords: SessionHistoryThinkingItem[] = []): AssistantReplyTimelineItem[] {
  const timeline: AssistantReplyTimelineItem[] = []
  for (const item of thinkingRecords) {
    if (item.parentToolCallId || item.subagentRunId) continue
    timeline.push({
      id: crypto.randomUUID(),
      kind: 'thinking',
      content: item.content || '',
      done: item.status !== 'streaming',
      durationMs: item.durationMs,
    })
  }
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

export function buildInterleavedTimeline(
  toolCalls: ToolCallItem[],
  content: string,
  thinkingRecords: SessionHistoryThinkingItem[] = [],
): AssistantReplyTimelineItem[] {
  const toolCallMap = new Map(toolCalls.map(tc => [tc.toolCallId, tc]))
  const thinkingMap = new Map(thinkingRecords.filter(item => !item.parentToolCallId && !item.subagentRunId).map(item => [item.thinkingId, item]))
  const segments = parseContentMarkers(content)
  const timeline: AssistantReplyTimelineItem[] = []
  const planParts: string[] = []
  let inPlan = false

  const pushPlan = () => {
    const planContent = planParts.join('')
    if (planContent.trim() !== '') {
      timeline.push({ id: crypto.randomUUID(), kind: 'plan', content: planContent })
    }
    planParts.length = 0
  }

  for (const seg of segments) {
    if (seg.type === 'plan_start') {
      if (inPlan) pushPlan()
      inPlan = true
      continue
    }
    if (seg.type === 'plan_end') {
      if (inPlan) pushPlan()
      inPlan = false
      continue
    }
    if (inPlan) {
      if (seg.type === 'text') planParts.push(seg.content)
      continue
    }
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
    } else if (seg.type === 'thinking_marker' && seg.thinkingId) {
      const thinking = thinkingMap.get(seg.thinkingId)
      if (thinking) {
        timeline.push({
          id: crypto.randomUUID(),
          kind: 'thinking',
          content: thinking.content || '',
          done: thinking.status !== 'streaming',
          durationMs: thinking.durationMs,
        })
      }
    }
  }
  if (inPlan) pushPlan()

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
  const thinkingMarkerIds = new Set(segments.filter(s => s.thinkingId).map(s => s.thinkingId))
  for (const thinking of thinkingRecords) {
    if (thinking.parentToolCallId || thinking.subagentRunId) continue
    if (!thinkingMarkerIds.has(thinking.thinkingId)) {
      timeline.unshift({
        id: crypto.randomUUID(),
        kind: 'thinking',
        content: thinking.content || '',
        done: thinking.status !== 'streaming',
        durationMs: thinking.durationMs,
      })
    }
  }

  return timeline
}

export function buildReplyBatchesFromHistory(sessionId: string, history: SessionHistoryPayload): AssistantReplyBatch[] {
  const nextBatches: AssistantReplyBatch[] = []
  for (const message of history.messages) {
    if (message.role !== 'assistant') continue
    const historyToolCalls = history.toolCallsByAssistantMessageId[message.id] || []
    const historyThinking = history.thinkingByAssistantMessageId?.[message.id] || []

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
      startedAt: parseTimestamp(item.startedAt),
      finishedAt: parseTimestamp(item.finishedAt),
      parentToolCallId: item.parentToolCallId,
      subagentRunId: item.subagentRunId,
    }))

    const subagentThinkingRecords = historyThinking
      .filter((thinking) => thinking.parentToolCallId || thinking.subagentRunId)
      .sort((left, right) => {
        const leftAt = new Date(left.startedAt || 0).getTime()
        const rightAt = new Date(right.startedAt || 0).getTime()
        return leftAt - rightAt
      })

    for (const thinking of subagentThinkingRecords) {
      if (!thinking.parentToolCallId && !thinking.subagentRunId) continue
      const parent = toolCalls.find((item) => item.toolCallId === thinking.parentToolCallId)
      if (!parent) continue
      const entry = {
        content: thinking.content || '',
        done: thinking.status !== 'streaming',
        durationMs: thinking.durationMs,
        startedAt: parseTimestamp(thinking.startedAt),
        finishedAt: parseTimestamp(thinking.finishedAt),
      }
      parent.subagentThinkings = [...(parent.subagentThinkings ?? (parent.subagentThinking ? [parent.subagentThinking] : [])), entry]
      parent.subagentThinking = entry
      if (thinking.subagentRunId && !parent.subagentRunId) {
        parent.subagentRunId = thinking.subagentRunId
      }
    }

    const timeline = hasContentMarkers(message.content)
      ? buildInterleavedTimeline(toolCalls, message.content, historyThinking)
      : buildLegacyTimeline(toolCalls, message.content, historyThinking)

    nextBatches.push({
      id: crypto.randomUUID(),
      sessionId: sessionId,
      assistantMessageId: message.id,
      toolCalls,
      timeline,
      collapsed: true,
      ...deriveReplyTiming(message.createdAt, historyToolCalls, historyThinking, history.replyTimingByAssistantMessageId?.[message.id]),
    })
  }
  return nextBatches
}
