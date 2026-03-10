<script setup lang="ts">
import { computed } from 'vue'
import { mdiCheck, mdiClose, mdiConsoleLine, mdiLoading, mdiWeb } from '@mdi/js'
import { useI18n } from 'vue-i18n'
import MdiIcon from './MdiIcon.vue'
import type { ToolCallItem } from '../api'

const props = defineProps<{
  item: ToolCallItem
}>()

const emit = defineEmits<{
  approve: [toolCallId: string]
  reject: [toolCallId: string]
}>()

const { t } = useI18n()

const toolIcon = computed(() => {
  if (props.item.toolName === 'exec') return mdiConsoleLine
  if (props.item.toolName === 'http_request') return mdiWeb
  return mdiConsoleLine
})

const toolLabel = computed(() => {
  if (props.item.toolName === 'exec') return t('toolExec')
  if (props.item.toolName === 'http_request') return t('toolHttpRequest')
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

const paramsDisplay = computed(() => {
  const entries = Object.entries(props.item.params)
  if (entries.length === 0) return ''
  return entries.map(([k, v]) => `${k}: ${v}`).join('\n')
})

const showActions = computed(() => props.item.status === 'pending')
const showResult = computed(() => props.item.status === 'completed' || props.item.status === 'error')
</script>

<template>
  <div class="tool-call-card" :class="item.status">
    <div class="tool-call-header">
      <MdiIcon :path="toolIcon" :size="16" />
      <span class="tool-name">{{ toolLabel }}</span>
      <span class="tool-command">{{ item.command }}</span>
      <span class="tool-status" :class="item.status">{{ statusLabel }}</span>
      <MdiIcon v-if="item.status === 'executing'" :path="mdiLoading" :size="14" class="spin" />
    </div>

    <div v-if="paramsDisplay" class="tool-call-params">
      <pre>{{ paramsDisplay }}</pre>
    </div>

    <div v-if="showActions" class="tool-call-actions">
      <button class="action-btn approve" @click="emit('approve', item.toolCallId)">
        <MdiIcon :path="mdiCheck" :size="14" />
        <span>{{ t('toolCallApprove') }}</span>
      </button>
      <button class="action-btn reject" @click="emit('reject', item.toolCallId)">
        <MdiIcon :path="mdiClose" :size="14" />
        <span>{{ t('toolCallReject') }}</span>
      </button>
    </div>

    <div v-if="showResult" class="tool-call-result">
      <div v-if="item.error" class="result-error">{{ item.error }}</div>
      <details v-if="item.output">
        <summary>{{ t('toolCallOutput') }}</summary>
        <pre class="result-output">{{ item.output }}</pre>
      </details>
    </div>
  </div>
</template>

<style scoped>
.tool-call-card {
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  padding: 10px 12px;
  margin: 8px 0;
  background: #fafafa;
  font-size: 13px;
  max-width: min(100%, 640px);
}

.tool-call-card.pending {
  border-color: #f0c040;
  background: #fffdf5;
}

.tool-call-card.executing {
  border-color: #7dc8f4;
  background: #f5fbff;
}

.tool-call-card.completed {
  border-color: #68c76a;
  background: #f5fff5;
}

.tool-call-card.error,
.tool-call-card.rejected {
  border-color: #e0a0a0;
  background: #fff8f8;
}

.tool-call-header {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 500;
  color: #333;
}

.tool-name {
  color: #1a73e8;
}

.tool-command {
  color: #666;
  font-family: 'Consolas', 'Courier New', monospace;
}

.tool-status {
  margin-left: auto;
  font-size: 12px;
  font-weight: 400;
}

.tool-status.pending { color: #c78112; }
.tool-status.executing { color: #1a73e8; }
.tool-status.completed { color: #2e8b2e; }
.tool-status.error, .tool-status.rejected { color: #d54941; }

.spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.tool-call-params {
  margin-top: 6px;
}

.tool-call-params pre {
  margin: 0;
  padding: 6px 8px;
  background: #f0f0f0;
  border-radius: 4px;
  font-size: 12px;
  font-family: 'Consolas', 'Courier New', monospace;
  white-space: pre-wrap;
  word-break: break-all;
  color: #333;
}

.tool-call-actions {
  margin-top: 8px;
  display: flex;
  gap: 8px;
}

.action-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 12px;
  border: 1px solid #d0d0d0;
  border-radius: 4px;
  font-size: 12px;
  cursor: pointer;
  background: #fff;
  color: #333;
  transition: all 0.15s ease;
}

.action-btn.approve {
  border-color: #68c76a;
  color: #2e8b2e;
}

.action-btn.approve:hover {
  background: #e8f8e8;
}

.action-btn.reject {
  border-color: #e0a0a0;
  color: #d54941;
}

.action-btn.reject:hover {
  background: #fff0f0;
}

.tool-call-result {
  margin-top: 6px;
}

.result-error {
  color: #d54941;
  font-size: 12px;
  margin-bottom: 4px;
}

.tool-call-result details {
  font-size: 12px;
}

.tool-call-result summary {
  cursor: pointer;
  color: #666;
  user-select: none;
}

.result-output {
  margin: 4px 0 0;
  padding: 6px 8px;
  background: #1f2329;
  color: #e6edf3;
  border-radius: 4px;
  font-size: 12px;
  font-family: 'Consolas', 'Courier New', monospace;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 200px;
  overflow-y: auto;
}
</style>
