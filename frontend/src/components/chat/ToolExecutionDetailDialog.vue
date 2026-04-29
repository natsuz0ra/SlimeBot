<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallItem } from '@/api/chat'
import type { ToolTimelineEntry } from '@/types/chat'
import AppDialog from '@/components/ui/AppDialog.vue'
import ToolCallCard from '@/components/chat/ToolCallCard.vue'

const props = withDefaults(defineProps<{
  visible: boolean
  width?: string
  items: ToolCallItem[]
  toolTimeline: ToolTimelineEntry[]
}>(), {
  width: 'min(688px, calc(100vw - 36px))',
})

const emit = defineEmits<{
  'update:visible': [value: boolean]
  approve: [toolCallId: string]
  reject: [toolCallId: string]
}>()

const { t } = useI18n()

const orderedToolCalls = computed(() => {
  return props.toolTimeline
    .filter((entry) => entry.kind === 'tool_start')
    .map((entry) => props.items.find((toolCall) => toolCall.toolCallId === entry.toolCallId))
    .filter((toolCall): toolCall is ToolCallItem => !!toolCall)
    .filter((item) => !item.parentToolCallId)
})

function nestedForParent(parentId: string) {
  return props.items.filter((tc) => tc.parentToolCallId === parentId)
}

const totalCount = computed(() => orderedToolCalls.value.length)
const inProgressCount = computed(() => {
  return orderedToolCalls.value.filter((item) => item.status === 'pending' || item.status === 'executing').length
})
const doneCount = computed(() => orderedToolCalls.value.filter((item) => item.status === 'completed').length)
const failedCount = computed(() => orderedToolCalls.value.filter((item) => item.status === 'error' || item.status === 'rejected').length)

function closeDialog() {
  emit('update:visible', false)
}
</script>

<template>
  <AppDialog
    :visible="visible"
    :title="t('toolExecutionDetailTitle')"
    :cancel-text="t('close')"
    :width="width"
    compact
    large-title
    hide-footer
    @update:visible="emit('update:visible', $event)"
  >
    <section class="tool-detail-shell" aria-live="polite">
      <header class="tool-detail-summary" :aria-label="t('toolExecutionDetailTitle')">
        <div class="tool-summary-item">
          <span class="tool-summary-label">{{ t('toolExecutionCount', { count: totalCount }) }}</span>
        </div>
        <div v-if="inProgressCount > 0" class="tool-summary-item tool-summary-item-running">
          <span class="tool-summary-label">{{ t('toolExecutionInProgress', { count: inProgressCount }) }}</span>
        </div>
        <div v-if="doneCount > 0" class="tool-summary-item tool-summary-item-success">
          <span class="tool-summary-label">{{ t('toolExecutionDoneCount', { count: doneCount }) }}</span>
        </div>
        <div v-if="failedCount > 0" class="tool-summary-item tool-summary-item-failed">
          <span class="tool-summary-label">{{ t('toolExecutionFailedCount', { count: failedCount }) }}</span>
        </div>
      </header>

      <div v-if="orderedToolCalls.length > 0" class="tool-detail-list sb-scrollbar" role="list">
        <ToolCallCard
          v-for="item in orderedToolCalls"
          :key="item.toolCallId"
          :item="item"
          :nested-tools="nestedForParent(item.toolCallId)"
          :show-preamble="true"
          :dense="true"
          role="listitem"
          @approve="emit('approve', $event)"
          @reject="emit('reject', $event)"
        />
      </div>
      <p v-else class="tool-detail-empty sb-text-secondary text-sm">
        {{ t('toolExecutionEmpty') }}
      </p>
    </section>

    <div class="tool-detail-footer flex justify-end gap-2 mt-2 pt-2">
      <button
        type="button"
        class="px-4 py-2 text-sm rounded-xl transition-all duration-150 cursor-pointer dialog-cancel-btn"
        @click="closeDialog"
      >
        {{ t('close') }}
      </button>
    </div>
  </AppDialog>
</template>

<style scoped>
.tool-detail-shell {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.tool-detail-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.tool-summary-item {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  border: 1px solid var(--tool-summary-border);
  background: var(--tool-summary-bg);
  padding: 6px 12px;
}

.tool-summary-label {
  color: var(--tool-summary-text);
  font-size: 14px;
  font-weight: 600;
  line-height: 1;
}

.tool-summary-item-running {
  border-color: var(--tool-running-border);
  background: var(--tool-running-bg);
}

.tool-summary-item-running .tool-summary-label {
  color: var(--tool-running-text);
}

.tool-summary-item-success {
  border-color: var(--tool-success-border);
  background: var(--tool-success-bg);
}

.tool-summary-item-success .tool-summary-label {
  color: var(--tool-success-text);
}

.tool-summary-item-failed {
  border-color: var(--tool-error-border);
  background: var(--tool-error-bg);
}

.tool-summary-item-failed .tool-summary-label {
  color: var(--tool-error-text);
}

.tool-detail-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-height: 0;
  max-height: min(58vh, 560px);
  overflow-y: auto;
  overflow-x: hidden;
  padding: 1px 1px 4px 0;
}

.tool-detail-empty {
  border: 1px dashed var(--card-border);
  border-radius: 10px;
  padding: 8px 10px;
}

@media (max-width: 640px) {
  .tool-detail-shell {
    gap: 5px;
  }

  .tool-summary-item {
    padding: 5px 10px;
  }

  .tool-summary-label {
    font-size: 13px;
  }

  .tool-detail-list {
    gap: 5px;
    padding-bottom: 3px;
  }
}
</style>
