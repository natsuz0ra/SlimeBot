import assert from 'node:assert/strict'
import test from 'node:test'
import { taskRunDurationSeconds, taskRunPreview } from '../src/utils/scheduleHistory'

test('run durations use actual timestamps and handle incomplete/invalid records', () => {
  const start = '2026-09-30T09:00:00+08:00'
  assert.equal(taskRunDurationSeconds(start, '2026-09-30T01:01:05Z'), 65)
  assert.equal(taskRunDurationSeconds(start, start), 0)
  assert.equal(taskRunDurationSeconds(start), null)
  assert.equal(taskRunDurationSeconds(start, 'invalid'), null)
  assert.equal(taskRunDurationSeconds(start, '2026-09-30T08:59:59+08:00'), null)
})

test('history previews collapse Markdown headings and blank lines into a readable excerpt', () => {
  assert.equal(taskRunPreview('## 项目进展\n\n- 搜索完成\n- **运行记录**验证中'), '项目进展 搜索完成 运行记录验证中')
  assert.equal(taskRunPreview('<script>alert(1)</script>'), '<script>alert(1)</script>')
})
