import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { ChatSocket, type ConnectionStatus, type ContextUsageData, type RuntimeTodoItem, type TodoUpdateData } from '@/api/chatSocket'
import { sessionAPI } from '@/api/chat'
import { isMessagePlatformSessionId } from '@/utils/messagePlatformSessions'
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
import { applyEditedUserMessage, findLatestEditableUserMessageId } from '@/utils/messageEditing'
import { createClientId } from '@/utils/uuid'
import { createAgentTeamState, mergeAgentTeamMember, mergeAgentTeamRun } from '@/utils/agentTeam'

const HISTORY_PAGE_SIZE = 10
const MAX_SESSION_PAGE_SIZE = 100

export const useChatStore = defineStore('chat', () => {
  const sessions = ref<SessionItem[]>([])
  const sessionPageSize = ref(30)
  const hasMoreSessions = ref(false)
  const loadingMoreSessions = ref(false)
  const sessionSearchQuery = ref('')
  const currentSessionId = ref<string>()
  const creatingSession = ref(false)
  const draftWorkingDirectory = ref(typeof window !== 'undefined' ? window.localStorage.getItem('slimebot:last-working-directory') || '' : '')
  const currentWorkingDirectory = computed(() => currentSessionId.value
    ? sessions.value.find((item) => item.id === currentSessionId.value)?.workingDirectory || ''
    : draftWorkingDirectory.value)

  function setDraftWorkingDirectory(path: string) {
    if (currentSessionId.value || creatingSession.value) return
    draftWorkingDirectory.value = path
    if (path) window.localStorage.setItem('slimebot:last-working-directory', path)
    else window.localStorage.removeItem('slimebot:last-working-directory')
  }
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
  const contextUsage = ref<ContextUsageData | null>(null)

  const replyBatches = ref<AssistantReplyBatch[]>([])
  const currentBatchId = ref<string>('')
  const assistantErrorIds = ref(new Set<string>())
  const failedUserMessageIds = ref(new Set<string>())
  const pendingPlanConfirmation = ref<{ sessionId: string; planId: string; content: string } | null>(null)
  const pendingApprovalToolCallIds = computed(() => replyBatches.value.flatMap((batch) => getBatchApprovalToolCallIds(batch.toolCalls)))
  const pendingEditMessageId = ref('')
  const latestEditableUserMessageId = computed(() => pendingEditMessageId.value ? '' : findLatestEditableUserMessageId(messages.value, waiting.value, failedUserMessageIds.value))

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
    pendingEditMessageId.value = ''
    clearRuntimeTodos()
  }

  function clearContextUsage() {
    contextUsage.value = null
  }

  function applyContextUsage(usage: ContextUsageData, sessionId?: string) {
    const targetSessionId = sessionId || usage.sessionId
    if (!targetSessionId || targetSessionId !== currentSessionId.value) return
    contextUsage.value = { ...usage, sessionId: targetSessionId }
  }

  function appendContextCompactedNotice(sessionId?: string) {
    if (!sessionId || sessionId !== currentSessionId.value) return
    const batch = getCurrentBatch()
    if (!batch) return
    const content = i18n.global.t('contextCompactedNotice') as string
    const lastNotice = [...batch.timeline].reverse().find((entry) => entry.kind === 'notice')
    if (lastNotice?.kind === 'notice' && lastNotice.content === content) return
    batch.timeline.push({
      id: createClientId(),
      kind: 'notice',
      content,
    })
  }

  async function refreshContextUsage(modelId: string) {
    const sessionId = currentSessionId.value
    if (!sessionId || !modelId || isMessagePlatformSessionId(sessionId)) {
      clearContextUsage()
      return
    }
    try {
      const usage = await sessionAPI.contextUsage(sessionId, modelId)
      if (currentSessionId.value === sessionId) {
        contextUsage.value = usage
      }
    } catch {
      clearContextUsage()
    }
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
    const messageId = createClientId()
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
        id: createClientId(),
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

    const assistantMessageId = createClientId()
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
    hasMoreSessions.value = false
    const res = await sessionAPI.list({ limit: sessionPageSize.value, offset: 0 })
    sessions.value = res.sessions
    hasMoreSessions.value = res.hasMore
    const isVirtualMessagePlatformSession =
      isMessagePlatformSessionId(currentSessionId.value) &&
      !sessions.value.some((item) => item.id === currentSessionId.value)
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
    hasMoreSessions.value = false
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

  function resetToNewSession(workingDirectory?: string) {
    currentSessionId.value = undefined
    draftWorkingDirectory.value = workingDirectory ?? window.localStorage.getItem('slimebot:last-working-directory') ?? ''
    messages.value = []
    clearContextUsage()
    resetSessionRuntimeState()
    resetHistoryState()
  }

  async function createSession() {
    creatingSession.value = true
    try {
      const item = await sessionAPI.create(i18n.global.t('newSession') as string, draftWorkingDirectory.value)
      currentSessionId.value = item.id
      sessions.value = [item, ...sessions.value]
      messages.value = []
      clearContextUsage()
      resetSessionRuntimeState()
      resetHistoryState()
    } finally {
      creatingSession.value = false
    }
  }

  async function selectSession(id: string) {
    try {
      const history = await sessionAPI.history(id, { limit: HISTORY_PAGE_SIZE })
      currentSessionId.value = id
      messages.value = materializeMessages(history.messages)
      clearContextUsage()
      resetHistoryState()
      hasMoreHistory.value = history.hasMore
      resetSessionRuntimeState()
      rebuildReplyBatchesFromHistory(id, history)
    } catch {
      // Message-platform session may have no DB row before the first platform message; show read-only empty state first.
      if (isMessagePlatformSessionId(id)) {
        currentSessionId.value = id
        messages.value = []
        clearContextUsage()
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
        const assistantMessageId = createClientId()
        messages.value.push({
          id: assistantMessageId,
          sessionId: currentSessionId.value || '',
          role: 'assistant',
          content: '',
          createdAt: new Date().toISOString(),
        })
        clearAssistantError(assistantMessageId)
        const batchId = createClientId()
        currentBatchId.value = batchId
        replyBatches.value.push({
          id: batchId,
          sessionId: sessionId,
          assistantMessageId,
          toolCalls: [],
          teamRuns: [],
          timeline: [],
          collapsed: false,
          startedAt: parseSocketTimestamp(meta?.startedAt),
        })
      },
      onMessageEdited: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        if (!data.messageId || (pendingEditMessageId.value && data.messageId !== pendingEditMessageId.value)) return
        const applied = applyEditedUserMessage(messages.value, replyBatches.value, data.messageId, data.content, data.createdAt)
        messages.value = applied.messages
        replyBatches.value = applied.replyBatches
        currentBatchId.value = ''
        pendingPlanConfirmation.value = null
        clearContextUsage()
        clearRuntimeTodos()
        pendingEditMessageId.value = ''
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
        pendingEditMessageId.value = ''
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
        const existingToolCall = batch.toolCalls.find((tc) => tc.toolCallId === data.toolCallId)
        const startedToolCall = {
          toolCallId: data.toolCallId,
          toolName: data.toolName,
          command: data.command,
          params: data.params,
          preamble: data.preamble,
          requiresApproval: data.requiresApproval,
          reviewStatus: data.reviewStatus,
          reviewRisk: data.reviewRisk,
          reviewReason: data.reviewReason,
          status: data.reviewStatus === 'reviewing' ? 'reviewing' as const : data.requiresApproval ? 'pending' as const : 'executing' as const,
          startedAt: parseSocketTimestamp(data.startedAt),
          parentToolCallId: data.parentToolCallId,
          subagentRunId: data.subagentRunId,
          teamRunId: data.teamRunId,
          memberRunId: data.memberRunId,
        }
        if (existingToolCall) {
          Object.assign(existingToolCall, startedToolCall)
        } else {
          batch.toolCalls.push(startedToolCall)
        }
        if (!data.parentToolCallId) {
          batch.timeline.push({
            id: createClientId(),
            kind: 'tool_start',
            toolCallId: data.toolCallId,
          })
        }
      },
      onToolCallReview: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        const item = batch.toolCalls.find((tc) => tc.toolCallId === data.toolCallId)
        if (!item) return
        item.reviewStatus = data.reviewStatus
        item.reviewRisk = data.reviewRisk
        item.reviewReason = data.reviewReason
        if (data.reviewStatus === 'reviewing') item.status = 'reviewing'
        if (data.reviewStatus === 'approved') item.status = 'executing'
      },
      onToolApprovalRequired: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        const item = batch.toolCalls.find((tc) => tc.toolCallId === data.toolCallId)
        if (item) {
          item.requiresApproval = data.requiresApproval
          item.reviewStatus = data.reviewStatus
          item.reviewRisk = data.reviewRisk
          item.reviewReason = data.reviewReason
          item.status = 'pending'
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
          if (data.teamRunId) item.teamRunId = data.teamRunId
          if (data.memberRunId) item.memberRunId = data.memberRunId
          // Auto-close ask_questions drawer when tool times out or is rejected
          if (item.toolName === 'ask_questions' && (item.status === 'error' || item.status === 'rejected')) {
            pendingQuestions.value = null
          }
        }
        if (!data.parentToolCallId) {
          batch.timeline.push({
            id: createClientId(),
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
          if (data.teamRunId) parent.teamRunId = data.teamRunId
          if (data.memberRunId) parent.memberRunId = data.memberRunId
        }
        if (data.teamRunId && data.memberRunId) {
          const state = createAgentTeamState(batch.teamRuns)
          mergeAgentTeamMember(state, {
            id: data.memberRunId,
            teamRunId: data.teamRunId,
            toolCallId: data.parentToolCallId,
            subagentRunId: data.subagentRunId,
            title: data.title,
            task: data.task,
            status: 'running',
            startedAt: new Date().toISOString(),
          })
          batch.teamRuns = state.runs
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
          if (parent && (parent.status === 'pending' || parent.status === 'reviewing' || parent.status === 'executing')) {
            markToolCallError(batch, data.parentToolCallId, data.error)
          }
        }
        if (data.teamRunId && data.memberRunId) {
          const state = createAgentTeamState(batch.teamRuns)
          mergeAgentTeamMember(state, {
            id: data.memberRunId,
            teamRunId: data.teamRunId,
            toolCallId: data.parentToolCallId,
            subagentRunId: data.subagentRunId,
            title: '',
            task: '',
            status: data.error ? 'failed' : 'succeeded',
            error: data.error,
            finishedAt: new Date().toISOString(),
          })
          batch.teamRuns = state.runs
        }
      },
      onTeamStart: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        const state = createAgentTeamState(batch.teamRuns)
        mergeAgentTeamRun(state, data)
        batch.teamRuns = state.runs
      },
      onTeamMemberQueued: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        const state = createAgentTeamState(batch.teamRuns)
        mergeAgentTeamMember(state, data)
        batch.teamRuns = state.runs
      },
      onTeamDone: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const batch = getCurrentBatch()
        if (!batch) return
        const state = createAgentTeamState(batch.teamRuns)
        mergeAgentTeamRun(state, data)
        batch.teamRuns = state.runs
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
          id: createClientId(),
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
      onContextUsage: (usage, sessionId) => {
        applyContextUsage(usage, sessionId)
      },
      onContextCompacted: (usage, sessionId) => {
        applyContextUsage(usage, sessionId)
        appendContextCompactedNotice(sessionId)
      },
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
        pendingEditMessageId.value = ''
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
        pendingEditMessageId.value = ''
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
      id: createClientId(),
      sessionId: currentSessionId.value,
      role: 'user',
      content: trimmed,
      attachments: toMessageAttachments(uploaded),
      createdAt: new Date().toISOString(),
    })
    return true
  }

  async function sendEditedMessage(messageId: string, content: string, modelId: string, thinkingLevel: string = 'off', subagentModelId: string = '') {
    const trimmed = content.trim()
    if (!trimmed || !messageId || messageId !== latestEditableUserMessageId.value) {
      return false
    }
    if (!modelId) {
      connectionError.value = 'modelId is required'
      return false
    }
    const sessionId = currentSessionId.value
    if (!sessionId || !isSocketReady.value) {
      connectionError.value = 'socket is not connected'
      return false
    }
    const sent = ws.sendEdit(messageId, trimmed, sessionId, modelId, thinkingLevel, planMode.value, subagentModelId)
    if (!sent) {
      connectionError.value = 'socket is not connected'
      return false
    }
    pendingEditMessageId.value = messageId
    waiting.value = true
    streamingStarted.value = false
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
    pendingEditMessageId.value = ''
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
      id: createClientId(),
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

  function dismissPlanConfirmation() {
    pendingPlanConfirmation.value = null
  }

  return {
    sessions,
    draftWorkingDirectory,
    currentWorkingDirectory,
    creatingSession,
    setDraftWorkingDirectory,
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
    latestEditableUserMessageId,
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
    sendEditedMessage,
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
    contextUsage,
    refreshContextUsage,
    applyRuntimeTodoUpdate,
    clearRuntimeTodos,
    toggleTodoPanel,
  }
})
