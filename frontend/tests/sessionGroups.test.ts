import assert from 'node:assert/strict'
import test from 'node:test'
import { groupSessionsByDirectory } from '../src/utils/sessionGroups'

test('groups sessions by full directory and leaves legacy sessions unclassified', () => {
  const sessions = [
    { id: 'a', name: 'newest', updatedAt: '', workingDirectory: '/one/app' },
    { id: 'b', name: 'legacy', updatedAt: '' },
    { id: 'c', name: 'other app', updatedAt: '', workingDirectory: '/two/app' },
    { id: 'd', name: 'older', updatedAt: '', workingDirectory: '/one/app' },
  ]
  const groups = groupSessionsByDirectory(sessions, '未分类')
  assert.deepEqual(groups.map((group) => group.path), ['/one/app', '/two/app', ''])
  assert.deepEqual(groups[0]?.sessions.map((session) => session.id), ['a', 'd'])
  assert.equal(groups[2]?.name, '未分类')
})
