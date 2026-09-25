<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { mdiAlertCircleOutline, mdiCheckCircleOutline, mdiDownload, mdiOpenInNew, mdiRefresh } from '@mdi/js'

import MdiIcon from '@/components/ui/MdiIcon.vue'
import { updateAPI } from '@/api/update'
import type { UpdateCheckResult, UpdateJobStatus } from '@/types/update'
import { isUpdateJobActive, normalizeUpdateJob, updateJobProgressPercent, updateJobStatusLabelKey, updatePhaseTone } from '@/utils/updateStatus'
import { renderMarkdown } from '@/utils/markdown'

const { t } = useI18n()
const isDesktop = Boolean(window.slimebotDesktop)
const emit = defineEmits<{
  updateCheckLoaded: [result: UpdateCheckResult]
}>()

const version = `v${__APP_VERSION__}`
const checkResult = ref<UpdateCheckResult | null>(null)
const job = ref<UpdateJobStatus>(normalizeUpdateJob(null))
const checking = ref(false)
const applying = ref(false)
const errorMessage = ref('')
let pollTimer: ReturnType<typeof setInterval> | null = null

const hasUpdate = computed(() => Boolean(checkResult.value?.updateAvailable))
const hasActiveUpdateJob = computed(() => isUpdateJobActive(job.value.phase))
const shouldShowTerminalJob = computed(() => job.value.phase === 'ready' || (!hasUpdate.value && (job.value.phase === 'succeeded' || job.value.phase === 'failed')))
const canApplyUpdate = computed(() => Boolean(checkResult.value?.canApply) && !checking.value && !applying.value && !hasActiveUpdateJob.value)
const releaseNotes = computed(() => {
  const notes = checkResult.value?.releaseNotes?.trim() || ''
  if (!notes) return ''
  return renderMarkdown(notes)
})
const statusTone = computed(() => {
  if (errorMessage.value || job.value.phase === 'failed') return 'danger'
  if (hasUpdate.value && !hasActiveUpdateJob.value) return 'info'
  if (checkResult.value && !hasUpdate.value && !hasActiveUpdateJob.value) return 'success'
  return updatePhaseTone(job.value.phase)
})
const statusIcon = computed(() => {
  if (statusTone.value === 'success') return mdiCheckCircleOutline
  if (statusTone.value === 'danger') return mdiAlertCircleOutline
  return mdiRefresh
})
const statusText = computed(() => {
  if (errorMessage.value) return errorMessage.value
  const labelKey = updateJobStatusLabelKey(job.value.phase)
  if ((hasActiveUpdateJob.value || shouldShowTerminalJob.value) && labelKey) return t(labelKey)
  if (!checkResult.value) return t('updateNotChecked')
  if (!hasUpdate.value) return t('updateAlreadyLatest')
  if (!checkResult.value.canApply) return checkResult.value.reason || t('updateManualOnly')
  return t('updateAvailable')
})
const manualHint = computed(() => job.value.manualHint || checkResult.value?.manualHint || '')
const showProgress = computed(() => hasActiveUpdateJob.value || shouldShowTerminalJob.value)
const progressPercent = computed(() => updateJobProgressPercent(job.value))
const progressStyle = computed(() => ({ width: `${progressPercent.value}%` }))
const progressLabel = computed(() => {
  if (job.value.phase === 'checking') return t('updateChecking')
  if (job.value.phase === 'downloading') {
    if (job.value.totalBytes > 0) {
      return t('updateDownloadProgress', {
        percent: progressPercent.value,
        downloaded: formatBytes(job.value.downloadedBytes),
        total: formatBytes(job.value.totalBytes),
      })
    }
    if (job.value.downloadedBytes > 0) {
      return t('updateDownloadUnknownTotal', { downloaded: formatBytes(job.value.downloadedBytes) })
    }
  }
  if (job.value.phase === 'installing') return t('updateInstalling')
  if (job.value.phase === 'restarting') return t('updateRestarting')
  if (job.value.phase === 'succeeded') return t('updateSucceeded')
  if (job.value.phase === 'failed') return job.value.error || t('updateFailed')
  return ''
})
const versionLine = computed(() => {
  if (!checkResult.value?.latest || !hasUpdate.value) return checkResult.value?.current || version
  return `${checkResult.value.current || version} → ${checkResult.value.latest}`
})
const currentVersionText = computed(() => checkResult.value?.current || version)
const latestVersionText = computed(() => checkResult.value?.latest || '-')
const heroTitle = computed(() => versionLine.value)
const heroDescription = computed(() => {
  if (checking.value) return t('updateChecking')
  if (errorMessage.value) return errorMessage.value
  if (!checkResult.value) return t('updateNotChecked')
  if (!hasUpdate.value) return t('updateAlreadyLatest')
  if (!checkResult.value.canApply) return checkResult.value.reason || t('updateManualOnly')
  return t('updateAvailable')
})
const primaryActionLabel = computed(() => {
  if (hasUpdate.value) return applying.value ? t('updateApplying') : job.value.phase === 'ready' ? t('updateInstallRestart') : t('downloadUpdate')
  return checking.value ? t('updateChecking') : t('checkUpdate')
})
const primaryActionIcon = computed(() => hasUpdate.value ? mdiDownload : mdiRefresh)
const showManualCommand = computed(() => Boolean(manualHint.value || (checkResult.value && hasUpdate.value && !checkResult.value.canApply)))
const releaseSummaryVisible = computed(() => Boolean(releaseNotes.value))

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

