import type { PrivacyConfig, PrivacyRecord } from '../types/privacy'

export const privacyCoreKinds = ['API_KEY', 'PRIVATE_KEY', 'TOKEN', 'PASSWORD', 'SECRET', 'EMAIL', 'PHONE', 'ID_CARD', 'BANK_CARD', 'PERSON', 'ADDRESS', 'ACCOUNT', 'TERM'] as const
export const privacyExtraKinds = ['SSN', 'PASSPORT', 'IBAN', 'BIRTHDAY', 'MAC', 'SERIAL', 'USERNAME', 'HOSTNAME', 'IP', 'BUCKET'] as const

export function privacyLines(text: string): string[] {
  return [...new Set(text.split(/\r?\n/).map(value => value.trim()).filter(Boolean))]
}

export function privacyKindEnabled(config: PrivacyConfig, kind: string): boolean {
  return config.kinds[kind] ?? (privacyCoreKinds as readonly string[]).includes(kind)
}

export function privacyTotals(records: PrivacyRecord[]) {
  return records.reduce((total, record) => ({
    masked: total.masked + record.masked,
    warned: total.warned + record.warned,
    blocked: total.blocked + Number(record.blocked),
  }), { masked: 0, warned: 0, blocked: 0 })
}

export function buildPrivacyConfig(config: PrivacyConfig, words: string, allow: string): PrivacyConfig {
  return { ...config, kinds: { ...config.kinds }, actions: { ...config.actions }, rules: config.rules.map(rule => ({ ...rule })), words: privacyLines(words), allow: privacyLines(allow), bypassProviderIds: [...config.bypassProviderIds] }
}
