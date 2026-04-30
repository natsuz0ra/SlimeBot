import { inject, provide, type ComputedRef, type InjectionKey } from 'vue'
import type { ToolCallItem } from '@/api/chat'
import type { ReplyTimelineEntry } from '@/types/chat'

export interface ChatMessageContext {
  waiting: ComputedRef<boolean>
  planGenerating: ComputedRef<boolean>
  isStreamingMessage: (messageId: string) => boolean
  getReplyToolCount: (messageId: string) => number
  getReplyToolSummary: (messageId: string) => string
  getReplyTimeline: (messageId: string) => ReplyTimelineEntry[]
  getVisibleReplyTimeline: (messageId: string) => ReplyTimelineEntry[]
  getReplyToolItem: (messageId: string, toolCallId: string) => ToolCallItem | undefined
  getSubagentChildTools: (messageId: string, parentToolCallId: string) => ToolCallItem[]
  shouldShowInlineToolCall: (messageId: string, toolCallId: string) => boolean
  isReplyToolCollapsed: (messageId: string) => boolean
  toggleReplyCollapsed: (messageId: string) => void
  getReplyElapsedMs: (messageId: string) => number | undefined
  shouldShowReplyCollapseBar: (messageId: string) => boolean
  isEmptyPlaceholder: (messageId: string) => boolean
  openToolDetail: (messageId: string) => void
  approveToolCall: (toolCallId: string, approved: boolean) => void
  approveAllPendingToolCalls: () => void
  rejectAllPendingToolCalls: () => void
  isFailedUserMessage: (messageId: string) => boolean
  isAssistantErrorMessage: (messageId: string) => boolean
  isChatAssistantAvatarAnimated: (messageId: string) => boolean
  sendBlockedOfflineText: ComputedRef<string>
  toolExecutionDetailTitle: ComputedRef<string>
}

export const CHAT_CONTEXT_KEY: InjectionKey<ChatMessageContext> = Symbol('ChatMessageContext')

export function provideChatContext(ctx: ChatMessageContext) {
  provide(CHAT_CONTEXT_KEY, ctx)
}

export function useChatContext() {
  const ctx = inject(CHAT_CONTEXT_KEY)
  if (!ctx) {
    throw new Error('useChatContext() must be used within provideChatContext()')
  }
  return ctx
}
