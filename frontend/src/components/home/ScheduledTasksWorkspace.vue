<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { mdiClockOutline, mdiChevronRight, mdiRefresh, mdiCalendarClockOutline, mdiPlus, mdiPlayOutline, mdiCheckCircleOutline, mdiAlertCircleOutline, mdiPauseCircleOutline } from '@mdi/js'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import { useToast } from '@/composables/useToast'
import AppTextInput from '@/components/ui/AppTextInput.vue'
import CooperativePanel from '@/components/chat/CooperativePanel.vue'
import TaskRunTimeline from '@/components/home/TaskRunTimeline.vue'
import { scheduleAPI, type CreateScheduledTaskInput, type ScheduledTask, type ScheduledTaskRun, type TaskRunStatus } from '@/api/schedule'
import { taskRunDurationSeconds, taskRunPreview } from '@/utils/scheduleHistory'
import { renderMarkdown } from '@/utils/markdown'

const props = defineProps<{ modelOptions: Array<{ value: string; label: string }>; initialWorkingDirectory?: string }>()
const toast = useToast()
const view = ref<'tasks' | 'history'>('tasks')
const createVisible = ref(false)
const creating = ref(false)
const taskActionId = ref('')
const taskForm = ref<HTMLFormElement | null>(null)
const draft = reactive({ name: '', prompt: '', kind: 'interval' as 'once' | 'interval' | 'cron', runAt: '', intervalMinutes: 60, cronExpr: '0 9 * * *', modelConfigId: '', workingDirectory: '' })
const { t, locale } = useI18n()
const tasks = ref<ScheduledTask[]>([])
const runs = ref<ScheduledTaskRun[]>([])
const taskId = ref('')
const status = ref('')
const loading = ref(false)
const loadingMore = ref(false)
const error = ref(false)
const hasMore = ref(false)
const detailVisible = ref(false)
const detail = ref<ScheduledTaskRun | null>(null)
const detailLoading = ref(false)
const detailError = ref(false)
const selectedTask = computed(() => tasks.value.find(task => task.id === taskId.value))
const waitingCount = computed(() => tasks.value.filter(task => task.status === 'scheduled').length)
const runningCount = computed(() => tasks.value.filter(task => task.status === 'running').length)
const statuses: Array<'' | TaskRunStatus> = ['', 'running', 'ok', 'error', 'interrupted']
const statusIcons = { running: mdiClockOutline, ok: mdiCheckCircleOutline, error: mdiAlertCircleOutline, interrupted: mdiPauseCircleOutline }
let listRequest: AbortController | undefined
let detailRequest: AbortController | undefined
let timer: ReturnType<typeof setInterval> | undefined
let generation = 0
let polling = false

