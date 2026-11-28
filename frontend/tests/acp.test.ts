import test from 'node:test'
import assert from 'node:assert/strict'
import {
  acpAgentIdFromEngine,
  acpEngineId,
  chooseEngine,
  composeEngineOptions,
  isACPEngine,
  parseACPArguments,
} from '../src/utils/acp.ts'
import type { ACPConfig } from '../src/api/acp.ts'
import type { SelectOption } from '../src/components/ui/AppSelect.vue'

const localOptions: SelectOption[] = [
  { value: 'a1', label: 'Prime', group: 'Alpha' },
  { value: 'b1', label: 'Atlas', group: 'Beta' },
]

function agent(overrides: Partial<ACPConfig> & Pick<ACPConfig, 'id' | 'name'>): ACPConfig {
  return {
    command: 'zed',
    args: [],
    timeoutSec: 90,
    isEnabled: true,
    ...overrides,
  }
}

test('acp engine ids round-trip through the acp: prefix', () => {
  assert.equal(acpEngineId('agent-1'), 'acp:agent-1')
  assert.equal(isACPEngine('acp:agent-1'), true)
  assert.equal(acpAgentIdFromEngine('acp:agent-1'), 'agent-1')

  assert.equal(isACPEngine('a1'), false)
  assert.equal(isACPEngine('acp:'), false)
  assert.equal(acpAgentIdFromEngine('a1'), undefined)
  assert.equal(acpAgentIdFromEngine(undefined), undefined)
})

test('composeEngineOptions returns local options untouched without enabled external agents', () => {
  assert.equal(composeEngineOptions(localOptions, [], '本地', '外部'), localOptions)
  assert.equal(
    composeEngineOptions(localOptions, [agent({ id: 'x', name: 'X', isEnabled: false })], '本地', '外部'),
    localOptions,
  )
})

test('composeEngineOptions groups local models and enabled external agents', () => {
  const options = composeEngineOptions(
    localOptions,
    [
      agent({ id: 'on', name: 'Zed' }),
      agent({ id: 'off', name: 'Hidden', isEnabled: false }),
      agent({ id: 'on2', name: 'Claude Code', timeoutSec: 30 }),
    ],
    'SlimeBot 模型',
    '外部 ACP 代理',
  )

  assert.deepEqual(options, [
    { value: 'a1', label: 'Prime', group: 'SlimeBot 模型' },
    { value: 'b1', label: 'Atlas', group: 'SlimeBot 模型' },
    { value: 'acp:on', label: 'Zed', group: '外部 ACP 代理' },
    { value: 'acp:on2', label: 'Claude Code', group: '外部 ACP 代理' },
  ])
})

test('chooseEngine prefers a still-valid selection, then the bound session agent, then the remembered value', () => {
  const options = composeEngineOptions(localOptions, [agent({ id: 'on', name: 'Zed' })], '本地', '外部')

  assert.equal(chooseEngine(options, 'acp:on', 'other', 'a1'), 'acp:on')
  assert.equal(chooseEngine(options, 'stale', 'on', 'a1'), 'acp:on')
  assert.equal(chooseEngine(options, undefined, undefined, 'b1'), 'b1')
  assert.equal(chooseEngine(options, undefined, undefined, 'stale'), 'a1')
  assert.equal(chooseEngine([], undefined, 'on', 'b1'), 'acp:on')
  assert.equal(chooseEngine([], undefined, undefined, 'b1'), '')
})

test('parseACPArguments accepts JSON arrays and newline separated values', () => {
  assert.deepEqual(parseACPArguments(''), [])
  assert.deepEqual(parseACPArguments('   '), [])
  assert.deepEqual(parseACPArguments('["--foo", "bar"]'), ['--foo', 'bar'])
  assert.deepEqual(parseACPArguments('["--foo"]\n'), ['--foo'])
  assert.deepEqual(parseACPArguments('--foo\nbar\n'), ['--foo', 'bar'])
  assert.deepEqual(parseACPArguments('--foo\n\nbar'), ['--foo', 'bar'])
  assert.throws(() => parseACPArguments('[1, 2]'), TypeError)
  assert.deepEqual(parseACPArguments('{"a": 1}'), ['{"a": 1}'])
})
