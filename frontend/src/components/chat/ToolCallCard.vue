<script setup lang="ts">
import { computed, ref } from 'vue'
import { mdiCheck, mdiClose, mdiConsoleLine, mdiWeb } from '@mdi/js'
import { useI18n } from 'vue-i18n'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import type { ToolCallItem } from '@/api/chat'

const props = withDefaults(defineProps<{
  item: ToolCallItem & { preamble?: string }
  showPreamble?: boolean
  dense?: boolean
}>(), {
  showPreamble: false,
  dense: false,
})

const emit = defineEmits<{
  approve: [toolCallId: string]
  reject: [toolCallId: string]
}>()

const { t } = useI18n()
const isOutputExpanded = ref(false)

const toolIcon = computed(() => {
  if (props.item.toolName === 'exec') return mdiConsoleLine
  if (props.item.toolName === 'http_request') return mdiWeb
  if (props.item.toolName === 'web_search') return mdiWeb
  return mdiConsoleLine
})

const toolLabel = computed(() => {
  if (props.item.toolName === 'exec') return t('toolExec')
  if (props.item.toolName === 'http_request') return t('toolHttpRequest')
  if (props.item.toolName === 'web_search') return t('toolWebSearch')
  return props.item.toolName
})

const statusLabel = computed(() => {
  switch (props.item.status) {
    case 'pending': return t('toolCallPending')
    case 'executing': return t('toolCallExecuting')
    case 'completed': return t('toolCallCompleted')
    case 'rejected': return t('toolCallRejected')
    case 'error': return t('toolCallError')
    default: return ''
  }
})

const statusDotClass = computed(() => {
  switch (props.item.status) {
    case 'pending': return 'status-dot-pending'
    case 'executing': return 'status-dot-executing'
    case 'completed': return 'status-dot-success'
    case 'rejected':
    case 'error': return 'status-dot-error'
    default: return 'status-dot-default'
  }
})

const statusTextClass = computed(() => {
  switch (props.item.status) {
    case 'pending': return 'tool-status-text tool-status-text-pending'
    case 'executing': return 'tool-status-text tool-status-text-executing'
    case 'completed': return 'tool-status-text tool-status-text-success'
    case 'rejected':
    case 'error': return 'tool-status-text tool-status-text-error'
    default: return 'tool-status-text tool-status-text-default'
  }
})

const paramsDisplay = computed(() => {
  const entries = Object.entries(props.item.params)
  if (entries.length === 0) return ''
  return entries.map(([k, v]) => `${k}: ${v}`).join('\n')
})

const showActions = computed(() => props.item.status === 'pending')
const showResult = computed(() => props.item.status === 'completed' || props.item.status === 'error')
const shouldShowPreamble = computed(() => !!props.showPreamble && !!props.item.preamble)
const outputPanelId = computed(() => `tool-output-${props.item.toolCallId}`)

function onOutputToggle(event: Event) {
  const target = event.currentTarget as HTMLDetailsElement | null
  isOutputExpanded.value = !!target?.open
}
</script>

