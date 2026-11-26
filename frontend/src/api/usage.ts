import { apiClient } from './client'

export interface UsageSummary {
  groupKey: string
  rootSessionId?: string
  sessionName?: string
  sessionKind?: string
  sessionExists?: boolean
  modelConfigId?: string
  model?: string
  workingDirectory?: string
  requestCount: number
  inputTokens: number
  outputTokens: number
  cacheCreationInputTokens: number
  cacheReadInputTokens: number
  totalTokens: number
}

export type UsageGroup = 'session' | 'model' | 'project'

export interface UsageSummaryOptions {
  since?: string
  until?: string
  rootSessionId?: string
  signal?: AbortSignal
}

export const usageAPI = {
  summary: async (groupBy: UsageGroup, options?: UsageSummaryOptions | AbortSignal): Promise<UsageSummary[]> => {
    const resolved: UsageSummaryOptions = options && 'aborted' in options ? { signal: options } : (options || {})
    const params: Record<string, string> = { groupBy }
    if (resolved.since) params.since = resolved.since
    if (resolved.until) params.until = resolved.until
    if (resolved.rootSessionId) params.rootSessionId = resolved.rootSessionId
    return (await apiClient.get('/api/usage/summary', { params, signal: resolved.signal })).data.items
  },
}
