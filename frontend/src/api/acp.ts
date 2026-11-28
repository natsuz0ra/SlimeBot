import { apiClient } from './client'

export interface ACPConfig {
  id: string
  name: string
  command: string
  args: string[]
  envKeys?: string[]
  defaultCwd?: string
  timeoutSec: number
  isEnabled: boolean
}

export type ACPConfigInput = Partial<Pick<ACPConfig, 'name' | 'command' | 'args' | 'defaultCwd' | 'timeoutSec' | 'isEnabled'>>
  & { env?: Record<string, string>; removeEnvKeys?: string[] }

export interface ACPConnectionTest {
  ok: boolean
  protocolVersion?: number
  agentInfo?: { name: string; title?: string; version?: string }
  capabilities?: { loadSession?: boolean; promptCapabilities?: { image?: boolean; embeddedContext?: boolean; audio?: boolean } }
  latencyMs: number
  error?: string
}

const configPath = (id: string) => `/api/acp-configs/${encodeURIComponent(id)}`
export const acpAPI = {
  list: async () => (await apiClient.get<ACPConfig[]>('/api/acp-configs')).data,
  create: async (input: ACPConfigInput) => (await apiClient.post<ACPConfig>('/api/acp-configs', input)).data,
  update: async (id: string, input: ACPConfigInput) => (await apiClient.put<ACPConfig>(configPath(id), input)).data,
  remove: (id: string) => apiClient.delete(configPath(id)),
  test: async (id: string) => (await apiClient.post<ACPConnectionTest>(`${configPath(id)}/test`, undefined, { timeout: 15000 })).data,
  reset: (id: string) => apiClient.post(`/api/sessions/${encodeURIComponent(id)}/acp/reset`, undefined, { timeout: 26000 }),
}
