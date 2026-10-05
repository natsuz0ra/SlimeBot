import { defineStore } from 'pinia'
import { computed, ref, shallowReactive, shallowRef, type Ref } from 'vue'
import { sessionAPI, type ChatSearchHit, type MessageAttachmentItem, type MessageItem, type SessionItem, type UploadedAttachmentItem } from '@/api/chat'
import { isMessagePlatformSessionId } from '@/utils/messagePlatformSessions'
import { i18n } from '@/i18n'
import { createClientId } from '@/utils/uuid'
import { markToolApprovalDecision } from '@/utils/toolApprovals'
import { createChatSessionRuntime } from './chatSessionRuntime'

const HISTORY_PAGE_SIZE = 10
const MAX_SESSION_PAGE_SIZE = 100

export const useChatStore = defineStore('chat', () => {
  const sessions = ref<SessionItem[]>([])
  const sessionPageSize = ref(30)
  const hasMoreSessions = ref(false)
  const loadingMoreSessions = ref(false)
  const searchSelection = ref<SessionItem>()
  const sessionMetadata = new Map<string, SessionItem>()
  const currentSessionId = ref<string>()
  let historyGeneration = 0
  const creatingSession = ref(false)
  const draftWorkingDirectory = ref(typeof window !== 'undefined' ? window.localStorage.getItem('slimebot:last-working-directory') || '' : '')
  const currentWorkingDirectory = computed(() => currentSessionId.value
    ? (sessions.value.find((item) => item.id === currentSessionId.value) ?? (searchSelection.value?.id === currentSessionId.value ? searchSelection.value : undefined))?.workingDirectory || ''
    : draftWorkingDirectory.value)

  function setDraftWorkingDirectory(path: string) {
    if (currentSessionId.value || creatingSession.value) return
    draftWorkingDirectory.value = path
    if (path) window.localStorage.setItem('slimebot:last-working-directory', path)
    else window.localStorage.removeItem('slimebot:last-working-directory')
  }
  function createSessionRuntime(id?: string) {
    return createChatSessionRuntime(id, {
      onCompleted: completeSession,
      onTitle: (sessionId, title) => {
        const item = sessions.value.find((session) => session.id === sessionId)
        if (item) item.name = title
        const selected = sessionMetadata.get(sessionId)
        if (selected) selected.name = title
      },
      onSessionsChanged: loadSessions,
    })
  }

  const planMode = ref(false)
  const suppressNextConnectionNotice = ref(false)
  const chatViewActive = ref(false)
  let socketEnabled = false
  const unreadSessionIds = ref(new Set<string>())
  const sessionRuntimes = shallowReactive(new Map<string, ReturnType<typeof createSessionRuntime>>())
  const draftRuntime = shallowRef(createSessionRuntime())
  const activeRuntime = computed(() => {
    const id = currentSessionId.value
    if (!id) return draftRuntime.value
    let runtime = sessionRuntimes.get(id)
    if (!runtime) {
      runtime = createSessionRuntime(id)
      sessionRuntimes.set(id, runtime)
    }
    return runtime
  })
  const runningSessionIds = computed(() => new Set(
    [...sessionRuntimes].filter(([, runtime]) => runtime.waiting.value).map(([id]) => id),
  ))

  function isSessionVisible(id: string) {
    return currentSessionId.value === id && chatViewActive.value && (typeof document === 'undefined' || document.visibilityState !== 'hidden')
  }

  function markSessionRead(id?: string) {
    if (id && isSessionVisible(id)) unreadSessionIds.value.delete(id)
  }

  function setChatViewActive(active: boolean) {
    chatViewActive.value = active
    if (active) markSessionRead(currentSessionId.value)
  }

  function completeSession(id: string) {
    if (isSessionVisible(id)) unreadSessionIds.value.delete(id)
    else unreadSessionIds.value.add(id)
    if (currentSessionId.value !== id) sessionRuntimes.get(id)?.closeSocket()
  }

  function connectSocket() {
    socketEnabled = true
    activeRuntime.value.connectSocket()
  }

  function releaseIdleSocket() {
    if (!activeRuntime.value.waiting.value) activeRuntime.value.closeSocket()
    if (sessionRuntimes.size <= 20) return
    for (const [id, runtime] of sessionRuntimes) {
      if (sessionRuntimes.size <= 20) break
      const latest = runtime.messages.value[runtime.messages.value.length - 1]
      if (id === currentSessionId.value || runtime.waiting.value || runtime.pendingPlanConfirmation.value || unreadSessionIds.value.has(id) || (latest && runtime.assistantErrorIds.value.has(latest.id))) continue
      runtime.closeSocket()
      sessionRuntimes.delete(id)
      sessionMetadata.delete(id)
    }
  }

  function forgetSession(id: string) {
    const runtime = sessionRuntimes.get(id)
    if (runtime?.waiting.value) runtime.ws.sendStop(id)
    runtime?.closeSocket()
    sessionRuntimes.delete(id)
    sessionMetadata.delete(id)
    unreadSessionIds.value.delete(id)
  }

  // The public store exposes the selected session; event reducers retain their own runtime refs.
  function bindRuntimeRef<T>(select: (runtime: ReturnType<typeof createSessionRuntime>) => Ref<T>) {
    return computed({
      get: () => select(activeRuntime.value).value,
      set: (value) => { select(activeRuntime.value).value = value },
    })
  }

  const messages = bindRuntimeRef((runtime) => runtime.messages)
  const waiting = bindRuntimeRef((runtime) => runtime.waiting)
  const streamingStarted = bindRuntimeRef((runtime) => runtime.streamingStarted)
  const hasMoreHistory = bindRuntimeRef((runtime) => runtime.hasMoreHistory)
  const loadingOlderHistory = bindRuntimeRef((runtime) => runtime.loadingOlderHistory)
  const loadingNewerMessages = bindRuntimeRef((runtime) => runtime.loadingNewerMessages)
  const focusedMessageId = bindRuntimeRef((runtime) => runtime.focusedMessageId)
  const hasNewerHistory = bindRuntimeRef((runtime) => runtime.hasNewerHistory)
  const connectionStatus = bindRuntimeRef((runtime) => runtime.connectionStatus)
  const connectionError = bindRuntimeRef((runtime) => runtime.connectionError)
  const planGenerating = bindRuntimeRef((runtime) => runtime.planGenerating)
  const runtimeTodos = bindRuntimeRef((runtime) => runtime.runtimeTodos)
  const runtimeTodoNote = bindRuntimeRef((runtime) => runtime.runtimeTodoNote)
  const runtimeTodoUpdatedAt = bindRuntimeRef((runtime) => runtime.runtimeTodoUpdatedAt)
  const todoPanelOpen = bindRuntimeRef((runtime) => runtime.todoPanelOpen)
  const contextUsage = bindRuntimeRef((runtime) => runtime.contextUsage)
  const replyBatches = bindRuntimeRef((runtime) => runtime.replyBatches)
  const currentBatchId = bindRuntimeRef((runtime) => runtime.currentBatchId)
  const pendingPlanConfirmation = bindRuntimeRef((runtime) => runtime.pendingPlanConfirmation)
  const pendingEditMessageId = bindRuntimeRef((runtime) => runtime.pendingEditMessageId)
  const pendingQuestions = bindRuntimeRef((runtime) => runtime.pendingQuestions)
  const isSocketReady = computed(() => activeRuntime.value.isSocketReady.value)
  const pendingApprovalToolCallIds = computed(() => activeRuntime.value.pendingApprovalToolCallIds.value)
  const latestEditableUserMessageId = computed(() => activeRuntime.value.latestEditableUserMessageId.value)
  const resetSessionRuntimeState = (...args: Parameters<ReturnType<typeof createSessionRuntime>['resetSessionRuntimeState']>) => activeRuntime.value.resetSessionRuntimeState(...args)
  const clearContextUsage = (...args: Parameters<ReturnType<typeof createSessionRuntime>['clearContextUsage']>) => activeRuntime.value.clearContextUsage(...args)
  const refreshContextUsage = (...args: Parameters<ReturnType<typeof createSessionRuntime>['refreshContextUsage']>) => activeRuntime.value.refreshContextUsage(...args)
  const clearRuntimeTodos = (...args: Parameters<ReturnType<typeof createSessionRuntime>['clearRuntimeTodos']>) => activeRuntime.value.clearRuntimeTodos(...args)
  const applyRuntimeTodoUpdate = (...args: Parameters<ReturnType<typeof createSessionRuntime>['applyRuntimeTodoUpdate']>) => activeRuntime.value.applyRuntimeTodoUpdate(...args)
  const toggleTodoPanel = (...args: Parameters<ReturnType<typeof createSessionRuntime>['toggleTodoPanel']>) => activeRuntime.value.toggleTodoPanel(...args)
  const resetHistoryState = (...args: Parameters<ReturnType<typeof createSessionRuntime>['resetHistoryState']>) => activeRuntime.value.resetHistoryState(...args)
  const materializeMessages = (...args: Parameters<ReturnType<typeof createSessionRuntime>['materializeMessages']>) => activeRuntime.value.materializeMessages(...args)
  const rebuildReplyBatchesFromHistory = (...args: Parameters<ReturnType<typeof createSessionRuntime>['rebuildReplyBatchesFromHistory']>) => activeRuntime.value.rebuildReplyBatchesFromHistory(...args)
  const mergeReplyBatchesFromHistory = (...args: Parameters<ReturnType<typeof createSessionRuntime>['mergeReplyBatchesFromHistory']>) => activeRuntime.value.mergeReplyBatchesFromHistory(...args)
  const isStreamingMessage = (...args: Parameters<ReturnType<typeof createSessionRuntime>['isStreamingMessage']>) => activeRuntime.value.isStreamingMessage(...args)
  const isAssistantErrorMessage = (...args: Parameters<ReturnType<typeof createSessionRuntime>['isAssistantErrorMessage']>) => activeRuntime.value.isAssistantErrorMessage(...args)
  const isFailedUserMessage = (...args: Parameters<ReturnType<typeof createSessionRuntime>['isFailedUserMessage']>) => activeRuntime.value.isFailedUserMessage(...args)

  function setSessionPageSize(size: number) {
    sessionPageSize.value = Math.min(Math.max(size, 10), MAX_SESSION_PAGE_SIZE)
  }

  async function loadSessions() {
    hasMoreSessions.value = false
    const res = await sessionAPI.list({ limit: sessionPageSize.value, offset: 0 })
    sessions.value = res.sessions
    hasMoreSessions.value = res.hasMore
  }

  async function loadMoreSessions() {
    if (loadingMoreSessions.value || !hasMoreSessions.value) return
    loadingMoreSessions.value = true
    try {
      const res = await sessionAPI.list({
        limit: sessionPageSize.value,
        offset: sessions.value.length,
      })
      const existing = new Set(sessions.value.map((s) => s.id))
      const next = res.sessions.filter((s) => !existing.has(s.id))
      sessions.value = [...sessions.value, ...next]
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
    historyGeneration++
    searchSelection.value = undefined
    releaseIdleSocket()
    // Keep any running session's channel alive when opening a new draft.
    draftRuntime.value.closeSocket()
    draftRuntime.value = createSessionRuntime()
    currentSessionId.value = undefined
    if (socketEnabled) connectSocket()
    draftWorkingDirectory.value = workingDirectory ?? window.localStorage.getItem('slimebot:last-working-directory') ?? ''
    messages.value = []
    clearContextUsage()
    resetSessionRuntimeState()
    resetHistoryState()
  }

  async function createSession() {
    const generation = ++historyGeneration
    const runtime = draftRuntime.value
    searchSelection.value = undefined
    creatingSession.value = true
    try {
      const item = await sessionAPI.create(i18n.global.t('newSession') as string, draftWorkingDirectory.value)
      sessions.value = [item, ...sessions.value]
      sessionMetadata.set(item.id, item)
      if (generation !== historyGeneration) return undefined
      runtime.sessionId.value = item.id
      sessionRuntimes.set(item.id, runtime)
      draftRuntime.value = createSessionRuntime()
      currentSessionId.value = item.id
      connectSocket()
      messages.value = []
      clearContextUsage()
      resetSessionRuntimeState()
      resetHistoryState()
      return item.id
    } finally {
      creatingSession.value = false
    }
  }

  async function selectSession(id: string, target?: ChatSearchHit) {
    const generation = ++historyGeneration
    loadingOlderHistory.value = false
    loadingNewerMessages.value = false
    const cached = sessionRuntimes.get(id)
    const latestCachedMessage = cached?.messages.value[cached.messages.value.length - 1]
    const preserveRuntime = cached && (cached.waiting.value || cached.pendingPlanConfirmation.value || (latestCachedMessage && cached.assistantErrorIds.value.has(latestCachedMessage.id)))
    if (preserveRuntime && cached.messages.value.length > 0 && (!target?.messageId || cached.messages.value.some((item) => item.id === target.messageId))) {
      releaseIdleSocket()
      currentSessionId.value = id
      focusedMessageId.value = target?.messageId || ''
      searchSelection.value = sessionMetadata.get(id)
      connectSocket()
      markSessionRead(id)
      return
    }
    try {
      const selected = isMessagePlatformSessionId(id) ? undefined : await sessionAPI.get(id)
      const history = await sessionAPI.history(id, target?.messageId ? {
        limit: HISTORY_PAGE_SIZE, before: target.createdAt, beforeSeq: target.seq! + 1,
      } : { limit: HISTORY_PAGE_SIZE })
      const newer = target?.messageId ? await sessionAPI.history(id, {
        limit: HISTORY_PAGE_SIZE, after: target.createdAt, afterSeq: target.seq,
      }) : undefined
      if (generation !== historyGeneration) return
      if (target?.messageId && !history.messages.some((item) => item.id === target.messageId)) {
        throw new Error('Search result no longer exists')
      }
      releaseIdleSocket()
      if (selected) sessionMetadata.set(id, selected)
      currentSessionId.value = id
      // Search may load an older page while the live reply continues. Keep its runtime intact.
      if (preserveRuntime && cached) {
        prependUniqueMessages(materializeMessages(history.messages))
        mergeReplyBatchesFromHistory(id, history, 'prepend')
        if (newer) {
          prependUniqueMessages(materializeMessages(newer.messages))
          mergeReplyBatchesFromHistory(id, newer, 'prepend')
        }
        hasMoreHistory.value = history.hasMore
        focusedMessageId.value = target?.messageId || ''
        searchSelection.value = selected
        connectSocket()
        markSessionRead(id)
        return
      }
      messages.value = materializeMessages(history.messages)
      clearContextUsage()
      resetHistoryState()
      hasMoreHistory.value = history.hasMore
      focusedMessageId.value = target?.messageId || ''
      hasNewerHistory.value = newer?.hasMore || false
      searchSelection.value = selected
      resetSessionRuntimeState()
      rebuildReplyBatchesFromHistory(id, history)
      connectSocket()
      markSessionRead(id)
      if (newer) {
        appendUniqueMessages(materializeMessages(newer.messages))
        mergeReplyBatchesFromHistory(id, newer, 'append')
      }
    } catch (error) {
      if (generation !== historyGeneration) return
      // Message-platform session may have no DB row before the first platform message; show read-only empty state first.
      if (isMessagePlatformSessionId(id) && !target?.messageId) {
        releaseIdleSocket()
        currentSessionId.value = id
        messages.value = []
        clearContextUsage()
        resetSessionRuntimeState()
        resetHistoryState()
        return
      }
      throw Object.assign(new Error('load session history failed'), { cause: error })
    }
  }

  async function loadOlderMessages() {
    const generation = historyGeneration
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
      if (currentSessionId.value !== sessionId || generation !== historyGeneration) return false
      prependUniqueMessages(materializeMessages(history.messages))
      hasMoreHistory.value = history.hasMore
      mergeReplyBatchesFromHistory(sessionId, history, 'prepend')
      return history.messages.length > 0
    } finally {
      if (generation === historyGeneration) loadingOlderHistory.value = false
    }
  }

  async function loadNewMessagesForSession(sessionId: string) {
    const generation = historyGeneration
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
      if (currentSessionId.value !== sessionId || generation !== historyGeneration) return false
      hasNewerHistory.value = history.hasMore
      appendUniqueMessages(materializeMessages(history.messages))
      mergeReplyBatchesFromHistory(sessionId, history, 'append')
      return history.messages.length > 0
    } finally {
      if (generation === historyGeneration) loadingNewerMessages.value = false
    }
  }

  async function ensureSessionReady() {
    if (currentSessionId.value) return currentSessionId.value
    if (creatingSession.value) return undefined
    return createSession()
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

  async function sendMessage(content: string, modelId: string, files: File[] = [], thinkingLevel: string = 'off', subagentModelId: string = '') {
    const trimmed = content.trim()
    if (!trimmed && files.length === 0) return false
    if (!modelId) {
      activeRuntime.value.connectionError.value = 'modelId is required'
      activeRuntime.value.pushFailedUserMessage(trimmed)
      return false
    }
    const sessionId = await ensureSessionReady()
    if (!sessionId) return false
    const runtime = sessionRuntimes.get(sessionId)
    if (!runtime) return false
    if (runtime.waiting.value) return false
    if (runtime.hasNewerHistory.value) await selectSession(sessionId)
    runtime.focusedMessageId.value = ''
    if (!runtime.isSocketReady.value) {
      runtime.connectionError.value = 'socket is not connected'
      runtime.pushFailedUserMessage(trimmed)
      return false
    }
    const usePlanMode = planMode.value
    runtime.waiting.value = true
    unreadSessionIds.value.delete(sessionId)
    try {
      const uploaded = files.length > 0 ? (await sessionAPI.uploadAttachments(sessionId, files)).items || [] : []
      const sent = runtime.ws.send(trimmed, sessionId, modelId, uploaded.map((item) => item.id), thinkingLevel, usePlanMode, subagentModelId)
      if (!sent) {
        runtime.waiting.value = false
        runtime.connectionError.value = 'socket is not connected'
        runtime.pushFailedUserMessage(trimmed)
        return false
      }
      runtime.messages.value.push({
        id: createClientId(), sessionId, role: 'user', content: trimmed,
        attachments: toMessageAttachments(uploaded), createdAt: new Date().toISOString(),
      })
      return true
    } catch (error) {
      runtime.waiting.value = false
      throw error
    }
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
    const sent = activeRuntime.value.ws.sendEdit(messageId, trimmed, sessionId, modelId, thinkingLevel, planMode.value, subagentModelId)
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
    return activeRuntime.value.ws.sendStop(sessionId)
  }

  function approveToolCall(toolCallId: string, approved: boolean) {
    const batch = replyBatches.value.find((group) => group.toolCalls.some((tc) => tc.toolCallId === toolCallId))
    if (batch) markToolApprovalDecision(batch.toolCalls, toolCallId, approved)
    activeRuntime.value.ws.sendToolApproval(toolCallId, approved)
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
    activeRuntime.value.ws.sendToolApproval(toolCallId, true, answers)
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
    activeRuntime.value.ws.sendToolApproval(toolCallId, true, nullAnswers)
    pendingQuestions.value = null
  }

  function disconnectSocket(options?: { silentConnectionNotice?: boolean }) {
    socketEnabled = false
    if (options?.silentConnectionNotice) {
      markSuppressNextConnectionNotice()
    }
    for (const runtime of sessionRuntimes.values()) runtime.closeSocket()
    sessionRuntimes.clear()
    sessionMetadata.clear()
    draftRuntime.value.closeSocket()
    draftRuntime.value = createSessionRuntime()
    unreadSessionIds.value.clear()
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
    if (!pendingPlanConfirmation.value || !isSocketReady.value) return
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
    if (activeRuntime.value.ws.sendPlanApprove(planId, sessionId, modelId, visibleContent)) {
      waiting.value = true
      unreadSessionIds.value.delete(sessionId)
      pendingPlanConfirmation.value = null
    }
  }

  function rejectPlan() {
    if (!pendingPlanConfirmation.value || !isSocketReady.value) return
    const { planId } = pendingPlanConfirmation.value
    const sessionId = currentSessionId.value
    if (!sessionId) return
    if (pendingPlanConfirmation.value.sessionId !== sessionId) return
    activeRuntime.value.ws.sendPlanReject(planId, sessionId)
    pendingPlanConfirmation.value = null
  }

  function dismissPlanConfirmation() {
    pendingPlanConfirmation.value = null
  }

  return {
    sessions,
    runningSessionIds,
    unreadSessionIds,
    setChatViewActive,
    markSessionRead,
    forgetSession,
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
    hasMoreSessions,
    loadingMoreSessions,
    searchSelection,
    focusedMessageId,
    hasNewerHistory,
    loadingNewerMessages,
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