function date(value?: string) {
  if (!value) return '—'
  const parsed = new Date(value)
  return Number.isFinite(parsed.getTime()) ? parsed.toLocaleString(locale.value, { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' }) : '—'
}
function duration(run: ScheduledTaskRun) {
  const seconds = taskRunDurationSeconds(run.startedAt, run.finishedAt)
  if (seconds === null) return t(run.status === 'running' ? 'scheduleRunInProgress' : 'scheduleNoDuration')
  return seconds < 60 ? t('scheduleSeconds', { count: seconds }) : t('scheduleMinutesSeconds', { minutes: Math.floor(seconds / 60), seconds: seconds % 60 })
}
function preview(run: ScheduledTaskRun) {
  if (run.status === 'running') return t('scheduleRunningHint')
  if (run.status === 'interrupted') return t('scheduleInterruptedHint')
  return taskRunPreview(run.error || run.answer || '') || t('scheduleNoOutput')
}

async function load(quiet = false) {
  listRequest?.abort()
  const controller = new AbortController()
  listRequest = controller
  const current = ++generation
  loadingMore.value = false
  if (!quiet) { loading.value = true; error.value = false }
  try {
    const [taskRows, page] = await Promise.all([scheduleAPI.tasks(controller.signal), scheduleAPI.runs(taskId.value, status.value, 0, controller.signal)])
    if (current !== generation) return
    tasks.value = taskRows
    // Keep already loaded history in place during background status refreshes.
    if (quiet && runs.value.length > 20 && status.value !== 'running') {
      const fresh = new Map(page.runs.map(run => [run.id, run]))
      runs.value = [...page.runs, ...runs.value.filter(run => !fresh.has(run.id))]
    } else { runs.value = page.runs; hasMore.value = page.hasMore }
    error.value = false
  } catch {
    if (current === generation && !controller.signal.aborted && !quiet) error.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}
async function loadMore() {
  if (loadingMore.value || loading.value) return
  loadingMore.value = true
  const current = generation
  const controller = listRequest
  try {
    const page = await scheduleAPI.runs(taskId.value, status.value, runs.value.length, controller?.signal)
    if (current !== generation) return
    const seen = new Set(runs.value.map(run => run.id))
    runs.value.push(...page.runs.filter(run => !seen.has(run.id)))
    hasMore.value = page.hasMore
    error.value = false
  } catch { if (current === generation && !controller?.signal.aborted) error.value = true }
  finally { if (current === generation) loadingMore.value = false }
}
async function openDetail(run: ScheduledTaskRun, quiet = false) {
  detailRequest?.abort()
  const controller = new AbortController()
  detailRequest = controller
  if (!quiet) {
    detail.value = run
    detailVisible.value = true
    detailLoading.value = true
    detailError.value = false
  }
  try {
    const result = await scheduleAPI.run(run.id, controller.signal)
    if (!controller.signal.aborted && detailVisible.value) { detail.value = result; detailError.value = false }
  } catch {
    if (!controller.signal.aborted && detailVisible.value && !quiet) detailError.value = true
  } finally { if (!controller.signal.aborted) detailLoading.value = false }
}
function showTaskHistory(id: string) {
  view.value = 'history'
  taskId.value = id
  status.value = ''
}
function scheduleLabel(task: ScheduledTask) {
  if (task.scheduleKind === 'interval') return t('scheduleEveryMinutes', { count: task.intervalMinutes })
  if (task.scheduleKind === 'cron') return t('scheduleCronLabel', { expression: task.cronExpr })
  return t('scheduleOnceLabel', { time: date(task.runAt) })
}
function openCreate() {
  const next = new Date(Date.now() + 3600000)
  const localTime = new Date(next.getTime() - next.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
  Object.assign(draft, { name: '', prompt: '', kind: 'interval', runAt: localTime, intervalMinutes: 60, cronExpr: '0 9 * * *', modelConfigId: '', workingDirectory: props.initialWorkingDirectory ?? '' })
  createVisible.value = true
}
async function createTask() {
  if (creating.value || !taskForm.value?.reportValidity()) return
  creating.value = true
  try {
    const schedule: CreateScheduledTaskInput['schedule'] = { kind: draft.kind }
    if (draft.kind === 'once') schedule.runAt = new Date(draft.runAt).toISOString()
    if (draft.kind === 'interval') schedule.intervalMinutes = Number(draft.intervalMinutes)
    if (draft.kind === 'cron') schedule.cronExpr = draft.cronExpr.trim()
    await scheduleAPI.create({ name: draft.name.trim(), prompt: draft.prompt.trim(), workingDirectory: draft.workingDirectory.trim(), modelConfigId: draft.modelConfigId, schedule })
    createVisible.value = false
    view.value = 'tasks'
    await load()
    toast.success(t('scheduleCreated'))
  } catch { toast.error(t('scheduleCreateFailed')) }
  finally { creating.value = false }
}
async function taskAction(task: ScheduledTask, action: 'pause' | 'resume' | 'run') {
  if (taskActionId.value) return
  taskActionId.value = task.id
  try {
    await scheduleAPI.action(task.id, action)
    await load(true)
    toast.success(t(action === 'run' ? 'scheduleQueued' : 'scheduleTaskUpdated'))
  } catch { toast.error(t('scheduleActionFailed')) }
  finally { taskActionId.value = '' }
}
watch([taskId, status], () => { runs.value = []; hasMore.value = false; void load() })
watch(detailVisible, visible => { if (!visible) detailRequest?.abort() })
onMounted(() => {
  void load()
  timer = setInterval(async () => {
    if (document.hidden || loading.value || loadingMore.value || polling) return
    polling = true
    try {
      const updates: Promise<void>[] = []
      if (waitingCount.value || runningCount.value || runs.value.some(run => run.status === 'running')) updates.push(load(true))
      if (detailVisible.value && !detailLoading.value && detail.value?.status === 'running') updates.push(openDetail(detail.value, true))
      await Promise.all(updates)
    } finally { polling = false }
  }, 5000)
})
onUnmounted(() => { generation++; listRequest?.abort(); detailRequest?.abort(); clearInterval(timer) })
</script>

<template>
  <div class="schedule-tab">
    <header class="schedule-heading">
      <div>
        <div class="schedule-page-title"><span><MdiIcon :path="mdiCalendarClockOutline" :size="25" /></span><h2>{{ t('scheduleSettings') }}</h2></div>
        <p>{{ t('scheduleDescription') }}</p>
      </div>
      <div class="schedule-header-actions">
        <button type="button" class="schedule-button" :disabled="loading || loadingMore" :aria-label="t('scheduleRefresh')" @click="load()"><MdiIcon :path="mdiRefresh" :size="16" /></button>
        <button type="button" class="schedule-button schedule-new-task" @click="openCreate"><MdiIcon :path="mdiPlus" :size="16" />{{ t('scheduleCreate') }}</button>
      </div>
    </header>

    <div class="schedule-summary">
      <div><span>{{ t('scheduleTotalTasks') }}</span><strong>{{ tasks.length }}</strong></div>
      <div><span>{{ t('scheduleWaitingTasks') }}</span><strong>{{ waitingCount }}</strong></div>
      <div><span>{{ t('scheduleRunningTasks') }}</span><strong class="schedule-accent">{{ runningCount }}</strong></div>
    </div>

    <div class="schedule-view-tabs" role="group" :aria-label="t('scheduleViews')">
      <button type="button" :aria-pressed="view === 'tasks'" :class="{ active: view === 'tasks' }" @click="view = 'tasks'">{{ t('scheduleTaskList') }}<span>{{ tasks.length }}</span></button>
      <button type="button" :aria-pressed="view === 'history'" :class="{ active: view === 'history' }" @click="view = 'history'">{{ t('scheduleHistory') }}</button>
    </div>
    <section v-if="view === 'tasks'" class="schedule-task-grid" :aria-label="t('scheduleTaskList')">
      <div v-if="loading" class="schedule-empty"><LoadingSpinner /></div>
      <div v-else-if="error" class="schedule-error" role="alert"><span>{{ t('scheduleLoadFailed') }}</span><button type="button" @click="load()">{{ t('scheduleRetry') }}</button></div>
      <div v-else-if="!tasks.length" class="schedule-empty schedule-task-empty">
        <span class="schedule-empty-icon"><MdiIcon :path="mdiCalendarClockOutline" :size="32" /></span>
        <strong>{{ t('scheduleNoTasks') }}</strong><p>{{ t('scheduleTaskEmptyHint') }}</p>
        <button type="button" class="schedule-button schedule-new-task" @click="openCreate">{{ t('scheduleCreate') }}</button>
      </div>
      <article v-for="task in loading ? [] : tasks" :key="task.id" class="schedule-task-card">
        <div class="schedule-task-card-heading"><h3>{{ task.name }}</h3><span class="schedule-badge" :class="task.status === 'running' ? 'status-running' : ''">{{ t(`scheduleTaskStatus_${task.status}`) }}</span></div>
        <p class="schedule-task-prompt">{{ task.prompt }}</p>
        <div class="schedule-rule"><MdiIcon :path="mdiClockOutline" :size="15" /><span>{{ scheduleLabel(task) }}</span></div>
        <dl class="schedule-task-timing"><div><dt>{{ t('scheduleNextRun') }}</dt><dd>{{ date(task.nextRunAt) }}</dd></div><div><dt>{{ t('scheduleCompletedRuns') }}</dt><dd>{{ task.completedRuns }}</dd></div></dl>
        <div class="schedule-task-actions">
          <button type="button" class="schedule-button schedule-primary" @click="showTaskHistory(task.id)">{{ t('scheduleViewHistory') }}</button>
          <div>
            <button v-if="task.status === 'scheduled' || task.status === 'paused'" type="button" class="schedule-button schedule-text-action" :disabled="!!taskActionId" @click="taskAction(task, task.status === 'paused' ? 'resume' : 'pause')">{{ t(task.status === 'paused' ? 'scheduleResume' : 'schedulePause') }}</button>
            <button type="button" class="schedule-button schedule-text-action" :disabled="task.status === 'running' || !!taskActionId" @click="taskAction(task, 'run')"><MdiIcon :path="mdiPlayOutline" :size="16" />{{ t('scheduleRunNow') }}</button>
          </div>
        </div>
      </article>
    </section>
    <section v-else class="schedule-history" :aria-label="t('scheduleHistory')">
      <div class="schedule-toolbar">
        <h3>{{ t('scheduleHistory') }}</h3>
        <select v-model="taskId" :aria-label="t('scheduleFilterTask')" class="schedule-select">
          <option value="">{{ t('scheduleAllTasks') }}</option>
          <option v-for="task in tasks" :key="task.id" :value="task.id">{{ task.name }}</option>
        </select>
      </div>
      <div v-if="selectedTask" class="schedule-task-info">
        <strong>{{ selectedTask.name }}</strong>
        <span>{{ t('scheduleNextRun') }} · {{ date(selectedTask.nextRunAt) }}</span>
        <p>{{ selectedTask.prompt }}</p>
      </div>
      <div class="schedule-filters" role="group" :aria-label="t('scheduleFilterStatus')">
        <button v-for="item in statuses" :key="item" type="button" :aria-pressed="status === item" :class="{ active: status === item }" @click="status = item">
          {{ t(item ? `scheduleRunStatus_${item}` : 'scheduleAllRuns') }}
        </button>
      </div>
      <div v-if="error" class="schedule-error" role="alert">
        <span>{{ t('scheduleLoadFailed') }}</span><button type="button" @click="load()">{{ t('scheduleRetry') }}</button>
      </div>
      <div v-if="loading" class="schedule-empty" role="status"><LoadingSpinner /><span>{{ t('scheduleLoading') }}</span></div>
      <div v-else-if="!runs.length && !error" class="schedule-empty">
        <span class="schedule-empty-icon"><MdiIcon :path="mdiClockOutline" :size="28" /></span>
        <strong>{{ t('scheduleEmpty') }}</strong>
        <p>{{ t(tasks.length ? 'scheduleEmptyFilteredHint' : 'scheduleEmptyHint') }}</p>
      </div>
      <div v-else-if="!loading" class="schedule-run-list">
        <button v-for="run in runs" :key="run.id" type="button" class="schedule-run-row" @click="openDetail(run)">
          <span class="schedule-status-icon" :class="`status-${run.status}`"><MdiIcon :path="statusIcons[run.status]" :size="19" /></span>
          <span class="schedule-run-content">
            <span class="schedule-run-title"><strong>{{ run.taskName }}</strong><span class="schedule-badge" :class="`status-${run.status}`">{{ t(`scheduleRunStatus_${run.status}`) }}</span></span>
            <span class="schedule-preview">{{ preview(run) }}</span>
            <span class="schedule-run-meta"><span>{{ date(run.startedAt) }}</span><span>·</span><span>{{ duration(run) }}</span></span>
          </span>
          <MdiIcon :path="mdiChevronRight" :size="18" class="schedule-chevron" />
        </button>
      </div>
      <button v-if="hasMore && !loading" type="button" class="schedule-more schedule-button" :disabled="loadingMore" @click="loadMore()">
        <LoadingSpinner v-if="loadingMore" size-class="w-3.5 h-3.5" />{{ t('scheduleLoadMore') }}
      </button>
    </section>

    <AppDialog v-model:visible="detailVisible" :title="t('scheduleRunDetail')" width="700px" hide-footer>
      <div v-if="detail" class="schedule-detail">
        <div class="schedule-detail-heading"><h3>{{ detail.taskName }}</h3><span class="schedule-badge" :class="`status-${detail.status}`">{{ t(`scheduleRunStatus_${detail.status}`) }}</span></div>
        <dl class="schedule-detail-meta">
          <div><dt>{{ t('scheduleStartedAt') }}</dt><dd>{{ date(detail.startedAt) }}</dd></div>
          <div><dt>{{ t('scheduleFinishedAt') }}</dt><dd>{{ date(detail.finishedAt) }}</dd></div>
          <div><dt>{{ t('scheduleDuration') }}</dt><dd>{{ duration(detail) }}</dd></div>
        </dl>
        <div v-if="detailLoading" class="schedule-empty" role="status"><LoadingSpinner /></div>
        <div v-else-if="detailError" class="schedule-error" role="alert"><span>{{ t('scheduleDetailFailed') }}</span><button type="button" @click="openDetail(detail)">{{ t('scheduleRetry') }}</button></div>
        <template v-else>
          <div v-if="detail.error || detail.status === 'interrupted'" class="schedule-failure"><h4>{{ t('scheduleErrorDetail') }}</h4><p>{{ detail.status === 'interrupted' ? t('scheduleInterruptedHint') : detail.error }}</p></div>
          <section class="schedule-output"><h4>{{ t('scheduleOutput') }}</h4><div v-if="detail.answer" class="bubble-markdown" v-html="renderMarkdown(detail.answer)" /><p v-else>{{ t(detail.status === 'running' ? 'scheduleRunningHint' : 'scheduleNoOutput') }}</p></section>
          <CooperativePanel v-if="detail.sessionId" :session-id="detail.sessionId" />
          <TaskRunTimeline v-if="detail.sessionId" :run-id="detail.id" :run-status="detail.status" />
        </template>
      </div>
    </AppDialog>
    <AppDialog v-model:visible="createVisible" :title="t('scheduleCreate')" :confirm-text="t('scheduleCreate')" :confirm-loading="creating" width="540px" @confirm="createTask">
      <form ref="taskForm" class="schedule-create-form" @submit.prevent="createTask">
        <label>{{ t('scheduleTaskName') }}<AppTextInput v-model="draft.name" required maxlength="128" :placeholder="t('scheduleTaskNamePlaceholder')" autofocus /></label>
        <label>{{ t('scheduleInstructions') }}<textarea v-model="draft.prompt" required maxlength="20000" rows="4" :placeholder="t('scheduleInstructionsPlaceholder')" /></label>
        <label>{{ t('scheduleRule') }}<select v-model="draft.kind"><option value="interval">{{ t('scheduleInterval') }}</option><option value="once">{{ t('scheduleOnce') }}</option><option value="cron">{{ t('scheduleCron') }}</option></select></label>
        <label v-if="draft.kind === 'once'">{{ t('scheduleRunAt') }}<input v-model="draft.runAt" required type="datetime-local" /></label>
        <label v-else-if="draft.kind === 'interval'">{{ t('scheduleIntervalMinutes') }}<input v-model.number="draft.intervalMinutes" required type="number" min="1" max="525600" /></label>
        <label v-else>{{ t('scheduleCronExpression') }}<AppTextInput v-model="draft.cronExpr" required :placeholder="'0 9 * * *'" /><span>{{ t('scheduleCronHint') }}</span></label>
        <label>{{ t('scheduleTaskModel') }}<select v-model="draft.modelConfigId"><option value="">{{ t('scheduleDefaultModel') }}</option><option v-for="model in modelOptions" :key="model.value" :value="model.value">{{ model.label }}</option></select></label>
        <label>{{ t('scheduleWorkingDirectory') }}<AppTextInput v-model="draft.workingDirectory" :placeholder="t('scheduleWorkingDirectoryPlaceholder')" /></label>
        <p>{{ t('scheduleCreateHint') }}</p>
        <button type="submit" hidden />
      </form>
    </AppDialog>
  </div>
</template>

<style scoped>
.schedule-tab, .schedule-detail, .schedule-create-form {
  --schedule-danger: #b91c1c;
  --schedule-muted: #696780;
}
:global(.dark .schedule-tab), :global(.dark .schedule-detail), :global(.dark .schedule-create-form) {
  --schedule-danger: #f87171;
  --schedule-muted: #a7a4bd;
}
:global(.dark .status-running) {
  color: var(--sb-brand-soft);
}
.schedule-tab {
  width: 100%;
  max-width: 1100px;
  padding: 30px 32px 48px;
  margin: 0 auto;
  color: var(--text-primary);
  display: flex;
  flex-direction: column;
  gap: 20px;
  min-width: 0;
}
.schedule-heading, .schedule-toolbar, .schedule-detail-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
h2 {
  margin: 0;
  font-size: 17px;
  font-weight: 650;
}
h3 {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
}
.schedule-heading p {
  color: var(--schedule-muted);
  font-size: 12px;
  line-height: 1.7;
  margin: 6px 0 0;
}
.schedule-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 7px 11px;
  border: 1px solid var(--input-border);
  border-radius: 9px;
  background: var(--input-bg);
  color: var(--text-secondary);
  font-size: 12px;
  white-space: nowrap;
  cursor: pointer;
  transition: background 150ms, border-color 150ms;
}
.schedule-button:hover:not(:disabled) {
  background: var(--primary-alpha-08);
  border-color: var(--sb-brand);
}
button:disabled {
  opacity: .5;
  cursor: wait;
}
button:focus-visible, select:focus-visible {
  outline: 2px solid var(--sb-brand);
  outline-offset: 3px;
}
.schedule-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  border: 1px solid var(--card-border);
  border-radius: 12px;
  background: var(--primary-alpha-05);
}
.schedule-summary > div {
  padding: 15px 18px;
  display: flex;
  flex-direction: column;
  gap: 7px;
}
.schedule-summary > div + div {
  border-left: 1px solid var(--card-border);
}
.schedule-summary span {
  font-size: 12px;
  color: var(--schedule-muted);
}
.schedule-summary strong {
  font-size: 25px;
  font-weight: 600;
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}
.schedule-accent {
  color: var(--sb-brand);
}
.schedule-history {
  min-width: 0;
}
.schedule-toolbar {
  margin-bottom: 14px;
}
.schedule-select {
  max-width: 60%;
  min-width: 0;
  padding: 7px 28px 7px 10px;
  border: 1px solid var(--input-border);
  border-radius: 8px;
  background: var(--input-bg);
  color: var(--text-secondary);
  font-size: 12px;
  text-overflow: ellipsis;
  cursor: pointer;
}
.schedule-task-info {
  padding: 12px;
  border-radius: 9px;
  background: var(--primary-alpha-05);
  font-size: 12px;
  margin-bottom: 12px;
  overflow-wrap: anywhere;
}
.schedule-task-info strong {
  display: block;
  margin-bottom: 5px;
}
.schedule-task-info span, .schedule-task-info p {
  color: var(--schedule-muted);
}
.schedule-task-info p {
  margin: 7px 0 0;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.schedule-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-bottom: 12px;
}
.schedule-filters button {
  padding: 5px 10px;
  border: 1px solid transparent;
  border-radius: 7px;
  color: var(--schedule-muted);
  font-size: 12px;
  cursor: pointer;
  transition: background 150ms;
}
.schedule-filters button:hover {
  background: var(--primary-alpha-05);
}
.schedule-filters button.active {
  color: var(--sb-brand);
  background: var(--primary-alpha-10);
  border-color: var(--primary-alpha-15);
}
.schedule-run-list {
  border: 1px solid var(--card-border);
  border-radius: 12px;
  overflow: hidden;
}
.schedule-run-row {
  display: flex;
  align-items: flex-start;
  gap: 11px;
  padding: 15px;
  width: 100%;
  min-width: 0;
  text-align: left;
  cursor: pointer;
  background: var(--card-bg);
  transition: background 150ms;
}
.schedule-run-row + .schedule-run-row {
  border-top: 1px solid var(--card-border);
}
.schedule-run-row:hover {
  background: var(--primary-alpha-05);
}
.schedule-status-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 9px;
  flex-shrink: 0;
  background: var(--primary-alpha-05);
}
.schedule-run-content {
  flex: 1;
  min-width: 0;
}
.schedule-run-title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.schedule-run-title strong {
  font-size: 13px;
  font-weight: 550;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.schedule-badge {
  display: inline-flex;
  flex-shrink: 0;
  padding: 2px 6px;
  border-radius: 5px;
  font-size: 10px;
  font-weight: 500;
  background: var(--primary-alpha-05);
  white-space: nowrap;
}
.status-running {
  color: var(--sb-brand);
}
.status-ok {
  color: #15803d;
}
.status-error {
  color: #b91c1c;
}
.status-interrupted {
  color: var(--schedule-muted);
}
:global(.dark .status-ok) {
  color: #4ade80;
}
:global(.dark .status-error) {
  color: #f87171;
}
.schedule-preview {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  font-size: 12px;
  line-height: 1.65;
  color: var(--text-secondary);
  margin-top: 6px;
  overflow-wrap: anywhere;
  white-space: pre-line;
}
.schedule-run-meta {
  display: flex;
  gap: 7px;
  flex-wrap: wrap;
  font-size: 11px;
  color: var(--schedule-muted);
  margin-top: 7px;
}
.schedule-chevron {
  flex-shrink: 0;
  color: var(--schedule-muted);
  margin-top: 7px;
}
.schedule-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  text-align: center;
  padding: 38px 16px;
  color: var(--schedule-muted);
  font-size: 12px;
}
.schedule-empty strong {
  font-size: 14px;
  color: var(--text-secondary);
}
.schedule-empty p {
  max-width: 320px;
  line-height: 1.8;
  margin: 0;
}
.schedule-empty-icon {
  display: flex;
  padding: 14px;
  border-radius: 16px;
  background: var(--primary-alpha-05);
  color: var(--sb-brand);
}
.schedule-more {
  width: 100%;
  margin-top: 12px;
}
.schedule-error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 12px;
  margin: 8px 0;
  border-radius: 9px;
  border: 1px solid var(--card-border);
  color: var(--schedule-danger);
  font-size: 12px;
}
.schedule-error button {
  cursor: pointer;
  white-space: nowrap;
  text-decoration: underline;
}
.schedule-detail {
  min-width: 0;
  color: var(--text-primary);
}
.schedule-detail-heading h3 {
  font-size: 16px;
  overflow-wrap: anywhere;
}
.schedule-detail-meta {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  padding: 16px 0;
  margin: 16px 0;
  border-top: 1px solid var(--card-border);
  border-bottom: 1px solid var(--card-border);
  font-size: 12px;
}
dt {
  color: var(--schedule-muted);
  margin-bottom: 7px;
}
dd {
  margin: 0;
  overflow-wrap: anywhere;
  font-variant-numeric: tabular-nums;
}
.schedule-failure {
  padding: 14px;
  border: 1px solid var(--card-border);
  border-left: 3px solid var(--color-danger);
  border-radius: 8px;
  margin-bottom: 18px;
  background: var(--primary-alpha-05);
}
h4 {
  font-size: 12px;
  font-weight: 600;
  margin: 0 0 10px;
}
.schedule-failure p {
  margin: 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  color: var(--schedule-danger);
  font-size: 12px;
  line-height: 1.7;
}
.schedule-output {
  font-size: 13px;
  line-height: 1.8;
  min-width: 0;
}
.schedule-output > p {
  color: var(--schedule-muted);
}
.schedule-output h4 {
  color: var(--schedule-muted);
}
.schedule-primary {
  color: var(--sb-brand);
  border-color: var(--primary-alpha-20);
  background: var(--primary-alpha-10);
}
@media (max-width: 480px) {

  .schedule-heading {
    align-items: flex-start;
  }
  .schedule-summary > div {
    padding: 12px 10px;
  }
  .schedule-summary span {
    font-size: 11px;
  }
  .schedule-run-row {
    padding: 12px;
    gap: 8px;
  }
  .schedule-status-icon {
    width: 26px;
    height: 26px;
  }
  .schedule-detail-meta {
    grid-template-columns: 1fr;
    gap: 12px;
  }
  .schedule-detail-meta > div {
    display: flex;
    justify-content: space-between;
    gap: 12px;
  }
  dt {
    margin: 0;
  }
}

