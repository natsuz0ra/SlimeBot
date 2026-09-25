import type { UpdateCheckResult, UpdateJobStatus, UpdatePhase } from '@/types/update'

export type UpdateTone = 'neutral' | 'info' | 'success' | 'danger'

const phases = new Set<UpdatePhase>([
  'idle',
  'checking',
  'downloading',
  'installing',
  'restarting',
  'ready',
  'succeeded',
  'failed',
])

export function normalizeUpdatePhase(value: unknown): UpdatePhase {
  return typeof value === 'string' && phases.has(value as UpdatePhase) ? value as UpdatePhase : 'idle'
}

export function normalizeUpdateCheck(payload: Partial<UpdateCheckResult> | null | undefined): UpdateCheckResult {
  return {
    current: payload?.current || '',
    latest: payload?.latest || '',
    updateAvailable: Boolean(payload?.updateAvailable),
    canApply: Boolean(payload?.canApply),
    reason: payload?.reason || '',
    releaseName: payload?.releaseName || '',
    releaseNotes: payload?.releaseNotes || '',
    releaseUrl: payload?.releaseUrl || '',
    publishedAt: payload?.publishedAt || '',
    assetName: payload?.assetName || '',
    manualHint: payload?.manualHint || '',
  }
}

export function normalizeUpdateJob(payload: Partial<UpdateJobStatus> | null | undefined): UpdateJobStatus {
  const downloadedBytes = normalizeNonNegativeNumber(payload?.downloadedBytes)
  const totalBytes = normalizeNonNegativeNumber(payload?.totalBytes)
  const progressPercent = Math.min(100, Math.max(0, Math.trunc(normalizeNonNegativeNumber(payload?.progressPercent))))
  return {
    phase: normalizeUpdatePhase(payload?.phase),
    current: payload?.current || '',
    target: payload?.target || '',
    message: payload?.message || '',
    error: payload?.error || '',
    manualHint: payload?.manualHint || '',
    downloadedBytes,
    totalBytes,
    progressPercent,
    updatedAt: payload?.updatedAt || '',
  }
}

function normalizeNonNegativeNumber(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : 0
}

export function isUpdateJobActive(phase: UpdatePhase): boolean {
  return phase === 'checking' || phase === 'downloading' || phase === 'installing' || phase === 'restarting'
}

export function updateJobStatusLabelKey(phase: UpdatePhase): string {
  if (phase === 'checking') return 'updateChecking'
  if (phase === 'downloading') return 'updateDownloading'
  if (phase === 'installing') return 'updateInstalling'
  if (phase === 'restarting') return 'updateRestarting'
  if (phase === 'ready') return 'updateReady'
  if (phase === 'succeeded') return 'updateSucceeded'
  if (phase === 'failed') return 'updateFailed'
  return ''
}

export function updateJobProgressPercent(job: UpdateJobStatus): number {
  if (job.phase === 'downloading') {
    return job.totalBytes > 0 ? job.progressPercent : 42
  }
  if (job.phase === 'installing' || job.phase === 'restarting' || job.phase === 'ready' || job.phase === 'succeeded' || job.phase === 'failed') {
    return 100
  }
  return 0
}

export function updatePhaseTone(phase: UpdatePhase): UpdateTone {
  if (phase === 'succeeded') return 'success'
  if (phase === 'failed') return 'danger'
  if (isUpdateJobActive(phase)) return 'info'
  return 'neutral'
}
