import type { AssistantReplyBatch, AssistantReplyTimelineItem } from './replyBatchBuilder'
import type { ToolCallStatus } from '@/types/chat'

type ThinkingTimelineItem = Extract<AssistantReplyTimelineItem, { kind: 'thinking' }>

function isOpenThinkingEntry(entry: AssistantReplyTimelineItem | undefined): entry is ThinkingTimelineItem {
  return !!entry && entry.kind === 'thinking' && !entry.done
}

function completeThinkingEntry(entry: ThinkingTimelineItem, finishedAt: number): ThinkingTimelineItem {
  return {
    ...entry,
    done: true,
    durationMs: entry.startedAt ? Math.max(0, finishedAt - entry.startedAt) : entry.durationMs,
  }
}

export function finishOpenThinkingEntries(batch: AssistantReplyBatch, finishedAt = Date.now()) {
  const entries = [...batch.timeline]
  let changed = false
  for (let i = 0; i < entries.length; i++) {
    const entry = entries[i]
    if (!isOpenThinkingEntry(entry)) continue
    entries[i] = completeThinkingEntry(entry, finishedAt)
    changed = true
  }
  if (changed) {
    batch.timeline = entries
  }
}

export function finalizeReplyBatchTiming(batch: AssistantReplyBatch, finishedAt = Date.now(), durationMs?: number) {
  batch.finishedAt = finishedAt
  batch.durationMs = typeof durationMs === 'number' && Number.isFinite(durationMs)
    ? Math.max(0, durationMs)
    : (typeof batch.startedAt === 'number' ? Math.max(0, finishedAt - batch.startedAt) : batch.durationMs)
  batch.collapsed = true
}

function findToolCall(batch: AssistantReplyBatch, toolCallId: string) {
  return batch.toolCalls.find((item) => item.toolCallId === toolCallId)
}

export function startSubagentThinking(batch: AssistantReplyBatch, parentToolCallId: string, startedAt = Date.now()) {
  const toolCall = findToolCall(batch, parentToolCallId)
  if (!toolCall) return
  const entries = toolCall.subagentThinkings ?? (toolCall.subagentThinking ? [toolCall.subagentThinking] : [])
  const nextEntry = {
    content: '',
    done: false,
    startedAt,
    durationMs: undefined,
  }
  toolCall.subagentThinkings = [...entries, nextEntry]
  toolCall.subagentThinking = nextEntry
}

export function appendSubagentThinkingChunk(batch: AssistantReplyBatch, parentToolCallId: string, chunk: string, startedAt = Date.now()) {
  if (chunk === '') return
  const toolCall = findToolCall(batch, parentToolCallId)
  if (!toolCall) return
  const entries = toolCall.subagentThinkings ?? (toolCall.subagentThinking ? [toolCall.subagentThinking] : [])
  let targetIndex = -1
  for (let i = entries.length - 1; i >= 0; i--) {
    if (!entries[i]!.done) {
      targetIndex = i
      break
    }
  }
  if (targetIndex === -1) {
    entries.push({ content: '', done: false, startedAt })
    targetIndex = entries.length - 1
  }
  const target = entries[targetIndex]!
  entries[targetIndex] = {
    ...target,
    content: target.content + chunk,
    done: false,
  }
  toolCall.subagentThinkings = [...entries]
  toolCall.subagentThinking = toolCall.subagentThinkings[toolCall.subagentThinkings.length - 1]
}

export function finishSubagentThinking(batch: AssistantReplyBatch, parentToolCallId: string, finishedAt = Date.now()) {
  const toolCall = findToolCall(batch, parentToolCallId)
  if (!toolCall) return
  const entries = toolCall.subagentThinkings ?? (toolCall.subagentThinking ? [toolCall.subagentThinking] : [])
  let targetIndex = -1
  for (let i = entries.length - 1; i >= 0; i--) {
    if (!entries[i]!.done) {
      targetIndex = i
      break
    }
  }
  if (targetIndex === -1) return
  const target = entries[targetIndex]!
  const startedAt = target.startedAt
  entries[targetIndex] = {
    ...target,
    done: true,
    durationMs: startedAt ? Math.max(0, finishedAt - startedAt) : target.durationMs,
  }
  toolCall.subagentThinkings = [...entries]
  toolCall.subagentThinking = toolCall.subagentThinkings[toolCall.subagentThinkings.length - 1]
}

export function finishAllSubagentThinkings(batch: AssistantReplyBatch, finishedAt = Date.now()) {
  for (const toolCall of batch.toolCalls) {
    const entries = toolCall.subagentThinkings ?? (toolCall.subagentThinking ? [toolCall.subagentThinking] : [])
    if (entries.length === 0) continue
    let changed = false
    const nextEntries = entries.map((entry) => {
      if (entry.done) return entry
      changed = true
      return {
        ...entry,
        done: true,
        finishedAt,
        durationMs: entry.startedAt ? Math.max(0, finishedAt - entry.startedAt) : entry.durationMs,
      }
    })
    if (!changed) continue
    toolCall.subagentThinkings = nextEntries
    toolCall.subagentThinking = nextEntries[nextEntries.length - 1]
  }
}

