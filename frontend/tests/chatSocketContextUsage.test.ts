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
    inputBudget: 940000,
    outputReserve: 4096,
    source: 'estimated',
    state: 'compacting',
    compactionBeforeTokens: 20000,
    compactionAfterTokens: 2000,
    compactionReason: 'pressure',
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
      inputBudget: 940000,
      outputReserve: 4096,
      source: 'estimated',
      state: 'compacting',
      compactionBeforeTokens: 20000,
      compactionAfterTokens: 2000,
      compactionReason: 'pressure',
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

test('dispatchChatSocketMessage routes auto approval review events', () => {
  const reviews: Array<{ status: string; reason?: string }> = []
  const required: Array<{ toolCallId: string; command: string; params: Record<string, unknown>; reason?: string }> = []
  const handlers: ChatSocketHandlers = {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onSessionTitle: () => {},
    onDone: () => {},
    onError: () => {},
    onToolCallReview: (data) => {
      reviews.push({ status: data.reviewStatus, reason: data.reviewReason })
    },
    onToolApprovalRequired: (data) => {
      required.push({ toolCallId: data.toolCallId, command: data.command, params: data.params, reason: data.reviewReason })
    },
  }

  dispatchChatSocketMessage(JSON.stringify({
    type: 'tool_call_review',
    sessionId: 'sid-1',
    toolCallId: 'call-1',
    toolName: 'exec',
    command: 'run',
    reviewStatus: 'reviewing',
  }), handlers)
  dispatchChatSocketMessage(JSON.stringify({
    type: 'tool_call_approval_required',
    sessionId: 'sid-1',
    toolCallId: 'call-1',
    toolName: 'exec',
    command: 'run',
    params: { command: 'rm -rf /tmp/example' },
    requiresApproval: true,
    reviewStatus: 'needs_user',
    reviewReason: 'destructive command',
  }), handlers)

  assert.deepEqual(reviews, [{ status: 'reviewing', reason: '' }])
  assert.deepEqual(required, [{
    toolCallId: 'call-1',
    command: 'run',
    params: { command: 'rm -rf /tmp/example' },
    reason: 'destructive command',
  }])
})

test('dispatchChatSocketMessage routes message_edited payloads', () => {
  const calls: Array<{ sessionId?: string; messageId: string; content: string; createdAt?: string }> = []
  const handlers: ChatSocketHandlers = {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onSessionTitle: () => {},
    onDone: () => {},
    onError: () => {},
    onMessageEdited: (data, sessionId) => {
      calls.push({ sessionId, messageId: data.messageId, content: data.content, createdAt: data.createdAt })
    },
  }
  const createdAt = '2026-04-29T01:02:03Z'

  dispatchChatSocketMessage(JSON.stringify({
    type: 'message_edited',
    sessionId: 'sid-1',
    messageId: 'msg-1',
    content: 'edited',
    createdAt,
  }), handlers)

  assert.deepEqual(calls, [{ sessionId: 'sid-1', messageId: 'msg-1', content: 'edited', createdAt }])
})
