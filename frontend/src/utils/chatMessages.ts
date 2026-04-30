import type { MessageItem } from '../api/chat'

export function materializeStoppedMessage(item: MessageItem, stoppedText: string): MessageItem {
  if (item.isStopPlaceholder && (!item.content || item.content.trim() === '')) {
    return { ...item, content: stoppedText }
  }
  return item
}

export function materializeStoppedMessages(items: MessageItem[], stoppedText: string): MessageItem[] {
  return items.map((item) => materializeStoppedMessage(item, stoppedText))
}
