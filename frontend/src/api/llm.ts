import { apiClient } from './client'
import type { DiscoveredModel, LLMConfig, LLMProvider } from '@/types/settings'

type ModelPayload = { providerId: string; name: string; model: string; contextSize: number; contextSizeSource: 'auto' | 'detected' | 'fallback' | 'manual' }
type ProviderPayload = { name: string; protocol: LLMProvider['protocol']; baseUrl: string; apiKey: string; clearApiKey?: boolean }

export const llmAPI = {
  list: async () => (await apiClient.get<LLMConfig[]>('/api/llm-configs')).data,
  create: async (payload: ModelPayload) => (await apiClient.post<LLMConfig>('/api/llm-configs', payload)).data,
  update: async (id: string, payload: ModelPayload) => apiClient.put(`/api/llm-configs/${id}`, payload),
  remove: async (id: string) => apiClient.delete(`/api/llm-configs/${id}`),
  providers: async () => (await apiClient.get<LLMProvider[]>('/api/llm-providers')).data,
  createProvider: async (payload: ProviderPayload) => (await apiClient.post<LLMProvider>('/api/llm-providers', payload)).data,
  updateProvider: async (id: string, payload: ProviderPayload) => apiClient.put(`/api/llm-providers/${id}`, payload),
  removeProvider: async (id: string) => apiClient.delete(`/api/llm-providers/${id}`),
  discover: async (id: string) => (await apiClient.post<DiscoveredModel[]>(`/api/llm-providers/${id}/discover`)).data,
}
