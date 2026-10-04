export interface CooperativeSnapshot {
  agents: Array<{id: string; title: string; parentId: string; profile: string; depth: number; workspace: string}>
  turns: Array<{id: string; agentId: string; status: string; answer?: string; error?: string; createdAt: string; inputTokens: number; outputTokens: number}>
  tasks: Array<{id: string; title: string; status: string; ownerId?: string; revision: number; dependencies: string}>
  artifacts: Array<{id: string; agentId: string; status: string; workspace: string; report?: string; validation?: string}>
  approvals: Array<{id: string; agentId: string; payload: string; status: string}>
}
