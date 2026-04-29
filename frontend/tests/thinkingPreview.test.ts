import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import test from 'node:test'
import type { ToolCallItem } from '../src/api/chat'
import { buildSubagentTimeline } from '../src/utils/subagentTimeline'
import { getThinkingPreviewLine } from '../src/utils/thinkingPreview'

test('getThinkingPreviewLine returns the latest non-empty line', () => {
  assert.equal(getThinkingPreviewLine('first line\nsecond line'), 'second line')
  assert.equal(getThinkingPreviewLine('first line\nsecond line\n'), 'second line')
})

test('getThinkingPreviewLine preserves the active empty line while typing after a newline', () => {
  assert.equal(getThinkingPreviewLine('first line\n'), 'first line')
  assert.equal(getThinkingPreviewLine('first line\nn'), 'n')
})

test('ThinkingBlock renders a one-line preview while running and gates full content until done', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ThinkingBlock.vue'), 'utf8')

  assert.match(source, /getThinkingPreviewLine/)
  assert.match(source, /thinking-preview/)
  assert.match(source, /v-if="hasVisibleContent && done && expanded"/)
  assert.match(source, /subagentThinkingLabel/)
  assert.match(source, /subagentThoughtFor/)
  assert.match(source, /\.thinking-chevron\s*\{[\s\S]*margin-left:\s*auto/)
  assert.doesNotMatch(source, /Sub-agent thought/)
  assert.doesNotMatch(source, /Sub-agent thinking/)
  assert.doesNotMatch(source, /v-if="hasVisibleContent && \(!done \|\| expanded\)"/)
})

test('ToolCallInline renders subagent thinking entries with the shared ThinkingBlock', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ToolCallInline.vue'), 'utf8')

  assert.match(source, /import ThinkingBlock/)
  assert.match(source, /subagentTimelineItems/)
  assert.match(source, /subagentTaskSummary/)
  assert.match(source, /subagentContextSummary/)
  assert.match(source, /subagentTaskLabel/)
  assert.match(source, /subagentContextLabel/)
  assert.match(source, /variant="subagent"/)
  assert.match(source, /subagentTimelineExpanded/)
})

test('run_subagent cards hoist task and context while hiding duplicate params', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ToolCallCard.vue'), 'utf8')

  assert.match(source, /subagentTaskSummary/)
  assert.match(source, /subagentContextSummary/)
  assert.match(source, /subagentTaskLabel/)
  assert.match(source, /subagentContextLabel/)
  assert.match(source, /subagentResultLabel/)
  assert.match(source, /subagentTimelineItems/)
  assert.match(source, /subagentTimelineExpanded/)
  assert.match(source, /runSubagentParamsDisplay/)
})

test('buildSubagentTimeline interleaves thinking and tools by start time', () => {
  const nestedTools: ToolCallItem[] = [
    {
      toolCallId: 'tool-late',
      toolName: 'exec',
      command: 'run',
      params: {},
      requiresApproval: false,
      status: 'completed',
      startedAt: 4000,
    },
    {
      toolCallId: 'tool-early',
      toolName: 'web_search',
      command: 'search',
      params: {},
      requiresApproval: false,
      status: 'completed',
      startedAt: 2000,
    },
  ]

  const timeline = buildSubagentTimeline(
    [
      { content: 'thinking late', done: true, startedAt: 3000 },
      { content: 'thinking early', done: true, startedAt: 1000 },
    ],
    nestedTools,
  )

  assert.deepEqual(timeline.map((item) => item.kind), ['thinking', 'tool', 'thinking', 'tool'])
  assert.deepEqual(timeline.map((item) => item.kind === 'thinking' ? item.thinking.content : item.tool.toolCallId), [
    'thinking early',
    'tool-early',
    'thinking late',
    'tool-late',
  ])
})

test('buildSubagentTimeline keeps missing timestamps stable after timestamped entries', () => {
  const timeline = buildSubagentTimeline(
    [{ content: 'no time thinking', done: true }],
    [{
      toolCallId: 'no-time-tool',
      toolName: 'exec',
      command: 'run',
      params: {},
      requiresApproval: false,
      status: 'completed',
    }],
  )

  assert.deepEqual(timeline.map((item) => item.kind === 'thinking' ? item.thinking.content : item.tool.toolCallId), [
    'no time thinking',
    'no-time-tool',
  ])
})
