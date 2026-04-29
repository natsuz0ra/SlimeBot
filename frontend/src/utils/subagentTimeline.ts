import type { SubagentThinkingItem, ToolCallItem } from '@/api/chat'

export type SubagentTimelineItem =
  | {
      kind: 'thinking'
      id: string
      startedAt?: number
      thinking: SubagentThinkingItem
    }
  | {
      kind: 'tool'
      id: string
      startedAt?: number
      tool: ToolCallItem
    }

type SortableSubagentTimelineItem = SubagentTimelineItem & { order: number }

function timestampRank(startedAt: number | undefined) {
  return typeof startedAt === 'number' && Number.isFinite(startedAt)
    ? startedAt
    : Number.POSITIVE_INFINITY
}

export function buildSubagentTimeline(
  thinkings: SubagentThinkingItem[],
  tools: ToolCallItem[],
): SubagentTimelineItem[] {
  const timeline: SortableSubagentTimelineItem[] = []
  let order = 0

  for (const thinking of thinkings) {
    timeline.push({
      kind: 'thinking',
      id: `thinking-${thinking.startedAt ?? order}-${order}`,
      startedAt: thinking.startedAt,
      thinking,
      order,
    })
    order += 1
  }

  for (const tool of tools) {
    timeline.push({
      kind: 'tool',
      id: `tool-${tool.toolCallId}`,
      startedAt: tool.startedAt,
      tool,
      order,
    })
    order += 1
  }

  return timeline
    .sort((left, right) => {
      const timeDiff = timestampRank(left.startedAt) - timestampRank(right.startedAt)
      if (timeDiff !== 0) return timeDiff
      return left.order - right.order
    })
    .map(({ order: _order, ...item }) => item)
}
