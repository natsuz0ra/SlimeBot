import { apiClient } from './client'
import type { ToolCallStatus } from '@/types/chat'

export type { ToolCallStatus } from '@/types/chat'

export const MESSAGE_PLATFORM_SESSION_ID = 'im-platform-session'

export interface SessionItem {
  id: string
  name: string
  updatedAt: string
}

export interface MessageItem {
  id: string
  sessionId: string
  role: 'user' | 'assistant' | 'system'
  content: string
  seq?: number
  isInterrupted?: boolean
  isStopPlaceholder?: boolean
  attachments?: MessageAttachmentItem[]
  createdAt: string
}

export interface MessageAttachmentItem {
  id?: string
  name: string
  ext: string
  sizeBytes: number
  mimeType: string
  category?: string
  iconType: string
}

export interface UploadedAttachmentItem extends MessageAttachmentItem {
  id: string
}

export interface SessionHistoryToolCallItem {
  toolCallId: string
  toolName: string
  command: string
  params: Record<string, string>
  status: ToolCallStatus
  requiresApproval: boolean
  parentToolCallId?: string
  subagentRunId?: string
  output?: string
  error?: string
  startedAt?: string
  finishedAt?: string
}

export interface SessionHistoryThinkingItem {
  thinkingId: string
  parentToolCallId?: string
  subagentRunId?: string
  content: string
  status: string
  startedAt?: string
  finishedAt?: string
  durationMs?: number
}

export interface SessionHistoryPayload {
  messages: MessageItem[]
  toolCallsByAssistantMessageId: Record<string, SessionHistoryToolCallItem[]>
  thinkingByAssistantMessageId: Record<string, SessionHistoryThinkingItem[]>
  hasMore: boolean
}

export interface SessionHistoryQuery {
  limit?: number
  before?: string
  beforeSeq?: number
  after?: string
  afterSeq?: number
}

export interface SessionListQuery {
  limit?: number
  offset?: number
  q?: string
}

export interface SessionListResponse {
  sessions: SessionItem[]
  hasMore: boolean
}

export interface ToolCallItem {
  toolCallId: string
  toolName: string
  command: string
  params: Record<string, string>
  preamble?: string
  requiresApproval: boolean
  status: ToolCallStatus
  output?: string
  error?: string
  /** Present on tools invoked inside a sub-agent run */
  parentToolCallId?: string
  subagentRunId?: string
  /** Streaming text from nested agent (parent run_subagent only) */
  subagentStream?: string
  /** Task summary from subagent_start */
  subagentTask?: string
  subagentThinking?: SubagentThinkingItem
}

export interface SubagentThinkingItem {
  content: string
  done: boolean
  startedAt?: number
  durationMs?: number
}

export const sessionAPI = {
  list: async (query: SessionListQuery = {}) =>
    (await apiClient.get<SessionListResponse>('/api/sessions', { params: query })).data,
  create: async (name?: string) => (await apiClient.post<SessionItem>('/api/sessions', { name })).data,
  rename: async (id: string, name: string) => apiClient.patch(`/api/sessions/${id}/name`, { name }),
  remove: async (id: string) => apiClient.delete(`/api/sessions/${id}`),
  history: async (id: string, query: SessionHistoryQuery = {}) =>
    (
      await apiClient.get<SessionHistoryPayload>(`/api/sessions/${id}/messages`, {
        params: query,
      })
    ).data,
  uploadAttachments: async (id: string, files: File[]) => {
    const formData = new FormData()
    files.forEach((file) => formData.append('files', file))
    return (
      await apiClient.post<{ items: UploadedAttachmentItem[] }>(`/api/sessions/${id}/attachments`, formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
    ).data
  },
}
