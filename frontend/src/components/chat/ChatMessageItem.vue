<script setup lang="ts">
import {
  mdiAlert,
  mdiFile,
  mdiFileCodeOutline,
  mdiFileDocumentOutline,
  mdiFileExcelOutline,
  mdiFileImageOutline,
  mdiMusic,
  mdiFolderZipOutline,
} from '@mdi/js'
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

function attachmentIcon(iconType?: string, category?: string) {
  if (iconType === 'audio' || category === 'audio') {
    return mdiMusic
  }
  switch (iconType) {
    case 'image':
      return mdiFileImageOutline
    case 'pdf':
      return mdiFileDocumentOutline
    case 'word':
      return mdiFileDocumentOutline
    case 'excel':
      return mdiFileExcelOutline
    case 'archive':
      return mdiFolderZipOutline
    case 'code':
      return mdiFileCodeOutline
    default:
      if (category === 'document') {
        return mdiFileDocumentOutline
      }
      return mdiFile
  }
}

function formatSize(sizeBytes: number) {
  if (sizeBytes < 1024) return `${sizeBytes}B`
  if (sizeBytes < 1024 * 1024) return `${(sizeBytes / 1024).toFixed(1)}KB`
  return `${(sizeBytes / (1024 * 1024)).toFixed(1)}MB`
}
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
        <div v-if="item.attachments && item.attachments.length > 0" class="user-attachments-row">
          <div
            v-for="(file, idx) in item.attachments"
            :key="`${file.name}-${idx}`"
            class="user-attachment-card"
          >
            <div class="flex items-center gap-2 mb-1">
              <MdiIcon :path="attachmentIcon(file.iconType, file.category)" :size="16" />
              <span class="user-attachment-name text-xs font-medium">{{ file.name }}</span>
            </div>
            <div class="text-[11px] opacity-80">
              {{ file.ext }} · {{ formatSize(file.sizeBytes) }}
            </div>
          </div>
        </div>
        <div v-if="item.content !== ''">{{ item.content }}</div>
      </template>
    </div>
  </div>
</template>
