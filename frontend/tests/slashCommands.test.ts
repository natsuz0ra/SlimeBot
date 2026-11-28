import assert from 'node:assert/strict'
import test from 'node:test'
import { builtInCommands, filterSlashCommands, getSlashCommands, slashQuery } from '../src/utils/slashCommands'

test('slash search activates only for a standalone slash prefix at the caret', () => {
  assert.equal(slashQuery('/', 1), '')
  assert.equal(slashQuery('/CoM', 4), 'com')
  assert.equal(slashQuery('/terminal', 1), undefined)
  assert.equal(slashQuery('abc /terminal', 13), undefined)
  assert.equal(slashQuery('/usr/bin', 8), undefined)
  assert.equal(slashQuery('/review details', 15), undefined)
})

test('builtins reserve names and enabled skills get stable, independent commands', () => {
  const commands = getSlashCommands(
    [
      { id: 'a', name: 'Review', description: 'Skill review', enabled: true },
      { id: 'b', name: 'Plan', description: '', enabled: true },
      { id: 'c', name: 'Off', description: '', enabled: false },
      { id: 'd', name: 'review', description: '', enabled: true },
    ],
    name => name, name => `Use the ${name} skill`,
  )
  assert.deepEqual(commands.filter(item => item.kind === 'workbench').map(item => item.slash), [...builtInCommands])
  assert.deepEqual(filterSlashCommands(commands, 'review').map(item => item.slash), ['review', 'review-2'])
  const skills = commands.filter(item => item.kind === 'skill')
  assert.deepEqual(skills.map(item => item.slash), ['review', 'plan-2', 'review-2'])
  assert.equal(skills[0]?.content, 'Use the Review skill')
})
