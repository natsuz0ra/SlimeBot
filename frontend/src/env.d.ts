/// <reference types="vite/client" />

declare const __APP_VERSION__: string

interface Window {
  slimebotDesktop?: {
    updateCheck: () => Promise<import('@/types/update').UpdateCheckResult>
    updateJob: () => Promise<import('@/types/update').UpdateJobStatus>
    updateApply: () => Promise<import('@/types/update').UpdateJobStatus>
  }
}
