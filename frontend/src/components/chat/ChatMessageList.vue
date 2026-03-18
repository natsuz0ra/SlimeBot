<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { MessageItem, ToolCallItem } from '@/api/chat'
import ChatMessageItem from '@/components/chat/ChatMessageItem.vue'

type ReplyTimelineEntry =
  | { id: string; kind: 'text'; content: string }
  | { id: string; kind: 'tool_start'; toolCallId: string }
  | { id: string; kind: 'tool_result'; toolCallId: string }

const props = defineProps<{
  messages: MessageItem[]
  waiting: boolean
  isMessagePlatformSession: boolean
  showScrollToBottom: boolean
  hasMoreHistory: boolean
  loadingOlderHistory: boolean
  setMessagesRef: (el: unknown) => void
  isFailedUserMessage: (messageId: string) => boolean
  isAssistantErrorMessage: (messageId: string) => boolean
  isEmptyPlaceholder: (messageId: string) => boolean
  isChatAssistantAvatarAnimated: (messageId: string) => boolean
  getReplyToolCount: (messageId: string) => number
  getReplyToolSummary: (messageId: string) => string
  getReplyTimeline: (messageId: string) => ReplyTimelineEntry[]
  getReplyToolItem: (messageId: string, toolCallId: string) => ToolCallItem | undefined
  shouldShowInlineToolCall: (messageId: string, toolCallId: string) => boolean
  isReplyToolCollapsed: (messageId: string) => boolean
  openToolDetail: (messageId: string) => void
  approveToolCall: (toolCallId: string, approved: boolean) => void
}>()

const emit = defineEmits<{
  scrollToBottom: []
}>()

const { t } = useI18n()
</script>

<template>
  <section
    :ref="setMessagesRef"
    class="messages-section scroll-area flex-1 overflow-y-auto px-4 py-6"
  >
    <div class="flex flex-col gap-5 max-w-[720px] mx-auto">
      <ChatMessageItem
        v-for="item in messages"
        :key="item.id"
        :item="item"
        :waiting="waiting"
        :is-failed-user-message="isFailedUserMessage"
        :is-assistant-error-message="isAssistantErrorMessage"
        :is-empty-placeholder="isEmptyPlaceholder"
        :is-chat-assistant-avatar-animated="isChatAssistantAvatarAnimated"
        :get-reply-tool-count="getReplyToolCount"
        :get-reply-tool-summary="getReplyToolSummary"
        :get-reply-timeline="getReplyTimeline"
        :get-reply-tool-item="getReplyToolItem"
        :should-show-inline-tool-call="shouldShowInlineToolCall"
        :is-reply-tool-collapsed="isReplyToolCollapsed"
        :open-tool-detail="openToolDetail"
        :approve-tool-call="approveToolCall"
        :send-blocked-offline-text="t('sendBlockedOffline')"
        :tool-execution-detail-title="t('toolExecutionDetailTitle')"
      />
    </div>
  </section>

  <Transition name="scroll-bottom-fade">
    <div
      v-if="showScrollToBottom"
      class="pointer-events-none absolute right-6 bottom-[132px] z-20"
    >
      <button
        type="button"
        class="scroll-bottom-btn pointer-events-auto w-10 h-10 rounded-full inline-flex items-center justify-center cursor-pointer"
        aria-label="Scroll to bottom"
        @click="emit('scrollToBottom')"
      >
        <span class="scroll-bottom-arrow" aria-hidden="true">↓</span>
      </button>
    </div>
  </Transition>
</template>
