<script setup lang="ts">
import { computed, ref } from 'vue'
import { mdiBrain, mdiCheck, mdiClose, mdiConsoleLine, mdiHelpCircleOutline, mdiSourceBranch, mdiWeb } from '@mdi/js'
import { useI18n } from 'vue-i18n'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import ThinkingBlock from '@/components/chat/ThinkingBlock.vue'
import type { ToolCallItem } from '@/api/chat'
import { buildSubagentTimeline } from '@/utils/subagentTimeline'
import { buildToolResultDisplay, formatDisplayText, formatToolParams, parseAskQuestionsReadableAnswers } from '@/utils/toolDisplay'

const props = withDefaults(defineProps<{
  item: ToolCallItem & { preamble?: string }
  showPreamble?: boolean
  dense?: boolean
  nestedTools?: ToolCallItem[]
}>(), {
  showPreamble: false,
  dense: false,
  nestedTools: () => [],
})

const emit = defineEmits<{
  approve: [toolCallId: string]
  reject: [toolCallId: string]
}>()

const { t } = useI18n()
const isOutputExpanded = ref(false)
const isCollapsed = ref(props.item.toolName === 'ask_questions')
const subagentTimelineExpanded = ref(false)

const toolIcon = computed(() => {
  if (props.item.toolName === 'run_subagent') return mdiSourceBranch
  if (props.item.toolName === 'exec') return mdiConsoleLine
  if (props.item.toolName === 'http_request') return mdiWeb
  if (props.item.toolName === 'web_search') return mdiWeb
  if (props.item.toolName === 'search_memory') return mdiBrain
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

const showSubagentStream = computed(() => {
  return props.item.toolName === 'run_subagent' && !!props.item.subagentStream && props.item.subagentStream.trim() !== ''
})
const subagentThinkingItems = computed(() => {
  if (props.item.toolName !== 'run_subagent') return []
  return props.item.subagentThinkings ?? (props.item.subagentThinking ? [props.item.subagentThinking] : [])
})
const subagentTimelineItems = computed(() => buildSubagentTimeline(subagentThinkingItems.value, props.nestedTools))
const showSubagentToolCallsThinking = computed(() => subagentTimelineItems.value.length > 0)

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
  return formatToolParams(props.item.params)
})
const runSubagentParamsDisplay = computed(() => {
  if (!isRunSubagent.value) return paramsDisplay.value
  const { context: _context, task: _task, ...rest } = props.item.params || {}
  return formatToolParams(rest)
})
const subagentContextSummary = computed(() => {
  if (!isRunSubagent.value) return ''
  return formatDisplayText(String(props.item.params?.context ?? '')).trim()
})
const subagentTaskSummary = computed(() => {
  if (!isRunSubagent.value) return ''
  return formatDisplayText(String(props.item.subagentTask || props.item.params?.task || '')).trim()
})
const showSubagentContext = computed(() => subagentContextSummary.value !== '')
const showSubagentTask = computed(() => subagentTaskSummary.value !== '')

const showActions = computed(() => props.item.status === 'pending' && !isAskQuestions.value)
const showResult = computed(() => props.item.status === 'completed' || props.item.status === 'error')
const resultDisplay = computed(() => buildToolResultDisplay(props.item.toolName, props.item.command, props.item.output))
const errorDisplay = computed(() => (props.item.error ? formatDisplayText(props.item.error) : ''))
const isRunSubagent = computed(() => props.item.toolName === 'run_subagent')
const showRunSubagentResult = computed(() => isRunSubagent.value && (showResult.value || showSubagentStream.value))
const execExitOk = computed(() => resultDisplay.value.mode === 'exec' && resultDisplay.value.exec && resultDisplay.value.exec.exit_code === 0)
const shouldShowPreamble = computed(() => !!props.showPreamble && !!props.item.preamble && !isRunSubagent.value)
const outputPanelId = computed(() => `tool-output-${props.item.toolCallId}`)
const isAskQuestions = computed(() => props.item.toolName === 'ask_questions')

const askQuestionsData = computed(() => {
  if (!isAskQuestions.value) return null
  const readableAnswers = parseAskQuestionsReadableAnswers(props.item.output ?? '')
  if (readableAnswers) {
    return readableAnswers.map((a) => ({ question: a.question, answer: a.answer }))
  }
  return null
})

