import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { ChatSocket, type ConnectionStatus, type RuntimeTodoItem, type TodoUpdateData } from '@/api/chatSocket'
import { MESSAGE_PLATFORM_SESSION_ID, sessionAPI } from '@/api/chat'
import type { MessageAttachmentItem, MessageItem, SessionHistoryPayload, SessionHistoryThinkingItem, SessionItem, UploadedAttachmentItem } from '@/api/chat'
import { i18n } from '@/i18n'
import {
  buildInterleavedTimeline,
  buildLegacyTimeline,
  buildReplyBatchesFromHistory,
  normalizeToolStatus,
  type AssistantReplyBatch,
  type AssistantReplyTimelineItem,
} from '@/utils/replyBatchBuilder'
import { hasContentMarkers, parseContentMarkers, stripContentMarkers } from '@/utils/contentMarkers'
import { appendPlanBodyToBatch, appendPlanChunkToBatch, appendSubagentThinkingChunk, appendTextChunkToBatch, finalizeOpenReplyRuntimeState, finalizeReplyBatchTiming, finishOpenThinkingEntries, finishSubagentThinking, markLastThinkingDone, markToolCallError, startSubagentThinking } from '@/utils/liveReplyTimeline'
import { getBatchApprovalToolCallIds, markToolApprovalDecision } from '@/utils/toolApprovals'
import { materializeStoppedMessages } from '@/utils/chatMessages'

const HISTORY_PAGE_SIZE = 10
const MAX_SESSION_PAGE_SIZE = 100

