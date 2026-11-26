import type { ContextUsageData } from '../api/chatSocket'
import type { UsageGroup, UsageSummary } from '../api/usage'
import { isMessagePlatformSessionId, platformSessionName } from './messagePlatformSessions'

export type UsageTimeRange = 'all' | 'today' | '7d' | '30d'

export interface UsageTimeWindow {
  since?: string
  until?: string
}

export interface OutputBudgetProgress {
  budget: number
  used: number
  remaining: number
  percent: number
  barPercent: number
  tone: 'normal' | 'warning' | 'danger'
  exceeded: boolean
}

export function resolveUsageTimeRange(range: UsageTimeRange, now: Date = new Date()): UsageTimeWindow {
  if (range === 'all') return {}
  const year = now.getFullYear()
  const month = now.getMonth()
  const day = now.getDate()
  const offsetDays = range === 'today' ? 0 : range === '7d' ? 6 : 29
  const start = new Date(year, month, day - offsetDays, 0, 0, 0, 0)
  return { since: start.toISOString() }
}

export function formatUsageRowLabel(
  group: UsageGroup,
  row: UsageSummary,
  labels: { unclassified: string; telegram: string },
): string {
  if (group === 'session') {
    const id = (row.rootSessionId || row.groupKey || '').trim()
    if (id && isMessagePlatformSessionId(id)) {
      return platformSessionName(id, labels.telegram)
    }
    if (row.sessionName?.trim()) {
      return row.sessionName.trim()
    }
    if (id) {
      return id.length > 14 ? `${id.slice(0, 8)}…` : id
    }
    return labels.unclassified
  }
  if (group === 'model') {
    if (row.model?.trim()) {
      return row.model.trim()
    }
    return row.groupKey?.trim() || labels.unclassified
  }
  return row.workingDirectory?.trim() || labels.unclassified
}

export function canJumpToUsageSession(row: UsageSummary): boolean {
  const id = (row.rootSessionId || '').trim()
  if (!id || !row.sessionExists) return false
  return !row.sessionKind || row.sessionKind === 'chat'
}

export function getOutputBudgetProgress(
  usage?: Pick<ContextUsageData, 'outputBudgetTokens' | 'outputBudgetUsed'> | null,
): OutputBudgetProgress | null {
  if (!usage) return null
  const budget = Math.round(Number(usage.outputBudgetTokens) || 0)
  if (budget <= 0) return null
  const used = Math.max(0, Math.round(Number(usage.outputBudgetUsed) || 0))
  const remaining = Math.max(0, budget - used)
  const exceeded = used >= budget
  const rawPercent = Math.round((used / budget) * 100)
  const percent = exceeded ? Math.max(100, rawPercent) : Math.min(99, Math.max(0, rawPercent))
  const barPercent = Math.min(100, Math.max(0, percent))
  const tone: OutputBudgetProgress['tone'] = exceeded
    ? 'danger'
    : rawPercent >= 80
      ? 'warning'
      : 'normal'
  return {
    budget,
    used,
    remaining,
    percent,
    barPercent,
    tone,
    exceeded,
  }
}

export function isBudgetExceededError(rawError: string | undefined | null): boolean {
  return /session output token budget exceeded/i.test(rawError || '')
}
