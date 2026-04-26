<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { mdiConsoleLine, mdiHelpCircleOutline, mdiWeb } from '@mdi/js'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import type { ToolCallItem } from '@/api/chat'
import { buildToolResultDisplay, formatDisplayText, formatToolParams, parseAskQuestionsReadableAnswers } from '@/utils/toolDisplay'
import { shouldAutoExpandToolCall } from '@/utils/toolApprovalExpansion'

const props = withDefaults(defineProps<{
  item: ToolCallItem
  nestedTools?: ToolCallItem[]
}>(), {
  nestedTools: () => [],
})

const emit = defineEmits<{
  approve: [toolCallId: string]
  reject: [toolCallId: string]
}>()

const { t } = useI18n()
const expanded = ref(false)

const toolIcon = computed(() => {
  if (props.item.toolName === 'exec') return mdiConsoleLine
  if (props.item.toolName === 'http_request') return mdiWeb
  if (props.item.toolName === 'web_search') return mdiWeb
  if (props.item.toolName === 'ask_questions') return mdiHelpCircleOutline
  return mdiConsoleLine
})

const toolLabel = computed(() => {
  if (props.item.toolName === 'exec') return t('toolExec')
  if (props.item.toolName === 'http_request') return t('toolHttpRequest')
  if (props.item.toolName === 'web_search') return t('toolWebSearch')
  if (props.item.toolName === 'run_subagent') return t('toolRunSubagent')
  if (props.item.toolName === 'search_memory') return t('toolSearchMemory')
  if (props.item.toolName === 'ask_questions') return t('toolAskQuestions')
  return props.item.toolName
})

const commandDisplay = computed(() => {
  if (!props.item.command) return ''
  return `${props.item.toolName}.${props.item.command}()`
})

const statusIcon = computed(() => {
  switch (props.item.status) {
    case 'completed':
      return { symbol: '\u2713', class: 'inline-status--success' }
    case 'error':
    case 'rejected':
      return { symbol: '\u2717', class: 'inline-status--error' }
    case 'executing':
      return { symbol: '\u27F3', class: 'inline-status--executing' }
    case 'pending':
      return { symbol: '\u23F3', class: 'inline-status--pending' }
    default:
      return { symbol: '', class: '' }
  }
})

const showPendingLabel = computed(() => props.item.status === 'pending')

const paramsDisplay = computed(() => formatToolParams(props.item.params))

const showResult = computed(() =>
  props.item.status === 'completed' || props.item.status === 'error',
)

const resultDisplay = computed(() =>
  buildToolResultDisplay(props.item.toolName, props.item.command, props.item.output),
)

const errorDisplay = computed(() =>
  props.item.error ? formatDisplayText(props.item.error) : '',
)

const showPreamble = computed(() => !!props.item.preamble && props.item.preamble.trim() !== '')

const showActions = computed(() => props.item.status === 'pending')
const showNested = computed(() => props.nestedTools.length > 0)
const shouldAutoExpand = computed(() => shouldAutoExpandToolCall(props.item, props.nestedTools))
const isAskQuestions = computed(() => props.item.toolName === 'ask_questions')

const askQuestionsData = computed(() => {
  if (!isAskQuestions.value) return null
  const readableAnswers = parseAskQuestionsReadableAnswers(props.item.output ?? '')
  if (readableAnswers) {
    return readableAnswers.map((a) => ({ question: a.question, answer: a.answer }))
  }
  return null
})

watch(
  shouldAutoExpand,
  (value) => {
    if (value) expanded.value = true
  },
  { immediate: true },
)

function toggleExpanded() {
  expanded.value = shouldAutoExpand.value ? true : !expanded.value
}
</script>

