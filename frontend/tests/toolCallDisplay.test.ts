import assert from 'node:assert/strict'
import test from 'node:test'
import { mdiConsoleLine, mdiHelpCircleOutline, mdiWeb } from '@mdi/js'
import { getToolCallIcon, getToolCallLabel, getToolCallStatusLabel, getToolCallStatusTone } from '../src/composables/chat/useToolCallDisplay'

const t = (key: string) => `t:${key}`

test('tool call display maps known tools to labels and icons', () => {
  assert.equal(getToolCallLabel('exec', t), 't:toolExec')
  assert.equal(getToolCallLabel('http_request', t), 't:toolHttpRequest')
  assert.equal(getToolCallLabel('custom_tool', t), 'custom_tool')
  assert.equal(getToolCallIcon('web_search'), mdiWeb)
  assert.equal(getToolCallIcon('ask_questions'), mdiHelpCircleOutline)
  assert.equal(getToolCallIcon('custom_tool'), mdiConsoleLine)
})

test('tool call display maps statuses to labels and tones', () => {
  assert.equal(getToolCallStatusLabel('pending', t), 't:toolCallPending')
  assert.equal(getToolCallStatusTone('completed'), 'success')
  assert.equal(getToolCallStatusTone('rejected'), 'error')
  assert.equal(getToolCallStatusTone('error'), 'error')
})
