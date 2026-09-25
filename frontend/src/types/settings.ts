export type ApprovalMode = 'standard' | 'auto_review' | 'auto'
export type ThinkingLevel = 'off' | 'low' | 'medium' | 'high' | 'max'
export type SandboxMode = 'read-only' | 'workspace-write' | 'danger-full-access'
export type SettingsTabKey = 'basic' | 'llm' | 'mcp' | 'skills' | 'agents' | 'memory' | 'platform' | 'update' | 'about'
export type MemoryTarget = 'memory' | 'user'

export interface AppSettings {
  language: 'zh-CN' | 'en-US'
  defaultModel?: string
  messagePlatformDefaultModel?: string
  messagePlatformThinkingLevel?: ThinkingLevel
  messagePlatformApprovalMode?: ApprovalMode
  webSearchKey?: string
  approvalMode?: ApprovalMode
  thinkingLevel?: ThinkingLevel
  sandboxMode?: SandboxMode
  sandboxWritableRoots?: string[]
  sandboxNetworkEnabled?: boolean
  sandboxNetworkAllowedDomains?: string[]
  cliSandboxMode?: SandboxMode
  cliSandboxWritableRoots?: string[]
  cliSandboxNetworkEnabled?: boolean
  cliSandboxNetworkAllowedDomains?: string[]
  memoryEnabled?: boolean
  memoryUserProfileEnabled?: boolean
  memoryCharLimit?: number
  memoryUserCharLimit?: number
  memoryNudgeInterval?: number
}

export interface MemoryTargetState {
  target: MemoryTarget
  entries: string[]
  usageChars: number
  charLimit: number
  entryCount: number
  enabled: boolean
}

export interface MemorySnapshot {
  memory: MemoryTargetState
  user: MemoryTargetState
  memoryEnabled: boolean
  memoryUserProfileEnabled: boolean
  memoryNudgeInterval: number
  memoryDirectory: string
}

export interface LLMConfig {
  id: string
  name: string
  providerId: string
  providerName?: string
  provider: 'openai' | 'anthropic' | 'deepseek'
  baseUrl: string
  model: string
  contextSize?: number
  contextSizeSource?: 'detected' | 'fallback' | 'manual'
}

export interface LLMProvider {
  id: string
  name: string
  protocol: 'openai' | 'anthropic' | 'deepseek'
  baseUrl: string
  hasApiKey: boolean
}

export interface DiscoveredModel {
  id: string
  name: string
  contextSize?: number
}

export interface MCPConfig {
  id: string
  name: string
  config: string
  isEnabled: boolean
  createdAt?: string
  updatedAt?: string
}

export type MCPToolLoadStatus = 'loaded' | 'error' | 'disabled'

export interface MCPToolParameterSummary {
  name: string
  type: string
  required: boolean
  description: string
}

export interface MCPToolItem {
  name: string
  functionName: string
  description: string
  parameterCount: number
  requiredParameters: string[]
  parameters: MCPToolParameterSummary[]
  inputSchema: Record<string, unknown>
}

export interface MCPToolListResponse {
  configId: string
  name: string
  isEnabled: boolean
  status: MCPToolLoadStatus
  toolCount: number
  loadedAt: string
  tools: MCPToolItem[]
  error: string
}

export interface SkillItem {
  id: string
  name: string
  relativePath: string
  description: string
  source: string
  sourceLabel: string
  provider: string
  readOnly: boolean
  enabled: boolean
  absolutePath?: string
  uploadedAt: string
  createdAt?: string
  updatedAt?: string
}

export interface MessagePlatformConfig {
  id: string
  platform: string
  displayName: string
  authConfigJson: string
  isEnabled: boolean
  createdAt?: string
  updatedAt?: string
}
