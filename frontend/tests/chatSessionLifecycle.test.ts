import assert from 'node:assert/strict'
import { after, before, beforeEach, test } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { watch } from 'vue'
import { createServer, type ViteDevServer } from 'vite'

let server: ViteDevServer
let useChatStore: typeof import('../src/stores/chat').useChatStore
let sessionAPI: typeof import('../src/api/chat').sessionAPI
const histories = new Map<string, import('../src/api/chat').SessionHistoryPayload>()

class MockWebSocket {
  static OPEN = 1
  static instances: MockWebSocket[] = []
  readyState = 0
  closed = false
  sent: Record<string, unknown>[] = []
  onopen: (() => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  constructor(_url: string) { MockWebSocket.instances.push(this) }
  open() { this.readyState = 1; this.onopen?.() }
  send(data: string) { this.sent.push(JSON.parse(data)) }
  close() { this.closed = true; this.readyState = 3; this.onclose?.() }
  emit(type: string, sessionId: string, data: Record<string, unknown> = {}) {
    if (type === 'done') histories.set(sessionId, {
      messages: [
        { id: `user-db-${sessionId}`, sessionId, role: 'user', content: 'persisted user', seq: 1, createdAt: '' },
        { id: `assistant-db-${sessionId}`, sessionId, role: 'assistant', content: String(data.answer || ''), seq: 2, createdAt: '' },
      ], toolCallsByAssistantMessageId: {}, hasMore: false,
    })
    this.onmessage?.({ data: JSON.stringify({ type, sessionId, ...data }) })
  }
}

before(async () => {
  // Vite resolves the app's aliases and import.meta.env, so these tests exercise the actual store and socket dispatcher.
  server = await createServer({ server: { middlewareMode: true, hmr: false }, appType: 'custom' })
  ;({ useChatStore } = await server.ssrLoadModule('/src/stores/chat.ts'))
  ;({ sessionAPI } = await server.ssrLoadModule('/src/api/chat.ts'))
})
after(async () => { await server.close() })
beforeEach(() => {
  const storage = new Map<string, string>()
  storage.set('slimebot:auth:token', 'test-token')
  Object.assign(globalThis, {
    WebSocket: MockWebSocket,
    location: { protocol: 'http:', host: 'test.local' },
    document: { visibilityState: 'visible' },
    window: {
      slimebotDesktop: {},
      localStorage: { getItem: (key: string) => storage.get(key) || null, setItem: (key: string, value: string) => storage.set(key, value), removeItem: (key: string) => storage.delete(key) },
      setInterval: () => 1, clearInterval: () => {}, setTimeout: () => 2, clearTimeout: () => {},
    },
  })
  Object.assign(globalThis, { localStorage: window.localStorage })
  MockWebSocket.instances = []
  histories.clear()
  sessionAPI.get = async (id) => ({ id, name: id, updatedAt: '' })
  sessionAPI.list = async () => ({ sessions: ['a', 'b'].map(id => ({ id, name: id, updatedAt: '' })), hasMore: false })
  sessionAPI.history = async (id) => histories.get(id) || { messages: [], toolCallsByAssistantMessageId: {}, hasMore: false }
  sessionAPI.create = async () => ({ id: 'new', name: 'new', updatedAt: '' })
  setActivePinia(createPinia())
})

async function select(store: ReturnType<typeof useChatStore>, id: string) {
  try { await store.selectSession(id) } catch (error) { throw (error as Error).cause || error }
  const socket = MockWebSocket.instances.at(-1)!
  socket.open()
  return socket
}

test('background chunks and completion stay with their session; reading clears the unread dot', async () => {
  const store = useChatStore()
  store.setChatViewActive(true)
  const a = await select(store, 'a')
  assert.equal(await store.sendMessage('hello A', 'model'), true)
  a.emit('start', 'a')
  a.emit('chunk', 'a', { content: 'first ' })
  const b = await select(store, 'b')
  assert.equal(a.closed, false)
  assert.equal(store.waiting, false)
  assert.deepEqual([...store.runningSessionIds], ['a'])
  a.emit('chunk', 'a', { content: 'second' })
  assert.equal(store.messages.length, 0)
  a.emit('done', 'a', { answer: 'first second' })
  assert.equal(store.runningSessionIds.has('a'), false)
  assert.equal(store.unreadSessionIds.has('a'), true)
  assert.equal(a.closed, true)
  assert.equal(b.closed, false)
  await store.selectSession('a')
  MockWebSocket.instances.at(-1)!.open()
  assert.equal(store.messages.at(-1)?.content, 'first second')
  assert.equal(store.unreadSessionIds.has('a'), false)
  store.disconnectSocket()
})

test('parallel sessions have independent stop commands and reactive state', async () => {
  const store = useChatStore()
  store.setChatViewActive(true)
  const a = await select(store, 'a')
  await store.sendMessage('A', 'model')
  a.emit('start', 'a')
  const b = await select(store, 'b')
  await store.sendMessage('B', 'model')
  b.emit('start', 'b')
  assert.deepEqual([...store.runningSessionIds].sort(), ['a', 'b'])
  let activeMessageChanges = 0
  const unwatch = watch(() => store.messages.at(-1)?.content, () => { activeMessageChanges++ }, { flush: 'sync' })
  a.emit('chunk', 'a', { content: 'background answer' })
  assert.equal(activeMessageChanges, 0)
  assert.equal(store.stopCurrentResponse(), true)
  assert.equal(b.sent.at(-1)?.type, 'stop')
  assert.equal(b.sent.at(-1)?.sessionId, 'b')
  assert.equal(a.sent.some(item => item.type === 'stop'), false)
  b.emit('done', 'b', { answer: 'stopped', isInterrupted: true })
  assert.equal(store.unreadSessionIds.has('b'), false)
  assert.equal(store.runningSessionIds.has('a'), true)
  await store.selectSession('a')
  assert.equal(store.waiting, true)
  assert.equal(store.messages.at(-1)?.content, 'background answer')
  assert.equal(a.closed, false)
  unwatch()
  store.disconnectSocket()
})

test('background questions, approvals, todos and plans survive navigation', async () => {
  const store = useChatStore()
  store.setChatViewActive(true)
  const a = await select(store, 'a')
  await store.sendMessage('A', 'model')
  a.emit('start', 'a')
  await select(store, 'b')
  a.emit('tool_call_start', 'a', { toolCallId: 'question', toolName: 'ask_questions', params: { questions: JSON.stringify([{ id: 'q', question: 'Continue?', options: ['yes'] }]) }, requiresApproval: true })
  a.emit('tool_call_start', 'a', { toolCallId: 'shell', toolName: 'shell', requiresApproval: true })
  a.emit('todo_update', 'a', { items: [{ id: 'todo', content: 'work', status: 'in_progress' }], note: 'note' })
  await store.selectSession('a')
  assert.equal(store.pendingQuestions?.toolCallId, 'question')
  assert.equal(store.runtimeTodos.length, 1)
  assert.deepEqual(store.pendingApprovalToolCallIds, ['shell'])
  store.submitQuestionAnswers('question', 'yes')
  assert.equal(a.sent.at(-1)?.type, 'tool_approve')
  assert.equal(a.sent.at(-1)?.answers, 'yes')
  await store.selectSession('b')
  a.emit('done', 'a', { answer: 'plan', planId: 'plan-a', planBody: '# Plan A' })
  await store.selectSession('a')
  MockWebSocket.instances.at(-1)!.open()
  assert.equal(store.pendingPlanConfirmation?.planId, 'plan-a')
  store.approvePlan('model', 'execute')
  assert.equal(store.waiting, true)
  assert.equal(MockWebSocket.instances.at(-1)!.sent.at(-1)?.type, 'plan_approve')
  store.disconnectSocket()
})

test('opening a new draft and navigating away retain the running channel', async () => {
  const store = useChatStore()
  store.setChatViewActive(true)
  const a = await select(store, 'a')
  await store.sendMessage('A', 'model')
  a.emit('start', 'a')
  store.resetToNewSession()
  assert.equal(a.closed, false)
  assert.equal(store.waiting, false)
  assert.equal(store.currentSessionId, undefined)
  await store.selectSession('a')
  store.setChatViewActive(false)
  a.emit('done', 'a', { answer: 'done while viewing tasks' })
  assert.equal(store.unreadSessionIds.has('a'), true)
  store.setChatViewActive(true)
  assert.equal(store.unreadSessionIds.has('a'), false)
  store.disconnectSocket()
})

test('background errors become unread and cannot clear another session’s running state', async () => {
  const store = useChatStore()
  const a = await select(store, 'a')
  await store.sendMessage('A', 'model')
  a.emit('start', 'a')
  const b = await select(store, 'b')
  await store.sendMessage('B', 'model')
  b.emit('start', 'b')
  a.emit('error', 'a', { error: 'provider failed' })
  assert.equal(store.waiting, true)
  assert.equal(store.runningSessionIds.has('a'), false)
  assert.equal(store.unreadSessionIds.has('a'), true)
  await store.selectSession('a')
  assert.match(store.messages.at(-1)!.content, /provider failed/)
  assert.equal(store.isAssistantErrorMessage(store.messages.at(-1)!.id), true)
  store.disconnectSocket()
})

test('a send with attachments stays bound to its session while upload awaits', async () => {
  const store = useChatStore()
  const a = await select(store, 'a')
  let finishUpload!: (response: { items: [] }) => void
  sessionAPI.uploadAttachments = () => new Promise(resolve => { finishUpload = resolve })
  const sending = store.sendMessage('file for A', 'model', [{} as File])
  await new Promise(resolve => setImmediate(resolve))
  await select(store, 'b')
  finishUpload({ items: [] })
  assert.equal(await sending, true)
  assert.equal(a.sent.at(-1)?.sessionId, 'a')
  assert.equal(store.messages.length, 0)
  await store.selectSession('a')
  assert.equal(store.messages.at(-1)?.content, 'file for A')
  store.disconnectSocket()
})

test('a newly created session adopts the already connected draft channel', async () => {
  const store = useChatStore()
  store.connectSocket()
  const draft = MockWebSocket.instances.at(-1)!
  draft.open()
  assert.equal(await store.sendMessage('first turn', 'model'), true)
  assert.equal(store.currentSessionId, 'new')
  assert.equal(MockWebSocket.instances.length, 1)
  assert.equal(draft.closed, false)
  assert.equal(draft.sent.at(-1)?.sessionId, 'new')
  draft.emit('start', 'new')
  assert.equal(store.runningSessionIds.has('new'), true)
  store.disconnectSocket()
})

test('navigation during creation cannot send a draft’s message to a different session', async () => {
  const store = useChatStore()
  store.connectSocket()
  MockWebSocket.instances.at(-1)!.open()
  let finishCreation!: (item: import('../src/api/chat').SessionItem) => void
  sessionAPI.create = () => new Promise(resolve => { finishCreation = resolve })
  const sending = store.sendMessage('draft message', 'model')
  await new Promise(resolve => setImmediate(resolve))
  const b = await select(store, 'b')
  finishCreation({ id: 'new', name: 'new', updatedAt: '' })
  assert.equal(await sending, false)
  assert.equal(store.currentSessionId, 'b')
  assert.equal(b.sent.length, 0)
  store.disconnectSocket()
})

test('completion loads persisted message IDs when returning, so editing remains available', async () => {
  const store = useChatStore()
  store.setChatViewActive(true)
  const a = await select(store, 'a')
  await store.sendMessage('A', 'model')
  a.emit('start', 'a')
  await select(store, 'b')
  a.emit('done', 'a', { answer: 'saved answer' })
  await select(store, 'a')
  assert.equal(store.messages.at(-1)?.id, 'assistant-db-a')
  assert.equal(store.latestEditableUserMessageId, 'user-db-a')
  assert.equal(await store.sendEditedMessage('user-db-a', 'edited A', 'model'), true)
  assert.equal(MockWebSocket.instances.at(-1)?.sent.at(-1)?.sessionId, 'a')
  store.disconnectSocket()
})

test('completion while the browser is hidden remains unread until it becomes visible', async () => {
  const store = useChatStore()
  store.setChatViewActive(true)
  const a = await select(store, 'a')
  await store.sendMessage('A', 'model')
  a.emit('start', 'a')
  Object.assign(document, { visibilityState: 'hidden' })
  a.emit('done', 'a', { answer: 'hidden result' })
  assert.equal(store.unreadSessionIds.has('a'), true)
  store.markSessionRead('a')
  assert.equal(store.unreadSessionIds.has('a'), true)
  Object.assign(document, { visibilityState: 'visible' })
  store.markSessionRead('a')
  assert.equal(store.unreadSessionIds.has('a'), false)
  store.disconnectSocket()
})

test('repeat sends are blocked before the server acknowledges start', async () => {
  const store = useChatStore()
  const a = await select(store, 'a')
  assert.equal(await store.sendMessage('first', 'model'), true)
  assert.equal(await store.sendMessage('second', 'model'), false)
  assert.equal(a.sent.filter(item => item.type === 'chat').length, 1)
  store.disconnectSocket()
})

test('logout closes all channels and opening a clean draft does not reconnect', async () => {
  const store = useChatStore()
  const a = await select(store, 'a')
  await store.sendMessage('A', 'model')
  a.emit('start', 'a')
  const b = await select(store, 'b')
  await store.sendMessage('B', 'model')
  b.emit('start', 'b')
  store.disconnectSocket({ silentConnectionNotice: true })
  const count = MockWebSocket.instances.length
  store.resetToNewSession()
  assert.equal(a.closed, true)
  assert.equal(b.closed, true)
  assert.equal(MockWebSocket.instances.length, count)
  assert.equal(store.runningSessionIds.size, 0)
  assert.equal(store.unreadSessionIds.size, 0)
})
