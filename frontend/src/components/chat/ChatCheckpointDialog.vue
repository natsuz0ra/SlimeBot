<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { isAxiosError } from 'axios'
import { mdiAlertCircleOutline, mdiHistory, mdiRefresh, mdiRestore, mdiUndoVariant } from '@mdi/js'
import { checkpointAPI, type Checkpoint, type CheckpointDetail, type CheckpointDiff, type CheckpointRollbackMode } from '@/api/checkpoints'
import AppDialog from '@/components/ui/AppDialog.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import CheckpointDiffView from './CheckpointDiff.vue'

const props = defineProps<{
  visible: boolean
  sessionId: string
  workingDirectory: string
  turnRunning: boolean
  workspaceRunning: boolean
}>()
const emit = defineEmits<{ close: [] }>()

const { t, locale } = useI18n()
const dateFormatter = computed(() => new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }))
const checkpoints = ref<Checkpoint[]>([])
const selectedId = ref('')
const detail = ref<CheckpointDetail>()
const selectedPath = ref('')
const diff = ref<CheckpointDiff>()
const listLoading = ref(false)
const detailLoading = ref(false)
const diffLoading = ref(false)
const listError = ref('')
const detailError = ref('')
const diffError = ref('')
const rollbackError = ref('')
const rollbackDone = ref(false)
const rollingBack = ref(false)
const confirmation = ref<{ id: string; mode: CheckpointRollbackMode; latestId?: string }>()
const undoButton = ref<HTMLButtonElement>()
const restoreButton = ref<HTMLButtonElement>()
const refreshButton = ref<HTMLButtonElement>()
const cancelButton = ref<HTMLButtonElement>()

let listRequest: AbortController | undefined
let detailRequest: AbortController | undefined
let diffRequest: AbortController | undefined
let pollTimer: number | undefined
let viewGeneration = 0

const selectedIndex = computed(() => checkpoints.value.findIndex(({ id }) => id === selectedId.value))
const selected = computed(() => checkpoints.value[selectedIndex.value])
const newerCheckpoints = computed(() => selectedIndex.value > 0 ? checkpoints.value.slice(0, selectedIndex.value) : [])
const restoreGap = computed(() => newerCheckpoints.value.some(({ unavailable }) => unavailable))
const canRestoreChanges = computed(() => newerCheckpoints.value.some(({ fileCount }) => fileCount > 0))
const actionsReady = computed(() => !!selected.value && !selected.value.unavailable && !props.workspaceRunning && !listLoading.value && !listError.value && !rollingBack.value)
const canUndo = computed(() => actionsReady.value && (selected.value?.fileCount ?? 0) > 0)
const canRestore = computed(() => actionsReady.value && !restoreGap.value && canRestoreChanges.value)
const isUndo = computed(() => confirmation.value?.mode === 'undo')
const undoLabel = computed(() => t(selected.value?.kind === 'turn' ? 'checkpoints.undo' : 'checkpoints.undoOperation'))
const confirmationChanged = computed(() => {
  const target = confirmation.value
  return !!target && (target.id !== selectedId.value || (target.mode === 'restore' && target.latestId !== checkpoints.value[0]?.id))
})
const canConfirm = computed(() => !confirmationChanged.value && (isUndo.value ? canUndo.value : canRestore.value))
const limitedConfirmation = computed(() => selected.value?.limited || (!isUndo.value && newerCheckpoints.value.some(({ limited }) => limited)))
const checkpointIcons = { turn: mdiHistory, undo: mdiUndoVariant, restore: mdiRestore }
const fileMarks = { added: '+', modified: '±', deleted: '−' }

function displayDate(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : dateFormatter.value.format(date)
}

function requestError(error: unknown, fallback: string) {
  const message = isAxiosError<{ error?: string }>(error) ? error.response?.data?.error : undefined
  return typeof message === 'string' && message.trim() ? message : t(fallback)
}

async function loadDiff(path: string) {
  diffRequest?.abort()
  diff.value = undefined
  diffError.value = ''
  selectedPath.value = path
  const checkpointId = selectedId.value
  const request = new AbortController()
  diffRequest = request
  diffLoading.value = true
  try {
    const result = await checkpointAPI.diff(props.sessionId, checkpointId, path, request.signal)
    if (!request.signal.aborted) diff.value = result
  } catch (error) {
    if (!request.signal.aborted) diffError.value = requestError(error, 'checkpoints.diffError')
  } finally {
    if (!request.signal.aborted) diffLoading.value = false
  }
}

