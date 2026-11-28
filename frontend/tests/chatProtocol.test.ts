import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import type { ChatWSDiagnostic } from '../../shared/chat-protocol/events.js'
import type { ChatDoneData, ThinkingEventData } from '../../shared/chat-protocol/dispatch.js'
import { dispatchChatSocketMessage as dispatch } from '../src/api/chatSocket'
import { buildReplyBatchesFromHistory } from '../src/utils/replyBatchBuilder'

const wire = JSON.parse(readFileSync(new URL('../../shared/chat-protocol/fixtures/ws-events.golden.json', import.meta.url), 'utf8'))

test('dispatches the shared WebSocket golden fixtures', () => {
  const thinking: ThinkingEventData[] = []
  const done: ChatDoneData[] = []
  const diagnostics: ChatWSDiagnostic[] = []
  for (const [name, event] of Object.entries(wire)) {
    const result = dispatch(JSON.stringify(event), {
      onStart: () => {},
      onThinkingStart: data => thinking.push(data),
      onThinkingChunk: data => thinking.push(data),
      onThinkingDone: data => thinking.push(data),
      onDone: (_sid, _answer, meta) => { if (meta) done.push(meta) },
      onProtocolError: diagnostic => diagnostics.push(diagnostic),
    })
    assert.equal(result, true, name)
  }
  assert.deepEqual(diagnostics, [])
  assert.equal(thinking.length, 3)
  assert.equal(thinking[0]?.startedAt, wire.thinking_start.startedAt)
  assert.equal(thinking[1]?.content, wire.thinking_chunk.content)
  assert.equal(thinking[1]?.startedAt, wire.thinking_chunk.startedAt)
  assert.equal(thinking[2]?.finishedAt, wire.thinking_done.finishedAt)
  assert.equal(thinking[2]?.parentToolCallId, wire.thinking_done.parentToolCallId)
  assert.deepEqual(done[0]?.skillLearning, wire.done.skillLearning)
  assert.equal(done[0]?.durationMs, wire.done.durationMs)
  assert.equal(done[0]?.planId, wire.done.planId)
  assert.equal(done[1]?.isInterrupted, true)
  assert.equal(done[2]?.planId, '')
})

test('reports explicit diagnostics for unknown and malformed WebSocket frames', () => {
  const diagnostics: ChatWSDiagnostic[] = []
  const handlers = {
    onStart: () => {},
    onDone: () => assert.fail('malformed events cannot dispatch callbacks'),
    onProtocolError: (diagnostic: ChatWSDiagnostic) => diagnostics.push(diagnostic),
  }
  for (const frame of ['{', 'null', '{"type":"future_event"}', '{"type":"chunk","content":19}', '{"type":"done","skillLearning":{}}']) {
    const result = dispatch(frame, handlers)
    assert.equal(result, false)
  }
  assert.deepEqual(diagnostics, [
    { code: 'invalid_json' },
    { code: 'invalid_message' },
    { code: 'unknown_type', eventType: 'future_event' },
    { code: 'invalid_field', eventType: 'chunk', field: 'content' },
    { code: 'invalid_field', eventType: 'done', field: 'skillLearning' },
  ])
})

const history = JSON.parse(readFileSync(new URL('../../shared/chat-protocol/fixtures/timeline.golden.json', import.meta.url), 'utf8'))
const message = { id: 'message-1', sessionId: 'session-1', role: 'assistant' as const, createdAt: '2026-04-29T01:02:03Z' }

for (const fixture of history) {
  test('projects shared history in Vue: ' + fixture.name, () => {
    const [batch] = buildReplyBatchesFromHistory(message.sessionId, {
      messages: [{ ...message, content: fixture.content, isInterrupted: fixture.interrupted }],
      toolCallsByAssistantMessageId: { [message.id]: fixture.toolCalls },
      thinkingByAssistantMessageId: { [message.id]: fixture.thinking },
    })
    assert.ok(batch)
    const entries = batch.timeline.flatMap((entry): object[] => {
      switch (entry.kind) {
        case 'tool_result':
        case 'notice':
          return []
        case 'tool_start': {
          const tool = batch.toolCalls.find(item => item.toolCallId === entry.toolCallId)
          assert.ok(tool)
          return [{
            kind: 'tool', toolCallId: tool.toolCallId, status: tool.status, error: tool.error,
            subagentThinkings: tool.subagentThinkings?.map(({ content, done, durationMs }) => ({ content, done, durationMs })),
          }]
        }
        case 'thinking':
          return [{ kind: 'thinking', content: entry.content, done: entry.done, durationMs: entry.durationMs }]
        default:
          return [{ kind: entry.kind, content: entry.content }]
      }
    })
    assert.deepEqual(JSON.parse(JSON.stringify(entries)), fixture.entries)
    assert.deepEqual(batch.toolCalls.filter(tool => tool.parentToolCallId).map(tool => tool.toolCallId), fixture.nestedToolIds ?? [])
    const results = fixture.entries
      .filter((entry: { kind: string; status?: string }) => entry.kind === 'tool' && ['rejected', 'completed', 'error'].includes(entry.status ?? ''))
      .map((entry: { toolCallId: string }) => entry.toolCallId)
    assert.deepEqual(batch.timeline.flatMap(entry => entry.kind === 'tool_result' ? [entry.toolCallId] : []), results)
  })
}
