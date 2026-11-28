import { apiClient } from './client'

export interface WorkspaceTreeEntry {
  name: string
  path: string
  isDirectory: boolean
  isSymlink?: boolean
}

export interface WorkspaceFile {
  path: string
  content?: string
  language?: string
  size: number
  binary: boolean
  truncated: boolean
}

export interface WorkspaceGitChange {
  path: string
  oldPath?: string
  staged?: string
  unstaged?: string
  untracked?: boolean
}

export interface WorkspaceGitStatus {
  isRepo: boolean
  branch?: string
  upstream?: string
  ahead: number
  behind: number
  changes: WorkspaceGitChange[]
  truncated: boolean
}

export interface WorkspaceGitDiff {
  path: string
  diff?: string
  binary: boolean
  truncated: boolean
}

const endpoint = (sessionId: string, method: string) =>
  `/api/sessions/${encodeURIComponent(sessionId)}/workspace/${method}`

export const workspaceAPI = {
  status: async (id: string, signal: AbortSignal) =>
    (await apiClient.get<WorkspaceGitStatus>(endpoint(id, 'git-status'), { signal })).data,
  diff: async (id: string, path: string, staged: boolean, signal: AbortSignal) =>
    (await apiClient.get<WorkspaceGitDiff>(endpoint(id, 'git-diff'), { params: { path, staged }, signal })).data,
  tree: async (id: string, path: string, showHidden: boolean, signal: AbortSignal) =>
    (await apiClient.get<WorkspaceTreeEntry[]>(endpoint(id, 'tree'), { params: { path, showHidden }, signal })).data,
  file: async (id: string, path: string, signal: AbortSignal) =>
    (await apiClient.get<WorkspaceFile>(endpoint(id, 'file'), { params: { path }, signal })).data,
}
