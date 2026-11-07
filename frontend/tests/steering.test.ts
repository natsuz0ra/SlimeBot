import assert from 'node:assert/strict'
import test from 'node:test'
import { dispatchChatSocketMessage, type ChatSocketHandlers } from '../src/api/chatSocket'

test('steering acknowledgements preserve the current stream and expose the expected turn', () => {
  const starts: unknown[] = []
  const changes: unknown[] = []
  let completed = false
  const handlers: ChatSocketHandlers = {
    onSession: () => {}, onStart: (sid, meta) => starts.push([sid, meta]), onChunk: () => {}, onSessionTitle: () => {},
    onDone: () => { completed = true }, onError: () => { throw new Error('steering rejection must not stop the stream') },
    onSteering: (change, sid) => changes.push([sid, change]),
  }
  dispatchChatSocketMessage(JSON.stringify({type: 'start', sessionId: 's', turnId: 't'}), handlers)
  dispatchChatSocketMessage(JSON.stringify({type: 'steer_accepted', sessionId: 's', change: {submissionId: 'id', messageId: 'm', turnId: 't', content: 'change'}}), handlers)
  dispatchChatSocketMessage(JSON.stringify({type: 'steer_rejected', sessionId: 's', submissionId: 'late', error: 'stale turn'}), handlers)
  assert.equal((starts[0] as [string, { turnId: string }])[1].turnId, 't')
  assert.deepEqual(changes, [
    ['s', {submissionId: 'id', messageId: 'm', turnId: 't', content: 'change'}],
    ['s', {submissionId: 'late', error: 'stale turn'}],
  ])
  assert.equal(completed, false)
})
