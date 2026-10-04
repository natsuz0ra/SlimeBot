import { apiClient } from './client'

export interface CooperativeAgent {
  id: string; rootId: string; parentId: string; title: string; task: string
  profile: string; mode: string; contextMode: string; modelId: string
  workspace: string; depth: number; queued?: number; createdAt: string; requestId: string
}
export interface CooperativeTurn {
  id: string; agentId: string; requestId: string; status: string; stopReason?: string
  answer?: string; error?: string; activity?: string; revision: number
  inputTokens: number; outputTokens: number; createdAt: string; finishedAt?: string
}
export interface CooperativeTask {
  id: string; title: string; description: string; acceptance: string; status: string
  ownerId?: string; revision: number; dependencies: string; result?: string; artifactId?: string
}
export interface CooperativeArtifact {
  id: string; agentId: string; taskId?: string; workspace: string; parentWorkspace: string
  status: string; report?: string; validation?: string; validationCommand?: string; resultCommit?: string; validatedCommit?: string; createdAt: string
}
export interface CooperativeApproval {
  id: string; agentId: string; toolCallId: string; status: string; payload: string
}
export interface CooperativeEvent { seq: number; agentId: string; turnId?: string; kind: string; payload: string }
export interface CooperativeSnapshot {
  roots?: Array<{id: string; status: string; consumed: number; reserved: number; budget: number}>
  agents: CooperativeAgent[]; turns: CooperativeTurn[]; tasks: CooperativeTask[]
  artifacts: CooperativeArtifact[]; approvals: CooperativeApproval[]; events: CooperativeEvent[]
}
export const cooperativeAPI = {
  snapshot: async (id: string, after = 0, signal?: AbortSignal, wait = 0): Promise<CooperativeSnapshot> => (
    await apiClient.get(`/api/sessions/${encodeURIComponent(id)}/cooperative`, { params: { after, wait }, signal, timeout: Math.max(15000, wait + 5000) })
  ).data,
  stop: async (id: string) => (await apiClient.post(`/api/sessions/${encodeURIComponent(id)}/cooperative/stop`)).data,
  turns: async (id: string): Promise<CooperativeTurn[]> => (await apiClient.get(`/api/agents/${encodeURIComponent(id)}/turns`)).data,
  validate: async (id: string, root: string, command: string) => (await apiClient.post(`/api/agent-artifacts/${encodeURIComponent(id)}/actions`, {rootId: root, action: 'validate', command})).data,
  action: async (id: string, input: Record<string, unknown>) => (
    await apiClient.post(`/api/agents/${encodeURIComponent(id)}/actions`, input)
  ).data,
  task: async (root: string, input: Record<string, unknown>) => (
    await apiClient.post(`/api/sessions/${encodeURIComponent(root)}/cooperative/tasks`, input)
  ).data,
  approve: async (id: string, approved: boolean) => (
    await apiClient.post(`/api/agent-approvals/${encodeURIComponent(id)}`, { approved })
  ).data,
  artifact: async (id: string, root: string, integrate = false) => (
    await apiClient.post(`/api/agent-artifacts/${encodeURIComponent(id)}/actions`, { rootId: root, action: integrate ? 'integrate' : 'inspect' })
  ).data as { diff?: string; artifact: CooperativeArtifact },
}