.schedule-page-title { display: flex; align-items: center; gap: 12px; }
.schedule-page-title > span { display: flex; align-items: center; justify-content: center; width: 44px; height: 44px; border-radius: 13px; background: var(--primary-alpha-10); color: var(--sb-brand); }
.schedule-page-title h2 { font-size: 23px; }
.schedule-header-actions { display: flex; gap: 8px; }
.schedule-heading p { margin-left: 56px; }
.schedule-new-task { background: var(--sb-brand); border-color: var(--sb-brand); color: white; padding: 9px 13px; }
.schedule-new-task:hover:not(:disabled) { background: var(--sb-brand-hover); }
.schedule-view-tabs { display: flex; gap: 24px; border-bottom: 1px solid var(--card-border); }
.schedule-view-tabs button { display: flex; align-items: center; gap: 8px; padding: 5px 0 13px; font-size: 13px; font-weight: 550; cursor: pointer; color: var(--schedule-muted); border-bottom: 2px solid transparent; }
.schedule-view-tabs button.active { color: var(--sb-brand); border-color: var(--sb-brand); }
.schedule-view-tabs span { font-size: 10px; padding: 1px 6px; border-radius: 5px; background: var(--primary-alpha-08); }
.schedule-task-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.schedule-task-grid > .schedule-empty, .schedule-task-grid > .schedule-error { grid-column: 1 / -1; }
.schedule-task-card { display: flex; flex-direction: column; padding: 20px; border: 1px solid var(--card-border); border-radius: 14px; background: var(--card-bg); min-width: 0; }
.schedule-task-card-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
.schedule-task-card h3 { line-height: 1.6; overflow-wrap: anywhere; }
.schedule-task-prompt { font-size: 12px; color: var(--text-secondary); line-height: 1.8; margin: 10px 0 18px; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
.schedule-rule { display: flex; align-items: center; gap: 7px; font-size: 12px; color: var(--schedule-muted); margin-top: auto; overflow-wrap: anywhere; }
.schedule-rule > svg { flex-shrink: 0; }
.schedule-task-timing { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 12px; padding: 15px 0; margin: 0; font-size: 11px; }
.schedule-task-timing dd { color: var(--text-secondary); font-size: 12px; }
.schedule-task-actions { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px; border-top: 1px solid var(--card-border); padding-top: 13px; }
.schedule-task-actions > div { display: flex; gap: 4px; }
.schedule-text-action { border-color: transparent; background: transparent; padding-inline: 7px; }
.schedule-task-empty { padding: 70px 15px; }
.schedule-create-form { display: flex; flex-direction: column; gap: 16px; color-scheme: light; }
:global(.dark .schedule-create-form) { color-scheme: dark; }
.schedule-create-form label { display: flex; flex-direction: column; gap: 8px; color: var(--text-secondary); font-size: 12px; font-weight: 550; }
.schedule-create-form input, .schedule-create-form select, .schedule-create-form textarea { width: 100%; padding: 9px 11px; border: 1px solid var(--input-border); border-radius: 9px; background: var(--input-bg); color: var(--text-primary); font-size: 13px; font-weight: 400; }
.schedule-create-form textarea { min-height: 100px; resize: vertical; }
.schedule-create-form input:focus-visible, .schedule-create-form textarea:focus-visible { outline: 2px solid var(--sb-brand); outline-offset: 2px; }
.schedule-create-form p, .schedule-create-form label > span { color: var(--schedule-muted); font-size: 11px; line-height: 1.7; margin: 0; font-weight: 400; }
@media (max-width: 1050px) { .schedule-task-grid { grid-template-columns: 1fr; } }
@media (max-width: 600px) {
  .schedule-tab { padding: 22px 16px 32px; }
  .schedule-heading { flex-wrap: wrap; gap: 18px; }
  .schedule-heading p { margin-left: 0; margin-top: 10px; }
  .schedule-page-title h2 { font-size: 20px; }
  .schedule-page-title > span { width: 36px; height: 36px; border-radius: 10px; }
  .schedule-header-actions { margin-left: auto; }
  .schedule-task-card { padding: 16px; }
}
</style>