function toggleCollapse() {
  if (isAskQuestions.value) isCollapsed.value = !isCollapsed.value
  if (isCollapsed.value) subagentTimelineExpanded.value = false
}

function onOutputToggle(event: Event) {
  const target = event.currentTarget as HTMLDetailsElement | null
  isOutputExpanded.value = !!target?.open
}

function toggleSubagentTimeline() {
  subagentTimelineExpanded.value = !subagentTimelineExpanded.value
}
</script>

<template>
  <article :class="['tool-card w-full rounded-xl text-base', { 'tool-card--dense': dense, 'tool-card--collapsed': isCollapsed }]">
    <header :class="['tool-header', { 'tool-header--clickable': isAskQuestions }]" @click="toggleCollapse">
      <div class="tool-header-main">
        <div class="tool-meta">
          <MdiIcon :path="toolIcon" :size="16" class="tool-icon flex-shrink-0" />
          <span class="tool-label">{{ toolLabel }}</span>
          <code
            v-if="item.command && !isAskQuestions"
            class="tool-command"
            :title="item.command"
          >
            {{ item.command }}
          </code>
          <span v-if="isAskQuestions && askQuestionsData" class="tool-qa-count">
            {{ askQuestionsData.length }} {{ t('qaTitle') }}
          </span>
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
          <svg
            v-if="isAskQuestions"
            class="tool-collapse-arrow"
            :class="{ 'tool-collapse-arrow--open': !isCollapsed }"
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
        </div>
      </div>
    </header>

    <!-- ask_questions: custom Q&A result section (hidden when collapsed) -->
    <template v-if="isAskQuestions && !isCollapsed">
      <section v-if="askQuestionsData && askQuestionsData.length > 0" class="tool-qa-list mt-2">
        <div v-for="(qa, idx) in askQuestionsData" :key="idx" class="tool-qa-pair">
          <div class="tool-qa-q">{{ idx + 1 }}. {{ qa.question }}</div>
          <div v-if="qa.answer" class="tool-qa-a">{{ qa.answer }}</div>
          <div v-else class="tool-qa-a tool-qa-a--empty">{{ t('qaNotSelected') }}</div>
        </div>
      </section>
      <section v-else-if="showResult && item.error" class="tool-section mt-2">
        <div class="tool-error">{{ errorDisplay }}</div>
      </section>
    </template>

    <section v-if="showSubagentContext" class="tool-section mt-2.5">
      <p class="tool-section-title">{{ t('subagentContextLabel') }}</p>
      <pre class="tool-params sb-scrollbar">{{ subagentContextSummary }}</pre>
    </section>

    <section v-if="showSubagentTask" class="tool-section mt-2.5">
      <p class="tool-section-title">{{ t('subagentTaskLabel') }}</p>
      <pre class="tool-params sb-scrollbar">{{ subagentTaskSummary }}</pre>
    </section>

    <!-- non-ask_questions params section -->
    <section v-if="!isAskQuestions && runSubagentParamsDisplay.length > 0" class="tool-section mt-2">
      <p class="tool-section-title">{{ t('toolCallParams') }}</p>
      <div class="tool-kv-list sb-scrollbar">
        <div v-for="row in runSubagentParamsDisplay" :key="row.key" class="tool-kv-row">
          <div class="tool-kv-key">{{ row.key }}</div>
          <pre class="tool-kv-value">{{ row.value }}</pre>
        </div>
      </div>
    </section>

    <section v-if="!isRunSubagent && !isAskQuestions && showResult" class="tool-section mt-2">
      <details v-if="item.output" class="tool-output-details text-sm" @toggle="onOutputToggle">
        <summary
          class="tool-result-summary"
          :aria-expanded="isOutputExpanded ? 'true' : 'false'"
          :aria-controls="outputPanelId"
        >
          <span class="tool-result-label">{{ t('toolCallResult') }}</span>
          <span class="tool-output-summary">{{ t('toolCallOutput') }}</span>
          <svg
            class="tool-result-arrow"
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
        </summary>
        <div :id="outputPanelId" class="tool-output sb-scrollbar">
          <template v-if="resultDisplay.mode === 'exec' && resultDisplay.exec">
            <div class="tool-kv-grid">
              <div :class="['tool-kv-pill', execExitOk ? 'tool-kv-pill--ok' : 'tool-kv-pill--err']">exit_code: {{ resultDisplay.exec.exit_code }}</div>
              <div class="tool-kv-pill">timed_out: {{ resultDisplay.exec.timed_out }}</div>
              <div class="tool-kv-pill">truncated: {{ resultDisplay.exec.truncated }}</div>
              <div class="tool-kv-pill">duration_ms: {{ resultDisplay.exec.duration_ms }}</div>
              <div class="tool-kv-pill">shell: {{ resultDisplay.exec.shell }}</div>
            </div>
            <div v-if="resultDisplay.exec.stdout.trim() !== ''" class="tool-exec-block tool-exec-block--stdout">
              <p class="tool-exec-label">stdout</p>
              <pre class="tool-exec-pre">{{ formatDisplayText(resultDisplay.exec.stdout) }}</pre>
            </div>
            <div v-if="resultDisplay.exec.stderr.trim() !== ''" class="tool-exec-block tool-exec-block--stderr">
              <p class="tool-exec-label">stderr</p>
              <pre class="tool-exec-pre">{{ formatDisplayText(resultDisplay.exec.stderr) }}</pre>
            </div>
            <div v-if="resultDisplay.exec.stdout.trim() === '' && resultDisplay.exec.stderr.trim() === ''" class="tool-exec-empty">
              (No output)
            </div>
          </template>
          <pre v-else class="tool-output-pre">{{ resultDisplay.outputText }}</pre>
        </div>
      </details>

      <div v-else class="tool-result-summary tool-result-summary--plain" aria-live="polite">
        <span class="tool-result-label">{{ t('toolCallResult') }}</span>
      </div>

      <div v-if="item.error" class="tool-error">{{ errorDisplay }}</div>
    </section>

    <section v-if="shouldShowPreamble" class="tool-section mt-2.5">
      <p class="tool-section-title">{{ t('toolCallPreamble') }}</p>
      <div class="tool-preamble">
      {{ item.preamble }}
      </div>
    </section>

    <section
      v-if="showSubagentToolCallsThinking"
      class="tool-section mt-2.5 subagent-tool-calls-thinking-wrap"
    >
      <button
        type="button"
        class="tool-result-summary tool-result-summary--button"
        :aria-expanded="subagentTimelineExpanded ? 'true' : 'false'"
        @click="toggleSubagentTimeline"
      >
        <span class="tool-result-label">{{ t('subagentToolCallsThinkingTitle') }}</span>
        <span class="tool-output-summary">{{ subagentTimelineItems.length }}</span>
        <svg
          class="tool-result-arrow"
          :class="{ 'tool-result-arrow--open': subagentTimelineExpanded }"
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
      <Transition name="tool-subagent-expand">
        <div v-if="subagentTimelineExpanded" class="subagent-timeline-list">
          <template v-for="timelineItem in subagentTimelineItems" :key="timelineItem.id">
            <ThinkingBlock
              v-if="timelineItem.kind === 'thinking'"
              :content="timelineItem.thinking.content"
              :done="timelineItem.thinking.done"
              :duration-ms="timelineItem.thinking.durationMs"
              variant="subagent"
            />
            <ToolCallCard
              v-else
              :item="timelineItem.tool"
              :dense="true"
              @approve="emit('approve', $event)"
              @reject="emit('reject', $event)"
            />
          </template>
        </div>
      </Transition>
    </section>

    <section v-if="showRunSubagentResult" class="tool-section mt-2.5">
      <details v-if="item.output" class="tool-output-details text-sm" @toggle="onOutputToggle">
        <summary
          class="tool-result-summary"
          :aria-expanded="isOutputExpanded ? 'true' : 'false'"
          :aria-controls="outputPanelId"
        >
          <span class="tool-result-label">{{ t('subagentResultLabel') }}</span>
          <span class="tool-output-summary">{{ t('toolCallOutput') }}</span>
          <svg
            class="tool-result-arrow"
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
        </summary>
        <div :id="outputPanelId" class="tool-output sb-scrollbar">
          <template v-if="resultDisplay.mode === 'exec' && resultDisplay.exec">
            <div class="tool-kv-grid">
              <div :class="['tool-kv-pill', execExitOk ? 'tool-kv-pill--ok' : 'tool-kv-pill--err']">exit_code: {{ resultDisplay.exec.exit_code }}</div>
              <div class="tool-kv-pill">timed_out: {{ resultDisplay.exec.timed_out }}</div>
              <div class="tool-kv-pill">truncated: {{ resultDisplay.exec.truncated }}</div>
              <div class="tool-kv-pill">duration_ms: {{ resultDisplay.exec.duration_ms }}</div>
              <div class="tool-kv-pill">shell: {{ resultDisplay.exec.shell }}</div>
            </div>
            <div v-if="resultDisplay.exec.stdout.trim() !== ''" class="tool-exec-block tool-exec-block--stdout">
              <p class="tool-exec-label">stdout</p>
              <pre class="tool-exec-pre">{{ formatDisplayText(resultDisplay.exec.stdout) }}</pre>
            </div>
            <div v-if="resultDisplay.exec.stderr.trim() !== ''" class="tool-exec-block tool-exec-block--stderr">
              <p class="tool-exec-label">stderr</p>
              <pre class="tool-exec-pre">{{ formatDisplayText(resultDisplay.exec.stderr) }}</pre>
            </div>
            <div v-if="resultDisplay.exec.stdout.trim() === '' && resultDisplay.exec.stderr.trim() === ''" class="tool-exec-empty">
              (No output)
            </div>
          </template>
          <pre v-else class="tool-output-pre">{{ resultDisplay.outputText }}</pre>
        </div>
      </details>

      <template v-else-if="showSubagentStream">
        <p class="tool-section-title">{{ t('subagentResultLabel') }}</p>
        <pre class="tool-params sb-scrollbar">{{ item.subagentStream }}</pre>
      </template>

      <div v-else class="tool-result-summary tool-result-summary--plain" aria-live="polite">
        <span class="tool-result-label">{{ t('subagentResultLabel') }}</span>
      </div>

      <div v-if="item.error" class="tool-error">{{ errorDisplay }}</div>
    </section>

    <section v-if="showActions" class="tool-actions">
      <button
        type="button"
        class="tool-action-btn approve-btn"
        @click="emit('approve', item.toolCallId)"
      >
        <MdiIcon :path="mdiCheck" :size="12" />
        {{ t('toolCallApprove') }}
      </button>
      <button
        type="button"
        class="tool-action-btn reject-btn"
        @click="emit('reject', item.toolCallId)"
      >
        <MdiIcon :path="mdiClose" :size="12" />
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
  font-size: 14px;
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
  font-size: 13px;
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
  font-size: 14px;
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
  font-size: 14px;
}

