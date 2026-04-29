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
import { unref } from 'vue'
import type { MessageItem } from '@/api/chat'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import TruncationTooltip from '@/components/ui/TruncationTooltip.vue'
import AppLogo from '@/components/ui/AppLogo.vue'
import AssistantMessageBody from '@/components/chat/AssistantMessageBody.vue'
import { formatSize } from '@/utils/format'
import { useChatContext } from '@/composables/chat/useChatContext'

defineProps<{
  item: MessageItem
}>()

const ctx = useChatContext()

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
</script>

<template>
  <div
    class="flex min-w-0 message-animate"
    :class="[
      item.role === 'assistant' ? 'gap-2' : 'gap-3',
      item.role === 'user' ? 'flex-row-reverse' : 'flex-row',
      item.role === 'user' && ctx.isFailedUserMessage(item.id)
        ? 'items-end'
        : (item.role === 'assistant' && ctx.isEmptyPlaceholder(item.id) && ctx.waiting
            ? 'items-center'
            : 'items-start'),
    ]"
  >
    <div v-if="item.role === 'assistant'" class="flex-shrink-0 w-10 h-10 flex items-center justify-center">
      <AppLogo
        :size="40"
        :animated="ctx.isChatAssistantAvatarAnimated(item.id)"
        class="w-10 h-10 object-contain"
        :class="ctx.isChatAssistantAvatarAnimated(item.id) ? 'chat-ai-avatar-animated' : 'chat-ai-avatar'"
      />
    </div>

    <div
      v-if="item.role === 'user' && ctx.isFailedUserMessage(item.id)"
      class="failed-user-icon flex-shrink-0"
      :title="unref(ctx.sendBlockedOfflineText)"
    >
      <MdiIcon :path="mdiAlert" :size="15" />
    </div>

    <div
      class="min-w-0 text-sm leading-relaxed"
      :class="[
        item.role === 'user'
          ? [
              'user-bubble max-w-[calc(100%-52px)] rounded-2xl rounded-tr-sm px-4',
              item.attachments && item.attachments.length > 0 ? 'py-4' : 'py-2.5',
            ]
          : 'w-full',
        item.role === 'assistant' && ctx.isAssistantErrorMessage(item.id)
          ? 'error-bubble rounded-xl px-4 py-3'
          : '',
      ]"
    >
      <AssistantMessageBody v-if="item.role === 'assistant'" :item="item" />
      <template v-else>
        <div
          v-if="item.attachments && item.attachments.length > 0"
          class="user-attachments-row"
          :class="item.content === '' ? 'user-attachments-row--solo' : ''"
        >
          <div
            v-for="(file, idx) in item.attachments"
            :key="`${file.name}-${idx}`"
            class="user-attachment-card"
          >
            <div class="group/tip flex min-w-0 items-center gap-2 mb-1">
              <MdiIcon class="flex-shrink-0" :path="attachmentIcon(file.iconType, file.category)" :size="16" />
              <TruncationTooltip
                inherit-group
                :text="file.name"
                wrapper-class="min-w-0 flex-1"
                content-class="user-attachment-name text-xs font-medium"
              />
            </div>
            <div class="text-[11px] opacity-80">
              {{ file.ext }} · {{ formatSize(file.sizeBytes) }}
            </div>
          </div>
        </div>
        <div v-if="item.content !== ''" class="user-message-content">{{ item.content }}</div>
      </template>
    </div>
  </div>
</template>
