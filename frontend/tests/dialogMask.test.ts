import assert from 'node:assert/strict'
import test from 'node:test'
import { shouldCloseOnMaskInteraction } from '../src/utils/dialogMask'

test('mask interaction does not close when pointer starts inside dialog and ends on mask', () => {
  const mask = {}
  const panel = {}

  assert.equal(
    shouldCloseOnMaskInteraction({
      closeOnMask: true,
      pointerDownStartedOnMask: false,
      eventTarget: mask,
      eventCurrentTarget: mask,
    }),
    false,
  )

  assert.equal(
    shouldCloseOnMaskInteraction({
      closeOnMask: true,
      pointerDownStartedOnMask: false,
      eventTarget: panel,
      eventCurrentTarget: mask,
    }),
    false,
  )
})

test('mask interaction closes only when pointer starts and ends on mask', () => {
  const mask = {}

  assert.equal(
    shouldCloseOnMaskInteraction({
      closeOnMask: true,
      pointerDownStartedOnMask: true,
      eventTarget: mask,
      eventCurrentTarget: mask,
    }),
    true,
  )
})

test('mask interaction respects closeOnMask=false', () => {
  const mask = {}

  assert.equal(
    shouldCloseOnMaskInteraction({
      closeOnMask: false,
      pointerDownStartedOnMask: true,
      eventTarget: mask,
      eventCurrentTarget: mask,
    }),
    false,
  )
})
