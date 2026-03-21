import { apiClient } from './client'
import type { MCPConfig } from '@/types/settings'

export const mcpAPI = {
  list: async () => (await apiClient.get<MCPConfig[]>('/api/mcp-configs')).data,
  create: async (payload: Omit<MCPConfig, 'id'>) => (await apiClient.post<MCPConfig>('/api/mcp-configs', payload)).data,
  update: async (id: string, payload: Omit<MCPConfig, 'id'>) => apiClient.put(`/api/mcp-configs/${id}`, payload),
  remove: async (id: string) => apiClient.delete(`/api/mcp-configs/${id}`),
}
