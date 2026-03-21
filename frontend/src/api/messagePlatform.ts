import { apiClient } from './client'
import type { MessagePlatformConfig } from '@/types/settings'

export const messagePlatformAPI = {
  list: async () => (await apiClient.get<MessagePlatformConfig[]>('/api/message-platform-configs')).data,
  create: async (payload: Omit<MessagePlatformConfig, 'id'>) =>
    (await apiClient.post<MessagePlatformConfig>('/api/message-platform-configs', payload)).data,
  update: async (id: string, payload: Omit<MessagePlatformConfig, 'id'>) =>
    apiClient.put(`/api/message-platform-configs/${id}`, payload),
  remove: async (id: string) => apiClient.delete(`/api/message-platform-configs/${id}`),
}