<template>
  <article :class="['tool-card w-full rounded-xl text-sm', { 'tool-card--dense': dense }]">
    <header class="tool-header">
      <div class="tool-header-main">
        <div class="tool-meta">
          <MdiIcon :path="toolIcon" :size="14" class="tool-icon flex-shrink-0" />
          <span class="tool-label">{{ toolLabel }}</span>
          <code
            v-if="item.command"
            class="tool-command"
            :title="item.command"
          >
            {{ item.command }}
          </code>
        </div>

        <div class="tool-status ml-auto">
          <span
            class="tool-status-dot"
            :class="[statusDotClass, item.status === 'executing' ? 'status-dot-pulse' : '']"
          />
          <span :class="statusTextClass">{{ statusLabel }}</span>
          <svg
            v-if="item.status === 'executing'"
            class="animate-spin-icon w-3.5 h-3.5 flex-shrink-0 tool-status-spinner"
            fill="none"
            viewBox="0 0 24 24"
            aria-hidden="true"
          >
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
          </svg>
        </div>
      </div>
    </header>

    <section v-if="paramsDisplay || showResult" class="tool-section mt-2">
      <template v-if="paramsDisplay">
        <p class="tool-section-title">{{ t('toolCallParams') }}</p>
        <pre class="tool-params sb-scrollbar">{{ paramsDisplay }}</pre>
      </template>

      <div v-if="showResult" class="tool-result-block">
        <details v-if="item.output" class="tool-output-details text-xs" @toggle="onOutputToggle">
          <summary
            class="tool-result-summary"
            :aria-expanded="isOutputExpanded ? 'true' : 'false'"
            :aria-controls="outputPanelId"
          >
            <span class="tool-result-label">{{ t('toolCallResult') }}</span>
            <span class="tool-output-summary">{{ t('toolCallOutput') }}</span>
          </summary>
          <!-- Keep scrollbar visual style consistent with detail dialog via global sb-scrollbar -->
          <pre :id="outputPanelId" class="tool-output sb-scrollbar">{{ item.output }}</pre>
        </details>

        <div v-else class="tool-result-summary tool-result-summary--plain" aria-live="polite">
          <span class="tool-result-label">{{ t('toolCallResult') }}</span>
        </div>

        <div v-if="item.error" class="tool-error">{{ item.error }}</div>
      </div>
    </section>

    <section v-if="shouldShowPreamble" class="tool-section mt-2.5">
      <p class="tool-section-title">{{ t('toolCallPreamble') }}</p>
      <div class="tool-preamble">
      {{ item.preamble }}
      </div>
    </section>

    <section v-if="showActions" class="tool-actions">
      <button
        type="button"
        class="tool-action-btn approve-btn"
        @click="emit('approve', item.toolCallId)"
      >
        <MdiIcon :path="mdiCheck" :size="11" />
        {{ t('toolCallApprove') }}
      </button>
      <button
        type="button"
        class="tool-action-btn reject-btn"
        @click="emit('reject', item.toolCallId)"
      >
        <MdiIcon :path="mdiClose" :size="11" />
        {{ t('toolCallReject') }}
      </button>
    </section>

  </article>
</template>

<style scoped>
.tool-card {
  background: var(--card-bg);
  border: 1px solid var(--tool-card-border);
  box-shadow: var(--floating-elevation-shadow);
  padding: 10px 12px;
  overflow: visible;
  transition: border-color 180ms ease, background-color 180ms ease, box-shadow 180ms ease, transform 180ms ease;
}

.tool-card:hover {
  border-color: var(--tool-card-border-hover);
  box-shadow: var(--tool-card-shadow-hover);
}

.tool-icon {
  color: var(--tool-meta-icon);
}

.tool-label {
  color: var(--tool-meta-text);
  font-size: 12px;
  font-weight: 600;
  line-height: 1;
}

.tool-header {
  display: flex;
  flex-direction: column;
  gap: 0;
  min-width: 0;
}

