<script setup lang="ts">
import { ref, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { mdiChevronDown, mdiConsoleLine } from '@mdi/js'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import { scheduleAPI, type TaskRunStatus } from '@/api/schedule'
import type { SessionHistoryPayload } from '@/api/chat'
import { renderMarkdown } from '@/utils/markdown'

const props = defineProps<{ runId: string; runStatus: TaskRunStatus }>()
const { t } = useI18n()
const expanded = ref(false)
const history = ref<SessionHistoryPayload | null>(null)
const loading = ref(false)
const failed = ref(false)
let request: AbortController | undefined

async function load(older = false) {
  if (loading.value) return
  request?.abort()
  const controller = new AbortController()
  request = controller
  loading.value = true
  failed.value = false
  const first = older ? history.value?.messages[0] : undefined
  try {
    const page = await scheduleAPI.history(props.runId, first?.createdAt, first?.seq, controller.signal)
    if (controller.signal.aborted) return
    if (older && history.value) {
      const seen = new Set(history.value.messages.map(message => message.id))
      history.value = {
        ...page,
        messages: [...page.messages.filter(message => !seen.has(message.id)), ...history.value.messages],
        toolCallsByAssistantMessageId: { ...page.toolCallsByAssistantMessageId, ...history.value.toolCallsByAssistantMessageId },
        thinkingByAssistantMessageId: { ...page.thinkingByAssistantMessageId, ...history.value.thinkingByAssistantMessageId },
      }
    } else history.value = page
  } catch { if (!controller.signal.aborted) failed.value = true }
  finally { if (!controller.signal.aborted) loading.value = false }
}
function toggle() {
  expanded.value = !expanded.value
  if (expanded.value && !history.value) void load()
}
watch(() => props.runId, () => { request?.abort(); history.value = null; loading.value = false; failed.value = false; expanded.value = false })
watch(() => props.runStatus, status => { if (status !== 'running' && expanded.value) void load() })
onUnmounted(() => request?.abort())
</script>

<template>
  <section class="run-timeline">
    <button type="button" class="run-timeline-toggle" :aria-expanded="expanded" @click="toggle">
      <MdiIcon :path="mdiConsoleLine" :size="17" /><span>{{ t('scheduleExecutionTimeline') }}</span><MdiIcon :path="mdiChevronDown" :size="16" :class="{ rotated: expanded }" />
    </button>
    <div v-if="expanded" class="run-timeline-body">
      <div v-if="failed" class="run-timeline-error" role="alert"><span>{{ t('scheduleTimelineFailed') }}</span><button type="button" @click="load()">{{ t('scheduleRetry') }}</button></div>
      <button v-if="history?.hasMore" type="button" class="run-timeline-more" :disabled="loading" @click="load(true)">{{ t('scheduleEarlierSteps') }}</button>
      <article v-for="message in history?.messages ?? []" :key="message.id" class="run-step">
        <h4>{{ t(message.role === 'user' ? 'scheduleExecutionInput' : 'scheduleExecutionResponse') }}</h4>
        <div v-if="message.role === 'assistant'" class="run-step-tools">
          <details v-for="thought in history?.thinkingByAssistantMessageId?.[message.id] ?? []" :key="thought.thinkingId" class="run-step-detail">
            <summary>{{ t('scheduleThinking') }}</summary><div class="bubble-markdown" v-html="renderMarkdown(thought.content)" />
          </details>
          <details v-for="tool in history?.toolCallsByAssistantMessageId?.[message.id] ?? []" :key="tool.toolCallId" class="run-step-detail">
            <summary><span>{{ tool.toolName }} · {{ tool.command }}</span><span :class="{ 'run-tool-error': tool.status === 'error' }">{{ t(`toolCall${tool.status.charAt(0).toUpperCase()}${tool.status.slice(1)}`) }}</span></summary>
            <h5>{{ t('scheduleToolInput') }}</h5><pre>{{ JSON.stringify(tool.params, null, 2) }}</pre>
            <template v-if="tool.output || tool.error"><h5>{{ t('scheduleToolOutput') }}</h5><pre>{{ tool.error || tool.output }}</pre></template>
          </details>
        </div>
        <div v-if="message.content" class="bubble-markdown" v-html="renderMarkdown(message.content)" />
      </article>
      <div v-if="loading" class="run-timeline-loading" role="status"><LoadingSpinner size-class="w-4 h-4" />{{ t('scheduleLoading') }}</div>
      <p v-else-if="history && !history.messages.length">{{ t(runStatus === 'running' ? 'scheduleRunningHint' : 'scheduleNoSteps') }}</p>
    </div>
  </section>
</template>

<style scoped>
.run-timeline { margin-top: 24px; border-top: 1px solid var(--card-border); padding-top: 15px; color: var(--text-secondary); }
.run-timeline-toggle { display: flex; align-items: center; gap: 8px; width: 100%; padding: 8px 0; text-align: left; cursor: pointer; font-size: 12px; font-weight: 550; }
.run-timeline-toggle > span { flex: 1; }
.rotated { transform: rotate(180deg); }
.run-timeline-body { font-size: 12px; line-height: 1.8; padding-top: 12px; }
.run-step { border-left: 2px solid var(--primary-alpha-20); padding: 0 0 16px 14px; margin: 10px 0 15px; min-width: 0; }
.run-step h4 { color: var(--text-primary); margin: 0 0 10px; font-size: 12px; }
.run-step-tools { display: flex; flex-direction: column; gap: 8px; margin-bottom: 10px; }
.run-step-detail { border: 1px solid var(--card-border); border-radius: 8px; padding: 10px; background: var(--primary-alpha-04); }
summary { cursor: pointer; overflow-wrap: anywhere; }
summary > span:last-child { color: var(--schedule-muted); float: right; margin-left: 8px; }
summary > span.run-tool-error { color: var(--schedule-danger); }
.run-step-detail[open] summary { padding-bottom: 12px; }
.run-step-detail h5 { font-size: 11px; color: var(--schedule-muted); margin: 8px 0; }
pre { max-width: 100%; max-height: 320px; overflow: auto; padding: 10px; border-radius: 6px; background: var(--input-bg); font-family: var(--font-mono); font-size: 11px; white-space: pre-wrap; overflow-wrap: anywhere; }
.run-timeline-loading, .run-timeline-error { display: flex; align-items: center; gap: 10px; padding: 12px 0; }
.run-timeline-error { color: var(--schedule-danger); justify-content: space-between; }
.run-timeline-error button, .run-timeline-more { cursor: pointer; color: var(--sb-brand); }
.run-timeline-more { display: block; width: 100%; margin-bottom: 14px; text-align: center; }
button:focus-visible, summary:focus-visible { outline: 2px solid var(--sb-brand); outline-offset: 3px; }
</style>
