import assert from 'node:assert/strict'
import test from 'node:test'
import type { SessionHistoryThinkingItem, ToolCallItem } from '../src/api/chat'
import {
  buildInterleavedTimeline,
  getCollapsedReplyTimeline,
  hasCollapsibleReplyContent,
} from '../src/utils/replyBatchBuilder'

test('buildInterleavedTimeline renders plan marker content as a plan item', () => {
  const timeline = buildInterleavedTimeline(
    [],
    [
      'Intro text.',
      '<!-- PLAN_START -->',
      '# Plan',
      '',
      '- Step one',
      '<!-- PLAN_END -->',
      'After text.',
    ].join('\n'),
  )

  assert.deepEqual(
    timeline.map((entry) => entry.kind),
    ['text', 'plan', 'text'],
  )
  assert.equal(timeline[1]!.kind, 'plan')
  assert.match('content' in timeline[1]! ? timeline[1].content : '', /# Plan/)
  assert.match('content' in timeline[1]! ? timeline[1].content : '', /Step one/)
})

test('buildInterleavedTimeline keeps an unclosed plan marker as a plan item', () => {
  const timeline = buildInterleavedTimeline(
    [],
    [
      'Before.',
      '<!-- PLAN_START -->',
      'Still a plan.',
    ].join('\n'),
  )

  assert.deepEqual(
    timeline.map((entry) => entry.kind),
    ['text', 'plan'],
  )
  assert.equal(timeline[1]!.kind, 'plan')
  assert.match('content' in timeline[1]! ? timeline[1].content : '', /Still a plan/)
})

test('buildInterleavedTimeline preserves thinking and tool ordering around plans', () => {
  const toolCalls: ToolCallItem[] = [{
    toolCallId: 'tool-1',
    toolName: 'web_search',
    command: 'search',
    params: {},
    requiresApproval: false,
    status: 'completed',
    output: 'ok',
  }]
  const thinkingRecords: SessionHistoryThinkingItem[] = [{
    thinkingId: 'think-1',
    content: 'reasoning',
    status: 'completed',
    durationMs: 1200,
  }]

  const timeline = buildInterleavedTimeline(
    toolCalls,
    [
      '<!-- THINKING:think-1 -->',
      '<!-- PLAN_START -->',
      'Plan body.',
      '<!-- PLAN_END -->',
      '<!-- TOOL_CALL:tool-1 -->',
      'Done.',
    ].join('\n'),
    thinkingRecords,
  )

  assert.deepEqual(
    timeline.map((entry) => entry.kind),
    ['thinking', 'plan', 'tool_start', 'tool_result', 'text'],
  )
})

test('getCollapsedReplyTimeline keeps every plan and only the final text segment', () => {
  const timeline = [
    { id: 'text-before', kind: 'text' as const, content: 'I will inspect first.' },
    { id: 'thinking-1', kind: 'thinking' as const, content: 'reasoning', done: true },
    { id: 'plan-1', kind: 'plan' as const, content: '# Plan' },
    { id: 'tool-1', kind: 'tool_start' as const, toolCallId: 'tool-1' },
    { id: 'tool-result-1', kind: 'tool_result' as const, toolCallId: 'tool-1' },
    { id: 'text-final', kind: 'text' as const, content: 'Final answer.' },
  ]

  assert.deepEqual(
    getCollapsedReplyTimeline(timeline).map((entry) => entry.id),
    ['plan-1', 'text-final'],
  )
})

test('hasCollapsibleReplyContent ignores plain final text and detects hidden process content', () => {
  assert.equal(
    hasCollapsibleReplyContent(
      [{ id: 'text-final', kind: 'text', content: 'Final answer.' }],
      [],
    ),
    false,
  )

  assert.equal(
    hasCollapsibleReplyContent(
      [
        { id: 'text-before', kind: 'text', content: 'Checking.' },
        { id: 'text-final', kind: 'text', content: 'Final answer.' },
      ],
      [],
    ),
    true,
  )

  assert.equal(
    hasCollapsibleReplyContent(
      [{ id: 'text-final', kind: 'text', content: 'Final answer.' }],
      [{
        toolCallId: 'tool-1',
        toolName: 'exec',
        command: 'run',
        params: {},
        preamble: 'I will run a command.',
        requiresApproval: false,
        status: 'completed',
      }],
    ),
    true,
  )
})

test('buildReplyBatchesFromHistory attaches ordered subagent thinking entries to parent run_subagent tool', async () => {
  const { buildReplyBatchesFromHistory } = await import('../src/utils/replyBatchBuilder')
  const batches = buildReplyBatchesFromHistory('session-1', {
    messages: [{
      id: 'assistant-1',
      role: 'assistant',
      content: '<!-- TOOL_CALL:parent-tool -->\nDone.',
      seq: 1,
    }],
    toolCallsByAssistantMessageId: {
      'assistant-1': [{
        toolCallId: 'parent-tool',
        toolName: 'run_subagent',
        command: 'delegate',
        params: {},
        requiresApproval: false,
        status: 'completed',
        output: 'child answer',
        startedAt: '2026-04-28T00:00:00.000Z',
      }],
    },
    thinkingByAssistantMessageId: {
      'assistant-1': [
        {
          thinkingId: 'child-think-late',
          content: 'later child reasoning',
          status: 'completed',
          durationMs: 500,
          startedAt: '2026-04-28T00:00:03.000Z',
          parentToolCallId: 'parent-tool',
          subagentRunId: 'sub-run',
        },
        {
          thinkingId: 'child-think-early',
          content: 'earlier child reasoning',
          status: 'completed',
          durationMs: 250,
          startedAt: '2026-04-28T00:00:01.000Z',
          parentToolCallId: 'parent-tool',
          subagentRunId: 'sub-run',
        },
      ],
    },
    hasMore: false,
  })

  const batch = batches[0]!
  assert.deepEqual(batch.timeline.map((entry) => entry.kind), ['tool_start', 'tool_result', 'text'])
  assert.deepEqual(batch.toolCalls[0]!.subagentThinkings?.map((item) => item.content), [
    'earlier child reasoning',
    'later child reasoning',
  ])
  assert.deepEqual(batch.toolCalls[0]!.subagentThinkings?.map((item) => item.done), [true, true])
})

test('buildReplyBatchesFromHistory restores run_subagent title from history params', async () => {
  const { buildReplyBatchesFromHistory } = await import('../src/utils/replyBatchBuilder')
  const batches = buildReplyBatchesFromHistory('session-1', {
    messages: [{
      id: 'assistant-1',
      role: 'assistant',
      content: '<!-- TOOL_CALL:parent-tool -->',
      seq: 1,
    }],
    toolCallsByAssistantMessageId: {
      'assistant-1': [{
        toolCallId: 'parent-tool',
        toolName: 'run_subagent',
        command: 'delegate',
        params: { title: 'Inspect UI cards', task: 'Inspect UI cards and report exact files' },
        requiresApproval: false,
        status: 'completed',
        startedAt: '2026-04-28T00:00:00.000Z',
      }],
    },
    thinkingByAssistantMessageId: {},
    hasMore: false,
  })

  assert.equal(batches[0]!.toolCalls[0]!.subagentTitle, 'Inspect UI cards')
  assert.equal(batches[0]!.toolCalls[0]!.subagentTask, 'Inspect UI cards and report exact files')
})

test('buildReplyBatchesFromHistory preserves nested tool start times for subagent timelines', async () => {
  const { buildReplyBatchesFromHistory } = await import('../src/utils/replyBatchBuilder')
  const batches = buildReplyBatchesFromHistory('session-1', {
    messages: [{
      id: 'assistant-1',
      role: 'assistant',
      content: '<!-- TOOL_CALL:parent-tool -->',
      seq: 1,
    }],
    toolCallsByAssistantMessageId: {
      'assistant-1': [
        {
          toolCallId: 'parent-tool',
          toolName: 'run_subagent',
          command: 'delegate',
          params: {},
          requiresApproval: false,
          status: 'completed',
          startedAt: '2026-04-28T00:00:00.000Z',
        },
        {
          toolCallId: 'child-tool',
          toolName: 'exec',
          command: 'run',
          params: {},
          requiresApproval: false,
          status: 'completed',
          parentToolCallId: 'parent-tool',
          subagentRunId: 'sub-run',
          startedAt: '2026-04-28T00:00:02.000Z',
        },
      ],
    },
    thinkingByAssistantMessageId: {
      'assistant-1': [{
        thinkingId: 'child-think',
        content: 'child reasoning',
        status: 'completed',
        startedAt: '2026-04-28T00:00:01.000Z',
        parentToolCallId: 'parent-tool',
        subagentRunId: 'sub-run',
      }],
    },
    hasMore: false,
  })

  const batch = batches[0]!
  assert.equal(batch.toolCalls.find((item) => item.toolCallId === 'child-tool')?.startedAt, Date.parse('2026-04-28T00:00:02.000Z'))
  assert.equal(batch.toolCalls.find((item) => item.toolCallId === 'parent-tool')?.subagentThinkings?.[0]?.startedAt, Date.parse('2026-04-28T00:00:01.000Z'))
})

test('buildReplyBatchesFromHistory derives reply timing from persisted timestamps', async () => {
  const { buildReplyBatchesFromHistory } = await import('../src/utils/replyBatchBuilder')
  const batches = buildReplyBatchesFromHistory('session-1', {
    messages: [{
      id: 'assistant-1',
      role: 'assistant',
      content: '<!-- TOOL_CALL:tool-1 -->\nDone.',
      createdAt: '2026-04-28T00:00:00.000Z',
      seq: 1,
    }],
    toolCallsByAssistantMessageId: {
      'assistant-1': [{
        toolCallId: 'tool-1',
        toolName: 'web_search',
        command: 'search',
        params: {},
        requiresApproval: false,
        status: 'completed',
        output: 'ok',
        startedAt: '2026-04-28T00:00:01.000Z',
        finishedAt: '2026-04-28T00:00:04.000Z',
      }],
    },
    thinkingByAssistantMessageId: {
      'assistant-1': [{
        thinkingId: 'think-1',
        content: 'reasoning',
        status: 'completed',
        startedAt: '2026-04-28T00:00:00.500Z',
        finishedAt: '2026-04-28T00:00:02.000Z',
      }],
    },
    hasMore: false,
  })

  assert.equal(batches[0]!.startedAt, Date.parse('2026-04-28T00:00:00.000Z'))
  assert.equal(batches[0]!.finishedAt, Date.parse('2026-04-28T00:00:04.000Z'))
  assert.equal(batches[0]!.durationMs, 4000)
})

test('buildReplyBatchesFromHistory prefers backend reply timing over derived tool timing', async () => {
  const { buildReplyBatchesFromHistory } = await import('../src/utils/replyBatchBuilder')
  const batches = buildReplyBatchesFromHistory('session-1', {
    messages: [{
      id: 'assistant-1',
      role: 'assistant',
      content: '<!-- TOOL_CALL:tool-1 -->\nDone.',
      createdAt: '2026-04-28T00:00:02.000Z',
      seq: 2,
    }],
    toolCallsByAssistantMessageId: {
      'assistant-1': [{
        toolCallId: 'tool-1',
        toolName: 'web_search',
        command: 'search',
        params: {},
        requiresApproval: false,
        status: 'completed',
        startedAt: '2026-04-28T00:00:05.000Z',
        finishedAt: '2026-04-28T00:00:10.000Z',
      }],
    },
    thinkingByAssistantMessageId: {},
    replyTimingByAssistantMessageId: {
      'assistant-1': {
        startedAt: '2026-04-28T00:00:01.000Z',
        finishedAt: '2026-04-28T00:00:03.500Z',
        durationMs: 2500,
      },
    },
    hasMore: false,
  })

  assert.equal(batches[0]!.startedAt, Date.parse('2026-04-28T00:00:01.000Z'))
  assert.equal(batches[0]!.finishedAt, Date.parse('2026-04-28T00:00:03.500Z'))
  assert.equal(batches[0]!.durationMs, 2500)
})