.tool-header-main {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.tool-meta {
  display: flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  flex: 1 1 auto;
  max-width: calc(100% - 110px);
}

.tool-command {
  display: inline-block;
  max-width: min(48%, 320px);
  color: var(--tool-command-text);
  background: var(--tool-command-bg);
  border: 1px solid var(--tool-command-border);
  border-radius: 7px;
  padding: 1px 6px;
  font-size: 10px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tool-status {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  white-space: nowrap;
}

.tool-status-dot {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  flex-shrink: 0;
}

.status-dot-pending { background: var(--tool-pending-dot); }
.status-dot-executing { background: var(--tool-running-dot); }
.status-dot-success { background: var(--tool-success-dot); }
.status-dot-error { background: var(--tool-error-dot); }
.status-dot-default { background: var(--text-muted); }

.status-dot-pulse {
  animation: tool-dot-pulse 1.2s ease-in-out infinite;
}

.tool-status-text {
  font-size: 12px;
  font-weight: 600;
  line-height: 1;
}

.tool-status-text-pending { color: var(--tool-pending-text); }
.tool-status-text-executing { color: var(--tool-running-text); }
.tool-status-text-success { color: var(--tool-success-text); }
.tool-status-text-error { color: var(--tool-error-text); }
.tool-status-text-default { color: var(--text-muted); }

.tool-status-spinner {
  color: var(--tool-running-text);
}

.tool-section {
  border: 1px solid var(--tool-section-border);
  border-radius: 8px;
  padding: 8px;
  background: var(--tool-section-bg);
}

.tool-card--dense {
  padding: 8px 10px;
}

.tool-card--dense .tool-header {
  gap: 0;
}

.tool-card--dense .tool-header-main {
  gap: 6px;
}

.tool-card--dense .tool-label,
.tool-card--dense .tool-status-text {
  font-size: 11px;
}

.tool-card--dense .tool-command {
  font-size: 10px;
  padding: 1px 5px;
}

.tool-card--dense .tool-section {
  border-radius: 7px;
  padding: 7px;
}

.tool-card--dense .tool-section-title {
  margin-bottom: 5px;
  font-size: 10px;
}

.tool-card--dense .tool-params,
.tool-card--dense .tool-preamble,
.tool-card--dense .tool-error,
.tool-card--dense .tool-output,
.tool-card--dense .tool-output-summary {
  font-size: 11px;
}

.tool-card--dense .tool-params,
.tool-card--dense .tool-output {
  padding: 8px;
}

.tool-card--dense .tool-actions {
  margin-top: 8px;
  gap: 6px;
}

.tool-card--dense .tool-action-btn {
  min-height: 28px;
  font-size: 11px;
  padding: 5px 10px;
}

.tool-section-title {
  margin: 0 0 6px 0;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.02em;
  color: var(--tool-summary-text);
  text-transform: uppercase;
}

.tool-params {
  margin: 0;
  background: #000000;
  color: var(--tool-detail-body-text);
  border: 1px solid var(--tool-section-border);
  border-radius: 7px;
  padding: 8px;
  font-size: 12px;
  line-height: 1.45;
  scrollbar-width: thin;
  max-height: 176px;
  overflow-y: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.tool-preamble {
  color: var(--tool-content-text);
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.tool-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
}

.tool-action-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 30px;
  padding: 6px 12px;
  border-radius: 8px;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: background-color 180ms ease, color 180ms ease, box-shadow 180ms ease, border-color 180ms ease;
}

.approve-btn {
  background: var(--tool-success-bg);
  border: 1px solid var(--tool-success-border);
  color: var(--tool-success-text);
}
.approve-btn:hover {
  background: var(--tool-success-bg-hover);
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.22);
}

.reject-btn {
  background: var(--tool-error-bg);
  border: 1px solid var(--tool-error-border);
  color: var(--tool-error-text);
}
.reject-btn:hover {
  background: var(--tool-error-bg-hover);
  box-shadow: 0 2px 8px rgba(239, 68, 68, 0.18);
}

.approve-btn:focus-visible,
.reject-btn:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 2px;
}

.tool-error {
  margin-top: 6px;
  margin-bottom: 0;
  color: var(--tool-error-text);
  font-size: 12px;
  line-height: 1.45;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.tool-result-block {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed var(--tool-section-border);
}

.tool-result-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  list-style: none;
  cursor: pointer;
}

.tool-result-summary::-webkit-details-marker {
  display: none;
}

.tool-result-summary--plain {
  cursor: default;
}

.tool-result-label {
  color: var(--tool-summary-text);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.02em;
  text-transform: uppercase;
}

.tool-output-summary {
  display: inline-flex;
  align-items: center;
  color: var(--tool-summary-text);
  background: var(--tool-summary-bg);
  border: 1px solid var(--tool-summary-border);
  border-radius: 6px;
  padding: 2px 8px;
  user-select: none;
  transition: color 150ms ease, border-color 150ms ease, background-color 150ms ease;
  font-size: 11px;
  font-weight: 600;
  line-height: 1.25;
}

.tool-result-summary:hover .tool-output-summary {
  color: var(--tool-content-text);
  border-color: var(--tool-card-border-hover);
}

.tool-output-details {
  margin: 0;
}

.tool-output {
  margin-top: 6px;
  padding: 8px;
  background: #000000;
  color: var(--tool-detail-body-text);
  border: 1px solid var(--tool-section-border);
  border-radius: 7px;
  font-size: 12px;
  line-height: 1.45;
  max-height: 224px;
  overflow-y: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.tool-result-summary:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 2px;
  border-radius: 6px;
}

@keyframes tool-dot-pulse {
  0%, 100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.6;
    transform: scale(0.92);
  }
}

@media (prefers-reduced-motion: reduce) {
  .status-dot-pulse {
    animation: none;
  }

  .tool-card {
    transition: none;
  }
}

@media (max-width: 640px) {
  .tool-card {
    padding: 8px 10px;
  }

  .tool-header-main {
    align-items: flex-start;
    gap: 8px;
  }

  .tool-meta {
    max-width: calc(100% - 80px);
  }

  .tool-command {
    max-width: 42%;
  }

  .tool-actions {
    flex-wrap: wrap;
  }

  .tool-action-btn {
    flex: 1 1 auto;
  }
}
</style>
