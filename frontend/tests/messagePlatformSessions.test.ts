import assert from 'node:assert/strict'
import test from 'node:test'
import { isMessagePlatformSessionId, listPlatformSessions, platformFromSessionId, platformSessionName } from '../src/utils/messagePlatformSessions'

test('legacy platform history belongs to Telegram and other platforms stay separate', () => {
  const sessions = [
    { id: 'im-platform-session:discord', name: 'discord', updatedAt: '' },
    { id: 'ordinary', name: 'ordinary', updatedAt: '' },
  ]
  assert.deepEqual(listPlatformSessions(sessions).map((item) => item.id), ['im-platform-session', 'im-platform-session:discord'])
  assert.equal(platformFromSessionId('im-platform-session'), 'telegram')
  assert.equal(platformSessionName('im-platform-session', 'Telegram'), 'Telegram')
  assert.equal(platformFromSessionId('im-platform-session:discord'), 'discord')
  assert.equal(isMessagePlatformSessionId('ordinary'), false)
})
