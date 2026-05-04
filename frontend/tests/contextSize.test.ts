import assert from 'node:assert/strict'
import test from 'node:test'
import {
  CONTEXT_SIZE_DEFAULT,
  CONTEXT_SIZE_MAX,
  CONTEXT_SIZE_MIN,
  clampContextSize,
  contextSizeToSlider,
  contextUsageTone,
  formatContextSize,
  formatContextTokenCount,
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

test('formatContextTokenCount renders K and M units', () => {
  assert.equal(formatContextTokenCount(987), '987')
  assert.equal(formatContextTokenCount(23_700), '23.7K')
  assert.equal(formatContextTokenCount(1_240_000), '1.2M')
})

test('contextUsageTone maps usage percent to visual states', () => {
  assert.equal(contextUsageTone(69), 'normal')
  assert.equal(contextUsageTone(70), 'warning')
  assert.equal(contextUsageTone(89), 'warning')
  assert.equal(contextUsageTone(90), 'danger')
})

test('context size slider round trips through logarithmic mapping', () => {
  assert.equal(contextSizeToSlider(CONTEXT_SIZE_MIN), 0)
  assert.equal(contextSizeToSlider(CONTEXT_SIZE_MAX), 100)

  const roundTrip = sliderToContextSize(contextSizeToSlider(128_000))
  assert.ok(Math.abs(roundTrip - 128_000) < 8_000)
})
