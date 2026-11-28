import { computed, ref } from 'vue'

import { ChatSocket, type BackgroundTaskData, type ConnectionStatus, type ContextUsageData, type RuntimeTodoItem, type TodoUpdateData } from '@/api/chatSocket'
import { sessionAPI } from '@/api/chat'
import { isMessagePlatformSessionId } from '@/utils/messagePlatformSessions'
import type { MessageItem, SessionHistoryPayload } from '@/api/chat'
import { i18n } from '@/i18n'
import { buildReplyBatchesFromHistory, type AssistantReplyBatch } from '@/utils/replyBatchBuilder'
import { describeChatWSDiagnostic } from '../../../shared/chat-protocol/events.js'
import { getBatchApprovalToolCallIds } from '@/utils/toolApprovals'
import { materializeStoppedMessages } from '@/utils/chatMessages'
import { findLatestEditableUserMessageId } from '@/utils/messageEditing'
import { createClientId } from '@/utils/uuid'
import { isBudgetExceededError } from '@/utils/usageRange'
import { useToast } from '@/composables/useToast'
import {
  reduceChatRuntimeEvent,
  type ChatRuntimeEvent,
  type ChatRuntimeReducerDeps,
  type ChatRuntimeState,
} from './chatSessionReducer'

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
  const pendingPlanConfirmation = ref<ChatRuntimeState['pendingPlanConfirmation']>(null)
  const pendingApprovalToolCallIds = computed(() => replyBatches.value.flatMap((batch) => getBatchApprovalToolCallIds(batch.toolCalls)))
  const pendingEditMessageId = ref('')
  const latestEditableUserMessageId = computed(() => (pendingEditMessageId.value || hasNewerHistory.value) ? '' : findLatestEditableUserMessageId(messages.value, waiting.value, failedUserMessageIds.value))
  const pendingQuestions = ref<ChatRuntimeState['pendingQuestions']>(null)

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
    batch.timeline.push({ id: createClientId(), kind: 'notice', content })
  }

  async function refreshContextUsage(modelId: string) {
    const sessionId = currentSessionId.value
    if (!sessionId || !modelId || isMessagePlatformSessionId(sessionId)) {
      clearContextUsage()
      return
    }
    try {
      const usage = await sessionAPI.contextUsage(sessionId, modelId)
      if (currentSessionId.value === sessionId) contextUsage.value = usage
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
    return getCurrentBatch()?.assistantMessageId === messageId
  }

  function formatAssistantError(rawError: string) {
    if (isBudgetExceededError(rawError)) return i18n.global.t('usageBudgetExceededError') as string
    const safeError = rawError?.trim() || 'unknown error'
    return i18n.global.t('assistantReplyFailed', { error: safeError }) as string
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

  const reducerDeps: ChatRuntimeReducerDeps = {
    createId: createClientId,
    now: Date.now,
    formatAssistantError,
    stoppedPlaceholder: getStoppedPlaceholderText,
  }

  function runtimeState(): ChatRuntimeState {
    return {
      messages: messages.value,
      replyBatches: replyBatches.value,
      currentBatchId: currentBatchId.value,
      activeTurnId: activeTurnId.value,
      waiting: waiting.value,
      streamingStarted: streamingStarted.value,
      planGenerating: planGenerating.value,
      assistantErrorIds: assistantErrorIds.value,
      failedUserMessageIds: failedUserMessageIds.value,
      pendingQuestions: pendingQuestions.value,
      pendingPlanConfirmation: pendingPlanConfirmation.value,
      pendingEditMessageId: pendingEditMessageId.value,
    }
  }

  function applyRuntimeState(next: ChatRuntimeState) {
    messages.value = next.messages
    replyBatches.value = next.replyBatches
    currentBatchId.value = next.currentBatchId
    activeTurnId.value = next.activeTurnId
    waiting.value = next.waiting
    streamingStarted.value = next.streamingStarted
    planGenerating.value = next.planGenerating
    assistantErrorIds.value = next.assistantErrorIds
    failedUserMessageIds.value = next.failedUserMessageIds
    pendingQuestions.value = next.pendingQuestions
    pendingPlanConfirmation.value = next.pendingPlanConfirmation
    pendingEditMessageId.value = next.pendingEditMessageId
  }

  function applyRuntimeEvent(event: ChatRuntimeEvent): boolean {
    const previous = runtimeState()
    const next = reduceChatRuntimeEvent(previous, event, reducerDeps)
    if (next === previous) return false
    applyRuntimeState(next)
    return true
  }

  function clearActiveBackgroundTurn() {
    backgroundTurnBatches.delete(activeTurnId.value)
  }

  function connectSocket() {
    if (connectionRequested) return
    connectionRequested = true
    ws.connect({
      onSession: (id) => {
        if (!currentSessionId.value) currentSessionId.value = id
      },
      onStart: (sessionId, meta) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        const knownBatchId = meta?.background && meta.turnId ? backgroundTurnBatches.get(meta.turnId) : undefined
        const knownBatch = knownBatchId ? replyBatches.value.find(batch => batch.id === knownBatchId) : undefined
        const reuseBatchId = knownBatch && messages.value.some(message => message.id === knownBatch.assistantMessageId) ? knownBatchId : undefined
        applyRuntimeEvent({ type: 'start', sessionId, meta, reuseBatchId })
        if (!reuseBatchId) clearRuntimeTodos()
        if (meta?.background && meta.turnId) backgroundTurnBatches.set(meta.turnId, currentBatchId.value)
      },
      onBackgroundError: (error, sessionId) => {
        if (sessionId === currentSessionId.value) connectionError.value = error
      },
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
        applyRuntimeEvent({ type: 'steering', sessionId, data })
      },
      onMessageEdited: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        if (applyRuntimeEvent({ type: 'message_edited', data })) {
          clearContextUsage()
          clearRuntimeTodos()
        }
      },
      onChunk: (chunk, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'chunk', chunk })
      },
      onSessionTitle: (title, sessionId) => {
        if (!sessionId || !title) return
        callbacks.onTitle(sessionId, title)
      },
      onDone: async (sessionId, answer, meta) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        clearActiveBackgroundTurn()
        applyRuntimeEvent({ type: 'done', sessionId, answer, meta })
        if (meta?.skillLearning) {
          const key = meta.skillLearning.status === 'completed' ? 'skillLearningCompleted' : 'skillLearningFailed'
          const message = i18n.global.t(key) as string
          if (meta.skillLearning.status === 'completed') toast.success(message)
          else toast.error(message)
        }
        callbacks.onCompleted(sessionId)
        void callbacks.onSessionsChanged().catch(() => {})
      },
      onError: (error, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        clearActiveBackgroundTurn()
        applyRuntimeEvent({ type: 'error', sessionId, error })
        connectionError.value = error
        callbacks.onCompleted(sessionId)
      },
      onToolCallStart: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'tool_call_start', data })
      },
      onToolCallReview: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'tool_call_review', data })
      },
      onToolApprovalRequired: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'tool_approval_required', data })
      },
      onToolCallResult: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'tool_call_result', data })
      },
      onSubagentStart: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'subagent_start', data })
      },
      onSubagentChunk: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'subagent_chunk', data })
      },
      onSubagentDone: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'subagent_done', data })
      },
      onTeamStart: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'team_start', data })
      },
      onTeamMemberQueued: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'team_member_queued', data })
      },
      onTeamDone: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'team_done', data })
      },
      onThinkingStart: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'thinking_start', data })
      },
      onThinkingChunk: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'thinking_chunk', data })
      },
      onThinkingDone: (data, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'thinking_done', data })
      },
      onTodoUpdate: applyRuntimeTodoUpdate,
      onContextUsage: applyContextUsage,
      onContextCompacted: (usage, sessionId) => {
        applyContextUsage(usage, sessionId)
        appendContextCompactedNotice(sessionId)
      },
      onPlanStart: (sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'plan_start' })
      },
      onPlanChunk: (chunk, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'plan_chunk', chunk })
      },
      onPlanBody: (content, sessionId) => {
        if (!sessionId || sessionId !== currentSessionId.value) return
        applyRuntimeEvent({ type: 'plan_body', content })
      },
      onProtocolError: (diagnostic) => {
        console.warn(describeChatWSDiagnostic(diagnostic))
      },
      onSocketError: (error) => {
        clearActiveBackgroundTurn()
        applyRuntimeEvent({ type: 'connection_closed', error })
        pendingEditMessageId.value = ''
        connectionError.value = error
        clearRuntimeTodos()
      },
      onClose: () => {
        if (!connectionRequested) return
        clearActiveBackgroundTurn()
        applyRuntimeEvent({ type: 'connection_closed' })
        clearRuntimeTodos()
      },
      onStatusChange: (status, error) => {
        connectionStatus.value = status
        if (error) connectionError.value = error
        else if (status === 'connected') {
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
