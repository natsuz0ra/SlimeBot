import assert from 'node:assert/strict'
import test from 'node:test'
import { buildPrivacyConfig, privacyKindEnabled, privacyLines, privacyTotals } from '../src/utils/privacy.ts'
import type { PrivacyConfig, PrivacyRecord } from '../src/types/privacy.ts'
import { dispatchChatSocketMessage, type ChatSocketHandlers } from '../src/api/chatSocket.ts'

test('privacy lists trim, deduplicate and preserve punctuation', () => {
  assert.deepEqual(privacyLines(' Project-A \r\n\nProject-A\nMySecret! '), ['Project-A', 'MySecret!'])
})

test('privacy draft preserves actions and isolates nested values', () => {
  const config = { kinds: { PHONE: false }, actions: { PHONE: 'block' }, rules: [{ kind: 'CUSTOM', prefix: 'company_' }], bypassProviderIds: ['provider-one'], mappingPersistence: true, mappingTtlMinutes: 60 } as PrivacyConfig
  const result = buildPrivacyConfig(config, 'word\nword', 'allowed')
  result.kinds.PHONE = true
  result.rules[0]!.prefix = 'changed'
  assert.equal(config.kinds.PHONE, false)
  assert.equal(config.rules[0]!.prefix, 'company_')
  assert.deepEqual(result.words, ['word'])
  assert.deepEqual(result.allow, ['allowed'])
  assert.equal(result.actions.PHONE, 'block')
  assert.equal(result.mappingPersistence, true)
  assert.equal(result.mappingTtlMinutes, 60)
  assert.equal(privacyKindEnabled(config, 'EMAIL'), true)
  assert.equal(privacyKindEnabled(config, 'IP'), false)
})

test('privacy totals distinguish warnings and blocked requests', () => {
  const records = [{ masked: 3, warned: 1, blocked: false }, { masked: 2, warned: 0, blocked: true }] as PrivacyRecord[]
  assert.deepEqual(privacyTotals(records), { masked: 5, warned: 1, blocked: 1 })
})

test('privacy socket event exposes only counts to a turn', () => {
  const calls: unknown[] = []
  const handlers = { onPrivacy: (data: unknown, sessionId?: string) => calls.push({ data, sessionId }) } as unknown as ChatSocketHandlers
  dispatchChatSocketMessage(JSON.stringify({ type: 'privacy', sessionId: 'session-one', masked: 3, warned: 1, blocked: false }), handlers)
  assert.deepEqual(calls, [{ data: { masked: 3, warned: 1, blocked: false }, sessionId: 'session-one' }])
})
