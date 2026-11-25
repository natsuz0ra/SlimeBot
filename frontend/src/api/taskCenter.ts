import { apiClient } from './client'
import type { CooperativeAgent, CooperativeTurn } from './cooperative'
import type { ScheduledTask, ScheduledTaskRun } from './schedule'

export type BackgroundTaskStatus = 'running' | 'queued' | 'waking' | 'completed' | 'failed' | 'stopped' | 'interrupted'
export interface BackgroundTask {
  id: string; taskId: string; processId: string; sessionId: string
  command: string; workingDirectory?: string; status: BackgroundTaskStatus
  output?: string; error?: string; exitCode: number; autoWake: boolean
  startedAt: string; finishedAt?: string
}
export interface TaskCenterAgent extends CooperativeAgent { latestTurn?: CooperativeTurn }
export interface TaskCenterSnapshot {
  scheduledTasks: ScheduledTask[]
  scheduledRuns: ScheduledTaskRun[]
  agents: TaskCenterAgent[]
  backgroundTasks: BackgroundTask[]
}
export const taskCenterAPI = {
  snapshot: async (signal?: AbortSignal): Promise<TaskCenterSnapshot> => (await apiClient.get('/api/task-center', { signal })).data,
  backgroundAction: async (id: string, action: 'retry' | 'continue' | 'stop') => (await apiClient.post(`/api/background-tasks/${encodeURIComponent(id)}/actions`, { action })).data,
}
