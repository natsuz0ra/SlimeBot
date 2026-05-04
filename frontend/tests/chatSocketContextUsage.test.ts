import assert from 'node:assert/strict'
import test from 'node:test'
import { dispatchChatSocketMessage, type ChatSocketHandlers, type ContextUsageData } from '../src/api/chatSocket'

test('dispatchChatSocketMessage routes context_usage payloads', () => {
  const calls: Array<{ sessionId?: string; usage: ContextUsageData }> = []
  const handlers: ChatSocketHandlers = {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onSessionTitle: () => {},
    onDone: () => {},
    onError: () => {},
    onContextUsage: (usage, sessionId) => {
      calls.push({ sessionId, usage })
    },
  }

  dispatchChatSocketMessage(JSON.stringify({
    type: 'context_usage',
    sessionId: 'sid-1',
    modelConfigId: 'model-1',
    usedTokens: 420000,
    totalTokens: 1000000,
    usedPercent: 42,
    availablePercent: 58,
    isCompacted: true,
    compactedAt: '2026-05-03T01:02:03Z',
  }), handlers)

  assert.deepEqual(calls, [{
    sessionId: 'sid-1',
    usage: {
      sessionId: 'sid-1',
      modelConfigId: 'model-1',
      usedTokens: 420000,
      totalTokens: 1000000,
      usedPercent: 42,
      availablePercent: 58,
      isCompacted: true,
      compactedAt: '2026-05-03T01:02:03Z',
    },
  }])
})

test('dispatchChatSocketMessage routes context_compacted with nested usage', () => {
  const calls: Array<{ sessionId?: string; usage: ContextUsageData }> = []
  const handlers: ChatSocketHandlers = {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onSessionTitle: () => {},
    onDone: () => {},
    onError: () => {},
    onContextCompacted: (usage, sessionId) => {
      calls.push({ sessionId, usage })
    },
  }

  dispatchChatSocketMessage(JSON.stringify({
    type: 'context_compacted',
    sessionId: 'sid-1',
    usage: {
      sessionId: 'sid-1',
      modelConfigId: 'model-1',
      usedTokens: 120000,
      totalTokens: 500000,
      usedPercent: 24,
      availablePercent: 76,
      isCompacted: true,
    },
  }), handlers)

  assert.equal(calls.length, 1)
  assert.equal(calls[0]!.sessionId, 'sid-1')
  assert.equal(calls[0]!.usage.usedPercent, 24)
  assert.equal(calls[0]!.usage.isCompacted, true)
})
