import { apiClient } from './client'

export interface UsageSummary {
  groupKey: string; rootSessionId?: string; modelConfigId?: string; model?: string; workingDirectory?: string
  requestCount: number; inputTokens: number; outputTokens: number
  cacheCreationInputTokens: number; cacheReadInputTokens: number; totalTokens: number
}
export type UsageGroup = 'session' | 'model' | 'project'
export const usageAPI = {
  summary: async (groupBy: UsageGroup, signal?: AbortSignal): Promise<UsageSummary[]> => (await apiClient.get('/api/usage/summary', { params: { groupBy }, signal })).data.items,
}
