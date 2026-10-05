import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import type { MessageItem } from '../src/api/chat'
import type { AssistantReplyBatch } from '../src/utils/replyBatchBuilder'
import { applyEditedUserMessage, findLatestEditableUserMessageId } from '../src/utils/messageEditing'

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

test('chat edit flow waits for backend confirmation before local prune', () => {
  const chatStoreSource = readFileSync(resolve(import.meta.dirname, '../src/stores/chat.ts'), 'utf8')
  const sendEditedBody = chatStoreSource.match(/async function sendEditedMessage[\s\S]*?return true\n  \}/)?.[0] || ''
  const runtimeSource = readFileSync(resolve(import.meta.dirname, '../src/stores/chatSessionRuntime.ts'), 'utf8')
  const messageEditedHandler = runtimeSource.match(/onMessageEdited: \(data, sessionId\) => \{[\s\S]*?\n      \},\n      onChunk:/)?.[0] || ''

  assert.doesNotMatch(sendEditedBody, /applyEditedUserMessage/)
  assert.match(sendEditedBody, /pendingEditMessageId\.value = messageId/)
  assert.match(messageEditedHandler, /applyEditedUserMessage/)
})

test('message edit textarea uses Enter to send and Shift Enter to keep newline', () => {
  const messageItemSource = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ChatMessageItem.vue'), 'utf8')

  assert.match(messageItemSource, /function handleEditKeydown\(event: KeyboardEvent\)/)
  assert.match(messageItemSource, /event\.key !== 'Enter' \|\| event\.shiftKey/)
  assert.match(messageItemSource, /@keydown="handleEditKeydown"/)
})
