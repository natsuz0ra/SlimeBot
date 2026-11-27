<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import type { LightweightToolDisplay } from '@/utils/toolDisplay'
import { buildLightweightToolGroupSummary } from '@/utils/toolDisplay'
import { getToolCallLabel, getToolCallStatusLabel, getToolCallStatusTone } from '@/composables/chat/useToolCallDisplay'
import { mdiChevronDown, mdiFileSearchOutline } from '@mdi/js'

const props = defineProps<{
  items: LightweightToolDisplay[]
  runningOverride?: boolean
}>()

const { locale, t } = useI18n()
const expanded = ref(false)

const summary = computed(() =>
  buildLightweightToolGroupSummary(props.items, locale.value.startsWith('zh') ? 'zh' : 'en'),
)

const hasActive = computed(() => props.items.some((item) => item.status === 'pending' || item.status === 'reviewing' || item.status === 'executing'))
const hasFailure = computed(() => props.items.some((item) => item.status === 'error' || item.status === 'rejected'))
const groupTone = computed(() => props.runningOverride || hasActive.value ? 'running' : hasFailure.value ? 'failed' : 'completed')

function statusSymbol(status: LightweightToolDisplay['status']) {
  if (status === 'completed') return '\u2713'
  if (status === 'error' || status === 'rejected') return '\u2717'
  return ''
}

function isActiveStatus(status: LightweightToolDisplay['status']) {
  return status === 'pending' || status === 'reviewing' || status === 'executing'
}

function toolLabel(toolName: string) {
  return getToolCallLabel(toolName === 'search_file' ? 'search_files' : toolName, (key) => t(key))
}

function toggleExpanded() {
  expanded.value = !expanded.value
}
</script>

<template>
  <section :class="['light-tool-group', `light-tool-group--${groupTone}`]">
    <button
      type="button"
      class="light-tool-summary"
      :aria-expanded="expanded ? 'true' : 'false'"
      @click="toggleExpanded"
    >
      <MdiIcon :path="mdiFileSearchOutline" :size="14" class="light-tool-icon" />
      <span class="light-tool-title">{{ summary || t('lightweightToolGroupFallback') }}</span>
      <span class="light-tool-count">{{ items.length }}</span>
      <MdiIcon
        :path="mdiChevronDown"
        :size="14"
        class="light-tool-chevron"
        :class="{ 'light-tool-chevron--open': expanded }"
      />
    </button>

    <Transition name="tool-subagent-expand">
      <div v-if="expanded" class="light-tool-list">
        <div v-for="item in items" :key="item.toolCallId" class="light-tool-item">
          <span
            :class="[
              'light-tool-status',
              `light-tool-status--${getToolCallStatusTone(item.status)}`,
              isActiveStatus(item.status) ? 'light-tool-status--active' : '',
            ]"
            :aria-label="getToolCallStatusLabel(item.status, (key) => t(key))"
          >
            <template v-if="statusSymbol(item.status)">{{ statusSymbol(item.status) }}</template>
            <span v-else-if="isActiveStatus(item.status)" class="light-tool-running-dot" aria-hidden="true" />
            <span v-else>{{ getToolCallStatusLabel(item.status, (key) => t(key)) }}</span>
          </span>
          <span class="light-tool-item-label">{{ toolLabel(item.toolName) }}</span>
          <span class="light-tool-item-main">
            <span class="light-tool-item-target" :title="item.target">{{ item.target }}</span>
            <span v-if="item.error" class="light-tool-error">{{ item.error }}</span>
          </span>
        </div>
      </div>
    </Transition>
  </section>
</template>

<style scoped>
.light-tool-group {
  position: relative;
  width: min(100%, 760px);
  border-radius: 8px;
  border: 1px solid var(--tool-card-border);
  background: var(--tool-card-bg);
  box-shadow: inset 2px 0 0 var(--tool-running-dot, var(--accent-primary));
  overflow: hidden;
  transition: border-color 180ms ease, background-color 180ms ease, box-shadow 180ms ease;
}

.light-tool-group:hover {
  border-color: var(--tool-card-border-hover);
}

.light-tool-group--running {
  border-color: var(--tool-running-border);
  animation: light-tool-running-breathe 2.3s ease-in-out infinite;
}

.light-tool-group--running::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(90deg, transparent 0%, color-mix(in srgb, var(--tool-running-dot, var(--accent-primary)) 8%, transparent) 38%, transparent 72%);
  opacity: 0;
  transform: translateX(-110%);
  animation: light-tool-sweep-breathe 2.7s ease-in-out infinite;
}

.light-tool-group--completed {
  border-color: var(--tool-card-border);
  box-shadow: inset 2px 0 0 color-mix(in srgb, var(--accent-primary) 52%, transparent);
}