<template>
  <div>
    <div v-if="showPreamble" class="inline-tool-preamble">{{ item.preamble }}</div>
    <div class="inline-tool" :class="[`inline-tool--${item.status}`]">
      <button
        type="button"
        class="inline-tool-row"
        :aria-expanded="expanded"
        @click="toggleExpanded"
      >
        <MdiIcon :path="toolIcon" :size="14" class="inline-tool-icon" />

        <span class="inline-tool-name">{{ toolLabel }}</span>
      <code v-if="commandDisplay && !isAskQuestions" class="inline-tool-cmd">{{ commandDisplay }}</code>

      <span v-if="showPendingLabel" class="inline-tool-pending-label">
        {{ t('toolWaitingApproval') }}
      </span>

      <span class="inline-tool-status" :class="statusIcon.class">
        <span
          v-if="item.status === 'executing'"
          class="inline-spinner"
          aria-hidden="true"
        >{{ statusIcon.symbol }}</span>
        <template v-else>{{ statusIcon.symbol }}</template>
      </span>

      <svg
        class="inline-tool-chevron"
        :class="{ 'inline-tool-chevron--open': expanded }"
        viewBox="0 0 16 16"
        width="12"
        height="12"
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

    <Transition name="inline-expand">
      <div v-if="expanded" class="inline-tool-detail">
        <section v-if="!isAskQuestions && paramsDisplay.length > 0" class="inline-section">
          <p class="inline-section-title">{{ t('toolCallParams') }}</p>
          <div class="inline-kv-list sb-scrollbar">
            <div v-for="row in paramsDisplay" :key="row.key" class="inline-kv-row">
              <span class="inline-kv-key">{{ row.key }}</span>
              <pre class="inline-kv-value">{{ row.value }}</pre>
            </div>
          </div>
        </section>

        <section v-if="showResult" class="inline-section">
          <template v-if="isAskQuestions && askQuestionsData && askQuestionsData.length > 0">
            <div class="inline-qa-list">
              <div v-for="(qa, idx) in askQuestionsData" :key="idx" class="inline-qa-pair">
                <div class="inline-qa-q">{{ idx + 1 }}. {{ qa.question }}</div>
                <div v-if="qa.answer" class="inline-qa-a">{{ qa.answer }}</div>
                <div v-else class="inline-qa-a inline-qa-a--empty">{{ t('qaNotSelected') }}</div>
              </div>
            </div>
          </template>
          <template v-else>
            <p class="inline-section-title">{{ t('toolCallResult') }}</p>
            <div v-if="item.output" class="inline-output sb-scrollbar">
              <template v-if="resultDisplay.mode === 'exec' && resultDisplay.exec">
                <div class="inline-kv-grid">
                  <div :class="['inline-kv-pill', resultDisplay.exec.exit_code === 0 ? 'inline-kv-pill--ok' : 'inline-kv-pill--err']">
                    exit_code: {{ resultDisplay.exec.exit_code }}
                  </div>
                  <div class="inline-kv-pill">duration_ms: {{ resultDisplay.exec.duration_ms }}</div>
                </div>
                <pre v-if="resultDisplay.exec.stdout.trim()" class="inline-exec-pre">{{ formatDisplayText(resultDisplay.exec.stdout) }}</pre>
                <pre v-if="resultDisplay.exec.stderr.trim()" class="inline-exec-pre inline-exec-pre--stderr">{{ formatDisplayText(resultDisplay.exec.stderr) }}</pre>
              </template>
              <template v-else-if="resultDisplay.mode === 'web_search' && resultDisplay.webSearch">
                <div class="inline-kv-grid">
                  <div class="inline-kv-pill">query: {{ resultDisplay.webSearch.query }}</div>
                  <div class="inline-kv-pill">results: {{ resultDisplay.webSearch.results.length }}</div>
                </div>
                <div v-if="resultDisplay.webSearch.results.length > 0" class="inline-web-list">
                  <article
                    v-for="(result, idx) in resultDisplay.webSearch.results"
                    :key="`${result.url}-${idx}`"
                    class="inline-web-card"
                  >
                    <a
                      :href="result.url"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="inline-web-title"
                    >
                      {{ result.title || result.url }}
                    </a>
                    <p v-if="result.content.trim()" class="inline-web-content">{{ result.content }}</p>
                  </article>
                </div>
                <div v-else class="inline-exec-empty">(No results)</div>
              </template>
              <pre v-else class="inline-output-text">{{ resultDisplay.outputText }}</pre>
            </div>
          </template>
          <div v-if="item.error" class="inline-error">{{ errorDisplay }}</div>
        </section>

        <section v-if="showNested" class="inline-section">
          <p class="inline-section-title">{{ t('subagentNestedTitle') }}</p>
          <div class="inline-nested-list">
            <ToolCallInline
              v-for="nested in nestedTools"
              :key="nested.toolCallId"
              :item="nested"
              @approve="emit('approve', $event)"
              @reject="emit('reject', $event)"
            />
          </div>
        </section>

        <div v-if="showActions" class="inline-actions">
          <button
            type="button"
            class="inline-action-btn inline-action-btn--approve"
            @click.stop="emit('approve', item.toolCallId)"
          >
            {{ t('toolCallApprove') }}
          </button>
          <button
            type="button"
            class="inline-action-btn inline-action-btn--reject"
            @click.stop="emit('reject', item.toolCallId)"
          >
            {{ t('toolCallReject') }}
          </button>
        </div>
      </div>
    </Transition>
  </div>
  </div>
</template>

