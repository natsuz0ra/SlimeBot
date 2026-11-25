export type PrivacyAction = 'mask' | 'warn' | 'block'
export interface PrivacyRule { kind: string; prefix?: string; pattern?: string }
export interface PrivacyConfig {
  enabled: boolean
  secrets: boolean
  personal: boolean
  kinds: Record<string, boolean>
  words: string[]
  ignoreCase: boolean
  wholeWords: boolean
  rules: PrivacyRule[]
  allow: string[]
  actions: Record<string, PrivacyAction>
  bypassProviderIds: string[]
  modelEnabled: boolean
  modelThreshold: number
  modelTimeoutSeconds: number
  mappingPersistence: boolean
  mappingTtlMinutes: number
}
export interface PrivacyMappingStatus {
  mode: 'memory' | 'encrypted' | 'unavailable'
  keyStatus: 'ready' | 'missing' | 'invalid' | 'mismatch'
  storageError?: boolean
}
export interface PrivacyModelStatus {
  installed: boolean
  installing: boolean
  running: boolean
  stage: string
  downloadedBytes: number
  totalBytes: number
  error?: string
  runtime: string
}
export interface PrivacyRecord {
  id: string
  time: string
  sessionId?: string
  source: string
  masked: number
  warned: number
  blocked: boolean
  restored: number
  kinds: Record<string, number>
  durationMs: number
  failed: boolean
}
export interface PrivacySnapshot {
  config: PrivacyConfig
  model: PrivacyModelStatus
  records: PrivacyRecord[]
  mappings: PrivacyMappingStatus
}
export interface PrivacyPreview {
  text: string
  hits: { kind: string; source: string; action: PrivacyAction; start: number; end: number; placeholder?: string }[]
  blocked: boolean
}
