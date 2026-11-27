import test from 'node:test'
import assert from 'node:assert/strict'

import {
  canJumpToUsageSession,
  formatUsageRowLabel,
  getOutputBudgetProgress,
  isBudgetExceededError,
  resolveUsageTimeRange,
} from '../src/utils/usageRange'
import type { UsageSummary } from '../src/api/usage'

function makeSummary(overrides: Partial<UsageSummary> = {}): UsageSummary {
  return {
    groupKey: 'k',
    requestCount: 1,
    inputTokens: 100,
    outputTokens: 50,
    cacheCreationInputTokens: 0,
    cacheReadInputTokens: 0,
    totalTokens: 150,
    ...overrides,
  }
}

test('resolveUsageTimeRange computes local midnight boundaries for presets', () => {
  const now = new Date(2026, 10, 26, 14, 30, 0, 0)

  assert.deepEqual(resolveUsageTimeRange('all', now), {})

  const today = resolveUsageTimeRange('today', now)
  assert.equal(today.since, new Date(2026, 10, 26, 0, 0, 0, 0).toISOString())

  const sevenDays = resolveUsageTimeRange('7d', now)
  assert.equal(sevenDays.since, new Date(2026, 10, 20, 0, 0, 0, 0).toISOString())

  const thirtyDays = resolveUsageTimeRange('30d', now)
  assert.equal(thirtyDays.since, new Date(2026, 9, 28, 0, 0, 0, 0).toISOString())
})

test('formatUsageRowLabel formats session, model, project, IM and deleted fallback labels', () => {
  const labels = { unclassified: '未分类', telegram: 'Telegram' }

  assert.equal(
    formatUsageRowLabel('session', makeSummary({ rootSessionId: 'abc-123', sessionName: '修复接口超时' }), labels),
    '修复接口超时',
  )
  assert.equal(
    formatUsageRowLabel('session', makeSummary({ rootSessionId: '12345678-1234-5678-9999-abcdef012345' }), labels),
    '12345678…',
  )
  assert.equal(
    formatUsageRowLabel('session', makeSummary({ rootSessionId: 'im-platform-session' }), labels),
    'Telegram',
  )
  assert.equal(
    formatUsageRowLabel('session', makeSummary({ rootSessionId: 'im-platform-session:slack' }), labels),
    'slack',
  )
  assert.equal(
    formatUsageRowLabel('model', makeSummary({ groupKey: 'cfg-uuid:gpt-5.4', modelConfigId: 'cfg-uuid', model: 'gpt-5.4' }), labels),
    'gpt-5.4',
  )
  assert.equal(
    formatUsageRowLabel('project', makeSummary({ workingDirectory: '/repo/slimebot' }), labels),
    '/repo/slimebot',
  )
  assert.equal(
    formatUsageRowLabel('project', makeSummary({ workingDirectory: '' }), labels),
    '未分类',
  )
})

test('canJumpToUsageSession only allows existing ordinary chat sessions', () => {
  assert.equal(canJumpToUsageSession(makeSummary({ rootSessionId: 's1', sessionExists: true, sessionKind: 'chat' })), true)
  assert.equal(canJumpToUsageSession(makeSummary({ rootSessionId: 's1', sessionExists: true })), true)
  assert.equal(canJumpToUsageSession(makeSummary({ rootSessionId: 's1', sessionExists: false, sessionKind: 'chat' })), false)
  assert.equal(canJumpToUsageSession(makeSummary({ rootSessionId: 's1', sessionExists: true, sessionKind: 'task_run' })), false)
})

test('getOutputBudgetProgress handles unlimited, normal, warning and exceeded states', () => {
  assert.equal(getOutputBudgetProgress(null), null)
  assert.equal(getOutputBudgetProgress({ outputBudgetTokens: 0, outputBudgetUsed: 500 }), null)

  const normal = getOutputBudgetProgress({ outputBudgetTokens: 10000, outputBudgetUsed: 5000 })
  assert.deepEqual(normal, {
    budget: 10000,
    used: 5000,
    remaining: 5000,
    percent: 50,
    barPercent: 50,
    tone: 'normal',
    exceeded: false,
  })

  const warning = getOutputBudgetProgress({ outputBudgetTokens: 10000, outputBudgetUsed: 8600 })
  assert.equal(warning?.tone, 'warning')
  assert.equal(warning?.percent, 86)
  assert.equal(warning?.exceeded, false)

  const exceeded = getOutputBudgetProgress({ outputBudgetTokens: 10000, outputBudgetUsed: 10200 })
  assert.equal(exceeded?.tone, 'danger')
  assert.equal(exceeded?.exceeded, true)
  assert.equal(exceeded?.remaining, 0)
  assert.equal(exceeded?.barPercent, 100)
})

test('isBudgetExceededError matches backend budget exceeded message', () => {
  assert.equal(isBudgetExceededError('session output token budget exceeded: root=s1 budget=100 used=100 minimum=1'), true)
  assert.equal(isBudgetExceededError('network error'), false)
})
