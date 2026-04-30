<script setup lang="ts">
import { computed } from 'vue'
import type { ToolCallItem } from '@/api/chat'
import { buildFileToolDisplay } from '@/utils/fileToolDisplay'

const props = defineProps<{
  item: ToolCallItem
}>()

const display = computed(() => buildFileToolDisplay(props.item))
const summaryText = computed(() => {
  if (!display.value) return ''
  if (props.item.status === 'completed') return display.value.summary
  return [display.value.operation, display.value.filePath].filter(Boolean).join(' ')
})
const showDiff = computed(() => {
  return props.item.status === 'completed' && display.value && display.value.diffLines.length > 0
})
const errorText = computed(() => props.item.status === 'error' || props.item.status === 'rejected' ? (props.item.error || props.item.output || '') : '')

function lineNumber(line: { kind: string; oldLine?: number; newLine?: number }) {
  const value = line.kind === 'added' ? line.newLine : line.oldLine ?? line.newLine
  return value === undefined ? '' : String(value)
}

function marker(kind: string) {
  if (kind === 'added') return '+'
  if (kind === 'removed') return '-'
  return ''
}
</script>

<template>
  <section v-if="display" class="file-tool">
    <div class="file-tool-summary">
      <div class="file-tool-tree">└─</div>
      <div class="file-tool-summary-main">
        <div class="file-tool-action">{{ summaryText }}</div>
        <div v-if="display.filePath" class="file-tool-path">{{ display.filePath }}</div>
      </div>
    </div>

    <div v-if="showDiff" class="file-tool-diff sb-scrollbar" role="list">
      <div
        v-for="(line, index) in display.diffLines"
        :key="`${line.kind}-${index}-${line.oldLine ?? ''}-${line.newLine ?? ''}`"
        :class="['file-tool-diff-row', `file-tool-diff-row--${line.kind}`]"
        role="listitem"
      >
        <span class="file-tool-diff-guide">{{ index === display.diffLines.length - 1 ? '└─' : '├─' }}</span>
        <span class="file-tool-diff-marker">{{ marker(line.kind) }}</span>
        <span class="file-tool-diff-line">{{ lineNumber(line) }}</span>
        <code class="file-tool-diff-code">{{ line.text || ' ' }}</code>
      </div>
    </div>

    <div v-if="errorText" class="file-tool-error">
      {{ errorText }}
    </div>
  </section>
</template>

<style scoped>
.file-tool {
  display: flex;
  flex-direction: column;
  gap: 8px;
  border: 1px solid var(--tool-section-border);
  border-radius: 8px;
  background: var(--tool-section-bg);
  padding: 8px;
  overflow: hidden;
}

.file-tool-summary {
  display: flex;
  gap: 8px;
  min-width: 0;
}

.file-tool-tree {
  color: var(--tool-summary-text);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  line-height: 1.35;
  flex: 0 0 auto;
}

.file-tool-summary-main {
  min-width: 0;
}

.file-tool-action {
  color: var(--tool-content-text);
  font-size: 14px;
  font-weight: 700;
  line-height: 1.35;
  overflow-wrap: anywhere;
}

.file-tool-path {
  color: var(--text-muted);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  font-size: 12px;
  line-height: 1.35;
  overflow-wrap: anywhere;
}

.file-tool-diff {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-width: 100%;
  overflow-x: auto;
  padding-bottom: 2px;
}

.file-tool-diff-row {
  display: grid;
  grid-template-columns: 28px 18px 42px minmax(0, 1fr);
  align-items: start;
  min-width: max-content;
  border-radius: 5px;
  padding: 2px 8px 2px 0;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  font-size: 13px;
  line-height: 1.45;
  color: var(--tool-detail-body-text);
}

.file-tool-diff-row--added {
  background: #15803d;
  color: #ffffff;
}

.file-tool-diff-row--removed {
  background: #b91c1c;
  color: #ffffff;
}

.file-tool-diff-row--context {
  background: rgba(148, 163, 184, 0.12);
}

.file-tool-diff-guide,
.file-tool-diff-marker,
.file-tool-diff-line {
  color: inherit;
  opacity: 0.92;
  text-align: right;
  white-space: pre;
}

.file-tool-diff-code {
  color: inherit;
  white-space: pre;
  font: inherit;
}

.file-tool-error {
  border: 1px solid var(--tool-error-border);
  border-radius: 6px;
  background: var(--tool-error-bg);
  color: var(--tool-error-text);
  padding: 7px 8px;
  font-size: 13px;
  line-height: 1.45;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

@media (max-width: 640px) {
  .file-tool {
    padding: 7px;
  }

  .file-tool-diff-row {
    grid-template-columns: 24px 16px 36px minmax(0, 1fr);
    font-size: 12px;
  }
}
</style>
