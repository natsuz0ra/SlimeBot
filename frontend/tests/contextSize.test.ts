import assert from 'node:assert/strict'
import test from 'node:test'
import {
  CONTEXT_SIZE_DEFAULT,
  CONTEXT_SIZE_MAX,
  CONTEXT_SIZE_MIN,
  clampContextSize,
  contextSizeToSlider,
  formatContextSize,
  sliderToContextSize,
} from '../src/utils/contextSize'

test('clampContextSize keeps values in the supported range', () => {
  assert.equal(clampContextSize(4_000), CONTEXT_SIZE_MIN)
  assert.equal(clampContextSize(2_000_000), CONTEXT_SIZE_MAX)
  assert.equal(clampContextSize('bad'), CONTEXT_SIZE_DEFAULT)
  assert.equal(clampContextSize('128000'), 128_000)
})

test('formatContextSize renders compact labels', () => {
  assert.equal(formatContextSize(8_000), '8K')
  assert.equal(formatContextSize(128_000), '128K')
  assert.equal(formatContextSize(1_000_000), '1M')
})

test('context size slider round trips through logarithmic mapping', () => {
  assert.equal(contextSizeToSlider(CONTEXT_SIZE_MIN), 0)
  assert.equal(contextSizeToSlider(CONTEXT_SIZE_MAX), 100)

  const roundTrip = sliderToContextSize(contextSizeToSlider(128_000))
  assert.ok(Math.abs(roundTrip - 128_000) < 8_000)
})