.light-tool-group--completed:hover {
  border-color: var(--tool-card-border-hover);
  box-shadow: inset 2px 0 0 color-mix(in srgb, var(--accent-primary) 75%, transparent);
}

.light-tool-group--failed {
  border-color: var(--tool-error-border);
  background: var(--tool-card-bg);
  box-shadow: inset 2px 0 0 color-mix(in srgb, var(--tool-error-dot, #ef4444) 70%, transparent);
}

.light-tool-group--failed:hover {
  border-color: var(--tool-error-border);
  box-shadow: inset 2px 0 0 var(--tool-error-dot, #ef4444);
}

.light-tool-summary {
  width: 100%;
  min-height: 34px;
  display: grid;
  grid-template-columns: 18px minmax(0, 1fr) auto 18px;
  align-items: center;
  gap: 8px;
  padding: 7px 10px;
  border: 0;
  background: transparent;
  color: var(--text-primary);
  cursor: pointer;
  text-align: left;
}

.light-tool-summary:hover {
  background: color-mix(in srgb, var(--tool-summary-bg, rgba(100, 116, 139, 0.06)) 72%, transparent);
}

.light-tool-icon {
  color: var(--accent-primary);
}

.light-tool-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 650;
}

.light-tool-count {
  min-width: 22px;
  height: 20px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  padding: 0 7px;
  background: var(--tool-summary-bg);
  color: var(--tool-summary-text);
  font-size: 12px;
  font-weight: 650;
}

.light-tool-chevron {
  color: var(--text-secondary);
  transition: transform 0.16s ease;
}

.light-tool-chevron--open {
  transform: rotate(180deg);
}

.light-tool-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 0 10px 10px 10px;
}

.light-tool-item {
  display: grid;
  grid-template-columns: 18px auto minmax(0, 1fr);
  align-items: start;
  gap: 8px;
  color: var(--text-secondary);
  font-size: 12px;
}

.light-tool-item-label {
  color: var(--text-primary);
  font-weight: 650;
  line-height: 20px;
  white-space: nowrap;
}

.light-tool-item-main {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
  line-height: 20px;
}

.light-tool-item-target {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.light-tool-status {
  min-width: 18px;
  height: 20px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  font-weight: 650;
  line-height: 20px;
  white-space: nowrap;
}

.light-tool-status--success {
  color: var(--tool-success-dot, #10b981);
}

.light-tool-status--error {
  color: var(--tool-error-dot, #ef4444);
}

.light-tool-status--pending,
.light-tool-status--executing {
  color: var(--tool-pending-dot, #facc15);
}

.light-tool-status--active {
  color: var(--tool-running-dot, var(--accent-primary));
}

.light-tool-running-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: currentColor;
  box-shadow: 0 0 0 3px color-mix(in srgb, currentColor 18%, transparent);
  animation: light-tool-running-dot-pulse 1.4s ease-in-out infinite;
}

.light-tool-error {
  color: var(--tool-error-text);
  overflow-wrap: anywhere;
  line-height: 1.45;
}

.tool-subagent-expand-enter-active {
  transition: opacity 180ms ease, max-height 250ms ease;
}

.tool-subagent-expand-leave-active {
  transition: opacity 120ms ease, max-height 180ms ease;
}

.tool-subagent-expand-enter-active,
.tool-subagent-expand-leave-active {
  overflow: hidden;
}

.tool-subagent-expand-enter-from,
.tool-subagent-expand-leave-to {
  opacity: 0;
  max-height: 0;
}

.tool-subagent-expand-enter-to,
.tool-subagent-expand-leave-from {
  opacity: 1;
  max-height: 500px;
}

@keyframes light-tool-running-breathe {
  0%,
  100% {
    border-color: var(--tool-card-border);
    box-shadow: inset 2px 0 0 color-mix(in srgb, var(--tool-running-dot, var(--accent-primary)) 60%, transparent);
  }

  50% {
    border-color: var(--tool-running-border);
    box-shadow: inset 2px 0 0 var(--tool-running-dot, var(--accent-primary));
  }
}

@keyframes light-tool-running-dot-pulse {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }

  50% {
    opacity: 0.35;
    transform: scale(0.8);
  }
}

@keyframes light-tool-sweep-breathe {
  0%,
  42% {
    opacity: 0;
    transform: translateX(-110%);
  }

  58% {
    opacity: 1;
  }

  100% {
    opacity: 0;
    transform: translateX(110%);
  }
}

@media (prefers-reduced-motion: reduce) {
  .light-tool-chevron,
  .tool-subagent-expand-enter-active,
  .tool-subagent-expand-leave-active {
    transition: none;
  }

  .light-tool-group--running,
  .light-tool-running-dot,
  .light-tool-group--running::before {
    animation: none;
  }

  .light-tool-group--running::before {
    opacity: 0;
    transform: none;
  }
}
</style>
