import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import test from 'node:test'
import type { ToolCallItem } from '../src/api/chat'
import { getToolCallLabel } from '../src/composables/chat/useToolCallDisplay'
import {
  buildLightweightToolGroupSummary,
  buildLightweightToolTimelineRows,
  buildLightweightToolDisplay,
  buildToolCallSummary,
  filterToolParamsForDetail,
  isLightweightToolCall,
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

test('schedule tool label is translated', () => {
  const labels: Record<string, string> = {
    toolSchedule: '定时任务',
  }

  assert.equal(getToolCallLabel('schedule', (key) => labels[key] || key), '定时任务')
  assert.equal(getToolCallLabel('unknown_tool', (key) => labels[key] || key), 'unknown_tool')
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

test('buildLightweightToolDisplay summarizes activity without output body', () => {
  const display = buildLightweightToolDisplay(tool({
    toolName: 'web_search',
    command: 'search',
    params: { query: 'SlimeBot latest' },
    status: 'completed',
    output: '{"results":[{"title":"Hidden body","url":"https://example.test","content":"do not show"}]}',
  }))

  assert.equal(display?.kind, 'search')
  assert.equal(display?.target, 'SlimeBot latest')
  assert.equal(display?.status, 'completed')
  assert.equal(display?.error, '')
  assert.equal(JSON.stringify(display).includes('do not show'), false)
})

test('buildLightweightToolDisplay hides web_extract failure body', () => {
  const display = buildLightweightToolDisplay(tool({
    toolName: 'web_extract',
    command: 'extract',
    params: { url: 'https://example.test/fail' },
    status: 'error',
    error: 'web_extract request failed (status 500): <html><body>secret response body</body></html>',
  }))

  assert.equal(display?.error, 'web_extract request failed (status 500).')
  assert.equal(JSON.stringify(display).includes('secret response body'), false)
  assert.equal(JSON.stringify(display).includes('<html>'), false)
})

test('buildLightweightToolTimelineRows merges consecutive lightweight tools only', () => {
  const calls = [
    tool({ toolCallId: 'search-1', toolName: 'web_search', command: 'search', params: { query: 'SlimeBot latest' }, status: 'completed' }),
    tool({ toolCallId: 'web-1', toolName: 'web_extract', command: 'extract', params: { url: 'https://example.test/docs' }, status: 'completed' }),
    tool({ toolCallId: 'read-1', toolName: 'file_read', command: 'read', params: { file_path: 'frontend/src/App.vue' }, status: 'completed', output: 'secret file body' }),
    tool({ toolCallId: 'edit-1', toolName: 'file_edit', command: 'edit', params: { file_path: 'frontend/src/App.vue', old_string: 'a', new_string: 'b' }, status: 'completed' }),
    tool({ toolCallId: 'search-2', toolName: 'search_files', command: 'search', params: { query: 'ToolCall', path: 'frontend/src' }, status: 'error', error: 'permission denied', output: 'hidden search hits' }),
  ]
  const rows = buildLightweightToolTimelineRows(
    [
      { id: 's1', kind: 'tool_start' as const, toolCallId: 'search-1' },
      { id: 'w1', kind: 'tool_start' as const, toolCallId: 'web-1' },
      { id: 'r1', kind: 'tool_start' as const, toolCallId: 'read-1' },
      { id: 'e1', kind: 'tool_start' as const, toolCallId: 'edit-1' },
      { id: 's2', kind: 'tool_start' as const, toolCallId: 'search-2' },
    ],
    (id) => calls.find((item) => item.toolCallId === id),
  )

  assert.deepEqual(rows.map((row) => row.kind), ['lightweight_tool_group', 'timeline', 'lightweight_tool_group'])
  assert.equal(rows[0]!.kind, 'lightweight_tool_group')
  assert.equal(rows[0]!.kind === 'lightweight_tool_group' ? rows[0].items.length : 0, 3)
  assert.equal(rows[0]!.kind === 'lightweight_tool_group' ? rows[0].trailing : true, false)
  assert.equal(rows[1]!.kind === 'timeline' ? rows[1].entry.toolCallId : '', 'edit-1')
  assert.equal(rows[2]!.kind, 'lightweight_tool_group')
  assert.equal(rows[2]!.kind === 'lightweight_tool_group' ? rows[2].items[0]!.error : '', 'permission denied')
  assert.equal(rows[2]!.kind === 'lightweight_tool_group' ? rows[2].trailing : false, true)
  assert.equal(JSON.stringify(rows).includes('secret file body'), false)
  assert.equal(JSON.stringify(rows).includes('hidden search hits'), false)
})

test('buildLightweightToolTimelineRows ignores lightweight results while grouping consecutive tools', () => {
  const calls = [
    tool({ toolCallId: 'search-1', toolName: 'web_search', command: 'search', params: { query: 'alpha' }, status: 'completed', output: 'hidden alpha body' }),
    tool({ toolCallId: 'search-2', toolName: 'web_search', command: 'search', params: { query: 'beta' }, status: 'completed', output: 'hidden beta body' }),
  ]
  const rows = buildLightweightToolTimelineRows(
    [
      { id: 's1', kind: 'tool_start' as const, toolCallId: 'search-1' },
      { id: 'r1', kind: 'tool_result' as const, toolCallId: 'search-1' },
      { id: 's2', kind: 'tool_start' as const, toolCallId: 'search-2' },
      { id: 'r2', kind: 'tool_result' as const, toolCallId: 'search-2' },
    ],
    (id) => calls.find((item) => item.toolCallId === id),
  )

  assert.deepEqual(rows.map((row) => row.kind), ['lightweight_tool_group'])
  const group = rows[0]!
  assert.equal(group.kind, 'lightweight_tool_group')
  assert.equal(group.kind === 'lightweight_tool_group' ? group.items.length : 0, 2)
  assert.equal(group.kind === 'lightweight_tool_group' ? buildLightweightToolGroupSummary(group.items, 'zh') : '', '搜索 2 次')
  assert.equal(JSON.stringify(rows).includes('tool_result'), false)
  assert.equal(JSON.stringify(rows).includes('hidden alpha body'), false)
  assert.equal(JSON.stringify(rows).includes('hidden beta body'), false)
})

test('buildLightweightToolTimelineRows keeps non-lightweight results as group breakers', () => {
  const calls = [
    tool({ toolCallId: 'search-1', toolName: 'web_search', command: 'search', params: { query: 'alpha' }, status: 'completed' }),
    tool({ toolCallId: 'exec-1', toolName: 'exec', command: 'run', params: { command: 'npm test' }, status: 'completed', output: 'ok' }),
    tool({ toolCallId: 'search-2', toolName: 'web_search', command: 'search', params: { query: 'beta' }, status: 'completed' }),
  ]
  const rows = buildLightweightToolTimelineRows(
    [
      { id: 's1', kind: 'tool_start' as const, toolCallId: 'search-1' },
      { id: 'r1', kind: 'tool_result' as const, toolCallId: 'search-1' },
      { id: 'e1', kind: 'tool_start' as const, toolCallId: 'exec-1' },
      { id: 'er1', kind: 'tool_result' as const, toolCallId: 'exec-1' },
      { id: 's2', kind: 'tool_start' as const, toolCallId: 'search-2' },
      { id: 'r2', kind: 'tool_result' as const, toolCallId: 'search-2' },
    ],
    (id) => calls.find((item) => item.toolCallId === id),
  )

  assert.deepEqual(rows.map((row) => row.kind), ['lightweight_tool_group', 'timeline', 'timeline', 'lightweight_tool_group'])
  assert.equal(rows[1]!.kind === 'timeline' ? rows[1].entry.id : '', 'e1')
  assert.equal(rows[2]!.kind === 'timeline' ? rows[2].entry.id : '', 'er1')
  assert.equal(rows[0]!.kind === 'lightweight_tool_group' ? rows[0].trailing : true, false)
  assert.equal(rows[3]!.kind === 'lightweight_tool_group' ? rows[3].trailing : false, true)
})

test('buildLightweightToolTimelineRows marks only the unbroken final group as trailing', () => {
  const calls = [
    tool({ toolCallId: 'search-1', toolName: 'web_search', command: 'search', params: { query: 'alpha' }, status: 'completed' }),
    tool({ toolCallId: 'search-2', toolName: 'web_search', command: 'search', params: { query: 'beta' }, status: 'completed' }),
  ]
  const rows = buildLightweightToolTimelineRows(
    [
      { id: 's1', kind: 'tool_start' as const, toolCallId: 'search-1' },
      { id: 'text-1', kind: 'text' as const, content: 'next content' },
      { id: 's2', kind: 'tool_start' as const, toolCallId: 'search-2' },
    ],
    (id) => calls.find((item) => item.toolCallId === id),
  )

  assert.deepEqual(rows.map((row) => row.kind), ['lightweight_tool_group', 'timeline', 'lightweight_tool_group'])
  assert.equal(rows[0]!.kind === 'lightweight_tool_group' ? rows[0].trailing : true, false)
  assert.equal(rows[2]!.kind === 'lightweight_tool_group' ? rows[2].trailing : false, true)
})

test('buildLightweightToolGroupSummary counts activity categories', () => {
  const items = [
    buildLightweightToolDisplay(tool({ toolName: 'web_search', command: 'search', params: { query: 'a' } }))!,
    buildLightweightToolDisplay(tool({ toolName: 'grep', command: 'search', params: { pattern: 'b' } }))!,
    buildLightweightToolDisplay(tool({ toolName: 'web_extract', command: 'extract', params: { url: 'https://example.test' } }))!,
    buildLightweightToolDisplay(tool({ toolName: 'http_request', command: 'request', params: { method: 'GET', url: 'https://api.test' } }))!,
    buildLightweightToolDisplay(tool({ toolName: 'file_read', command: 'read', params: { requests: [{ file_path: 'a.ts' }, { file_path: 'b.ts' }] } }))!,
  ]

  assert.equal(buildLightweightToolGroupSummary(items, 'zh'), '搜索 2 次，浏览 2 个网页，读取 2 个文件')
  assert.equal(buildLightweightToolGroupSummary(items, 'en'), '2 searches, 2 web pages, 2 files read')
})

test('isLightweightToolCall excludes file edits and writes', () => {
  assert.equal(isLightweightToolCall(tool({ toolName: 'file_read', command: 'read' })), true)
  assert.equal(isLightweightToolCall(tool({ toolName: 'grep', command: 'search' })), true)
  assert.equal(isLightweightToolCall(tool({ toolName: 'glob', command: 'find' })), true)
  assert.equal(isLightweightToolCall(tool({ toolName: 'search_file', command: 'search' })), true)
  assert.equal(isLightweightToolCall(tool({ toolName: 'file_edit', command: 'edit' })), false)
  assert.equal(isLightweightToolCall(tool({ toolName: 'file_write', command: 'write' })), false)
  assert.equal(isLightweightToolCall(tool({ toolName: 'exec', command: 'run' })), false)
})

test('buildToolCallSummary formats newly added tools compactly', () => {
  assert.equal(
    buildToolCallSummary(tool({ toolName: 'grep', command: 'search', params: { pattern: 'BuildToolDefs', path: 'internal/tools', glob: '*.go' } })),
    'BuildToolDefs in internal/tools (*.go)',
  )
  assert.equal(
    buildToolCallSummary(tool({ toolName: 'glob', command: 'find', params: { pattern: '**/*.go', path: 'internal/tools' } })),
    '**/*.go in internal/tools',
  )
  assert.equal(
    buildToolCallSummary(tool({ toolName: 'search_files', command: 'search', params: { query: 'BuildToolDefs', path: 'internal/tools', pattern: '*.go' } })),
    'BuildToolDefs in internal/tools (*.go)',
  )
  assert.equal(
    buildToolCallSummary(tool({ toolName: 'web_extract', command: 'extract', params: { url: 'https://example.test/docs/intro?utm=long' } })),
    'example.test/docs/intro',
  )
  assert.equal(buildToolCallSummary(tool({ toolName: 'skills', command: 'list', params: {} })), 'List skills')
  assert.equal(buildToolCallSummary(tool({ toolName: 'skills', command: 'view', params: { name: 'imagegen' } })), 'imagegen')
  assert.equal(buildToolCallSummary(tool({ toolName: 'todo', command: 'list', params: {} })), 'List todos')
  assert.equal(
    buildToolCallSummary(tool({ toolName: 'todo', command: 'update', params: { items: [{ id: 'a' }, { id: 'b' }] } })),
    'Update 2 todos',
  )
  assert.equal(buildToolCallSummary(tool({ toolName: 'process', command: 'list', params: {} })), 'List processes')
  assert.equal(buildToolCallSummary(tool({ toolName: 'process', command: 'status', params: { process_id: 'proc-1' } })), 'proc-1')
})

test('buildToolCallSummary uses file tool paths and operations', () => {
  assert.equal(
    buildToolCallSummary(tool({ toolName: 'file_read', command: 'read', params: { file_path: 'frontend/src/App.vue' } })),
    'Read App.vue',
  )
  assert.equal(
    buildToolCallSummary(tool({
      toolName: 'file_edit',
      command: 'edit',
      params: { file_path: 'cli/src/utils/format.ts', old_string: 'old', new_string: 'new' },
    })),
    'Update format.ts',
  )
  assert.equal(
    buildToolCallSummary(tool({ toolName: 'file_write', command: 'write', params: { file_path: 'frontend/src/utils/fileToolDisplay.ts' } })),
    'Write fileToolDisplay.ts',
  )
  assert.equal(
    buildToolCallSummary(tool({ toolName: 'file_read', command: 'read', params: { requests: [{ file_path: 'a.ts' }, { file_path: 'b.ts' }] } })),
    'Read 2 files',
  )
  assert.equal(
    buildToolCallSummary(tool({ toolName: 'file_write', command: 'write', params: { writes: [{ file_path: 'a.ts', content: 'a' }] } })),
    'Write 1 file',
  )
  assert.equal(
    buildToolCallSummary(tool({
      toolName: 'file_edit',
      command: 'edit',
      params: { edits: [{ file_path: 'a.ts', operations: [{ old_string: '', new_string: 'x' }] }, { file_path: 'b.ts', operations: [{ old_string: 'a', new_string: 'b' }] }] },
    })),
    'Update 1 file / Create 1 file',
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
    filterToolParamsForDetail(tool({ toolName: 'search_files', command: 'search', params: { query: 'BuildToolDefs', path: 'internal/tools', pattern: '*.go', max_matches: 20 } })),
    { max_matches: 20 },
  )
  assert.deepEqual(
    filterToolParamsForDetail(tool({ toolName: 'grep', command: 'search', params: { pattern: 'BuildToolDefs', path: 'internal/tools', glob: '*.go', head_limit: 20 } })),
    { head_limit: 20 },
  )
  assert.deepEqual(
    filterToolParamsForDetail(tool({ toolName: 'glob', command: 'find', params: { pattern: '**/*.go', path: 'internal/tools', limit: 20 } })),
    { limit: 20 },
  )
  assert.deepEqual(
    filterToolParamsForDetail(tool({ toolName: 'web_extract', command: 'extract', params: { url: 'https://example.test/docs/intro' } })),
    {},
  )
  assert.deepEqual(
    filterToolParamsForDetail(tool({ toolName: 'process', command: 'stop', params: { process_id: 'proc-1', reason: 'cleanup' } })),
    { reason: 'cleanup' },
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

test('parseFileReadOutput supports range-based output format', () => {
  const parsed = parseFileReadOutput([
    'File: cli/src/utils/timelineFormat.ts',
    'Total lines: 462',
    'Range 1 lines 120-159:',
    '   120\tconst hidden = true',
  ].join('\n'))

  assert.deepEqual(parsed, {
    filePath: 'cli/src/utils/timelineFormat.ts',
    totalLines: 462,
    startLine: 120,
    endLine: 159,
    truncated: false,
  })
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

  assert.equal(display?.summary, 'Wrote 1 line to fileToolDisplay.ts')
  assert.equal(display?.fileName, 'fileToolDisplay.ts')
  assert.deepEqual(display?.diffLines, [
    { kind: 'added', newLine: 1, text: 'export const ok = true' },
  ])
})

test('buildFileToolDisplay prefers backend metadata diff and basename summary', () => {
  const display = buildFileToolDisplay(tool({
    toolName: 'file_edit',
    command: 'edit',
    params: {
      file_path: 'frontend/src/utils/fileToolDisplay.ts',
      old_string: 'old',
      new_string: 'new',
    },
    metadata: {
      filePath: 'frontend/src/utils/fileToolDisplay.ts',
      operation: 'Update',
      summary: 'Updated fileToolDisplay.ts',
      diffLines: [
        { kind: 'context', oldLine: 9, newLine: 9, text: 'before' },
        { kind: 'removed', oldLine: 10, text: 'old' },
        { kind: 'added', newLine: 10, text: 'new' },
        { kind: 'context', oldLine: 11, newLine: 11, text: 'after' },
      ],
    },
  }))

  assert.equal(display?.fileName, 'fileToolDisplay.ts')
  assert.equal(display?.filePath, 'frontend/src/utils/fileToolDisplay.ts')
  assert.equal(display?.summary, 'Updated fileToolDisplay.ts')
  assert.deepEqual(display?.diffLines, [
    { kind: 'context', oldLine: 9, newLine: 9, text: 'before' },
    { kind: 'removed', oldLine: 10, text: 'old' },
    { kind: 'added', newLine: 10, text: 'new' },
    { kind: 'context', oldLine: 11, newLine: 11, text: 'after' },
  ])
})

test('buildFileToolDisplay uses first item when metadata is array', () => {
  const display = buildFileToolDisplay(tool({
    toolName: 'file_edit',
    command: 'edit',
    params: {},
    metadata: [{
      filePath: 'a.ts',
      operation: 'Update',
      summary: 'Updated a.ts',
      diffLines: [{ kind: 'added', newLine: 1, text: 'x' }],
    }, {
      filePath: 'b.ts',
      operation: 'Create',
      summary: 'Created b.ts',
      diffLines: [{ kind: 'added', newLine: 1, text: 'y' }],
    }],
  }))

  assert.equal(display?.filePath, 'a.ts')
  assert.equal(display?.summary, 'Updated a.ts')
})

test('ToolCallInline routes file tools through FileToolDisplay', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ToolCallInline.vue'), 'utf8')

  assert.match(source, /import FileToolDisplay/)
  assert.match(source, /<FileToolDisplay v-if="isFileToolCall"/)
  assert.match(source, /showResult && !isFileToolCall/)
  assert.doesNotMatch(readFileSync(resolve(import.meta.dirname, '../src/components/chat/FileToolDisplay.vue'), 'utf8'), /file-tool-diff-guide|├─|└─/)
  assert.doesNotMatch(readFileSync(resolve(import.meta.dirname, '../src/components/chat/FileToolDisplay.vue'), 'utf8'), /file-tool-diff-separator\">\\.\\.\\.</)
})

test('LightweightToolGroup left-aligns expanded items and uses status icons', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/components/chat/LightweightToolGroup.vue'), 'utf8')

  assert.match(source, /import \{ mdiChevronDown, mdiFileSearchOutline \} from '@mdi\/js'/)
  assert.match(source, /getToolCallLabel\(toolName === 'search_file' \? 'search_files' : toolName/)
  assert.match(source, /\{\{ toolLabel\(item\.toolName\) \}\}/)
  assert.doesNotMatch(source, /<span class="light-tool-item-label">\{\{ item\.label \}\}<\/span>/)
  assert.match(source, /statusSymbol\(item\.status\)/)
  assert.match(source, /function isActiveStatus\(status: LightweightToolDisplay\['status'\]\)/)
  assert.match(source, /status === 'pending' \|\| status === 'reviewing' \|\| status === 'executing'/)
  assert.match(source, /return '\\u2713'/)
  assert.match(source, /return '\\u2717'/)
  assert.match(source, /:aria-label="getToolCallStatusLabel\(item\.status, \(key\) => t\(key\)\)"/)
  assert.match(source, /v-else-if="isActiveStatus\(item\.status\)" class="light-tool-running-dot" aria-hidden="true"/)
  assert.match(source, /var\(--tool-success-dot/)
  assert.match(source, /\.light-tool-group--completed\s*\{[\s\S]*var\(--accent-primary\) 52%/)
  assert.match(source, /\.light-tool-group--completed:hover\s*\{[\s\S]*var\(--accent-primary\) 75%/)
  assert.doesNotMatch(source, /\.light-tool-group--completed\s*\{[^}]*var\(--tool-success-(?:border|dot)/)
  assert.match(source, /var\(--tool-error-dot/)
  assert.match(source, /padding: 0 10px 10px 10px;/)
  assert.match(source, /grid-template-columns: 18px auto minmax\(0, 1fr\);/)
  assert.match(source, /<Transition name="tool-subagent-expand">/)
  assert.match(source, /runningOverride/)
  assert.match(source, /transition: opacity 180ms ease, max-height 250ms ease;/)
  assert.match(source, /transition: opacity 120ms ease, max-height 180ms ease;/)
  assert.match(source, /\.tool-subagent-expand-enter-active,[\s\S]*\.tool-subagent-expand-leave-active\s*\{[\s\S]*overflow: hidden;/)
  assert.match(source, /linear-gradient\(/)
  assert.match(source, /position: relative;/)
  assert.match(source, /\.light-tool-group--running\s*\{[\s\S]*animation: light-tool-running-breathe 2\.3s ease-in-out infinite;/)
  assert.match(source, /\.light-tool-group--running::before\s*\{[\s\S]*pointer-events: none;[\s\S]*animation: light-tool-sweep-breathe 2\.7s ease-in-out infinite;/)
  assert.match(source, /\.light-tool-status--active\s*\{[\s\S]*color: var\(--tool-running-dot, var\(--accent-primary\)\);/)
  assert.match(source, /\.light-tool-running-dot\s*\{[\s\S]*width: 8px;[\s\S]*height: 8px;[\s\S]*animation: light-tool-running-dot-pulse 1\.4s ease-in-out infinite;/)
  assert.match(source, /@keyframes light-tool-running-breathe/)
  assert.match(source, /@keyframes light-tool-running-dot-pulse/)
  assert.match(source, /@keyframes light-tool-sweep-breathe/)
  assert.match(source, /transform: translateX\(-110%\);/)
  assert.match(source, /transform: translateX\(110%\);/)
  assert.doesNotMatch(source, /background: var\(--card-bg\);/)
  assert.match(source, /@media \(prefers-reduced-motion: reduce\)/)
  assert.match(source, /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.light-tool-group--running,[\s\S]*\.light-tool-running-dot,[\s\S]*\.light-tool-group--running::before\s*\{[\s\S]*animation: none;/)
  assert.match(source, /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.light-tool-group--running::before\s*\{[\s\S]*opacity: 0;[\s\S]*transform: none;/)
  assert.doesNotMatch(source, /\.light-tool-group--completed\s*\{[^}]*animation:/)
  assert.doesNotMatch(source, /\.light-tool-group--failed\s*\{[^}]*animation:/)
  assert.doesNotMatch(source, /padding: 0 10px 10px 36px;/)
  assert.doesNotMatch(source, /grid-column: 2 \/ 4;/)
})

test('AssistantMessageBody passes streaming tail state to lightweight tool groups', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/components/chat/AssistantMessageBody.vue'), 'utf8')

  assert.match(source, /:running-override="isStreaming && row\.trailing"/)
})

test('ToolExecutionDetailDialog does not force lightweight groups into running state', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ToolExecutionDetailDialog.vue'), 'utf8')

  assert.doesNotMatch(source, /running-override/)
})

test('tool executing spinner uses a centered circular stroke instead of a wedge path', () => {
  const headerSource = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ToolCallHeader.vue'), 'utf8')
  const inlineSource = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ToolCallInline.vue'), 'utf8')

  assert.match(headerSource, /tool-status-spinner-track/)
  assert.match(headerSource, /tool-status-spinner-head/)
  assert.match(headerSource, /stroke-dasharray="18 44"/)
  assert.doesNotMatch(headerSource, /d="M4 12a8 8 0 018-8v8H4z"/)
  assert.match(inlineSource, /inline-spinner-track/)
  assert.match(inlineSource, /inline-spinner-head/)
})