async function selectCheckpoint(checkpoint?: Checkpoint) {
  detailRequest?.abort()
  diffRequest?.abort()
  selectedId.value = checkpoint?.id ?? ''
  detail.value = undefined
  detailError.value = ''
  detailLoading.value = false
  selectedPath.value = ''
  diff.value = undefined
  diffError.value = ''
  diffLoading.value = false
  if (!checkpoint || checkpoint.unavailable) return

  const request = new AbortController()
  detailRequest = request
  detailLoading.value = true
  try {
    const result = await checkpointAPI.detail(props.sessionId, checkpoint.id, request.signal)
    if (request.signal.aborted) return
    detail.value = result
    if (result.files?.length) void loadDiff(result.files[0]!.path)
  } catch (error) {
    if (!request.signal.aborted) detailError.value = requestError(error, 'checkpoints.detailError')
  } finally {
    if (!request.signal.aborted) detailLoading.value = false
  }
}

async function refreshHistory(showLoading = true, recovery?: { id: string; mode: CheckpointRollbackMode; previousLatest?: string }) {
  if (!props.visible) return
  if (!showLoading && listRequest) return
  listRequest?.abort()
  const request = new AbortController()
  listRequest = request
  listLoading.value = showLoading
  if (showLoading) listError.value = ''
  try {
    const result = await checkpointAPI.list(props.sessionId, request.signal) ?? []
    if (request.signal.aborted) return
    const previous = selected.value
    checkpoints.value = result
    listError.value = ''
    const latest = result[0]
    const completed = recovery && latest && latest.id !== recovery.previousLatest && latest.sourceId === recovery.id && latest.kind === recovery.mode
    const next = completed ? latest : selected.value ?? latest
    if (next?.id !== previous?.id || next?.unavailable !== previous?.unavailable) void selectCheckpoint(next)
  } catch (error) {
    if (!request.signal.aborted && (showLoading || !checkpoints.value.length)) {
      listError.value = requestError(error, 'checkpoints.listError')
    }
  } finally {
    if (!request.signal.aborted) {
      listLoading.value = false
      listRequest = undefined
    }
  }
}

async function openConfirmation(mode: CheckpointRollbackMode) {
  if (!selected.value || !(mode === 'undo' ? canUndo.value : canRestore.value)) return
  rollbackError.value = ''
  confirmation.value = { id: selected.value.id, mode, latestId: checkpoints.value[0]?.id }
  await nextTick()
  cancelButton.value?.focus()
}

async function closeConfirmation() {
  if (rollingBack.value) return
  const mode = confirmation.value?.mode
  confirmation.value = undefined
  await nextTick()
  const button = mode === 'undo' ? undoButton.value : restoreButton.value
  if (button && !button.disabled) button.focus()
  else refreshButton.value?.focus()
}

async function rollback() {
  const target = confirmation.value
  if (!target || !canConfirm.value) return
  const generation = viewGeneration
  const previousLatest = checkpoints.value[0]?.id
  rollbackError.value = ''
  rollbackDone.value = false
  rollingBack.value = true
  try {
    await checkpointAPI.rollback(props.sessionId, target.id, target.mode)
    if (generation !== viewGeneration) return
    rollbackDone.value = true
    confirmation.value = undefined
    void refreshHistory(true, { ...target, previousLatest })
    await nextTick()
    refreshButton.value?.focus()
  } catch (error) {
    if (generation !== viewGeneration) return
    const conflict = isAxiosError(error) && error.response?.status === 409
    rollbackError.value = requestError(error, conflict ? 'checkpoints.rollbackConflict' : 'checkpoints.rollbackError')
    if (conflict) void refreshHistory()
  } finally {
    if (generation === viewGeneration) rollingBack.value = false
  }
}

function clearRequests() {
  viewGeneration++
  listRequest?.abort()
  listRequest = undefined
  detailRequest?.abort()
  diffRequest?.abort()
  window.clearInterval(pollTimer)
}

watch(() => props.visible, (visible) => {
  clearRequests()
  checkpoints.value = []
  void selectCheckpoint()
  listError.value = ''
  listLoading.value = false
  rollbackError.value = ''
  rollbackDone.value = false
  rollingBack.value = false
  confirmation.value = undefined
  if (!visible) return
  void refreshHistory()
  pollTimer = window.setInterval(() => {
    if (!rollingBack.value) void refreshHistory(false)
  }, 5000)
}, { immediate: true })

