import assert from 'node:assert/strict'
import test from 'node:test'
import { taskCenterRows, taskCenterStatus } from '../src/utils/taskCenter'

test('任务中心统一来源 ID、状态和时间排序', () => {
  const rows = taskCenterRows([
    { id: 'same', taskId: 'same', processId: 'p', sessionId: 'chat', command: 'npm test', status: 'failed', error: 'failed', exitCode: 1, autoWake: true, startedAt: '2026-01-01T00:00:00Z' },
  ], [
    { id: 'same', rootId: 'root', parentId: '', title: 'Worker', task: 'work', profile: '', mode: '', contextMode: '', modelId: '', workspace: '', depth: 1, requestId: '', createdAt: '2026-01-02T00:00:00Z', latestTurn: { id: 'turn', agentId: 'same', requestId: '', status: 'completed', revision: 1, inputTokens: 1, outputTokens: 2, createdAt: '2026-01-03T00:00:00Z' } },
  ], [])
  assert.deepEqual(rows.map(row => row.id), ['agent:same', 'background:same'])
  assert.equal(taskCenterStatus('waking'), 'running')
  assert.equal(taskCenterStatus('completed'), 'ok')
})
