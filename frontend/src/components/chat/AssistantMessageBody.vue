<script setup lang="ts">
import ToolCallCard from '@/components/chat/ToolCallCard.vue'
import TypingDots from '@/components/chat/TypingDots.vue'
import { renderMarkdown } from '@/utils/markdown'
import { unref } from 'vue'
import type { MessageItem } from '@/api/chat'
import { useChatContext } from '@/composables/chat/useChatContext'

defineProps<{
  item: MessageItem
}>()

const ctx = useChatContext()
</script>

<template>
  <div class="text-sm leading-relaxed w-full">
    <div v-if="ctx.getReplyToolCount(item.id) > 0" class="assistant-tool-summary-row mb-2.5">
      <button
        type="button"
        class="tool-summary-btn inline-flex items-center gap-2 px-3 py-1.5 text-xs rounded-full transition-all duration-150 cursor-pointer max-w-full"
        aria-haspopup="dialog"
        :aria-label="`${unref(ctx.toolExecutionDetailTitle)} - ${ctx.getReplyToolSummary(item.id)}`"
        @click="ctx.openToolDetail(item.id)"
      >
        <svg class="w-3.5 h-3.5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
          <path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
        </svg>
        <span class="truncate max-w-[min(62vw,420px)]">{{ ctx.getReplyToolSummary(item.id) }}</span>
      </button>
    </div>

    <div v-if="ctx.getReplyToolCount(item.id) > 0 && !ctx.isReplyToolCollapsed(item.id)" class="flex flex-col gap-2 mb-3">
      <template v-for="entry in ctx.getReplyTimeline(item.id)" :key="entry.id">
        <div v-if="entry.kind === 'text'" class="bubble-markdown sb-text-primary" v-html="renderMarkdown(entry.content)" />
        <div v-else-if="entry.kind === 'tool_start' && ctx.shouldShowInlineToolCall(item.id, entry.toolCallId)" class="w-full">
          <ToolCallCard
            v-if="ctx.getReplyToolItem(item.id, entry.toolCallId)"
            :item="ctx.getReplyToolItem(item.id, entry.toolCallId)!"
            :show-preamble="false"
            @approve="ctx.approveToolCall($event, true)"
            @reject="ctx.approveToolCall($event, false)"
          />
        </div>
      </template>
    </div>

    <div
      v-if="ctx.getReplyToolCount(item.id) === 0 || ctx.isReplyToolCollapsed(item.id)"
      class="bubble-markdown sb-text-primary"
      v-html="renderMarkdown(item.content)"
    />

    <TypingDots v-if="ctx.isEmptyPlaceholder(item.id) && ctx.waiting" />
  </div>
</template>
