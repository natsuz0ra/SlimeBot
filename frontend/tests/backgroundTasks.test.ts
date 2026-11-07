import assert from 'node:assert/strict'
import test from 'node:test'
import { dispatchChatSocketMessage, type ChatSocketHandlers } from '../src/api/chatSocket'

test('background task snapshots and wake start are routed without fabricating user input', () => {
  const events: unknown[] = []
  const handlers: ChatSocketHandlers = {
    onSession: () => {}, onStart: (sid, meta) => events.push(['start', sid, meta?.background, meta?.turnId]),
    onChunk: () => {}, onSessionTitle: () => {}, onDone: () => {}, onError: () => {},
    onBackgroundTasks: (tasks, sid) => events.push(['tasks', sid, tasks]),
    onBackgroundTask: (task, sid) => events.push(['task', sid, task]),
  }
  dispatchChatSocketMessage(JSON.stringify({type: 'background_tasks', sessionId: 's', tasks: [{taskId: 't:p', processId: 'p', status: 'queued'}]}), handlers)
  dispatchChatSocketMessage(JSON.stringify({type: 'start', sessionId: 's', turnId: 'wake', background: true}), handlers)
  dispatchChatSocketMessage(JSON.stringify({type: 'background_task', sessionId: 's', taskId: 't:p', processId: 'p', status: 'finished'}), handlers)
  assert.deepEqual(events, [
    ['tasks', 's', [{taskId: 't:p', processId: 'p', status: 'queued'}]],
    ['start', 's', true, 'wake'],
    ['task', 's', {taskId: 't:p', processId: 'p', status: 'finished'}],
  ])
})
