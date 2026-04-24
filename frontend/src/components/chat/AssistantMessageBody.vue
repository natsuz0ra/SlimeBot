<script setup lang="ts">
import ToolCallInline from '@/components/chat/ToolCallInline.vue'
import ThinkingBlock from '@/components/chat/ThinkingBlock.vue'
import PlanBlock from '@/components/chat/PlanBlock.vue'
import TypingDots from '@/components/chat/TypingDots.vue'
import { renderMarkdown } from '@/utils/markdown'
import type { MessageItem } from '@/api/chat'
import { useChatContext } from '@/composables/chat/useChatContext'

defineProps<{
  item: MessageItem
}>()

const ctx = useChatContext()
</script>

<template>
  <div class="assistant-reply-body text-sm leading-relaxed w-full">
    <template v-for="entry in ctx.getReplyTimeline(item.id)" :key="entry.id">
      <div class="assistant-reply-segment" :class="`assistant-reply-segment--${entry.kind}`">
        <ThinkingBlock
          v-if="entry.kind === 'thinking'"
          :content="entry.content"
          :done="entry.done"
          :duration-ms="entry.durationMs"
        />

        <div v-else-if="entry.kind === 'text'" class="bubble-markdown sb-text-primary" v-html="renderMarkdown(entry.content)" />

        <PlanBlock
          v-else-if="entry.kind === 'plan'"
          :content="entry.content"
          :generating="entry.generating ?? false"
        />

        <ToolCallInline
          v-else-if="entry.kind === 'tool_start' && ctx.getReplyToolItem(item.id, entry.toolCallId)"
          :item="ctx.getReplyToolItem(item.id, entry.toolCallId)!"
          :nested-tools="ctx.getSubagentChildTools(item.id, entry.toolCallId)"
          @approve="ctx.approveToolCall($event, true)"
          @reject="ctx.approveToolCall($event, false)"
        />
      </div>
    </template>

    <TypingDots v-if="ctx.isEmptyPlaceholder(item.id) && ctx.waiting" />
  </div>
</template>

<style scoped>
.assistant-reply-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.assistant-reply-segment {
  min-width: 0;
}

.assistant-reply-segment--text + .assistant-reply-segment--text {
  margin-top: -2px;
}

.assistant-reply-segment--thinking,
.assistant-reply-segment--tool_start,
.assistant-reply-segment--plan {
  max-width: min(100%, 680px);
}
</style>
