import { apiClient } from './client'

export interface AppSettings {
  language: 'zh-CN' | 'en-US'
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
  config: string
  isEnabled: boolean
  createdAt?: string
  updatedAt?: string
}

export interface SkillItem {
  id: string
  name: string
  relativePath: string
  description: string
  uploadedAt: string
  createdAt?: string
  updatedAt?: string
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

export const skillsAPI = {
  list: async () => (await apiClient.get<SkillItem[]>('/api/skills')).data,
  upload: async (files: File[]) => {
    const formData = new FormData()
    files.forEach((file) => formData.append('files', file))
    return (await apiClient.post('/api/skills/upload', formData, { headers: { 'Content-Type': 'multipart/form-data' } })).data
  },
  remove: async (id: string) => apiClient.delete(`/api/skills/${id}`),
}
