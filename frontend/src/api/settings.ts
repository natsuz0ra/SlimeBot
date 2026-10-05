import { apiClient } from './client'
import type { AppSettings, ProxyTestResult } from '@/types/settings'
import { buildSettingsPayload, normalizeSettingsPayload, type SettingsPayload } from '@/utils/settingsPayload'

export type { AppSettings, LLMConfig, MCPConfig, MessagePlatformConfig, SkillItem } from '@/types/settings'

export const settingAPI = {
  get: async (): Promise<AppSettings> => {
    const data = (await apiClient.get<SettingsPayload>('/api/settings')).data
    return normalizeSettingsPayload(data)
  },
  update: async (payload: Partial<AppSettings>) => {
    return apiClient.put('/api/settings', buildSettingsPayload(payload))
  },
  testProxy: async (proxyUrl: string): Promise<ProxyTestResult> => {
    return (await apiClient.post<ProxyTestResult>('/api/settings/proxy/test', { proxyUrl })).data
  },
}
