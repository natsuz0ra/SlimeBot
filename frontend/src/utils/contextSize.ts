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
