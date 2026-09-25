import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import test from 'node:test'
import assert from 'node:assert/strict'

const projectRoot = resolve(import.meta.dirname, '..')

test('settings provider cards show protocol badges separately from provider names', () => {
  const source = readFileSync(resolve(projectRoot, 'src/components/settings/SettingsLLMTab.vue'), 'utf8')

  assert.match(source, /protocol === 'anthropic'/)
  assert.match(source, /'providerOpenAI'/)
  assert.match(source, /llm-protocol-badge/)
  assert.match(source, /provider\.name/)
})
