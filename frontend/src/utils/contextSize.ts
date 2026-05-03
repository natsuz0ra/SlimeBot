export const CONTEXT_SIZE_MIN = 8_000
export const CONTEXT_SIZE_MAX = 1_000_000
export const CONTEXT_SIZE_DEFAULT = 1_000_000

export function clampContextSize(value: number | string | null | undefined): number {
  const numeric = typeof value === 'string' ? Number(value.replace(/,/g, '').trim()) : Number(value)
  if (!Number.isFinite(numeric) || numeric <= 0) return CONTEXT_SIZE_DEFAULT
  return Math.min(CONTEXT_SIZE_MAX, Math.max(CONTEXT_SIZE_MIN, Math.round(numeric)))
}

export function formatContextSize(value: number): string {
  const clamped = clampContextSize(value)
  if (clamped >= 1_000_000) return '1M'
  if (clamped % 1_000 === 0) return `${clamped / 1_000}K`
  return clamped.toLocaleString()
}

export function formatContextTokenCount(value: number): string {
  const count = Math.max(0, Math.round(value))
  if (count < 1_000) return String(count)
  if (count < 1_000_000) return `${(count / 1_000).toFixed(1).replace(/\.0$/, '')}K`
  return `${(count / 1_000_000).toFixed(1).replace(/\.0$/, '')}M`
}

export type ContextUsageEstimate = {
  usedTokens: number
  totalTokens: number
  usedPercent: number
  availablePercent: number
}

export function estimateContextTextTokens(text: string): number {
  const runes = Array.from(text || '').length
  if (runes === 0) return 0
  return Math.ceil(runes / 4)
}

export function estimateContextUsageWithText<T extends ContextUsageEstimate | null | undefined>(
  usage: T,
  text: string,
): T {
  if (!usage) return usage
  const delta = estimateContextTextTokens(text)
  if (delta <= 0) return usage
  const total = Math.max(0, Math.round(usage.totalTokens))
  const nextUsed = total > 0
    ? Math.min(total, Math.max(0, Math.round(usage.usedTokens) + delta))
    : Math.max(0, Math.round(usage.usedTokens) + delta)
  const usedPercent = total > 0
    ? Math.max(0, Math.min(100, Math.round((nextUsed / total) * 100)))
    : 0
  return {
    ...usage,
    usedTokens: nextUsed,
    usedPercent,
    availablePercent: 100 - usedPercent,
  }
}

export type ContextUsageTone = 'normal' | 'warning' | 'danger'

export function contextUsageTone(usedPercent: number): ContextUsageTone {
  if (usedPercent >= 90) return 'danger'
  if (usedPercent >= 70) return 'warning'
  return 'normal'
}

export function contextSizeToSlider(value: number): number {
  const clamped = clampContextSize(value)
  const min = Math.log(CONTEXT_SIZE_MIN)
  const max = Math.log(CONTEXT_SIZE_MAX)
  return Math.round(((Math.log(clamped) - min) / (max - min)) * 100)
}

export function sliderToContextSize(value: number | string): number {
  const slider = Math.min(100, Math.max(0, Number(value) || 0))
  const min = Math.log(CONTEXT_SIZE_MIN)
  const max = Math.log(CONTEXT_SIZE_MAX)
  return clampContextSize(Math.exp(min + (slider / 100) * (max - min)))
}
