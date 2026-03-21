import { apiClient } from './client'
import type { LLMConfig } from '@/types/settings'

export const llmAPI = {
  list: async () => (await apiClient.get<LLMConfig[]>('/api/llm-configs')).data,
  create: async (payload: Omit<LLMConfig, 'id'>) => (await apiClient.post<LLMConfig>('/api/llm-configs', payload)).data,
  remove: async (id: string) => apiClient.delete(`/api/llm-configs/${id}`),
}
