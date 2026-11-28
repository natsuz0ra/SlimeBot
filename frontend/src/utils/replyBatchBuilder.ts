import type { SessionHistoryPayload, SessionHistoryReplyTimingItem, SessionHistoryThinkingItem, SessionHistoryToolCallItem, ToolCallItem } from '@/api/chat'
import { isOpenToolStatus, parseChatTimestamp as parseTimestamp, projectChatHistory } from '../../../shared/chat-protocol/timeline.js'
import { createClientId } from './uuid'
import { buildAgentTeamsFromHistory, type AgentTeamRun } from './agentTeam'

export { normalizeToolStatus } from '../../../shared/chat-protocol/timeline.js'

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
      kind: 'notice'
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
  teamRuns: AgentTeamRun[]
  timeline: AssistantReplyTimelineItem[]
  collapsed: boolean
  startedAt?: number
  finishedAt?: number
  durationMs?: number
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

  const collapsed = timeline.filter((entry, index) => entry.kind === 'plan' || entry.kind === 'notice' || index === lastTextIndex)
  return collapsed.length > 0 ? collapsed : timeline
}

export function hasCollapsibleReplyContent(timeline: AssistantReplyTimelineItem[], toolCalls: ToolCallItem[]) {
  if (toolCalls.some((item) => item.preamble && item.preamble.trim() !== '')) return true
  const collapsed = getCollapsedReplyTimeline(timeline)
  if (collapsed.length !== timeline.length) return true
  return collapsed.some((entry, index) => entry.id !== timeline[index]?.id)
}

function adaptTimeline<T extends { toolCallId: string }>(
  timeline: ReturnType<typeof projectChatHistory<T>>['timeline'],
  batchId: string,
): AssistantReplyTimelineItem[] {
  return timeline.flatMap((entry): AssistantReplyTimelineItem[] => {
    const id = batchId + ':' + entry.id
    switch (entry.kind) {
      case 'thinking':
        return [{
          id, kind: 'thinking', content: entry.thinking.content, done: entry.thinking.done,
          durationMs: entry.thinking.durationMs, startedAt: entry.thinking.startedAt,
        }]
      case 'tool': {
        const { toolCallId, status } = entry.tool.record
        const start: AssistantReplyTimelineItem = { id: id + ':start', kind: 'tool_start', toolCallId }
        if (isOpenToolStatus(status)) return [start]
        return [start, { id: id + ':result', kind: 'tool_result', toolCallId }]
      }
      default:
        return [{ id, kind: entry.kind, content: entry.content }]
    }
  })
}

export function buildInterleavedTimeline(
  toolCalls: ToolCallItem[],
  content: string,
  thinkingRecords: SessionHistoryThinkingItem[] = [],
): AssistantReplyTimelineItem[] {
  return adaptTimeline(projectChatHistory(content, toolCalls, thinkingRecords).timeline, createClientId())
}

export function buildReplyBatchesFromHistory(sessionId: string, history: SessionHistoryPayload): AssistantReplyBatch[] {
  const nextBatches: AssistantReplyBatch[] = []
  for (const message of history.messages) {
    if (message.role !== 'assistant') continue
    const historyToolCalls = history.toolCallsByAssistantMessageId[message.id] || []
    const historyThinking = history.thinkingByAssistantMessageId?.[message.id] || []
    const projection = projectChatHistory(message.content, historyToolCalls, historyThinking, !!message.isInterrupted)
    const toolCalls: ToolCallItem[] = projection.toolCalls.map(({ record: item, subagentThinkings }) => ({
      toolCallId: item.toolCallId,
      toolName: item.toolName,
      command: item.command,
      params: item.params || {},
      requiresApproval: !!item.requiresApproval,
      status: item.status,
      output: item.output,
      error: item.error,
      metadata: item.metadata,
      startedAt: parseTimestamp(item.startedAt),
      finishedAt: parseTimestamp(item.finishedAt),
      parentToolCallId: item.parentToolCallId,
      subagentRunId: item.subagentRunId ?? subagentThinkings.find(thinking => thinking.subagentRunId)?.subagentRunId,
      teamRunId: item.teamRunId,
      memberRunId: item.memberRunId,
      subagentTitle: item.toolName === 'run_subagent' ? String(item.params?.title ?? '').trim() || undefined : undefined,
      subagentTask: item.toolName === 'run_subagent' ? String(item.params?.task ?? '').trim() || undefined : undefined,
      ...(subagentThinkings.length > 0 ? {
        subagentThinkings,
        subagentThinking: subagentThinkings[subagentThinkings.length - 1],
      } : {}),
    }))
    const timeline = adaptTimeline(projection.timeline, message.id)

    nextBatches.push({
      id: createClientId(),
      sessionId: sessionId,
      assistantMessageId: message.id,
      toolCalls,
      teamRuns: buildAgentTeamsFromHistory(history.teamRuns, history.teamMemberRuns, message.id),
      timeline,
      collapsed: true,
      ...deriveReplyTiming(message.createdAt, historyToolCalls, historyThinking, history.replyTimingByAssistantMessageId?.[message.id]),
    })
  }
  return nextBatches
}