watch([() => props.turnRunning, () => props.workspaceRunning], ([turn, workspace], [previousTurn, previousWorkspace]) => {
  if ((previousTurn && !turn) || (previousWorkspace && !workspace)) void refreshHistory(false)
})

onUnmounted(clearRequests)

function onClose() {
  if (rollingBack.value) return
  if (confirmation.value) void closeConfirmation()
  else emit('close')
}
</script>

<template>
  <AppDialog
    :visible="visible"
    :title="confirmation ? t(isUndo ? 'checkpoints.confirmUndo' : 'checkpoints.confirmRestore') : t('checkpoints.title')"
    width="880px"
    hide-footer
    :show-close="!rollingBack"
    :close-on-mask="!confirmation"
    :close-on-esc="!rollingBack"
    @cancel="onClose"
  >
    <section v-if="confirmation" class="rollback-confirmation" :aria-busy="rollingBack">
      <div class="confirm-content">
        <span class="confirm-icon"><MdiIcon :path="mdiAlertCircleOutline" :size="22" /></span>
        <div class="min-w-0 space-y-2">
          <p class="confirm-description">{{ t(isUndo ? 'checkpoints.undoHint' : 'checkpoints.restoreHint') }}</p>
          <p v-if="selected && !confirmationChanged" class="text-xs">
            {{ t(`checkpoints.kind.${selected.kind}`) }}
            <time class="ml-1" :datetime="selected.createdAt">{{ displayDate(selected.createdAt) }}</time>
          </p>
          <code class="checkpoint-path block" :title="workingDirectory">{{ workingDirectory }}</code>
          <p>{{ t('checkpoints.recoveryHint') }}</p>
          <p v-if="limitedConfirmation" class="checkpoint-warning">{{ t('checkpoints.limitedHint') }}</p>
        </div>
      </div>
      <p v-if="confirmationChanged || workspaceRunning" class="checkpoint-warning mt-3" role="status">
        {{ t(confirmationChanged ? 'checkpoints.historyChanged' : 'checkpoints.running') }}
      </p>
      <p v-if="rollbackError || listError" class="checkpoint-error mt-3" role="alert">{{ rollbackError || listError }}</p>
      <p v-if="rollingBack" class="mt-3" role="status">{{ t('checkpoints.rollbackWorking') }}</p>
      <div class="rollback-actions">
        <button ref="cancelButton" type="button" class="checkpoint-button" :disabled="rollingBack" @click="closeConfirmation">{{ t('cancel') }}</button>
        <button type="button" class="checkpoint-button button-danger" :disabled="!canConfirm" @click="rollback">
          <LoadingSpinner v-if="rollingBack" size-class="size-3.5" aria-hidden="true" />
          {{ isUndo ? undoLabel : t('checkpoints.restore') }}
        </button>
      </div>
    </section>

    <section v-else class="checkpoint-browser">
      <div class="checkpoint-toolbar">
        <code class="checkpoint-path" :title="workingDirectory">{{ workingDirectory }}</code>
        <button
          ref="refreshButton"
          type="button"
          class="checkpoint-icon-button"
          :aria-label="t('checkpoints.refresh')"
          :aria-disabled="listLoading"
          :title="t('checkpoints.refresh')"
          @click="!listLoading && refreshHistory()"
        >
          <LoadingSpinner v-if="listLoading" size-class="size-4" aria-hidden="true" />
          <MdiIcon v-else :path="mdiRefresh" :size="18" />
        </button>
      </div>
      <p v-if="workspaceRunning" class="checkpoint-warning status-message" role="status">
        <span class="running-dot" aria-hidden="true" />{{ t('checkpoints.running') }}
      </p>
      <p v-if="rollbackDone" class="status-message status-success" role="status">{{ t('checkpoints.rollbackDone') }}</p>
      <div v-if="listError" class="checkpoint-error status-message" role="alert">
        <span>{{ listError }}</span>
        <button type="button" class="checkpoint-button button-text" @click="refreshHistory()">{{ t('checkpoints.retry') }}</button>
      </div>
      <div v-if="!checkpoints.length" class="browser-empty">
        <p v-if="listLoading" role="status">{{ t('checkpoints.loading') }}</p>
        <template v-else-if="!listError">
          <MdiIcon :path="mdiHistory" :size="26" />
          <p class="empty-title">{{ t('checkpoints.empty') }}</p>
          <p>{{ t('checkpoints.emptyHint') }}</p>
        </template>
      </div>
      <div v-else class="checkpoint-layout">
        <section class="history-panel" :aria-label="t('checkpoints.history')">
          <h3 class="section-title">{{ t('checkpoints.history') }}</h3>
          <ol class="checkpoint-history sb-scrollbar">
            <li v-for="entry in checkpoints" :key="entry.id" class="history-entry">
              <button
                type="button"
                class="history-button"
                :aria-current="entry.id === selectedId ? 'true' : undefined"
                @click="entry.id !== selectedId && selectCheckpoint(entry)"
              >
                <span class="history-icon" :class="`history-icon-${entry.kind}`"><MdiIcon :path="checkpointIcons[entry.kind]" :size="16" /></span>
                <span class="grid min-w-0 gap-1 text-left">
                  <span class="history-kind">{{ t(`checkpoints.kind.${entry.kind}`) }}</span>
                  <time :datetime="entry.createdAt">{{ displayDate(entry.createdAt) }}</time>
                  <span class="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <span v-if="!entry.unavailable">{{ t('checkpoints.fileCount', entry.fileCount) }}</span>
                    <span v-if="entry.unavailable || entry.limited" class="history-note">{{ t(entry.unavailable ? 'checkpoints.unavailable' : 'checkpoints.limited') }}</span>
                  </span>
                </span>
              </button>
            </li>
          </ol>
        </section>
        <section v-if="selected" class="detail-panel" :aria-label="t('checkpoints.files')">
          <h3 class="section-title">{{ t('checkpoints.files') }}</h3>
          <p v-if="selected.unavailable" class="checkpoint-warning status-message">{{ t('checkpoints.unavailableHint') }}</p>
          <template v-else>
            <p v-if="selected.limited" class="checkpoint-warning status-message">{{ t('checkpoints.limitedHint') }}</p>
            <p v-if="detailLoading" class="files-state" role="status">{{ t('checkpoints.loadingFiles') }}</p>
            <div v-else-if="detailError" class="checkpoint-error status-message" role="alert">
              <span>{{ detailError }}</span>
              <button type="button" class="checkpoint-button button-text" @click="selectCheckpoint(selected)">{{ t('checkpoints.retry') }}</button>
            </div>
            <template v-else-if="detail?.files?.length">
              <ul class="file-list sb-scrollbar">
                <li v-for="file in detail.files" :key="file.path">
                  <button
                    type="button"
                    class="file-button"
                    :aria-pressed="file.path === selectedPath"
                    :title="file.path"
                    @click="file.path !== selectedPath && loadDiff(file.path)"
                  >
                    <span class="file-change" :class="`file-change-${file.kind}`" aria-hidden="true">{{ fileMarks[file.kind] }}</span>
                    <code class="file-path">{{ file.path }}</code>
                    <span class="file-kind">{{ t(`checkpoints.change.${file.kind}`) }}</span>
                  </button>
                </li>
              </ul>
              <div class="diff-header">
                <h4 class="section-title">{{ t('checkpoints.diff') }}</h4>
                <code v-if="selectedPath" class="checkpoint-path" :title="selectedPath">{{ selectedPath }}</code>
              </div>
              <p v-if="diffLoading" class="files-state" role="status">{{ t('checkpoints.loadingDiff') }}</p>
              <div v-else-if="diffError" class="checkpoint-error status-message" role="alert">
                <span>{{ diffError }}</span>
                <button type="button" class="checkpoint-button button-text" @click="loadDiff(selectedPath)">{{ t('checkpoints.retry') }}</button>
              </div>
              <template v-else-if="diff">
                <p v-if="diff.truncated" class="checkpoint-warning status-message mb-2">
                  {{ t(diff.diff ? 'checkpoints.truncated' : 'checkpoints.diffTooLarge') }}
                </p>
                <p v-if="diff.binary" class="files-state">{{ t('checkpoints.binary') }}</p>
                <CheckpointDiffView v-else-if="diff.diff" :key="selectedPath" :diff="diff.diff" />
                <p v-else-if="!diff.truncated" class="files-state">{{ t('checkpoints.emptyDiff') }}</p>
              </template>
            </template>
            <p v-else class="files-state">{{ t('checkpoints.noFiles') }}</p>
          </template>
        </section>
      </div>
      <div v-if="selected && !selected.unavailable" class="checkpoint-footer">
        <p v-if="restoreGap" class="checkpoint-warning">{{ t('checkpoints.restoreGap') }}</p>
        <p v-else-if="!canRestoreChanges">{{ t('checkpoints.restoreCurrent') }}</p>
        <div class="rollback-actions">
          <button ref="undoButton" type="button" class="checkpoint-button" :disabled="!canUndo" @click="openConfirmation('undo')">
            <MdiIcon :path="mdiUndoVariant" :size="16" />{{ undoLabel }}
          </button>
          <button ref="restoreButton" type="button" class="checkpoint-button button-brand" :disabled="!canRestore" @click="openConfirmation('restore')">
            <MdiIcon :path="mdiRestore" :size="16" />{{ t('checkpoints.restore') }}
          </button>
        </div>
      </div>
    </section>
  </AppDialog>
