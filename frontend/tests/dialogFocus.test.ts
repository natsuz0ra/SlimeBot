import assert from 'node:assert/strict'
import test from 'node:test'
import { topmostModal } from '../src/utils/dialogFocus'
test('a collaboration drawer owns keyboard focus over the scheduled-run dialog', () => {
  assert.equal(topmostModal([{ panel: 'run', layer: 200, visible: true }, { panel: 'collaboration', layer: 300, visible: true }]), 'collaboration')
  assert.equal(topmostModal([{ panel: 'run', layer: 200, visible: true }, { panel: 'collaboration', layer: 300, visible: false }]), 'run')
})
test('hidden and earlier dialogs do not steal focus', () => {
  assert.equal(topmostModal([{ panel: 'first', layer: 200, visible: true }, { panel: 'second', layer: 200, visible: true }, { panel: 'hidden', layer: 400, visible: false }]), 'second')
  assert.equal(topmostModal([]), undefined)
})