async function loadJob(pollIfActive = false) {
  job.value = await updateAPI.job()
  applying.value = isUpdateJobActive(job.value.phase)
  if (applying.value && pollIfActive) {
    startPolling()
    return
  }
  if (!applying.value) {
    stopPolling()
  }
}

function startPolling() {
  stopPolling()
  pollTimer = setInterval(() => {
    void loadJob().catch(() => {
      applying.value = false
      stopPolling()
    })
  }, 1500)
}

async function checkUpdate(force = true) {
  checking.value = true
  errorMessage.value = ''
  try {
    checkResult.value = await updateAPI.check(force)
    emit('updateCheckLoaded', checkResult.value)
    await loadJob(true)
  } catch (err: unknown) {
    const response = err as { response?: { data?: { error?: string } } }
    errorMessage.value = response.response?.data?.error || t('updateCheckFailed')
  } finally {
    checking.value = false
  }
}

async function applyUpdate() {
  if (!checkResult.value?.latest || !canApplyUpdate.value) return
  applying.value = true
  errorMessage.value = ''
  try {
    job.value = await updateAPI.apply(checkResult.value.latest)
    startPolling()
  } catch (err: unknown) {
    const response = err as { response?: { data?: { error?: string } } }
    errorMessage.value = response.response?.data?.error || t('updateApplyFailed')
    applying.value = false
  }
}

async function runPrimaryUpdateAction() {
  if (hasUpdate.value) {
    await applyUpdate()
    return
  }
  await checkUpdate(true)
}

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  return `${(value / 1024 / 1024).toFixed(1)} MiB`
}

onMounted(() => {
  void checkUpdate(false)
})

onUnmounted(stopPolling)
</script>

<template>
  <div>
    <p class="section-label">{{ t('updateSettings') }}</p>

    <div class="update-card">
      <div class="update-hero-card">
        <div class="update-hero-main">
          <div class="update-hero-copy">
            <div class="update-version-line">{{ heroTitle }}</div>
            <div class="update-hero-desc">{{ heroDescription }}</div>
          </div>

          <span class="update-status-pill" :data-tone="statusTone">
            <MdiIcon :path="statusIcon" :size="15" />
            <span>{{ statusText }}</span>
          </span>
        </div>

        <div v-if="showProgress" class="update-progress" :data-indeterminate="job.totalBytes <= 0 && job.phase === 'downloading'">
          <div class="update-progress-bar">
            <span :style="progressStyle"></span>
          </div>
          <div class="update-progress-label">{{ progressLabel }}</div>
        </div>

        <div class="update-actions">
          <button
            type="button"
            class="update-btn update-btn-primary"
            :disabled="hasUpdate ? !canApplyUpdate : checking || applying"
            @click="runPrimaryUpdateAction"
          >
            <MdiIcon :path="primaryActionIcon" :size="15" />
            <span>{{ primaryActionLabel }}</span>
          </button>
        </div>
      </div>

      <div class="update-info-grid">
        <div class="update-info-tile">
          <span>{{ t('appVersion') }}</span>
          <strong>{{ currentVersionText }}</strong>
        </div>
        <div class="update-info-tile">
          <span>{{ t('updateLatestVersion') }}</span>
          <strong>{{ latestVersionText }}</strong>
        </div>
        <a
          v-if="checkResult?.releaseUrl"
          class="update-info-tile update-info-link"
          :href="checkResult.releaseUrl"
          target="_blank"
          rel="noreferrer"
        >
          <span>{{ t('updateReleasePage') }}</span>
          <strong>
            <MdiIcon :path="mdiOpenInNew" :size="15" />
            {{ t('open') }}
          </strong>
        </a>
      </div>

      <div v-if="releaseSummaryVisible" class="update-notes-panel">
        <div class="update-panel-title">{{ checkResult?.releaseName || t('updateReleasePage') }}</div>
        <div class="update-notes bubble-markdown" v-html="releaseNotes"></div>
      </div>

      <div v-if="showManualCommand" class="update-manual">
        <span>{{ isDesktop ? t('updateManualOnly') : t('updateManualHint') }}</span>
        <code>{{ manualHint || checkResult?.reason || '-' }}</code>
      </div>
    </div>
  </div>
</template>

<style scoped>
.update-card {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.update-hero-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 16px;
  border-radius: 12px;
  background: linear-gradient(135deg, var(--card-bg) 0%, var(--primary-alpha-04) 58%, var(--primary-alpha-08) 100%);
  border: 1px solid var(--primary-alpha-15);
}

