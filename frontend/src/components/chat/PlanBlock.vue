<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { renderMarkdown } from '@/utils/markdown'

const props = withDefaults(defineProps<{
  content?: string
  generating?: boolean
}>(), {
  content: '',
  generating: false,
})

const { t } = useI18n()
const hasBody = computed(() => props.content.trim().length > 0)
</script>

<template>
  <section class="plan-block" :class="{ 'plan-block--generating': generating }" aria-label="Plan">
    <header class="plan-block-header">
      <span class="plan-block-dot" aria-hidden="true" />
      <span class="plan-block-title">{{ generating ? t('planningLabel') : 'Plan' }}</span>
    </header>
    <div v-if="hasBody" class="plan-block-body bubble-markdown" v-html="renderMarkdown(content)" />
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
  font-size: 13px;
  font-weight: 600;
  line-height: 1;
  letter-spacing: 0;
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

.dark .plan-block-dot {
  background: #fbbf24;
  box-shadow: 0 0 0 3px rgba(253, 224, 71, 0.2);
}

.dark .plan-block-header {
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
}
</style>
