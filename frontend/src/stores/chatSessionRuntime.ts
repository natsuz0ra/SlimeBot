import { computed, ref } from 'vue'

import { ChatSocket, type BackgroundTaskData, type ConnectionStatus, type ContextUsageData, type RuntimeTodoItem, type TodoUpdateData } from '@/api/chatSocket'
import { sessionAPI } from '@/api/chat'
import { isMessagePlatformSessionId } from '@/utils/messagePlatformSessions'
import type { MessageItem, SessionHistoryPayload, SessionHistoryThinkingItem } from '@/api/chat'
import { i18n } from '@/i18n'
import {
  buildInterleavedTimeline,
  buildReplyBatchesFromHistory,
  normalizeToolStatus,
  type AssistantReplyBatch,
  type AssistantReplyTimelineItem,
} from '@/utils/replyBatchBuilder'
import { parseContentMarkers, stripContentMarkers } from '@/utils/contentMarkers'
import { describeChatWSDiagnostic } from '../../../shared/chat-protocol/events.js'
import { appendPlanBodyToBatch, appendPlanChunkToBatch, appendSubagentThinkingChunk, appendTextChunkToBatch, finalizeOpenReplyRuntimeState, finalizeReplyBatchTiming, finishOpenThinkingEntries, finishSubagentThinking, markLastThinkingDone, markToolCallError, startSubagentThinking } from '@/utils/liveReplyTimeline'
import { getBatchApprovalToolCallIds } from '@/utils/toolApprovals'
import { materializeStoppedMessages } from '@/utils/chatMessages'
import { applyEditedUserMessage, findLatestEditableUserMessageId } from '@/utils/messageEditing'
import { createClientId } from '@/utils/uuid'
import { createAgentTeamState, mergeAgentTeamMember, mergeAgentTeamRun } from '@/utils/agentTeam'
import { isBudgetExceededError } from '@/utils/usageRange'
import { useToast } from '@/composables/useToast'