<style scoped>
.inline-tool {
  border-radius: 8px;
  border: 1px solid var(--tool-card-border, rgba(100, 116, 139, 0.15));
  background:
    linear-gradient(90deg, var(--tool-section-bg, rgba(99, 102, 241, 0.04)), transparent),
    var(--card-bg);
  overflow: hidden;
  transition: border-color 150ms ease, box-shadow 150ms ease;
}

.inline-tool:hover {
  border-color: var(--tool-card-border-hover, rgba(100, 116, 139, 0.3));
  box-shadow: var(--tool-card-shadow-hover, none);
}

.inline-tool-row {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-height: 34px;
  padding: 7px 10px;
  background: none;
  border: none;
  cursor: pointer;
  color: var(--tool-content-text, #64748b);
  font-size: 14px;
  font-weight: 500;
  line-height: 1;
  text-align: left;
  transition: background-color 150ms ease;
}

.inline-tool-row:hover {
  background: var(--tool-summary-bg, rgba(100, 116, 139, 0.06));
}

.inline-tool-row:focus-visible {
  outline: 2px solid var(--focus-ring, rgba(139, 92, 246, 0.5));
  outline-offset: -2px;
  border-radius: 8px;
}

.inline-tool-icon {
  color: var(--tool-meta-icon, #64748b);
  flex-shrink: 0;
}

.inline-tool-preamble {
  margin-bottom: 4px;
  color: var(--text-muted, #94a3b8);
  font-size: 13px;
  line-height: 1.4;
  font-style: italic;
}

.inline-tool-name {
  color: var(--tool-meta-text, #e2e8f0);
  font-weight: 600;
  white-space: nowrap;
}

.inline-tool-cmd {
  display: inline-block;
  max-width: 200px;
  color: var(--tool-command-text, #94a3b8);
  background: var(--tool-command-bg, rgba(30, 41, 59, 0.8));
  border: 1px solid var(--tool-command-border, rgba(100, 116, 139, 0.2));
  border-radius: 5px;
  padding: 1px 5px;
  font-size: 13px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.inline-tool-pending-label {
  color: var(--tool-pending-text, #facc15);
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
}

.inline-tool-status {
  margin-left: auto;
  font-size: 14px;
  flex-shrink: 0;
  line-height: 1;
}

.inline-status--success { color: var(--tool-success-dot, #10b981); }
.inline-status--error { color: var(--tool-error-dot, #ef4444); }
.inline-status--executing { color: var(--tool-running-dot, #3b82f6); }
.inline-status--pending { color: var(--tool-pending-dot, #facc15); }

.inline-spinner {
  display: inline-block;
  animation: inline-spin 1s linear infinite;
}

@keyframes inline-spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.inline-tool-chevron {
  flex-shrink: 0;
  color: var(--text-muted, #64748b);
  transition: transform 200ms ease;
}

.inline-tool-chevron--open {
  transform: rotate(180deg);
}

/* --- Expanded detail --- */

.inline-tool-detail {
  padding: 0 10px 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.inline-section {
  border: 1px solid var(--tool-section-border, rgba(100, 116, 139, 0.15));
  border-radius: 6px;
  padding: 6px 8px;
  background: var(--tool-section-bg, rgba(15, 23, 42, 0.5));
}

.inline-section-title {
  margin: 0 0 4px 0;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.03em;
  text-transform: uppercase;
  color: var(--tool-summary-text, #94a3b8);
}

.inline-kv-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 120px;
  overflow-y: auto;
  scrollbar-width: thin;
}

.inline-kv-row {
  display: flex;
  flex-direction: column;
  gap: 2px;
  border: 1px solid var(--tool-section-border, rgba(100, 116, 139, 0.1));
  border-radius: 5px;
  padding: 4px 6px;
  background: var(--tool-output-bg, rgba(0, 0, 0, 0.35));
}

.inline-kv-key {
  font-size: 13px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.02em;
  color: var(--tool-summary-text, #94a3b8);
}

.inline-kv-value {
  margin: 0;
  font-size: 14px;
  color: var(--tool-detail-body-text, #e2e8f0);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  line-height: 1.4;
}

.inline-output {
  max-height: 140px;
  overflow-y: auto;
  scrollbar-width: thin;
}

.inline-output-text {
  margin: 0;
  font-size: 14px;
  color: var(--tool-detail-body-text, #e2e8f0);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  line-height: 1.4;
}

.inline-kv-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 4px;
}

.inline-kv-pill {
  color: var(--tool-detail-body-text, #e2e8f0);
  background: var(--tool-summary-bg, rgba(0, 0, 0, 0.25));
  border: 1px solid var(--tool-section-border, rgba(100, 116, 139, 0.15));
  border-radius: 999px;
  padding: 1px 6px;
  font-size: 13px;
}

.inline-kv-pill--ok { border-color: rgba(16, 185, 129, 0.3); }
.inline-kv-pill--err { border-color: rgba(239, 68, 68, 0.3); }

.inline-exec-pre {
  margin: 4px 0 0;
  padding: 4px 6px;
  background: var(--tool-section-bg, rgba(0, 0, 0, 0.2));
  border: 1px solid var(--tool-section-border, rgba(100, 116, 139, 0.1));
  border-radius: 5px;
  font-size: 14px;
  color: var(--tool-detail-body-text, #e2e8f0);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  line-height: 1.4;
  border-left: 2px solid rgba(99, 102, 241, 0.4);
}

.inline-exec-pre--stderr {
  border-left-color: rgba(239, 68, 68, 0.4);
}

.inline-exec-empty {
  margin-top: 4px;
  color: var(--tool-detail-body-text, #e2e8f0);
  font-size: 14px;
  line-height: 1.4;
}

.inline-web-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 4px;
}

.inline-web-card {
  border: 1px solid var(--tool-section-border, rgba(100, 116, 139, 0.15));
  border-radius: 6px;
  background: var(--tool-output-bg, rgba(0, 0, 0, 0.35));
  padding: 5px 6px;
}

.inline-web-title {
  color: var(--tool-command-text, #4f46e5);
  font-size: 14px;
  font-weight: 600;
  line-height: 1.4;
  text-decoration: none;
  overflow-wrap: anywhere;
}

.inline-web-title:hover {
  text-decoration: underline;
}

.inline-web-content {
  margin: 3px 0 0;
  color: var(--tool-content-text, #2e2a64);
  font-size: 14px;
  line-height: 1.4;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.inline-error {
  margin-top: 4px;
  color: var(--tool-error-text, #ef4444);
  font-size: 14px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  line-height: 1.4;
}

.inline-qa-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.inline-qa-pair {
  padding: 4px 0;
  border-bottom: 1px solid var(--tool-section-border, rgba(100, 116, 139, 0.1));
}

.inline-qa-pair:last-child {
  border-bottom: none;
}

.inline-qa-q {
  font-size: 14px;
  color: var(--tool-summary-text, #94a3b8);
  line-height: 1.4;
  margin-bottom: 2px;
}

.inline-qa-a {
  font-size: 14px;
  font-weight: 600;
  color: var(--tool-detail-body-text, #e2e8f0);
  line-height: 1.4;
}

.inline-qa-a--empty {
  color: var(--text-muted, #94a3b8);
  font-weight: 400;
  font-style: italic;
}

.inline-nested-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.inline-nested-list :deep(.inline-tool) {
  border-color: var(--tool-section-border, rgba(100, 116, 139, 0.12));
}

/* --- Actions --- */

.inline-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  padding-top: 2px;
}

.inline-action-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 24px;
  padding: 3px 10px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: background-color 150ms ease, box-shadow 150ms ease;
}

.inline-action-btn:focus-visible {
  outline: 2px solid var(--focus-ring, rgba(139, 92, 246, 0.5));
  outline-offset: 2px;
}

.inline-action-btn--approve {
  background: var(--tool-success-bg, rgba(16, 185, 129, 0.12));
  border: 1px solid var(--tool-success-border, rgba(16, 185, 129, 0.25));
  color: var(--tool-success-text, #10b981);
}

.inline-action-btn--approve:hover {
  background: var(--tool-success-bg-hover, rgba(16, 185, 129, 0.2));
}

.inline-action-btn--reject {
  background: var(--tool-error-bg, rgba(239, 68, 68, 0.1));
  border: 1px solid var(--tool-error-border, rgba(239, 68, 68, 0.2));
  color: var(--tool-error-text, #ef4444);
}

.inline-action-btn--reject:hover {
  background: var(--tool-error-bg-hover, rgba(239, 68, 68, 0.18));
}

/* --- Transitions --- */

.inline-expand-enter-active {
  transition: opacity 180ms ease, max-height 250ms ease;
}

.inline-expand-leave-active {
  transition: opacity 120ms ease, max-height 180ms ease;
}

.inline-expand-enter-from,
.inline-expand-leave-to {
  opacity: 0;
  max-height: 0;
}

.inline-expand-enter-to,
.inline-expand-leave-from {
  opacity: 1;
  max-height: 500px;
}

@media (prefers-reduced-motion: reduce) {
  .inline-spinner {
    animation: none;
  }
  .inline-tool-chevron {
    transition: none;
  }
  .inline-expand-enter-active,
  .inline-expand-leave-active {
    transition: none;
  }
}
</style>
