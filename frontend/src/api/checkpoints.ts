import { apiClient } from './client'

export type CheckpointRollbackMode = 'undo' | 'restore'

export interface Checkpoint {
  id: string
  requestId?: string
  userMessageId?: string
  createdAt: string
  kind: 'turn' | CheckpointRollbackMode
  sourceId?: string
  fileCount: number
  limited?: boolean
  unavailable?: boolean
}

export interface CheckpointFile {
  path: string
  kind: 'added' | 'modified' | 'deleted'
}

export interface CheckpointDetail extends Checkpoint {
  files: CheckpointFile[]
}

export interface CheckpointDiff {
  diff: string
  binary: boolean
  truncated: boolean
}

function checkpointURL(sessionId: string, checkpointId?: string) {
  const base = `/api/sessions/${encodeURIComponent(sessionId)}/checkpoints`
  return checkpointId ? `${base}/${encodeURIComponent(checkpointId)}` : base
}

export const checkpointAPI = {
  list: async (sessionId: string, signal: AbortSignal) =>
    (await apiClient.get<{ checkpoints: Checkpoint[] }>(checkpointURL(sessionId), { signal })).data.checkpoints,
  detail: async (sessionId: string, checkpointId: string, signal: AbortSignal) =>
    (await apiClient.get<CheckpointDetail>(checkpointURL(sessionId, checkpointId), { signal })).data,
  diff: async (sessionId: string, checkpointId: string, path: string, signal: AbortSignal) =>
    (await apiClient.get<CheckpointDiff>(`${checkpointURL(sessionId, checkpointId)}/diff`, { params: { path }, signal })).data,
  rollback: async (sessionId: string, checkpointId: string, mode: CheckpointRollbackMode) => {
    await apiClient.post(`${checkpointURL(sessionId, checkpointId)}/rollback`, { mode })
  },
}
