import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { ChatSocket, type ConnectionStatus } from '../api/chatSocket'
import type { MessageItem, SessionItem } from '../api'
import { sessionAPI } from '../api'

export const useChatStore = defineStore('chat', () => {
  const sessions = ref<SessionItem[]>([])
  const currentSessionId = ref<string>()
  const messages = ref<MessageItem[]>([])
  const waiting = ref(false)
  const streamingStarted = ref(false)
  const connectionStatus = ref<ConnectionStatus>('disconnected')
  const connectionError = ref('')
  const isSocketReady = computed(() => connectionStatus.value === 'connected')

  const ws = new ChatSocket()

  async function loadSessions() {
    sessions.value = await sessionAPI.list()
    if (currentSessionId.value && !sessions.value.some((item) => item.id === currentSessionId.value)) {
      currentSessionId.value = undefined
      messages.value = []
    }
  }

  async function createSession() {
    const item = await sessionAPI.create()
    sessions.value.unshift(item)
    currentSessionId.value = item.id
    messages.value = []
  }

  async function selectSession(id: string) {
    currentSessionId.value = id
    messages.value = await sessionAPI.history(id)
  }

  function connectSocket() {
    ws.connect({
      onSession: (id) => {
        if (!currentSessionId.value) {
          currentSessionId.value = id
        }
      },
      onStart: (sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        waiting.value = true
        streamingStarted.value = false
      },
      onChunk: (chunk, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const last = messages.value[messages.value.length - 1]
        if (last && last.role === 'assistant') {
          last.content += chunk
          streamingStarted.value = true
          return
        }
        messages.value.push({
          id: crypto.randomUUID(),
          sessionId: currentSessionId.value || '',
          role: 'assistant',
          content: chunk,
          createdAt: new Date().toISOString(),
        })
        streamingStarted.value = true
      },
      onSessionTitle: (title, sessionId) => {
        if (!sessionId || !title) return
        const item = sessions.value.find((session) => session.id === sessionId)
        if (!item) return
        item.name = title
      },
      onDone: async (sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        waiting.value = false
        streamingStarted.value = false
        await loadSessions()
      },
      onError: (error, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        waiting.value = false
        streamingStarted.value = false
        connectionError.value = error
      },
      onSocketError: (error) => {
        waiting.value = false
        streamingStarted.value = false
        connectionError.value = error
      },
      onClose: () => {
        waiting.value = false
        streamingStarted.value = false
      },
      onStatusChange: (status, error) => {
        connectionStatus.value = status
        if (error) {
          connectionError.value = error
        } else if (status === 'connected') {
          connectionError.value = ''
        }
      },
    })
  }

  async function ensureSessionReady() {
    if (currentSessionId.value) return true
    await createSession()
    return !!currentSessionId.value
  }

  async function sendMessage(content: string, modelId: string) {
    if (!modelId) {
      connectionError.value = 'modelId is required'
      return false
    }
    const ready = await ensureSessionReady()
    if (!ready || !currentSessionId.value || !isSocketReady.value) return false
    const sent = ws.send(content, currentSessionId.value, modelId)
    if (!sent) {
      connectionError.value = 'socket is not connected'
      return false
    }
    messages.value.push({
      id: crypto.randomUUID(),
      sessionId: currentSessionId.value,
      role: 'user',
      content,
      createdAt: new Date().toISOString(),
    })
    return true
  }

  function disconnectSocket() {
    waiting.value = false
    streamingStarted.value = false
    ws.close()
  }

  return {
    sessions,
    currentSessionId,
    messages,
    waiting,
    streamingStarted,
    connectionStatus,
    connectionError,
    isSocketReady,
    loadSessions,
    createSession,
    selectSession,
    connectSocket,
    ensureSessionReady,
    sendMessage,
    disconnectSocket,
  }
})
