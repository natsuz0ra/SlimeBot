import assert from 'node:assert/strict'
import test from 'node:test'
import { expandSlashTemplate, filterSlashCommands, getSlashCommands, slashQuery, validTemplateSlash } from '../src/utils/slashCommands'

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
    [{ id: 'review', title: 'Review code', slash: 'review', content: 'Look at {{cursor}} now' }],
    [
      { id: 'a', name: 'Review', description: 'Skill review', enabled: true },
      { id: 'b', name: 'Plan', description: '', enabled: true },
      { id: 'c', name: 'Off', description: '', enabled: false },
    ],
    name => name, name => `Use the ${name} skill`,
  )
  assert.equal(validTemplateSlash('plan'), false)
  assert.equal(validTemplateSlash('a_b-2'), true)
  assert.equal(validTemplateSlash('UPPER'), false)
  assert.deepEqual(filterSlashCommands(commands, 'review').map(item => item.slash), ['review', 'review-2'])
  assert.deepEqual(commands.filter(item => item.kind === 'skill').map(item => item.slash), ['review-2', 'plan-2'])
})

test('cursor marker uses the textarea UTF-16 index', () => {
  assert.deepEqual(expandSlashTemplate('🔎 {{cursor}} here'), { text: '🔎  here', caret: 3 })
  assert.deepEqual(expandSlashTemplate('plain'), { text: 'plain', caret: 5 })
})
