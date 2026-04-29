import assert from 'node:assert/strict'
import test from 'node:test'
import type { AssistantReplyBatch } from '../src/utils/replyBatchBuilder'
import {
  startSubagentThinking,
  appendSubagentThinkingChunk,
  finalizeReplyBatchTiming,
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

test('subagent thinking helpers append multiple entries to the parent tool without adding timeline thinking', () => {
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

  startSubagentThinking(batch, 'parent-tool', 1000)
  appendSubagentThinkingChunk(batch, 'parent-tool', 'child thought', 1000)
  finishSubagentThinking(batch, 'parent-tool', 1250)
  startSubagentThinking(batch, 'parent-tool', 2000)
  appendSubagentThinkingChunk(batch, 'parent-tool', 'second thought', 2000)
  finishSubagentThinking(batch, 'parent-tool', 2600)

  assert.deepEqual(batch.timeline, [])
  assert.deepEqual(batch.toolCalls[0]!.subagentThinkings?.map((item) => item.content), ['child thought', 'second thought'])
  assert.deepEqual(batch.toolCalls[0]!.subagentThinkings?.map((item) => item.done), [true, true])
  assert.deepEqual(batch.toolCalls[0]!.subagentThinkings?.map((item) => item.durationMs), [250, 600])
})

test('subagent thinking chunks only update the latest open entry', () => {
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

  startSubagentThinking(batch, 'parent-tool', 1000)
  appendSubagentThinkingChunk(batch, 'parent-tool', 'first', 1000)
  startSubagentThinking(batch, 'parent-tool', 1500)
  appendSubagentThinkingChunk(batch, 'parent-tool', 'second', 1500)
  finishSubagentThinking(batch, 'parent-tool', 1800)

  assert.deepEqual(batch.toolCalls[0]!.subagentThinkings?.map((item) => item.content), ['first', 'second'])
  assert.deepEqual(batch.toolCalls[0]!.subagentThinkings?.map((item) => item.done), [false, true])
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

test('finalizeReplyBatchTiming records duration and collapses the completed live reply', () => {
  const batch: AssistantReplyBatch = {
    id: 'batch-1',
    sessionId: 'session-1',
    assistantMessageId: 'assistant-1',
    toolCalls: [],
    timeline: [],
    collapsed: false,
    startedAt: 1000,
  }

  finalizeReplyBatchTiming(batch, 2750)

  assert.equal(batch.collapsed, true)
  assert.equal(batch.finishedAt, 2750)
  assert.equal(batch.durationMs, 1750)
})

test('finalizeReplyBatchTiming prefers backend duration when provided', () => {
  const batch: AssistantReplyBatch = {
    id: 'batch-1',
    sessionId: 'session-1',
    assistantMessageId: 'assistant-1',
    toolCalls: [],
    timeline: [],
    collapsed: false,
    startedAt: 1000,
  }

  finalizeReplyBatchTiming(batch, 5000, 2400)

  assert.equal(batch.collapsed, true)
  assert.equal(batch.finishedAt, 5000)
  assert.equal(batch.durationMs, 2400)
})
