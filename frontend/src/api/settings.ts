import { apiClient } from './client'
import type { AppSettings } from '@/types/settings'

export type { AppSettings, LLMConfig, MCPConfig, MessagePlatformConfig, SkillItem } from '@/types/settings'

type SettingsPayload = {
  language: 'zh-CN' | 'en-US'
  defaultModel?: string
  messagePlatformDefaultModel?: string
  webSearchApiKey?: string
}

export const settingAPI = {
  get: async (): Promise<AppSettings> => {
    const data = (await apiClient.get<SettingsPayload>('/api/settings')).data
    return {
      ...data,
      webSearchKey: data.webSearchApiKey,
    }
  },
  update: async (payload: Partial<AppSettings>) => {
    const wirePayload: Partial<SettingsPayload> = {
      ...payload,
      webSearchApiKey: payload.webSearchKey,
    }
    return apiClient.put('/api/settings', wirePayload)
  },
}
