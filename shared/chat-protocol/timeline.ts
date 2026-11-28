import { isToolCallStatus, type ToolCallStatus } from './events.js'

export type ContentSegment =
  | { type: 'text'; content: string }
  | { type: 'tool_call_marker'; content: ''; toolCallId: string }
  | { type: 'thinking_marker'; content: ''; thinkingId: string }
  | { type: 'plan_start' | 'plan_end'; content: '' }

const markerPattern = /\n?<!-- (TOOL_CALL:(.+?)|THINKING:(.+?)|PLAN_START|PLAN_END) -->\n?/g

export function hasContentMarkers(content: string): boolean {
  return new RegExp(markerPattern.source).test(content)
}

export function stripContentMarkers(content: string): string {
  return content.replace(markerPattern, '')
}

export function parseContentMarkers(content: string): ContentSegment[] {
  const segments: ContentSegment[] = []
  let previousEnd = 0

  for (const match of content.matchAll(markerPattern)) {
    const text = content.slice(previousEnd, match.index)
    if (text.trim()) segments.push({ type: 'text', content: text })
    if (match[2]) {
      segments.push({ type: 'tool_call_marker', content: '', toolCallId: match[2] })
    } else if (match[3]) {
      segments.push({ type: 'thinking_marker', content: '', thinkingId: match[3] })
    } else {
      segments.push({ type: match[1] === 'PLAN_START' ? 'plan_start' : 'plan_end', content: '' })
    }
    previousEnd = match.index + match[0].length
  }
  const remaining = content.slice(previousEnd)
  if (remaining.trim()) segments.push({ type: 'text', content: remaining })
  return segments
}

export function parseChatTimestamp(value?: string | number): number | undefined {
  const timestamp = typeof value === 'number' ? value : Date.parse(value ?? '')
  return Number.isFinite(timestamp) ? timestamp : undefined
}

export function normalizeToolStatus(status?: string, error?: string): ToolCallStatus {
  return isToolCallStatus(status) ? status : error ? 'error' : 'completed'
}

export function isOpenToolStatus(status: ToolCallStatus): boolean {
  return status === 'pending' || status === 'reviewing' || status === 'executing'
}

interface HistoryTool {
  toolCallId: string
  parentToolCallId?: string
  startedAt?: string | number
  status?: string
  error?: string
}

interface HistoryThinking {
  thinkingId: string
  content: string
  status: string
  parentToolCallId?: string
  subagentRunId?: string
  startedAt?: string
  finishedAt?: string
  durationMs?: number
}

export type ProjectedThinking = {
  content: string
  subagentRunId?: string
  done: boolean
  startedAt?: number
  finishedAt?: number
  durationMs?: number
}

export type ProjectedTool<T extends HistoryTool> = {
  record: Omit<T, 'status'> & { status: ToolCallStatus }
  subagentThinkings: ProjectedThinking[]
}

type ProjectedEntry<T extends HistoryTool> =
  | { kind: 'text'; id: string; content: string }
  | { kind: 'plan'; id: string; content: string }
  | { kind: 'thinking'; id: string; thinking: ProjectedThinking }
  | { kind: 'tool'; id: string; tool: ProjectedTool<T> }

function compareStart(left: { startedAt?: string | number }, right: { startedAt?: string | number }): number {
  return (parseChatTimestamp(left.startedAt) ?? 0) - (parseChatTimestamp(right.startedAt) ?? 0)
}

function projectThinking(record: HistoryThinking, interrupted: boolean): ProjectedThinking {
  return {
    content: record.content || '',
    ...(record.subagentRunId ? { subagentRunId: record.subagentRunId } : {}),
    done: interrupted || record.status !== 'streaming',
    startedAt: parseChatTimestamp(record.startedAt),
    finishedAt: parseChatTimestamp(record.finishedAt),
    durationMs: record.durationMs,
  }
}

export function projectChatHistory<T extends HistoryTool>(
  content: string,
  tools: readonly T[],
  thinkingRecords: readonly HistoryThinking[] = [],
  interrupted = false,
): { timeline: ProjectedEntry<T>[]; toolCalls: ProjectedTool<T>[] } {
  const topThinking = new Map<string, ProjectedThinking>()
  const nestedThinking = new Map<string, ProjectedThinking[]>()
  for (const record of [...thinkingRecords].sort(compareStart)) {
    const thinking = projectThinking(record, interrupted)
    if (record.parentToolCallId) {
      const group = nestedThinking.get(record.parentToolCallId) ?? []
      group.push(thinking)
      nestedThinking.set(record.parentToolCallId, group)
    } else if (!record.subagentRunId) {
      topThinking.set(record.thinkingId, thinking)
    }
  }

  const toolCalls = [...tools].sort(compareStart).map((record): ProjectedTool<T> => {
    const status = normalizeToolStatus(record.status, record.error)
    const cancelled = interrupted && isOpenToolStatus(status)
    return {
      record: { ...record, status: cancelled ? 'error' : status, ...(cancelled ? { error: record.error || 'Execution cancelled.' } : {}) },
      subagentThinkings: nestedThinking.get(record.toolCallId) ?? [],
    }
  })
  const topTools = new Map(toolCalls
    .filter(({ record }) => !record.parentToolCallId)
    .map(tool => [tool.record.toolCallId, tool]))

  const segments = parseContentMarkers(content)
  const hasMarkers = segments.some(segment => segment.type !== 'text')
  const timeline: ProjectedEntry<T>[] = []
  const marked: ProjectedEntry<T>[] = []
  const planParts: string[] = []
  let inPlan = false
  let textIndex = 0
  let planIndex = 0

  const flushPlan = () => {
    if (planParts.length === 0) return
    marked.push({ kind: 'plan', id: 'plan:' + planIndex++, content: planParts.join('\n') })
    planParts.length = 0
  }

  for (const segment of segments) {
    switch (segment.type) {
      case 'plan_start':
      case 'plan_end':
        flushPlan()
        inPlan = segment.type === 'plan_start'
        break
      case 'text':
        if (inPlan) planParts.push(segment.content)
        else marked.push({ kind: 'text', id: 'text:' + textIndex++, content: segment.content })
        break
      case 'thinking_marker': {
        const thinking = topThinking.get(segment.thinkingId)
        if (thinking) {
          flushPlan()
          marked.push({ kind: 'thinking', id: 'thinking:' + segment.thinkingId, thinking })
          topThinking.delete(segment.thinkingId)
        }
        break
      }
      case 'tool_call_marker': {
        const tool = topTools.get(segment.toolCallId)
        if (tool) {
          flushPlan()
          marked.push({ kind: 'tool', id: 'tool:' + segment.toolCallId, tool })
          topTools.delete(segment.toolCallId)
        }
        break
      }
    }
  }
  flushPlan()

  // Unmarked thinking precedes the response. Legacy replies put all tools
  // before the text, while interleaved replies append tools missing a marker.
  for (const [id, thinking] of topThinking) {
    timeline.push({ kind: 'thinking', id: 'thinking:' + id, thinking })
  }
  const remainingTools = [...topTools].map(([id, tool]): ProjectedEntry<T> => ({ kind: 'tool', id: 'tool:' + id, tool }))
  if (hasMarkers) timeline.push(...marked, ...remainingTools)
  else timeline.push(...remainingTools, ...marked)
  return { timeline, toolCalls }
}