.tool-card--dense .tool-command {
  font-size: 13px;
  padding: 1px 5px;
}

.tool-card--dense .tool-section {
  border-radius: 7px;
  padding: 7px;
}

.tool-card--dense .tool-section-title {
  margin-bottom: 5px;
  font-size: 13px;
}

.tool-card--dense .tool-params,
.tool-card--dense .tool-preamble,
.tool-card--dense .tool-error,
.tool-card--dense .tool-output,
.tool-card--dense .tool-output-summary {
  font-size: 14px;
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
  font-size: 13px;
  padding: 5px 10px;
}

.tool-section-title {
  margin: 0 0 6px 0;
  font-size: 13px;
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
  font-size: 14px;
  line-height: 1.45;
  scrollbar-width: thin;
  max-height: 176px;
  overflow-y: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.tool-kv-list {
  max-height: 176px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tool-kv-row {
  border: 1px solid var(--tool-section-border);
  border-radius: 7px;
  padding: 6px 8px;
  background: #000000;
}

.tool-kv-key {
  color: var(--tool-summary-text);
  font-size: 13px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.02em;
  margin-bottom: 4px;
}

.tool-kv-value {
  margin: 0;
  color: var(--tool-detail-body-text);
  font-size: 14px;
  line-height: 1.45;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.tool-preamble {
  color: var(--tool-content-text);
  font-size: 14px;
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
  font-size: 14px;
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
  font-size: 14px;
  line-height: 1.45;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.tool-result-arrow {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  font-size: 0;
  margin-left: auto;
  color: var(--tool-summary-text);
  transition: transform 150ms ease;
  flex-shrink: 0;
  transform: rotate(-90deg);
}

details[open] > .tool-result-summary .tool-result-arrow {
  transform: rotate(0deg);
}

.tool-result-summary {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 8px;
  min-height: 34px;
  list-style: none;
  cursor: pointer;
}

.tool-result-summary::-webkit-details-marker {
  display: none;
}

.tool-result-summary--button {
  width: 100%;
  padding: 0;
  border: none;
  background: none;
  text-align: left;
}

.tool-result-summary--plain {
  cursor: default;
}

.tool-result-label {
  color: var(--tool-summary-text);
  font-size: 13px;
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
  font-size: 13px;
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
  font-size: 14px;
  line-height: 1.45;
  max-height: 224px;
  overflow-y: auto;
}

.tool-output-pre {
  margin: 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.tool-kv-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.tool-kv-pill {
  color: var(--tool-detail-body-text);
  background: #111111;
  border: 1px solid var(--tool-section-border);
  border-radius: 999px;
  padding: 2px 8px;
  font-size: 13px;
  transition: color 150ms ease, border-color 150ms ease, background-color 150ms ease;
}

.tool-exec-block {
  margin-top: 8px;
}

.tool-exec-block--stdout {
  border-left: 3px solid rgba(99, 102, 241, 0.4);
  padding-left: 8px;
}

.tool-exec-block--stderr {
  border-left: 3px solid rgba(239, 68, 68, 0.4);
  padding-left: 8px;
}

.tool-exec-label {
  margin: 0 0 4px 0;
  color: var(--tool-summary-text);
  font-size: 13px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.02em;
}

.tool-exec-pre {
  margin: 0;
  padding: 8px;
  background: #111111;
  border: 1px solid var(--tool-section-border);
  border-radius: 7px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.tool-exec-empty {
  margin-top: 8px;
  color: var(--tool-detail-body-text);
  font-size: 14px;
}

.tool-result-summary:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 2px;
  border-radius: 6px;
}

.subagent-tool-calls-thinking-wrap {
  border-left: 3px solid var(--tool-running-border);
  padding-left: 12px;
  margin-left: 4px;
  border-radius: 0 10px 10px 0;
  background: var(--tool-section-bg);
}

.subagent-timeline-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 8px;
}

.subagent-timeline-list :deep(.tool-card) {
  border-color: var(--tool-section-border);
  box-shadow: none;
}

.tool-result-arrow--open {
  transform: rotate(0deg);
}

.tool-subagent-expand-enter-active {
  transition: opacity 180ms ease, max-height 250ms ease;
}

.tool-subagent-expand-leave-active {
  transition: opacity 120ms ease, max-height 180ms ease;
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

.tool-header--clickable {
  cursor: pointer;
  border-radius: inherit;
}

.tool-header--clickable:hover {
  opacity: 0.85;
}

.tool-collapse-arrow {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  font-size: 0;
  color: var(--tool-summary-text);
  transition: transform 150ms ease;
  flex-shrink: 0;
  transform: rotate(-90deg);
}

.tool-collapse-arrow--open {
  transform: rotate(0deg);
}

.tool-qa-count {
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--primary-alpha-08);
  padding: 1px 6px;
  border-radius: 6px;
  white-space: nowrap;
}

.tool-qa-list {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.tool-qa-pair {
  padding: 6px 0;
  border-bottom: 1px solid var(--tool-section-border);
}

.tool-qa-pair:last-child {
  border-bottom: none;
}

.tool-qa-q {
  font-size: 14px;
  color: var(--tool-summary-text);
  margin-bottom: 2px;
  line-height: 1.45;
}

.tool-qa-a {
  font-size: 14px;
  font-weight: 600;
  color: var(--tool-detail-body-text);
  line-height: 1.45;
}

.tool-qa-a--empty {
  color: var(--text-muted);
  font-weight: 400;
  font-style: italic;
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

  .tool-result-arrow,
  .tool-collapse-arrow,
  .tool-subagent-expand-enter-active,
  .tool-subagent-expand-leave-active {
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
