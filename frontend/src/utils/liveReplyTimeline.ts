import type { AssistantReplyBatch, AssistantReplyTimelineItem } from './replyBatchBuilder'

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
  batch.timeline.push({
    id: crypto.randomUUID(),
    kind: 'plan',
    content: planBody,
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
