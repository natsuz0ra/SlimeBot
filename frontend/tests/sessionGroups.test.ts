import assert from 'node:assert/strict'
import test from 'node:test'
import { buildSidebarSessionGroups, groupSessionsByDirectory } from '../src/utils/sessionGroups'

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

test('pinned chats are sorted independently and directory filters also apply to pins', () => {
  const sessions = [
    { id: 'a', name: 'old pin', updatedAt: '2026-11-28', isPinned: true, pinnedAt: '2026-11-25', workingDirectory: '/a' },
    { id: 'b', name: 'new pin', updatedAt: '2026-11-20', isPinned: true, pinnedAt: '2026-11-27', workingDirectory: '/b' },
    { id: 'c', name: 'regular chat', updatedAt: '2026-11-28', workingDirectory: '/a' },
    { id: 'd', name: 'legacy', updatedAt: '2026-11-28' },
  ]
  const all = buildSidebarSessionGroups(sessions, 'Other')
  assert.deepEqual(all.pinnedSessions.map(session => session.id), ['b', 'a'])
  assert.deepEqual(all.directoryGroups.flatMap(group => group.sessions.map(session => session.id)), ['c', 'd'])
  assert.deepEqual(buildSidebarSessionGroups(sessions, 'Other', '/a').pinnedSessions.map(session => session.id), ['a'])
  assert.deepEqual(buildSidebarSessionGroups(sessions, 'Other', '').directoryGroups[0]?.sessions.map(session => session.id), ['d'])
})
