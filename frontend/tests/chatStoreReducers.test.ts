import assert from 'node:assert/strict'
import test from 'node:test'
import type { AssistantReplyBatch } from '../src/utils/replyBatchBuilder'
import {
  appendSubagentThinkingChunk,
  finishSubagentThinking,
  appendPlanBodyToBatch,
  appendPlanChunkToBatch,
  appendTextChunkToBatch,
  finishOpenThinkingEntries,
  getLiveReplyContentSignature,
} from '../src/utils/liveReplyTimeline'

test('appendTextChunkToBatch finishes open thinking before text is appended', () => {
  const batch: AssistantReplyBatch = {
    id: 'batch-1',
    sessionId: 'session-1',
    assistantMessageId: 'assistant-1',
    toolCalls: [],
    timeline: [{
      id: 'thinking-1',
      kind: 'thinking',
      content: 'reasoning',
      done: false,
      startedAt: Date.now() - 1000,
    }],
    collapsed: false,
  }

  appendTextChunkToBatch(batch, 'answer')

  assert.deepEqual(batch.timeline.map((entry) => entry.kind), ['thinking', 'text'])
  const thinking = batch.timeline[0]!
  assert.equal(thinking.kind, 'thinking')
  assert.equal(thinking.done, true)
  assert.equal(batch.timeline[1]!.kind, 'text')
})

test('appendPlanBodyToBatch appends a plan entry and finishes open thinking', () => {
  const batch: AssistantReplyBatch = {
    id: 'batch-1',
    sessionId: 'session-1',
    assistantMessageId: 'assistant-1',
    toolCalls: [],
    timeline: [{
      id: 'thinking-1',
      kind: 'thinking',
      content: 'reasoning',
      done: false,
      startedAt: Date.now() - 1000,
    }],
    collapsed: false,
  }

  appendPlanBodyToBatch(batch, '# Plan')

  assert.deepEqual(batch.timeline.map((entry) => entry.kind), ['thinking', 'plan'])
  const thinking = batch.timeline[0]!
  assert.equal(thinking.kind, 'thinking')
  assert.equal(thinking.done, true)
  const plan = batch.timeline[1]!
  assert.equal(plan.kind, 'plan')
  assert.equal(plan.content, '# Plan')
})

test('finishOpenThinkingEntries is a no-op when all thinking entries are done', () => {
  const batch: AssistantReplyBatch = {
    id: 'batch-1',
    sessionId: 'session-1',
    assistantMessageId: 'assistant-1',
    toolCalls: [],
    timeline: [{ id: 'thinking-1', kind: 'thinking', content: 'done', done: true, durationMs: 500 }],
    collapsed: false,
  }

  finishOpenThinkingEntries(batch)

  assert.deepEqual(batch.timeline, [{ id: 'thinking-1', kind: 'thinking', content: 'done', done: true, durationMs: 500 }])
})

test('subagent thinking helpers update the parent tool without adding timeline thinking', () => {
  const batch: AssistantReplyBatch = {
    id: 'batch-1',
    sessionId: 'session-1',
    assistantMessageId: 'assistant-1',
    toolCalls: [{
      toolCallId: 'parent-tool',
      toolName: 'run_subagent',
      command: 'delegate',
      params: {},
      requiresApproval: false,
      status: 'executing',
    }],
    timeline: [],
    collapsed: false,
  }

  appendSubagentThinkingChunk(batch, 'parent-tool', 'child thought', 1000)
  finishSubagentThinking(batch, 'parent-tool', 1250)

  assert.deepEqual(batch.timeline, [])
  assert.equal(batch.toolCalls[0]!.subagentThinking?.content, 'child thought')
  assert.equal(batch.toolCalls[0]!.subagentThinking?.done, true)
  assert.equal(batch.toolCalls[0]!.subagentThinking?.durationMs, 250)
})

test('getLiveReplyContentSignature changes when streaming plan content grows in place', () => {
  const batch: AssistantReplyBatch = {
    id: 'batch-1',
    sessionId: 'session-1',
    assistantMessageId: 'assistant-1',
    toolCalls: [],
    timeline: [],
    collapsed: false,
  }

  appendPlanChunkToBatch(batch, '# Plan')
  const before = getLiveReplyContentSignature(batch)

  appendPlanChunkToBatch(batch, '\n\n- More detail')
  const after = getLiveReplyContentSignature(batch)

  assert.notEqual(after, before)
  assert.match(after, /plan:21:1/)
})
