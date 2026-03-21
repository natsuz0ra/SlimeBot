import { apiClient } from './client'
import type { AppSettings } from '@/types/settings'

export type { AppSettings, LLMConfig, MCPConfig, MessagePlatformConfig, SkillItem } from '@/types/settings'

export const settingAPI = {
  get: async () => (await apiClient.get<AppSettings>('/api/settings')).data,
  update: async (payload: Partial<AppSettings>) => apiClient.put('/api/settings', payload),
}
