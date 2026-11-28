import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import type { MessageItem } from '../src/api/chat'
import type { AssistantReplyBatch } from '../src/utils/replyBatchBuilder'
import { applyEditedUserMessage, findLatestEditableUserMessageId } from '../src/utils/messageEditing'
import { createEmptyChatRuntimeState, reduceChatRuntimeEvent, type ChatRuntimeReducerDeps } from '../src/stores/chatSessionReducer'

function message(id: string, role: MessageItem['role'], seq?: number): MessageItem {
  return {
    id,
    sessionId: 's1',
    role,
    content: id,
    seq,
    createdAt: new Date(0).toISOString(),
  }
}

test('findLatestEditableUserMessageId returns the latest persisted user message', () => {
  const messages = [
    message('u1', 'user', 1),
    message('a1', 'assistant', 2),
    message('u2', 'user', 3),
    message('a2', 'assistant', 4),
  ]

  assert.equal(findLatestEditableUserMessageId(messages, false, new Set()), 'u2')
})

test('findLatestEditableUserMessageId blocks while waiting and failed local messages', () => {
  const messages = [message('u1', 'user', 1), message('failed', 'user')]

  assert.equal(findLatestEditableUserMessageId(messages, true, new Set()), '')
  assert.equal(findLatestEditableUserMessageId(messages, false, new Set(['failed'])), '')
})

test('applyEditedUserMessage trims following messages and updates edited createdAt', () => {
  const messages = [
    message('u1', 'user', 1),
    message('a1', 'assistant', 2),
    message('u2', 'user', 3),
    message('a2', 'assistant', 4),
  ]
  const replyBatches = [
    { id: 'b1', sessionId: 's1', assistantMessageId: 'a1', toolCalls: [], timeline: [], collapsed: false },
    { id: 'b2', sessionId: 's1', assistantMessageId: 'a2', toolCalls: [], timeline: [], collapsed: false },
  ] as AssistantReplyBatch[]
  const editedAt = new Date(12345).toISOString()

  const result = applyEditedUserMessage(messages, replyBatches, 'u2', 'edited', editedAt)

  assert.deepEqual(result.messages.map((item) => item.id), ['u1', 'a1', 'u2'])
  assert.equal(result.messages[2].content, 'edited')
  assert.equal(result.messages[2].createdAt, editedAt)
  assert.deepEqual(result.replyBatches.map((item) => item.id), ['b1'])
})

test('message_edited reducer prunes only the acknowledged edit', () => {
  const replyBatches = [
    { id: 'b1', sessionId: 's1', assistantMessageId: 'a1', toolCalls: [], teamRuns: [], timeline: [], collapsed: false },
    { id: 'b2', sessionId: 's1', assistantMessageId: 'a2', toolCalls: [], teamRuns: [], timeline: [], collapsed: false },
  ] as AssistantReplyBatch[]
  const state = {
    ...createEmptyChatRuntimeState(),
    messages: [message('u1', 'user', 1), message('a1', 'assistant', 2), message('u2', 'user', 3), message('a2', 'assistant', 4)],
    replyBatches,
    pendingEditMessageId: 'u2',
    waiting: true,
  }
  const deps: ChatRuntimeReducerDeps = {
    createId: () => 'generated', now: () => 0, formatAssistantError: error => error, stoppedPlaceholder: () => 'stopped',
  }

  const unrelated = reduceChatRuntimeEvent(state, {
    type: 'message_edited', data: { messageId: 'u1', content: 'other edit' },
  }, deps)
  assert.equal(unrelated, state)
  assert.deepEqual(state.messages.map(item => item.id), ['u1', 'a1', 'u2', 'a2'])

  const confirmed = reduceChatRuntimeEvent(state, {
    type: 'message_edited', data: { messageId: 'u2', content: 'edited', createdAt: new Date(12345).toISOString() },
  }, deps)
  assert.deepEqual(confirmed.messages.map(item => item.id), ['u1', 'a1', 'u2'])
  assert.deepEqual(confirmed.replyBatches.map(item => item.id), ['b1'])
  assert.equal(confirmed.pendingEditMessageId, '')
})

test('message edit textarea uses Enter to send and Shift Enter to keep newline', () => {
  const messageItemSource = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ChatMessageItem.vue'), 'utf8')

  assert.match(messageItemSource, /function handleEditKeydown\(event: KeyboardEvent\)/)
  assert.match(messageItemSource, /event\.key !== 'Enter' \|\| event\.shiftKey/)
  assert.match(messageItemSource, /@keydown="handleEditKeydown"/)
})