</template>

<style scoped>
.checkpoint-browser,
.rollback-confirmation {
  color: var(--text-secondary);
  font-size: 0.8125rem;
}

.checkpoint-toolbar,
.diff-header {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.checkpoint-toolbar { margin-bottom: 0.75rem; }

.checkpoint-path,
.file-path {
  min-width: 0;
  overflow: hidden;
  font-family: var(--font-mono);
  font-size: 0.75rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.checkpoint-path { color: var(--text-secondary); }

.checkpoint-icon-button,
.checkpoint-button {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  gap: 0.375rem;
  min-height: 2.125rem;
  padding: 0.25rem 0.625rem;
  border: 1px solid var(--tool-section-border);
  border-radius: 0.5rem;
  color: var(--text-primary);
  cursor: pointer;
  font-size: 0.8125rem;
  font-weight: 600;
}

.checkpoint-icon-button { width: 2.125rem; padding: 0; }
.button-brand { background: var(--tool-running-bg); color: var(--tool-running-text); border-color: var(--tool-running-border); }
.button-danger { background: var(--tool-error-bg); color: var(--tool-error-text); border-color: var(--tool-error-border); }
.button-text { min-height: 1.625rem; padding: 0 0.375rem; color: inherit; border-color: transparent; }
.checkpoint-button:hover:not(:disabled),
.checkpoint-icon-button:hover:not([aria-disabled='true']) { background: var(--bg-hover); }
.checkpoint-button.button-danger:hover:not(:disabled) { background: var(--tool-error-bg-hover); }
.checkpoint-button:disabled,
.checkpoint-icon-button[aria-disabled='true'] { opacity: 0.5; cursor: default; }

.checkpoint-browser button:focus-visible,
.rollback-confirmation button:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 1px; }

.status-message {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.375rem;
  padding: 0.4rem 0.625rem;
  border-radius: 0.5rem;
  overflow-wrap: anywhere;
}

.checkpoint-toolbar ~ .status-message { margin-bottom: 0.625rem; }
.checkpoint-warning { color: var(--tool-pending-text); }
.checkpoint-warning.status-message { background: var(--warning-alpha-12); }
.checkpoint-error { color: var(--tool-error-text); background: var(--tool-error-bg); }
.status-success { color: var(--tool-success-text); background: var(--tool-success-bg); }
.running-dot { width: 0.45rem; height: 0.45rem; border-radius: 50%; background: currentColor; }

.browser-empty {
  display: grid;
  justify-items: center;
  align-content: center;
  gap: 0.375rem;
  min-height: 11rem;
  padding-inline: 1rem;
  text-align: center;
}

.empty-title { font-weight: 600; color: var(--text-primary); }

.checkpoint-layout {
  display: grid;
  grid-template-columns: minmax(12rem, 27%) minmax(0, 1fr);
  min-height: 16rem;
  overflow: hidden;
  border: 1px solid var(--tool-section-border);
  border-radius: 0.75rem;
}

.history-panel {
  min-width: 0;
  padding: 0.75rem;
  border-inline-end: 1px solid var(--tool-section-border);
  background: var(--tool-section-bg);
}

.detail-panel { min-width: 0; padding: 0.75rem; }
.section-title { margin-bottom: 0.5rem; color: var(--text-primary); font-size: 0.8125rem; font-weight: 650; }
.checkpoint-history { max-height: min(48dvh, 26rem); overflow: auto; padding: 0.125rem; }
.history-entry { position: relative; }
.history-entry:not(:last-child)::after {
  position: absolute;
  inset-block: 1.9rem -0.5rem;
  inset-inline-start: 1.415rem;
  width: 1px;
  background: var(--tool-section-border);
  content: '';
  pointer-events: none;
}

.history-button {
  position: relative;
  display: grid;
  grid-template-columns: 1.75rem minmax(0, 1fr);
  align-items: start;
  gap: 0.5rem;
  width: 100%;
  padding: 0.5rem;
  border: 1px solid transparent;
  border-radius: 0.5rem;
  color: var(--text-secondary);
  cursor: pointer;
  font-size: 0.75rem;
}

.history-button[aria-current='true'],
.file-button[aria-pressed='true'] { background: var(--bg-active); border-color: var(--border-default); }
.history-button:hover,
.file-button:hover { background: var(--bg-hover); }

.history-kind { color: var(--text-primary); font-weight: 650; }
.history-icon {
  position: relative;
  z-index: 1;
  display: grid;
  width: 1.75rem;
  height: 1.75rem;
  place-items: center;
  border-radius: 50%;
  background: var(--bg-main);
  color: var(--tool-running-text);
  box-shadow: inset 0 0 0 1px var(--tool-section-border);
}

.history-icon-undo,
.history-icon-restore { color: var(--tool-success-text); }
.history-note { color: var(--tool-pending-text); }
.files-state { padding: 0.5rem 0; }

.file-list { max-height: 8.5rem; overflow: auto; padding: 0.125rem; }
.file-button {
  display: grid;
  grid-template-columns: 1.4rem minmax(0, 1fr) auto;
  align-items: center;
  gap: 0.5rem;
  width: 100%;
  min-height: 2rem;
  padding: 0.125rem 0.375rem;
  border: 1px solid transparent;
  border-radius: 0.375rem;
  color: var(--text-primary);
  cursor: pointer;
  text-align: start;
}

.file-change {
  display: grid;
  width: 1.4rem;
  height: 1.4rem;
  place-items: center;
  border-radius: 0.3rem;
  font-size: 0.875rem;
  font-weight: 650;
}

.file-change-added { color: var(--tool-success-text); background: var(--tool-success-bg); }
.file-change-modified { color: var(--tool-running-text); background: var(--tool-running-bg); }
.file-change-deleted { color: var(--tool-error-text); background: var(--tool-error-bg); }
.file-kind { color: var(--text-secondary); font-size: 0.75rem; }
.diff-header { margin-block: 0.85rem 0.5rem; }
.diff-header .section-title { flex: none; margin: 0; }
.detail-panel > .status-message { margin-block-end: 0.5rem; }

.checkpoint-footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 0.5rem 1rem;
  padding-top: 0.75rem;
}

