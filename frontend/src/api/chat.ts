import { apiClient } from './client'

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

export interface ToolCallItem {
  toolCallId: string
  toolName: string
  command: string
  params: Record<string, string>
  preamble?: string
  status: ToolCallStatus
  output?: string
  error?: string
}

export const sessionAPI = {
  list: async () => (await apiClient.get<SessionItem[]>('/api/sessions')).data,
  create: async (name?: string) => (await apiClient.post<SessionItem>('/api/sessions', { name })).data,
  rename: async (id: string, name: string) => apiClient.patch(`/api/sessions/${id}/name`, { name }),
  remove: async (id: string) => apiClient.delete(`/api/sessions/${id}`),
  history: async (id: string) => (await apiClient.get<MessageItem[]>(`/api/sessions/${id}/messages`)).data,
}
