import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { dispatchChatSocketMessage } from '../src/api/chatSocket'
import {
  createEmptyChatRuntimeState,
  reduceChatRuntimeEvent,
  type ChatRuntimeReducerDeps,
  type ChatRuntimeState,
} from '../src/stores/chatSessionReducer'
import { buildReplyBatchesFromHistory, type AssistantReplyBatch } from '../src/utils/replyBatchBuilder'

const fixture = JSON.parse(readFileSync(new URL('../../shared/chat-protocol/fixtures/turn-consistency.json', import.meta.url), 'utf8'))

function testDeps(): ChatRuntimeReducerDeps {
  let id = 0
  return {
    createId: () => `runtime-${++id}`,
    now: () => Date.parse('2026-04-29T01:00:09Z'),
    formatAssistantError: error => `Reply failed: ${error}`,
    stoppedPlaceholder: () => 'Stopped.',
  }
}

function startRuntime(state = createEmptyChatRuntimeState()): ChatRuntimeState {
  return reduceChatRuntimeEvent(state, {
    type: 'start',
    sessionId: 'session-consistency',
    meta: { turnId: 'turn-1', startedAt: '2026-04-29T01:00:00Z' },
  }, testDeps())
}

function reduceFixtureEvents(): ChatRuntimeState {
  let state = createEmptyChatRuntimeState()
  const deps = testDeps()
  const reduce = (event: Parameters<typeof reduceChatRuntimeEvent>[1]) => {
    state = reduceChatRuntimeEvent(state, event, deps)
  }
  for (const event of fixture.events) {
    const accepted = dispatchChatSocketMessage(JSON.stringify(event), {
      onStart: (sessionId, meta) => sessionId && reduce({ type: 'start', sessionId, meta }),
      onChunk: chunk => reduce({ type: 'chunk', chunk }),
      onThinkingStart: data => reduce({ type: 'thinking_start', data }),
      onThinkingChunk: data => reduce({ type: 'thinking_chunk', data }),
      onThinkingDone: data => reduce({ type: 'thinking_done', data }),
      onToolCallStart: data => reduce({ type: 'tool_call_start', data }),
      onToolCallResult: data => reduce({ type: 'tool_call_result', data }),
      onSubagentStart: data => reduce({ type: 'subagent_start', data }),
      onSubagentChunk: data => reduce({ type: 'subagent_chunk', data }),
      onSubagentDone: data => reduce({ type: 'subagent_done', data }),
      onPlanStart: () => reduce({ type: 'plan_start' }),
      onPlanChunk: chunk => reduce({ type: 'plan_chunk', chunk }),
      onPlanBody: content => reduce({ type: 'plan_body', content }),
      onDone: (sessionId, answer, meta) => sessionId && reduce({ type: 'done', sessionId, answer, meta }),
    })
    assert.equal(accepted, true)
  }
  return state
}

function comparableTimeline(batch: AssistantReplyBatch) {
  return batch.timeline.flatMap(entry => {
    if (entry.kind === 'tool_result') return []
    if (entry.kind === 'tool_start') {
      const tool = batch.toolCalls.find(item => item.toolCallId === entry.toolCallId)!
      return [{
        kind: 'tool',
        toolCallId: tool.toolCallId,
        status: tool.status,
        title: tool.subagentTitle,
        thinkings: tool.subagentThinkings?.map(thinking => ({ content: thinking.content, done: thinking.done })),
      }]
    }
    if (entry.kind === 'thinking') {
      return [{ kind: entry.kind, content: entry.content, done: entry.done, durationMs: entry.durationMs }]
    }
    return [{ kind: entry.kind, content: entry.content }]
  })
}

test('runtime reducer settles thinking, tool, subagent, plan and completion state', () => {
  const state = reduceFixtureEvents()
  const batch = state.replyBatches[0]!

  assert.equal(state.waiting, false)
  assert.equal(state.streamingStarted, false)
  assert.equal(state.currentBatchId, '')
  assert.equal(state.planGenerating, false)
  assert.deepEqual(batch.timeline.map(entry => entry.kind), [
    'thinking', 'text', 'tool_start', 'tool_result', 'plan', 'text',
  ])
  assert.equal(batch.toolCalls[0]?.status, 'completed')
  assert.equal(batch.toolCalls[0]?.subagentTitle, 'Worker')
  assert.deepEqual(batch.toolCalls[0]?.subagentThinkings?.map(thinking => thinking.content), [
    'First child thought', 'Second child thought',
  ])
  assert.equal(batch.toolCalls[0]?.subagentThinkings?.every(thinking => thinking.done), true)
  assert.equal(batch.timeline.at(-1)?.kind, 'text')
  assert.equal(batch.durationMs, 9000)
})

test('runtime reducer records errors without mutating the previous state', () => {
  const previous = startRuntime()
  const withText = reduceChatRuntimeEvent(previous, { type: 'chunk', chunk: 'partial' }, testDeps())
  const next = reduceChatRuntimeEvent(withText, {
    type: 'error', sessionId: 'session-consistency', error: 'provider failed',
  }, testDeps())

  assert.equal(withText.messages[0]?.content, 'partial')
  assert.equal(withText.currentBatchId !== '', true)
  assert.equal(next.messages[0]?.content, 'Reply failed: provider failed')
  assert.equal(next.assistantErrorIds.has(next.messages[0]!.id), true)
  assert.equal(next.currentBatchId, '')
  assert.equal(next.waiting, false)
})

test('runtime reducer keeps plan body separate from done confirmation and reply timing', () => {
  let state = startRuntime()
  const deps = testDeps()
  state = reduceChatRuntimeEvent(state, { type: 'plan_body', content: '# Plan body' }, deps)
  state = reduceChatRuntimeEvent(state, {
    type: 'done', sessionId: 'session-consistency', answer: '',
    meta: { planId: 'plan-1', planBody: '# Plan body', finishedAt: '2026-04-29T01:00:09Z', durationMs: 9000 },
  }, deps)

  assert.deepEqual(state.pendingPlanConfirmation, {
    sessionId: 'session-consistency', planId: 'plan-1', content: '# Plan body',
  })
  assert.deepEqual(state.replyBatches[0]?.timeline.map(entry => entry.kind), ['plan'])
  assert.equal(state.replyBatches[0]?.startedAt, Date.parse('2026-04-29T01:00:00Z'))
  assert.equal(state.replyBatches[0]?.finishedAt, Date.parse('2026-04-29T01:00:09Z'))
  assert.equal(state.replyBatches[0]?.durationMs, 9000)
})

test('live WebSocket reduction matches persisted history projection', () => {
  const live = reduceFixtureEvents().replyBatches[0]!
  const message = {
    id: 'assistant-1', sessionId: fixture.sessionId, role: 'assistant' as const,
    content: fixture.history.content, createdAt: '2026-04-29T01:00:00Z',
  }
  const replayed = buildReplyBatchesFromHistory(fixture.sessionId, {
    messages: [message],
    toolCallsByAssistantMessageId: { [message.id]: fixture.history.toolCalls },
    thinkingByAssistantMessageId: { [message.id]: fixture.history.thinking },
    hasMore: false,
  })[0]!

  assert.deepEqual(comparableTimeline(live), comparableTimeline(replayed))
  assert.deepEqual(comparableTimeline(live).map(entry => entry.kind), [
    'thinking', 'text', 'tool', 'plan', 'text',
  ])
  assert.equal(live.toolCalls[0]?.subagentThinkings?.length, 2)
})
