import type { MessageItem } from '@/api/chat'
import type { AssistantReplyBatch } from '@/utils/replyBatchBuilder'

export function findLatestEditableUserMessageId(messages: MessageItem[], waiting: boolean, failedMessageIds: Set<string>) {
  if (waiting) return ''
  for (let i = messages.length - 1; i >= 0; i--) {
    const item = messages[i]!
    if (item.role !== 'user') continue
    if (failedMessageIds.has(item.id)) return ''
    if (typeof item.seq !== 'number') return ''
    return item.id
  }
  return ''
}

export function applyEditedUserMessage(
  messages: MessageItem[],
  replyBatches: AssistantReplyBatch[],
  messageId: string,
  content: string,
  createdAt?: string,
) {
  const targetIndex = messages.findIndex((item) => item.id === messageId)
  if (targetIndex < 0) {
    return { messages, replyBatches }
  }
  const keptMessages = messages.slice(0, targetIndex + 1)
  const target = keptMessages[targetIndex]!
  keptMessages[targetIndex] = { ...target, content, createdAt: createdAt || target.createdAt }
  const keptMessageIds = new Set(keptMessages.map((item) => item.id))
  return {
    messages: keptMessages,
    replyBatches: replyBatches.filter((batch) => keptMessageIds.has(batch.assistantMessageId)),
  }
}
