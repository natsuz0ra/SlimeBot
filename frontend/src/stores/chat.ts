import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { ChatSocket, type ConnectionStatus } from '../api/chatSocket'
import type { MessageItem, SessionItem, ToolCallItem } from '../api'
import { sessionAPI } from '../api'

interface AssistantReplyBatch {
  id: string
  sessionId: string
  assistantMessageId: string
  toolCalls: ToolCallItem[]
  timeline: AssistantReplyTimelineItem[]
  collapsed: boolean
}

type AssistantReplyTimelineItem =
  | {
      id: string
      kind: 'tool_start'
      toolCallId: string
    }
  | {
      id: string
      kind: 'tool_result'
      toolCallId: string
    }
  | {
      id: string
      kind: 'text'
      content: string
    }

export const useChatStore = defineStore('chat', () => {
  const sessions = ref<SessionItem[]>([])
  const currentSessionId = ref<string>()
  const messages = ref<MessageItem[]>([])
  const waiting = ref(false)
  const streamingStarted = ref(false)
  const connectionStatus = ref<ConnectionStatus>('disconnected')
  const connectionError = ref('')
  const isSocketReady = computed(() => connectionStatus.value === 'connected')

  // 单次助手回复批次：工具调用和文本归并在同一次回复中展示
  const replyBatches = ref<AssistantReplyBatch[]>([])
  const currentBatchId = ref<string>('')

  const ws = new ChatSocket()

  function resetSessionRuntimeState() {
    replyBatches.value = []
    currentBatchId.value = ''
  }

  function getCurrentBatch() {
    if (!currentBatchId.value) return undefined
    return replyBatches.value.find((item) => item.id === currentBatchId.value)
  }

  async function loadSessions() {
    sessions.value = await sessionAPI.list()
    if (currentSessionId.value && !sessions.value.some((item) => item.id === currentSessionId.value)) {
      currentSessionId.value = undefined
      messages.value = []
      resetSessionRuntimeState()
    }
  }

  async function createSession() {
    const item = await sessionAPI.create()
    sessions.value.unshift(item)
    currentSessionId.value = item.id
    messages.value = []
    resetSessionRuntimeState()
  }

  async function selectSession(id: string) {
    currentSessionId.value = id
    messages.value = await sessionAPI.history(id)
    resetSessionRuntimeState()
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
        // 提前创建本次 assistant 占位消息，确保工具条目和文本固定在同一回复块
        const assistantMessageId = crypto.randomUUID()
        messages.value.push({
          id: assistantMessageId,
          sessionId: currentSessionId.value || '',
          role: 'assistant',
          content: '',
          createdAt: new Date().toISOString(),
        })
        const batchId = crypto.randomUUID()
        currentBatchId.value = batchId
        replyBatches.value.push({
          id: batchId,
          sessionId: sessionId,
          assistantMessageId,
          toolCalls: [],
          timeline: [],
          collapsed: false,
        })
      },
      onChunk: (chunk, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        const assistant = messages.value.find((msg) => msg.id === batch.assistantMessageId)
        if (!assistant) return
        assistant.content += chunk
        const lastTimeline = batch.timeline[batch.timeline.length - 1]
        if (lastTimeline && lastTimeline.kind === 'text') {
          lastTimeline.content += chunk
        } else {
          batch.timeline.push({
            id: crypto.randomUUID(),
            kind: 'text',
            content: chunk,
          })
        }
        streamingStarted.value = true
      },
      onSessionTitle: (title, sessionId) => {
        if (!sessionId || !title) return
        const item = sessions.value.find((session) => session.id === sessionId)
        if (!item) return
        item.name = title
      },
      onDone: async (sessionId, answer) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        waiting.value = false
        streamingStarted.value = false
        const batch = getCurrentBatch()
        if (batch) {
          if (typeof answer === 'string' && answer !== '') {
            const assistant = messages.value.find((msg) => msg.id === batch.assistantMessageId)
            if (assistant) {
              assistant.content = answer
            }
            const mergedTextEntry = {
              id: crypto.randomUUID(),
              kind: 'text' as const,
              content: answer,
            }
            const rebuiltTimeline: AssistantReplyTimelineItem[] = []
            let insertedText = false
            for (const entry of batch.timeline) {
              if (entry.kind === 'text') {
                if (!insertedText) {
                  rebuiltTimeline.push(mergedTextEntry)
                  insertedText = true
                }
                continue
              }
              rebuiltTimeline.push(entry)
            }
            if (!insertedText && answer.trim() !== '') {
              rebuiltTimeline.push(mergedTextEntry)
            }
            batch.timeline = rebuiltTimeline
          }
          batch.collapsed = true
        }
        currentBatchId.value = ''
        await loadSessions()
      },
      onError: (error, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        waiting.value = false
        streamingStarted.value = false
        currentBatchId.value = ''
        connectionError.value = error
      },
      onToolCallStart: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        batch.toolCalls.push({
          toolCallId: data.toolCallId,
          toolName: data.toolName,
          command: data.command,
          params: data.params,
          preamble: data.preamble,
          status: 'pending',
        })
        batch.timeline.push({
          id: crypto.randomUUID(),
          kind: 'tool_start',
          toolCallId: data.toolCallId,
        })
      },
      onToolCallResult: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        const item = batch.toolCalls.find((tc) => tc.toolCallId === data.toolCallId)
        if (item) {
          item.status = data.error ? 'error' : 'completed'
          item.output = data.output
          item.error = data.error
        }
        batch.timeline.push({
          id: crypto.randomUUID(),
          kind: 'tool_result',
          toolCallId: data.toolCallId,
        })
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

  function approveToolCall(toolCallId: string, approved: boolean) {
    const batch = replyBatches.value.find((group) => group.toolCalls.some((tc) => tc.toolCallId === toolCallId))
    const item = batch?.toolCalls.find((tc) => tc.toolCallId === toolCallId)
    if (item) {
      item.status = approved ? 'executing' : 'rejected'
    }
    ws.sendToolApproval(toolCallId, approved)
  }

  function disconnectSocket() {
    waiting.value = false
    streamingStarted.value = false
    ws.close()
    currentBatchId.value = ''
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
    replyBatches,
    currentBatchId,
    loadSessions,
    createSession,
    selectSession,
    connectSocket,
    ensureSessionReady,
    sendMessage,
    approveToolCall,
    disconnectSocket,
  }
})
