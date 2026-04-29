<script setup lang="ts">
import { TransitionGroup, computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import ToolCallInline from '@/components/chat/ToolCallInline.vue'
import ThinkingBlock from '@/components/chat/ThinkingBlock.vue'
import PlanBlock from '@/components/chat/PlanBlock.vue'
import TypingDots from '@/components/chat/TypingDots.vue'
import { renderMarkdown } from '@/utils/markdown'
import type { MessageItem } from '@/api/chat'
import { useChatContext } from '@/composables/chat/useChatContext'
import { getCollapsedReplyTimeline } from '@/utils/replyBatchBuilder'

const props = defineProps<{
  item: MessageItem
}>()

const ctx = useChatContext()
const { t } = useI18n()
const elapsedTick = ref(0)
let elapsedTimer: ReturnType<typeof setInterval> | undefined

const isStreaming = computed(() => ctx.isStreamingMessage(props.item.id))
const showCollapseBar = computed(() => ctx.shouldShowReplyCollapseBar(props.item.id))
const isExpanded = computed(() => isStreaming.value || !ctx.isReplyToolCollapsed(props.item.id))
const fullTimeline = computed(() => ctx.getReplyTimeline(props.item.id))
const collapsedEntryIds = computed(() => new Set(getCollapsedReplyTimeline(fullTimeline.value).map((entry) => entry.id)))
const renderedTimeline = computed(() => (
  isExpanded.value
    ? fullTimeline.value
    : fullTimeline.value.filter((entry) => collapsedEntryIds.value.has(entry.id))
))
const elapsedMs = computed(() => {
  elapsedTick.value
  return ctx.getReplyElapsedMs(props.item.id)
})

function formatDuration(ms: number | undefined) {
  if (typeof ms !== 'number') return '--'
  const safeMs = Math.max(0, ms)
  if (safeMs < 1000) return `${safeMs}ms`
  if (safeMs < 60_000) return `${(safeMs / 1000).toFixed(1)}s`
  const minutes = Math.floor(safeMs / 60_000)
  const seconds = Math.floor((safeMs % 60_000) / 1000)
  return `${minutes}m ${seconds}s`
}

const elapsedLabel = computed(() => t('replyElapsed', { duration: formatDuration(elapsedMs.value) }))

function stopElapsedTimer() {
  if (!elapsedTimer) return
  clearInterval(elapsedTimer)
  elapsedTimer = undefined
}

watch(
  isStreaming,
  (active) => {
    stopElapsedTimer()
    if (!active) return
    elapsedTimer = setInterval(() => {
      elapsedTick.value += 1
    }, 250)
  },
  { immediate: true },
)

onUnmounted(() => {
  stopElapsedTimer()
})
</script>

<template>
  <div class="assistant-reply-body min-w-0 text-sm leading-relaxed w-full">
    <button
      v-if="showCollapseBar"
      type="button"
      class="reply-collapse-bar"
      :aria-expanded="isExpanded ? 'true' : 'false'"
      :disabled="isStreaming"
      @click="ctx.toggleReplyCollapsed(item.id)"
    >
      <span class="reply-collapse-time">{{ elapsedLabel }}</span>
      <svg
        class="reply-collapse-arrow"
        :class="{ 'reply-collapse-arrow--open': isExpanded }"
        viewBox="0 0 16 16"
        width="14"
        height="14"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <path d="M4 6l4 4 4-4" />
      </svg>
    </button>

    <TransitionGroup name="reply-segment" tag="div" class="assistant-reply-timeline">
      <div
        v-for="(entry, index) in renderedTimeline"
        :key="entry.id"
        class="assistant-reply-segment"
        :class="[
          `assistant-reply-segment--${entry.kind}`,
          index === 0 ? 'assistant-reply-segment--first-visible' : '',
        ]"
      >
        <div class="assistant-reply-segment-inner">
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
      </div>
    </TransitionGroup>

    <TypingDots v-if="ctx.isEmptyPlaceholder(item.id) && ctx.waiting" />
  </div>
</template>

<style scoped>
.assistant-reply-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-width: 100%;
}

.assistant-reply-timeline {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.assistant-reply-segment {
  display: grid;
  grid-template-rows: 1fr;
  min-width: 0;
  margin-top: 10px;
  overflow: hidden;
}

.assistant-reply-segment:first-child,
.assistant-reply-segment--first-visible {
  margin-top: 0;
}

.assistant-reply-segment-inner {
  min-width: 0;
  overflow: hidden;
}

.reply-collapse-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  width: min(100%, 680px);
  min-height: 32px;
  padding: 6px 10px;
  border: 1px solid var(--tool-card-border);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.72);
  color: var(--text-secondary);
  cursor: pointer;
  text-align: left;
  transition: background-color 150ms ease, border-color 150ms ease, color 150ms ease;
}

.reply-collapse-bar:hover {
  border-color: var(--tool-card-border-hover);
  background: rgba(255, 255, 255, 0.92);
  color: var(--text-primary);
}

.reply-collapse-bar:disabled {
  cursor: default;
}

.reply-collapse-bar:disabled:hover {
  color: var(--text-secondary);
}

.reply-collapse-bar:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 2px;
}

.reply-collapse-time {
  min-width: 0;
  flex: 1 1 auto;
  color: inherit;
  font-size: 13px;
  font-weight: 650;
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.reply-collapse-arrow {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  color: var(--tool-summary-text);
  font-size: 0;
  flex-shrink: 0;
  transition: transform 150ms ease;
  transform: rotate(-90deg);
}

.reply-collapse-arrow--open {
  transform: rotate(0deg);
}

.assistant-reply-segment--text + .assistant-reply-segment--text {
  margin-top: 8px;
}

.assistant-reply-segment--thinking,
.assistant-reply-segment--tool_start,
.assistant-reply-segment--plan {
  max-width: min(100%, 680px);
}

.reply-segment-enter-active,
.reply-segment-leave-active {
  transition:
    grid-template-rows 300ms cubic-bezier(0.22, 1, 0.36, 1),
    margin-top 300ms cubic-bezier(0.22, 1, 0.36, 1),
    opacity 220ms ease,
    transform 300ms cubic-bezier(0.22, 1, 0.36, 1);
}

.reply-segment-move {
  transition: transform 300ms cubic-bezier(0.22, 1, 0.36, 1);
}

.reply-segment-enter-from,
.reply-segment-leave-to {
  grid-template-rows: 0fr;
  margin-top: 0;
  opacity: 0;
  transform: translateY(-6px);
}

.reply-segment-leave-active {
  pointer-events: none;
}

.dark .reply-collapse-bar {
  background: rgba(255, 255, 255, 0.05);
  border-color: var(--tool-card-border);
}

.dark .reply-collapse-bar:hover {
  background: rgba(255, 255, 255, 0.08);
  border-color: var(--tool-card-border-hover);
}

@media (prefers-reduced-motion: reduce) {
  .reply-collapse-bar,
  .reply-collapse-arrow,
  .reply-segment-enter-active,
  .reply-segment-leave-active,
  .reply-segment-move {
    transition: none;
  }
}
</style>