export const useChatStore = defineStore('chat', () => {
  const sessions = ref<SessionItem[]>([])
  const sessionPageSize = ref(30)
  const hasMoreSessions = ref(true)
  const loadingMoreSessions = ref(false)
  const sessionSearchQuery = ref('')
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
  const planMode = ref(false)
  const planGenerating = ref(false)
  const isSocketReady = computed(() => connectionStatus.value === 'connected')
  const runtimeTodos = ref<RuntimeTodoItem[]>([])
  const runtimeTodoNote = ref('')
  const runtimeTodoUpdatedAt = ref<number>()
  const todoPanelOpen = ref(false)

  const replyBatches = ref<AssistantReplyBatch[]>([])
  const currentBatchId = ref<string>('')
  const assistantErrorIds = ref(new Set<string>())
  const failedUserMessageIds = ref(new Set<string>())
  const pendingPlanConfirmation = ref<{ sessionId: string; planId: string; content: string } | null>(null)
  const pendingApprovalToolCallIds = computed(() => replyBatches.value.flatMap((batch) => getBatchApprovalToolCallIds(batch.toolCalls)))

  interface QuestionItem {
    id: string
    question: string
    options: string[]
    option_descriptions?: string[]
  }
  const pendingQuestions = ref<{ toolCallId: string; questions: QuestionItem[] } | null>(null)

  const ws = new ChatSocket()

  function resetSessionRuntimeState() {
    replyBatches.value = []
    currentBatchId.value = ''
    assistantErrorIds.value.clear()
    failedUserMessageIds.value.clear()
    pendingQuestions.value = null
    clearRuntimeTodos()
  }

  function clearRuntimeTodos() {
    runtimeTodos.value = []
    runtimeTodoNote.value = ''
    runtimeTodoUpdatedAt.value = undefined
    todoPanelOpen.value = false
  }

  function applyRuntimeTodoUpdate(update: TodoUpdateData, sessionId?: string) {
    if (!sessionId || sessionId !== currentSessionId.value) return
    runtimeTodos.value = update.items.map((item) => ({ ...item }))
    runtimeTodoNote.value = update.note || ''
    runtimeTodoUpdatedAt.value = parseSocketTimestamp(update.updatedAt)
    todoPanelOpen.value = runtimeTodos.value.length > 0
  }

  function toggleTodoPanel() {
    todoPanelOpen.value = !todoPanelOpen.value
  }

  function resetHistoryState() {
    hasMoreHistory.value = false
    loadingOlderHistory.value = false
    loadingNewerMessages.value = false
  }

  function getStoppedPlaceholderText() {
    return i18n.global.t('assistantStopped') as string
  }

  function materializeMessages(items: MessageItem[]): MessageItem[] {
    return materializeStoppedMessages(items, getStoppedPlaceholderText())
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

  function parseSocketTimestamp(value: string | undefined, fallback = Date.now()) {
    if (!value) return fallback
    const parsed = Date.parse(value)
    return Number.isFinite(parsed) ? parsed : fallback
  }

  function isStreamingMessage(messageId: string): boolean {
    if (!currentBatchId.value) return false
    const batch = getCurrentBatch()
    return batch?.assistantMessageId === messageId
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

  function buildLiveThinkingHistory(content: string, timeline: AssistantReplyTimelineItem[]): SessionHistoryThinkingItem[] {
    const thinkingEntries = timeline.filter((entry) => entry.kind === 'thinking')
    if (thinkingEntries.length === 0) return []
    const thinkingIds = parseContentMarkers(content)
      .filter((segment) => segment.type === 'thinking_marker' && segment.thinkingId)
      .map((segment) => segment.thinkingId as string)
    return thinkingIds.map((thinkingId, index) => {
      const entry = thinkingEntries[index]
      return {
        thinkingId,
        content: entry?.kind === 'thinking' ? entry.content : '',
        status: entry?.kind === 'thinking' && !entry.done ? 'streaming' : 'completed',
        durationMs: entry?.kind === 'thinking' ? entry.durationMs : undefined,
      }
    })
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
      finalizeReplyBatchTiming(batch)
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

  function setSessionPageSize(size: number) {
    sessionPageSize.value = Math.min(Math.max(size, 10), MAX_SESSION_PAGE_SIZE)
  }

  async function loadSessions() {
    sessionSearchQuery.value = ''
    const res = await sessionAPI.list({ limit: sessionPageSize.value, offset: 0 })
    sessions.value = res.sessions
    hasMoreSessions.value = res.hasMore
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

  async function loadMoreSessions() {
    if (loadingMoreSessions.value || !hasMoreSessions.value) return
    loadingMoreSessions.value = true
    try {
      const q = sessionSearchQuery.value.trim()
      const res = await sessionAPI.list({
        limit: sessionPageSize.value,
        offset: sessions.value.length,
        ...(q ? { q } : {}),
      })
      const existing = new Set(sessions.value.map((s) => s.id))
      const next = res.sessions.filter((s) => !existing.has(s.id))
      sessions.value = [...sessions.value, ...next]
      hasMoreSessions.value = res.hasMore
    } finally {
      loadingMoreSessions.value = false
    }
  }

  async function searchSessions(query: string) {
    const q = query.trim()
    sessionSearchQuery.value = q
    if (!q) {
      await loadSessions()
      return
    }
    loadingMoreSessions.value = true
    try {
      const res = await sessionAPI.list({ q, limit: sessionPageSize.value, offset: 0 })
      sessions.value = res.sessions
      hasMoreSessions.value = res.hasMore
    } finally {
      loadingMoreSessions.value = false
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
    const item = await sessionAPI.create(i18n.global.t('newSession') as string)
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
      // Message-platform session may have no DB row before the first platform message; show read-only empty state first.
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
    if (!sessionId || !first || typeof first.seq !== 'number' || !hasMoreHistory.value || loadingOlderHistory.value)
      return false
    loadingOlderHistory.value = true
    try {
      const history = await sessionAPI.history(sessionId, {
        limit: HISTORY_PAGE_SIZE,
        before: first.createdAt,
        beforeSeq: first.seq,
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
      if (!latest || typeof latest.seq !== 'number') return false
      const history = await sessionAPI.history(sessionId, {
        limit: 50,
        after: latest.createdAt,
        afterSeq: latest.seq,
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
      onStart: (sessionId, meta) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        waiting.value = true
        streamingStarted.value = false
        clearRuntimeTodos()
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
          startedAt: parseSocketTimestamp(meta?.startedAt),
        })
      },
      onChunk: (chunk, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        const assistant = messages.value.find((msg) => msg.id === batch.assistantMessageId)
        if (!assistant) return
        assistant.content += chunk
        appendTextChunkToBatch(batch, chunk)
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
        planGenerating.value = false
        const batch = getCurrentBatch()
        if (batch) {
          finalizeOpenReplyRuntimeState(batch, 'Execution cancelled.', parseSocketTimestamp(meta?.finishedAt))
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
              assistant.content = stripContentMarkers(finalAnswer)
              clearAssistantError(assistant.id)
            }
            const liveThinking = buildLiveThinkingHistory(finalAnswer, batch.timeline)
            batch.timeline = hasContentMarkers(finalAnswer)
              ? buildInterleavedTimeline(batch.toolCalls, finalAnswer, liveThinking)
              : buildLegacyTimeline(batch.toolCalls, stripContentMarkers(finalAnswer))
          }
          finalizeReplyBatchTiming(batch, parseSocketTimestamp(meta?.finishedAt), meta?.durationMs)
        }
        currentBatchId.value = ''
        if (meta?.planId) {
          pendingPlanConfirmation.value = {
            sessionId,
            planId: meta.planId,
            content: meta.planBody || (answer ? stripContentMarkers(answer) : ''),
          }
        }
        await loadSessions()
      },
      onError: (error, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        waiting.value = false
        streamingStarted.value = false
        connectionError.value = error
        const batch = getCurrentBatch()
        if (batch) {
          finalizeOpenReplyRuntimeState(batch, error || 'Execution cancelled.')
        }
        finalizeAssistantError(error, sessionId)
      },
      onToolCallStart: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        finishOpenThinkingEntries(batch)
        // Handle ask_questions tool: parse questions and show Q&A drawer.
        if (data.toolName === 'ask_questions' && data.params?.questions) {
          try {
            const questions = JSON.parse(String(data.params.questions)) as QuestionItem[]
            if (Array.isArray(questions) && questions.length > 0) {
              pendingQuestions.value = { toolCallId: data.toolCallId, questions }
            }
          } catch { /* ignore parse errors, tool will timeout */ }
        }
        batch.toolCalls.push({
          toolCallId: data.toolCallId,
          toolName: data.toolName,
          command: data.command,
          params: data.params,
          preamble: data.preamble,
          requiresApproval: data.requiresApproval,
          status: data.requiresApproval ? 'pending' : 'executing',
          startedAt: parseSocketTimestamp(data.startedAt),
          parentToolCallId: data.parentToolCallId,
          subagentRunId: data.subagentRunId,
        })
        if (!data.parentToolCallId) {
          batch.timeline.push({
            id: crypto.randomUUID(),
            kind: 'tool_start',
            toolCallId: data.toolCallId,
          })
        }
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
          item.metadata = data.metadata
          item.requiresApproval = data.requiresApproval
          item.finishedAt = parseSocketTimestamp(data.finishedAt)
          if (data.parentToolCallId) item.parentToolCallId = data.parentToolCallId
          if (data.subagentRunId) item.subagentRunId = data.subagentRunId
          // Auto-close ask_questions drawer when tool times out or is rejected
          if (item.toolName === 'ask_questions' && (item.status === 'error' || item.status === 'rejected')) {
            pendingQuestions.value = null
          }
        }
        if (!data.parentToolCallId) {
          batch.timeline.push({
            id: crypto.randomUUID(),
            kind: 'tool_result',
            toolCallId: data.toolCallId,
          })
        }
      },
      onSubagentStart: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        const parent = batch.toolCalls.find((tc) => tc.toolCallId === data.parentToolCallId)
        if (parent) {
          parent.subagentRunId = data.subagentRunId
          parent.subagentTitle = data.title
          parent.subagentTask = data.task
          if (parent.subagentStream === undefined) parent.subagentStream = ''
        }
      },
      onSubagentChunk: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        const parent = batch.toolCalls.find((tc) => tc.toolCallId === data.parentToolCallId)
        if (parent) {
          if (parent.subagentStream === undefined) parent.subagentStream = ''
          parent.subagentStream += data.content
        }
      },
      onSubagentDone: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        finishSubagentThinking(batch, data.parentToolCallId)
        if (data.error) {
          const parent = batch.toolCalls.find((tc) => tc.toolCallId === data.parentToolCallId)
          if (parent && (parent.status === 'pending' || parent.status === 'executing')) {
            markToolCallError(batch, data.parentToolCallId, data.error)
          }
        }
      },
      onThinkingStart: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        if (data.parentToolCallId && data.subagentRunId) {
          startSubagentThinking(batch, data.parentToolCallId, parseSocketTimestamp(data.startedAt))
          return
        }
        batch.timeline.push({
          id: crypto.randomUUID(),
          kind: 'thinking',
          content: '',
          done: false,
          startedAt: parseSocketTimestamp(data.startedAt),
        })
      },
      onThinkingChunk: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        if (data.parentToolCallId && data.subagentRunId) {
          appendSubagentThinkingChunk(batch, data.parentToolCallId, data.content || '', parseSocketTimestamp(data.startedAt))
          return
        }
        const entries = [...batch.timeline]
        for (let i = entries.length - 1; i >= 0; i--) {
          const e = entries[i]
          // @ts-ignore
          if (e.kind === 'thinking' && !e.done) {
            // @ts-ignore
            entries[i] = { ...e, content: (e.content || '') + (data.content || '') }
            batch.timeline = entries
            break
          }
        }
      },
      onThinkingDone: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        if (data.parentToolCallId && data.subagentRunId) {
          finishSubagentThinking(batch, data.parentToolCallId, parseSocketTimestamp(data.finishedAt))
          return
        }
        batch.timeline = markLastThinkingDone(batch.timeline, parseSocketTimestamp(data.finishedAt))
      },
      onTodoUpdate: applyRuntimeTodoUpdate,
      onPlanStart: (sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        finishOpenThinkingEntries(batch)
        planGenerating.value = true
      },
      onPlanChunk: (chunk, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        appendPlanChunkToBatch(batch, chunk)
      },
      onPlanBody: (content, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        appendPlanBodyToBatch(batch, content)
        planGenerating.value = false
      },
      onSocketError: (error) => {
        waiting.value = false
        streamingStarted.value = false
        connectionError.value = error
        const batch = getCurrentBatch()
        if (batch) {
          finalizeOpenReplyRuntimeState(batch, error || 'Execution cancelled.')
        }
        clearRuntimeTodos()
      },
      onClose: () => {
        waiting.value = false
        streamingStarted.value = false
        const batch = getCurrentBatch()
        if (batch) {
          finalizeOpenReplyRuntimeState(batch)
        }
        clearRuntimeTodos()
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

  async function sendMessage(content: string, modelId: string, files: File[] = [], thinkingLevel: string = 'off', subagentModelId: string = '') {
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
    const sent = ws.send(trimmed, currentSessionId.value, modelId, uploaded.map((item) => item.id), thinkingLevel, planMode.value, subagentModelId)
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
    if (batch) markToolApprovalDecision(batch.toolCalls, toolCallId, approved)
    ws.sendToolApproval(toolCallId, approved)
  }

  function approveAllPendingToolCalls() {
    for (const toolCallId of pendingApprovalToolCallIds.value) {
      approveToolCall(toolCallId, true)
    }
  }

  function rejectAllPendingToolCalls() {
    for (const toolCallId of pendingApprovalToolCallIds.value) {
      approveToolCall(toolCallId, false)
    }
  }

  function submitQuestionAnswers(toolCallId: string, answers: string) {
    const batch = replyBatches.value.find((group) => group.toolCalls.some((tc) => tc.toolCallId === toolCallId))
    const item = batch?.toolCalls.find((tc) => tc.toolCallId === toolCallId)
    if (item) {
      item.status = 'executing'
    }
    ws.sendToolApproval(toolCallId, true, answers)
    pendingQuestions.value = null
  }

  function cancelQuestionAnswers(toolCallId: string) {
    const batch = replyBatches.value.find((group) => group.toolCalls.some((tc) => tc.toolCallId === toolCallId))
    const item = batch?.toolCalls.find((tc) => tc.toolCallId === toolCallId)
    if (item) {
      item.status = 'executing'
    }
    const questions = pendingQuestions.value?.questions ?? []
    const nullAnswers = JSON.stringify(
      questions.map((q) => ({ questionId: q.id, selectedOption: -2, customAnswer: '' })),
    )
    ws.sendToolApproval(toolCallId, true, nullAnswers)
    pendingQuestions.value = null
  }

  function disconnectSocket(options?: { silentConnectionNotice?: boolean }) {
    if (options?.silentConnectionNotice) {
      markSuppressNextConnectionNotice()
    }
    waiting.value = false
    streamingStarted.value = false
    clearRuntimeTodos()
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

  function togglePlanMode() {
    planMode.value = !planMode.value
  }

  function approvePlan(modelId: string, displayContent: string) {
    if (!pendingPlanConfirmation.value) return
    const { planId } = pendingPlanConfirmation.value
    const sessionId = currentSessionId.value
    if (!sessionId) return
    if (pendingPlanConfirmation.value.sessionId !== sessionId) return
    planMode.value = false
    const visibleContent = displayContent.trim() || (i18n.global.t('planExecuteUserMessage') as string)
    messages.value.push({
      id: crypto.randomUUID(),
      sessionId,
      role: 'user',
      content: visibleContent,
      createdAt: new Date().toISOString(),
    })
    ws.sendPlanApprove(planId, sessionId, modelId, visibleContent)
    pendingPlanConfirmation.value = null
  }

  function rejectPlan() {
    if (!pendingPlanConfirmation.value) return
    const { planId } = pendingPlanConfirmation.value
    const sessionId = currentSessionId.value
    if (!sessionId) return
    if (pendingPlanConfirmation.value.sessionId !== sessionId) return
    ws.sendPlanReject(planId, sessionId)
    pendingPlanConfirmation.value = null
  }

  function modifyPlan(feedback: string, modelId: string, thinkingLevel: string) {
    if (!pendingPlanConfirmation.value) return
    const { planId } = pendingPlanConfirmation.value
    const sessionId = currentSessionId.value
    if (!sessionId) return
    if (pendingPlanConfirmation.value.sessionId !== sessionId) return
    ws.sendPlanModify(planId, sessionId, modelId, feedback, thinkingLevel)
    pendingPlanConfirmation.value = null
  }

  function dismissPlanConfirmation() {
    pendingPlanConfirmation.value = null
  }

  return {
    sessions,
    sessionPageSize,
    setSessionPageSize,
    currentSessionId,
    messages,
    waiting,
    streamingStarted,
    hasMoreHistory,
    loadingOlderHistory,
    connectionStatus,
    isSocketReady,
    isAssistantErrorMessage,
    isStreamingMessage,
    isFailedUserMessage,
    replyBatches,
    currentBatchId,
    loadSessions,
    loadMoreSessions,
    searchSessions,
    hasMoreSessions,
    loadingMoreSessions,
    sessionSearchQuery,
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
    planMode,
    planGenerating,
    togglePlanMode,
    pendingPlanConfirmation,
    pendingApprovalToolCallIds,
    approvePlan,
    rejectPlan,
    modifyPlan,
    dismissPlanConfirmation,

    pendingQuestions,
    approveAllPendingToolCalls,
    rejectAllPendingToolCalls,
    submitQuestionAnswers,
    cancelQuestionAnswers,
    runtimeTodos,
    runtimeTodoNote,
    runtimeTodoUpdatedAt,
    todoPanelOpen,
    applyRuntimeTodoUpdate,
    clearRuntimeTodos,
    toggleTodoPanel,
  }
})
