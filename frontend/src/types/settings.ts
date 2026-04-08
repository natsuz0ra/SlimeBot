export interface AppSettings {
  language: 'zh-CN' | 'en-US'
  defaultModel?: string
  messagePlatformDefaultModel?: string
  webSearchKey?: string
}

export interface LLMConfig {
  id: string
  name: string
  provider: 'openai' | 'anthropic'
  baseUrl: string
  apiKey: string
  model: string
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
