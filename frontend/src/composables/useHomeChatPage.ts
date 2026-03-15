import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter, useRoute } from 'vue-router'
import { useToast } from '@/composables/useToast'

import { sessionAPI, type ToolCallItem } from '@/api/chat'
import { llmAPI, type LLMConfig } from '@/api/settings'
import { useChatStore } from '@/stores/chat'

export function useHomeChatPage() {
  const { t } = useI18n()
  const store = useChatStore()
  const toast = useToast()
  const router = useRouter()
  const route = useRoute()
  const MODEL_STORAGE_KEY = 'slimebot:selectedModelId'

  const drawerOpen = ref(false)
  const renameVisible = ref(false)
  const renameValue = ref('')
  const renameTargetId = ref('')
  const inputValue = ref('')
  const loading = ref(false)
  const settingsVisible = ref(false)
  const hasConnectedOnce = ref(false)
  const showInitialConnectionNotice = ref(false)
  const suppressConnectionNoticeDisplay = ref(false)
  const initialConnectionNoticeTimer = ref<number | null>(null)
  const toolDetailVisible = ref(false)
  const toolDetailBatchId = ref('')
  const toolDetailDialogWidth = 'min(688px, calc(100vw - 36px))'

  const activeSessionMenu = ref<{ id: string; x: number; y: number } | null>(null)
  const topMenuVisible = ref(false)
  const deleteConfirmVisible = ref(false)
  const deleteTargetId = ref('')
  const modelOptions = ref<LLMConfig[]>([])
  const selectedModelId = ref('')
  const messagesRef = ref<HTMLElement | null>(null)
  const sidebarListRef = ref<HTMLElement | null>(null)
  const autoStickToBottom = ref(true)
  const scrollToBottomPending = ref(false)
  const scrollToBottomPendingTimer = ref<number | null>(null)
  const scrollToBottomEndHandler = ref<(() => void) | null>(null)
  const BOTTOM_STICK_THRESHOLD_PX = 32
  const SCROLL_TO_BOTTOM_PENDING_MAX_MS = 2000
  const INITIAL_CONNECTION_NOTICE_DELAY_MS = 1500
  const scrollTimers = new Map<HTMLElement, ReturnType<typeof setTimeout>>()
  const scrollHandlers = new Map<HTMLElement, () => void>()

  function setMessagesRef(el: any) {
    messagesRef.value = (el?.$el ?? el) as HTMLElement | null
  }

  function setSidebarListRef(el: any) {
    sidebarListRef.value = (el?.$el ?? el) as HTMLElement | null
  }

  function isNearBottom(el: HTMLElement, threshold = BOTTOM_STICK_THRESHOLD_PX) {
    const distanceToBottom = el.scrollHeight - (el.scrollTop + el.clientHeight)
    return distanceToBottom <= threshold
  }

  function syncAutoStickToBottom(el: HTMLElement | null = messagesRef.value) {
    if (!el) {
      autoStickToBottom.value = true
      scrollToBottomPending.value = false
      return
    }
    const nearBottom = isNearBottom(el)
    if (scrollToBottomPending.value) {
      if (nearBottom) {
        finishScrollToBottomPending(el)
      }
      return
    }
    autoStickToBottom.value = nearBottom
  }

  function clearScrollToBottomPendingTimer() {
    if (scrollToBottomPendingTimer.value !== null) {
      window.clearTimeout(scrollToBottomPendingTimer.value)
      scrollToBottomPendingTimer.value = null
    }
  }

  function clearScrollToBottomEndHandler() {
    const el = messagesRef.value
    const handler = scrollToBottomEndHandler.value
    if (!el || !handler) return
    el.removeEventListener('scrollend', handler as EventListener)
    scrollToBottomEndHandler.value = null
  }

  function finishScrollToBottomPending(el: HTMLElement) {
    clearScrollToBottomPendingTimer()
    clearScrollToBottomEndHandler()
    scrollToBottomPending.value = false
    syncAutoStickToBottom(el)
  }

  function unbindScrollFade(el: HTMLElement | null) {
    if (!el) return
    const handler = scrollHandlers.get(el)
    if (handler) {
      el.removeEventListener('scroll', handler)
      scrollHandlers.delete(el)
    }
    const prev = scrollTimers.get(el)
    if (prev) {
      clearTimeout(prev)
      scrollTimers.delete(el)
    }
  }

  function bindScrollFade(el: HTMLElement | null, onScroll?: () => void) {
    if (!el) return
    unbindScrollFade(el)
    const handler = () => {
      el.classList.add('is-scrolling')
      const prev = scrollTimers.get(el)
      if (prev) clearTimeout(prev)
      scrollTimers.set(
        el,
        setTimeout(() => {
          el.classList.remove('is-scrolling')
          scrollTimers.delete(el)
        }, 1500),
      )
      onScroll?.()
    }
    scrollHandlers.set(el, handler)
    el.addEventListener('scroll', handler, { passive: true })
  }

  const currentSession = computed(() => store.sessions.find((item) => item.id === store.currentSessionId))
  const modelSelectOptions = computed(() => modelOptions.value.map((m) => ({ value: m.id, label: m.name })))
  const hasModel = computed(() => modelOptions.value.length > 0)
  const isEmptySession = computed(() => !loading.value && store.messages.length === 0)
  const showScrollToBottom = computed(() => !isEmptySession.value && !autoStickToBottom.value)
  const sendDisabled = computed(() => !hasModel.value || !selectedModelId.value || !inputValue.value.trim() || store.waiting || !store.isSocketReady)
  const shouldShowConnectionNotice = computed(() => {
    if (suppressConnectionNoticeDisplay.value) return false
    if (store.connectionStatus === 'connected') return false
    return hasConnectedOnce.value || showInitialConnectionNotice.value
  })
  const networkStatusText = computed(() => {
    if (!shouldShowConnectionNotice.value) return ''
    if (store.connectionStatus === 'reconnecting') return t('networkReconnecting')
    if (store.connectionStatus === 'disconnected') return t('networkDisconnected')
    return ''
  })
  function findReplyBatchByMessageId(messageId: string) {
    return store.replyBatches.find((batch) => batch.assistantMessageId === messageId)
  }

  function getReplyToolCount(messageId: string) {
    return findReplyBatchByMessageId(messageId)?.toolCalls.length || 0
  }

  function getToolCallDesc(toolCall: ToolCallItem) {
    const params = toolCall.params || {}
    const nonEmptyEntries = Object.entries(params).filter(([, value]) => String(value ?? '').trim() !== '')

    if (toolCall.toolName === 'web_search') {
      const query = String(params.query ?? '').trim()
      if (query !== '') return `query: ${query}`
    }

    if (nonEmptyEntries.length === 0) return toolCall.command || ''
    return nonEmptyEntries
      .map(([key, value]) => `${key}: ${String(value)}`)
      .join(' | ')
  }

  function getReplyToolSummary(messageId: string) {
    const batch = findReplyBatchByMessageId(messageId)
    if (!batch) return ''

    const count = batch.toolCalls.length
    if (count === 0) return ''
    if (batch.collapsed) return t('toolExecutionCount', { count })

    const runningCall = [...batch.toolCalls].reverse().find((item) => item.status === 'pending' || item.status === 'executing')
    if (runningCall) {
      const desc = getToolCallDesc(runningCall).trim()
      if (desc !== '') {
        return t('toolExecutionRunning', { command: runningCall.toolName, desc })
      }
      return t('toolExecutionRunningNoDesc', { command: runningCall.toolName })
    }

    const latest = batch.toolCalls[count - 1]
    if (!latest) return t('toolExecutionCount', { count })
    if (latest.status === 'completed') {
      return t('toolExecutionSuccess', { command: latest.toolName })
    }
    return t('toolExecutionFailed', { command: latest.toolName })
  }

  function getReplyToolCalls(messageId: string): ToolCallItem[] {
    return findReplyBatchByMessageId(messageId)?.toolCalls || []
  }

  function getReplyTimeline(messageId: string) {
    return findReplyBatchByMessageId(messageId)?.timeline || []
  }

  function getReplyToolItem(messageId: string, toolCallId: string) {
    return getReplyToolCalls(messageId).find((item) => item.toolCallId === toolCallId)
  }

  function shouldShowInlineToolCall(messageId: string, toolCallId: string) {
    const item = getReplyToolItem(messageId, toolCallId)
    if (!item) return false
    return item.requiresApproval
  }

  function isReplyToolCollapsed(messageId: string) {
    return findReplyBatchByMessageId(messageId)?.collapsed ?? false
  }

  function isEmptyPlaceholder(messageId: string) {
    const batch = findReplyBatchByMessageId(messageId)
    if (!batch) return false
    if (batch.collapsed) return false
    const msg = store.messages.find((m) => m.id === messageId)
    return !!msg && msg.content === '' && batch.timeline.length === 0
  }

  function openToolDetail(messageId: string) {
    const batch = findReplyBatchByMessageId(messageId)
    if (!batch || batch.toolCalls.length === 0) return
    toolDetailBatchId.value = batch.id
    toolDetailVisible.value = true
  }

  const toolDetailItems = computed(() => {
    return store.replyBatches.find((batch) => batch.id === toolDetailBatchId.value)?.toolCalls || []
  })
  const toolDetailTimeline = computed(() => {
    return store.replyBatches.find((batch) => batch.id === toolDetailBatchId.value)?.timeline || []
  })
  const toolDetailToolTimeline = computed(() => {
    return toolDetailTimeline.value.filter((entry) => entry.kind !== 'text')
  })

  function onGlobalClick() {
    activeSessionMenu.value = null
    topMenuVisible.value = false
  }

  function toggleSidebar() {
    drawerOpen.value = !drawerOpen.value
  }

  function toggleSessionMenu(sessionId: string, event: MouseEvent) {
    const target = event.currentTarget as HTMLElement | null
    if (!target) return
    if (activeSessionMenu.value?.id === sessionId) {
      activeSessionMenu.value = null
      return
    }
    const rect = target.getBoundingClientRect()
    activeSessionMenu.value = { id: sessionId, x: rect.right + 6, y: rect.top }
  }

  function syncModelToLocal(modelId: string) {
    if (!modelId) {
      localStorage.removeItem(MODEL_STORAGE_KEY)
      return
    }
    localStorage.setItem(MODEL_STORAGE_KEY, modelId)
  }

  function resolveInitialModelId(items: LLMConfig[]) {
    const first = items[0]
    if (!first) return ''
    const remembered = localStorage.getItem(MODEL_STORAGE_KEY)
    const matched = remembered ? items.find((item) => item.id === remembered) : undefined
    return matched?.id || first.id
  }

  async function refreshModelOptions(useRemembered = false) {
    const latestModels = await llmAPI.list()
    modelOptions.value = latestModels

    let nextModelId = ''
    if (latestModels.length > 0) {
      const hasCurrent = selectedModelId.value && latestModels.some((item) => item.id === selectedModelId.value)
      if (hasCurrent) {
        nextModelId = selectedModelId.value
      } else if (useRemembered) {
        nextModelId = resolveInitialModelId(latestModels)
      } else {
        const firstModel = latestModels[0]
        nextModelId = firstModel ? firstModel.id : ''
      }
    }

    selectedModelId.value = nextModelId
    syncModelToLocal(nextModelId)
  }

  function showWarning(message: string) {
    toast.warning(message)
  }

  function showError(message: string) {
    toast.error(message)
  }

  function clearInitialConnectionNoticeTimer() {
    if (initialConnectionNoticeTimer.value !== null) {
      window.clearTimeout(initialConnectionNoticeTimer.value)
      initialConnectionNoticeTimer.value = null
    }
  }

  function scheduleInitialConnectionNotice() {
    if (showInitialConnectionNotice.value || initialConnectionNoticeTimer.value !== null) return
    initialConnectionNoticeTimer.value = window.setTimeout(() => {
      initialConnectionNoticeTimer.value = null
      if (hasConnectedOnce.value || suppressConnectionNoticeDisplay.value || store.connectionStatus === 'connected') return
      showInitialConnectionNotice.value = true
      showWarning(t(store.connectionStatus === 'reconnecting' ? 'networkReconnecting' : 'networkDisconnected'))
    }, INITIAL_CONNECTION_NOTICE_DELAY_MS)
  }

  function scrollMessagesToBottom(force = false) {
    const el = messagesRef.value
    if (!el) return
    if (!force && !autoStickToBottom.value) return
    el.scrollTop = el.scrollHeight
    autoStickToBottom.value = true
  }

  function queueScrollMessagesToBottom(force = false) {
    void nextTick(() => {
      scrollMessagesToBottom(force)
    })
  }

  function scrollToBottomByButton() {
    const el = messagesRef.value
    if (!el) return
    scrollToBottomPending.value = true
    el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
    clearScrollToBottomPendingTimer()
    clearScrollToBottomEndHandler()
    const onScrollEnd = () => {
      finishScrollToBottomPending(el)
    }
    scrollToBottomEndHandler.value = onScrollEnd
    el.addEventListener('scrollend', onScrollEnd as EventListener, { once: true })
    scrollToBottomPendingTimer.value = window.setTimeout(() => {
      finishScrollToBottomPending(el)
    }, SCROLL_TO_BOTTOM_PENDING_MAX_MS)
  }

  async function boot() {
    loading.value = true
    try {
      await refreshModelOptions(true)

      await store.loadSessions()
      const routeSessionId = route.params.sessionId as string | undefined
      const isNewChatRoute = route.name === 'new-chat' || route.name === 'home'
      if (routeSessionId && routeSessionId !== 'new_chat') {
        const matched = store.sessions.find((s) => s.id === routeSessionId)
        if (matched) {
          await store.selectSession(matched.id)
        } else if (store.sessions.length > 0) {
          const first = store.sessions[0]
          if (first) await store.selectSession(first.id)
        } else {
          store.resetToNewSession()
        }
      } else if (isNewChatRoute || store.sessions.length === 0) {
        store.resetToNewSession()
      } else {
        const first = store.sessions[0]
        if (first) await store.selectSession(first.id)
      }
      await nextTick()
      scrollMessagesToBottom(true)
      store.connectSocket()
    } finally {
      loading.value = false
    }
  }

  function openRename(sessionId: string, oldName: string) {
    renameTargetId.value = sessionId
    renameValue.value = oldName
    renameVisible.value = true
  }

  async function confirmRename() {
    if (!renameTargetId.value || !renameValue.value.trim()) return
    await sessionAPI.rename(renameTargetId.value, renameValue.value.trim())
    await store.loadSessions()
    renameVisible.value = false
  }

  function removeSession(id: string) {
    deleteTargetId.value = id
    deleteConfirmVisible.value = true
    activeSessionMenu.value = null
    topMenuVisible.value = false
  }

  async function confirmDeleteSession() {
    if (!deleteTargetId.value) return
    try {
      const isDeletingCurrent = deleteTargetId.value === store.currentSessionId
      await sessionAPI.remove(deleteTargetId.value)
      await store.loadSessions()
      if (isDeletingCurrent) {
        store.resetToNewSession()
      }
    } catch {
      showError('删除失败')
    } finally {
      deleteConfirmVisible.value = false
      deleteTargetId.value = ''
    }
  }

  async function pickSession(id: string) {
    await store.selectSession(id)
    await nextTick()
    scrollMessagesToBottom(true)
    drawerOpen.value = false
  }

  async function createSession() {
    store.resetToNewSession()
    autoStickToBottom.value = true
    queueScrollMessagesToBottom(true)
    drawerOpen.value = false
    void router.push('/chat/new_chat')
  }

  async function sendMessage() {
    if (sendDisabled.value) return
    autoStickToBottom.value = true
    queueScrollMessagesToBottom(true)
    const sent = await store.sendMessage(inputValue.value.trim(), selectedModelId.value)
    if (!sent) {
      showWarning(t('sendBlockedOffline'))
      return
    }
    inputValue.value = ''
    // 发送成功后立即刷新会话列表，确保顶部标题与侧边栏列表即时呈现新会话
    void store.loadSessions()
  }

  function renameFromFloatingMenu() {
    const menu = activeSessionMenu.value
    if (!menu) return
    const name = store.sessions.find((s) => s.id === menu.id)?.name || ''
    openRename(menu.id, name)
    activeSessionMenu.value = null
  }

  function deleteFromFloatingMenu() {
    const menu = activeSessionMenu.value
    if (!menu) return
    void removeSession(menu.id)
  }

  async function onModelChange(modelId: string) {
    selectedModelId.value = modelId
    syncModelToLocal(modelId)
  }

  onMounted(() => {
    void boot()
    document.addEventListener('click', onGlobalClick)
  })

  watch(
    () => store.currentSessionId,
    (id) => {
      const targetPath = id ? `/chat/${id}` : '/chat/new_chat'
      if (route.path !== targetPath) {
        void router.replace(targetPath)
      }
    },
  )

  watch(messagesRef, (el, prev) => {
    if (prev) unbindScrollFade(prev)
    if (el) {
      bindScrollFade(el, () => {
        syncAutoStickToBottom(el)
      })
      syncAutoStickToBottom(el)
    }
  })

  watch(sidebarListRef, (el, prev) => {
    if (prev) unbindScrollFade(prev)
    if (el) bindScrollFade(el)
  })

  watch(
    () => store.connectionStatus,
    (status, prev) => {
      if (status === 'connected') {
        hasConnectedOnce.value = true
        suppressConnectionNoticeDisplay.value = false
        showInitialConnectionNotice.value = false
        clearInitialConnectionNoticeTimer()
        return
      }
      if (status === prev) return
      if (store.consumeSuppressNextConnectionNotice()) {
        suppressConnectionNoticeDisplay.value = true
        showInitialConnectionNotice.value = false
        clearInitialConnectionNoticeTimer()
        return
      }
      if (!hasConnectedOnce.value) {
        scheduleInitialConnectionNotice()
        return
      }
      if (suppressConnectionNoticeDisplay.value) return
      showWarning(t(status === 'reconnecting' ? 'networkReconnecting' : 'networkDisconnected'))
    },
  )

  watch(
    () => store.currentSessionId,
    () => {
      autoStickToBottom.value = true
      queueScrollMessagesToBottom(true)
    },
  )

  watch(
    () => store.messages.length,
    () => {
      queueScrollMessagesToBottom()
    },
  )

  watch(
    () => store.messages[store.messages.length - 1]?.content,
    () => {
      queueScrollMessagesToBottom()
    },
  )

  watch(
    () => [store.waiting, store.streamingStarted],
    () => {
      queueScrollMessagesToBottom()
    },
  )

  watch(
    () => store.replyBatches.length,
    () => {
      queueScrollMessagesToBottom()
    },
  )

  watch(
    () => {
      const batchId = store.currentBatchId
      if (!batchId) return 0
      const batch = store.replyBatches.find((b) => b.id === batchId)
      return batch?.timeline.length ?? 0
    },
    () => {
      queueScrollMessagesToBottom()
    },
  )

  onUnmounted(() => {
    clearInitialConnectionNoticeTimer()
    clearScrollToBottomPendingTimer()
    clearScrollToBottomEndHandler()
    unbindScrollFade(messagesRef.value)
    unbindScrollFade(sidebarListRef.value)
    scrollTimers.forEach((timer) => clearTimeout(timer))
    scrollTimers.clear()
    scrollHandlers.forEach((handler, el) => {
      el.removeEventListener('scroll', handler)
    })
    scrollHandlers.clear()
    store.disconnectSocket()
    document.removeEventListener('click', onGlobalClick)
  })

  return {
    t,
    store,
    drawerOpen,
    renameVisible,
    renameValue,
    inputValue,
    loading,
    isEmptySession,
    showScrollToBottom,
    settingsVisible,
    toolDetailVisible,
    toolDetailDialogWidth,
    activeSessionMenu,
    topMenuVisible,
    modelOptions,
    selectedModelId,
    setMessagesRef,
    currentSession,
    sendDisabled,
    networkStatusText,
    getReplyToolCount,
    getReplyToolSummary,
    getReplyTimeline,
    getReplyToolItem,
    shouldShowInlineToolCall,
    isReplyToolCollapsed,
    isEmptyPlaceholder,
    openToolDetail,
    toolDetailItems,
    toolDetailToolTimeline,
    modelSelectOptions,
    setSidebarListRef,
    toggleSidebar,
    toggleSessionMenu,
    refreshModelOptions,
    openRename,
    confirmRename,
    removeSession,
    confirmDeleteSession,
    deleteConfirmVisible,
    pickSession,
    createSession,
    sendMessage,
    scrollToBottomByButton,
    renameFromFloatingMenu,
    deleteFromFloatingMenu,
    onModelChange,
  }
}
