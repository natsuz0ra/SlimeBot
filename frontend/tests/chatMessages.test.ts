import assert from 'node:assert/strict'
import test from 'node:test'
import type { MessageItem } from '../src/api/chat'
import { materializeStoppedMessage, materializeStoppedMessages } from '../src/utils/chatMessages'

const baseMessage: MessageItem = {
  id: 'message-1',
  sessionId: 'session-1',
  role: 'assistant',
  content: '',
  createdAt: '2026-01-01T00:00:00Z',
}

test('materializeStoppedMessage fills empty stop placeholders', () => {
  const item = materializeStoppedMessage({ ...baseMessage, isStopPlaceholder: true }, 'Stopped')

  assert.equal(item.content, 'Stopped')
  assert.equal(item.isStopPlaceholder, true)
})

test('materializeStoppedMessages leaves regular content unchanged', () => {
  const items = materializeStoppedMessages([
    { ...baseMessage, content: 'hello', isStopPlaceholder: true },
    { ...baseMessage, id: 'message-2', content: '' },
  ], 'Stopped')

  assert.deepEqual(items.map((item) => item.content), ['hello', ''])
})
