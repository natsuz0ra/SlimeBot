<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  content: string
  done: boolean
  durationMs?: number
}>()

const { t } = useI18n()
const expanded = ref(false)

const durationText = computed(() => {
  if (!props.durationMs) return ''
  return (props.durationMs / 1000).toFixed(1) + 's'
})

const summaryText = computed(() => {
  if (!props.done) return t('thinkingLabel')
  if (durationText.value) return t('thoughtFor', { duration: durationText.value })
  return t('thinkingLabel')
})
</script>

<template>
  <div class="thinking-block">
    <button
      type="button"
      class="thinking-summary"
      :aria-expanded="done && expanded"
      @click="done && (expanded = !expanded)"
    >
      <svg
        class="thinking-dot"
        :class="{ 'thinking-dot--pulsing': !done }"
        viewBox="0 0 8 8"
        width="8"
        height="8"
        aria-hidden="true"
      >
        <circle cx="4" cy="4" r="4" fill="currentColor" />
      </svg>

      <span class="thinking-summary-text">{{ summaryText }}</span>

      <svg
        v-if="done"
        class="thinking-chevron"
        :class="{ 'thinking-chevron--open': expanded }"
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

    <Transition name="thinking-expand">
      <div v-if="done && expanded" class="thinking-content">
        <pre class="thinking-content-text sb-scrollbar">{{ content }}</pre>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.thinking-block {
  display: flex;
  flex-direction: column;
  gap: 0;
  border-radius: 8px;
  border: 1px solid rgba(125, 211, 252, 0.22);
  background:
    linear-gradient(90deg, rgba(125, 211, 252, 0.08), rgba(125, 211, 252, 0.02)),
    var(--card-bg, rgba(139, 92, 246, 0.04));
  overflow: hidden;
  transition: border-color 180ms ease, background-color 180ms ease;
}

.thinking-block:hover {
  border-color: rgba(56, 189, 248, 0.36);
}

.thinking-summary {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 34px;
  padding: 8px 12px;
  background: none;
  border: none;
  cursor: default;
  color: var(--text-secondary, #4c4980);
  font-size: 14px;
  font-weight: 500;
  line-height: 1;
  text-align: left;
  transition: background-color 150ms ease;
}

.thinking-summary[aria-expanded] {
  cursor: pointer;
}

.thinking-summary[aria-expanded]:hover {
  background: rgba(14, 165, 233, 0.06);
}

.thinking-summary[aria-expanded]:focus-visible {
  outline: 2px solid var(--focus-ring, rgba(139, 92, 246, 0.5));
  outline-offset: -2px;
  border-radius: 10px;
}

.thinking-dot {
  flex-shrink: 0;
  color: #0ea5e9;
}

.thinking-dot--pulsing {
  animation: thinking-pulse 1.4s ease-in-out infinite;
}

@keyframes thinking-pulse {
  0%, 100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.4;
    transform: scale(0.85);
  }
}

.thinking-summary-text {
  flex: 1 1 auto;
  color: #0284c7;
  font-weight: 650;
}

.thinking-chevron {
  flex-shrink: 0;
  color: var(--text-muted, #9ca3af);
  transition: transform 200ms ease;
}

.thinking-chevron--open {
  transform: rotate(180deg);
}

.thinking-content {
  border-left: 3px solid #0ea5e9;
  margin: 0 12px 10px 12px;
  padding: 8px 10px;
  border-radius: 0 8px 8px 0;
  background: rgba(14, 165, 233, 0.05);
}

.thinking-content-text {
  margin: 0;
  color: var(--text-secondary, #4c4980);
  font-size: 14px;
  line-height: 1.55;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: 240px;
  overflow-y: auto;
  scrollbar-width: thin;
}

.dark .thinking-block {
  border-color: rgba(56, 189, 248, 0.24);
  background:
    linear-gradient(90deg, rgba(56, 189, 248, 0.12), rgba(56, 189, 248, 0.04)),
    var(--card-bg, rgba(255, 255, 255, 0.04));
}

.dark .thinking-block:hover {
  border-color: rgba(125, 211, 252, 0.36);
}

.dark .thinking-summary-text,
.dark .thinking-dot {
  color: #7dd3fc;
}

.dark .thinking-content-text {
  color: #dbeafe;
}

.thinking-expand-enter-active {
  transition: opacity 200ms ease, max-height 300ms ease;
}

.thinking-expand-leave-active {
  transition: opacity 150ms ease, max-height 200ms ease;
}

.thinking-expand-enter-from,
.thinking-expand-leave-to {
  opacity: 0;
  max-height: 0;
}

.thinking-expand-enter-to,
.thinking-expand-leave-from {
  opacity: 1;
  max-height: 280px;
}

@media (prefers-reduced-motion: reduce) {
  .thinking-dot--pulsing {
    animation: none;
  }
  .thinking-chevron {
    transition: none;
  }
  .thinking-expand-enter-active,
  .thinking-expand-leave-active {
    transition: none;
  }
}
</style>
