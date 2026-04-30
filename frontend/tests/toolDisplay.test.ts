import assert from 'node:assert/strict'
import test from 'node:test'
import type { ToolCallItem } from '../src/api/chat'
import {
  buildToolCallSummary,
  filterToolParamsForDetail,
} from '../src/utils/toolDisplay'
import {
  buildFileToolDisplay,
  buildLineDiff,
  formatFileReadSummary,
  parseFileReadOutput,
} from '../src/utils/fileToolDisplay'

function tool(overrides: Partial<ToolCallItem>): ToolCallItem {
  return {
    toolCallId: 'call-1',
    toolName: 'exec',
    command: 'run',
    params: {},
    requiresApproval: true,
    status: 'pending',
    ...overrides,
  }
}

test('buildToolCallSummary uses exec description', () => {
  const item = tool({ params: { command: 'go test ./...', description: 'Run Go tests' } })

  assert.equal(buildToolCallSummary(item), 'Run Go tests')
})

test('buildToolCallSummary uses query and http request fields', () => {
  assert.equal(
    buildToolCallSummary(tool({ toolName: 'web_search', command: 'search', params: { query: 'SlimeBot latest' } })),
    'SlimeBot latest',
  )
  assert.equal(
    buildToolCallSummary(tool({ toolName: 'http_request', command: 'request', params: { method: 'post', url: 'https://example.test/api' } })),
    'POST https://example.test/api',
  )
})

test('buildToolCallSummary uses file tool paths and operations', () => {
  assert.equal(
    buildToolCallSummary(tool({ toolName: 'file_read', command: 'read', params: { file_path: 'frontend/src/App.vue' } })),
    'Read frontend/src/App.vue',
  )
  assert.equal(
    buildToolCallSummary(tool({
      toolName: 'file_edit',
      command: 'edit',
      params: { file_path: 'cli/src/utils/format.ts', old_string: 'old', new_string: 'new' },
    })),
    'Update cli/src/utils/format.ts',
  )
  assert.equal(
    buildToolCallSummary(tool({ toolName: 'file_write', command: 'write', params: { file_path: 'frontend/src/utils/fileToolDisplay.ts' } })),
    'Write frontend/src/utils/fileToolDisplay.ts',
  )
})

test('buildToolCallSummary uses run_subagent title before task', () => {
  assert.equal(
    buildToolCallSummary(tool({
      toolName: 'run_subagent',
      command: 'delegate',
      params: { title: 'Inspect UI cards', task: 'Inspect UI cards and report exact files' },
    })),
    'Inspect UI cards',
  )
  assert.equal(
    buildToolCallSummary(tool({
      toolName: 'run_subagent',
      command: 'delegate',
      params: { task: 'Inspect UI cards and report exact files' },
      subagentTitle: 'Inspect UI cards',
    })),
    'Inspect UI cards',
  )
  assert.equal(
    buildToolCallSummary(tool({
      toolName: 'run_subagent',
      command: 'delegate',
      params: { task: 'Inspect UI cards and report exact files' },
    })),
    'task: Inspect UI cards and report exact files',
  )
})

test('buildToolCallSummary hides missing legacy exec description', () => {
  assert.equal(buildToolCallSummary(tool({ params: { command: 'go test ./...' } })), '')
})

test('filterToolParamsForDetail removes params already shown in summary', () => {
  assert.deepEqual(
    filterToolParamsForDetail(tool({ params: { command: 'go test ./...', description: 'Run Go tests' } })),
    { command: 'go test ./...' },
  )
  assert.deepEqual(
    filterToolParamsForDetail(tool({ toolName: 'web_search', command: 'search', params: { query: 'SlimeBot latest' } })),
    {},
  )
  assert.deepEqual(
    filterToolParamsForDetail(tool({
      toolName: 'run_subagent',
      command: 'delegate',
      params: { title: 'Inspect UI cards', task: 'Inspect UI cards and report exact files', context: 'repo state', priority: 'high' },
    })),
    { context: 'repo state', priority: 'high' },
  )
  assert.deepEqual(
    filterToolParamsForDetail(tool({
      toolName: 'file_edit',
      command: 'edit',
      params: { file_path: 'cli/src/utils/format.ts', old_string: 'old', new_string: 'new' },
    })),
    { old_string: 'old', new_string: 'new' },
  )
})

test('parseFileReadOutput summarizes reads without exposing file body', () => {
  const parsed = parseFileReadOutput([
    'File: cli/src/utils/timelineFormat.ts',
    'Total lines: 462',
    'Showing lines 120-159:',
    '   120\tconst hidden = true',
  ].join('\n'))

  assert.deepEqual(parsed, {
    filePath: 'cli/src/utils/timelineFormat.ts',
    totalLines: 462,
    startLine: 120,
    endLine: 159,
    truncated: false,
  })
  assert.equal(formatFileReadSummary(parsed!), 'Read 40 of 462 lines, showing 120-159')
})

test('buildLineDiff emits concrete removed and added lines', () => {
  assert.deepEqual(buildLineDiff('a\nb\nc\n', 'a\nx\nc\n'), [
    { kind: 'context', oldLine: 1, newLine: 1, text: 'a' },
    { kind: 'removed', oldLine: 2, text: 'b' },
    { kind: 'added', newLine: 2, text: 'x' },
    { kind: 'context', oldLine: 3, newLine: 3, text: 'c' },
  ])
})

test('buildFileToolDisplay formats file_write as concrete added lines', () => {
  const display = buildFileToolDisplay(tool({
    toolName: 'file_write',
    command: 'write',
    params: {
      file_path: 'frontend/src/utils/fileToolDisplay.ts',
      content: 'export const ok = true\n',
    },
  }))

  assert.equal(display?.summary, 'Wrote 1 line to frontend/src/utils/fileToolDisplay.ts')
  assert.deepEqual(display?.diffLines, [
    { kind: 'added', newLine: 1, text: 'export const ok = true' },
  ])
})
