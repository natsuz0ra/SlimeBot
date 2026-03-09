import { apiClient } from './client'

export interface SessionItem {
  id: string
  name: string
  modelConfigId?: string
  updatedAt: string
}

export interface MessageItem {
  id: string
  sessionId: string
  role: 'user' | 'assistant' | 'system'
  content: string
  createdAt: string
}

export interface AppSettings {
  language: 'zh-CN' | 'en-US'
  defaultModel?: string
}

export interface LLMConfig {
  id: string
  name: string
  baseUrl: string
  apiKey: string
  model: string
}

export interface MCPConfig {
  id: string
  name: string
  serverUrl: string
  authType: string
  authValue: string
  isEnabled: boolean
}

export const sessionAPI = {
  list: async () => (await apiClient.get<SessionItem[]>('/api/sessions')).data,
  create: async (name?: string) => (await apiClient.post<SessionItem>('/api/sessions', { name })).data,
  rename: async (id: string, name: string) => apiClient.patch(`/api/sessions/${id}/name`, { name }),
  remove: async (id: string) => apiClient.delete(`/api/sessions/${id}`),
  history: async (id: string) => (await apiClient.get<MessageItem[]>(`/api/sessions/${id}/messages`)).data,
  setModel: async (id: string, modelConfigId: string) => apiClient.put(`/api/sessions/${id}/model`, { modelConfigId }),
}

export const settingAPI = {
  get: async () => (await apiClient.get<AppSettings>('/api/settings')).data,
  update: async (payload: Partial<AppSettings>) => apiClient.put('/api/settings', payload),
}

export const llmAPI = {
  list: async () => (await apiClient.get<LLMConfig[]>('/api/llm-configs')).data,
  create: async (payload: Omit<LLMConfig, 'id'>) => (await apiClient.post<LLMConfig>('/api/llm-configs', payload)).data,
  remove: async (id: string) => apiClient.delete(`/api/llm-configs/${id}`),
}

export const mcpAPI = {
  list: async () => (await apiClient.get<MCPConfig[]>('/api/mcp-configs')).data,
  create: async (payload: Omit<MCPConfig, 'id'>) => (await apiClient.post<MCPConfig>('/api/mcp-configs', payload)).data,
  update: async (id: string, payload: Omit<MCPConfig, 'id'>) => apiClient.put(`/api/mcp-configs/${id}`, payload),
  remove: async (id: string) => apiClient.delete(`/api/mcp-configs/${id}`),
}
