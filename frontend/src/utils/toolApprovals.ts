import type { ToolCallItem } from '@/api/chat'

export function isBatchApprovableTool(item: ToolCallItem) {
  return item.status === 'pending' && item.toolName !== 'ask_questions'
}

export function getBatchApprovalToolCallIds(items: ToolCallItem[]) {
  return items.filter(isBatchApprovableTool).map((item) => item.toolCallId)
}

export function markToolApprovalDecision(items: ToolCallItem[], toolCallId: string, approved: boolean) {
  const item = items.find((tool) => tool.toolCallId === toolCallId)
  if (!item || item.status !== 'pending') return false
  item.status = approved ? 'executing' : 'rejected'
  return true
}
