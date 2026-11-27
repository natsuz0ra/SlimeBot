export interface PrivacyConfig {
  enabled: boolean;
  secrets: boolean;
  personal: boolean;
  kinds: Record<string, boolean>;
  words: string[];
  ignoreCase: boolean;
  wholeWords: boolean;
  rules: { kind: string; prefix?: string; pattern?: string }[];
  allow: string[];
  actions: Record<string, 'mask' | 'warn' | 'block'>;
  bypassProviderIds: string[];
  modelEnabled: boolean;
  modelThreshold: number;
  modelTimeoutSeconds: number;
  mappingPersistence?: boolean;
  mappingTtlMinutes?: number;
}
export interface PrivacyMappingStatus {
  mode: 'memory' | 'encrypted' | 'unavailable';
  keyStatus: 'ready' | 'missing' | 'invalid' | 'mismatch';
  storageError?: boolean;
}
export interface PrivacySnapshot {
  config: PrivacyConfig;
  model: { installed: boolean; installing: boolean; running: boolean; stage: string; downloadedBytes: number; totalBytes: number; error?: string };
  records: { id: string; time: string; source: string; masked: number; warned: number; blocked: boolean; failed: boolean; restored: number; durationMs: number; kinds: Record<string, number> }[];
  mappings?: PrivacyMappingStatus;
}
export interface PrivacyPreview { text: string; hits: { kind: string; action: string }[]; blocked: boolean }
