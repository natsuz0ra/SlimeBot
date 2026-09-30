import { apiClient } from './client'

export type TaskRunStatus = 'running' | 'ok' | 'error' | 'interrupted'
export interface ScheduledTask {
  id: string
  name: string
  prompt: string
  status: string
  scheduleKind: 'once' | 'interval' | 'cron'
  intervalMinutes?: number
  cronExpr?: string
  runAt?: string
  nextRunAt?: string
  completedRuns: number
}
export interface ScheduledTaskRun {
  id: string
  taskId: string
  taskName: string
  sessionId: string
  requestId: string
  status: TaskRunStatus
  answer?: string
  error?: string
  startedAt: string
  finishedAt?: string
}
export interface TaskRunPage {
  runs: ScheduledTaskRun[]
  hasMore: boolean
}
export const scheduleAPI = {
  tasks: async (signal?: AbortSignal): Promise<ScheduledTask[]> => (
    await apiClient.get('/api/scheduled-tasks', { signal })
  ).data,
  runs: async (taskId: string, status: string, offset = 0, signal?: AbortSignal): Promise<TaskRunPage> => (
    await apiClient.get('/api/scheduled-task-runs', { params: { taskId, status, offset, limit: 20 }, signal })
  ).data,
  run: async (id: string, signal?: AbortSignal): Promise<ScheduledTaskRun> => (
    await apiClient.get(`/api/scheduled-task-runs/${encodeURIComponent(id)}`, { signal })
  ).data,
}
