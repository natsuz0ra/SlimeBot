export type ApprovalMode = 'standard' | 'auto'
export type SettingsTabKey = 'basic' | 'llm' | 'mcp' | 'skills' | 'platform' | 'about'

export interface AppSettings {
  language: 'zh-CN' | 'en-US'
  defaultModel?: string
  messagePlatformDefaultModel?: string
  webSearchKey?: string
  approvalMode?: ApprovalMode
}

export interface LLMConfig {
  id: string
  name: string
  provider: 'openai' | 'anthropic' | 'deepseek'
  baseUrl: string
  apiKey: string
  model: string
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

export interface SkillItem {
  id: string
  name: string
  relativePath: string
  description: string
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
