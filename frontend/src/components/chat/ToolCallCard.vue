<script setup lang="ts">
import { computed } from 'vue'
import { mdiCheck, mdiClose, mdiConsoleLine, mdiWeb } from '@mdi/js'
import { useI18n } from 'vue-i18n'
import MdiIcon from '@/components/MdiIcon.vue'
import type { ToolCallItem } from '@/api/chat'

const props = defineProps<{
  item: ToolCallItem & { preamble?: string }
  showPreamble?: boolean
}>()

const emit = defineEmits<{
  approve: [toolCallId: string]
  reject: [toolCallId: string]
}>()

const { t } = useI18n()

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
    case 'pending': return 'status-text-pending'
    case 'executing': return 'status-text-executing'
    case 'completed': return 'status-text-success'
    case 'rejected':
    case 'error': return 'status-text-error'
    default: return 'status-text-default'
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
</script>

<template>
  <div class="tool-card w-full rounded-xl px-4 py-3 text-sm overflow-hidden">
    <!-- 头部 -->
    <div class="flex items-center flex-wrap gap-2">
      <!-- 工具图标 + 名称 -->
      <div class="flex items-center gap-1.5">
        <MdiIcon :path="toolIcon" :size="14" class="tool-icon flex-shrink-0" />
        <span class="tool-label font-medium text-xs">{{ toolLabel }}</span>
      </div>

      <!-- 命令 -->
      <code v-if="item.command" class="tool-command text-xs font-mono break-all flex-1 min-w-0 truncate">{{ item.command }}</code>

      <!-- 状态 badge -->
      <div class="ml-auto flex items-center gap-1.5 flex-shrink-0">
        <span
          class="w-1.5 h-1.5 rounded-full flex-shrink-0"
          :class="[statusDotClass, item.status === 'executing' ? 'animate-pulse' : '']"
        />
        <span class="text-xs font-medium" :class="statusTextClass">{{ statusLabel }}</span>
        <svg
          v-if="item.status === 'executing'"
          class="animate-spin-icon w-3 h-3 flex-shrink-0"
          :class="statusTextClass"
          fill="none"
          viewBox="0 0 24 24"
        >
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
        </svg>
      </div>
    </div>

    <!-- 参数 -->
    <div v-if="paramsDisplay" class="mt-2.5">
      <pre class="tool-params text-xs font-mono rounded-lg px-3 py-2.5 whitespace-pre-wrap break-all leading-relaxed">{{ paramsDisplay }}</pre>
    </div>

    <!-- 前言 -->
    <div v-if="shouldShowPreamble" class="mt-2 text-xs leading-relaxed whitespace-pre-wrap break-words tool-preamble">
      {{ item.preamble }}
    </div>

    <!-- 操作按钮（待审批） -->
    <div v-if="showActions" class="flex items-center gap-2 mt-3">
      <button
        type="button"
        class="approve-btn flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg transition-all duration-150 cursor-pointer"
        @click="emit('approve', item.toolCallId)"
      >
        <MdiIcon :path="mdiCheck" :size="11" />
        {{ t('toolCallApprove') }}
      </button>
      <button
        type="button"
        class="reject-btn flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg transition-all duration-150 cursor-pointer"
        @click="emit('reject', item.toolCallId)"
      >
        <MdiIcon :path="mdiClose" :size="11" />
        {{ t('toolCallReject') }}
      </button>
    </div>

    <!-- 执行结果 -->
    <div v-if="showResult" class="mt-2.5">
      <div v-if="item.error" class="text-xs mb-1.5 tool-error">{{ item.error }}</div>
      <details v-if="item.output" class="text-xs">
        <summary class="tool-output-summary cursor-pointer select-none transition-colors duration-150 text-xs font-medium">
          {{ t('toolCallOutput') }}
        </summary>
        <pre class="mt-2 px-3 py-2.5 tool-output rounded-lg text-xs font-mono whitespace-pre-wrap break-all leading-relaxed">{{ item.output }}</pre>
      </details>
    </div>
  </div>
</template>

<style scoped>
.tool-card {
  background: var(--card-bg);
  border: 1px solid var(--card-border);
  backdrop-filter: blur(8px);
}

.tool-icon {
  color: #6366f1;
}

.tool-label {
  color: #6366f1;
}

.tool-command {
  color: var(--text-muted);
}

/* Status dots */
.status-dot-pending { background: #f59e0b; }
.status-dot-executing { background: #6366f1; }
.status-dot-success { background: #10b981; }
.status-dot-error { background: #ef4444; }
.status-dot-default { background: var(--text-muted); }

/* Status text */
.status-text-pending { color: #f59e0b; }
.status-text-executing { color: #6366f1; }
.status-text-success { color: #10b981; }
.status-text-error { color: #ef4444; }
.status-text-default { color: var(--text-muted); }

/* Params block */
.tool-params {
  background: rgba(0, 0, 0, 0.15);
  color: var(--text-secondary);
  border: 1px solid var(--card-border);
}

.dark .tool-params {
  background: rgba(0, 0, 0, 0.3);
}

.tool-preamble {
  color: var(--text-secondary);
}

/* Approve / reject buttons */
.approve-btn {
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.3);
  color: #10b981;
}
.approve-btn:hover {
  background: rgba(16, 185, 129, 0.18);
}

.reject-btn {
  background: rgba(239, 68, 68, 0.08);
  border: 1px solid rgba(239, 68, 68, 0.25);
  color: #ef4444;
}
.reject-btn:hover {
  background: rgba(239, 68, 68, 0.14);
}

/* Output */
.tool-error {
  color: #ef4444;
}

.tool-output-summary {
  color: var(--text-muted);
}
.tool-output-summary:hover {
  color: var(--text-secondary);
}

.tool-output {
  background: rgba(0, 0, 0, 0.25);
  color: #c4b5fd;
  border: 1px solid rgba(99, 102, 241, 0.15);
}
</style>