.checkpoint-footer > p { margin-inline-end: auto; font-size: 0.75rem; }
.rollback-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 0.5rem; }
.rollback-confirmation { max-width: 34rem; margin-inline: auto; padding-block: 0.25rem; }
.rollback-confirmation > .rollback-actions { margin-top: 1rem; }
.rollback-confirmation > .checkpoint-error { padding: 0.5rem; border-radius: 0.5rem; }
.confirm-content { display: flex; gap: 0.75rem; align-items: flex-start; }
.confirm-icon {
  display: grid;
  width: 2.25rem;
  height: 2.25rem;
  flex: none;
  place-items: center;
  border-radius: 0.625rem;
  background: var(--tool-error-bg);
  color: var(--tool-error-text);
}
.confirm-description { color: var(--text-primary); font-size: 0.9375rem; font-weight: 600; }

@media (prefers-reduced-motion: reduce) {
  :deep(.animate-spin) { animation: none; }
}

@media (max-width: 680px) {
  .checkpoint-layout { grid-template-columns: minmax(0, 1fr); }
  .history-panel { border-inline-end: 0; border-bottom: 1px solid var(--tool-section-border); }
  .checkpoint-history { max-height: 9.75rem; }
  .file-list { max-height: 7rem; }
  .history-button, .file-button { min-height: 2.5rem; }
  .checkpoint-footer .rollback-actions { flex: 1 1 100%; }
  .rollback-actions .checkpoint-button { min-height: 2.5rem; }
}
</style>
