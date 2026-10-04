export interface CooperativeTurn {
  id: string;
  agentId: string;
  status: string;
  answer?: string;
  error?: string;
  createdAt: string;
  inputTokens: number;
  outputTokens: number;
  revision?: number;
}
export interface CooperativeSnapshot {
  roots?: Array<{
    id: string;
    status: string;
    consumed: number;
    reserved: number;
    budget: number;
  }>;
  agents: Array<{
    id: string;
    title: string;
    task?: string;
    parentId: string;
    profile: string;
    depth: number;
    workspace: string;
  }>;
  turns: CooperativeTurn[];
  tasks: Array<{
    id: string;
    title: string;
    description?: string;
    acceptance?: string;
    result?: string;
    status: string;
    ownerId?: string;
    revision: number;
    dependencies: string;
  }>;
  artifacts: Array<{
    id: string;
    agentId: string;
    status: string;
    workspace: string;
    report?: string;
    validation?: string;
  }>;
  approvals: Array<{
    id: string;
    agentId: string;
    payload: string;
    status: string;
  }>;
}
