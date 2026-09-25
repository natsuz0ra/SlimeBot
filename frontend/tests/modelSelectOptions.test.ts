import test from 'node:test'
import assert from 'node:assert/strict'
import { groupedModelOptions } from '../src/utils/modelSelectOptions.ts'
import type { LLMConfig } from '../src/types/settings.ts'

test('model choices group by provider and show model names only', () => {
  const models = [
    { id: 'b2', providerId: 'b', providerName: 'Beta', name: 'Zeta' },
    { id: 'a1', providerId: 'a', providerName: 'Alpha', name: 'Prime' },
    { id: 'b1', providerId: 'b', providerName: 'Beta', name: 'Atlas' },
  ] as LLMConfig[]

  assert.deepEqual(groupedModelOptions(models), [
    { value: 'a1', label: 'Prime', group: 'Alpha' },
    { value: 'b1', label: 'Atlas', group: 'Beta' },
    { value: 'b2', label: 'Zeta', group: 'Beta' },
  ])
  assert.equal(models[0]?.id, 'b2')
})
