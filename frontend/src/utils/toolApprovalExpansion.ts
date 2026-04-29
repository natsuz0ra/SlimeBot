import type { ToolCallItem } from '@/api/chat'

export function shouldAutoExpandToolCall(item: ToolCallItem, nestedTools: ToolCallItem[]): boolean {
  if (item.status === 'pending') return true
  return nestedTools.some((tool) => tool.status === 'pending')
}
