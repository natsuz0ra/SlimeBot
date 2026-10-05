import assert from 'node:assert/strict'
import test from 'node:test'
import { cooperativeError, cooperativeReport, cooperativeStatus, latestAgentTurns, orderedAgentTree } from '../src/utils/cooperative'
import type { CooperativeAgent, CooperativeTurn } from '../src/api/cooperative'

const turn = (id: string, agentId: string, createdAt: string, revision = 1): CooperativeTurn => ({ id, agentId, createdAt, revision, requestId: 'request', status: 'running', inputTokens: 0, outputTokens: 0 })
const agent = (id: string, parentId: string): CooperativeAgent => ({ id, parentId, rootId: 'root', title: id, task: '', profile: 'worker', mode: 'continuable', contextMode: 'isolated', modelId: 'model', workspace: '', depth: 1, createdAt: '', requestId: '' })

test('the latest turn wins even when snapshots arrive out of order', () => {
  const current = turn('new', 'agent', '2026-10-04T10:00:00Z', 2)
  const result = latestAgentTurns([current, turn('old', 'agent', '2026-10-04T09:00:00Z'), { ...current, revision: 1 }])
  assert.equal(result.get('agent')?.revision, 2)
})
test('the tree keeps descendants with their parent and preserves orphaned history', () => {
  assert.deepEqual(orderedAgentTree([agent('child', 'parent'), agent('orphan', 'missing'), agent('parent', 'root')], 'root').map(a => a.id), ['parent', 'child', 'orphan'])
})
test('a malformed cycle cannot cause an infinite tree traversal', () => {
  assert.deepEqual(orderedAgentTree([agent('a', 'b'), agent('b', 'a')], 'a').map(a => a.id), ['b', 'a'])
})
test('routing metadata does not show a stale artifact state in a report', () => {
  const report = '已创建文件。'
  assert.equal(cooperativeReport(`${report}\n\nArtifact: 30fbcfe4-2587-4ec2-a8a8-798cc0dc34be (ready); requires integration before the write task is complete.`), report)
  assert.equal(cooperativeReport('Artifact: explained by the agent'), 'Artifact: explained by the agent')
})
test('task and artifact states have clear Chinese and English labels', () => {
  assert.equal(cooperativeStatus('awaiting_integration'), '等待集成')
  assert.equal(cooperativeStatus('validation_failed', true), 'Validation failed')
  assert.equal(cooperativeStatus('custom'), 'custom')
})

test('action errors preserve server details and explain recovery', () => {
  assert.equal(cooperativeError({ response: { data: { error: 'artifact must pass validation before integration' } }, message: 'Request failed with status code 409' }), '成果尚未通过验证，请先运行验证。')
  assert.equal(cooperativeError({ response: { data: { error: 'specific conflict details' } } }), 'specific conflict details')
  assert.equal(cooperativeError(new Error('Network Error'), true), 'Connection unavailable. Please retry shortly.')
})