function isOpenToolStatus(status: ToolCallStatus | undefined) {
  return status === 'pending' || status === 'executing'
}

export function markOpenToolCallsError(batch: AssistantReplyBatch, error = 'Execution cancelled.', finishedAt = Date.now()) {
  const existingResultIds = new Set(
    batch.timeline
      .filter((entry) => entry.kind === 'tool_result')
      .map((entry) => entry.toolCallId),
  )
  for (const toolCall of batch.toolCalls) {
    if (!isOpenToolStatus(toolCall.status)) continue
    toolCall.status = 'error'
    toolCall.error = toolCall.error || error
    toolCall.finishedAt = toolCall.finishedAt || finishedAt
    if (!toolCall.parentToolCallId && !existingResultIds.has(toolCall.toolCallId)) {
      batch.timeline.push({
        id: crypto.randomUUID(),
        kind: 'tool_result',
        toolCallId: toolCall.toolCallId,
      })
      existingResultIds.add(toolCall.toolCallId)
    }
  }
}

export function markToolCallError(batch: AssistantReplyBatch, toolCallId: string, error = 'Execution cancelled.', finishedAt = Date.now()) {
  const toolCall = batch.toolCalls.find((item) => item.toolCallId === toolCallId)
  if (!toolCall || !isOpenToolStatus(toolCall.status)) return
  toolCall.status = 'error'
  toolCall.error = toolCall.error || error
  toolCall.finishedAt = toolCall.finishedAt || finishedAt
  const hasResult = batch.timeline.some((entry) => entry.kind === 'tool_result' && entry.toolCallId === toolCallId)
  if (!toolCall.parentToolCallId && !hasResult) {
    batch.timeline.push({
      id: crypto.randomUUID(),
      kind: 'tool_result',
      toolCallId,
    })
  }
}

export function finalizeOpenReplyRuntimeState(batch: AssistantReplyBatch, error = 'Execution cancelled.', finishedAt = Date.now()) {
  finishOpenThinkingEntries(batch, finishedAt)
  finishAllSubagentThinkings(batch, finishedAt)
  markOpenToolCallsError(batch, error, finishedAt)
}

export function appendTextChunkToBatch(batch: AssistantReplyBatch, chunk: string) {
  if (chunk === '') return
  finishOpenThinkingEntries(batch)
  const lastTimeline = batch.timeline[batch.timeline.length - 1]
  if (lastTimeline && lastTimeline.kind === 'text') {
    lastTimeline.content += chunk
    return
  }
  batch.timeline.push({
    id: crypto.randomUUID(),
    kind: 'text',
    content: chunk,
  })
}

export function appendPlanBodyToBatch(batch: AssistantReplyBatch, planBody: string) {
  if (planBody.trim() === '') return
  finishOpenThinkingEntries(batch)
  const lastEntry = batch.timeline[batch.timeline.length - 1]
  if (lastEntry && lastEntry.kind === 'plan' && lastEntry.generating) {
    lastEntry.content = planBody
    lastEntry.generating = false
    return
  }
  batch.timeline.push({
    id: crypto.randomUUID(),
    kind: 'plan',
    content: planBody,
  })
}

export function appendPlanChunkToBatch(batch: AssistantReplyBatch, chunk: string) {
  if (chunk === '') return
  finishOpenThinkingEntries(batch)
  const lastEntry = batch.timeline[batch.timeline.length - 1]
  if (lastEntry && lastEntry.kind === 'plan' && lastEntry.generating) {
    lastEntry.content += chunk
    return
  }
  batch.timeline.push({
    id: crypto.randomUUID(),
    kind: 'plan',
    content: chunk,
    generating: true,
  })
}

export function markLastThinkingDone(timeline: AssistantReplyTimelineItem[], finishedAt = Date.now()) {
  const entries = [...timeline]
  for (let i = entries.length - 1; i >= 0; i--) {
    const entry = entries[i]
    if (isOpenThinkingEntry(entry)) {
      entries[i] = completeThinkingEntry(entry, finishedAt)
      return entries
    }
  }
  return timeline
}

export function getLiveReplyContentSignature(batch: AssistantReplyBatch | undefined) {
  if (!batch) return ''
  const parts = batch.timeline.map((entry) => {
    if (entry.kind === 'text') return `text:${entry.content.length}`
    if (entry.kind === 'plan') return `plan:${entry.content.length}:${entry.generating ? 1 : 0}`
    if (entry.kind === 'thinking') return `thinking:${entry.content.length}:${entry.done ? 1 : 0}`
    return `${entry.kind}:${entry.toolCallId}`
  })

  for (const item of batch.toolCalls) {
    parts.push([
      'tool',
      item.toolCallId,
      item.status,
      item.output?.length ?? 0,
      item.error?.length ?? 0,
      item.subagentStream?.length ?? 0,
      item.subagentThinkings?.map((thinking) => `${thinking.content.length}:${thinking.done ? 1 : 0}`).join(',') ?? '',
      item.subagentThinking?.content.length ?? 0,
      item.subagentThinking?.done ? 1 : 0,
    ].join(':'))
  }

  return parts.join('|')
}