export function createChatSessionRuntime(sessionId: string | undefined, callbacks: {
  onCompleted: (id: string) => void
  onTitle: (id: string, title: string) => void
  onSessionsChanged: () => Promise<void>
  onBackgroundSnapshot: (id: string) => void
  onBackgroundIdle: (id: string) => void
}) {
  const currentSessionId = ref(sessionId)
  const focusedMessageId = ref('')
  const hasNewerHistory = ref(false)
  let connectionRequested = false
  const messages = ref<MessageItem[]>([])
  const backgroundTasks = ref<Record<string, BackgroundTaskData>>({})
  const backgroundTaskCount = computed(() => Object.keys(backgroundTasks.value).length)
  const backgroundTurnBatches = new Map<string, string>()
  const activeTurnId = ref('')
  const waiting = ref(false)
  const streamingStarted = ref(false)
  const hasMoreHistory = ref(false)
  const loadingOlderHistory = ref(false)
  const loadingNewerMessages = ref(false)
  const connectionStatus = ref<ConnectionStatus>('disconnected')
  const connectionError = ref('')
  const planGenerating = ref(false)
  const isSocketReady = computed(() => connectionStatus.value === 'connected')
  const runtimeTodos = ref<RuntimeTodoItem[]>([])
  const runtimeTodoNote = ref('')
  const runtimeTodoUpdatedAt = ref<number>()
  const todoPanelOpen = ref(false)
  const contextUsage = ref<ContextUsageData | null>(null)
  const contextCompacting = ref(false)
  const toast = useToast()

  const replyBatches = ref<AssistantReplyBatch[]>([])
  const currentBatchId = ref<string>('')
  const assistantErrorIds = ref(new Set<string>())
  const failedUserMessageIds = ref(new Set<string>())
  const pendingPlanConfirmation = ref<{ sessionId: string; planId: string; content: string } | null>(null)
  const pendingApprovalToolCallIds = computed(() => replyBatches.value.flatMap((batch) => getBatchApprovalToolCallIds(batch.toolCalls)))
  const pendingEditMessageId = ref('')
  const latestEditableUserMessageId = computed(() => (pendingEditMessageId.value || hasNewerHistory.value) ? '' : findLatestEditableUserMessageId(messages.value, waiting.value, failedUserMessageIds.value))

  interface QuestionItem {
    id: string
    question: string
    options: string[]
    option_descriptions?: string[]
  }
  const pendingQuestions = ref<{ toolCallId: string; questions: QuestionItem[] } | null>(null)

  const ws = new ChatSocket()

  function resetSessionRuntimeState() {
    backgroundTurnBatches.clear()
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

  async function compactContext(modelId: string) {
    const sessionId = currentSessionId.value
    if (!sessionId || !modelId || isMessagePlatformSessionId(sessionId) || contextCompacting.value || waiting.value) return false
    contextCompacting.value = true
    if (contextUsage.value) applyContextUsage({ ...contextUsage.value, state: 'compacting' }, sessionId)
    try {
      const { compacted, ...usage } = await sessionAPI.compactContext(sessionId, modelId)
      if (currentSessionId.value === sessionId) {
        contextUsage.value = { ...usage, state: 'ready' }
        if (compacted) {
          appendContextCompactedNotice(sessionId)
          toast.success(i18n.global.t('contextCompactSuccess') as string)
        } else {
          toast.info(i18n.global.t('contextCompactNoop') as string)
        }
      }
      return compacted
    } catch {
      if (currentSessionId.value === sessionId) {
        if (contextUsage.value) contextUsage.value = { ...contextUsage.value, state: 'ready' }
        toast.error(i18n.global.t('contextCompactFailed') as string)
      }
      return false
    } finally {
      contextCompacting.value = false
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
    hasNewerHistory.value = false
    focusedMessageId.value = ''
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
    if (isBudgetExceededError(rawError)) {
      return i18n.global.t('usageBudgetExceededError') as string
    }
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
      .flatMap((segment) => segment.type === 'thinking_marker' ? [segment.thinkingId] : [])
    return thinkingEntries.map((entry, index) => ({
      thinkingId: thinkingIds[index] ?? entry.id,
      content: entry.content,
      status: entry.done ? 'completed' : 'streaming',
      startedAt: entry.startedAt === undefined ? undefined : new Date(entry.startedAt).toISOString(),
      durationMs: entry.durationMs,
    }))
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

  function connectSocket() {
    if (connectionRequested) return
    connectionRequested = true
    ws.connect({
      onSession: (id) => {
        if (!currentSessionId.value) {
          currentSessionId.value = id
        }
      },
      onStart: (sessionId, meta) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        if (meta?.background && meta.turnId) {
          const known = backgroundTurnBatches.get(meta.turnId)
          const batch = known ? replyBatches.value.find(item => item.id === known) : undefined
          const assistant = batch ? messages.value.find(item => item.id === batch.assistantMessageId) : undefined
          if (batch && assistant) {
            assistant.content = ''; batch.timeline = []; batch.toolCalls = []; batch.teamRuns = []
            clearAssistantError(assistant.id)
            currentBatchId.value = batch.id; activeTurnId.value = meta.turnId
            waiting.value = true; streamingStarted.value = false
            return
          }
        }
        activeTurnId.value = meta?.turnId || ''
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
        if (meta?.background && meta.turnId) backgroundTurnBatches.set(meta.turnId, batchId)
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
      onBackgroundError: (error, sessionId) => {if (sessionId === currentSessionId.value) connectionError.value = error},
      onBackgroundTasks: (tasks, sessionId) => {
        if (sessionId !== currentSessionId.value) return
        backgroundTasks.value = Object.fromEntries(tasks.map(task => [task.taskId, task]))
        if (sessionId && !waiting.value) callbacks.onBackgroundSnapshot(sessionId)
      },
      onBackgroundTask: (task, sessionId) => {
        if (sessionId !== currentSessionId.value) return
        if (['finished', 'failed', 'stopped'].includes(task.status)) delete backgroundTasks.value[task.taskId]
        else backgroundTasks.value[task.taskId] = task
        if (sessionId && !waiting.value && !backgroundTaskCount.value) callbacks.onBackgroundIdle(sessionId)
      },
      onSteering: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        if (data.error) {
          connectionError.value = data.error
          if (data.content) pushFailedUserMessage(data.content)
          return
        }
        if (!data.messageId || messages.value.some(item => item.id === data.messageId)) return
        const batch = getCurrentBatch()
        const index = batch ? messages.value.findIndex(item => item.id === batch.assistantMessageId) : -1
        messages.value.splice(index < 0 ? messages.value.length : index, 0, {
          id: data.messageId,
          sessionId,
          role: 'user',
          content: data.content || '',
          createdAt: data.createdAt || new Date().toISOString(),
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
        callbacks.onTitle(sessionId, title)
      },
      onDone: async (sessionId, answer, meta) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        backgroundTurnBatches.delete(activeTurnId.value)
        activeTurnId.value = ''
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
            batch.timeline = buildInterleavedTimeline(batch.toolCalls, finalAnswer, liveThinking)
          }
          finalizeReplyBatchTiming(batch, parseSocketTimestamp(meta?.finishedAt), meta?.durationMs)
        }
        currentBatchId.value = ''
        if (meta?.skillLearning) {
          const key = meta.skillLearning.status === 'completed' ? 'skillLearningCompleted' : 'skillLearningFailed'
          const message = i18n.global.t(key) as string
          if (meta.skillLearning.status === 'completed') toast.success(message)
          else toast.error(message)
        }
        if (meta?.planId) {
          pendingPlanConfirmation.value = {
            sessionId,
            planId: meta.planId,
            content: meta.planBody || (answer ? stripContentMarkers(answer) : ''),
          }
        }
        callbacks.onCompleted(sessionId)
        void callbacks.onSessionsChanged().catch(() => {})
      },
      onError: (error, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        backgroundTurnBatches.delete(activeTurnId.value)
        activeTurnId.value = ''
        waiting.value = false
        streamingStarted.value = false
        connectionError.value = error
        pendingEditMessageId.value = ''
        const batch = getCurrentBatch()
        if (batch) {
          finalizeOpenReplyRuntimeState(batch, error || 'Execution cancelled.')
        }
        finalizeAssistantError(error, sessionId)
        callbacks.onCompleted(sessionId)
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
      onProtocolError: (diagnostic) => {
        console.warn(describeChatWSDiagnostic(diagnostic))
      },
      onSocketError: (error) => {
        backgroundTurnBatches.delete(activeTurnId.value)
        activeTurnId.value = ''
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
        if (!connectionRequested) return
        backgroundTurnBatches.delete(activeTurnId.value)
        activeTurnId.value = ''
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
          ws.watchSession(currentSessionId.value || '')
          connectionError.value = ''
        }
      },
    })
  }


  function closeSocket() {
    connectionRequested = false
    ws.close()
  }

  return {
    sessionId: currentSessionId, ws, connectSocket, closeSocket,
    messages, activeTurnId, backgroundTaskCount, waiting, streamingStarted, hasMoreHistory, loadingOlderHistory, loadingNewerMessages, focusedMessageId, hasNewerHistory, connectionStatus, connectionError, planGenerating, runtimeTodos, runtimeTodoNote, runtimeTodoUpdatedAt, todoPanelOpen, contextUsage, contextCompacting, replyBatches, currentBatchId, assistantErrorIds, failedUserMessageIds, pendingPlanConfirmation, pendingEditMessageId, pendingQuestions,
    resetSessionRuntimeState, clearContextUsage, refreshContextUsage, compactContext, clearRuntimeTodos, applyRuntimeTodoUpdate, toggleTodoPanel, resetHistoryState, materializeMessages, rebuildReplyBatchesFromHistory, mergeReplyBatchesFromHistory, isStreamingMessage, isAssistantErrorMessage, isFailedUserMessage, pushFailedUserMessage,
    isSocketReady, pendingApprovalToolCallIds, latestEditableUserMessageId,
  }
}
