import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { ChatSocket, type ConnectionStatus } from '@/api/chatSocket'
import { MESSAGE_PLATFORM_SESSION_ID, sessionAPI } from '@/api/chat'
import type {
  MessageAttachmentItem,
  MessageItem,
  SessionHistoryPayload,
  SessionHistoryToolCallItem,
  SessionItem,
  ToolCallItem,
  ToolCallStatus,
  UploadedAttachmentItem,
} from '@/api/chat'
import { i18n } from '@/i18n'

const HISTORY_PAGE_SIZE = 10

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
  const hasMoreHistory = ref(false)
  const loadingOlderHistory = ref(false)
  const loadingNewerMessages = ref(false)
  const connectionStatus = ref<ConnectionStatus>('disconnected')
  const connectionError = ref('')
  const suppressNextConnectionNotice = ref(false)
  const isSocketReady = computed(() => connectionStatus.value === 'connected')

  const replyBatches = ref<AssistantReplyBatch[]>([])
  const currentBatchId = ref<string>('')
  const assistantErrorIds = ref(new Set<string>())
  const failedUserMessageIds = ref(new Set<string>())

  const ws = new ChatSocket()

  function resetSessionRuntimeState() {
    replyBatches.value = []
    currentBatchId.value = ''
    assistantErrorIds.value.clear()
    failedUserMessageIds.value.clear()
  }

  function resetHistoryState() {
    hasMoreHistory.value = false
    loadingOlderHistory.value = false
    loadingNewerMessages.value = false
  }

  function getStoppedPlaceholderText() {
    return i18n.global.t('assistantStopped') as string
  }

  function materializeMessage(item: MessageItem): MessageItem {
    if (item.isStopPlaceholder && (!item.content || item.content.trim() === '')) {
      return { ...item, content: getStoppedPlaceholderText() }
    }
    return item
  }

  function materializeMessages(items: MessageItem[]): MessageItem[] {
    return items.map((item) => materializeMessage(item))
  }

  function normalizeToolStatus(status?: string, fallbackError?: string): ToolCallStatus {
    if (status === 'pending' || status === 'rejected' || status === 'executing' || status === 'completed' || status === 'error') {
      return status
    }
    return fallbackError ? 'error' : 'completed'
  }

  function buildReplyBatchesFromHistory(sessionId: string, history: SessionHistoryPayload): AssistantReplyBatch[] {
    const nextBatches: AssistantReplyBatch[] = []
    for (const message of history.messages) {
      if (message.role !== 'assistant') continue
      const historyToolCalls = history.toolCallsByAssistantMessageId[message.id] || []
      if (historyToolCalls.length === 0) continue

      const sortedCalls = [...historyToolCalls].sort((left, right) => {
        const leftAt = new Date(left.startedAt || 0).getTime()
        const rightAt = new Date(right.startedAt || 0).getTime()
        return leftAt - rightAt
      })

      const toolCalls: ToolCallItem[] = sortedCalls.map((item: SessionHistoryToolCallItem) => ({
        toolCallId: item.toolCallId,
        toolName: item.toolName,
        command: item.command,
        params: item.params || {},
        requiresApproval: !!item.requiresApproval,
        status: normalizeToolStatus(item.status, item.error),
        output: item.output,
        error: item.error,
      }))

      const timeline: AssistantReplyTimelineItem[] = []
      for (const item of toolCalls) {
        timeline.push({
          id: crypto.randomUUID(),
          kind: 'tool_start',
          toolCallId: item.toolCallId,
        })
        if (item.status !== 'pending' && item.status !== 'executing') {
          timeline.push({
            id: crypto.randomUUID(),
            kind: 'tool_result',
            toolCallId: item.toolCallId,
          })
        }
      }
      if (message.content.trim() !== '') {
        timeline.push({
          id: crypto.randomUUID(),
          kind: 'text',
          content: message.content,
        })
      }

      nextBatches.push({
        id: crypto.randomUUID(),
        sessionId: sessionId,
        assistantMessageId: message.id,
        toolCalls,
        timeline,
        collapsed: true,
      })
    }
    return nextBatches
  }

  function rebuildReplyBatchesFromHistory(sessionId: string, history: SessionHistoryPayload) {
    replyBatches.value = buildReplyBatchesFromHistory(sessionId, history)
    currentBatchId.value = ''
  }

  function mergeReplyBatchesFromHistory(sessionId: string, history: SessionHistoryPayload, position: 'prepend' | 'append') {
    const incoming = buildReplyBatchesFromHistory(sessionId, history)
    if (incoming.length === 0) return
    const existingAssistantIDs = new Set(replyBatches.value.map((item) => item.assistantMessageId))
    const filtered = incoming.filter((item) => !existingAssistantIDs.has(item.assistantMessageId))
    if (filtered.length === 0) return
    replyBatches.value = position === 'prepend' ? [...filtered, ...replyBatches.value] : [...replyBatches.value, ...filtered]
  }

  function getCurrentBatch() {
    if (!currentBatchId.value) return undefined
    return replyBatches.value.find((item) => item.id === currentBatchId.value)
  }

  function formatAssistantError(rawError: string) {
    const safeError = rawError?.trim() || 'unknown error'
    return i18n.global.t('assistantReplyFailed', { error: safeError }) as string
  }

  function markAssistantError(messageId: string) {
    assistantErrorIds.value.add(messageId)
  }

  function clearAssistantError(messageId: string) {
    assistantErrorIds.value.delete(messageId)
  }

  function isAssistantErrorMessage(messageId: string) {
    return assistantErrorIds.value.has(messageId)
  }

  function markFailedUserMessage(messageId: string) {
    failedUserMessageIds.value.add(messageId)
  }

  function isFailedUserMessage(messageId: string) {
    return failedUserMessageIds.value.has(messageId)
  }

  function pushFailedUserMessage(content: string) {
    const sessionId = currentSessionId.value
    if (!sessionId) return
    const messageId = crypto.randomUUID()
    messages.value.push({
      id: messageId,
      sessionId,
      role: 'user',
      content,
      createdAt: new Date().toISOString(),
    })
    markFailedUserMessage(messageId)
  }

  function finalizeAssistantError(rawError: string, sessionId?: string) {
    const targetSessionId = sessionId || currentSessionId.value
    if (!targetSessionId || targetSessionId !== currentSessionId.value) return
    const errorMessage = formatAssistantError(rawError)
    const batch = getCurrentBatch()
    if (batch) {
      const assistant = messages.value.find((msg) => msg.id === batch.assistantMessageId)
      if (assistant) {
        assistant.content = errorMessage
        markAssistantError(assistant.id)
      }
      const textEntry: AssistantReplyTimelineItem = {
        id: crypto.randomUUID(),
        kind: 'text',
        content: errorMessage,
      }
      const rebuiltTimeline: AssistantReplyTimelineItem[] = []
      let inserted = false
      for (const entry of batch.timeline) {
        if (entry.kind === 'text') {
          if (!inserted) {
            rebuiltTimeline.push(textEntry)
            inserted = true
          }
          continue
        }
        rebuiltTimeline.push(entry)
      }
      if (!inserted) {
        rebuiltTimeline.push(textEntry)
      }
      batch.timeline = rebuiltTimeline
      batch.collapsed = true
      currentBatchId.value = ''
      return
    }

    const assistantMessageId = crypto.randomUUID()
    messages.value.push({
      id: assistantMessageId,
      sessionId: targetSessionId,
      role: 'assistant',
      content: errorMessage,
      createdAt: new Date().toISOString(),
    })
    markAssistantError(assistantMessageId)
  }

  async function loadSessions() {
    sessions.value = await sessionAPI.list()
    const isVirtualMessagePlatformSession =
      currentSessionId.value === MESSAGE_PLATFORM_SESSION_ID &&
      !sessions.value.some((item) => item.id === MESSAGE_PLATFORM_SESSION_ID)
    if (isVirtualMessagePlatformSession) return
    if (currentSessionId.value && !sessions.value.some((item) => item.id === currentSessionId.value)) {
      currentSessionId.value = undefined
      messages.value = []
      resetSessionRuntimeState()
      resetHistoryState()
    }
  }

  function appendUniqueMessages(items: MessageItem[]) {
    if (items.length === 0) return
    const existingIDs = new Set(messages.value.map((item) => item.id))
    const next = items.filter((item) => !existingIDs.has(item.id))
    if (next.length > 0) {
      messages.value = [...messages.value, ...next]
    }
  }

  function prependUniqueMessages(items: MessageItem[]) {
    if (items.length === 0) return
    const existingIDs = new Set(messages.value.map((item) => item.id))
    const next = items.filter((item) => !existingIDs.has(item.id))
    if (next.length > 0) {
      messages.value = [...next, ...messages.value]
    }
  }

  function resetToNewSession() {
    currentSessionId.value = undefined
    messages.value = []
    resetSessionRuntimeState()
    resetHistoryState()
  }

  async function createSession() {
    const item = await sessionAPI.create()
    currentSessionId.value = item.id
    sessions.value = [item, ...sessions.value]
    messages.value = []
    resetSessionRuntimeState()
    resetHistoryState()
  }

  async function selectSession(id: string) {
    try {
      const history = await sessionAPI.history(id, { limit: HISTORY_PAGE_SIZE })
      currentSessionId.value = id
      messages.value = materializeMessages(history.messages)
      resetHistoryState()
      hasMoreHistory.value = history.hasMore
      resetSessionRuntimeState()
      rebuildReplyBatchesFromHistory(id, history)
    } catch {
      // 固定消息平台会话在首条平台消息前可能尚未落库，前端先展示只读空态。
      if (id === MESSAGE_PLATFORM_SESSION_ID) {
        currentSessionId.value = id
        messages.value = []
        resetSessionRuntimeState()
        resetHistoryState()
        return
      }
      throw new Error('load session history failed')
    }
  }

  async function loadOlderMessages() {
    const sessionId = currentSessionId.value
    const first = messages.value[0]
    if (!sessionId || !first || !hasMoreHistory.value || loadingOlderHistory.value) return false
    loadingOlderHistory.value = true
    try {
      const history = await sessionAPI.history(sessionId, {
        limit: HISTORY_PAGE_SIZE,
        before: first.createdAt,
      })
      prependUniqueMessages(materializeMessages(history.messages))
      hasMoreHistory.value = history.hasMore
      mergeReplyBatchesFromHistory(sessionId, history, 'prepend')
      return history.messages.length > 0
    } finally {
      loadingOlderHistory.value = false
    }
  }

  async function loadNewMessagesForSession(sessionId: string) {
    const activeSessionID = currentSessionId.value
    if (!activeSessionID || activeSessionID !== sessionId || loadingNewerMessages.value) return false
    loadingNewerMessages.value = true
    try {
      const latest = messages.value[messages.value.length - 1]
      const history = await sessionAPI.history(sessionId, {
        limit: 50,
        after: latest?.createdAt,
      })
      appendUniqueMessages(materializeMessages(history.messages))
      mergeReplyBatchesFromHistory(sessionId, history, 'append')
      return history.messages.length > 0
    } finally {
      loadingNewerMessages.value = false
    }
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
        const assistantMessageId = crypto.randomUUID()
        messages.value.push({
          id: assistantMessageId,
          sessionId: currentSessionId.value || '',
          role: 'assistant',
          content: '',
          createdAt: new Date().toISOString(),
        })
        clearAssistantError(assistantMessageId)
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
      onDone: async (sessionId, answer, meta) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        waiting.value = false
        streamingStarted.value = false
        const batch = getCurrentBatch()
        if (batch) {
          const assistant = messages.value.find((msg) => msg.id === batch.assistantMessageId)
          if (assistant) {
            assistant.isInterrupted = !!meta?.isInterrupted
            assistant.isStopPlaceholder = !!meta?.isStopPlaceholder
          }
          const finalAnswer =
            typeof answer === 'string' && answer !== ''
              ? answer
              : (meta?.isStopPlaceholder ? getStoppedPlaceholderText() : '')
          if (finalAnswer !== '') {
            if (assistant) {
              assistant.content = finalAnswer
              clearAssistantError(assistant.id)
            }
            const mergedTextEntry = {
              id: crypto.randomUUID(),
              kind: 'text' as const,
              content: finalAnswer,
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
            if (!insertedText && finalAnswer.trim() !== '') {
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
        connectionError.value = error
        finalizeAssistantError(error, sessionId)
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
          requiresApproval: data.requiresApproval,
          status: data.requiresApproval ? 'pending' : 'executing',
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
          item.status = normalizeToolStatus(data.status, data.error)
          item.output = data.output
          item.error = data.error
          item.requiresApproval = data.requiresApproval
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

  function toMessageAttachments(items: UploadedAttachmentItem[]): MessageAttachmentItem[] {
    return items.map((item) => ({
      id: item.id,
      name: item.name,
      ext: item.ext,
      sizeBytes: item.sizeBytes,
      mimeType: item.mimeType,
      category: item.category,
      iconType: item.iconType,
    }))
  }

  async function uploadAttachmentsForCurrentSession(files: File[]) {
    if (files.length === 0 || !currentSessionId.value) {
      return [] as UploadedAttachmentItem[]
    }
    const response = await sessionAPI.uploadAttachments(currentSessionId.value, files)
    return response.items || []
  }

  async function sendMessage(content: string, modelId: string, files: File[] = []) {
    const trimmed = content.trim()
    if (!trimmed && files.length === 0) {
      return false
    }
    if (!modelId) {
      const error = 'modelId is required'
      connectionError.value = error
      pushFailedUserMessage(trimmed)
      return false
    }
    const ready = await ensureSessionReady()
    if (!ready || !currentSessionId.value) return false
    if (!isSocketReady.value) {
      const error = 'socket is not connected'
      connectionError.value = error
      pushFailedUserMessage(trimmed)
      return false
    }
    let uploaded: UploadedAttachmentItem[] = []
    if (files.length > 0) {
      uploaded = await uploadAttachmentsForCurrentSession(files)
    }
    const sent = ws.send(trimmed, currentSessionId.value, modelId, uploaded.map((item) => item.id))
    if (!sent) {
      const error = 'socket is not connected'
      connectionError.value = error
      pushFailedUserMessage(trimmed)
      return false
    }
    messages.value.push({
      id: crypto.randomUUID(),
      sessionId: currentSessionId.value,
      role: 'user',
      content: trimmed,
      attachments: toMessageAttachments(uploaded),
      createdAt: new Date().toISOString(),
    })
    return true
  }

  function stopCurrentResponse() {
    const sessionId = currentSessionId.value
    if (!sessionId || !waiting.value) return false
    return ws.sendStop(sessionId)
  }

  function approveToolCall(toolCallId: string, approved: boolean) {
    const batch = replyBatches.value.find((group) => group.toolCalls.some((tc) => tc.toolCallId === toolCallId))
    const item = batch?.toolCalls.find((tc) => tc.toolCallId === toolCallId)
    if (item) {
      item.status = approved ? 'executing' : 'rejected'
    }
    ws.sendToolApproval(toolCallId, approved)
  }

  function disconnectSocket(options?: { silentConnectionNotice?: boolean }) {
    if (options?.silentConnectionNotice) {
      markSuppressNextConnectionNotice()
    }
    waiting.value = false
    streamingStarted.value = false
    ws.close()
    currentBatchId.value = ''
  }

  function markSuppressNextConnectionNotice() {
    suppressNextConnectionNotice.value = true
  }

  function consumeSuppressNextConnectionNotice() {
    const shouldSuppress = suppressNextConnectionNotice.value
    suppressNextConnectionNotice.value = false
    return shouldSuppress
  }

  return {
    sessions,
    currentSessionId,
    messages,
    waiting,
    streamingStarted,
    hasMoreHistory,
    loadingOlderHistory,
    connectionStatus,
    isSocketReady,
    isAssistantErrorMessage,
    isFailedUserMessage,
    replyBatches,
    currentBatchId,
    loadSessions,
    resetToNewSession,
    createSession,
    selectSession,
    loadOlderMessages,
    loadNewMessagesForSession,
    connectSocket,
    ensureSessionReady,
    sendMessage,
    stopCurrentResponse,
    approveToolCall,
    disconnectSocket,
    consumeSuppressNextConnectionNotice,
  }
})
