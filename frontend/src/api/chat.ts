import { apiClient } from './client'

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
  createdAt: string
}

export type ToolCallStatus = 'pending' | 'rejected' | 'executing' | 'completed' | 'error'

export interface SessionHistoryToolCallItem {
  toolCallId: string
  toolName: string
  command: string
  params: Record<string, string>
  status: ToolCallStatus
  requiresApproval: boolean
  output?: string
  error?: string
  startedAt?: string
  finishedAt?: string
}

export interface SessionHistoryPayload {
  messages: MessageItem[]
  toolCallsByAssistantMessageId: Record<string, SessionHistoryToolCallItem[]>
  hasMore: boolean
}

export interface SessionHistoryQuery {
  limit?: number
  before?: string
  after?: string
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
}

export const sessionAPI = {
  list: async () => (await apiClient.get<SessionItem[]>('/api/sessions')).data,
  create: async (name?: string) => (await apiClient.post<SessionItem>('/api/sessions', { name })).data,
  rename: async (id: string, name: string) => apiClient.patch(`/api/sessions/${id}/name`, { name }),
  remove: async (id: string) => apiClient.delete(`/api/sessions/${id}`),
  history: async (id: string, query: SessionHistoryQuery = {}) =>
    (
      await apiClient.get<SessionHistoryPayload>(`/api/sessions/${id}/messages`, {
        params: query,
      })
    ).data,
}
