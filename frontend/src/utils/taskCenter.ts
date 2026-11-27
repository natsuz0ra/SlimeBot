import type { BackgroundTask, TaskCenterAgent } from '@/api/taskCenter'
import type { ScheduledTaskRun } from '@/api/schedule'

export type TaskCenterKind = 'background' | 'agent' | 'schedule'
export type TaskCenterStatus = 'running' | 'ok' | 'error' | 'interrupted'
export interface TaskCenterRow {
  id: string; sourceId: string; kind: TaskCenterKind; title: string; description: string
  status: TaskCenterStatus; sessionId: string; startedAt: string; finishedAt?: string
  background?: BackgroundTask; agent?: TaskCenterAgent; run?: ScheduledTaskRun
}
export function taskCenterStatus(status: string): TaskCenterStatus {
  if (['running', 'queued', 'waking', 'scheduled', 'waiting', 'pending', 'claimed'].includes(status)) return 'running'
  if (['completed', 'ok', 'succeeded', 'done'].includes(status)) return 'ok'
  if (['failed', 'error', 'rejected', 'stopped'].includes(status)) return 'error'
  return 'interrupted'
}
export function taskCenterRows(background: BackgroundTask[], agents: TaskCenterAgent[], runs: ScheduledTaskRun[]): TaskCenterRow[] {
  const rows: TaskCenterRow[] = [
    ...background.map(item => ({ id: `background:${item.id}`, sourceId: item.id, kind: 'background' as const, title: item.command, description: item.error || item.output || item.workingDirectory || '', status: taskCenterStatus(item.status), sessionId: item.sessionId, startedAt: item.startedAt, finishedAt: item.finishedAt, background: item })),
    ...agents.map(item => ({ id: `agent:${item.id}`, sourceId: item.id, kind: 'agent' as const, title: item.title, description: item.latestTurn?.error || item.latestTurn?.answer || item.task, status: taskCenterStatus(item.latestTurn?.status || 'waiting'), sessionId: item.rootId, startedAt: item.latestTurn?.createdAt || item.createdAt, finishedAt: item.latestTurn?.finishedAt, agent: item })),
    ...runs.map(item => ({ id: `schedule:${item.id}`, sourceId: item.id, kind: 'schedule' as const, title: item.taskName, description: item.error || item.answer || '', status: taskCenterStatus(item.status), sessionId: item.sessionId, startedAt: item.startedAt, finishedAt: item.finishedAt, run: item })),
  ]
  return rows.sort((left, right) => {
    const leftTime = Date.parse(left.startedAt)
    const rightTime = Date.parse(right.startedAt)
    return (Number.isFinite(rightTime) ? rightTime : 0) - (Number.isFinite(leftTime) ? leftTime : 0)
  })
}