.update-hero-main,
.update-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.update-hero-copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.update-version-line {
  color: var(--text-primary);
  font-size: 22px;
  line-height: 1.25;
  font-weight: 750;
  overflow-wrap: anywhere;
}

.update-hero-desc {
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.update-status-pill {
  min-width: fit-content;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 42%;
  padding: 6px 9px;
  border-radius: 999px;
  font-size: 12px;
  line-height: 1.2;
  color: var(--text-secondary);
  background: var(--hover-bg);
  border: 1px solid var(--card-border);
}

.update-status-pill span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.update-status-pill[data-tone="info"] {
  color: var(--sb-brand);
  background: var(--primary-alpha-08);
  border-color: var(--primary-alpha-15);
}

.update-status-pill[data-tone="success"] {
  color: #16a34a;
  background: rgba(34, 197, 94, 0.1);
  border-color: rgba(34, 197, 94, 0.18);
}

.update-status-pill[data-tone="danger"] {
  color: #dc2626;
  background: rgba(239, 68, 68, 0.1);
  border-color: rgba(239, 68, 68, 0.18);
}

.update-info-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.update-info-tile {
  min-width: 0;
  min-height: 66px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 6px;
  padding: 12px;
  border-radius: 10px;
  color: var(--text-primary);
  background: var(--hover-bg);
  border: 1px solid var(--card-border);
  text-decoration: none;
}

.update-info-tile span {
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.25;
}

.update-info-tile strong {
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--text-primary);
  font-size: 14px;
  line-height: 1.35;
  font-weight: 650;
  overflow-wrap: anywhere;
}

.update-info-link {
  transition: border-color 0.15s, background-color 0.15s, color 0.15s;
}

.update-info-link:hover {
  color: var(--sb-brand);
  background: var(--primary-alpha-08);
  border-color: var(--primary-alpha-20);
}

.update-info-link:hover strong {
  color: var(--sb-brand);
}

.update-notes-panel {
  display: flex;
  flex-direction: column;
  gap: 9px;
  padding: 12px;
  border-radius: 12px;
  background: var(--hover-bg);
  border: 1px solid var(--card-border);
}

.update-panel-title {
  color: var(--text-primary);
  font-size: 13px;
  line-height: 1.35;
  font-weight: 650;
}

.update-notes {
  max-height: 150px;
  overflow: auto;
  margin: 0;
  padding: 0;
  border-radius: 0;
  color: var(--text-secondary);
  background: transparent;
  border: none;
  font-size: 12px;
  line-height: 1.55;
  word-break: break-word;
}

.update-notes :deep(h1),
.update-notes :deep(h2),
.update-notes :deep(h3) {
  font-size: 13px;
  line-height: 1.35;
  margin: 0 0 6px;
}

.update-notes :deep(ul),
.update-notes :deep(ol) {
  margin: 6px 0;
  padding-left: 18px;
}

.update-notes :deep(p) {
  margin: 6px 0;
}

.update-manual {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px;
  border-radius: 12px;
  color: var(--text-secondary);
  background: var(--primary-alpha-04);
  border: 1px dashed var(--primary-alpha-20);
  font-size: 12px;
}

.update-manual code {
  display: block;
  overflow-x: auto;
  padding: 9px 10px;
  border-radius: 9px;
  color: var(--text-primary);
  background: var(--hover-bg);
  border: 1px solid var(--card-border);
  font-family: var(--font-mono);
}

.update-progress {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.update-progress-bar {
  height: 8px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--hover-bg);
  border: 1px solid var(--card-border);
}

.update-progress-bar span {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--sb-brand);
  transition: width 0.2s ease;
}

.update-progress[data-indeterminate="true"] .update-progress-bar span {
  animation: update-progress-pulse 1.2s ease-in-out infinite alternate;
}

.update-progress-label {
  color: var(--text-secondary);
  font-size: 12px;
  line-height: 1.35;
}

@keyframes update-progress-pulse {
  from {
    transform: translateX(-30%);
  }
  to {
    transform: translateX(140%);
  }
}

.update-btn {
  min-height: 34px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  padding: 0 12px;
  border-radius: 10px;
  font-size: 13px;
  line-height: 1.2;
  border: 1px solid transparent;
  transition: all 0.15s;
  cursor: pointer;
}

.update-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.update-btn-primary {
  color: white;
  background: var(--sb-brand);
}

.update-btn-primary:not(:disabled):hover {
  background: var(--sb-brand-hover);
}

.update-btn-secondary {
  color: var(--text-primary);
  background: var(--hover-bg);
  border-color: var(--card-border);
}

.update-btn-secondary:not(:disabled):hover {
  background: var(--primary-alpha-08);
  border-color: var(--primary-alpha-20);
}

@media (max-width: 640px) {
  .update-hero-main,
  .update-actions {
    align-items: stretch;
    flex-direction: column;
  }

  .update-version-line {
    font-size: 18px;
  }

  .update-status-pill {
    max-width: 100%;
    justify-content: center;
  }

  .update-info-grid {
    grid-template-columns: 1fr;
  }

  .update-btn {
    width: 100%;
  }

}
</style>
