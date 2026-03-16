<script setup lang="ts">
import { mdiAlert } from '@mdi/js'
import type { MessageItem, ToolCallItem } from '@/api/chat'
import MdiIcon from '@/components/MdiIcon.vue'
import SlimeBotLogo from '@/components/ui/SlimeBotLogo.vue'
import AssistantMessageBody from '@/components/chat/AssistantMessageBody.vue'

type ReplyTimelineEntry =
  | { id: string; kind: 'text'; content: string }
  | { id: string; kind: 'tool_start'; toolCallId: string }
  | { id: string; kind: 'tool_result'; toolCallId: string }

const props = defineProps<{
  item: MessageItem
  waiting: boolean
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
  sendBlockedOfflineText: string
  toolExecutionDetailTitle: string
}>()
</script>

<template>
  <div
    class="flex message-animate"
    :class="[
      item.role === 'assistant' ? 'gap-2' : 'gap-3',
      item.role === 'user' ? 'flex-row-reverse' : 'flex-row',
      item.role === 'user' && isFailedUserMessage(item.id)
        ? 'items-end'
        : (item.role === 'assistant' && isEmptyPlaceholder(item.id) && waiting
            ? 'items-center'
            : 'items-start'),
    ]"
  >
    <div
      v-if="item.role === 'assistant'"
      class="flex-shrink-0 w-10 h-10 flex items-center justify-center"
    >
      <SlimeBotLogo
        :size="40"
        :animated="isChatAssistantAvatarAnimated(item.id)"
        class="w-10 h-10 object-contain"
        :class="isChatAssistantAvatarAnimated(item.id) ? 'chat-ai-avatar-animated' : 'chat-ai-avatar'"
      />
    </div>

    <div
      v-if="item.role === 'user' && isFailedUserMessage(item.id)"
      class="failed-user-icon flex-shrink-0"
      :title="sendBlockedOfflineText"
    >
      <MdiIcon :path="mdiAlert" :size="15" />
    </div>

    <div
      class="text-sm leading-relaxed"
      :class="[
        item.role === 'user'
          ? 'user-bubble max-w-[calc(100%-52px)] rounded-2xl rounded-tr-sm px-4 py-2.5'
          : 'w-full',
        item.role === 'assistant' && isAssistantErrorMessage(item.id)
          ? 'error-bubble rounded-xl px-4 py-3'
          : '',
      ]"
    >
      <AssistantMessageBody
        v-if="item.role === 'assistant'"
        :item="item"
        :waiting="waiting"
        :get-reply-tool-count="getReplyToolCount"
        :get-reply-tool-summary="getReplyToolSummary"
        :get-reply-timeline="getReplyTimeline"
        :get-reply-tool-item="getReplyToolItem"
        :should-show-inline-tool-call="shouldShowInlineToolCall"
        :is-reply-tool-collapsed="isReplyToolCollapsed"
        :is-empty-placeholder="isEmptyPlaceholder"
        :open-tool-detail="openToolDetail"
        :approve-tool-call="approveToolCall"
        :tool-execution-detail-title="toolExecutionDetailTitle"
      />
      <template v-else>
        {{ item.content }}
      </template>
    </div>
  </div>
</template>
