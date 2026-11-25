import assert from 'node:assert/strict'
import test from 'node:test'
import { dispatchChatSocketMessage, type SkillLearningData } from '../src/api/chatSocket'

test('done forwards Skill learning outcome with the owning session', () => {
  let received: { sessionId?: string; notice?: SkillLearningData } | undefined
  dispatchChatSocketMessage(JSON.stringify({
    type: 'done',
    sessionId: 'session-a',
    answer: 'main answer',
    skillLearning: { skillId: 'repeatable-fix', status: 'completed', message: 'saved' },
  }), {
    onSession: () => {}, onStart: () => {}, onChunk: () => {}, onSessionTitle: () => {}, onError: () => {},
    onDone: (sessionId, _answer, meta) => { received = { sessionId, notice: meta?.skillLearning } },
  })
  assert.deepEqual(received, {
    sessionId: 'session-a',
    notice: { skillId: 'repeatable-fix', status: 'completed', message: 'saved' },
  })
})
