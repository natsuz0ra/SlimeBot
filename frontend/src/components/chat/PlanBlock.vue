<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { renderMarkdown } from '@/utils/markdown'

const props = withDefaults(defineProps<{
  content?: string
  generating?: boolean
  activeTarget?: boolean
}>(), {
  content: '',
  generating: false,
  activeTarget: false,
})

const { t } = useI18n()
const hasBody = computed(() => props.content.trim().length > 0)
const expanded = ref(false)

const showBody = computed(() => {
  if (!props.generating) return hasBody.value
  return expanded.value && hasBody.value
})

const headerClickable = computed(() => props.generating && hasBody.value)

function toggleExpand() {
  if (headerClickable.value) expanded.value = !expanded.value
}
</script>

<template>
  <section
    class="plan-block"
    data-plan-block="true"
    :data-plan-block-active="activeTarget ? 'true' : undefined"
    :class="{
      'plan-block--generating': generating,
      'plan-block--clickable': headerClickable,
    }"
    aria-label="Plan"
  >
    <header class="plan-block-header" @click="toggleExpand">
      <span class="plan-block-dot" aria-hidden="true" />
      <span class="plan-block-title">{{ generating ? t('planningLabel') : 'Plan' }}</span>
      <span class="plan-block-spacer" />
      <svg
        v-if="headerClickable"
        class="plan-block-chevron"
        :class="{ 'plan-block-chevron--open': expanded }"
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
    </header>
    <Transition name="plan-expand">
      <div v-if="showBody" class="plan-block-body bubble-markdown" v-html="renderMarkdown(content)" />
    </Transition>
  </section>
</template>

<style scoped>
.plan-block {
  overflow: hidden;
  border: 1px solid rgba(251, 191, 36, 0.22);
  border-left: 3px solid #f59e0b;
  border-radius: 8px;
  background:
    linear-gradient(90deg, rgba(251, 191, 36, 0.08), rgba(251, 191, 36, 0.02)),
    var(--card-bg, rgba(139, 92, 246, 0.04));
  transition: border-color 180ms ease;
}

.plan-block:hover {
  border-color: rgba(245, 158, 11, 0.36);
  border-left-color: #f59e0b;
}

.plan-block--clickable .plan-block-header {
  cursor: pointer;
}

.plan-block--clickable:hover {
  border-color: rgba(245, 158, 11, 0.5);
  border-left-color: #f59e0b;
}

.plan-block-header {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 34px;
  padding: 8px 12px 6px;
  color: #d97706;
}

.plan-block-dot {
  flex-shrink: 0;
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: #f59e0b;
  box-shadow: 0 0 0 3px rgba(251, 191, 36, 0.18);
}

.plan-block--generating .plan-block-dot {
  animation: plan-gen-pulse 1.4s ease-in-out infinite;
}

.plan-block-title {
  font-size: 14px;
  font-weight: 600;
  line-height: 1;
  letter-spacing: 0;
}

.plan-block-spacer {
  flex: 1 1 auto;
}

.plan-block-chevron {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  margin-left: 4px;
  color: #d97706;
  font-size: 0;
  transition: transform 150ms ease;
  transform: rotate(-90deg);
}

.plan-block-chevron--open {
  transform: rotate(0deg);
}

.plan-block-body {
  padding: 0 12px 12px 23px;
  color: var(--text-primary);
}

.plan-block-body :deep(h1),
.plan-block-body :deep(h2),
.plan-block-body :deep(h3) {
  margin-top: 4px;
}

.plan-expand-enter-active {
  transition: opacity 180ms ease, max-height 250ms ease;
}

.plan-expand-leave-active {
  transition: opacity 120ms ease, max-height 180ms ease;
}

.plan-expand-enter-from,
.plan-expand-leave-to {
  opacity: 0;
  max-height: 0;
}

.plan-expand-enter-to,
.plan-expand-leave-from {
  opacity: 1;
  max-height: 800px;
}

.dark .plan-block {
  border-color: rgba(253, 224, 71, 0.24);
  border-left-color: #fbbf24;
  background:
    linear-gradient(90deg, rgba(253, 224, 71, 0.12), rgba(253, 224, 71, 0.04)),
    var(--card-bg, rgba(255, 255, 255, 0.04));
}

.dark .plan-block:hover {
  border-color: rgba(253, 224, 71, 0.36);
  border-left-color: #fbbf24;
}

.dark .plan-block--clickable:hover {
  border-color: rgba(253, 224, 71, 0.5);
  border-left-color: #fbbf24;
}

.dark .plan-block-dot {
  background: #fbbf24;
  box-shadow: 0 0 0 3px rgba(253, 224, 71, 0.2);
}

.dark .plan-block-header {
  color: #fcd34d;
}

.dark .plan-block-chevron {
  color: #fcd34d;
}

@keyframes plan-gen-pulse {
  0%, 100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.35;
    transform: scale(0.8);
  }
}

@media (prefers-reduced-motion: reduce) {
  .plan-block--generating .plan-block-dot {
    animation: none;
  }
  .plan-block-chevron {
    transition: none;
  }
  .plan-expand-enter-active,
  .plan-expand-leave-active {
    transition: none;
  }
}
</style>
