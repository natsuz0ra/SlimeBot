import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'

import { llmAPI, sessionAPI, type LLMConfig, type ToolCallItem } from '../../../api'
import { useChatStore } from '../stores/chat'

export function useHomeChatPage() {
  const { t } = useI18n()
  const store = useChatStore()
  const MODEL_STORAGE_KEY = 'corner:selectedModelId'

  const drawerOpen = ref(false)
  const renameVisible = ref(false)
  const renameValue = ref('')
  const renameTargetId = ref('')
  const inputValue = ref('')
  const loading = ref(false)
  const settingsVisible = ref(false)
  const hasConnectedOnce = ref(false)
  const toolDetailVisible = ref(false)
  const toolDetailBatchId = ref('')
  const toolDetailDialogWidth = 'min(688px, calc(100vw - 36px))'

  const activeSessionMenu = ref<{ id: string; x: number; y: number } | null>(null)
  const topMenuVisible = ref(false)
  const modelOptions = ref<LLMConfig[]>([])
  const selectedModelId = ref('')
  const messagesRef = ref<HTMLElement | null>(null)

  function setMessagesRef(el: any) {
    messagesRef.value = (el?.$el ?? el) as HTMLElement | null
  }

  const currentSession = computed(() => store.sessions.find((item) => item.id === store.currentSessionId))
  const hasModel = computed(() => modelOptions.value.length > 0)
  const sendDisabled = computed(() => !hasModel.value || !selectedModelId.value || !store.currentSessionId || !inputValue.value.trim() || store.waiting || !store.isSocketReady)
  const networkStatusText = computed(() => {
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

  function getReplyToolCalls(messageId: string): ToolCallItem[] {
    return findReplyBatchByMessageId(messageId)?.toolCalls || []
  }

  function getReplyTimeline(messageId: string) {
    return findReplyBatchByMessageId(messageId)?.timeline || []
  }

  function getReplyToolItem(messageId: string, toolCallId: string) {
    return getReplyToolCalls(messageId).find((item) => item.toolCallId === toolCallId)
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
    MessagePlugin.warning({
      content: message,
      placement: 'top-right',
    })
  }

  function showError(message: string) {
    MessagePlugin.error({
      content: message,
      placement: 'top-right',
    })
  }

  function scrollMessagesToBottom() {
    const el = messagesRef.value
    if (!el) return
    el.scrollTop = el.scrollHeight
  }

  function queueScrollMessagesToBottom() {
    void nextTick(() => {
      scrollMessagesToBottom()
    })
  }

  async function boot() {
    loading.value = true
    try {
      await refreshModelOptions(true)

      await store.loadSessions()
      if (store.sessions.length === 0) {
        await store.createSession()
      } else {
        const first = store.sessions[0]
        if (first) await store.selectSession(first.id)
      }
      await nextTick()
      scrollMessagesToBottom()
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

  async function removeSession(id: string) {
    if (!window.confirm(t('confirmDelete'))) return
    try {
      await sessionAPI.remove(id)
      await store.loadSessions()
      const first = store.sessions[0]
      if (first) {
        await store.selectSession(first.id)
      } else {
        await store.createSession()
      }
    } catch {
      showError('删除失败')
    } finally {
      activeSessionMenu.value = null
      topMenuVisible.value = false
    }
  }

  async function pickSession(id: string) {
    await store.selectSession(id)
    await nextTick()
    scrollMessagesToBottom()
    drawerOpen.value = false
  }

  async function createSession() {
    await store.createSession()
    drawerOpen.value = false
  }

  async function sendMessage() {
    if (sendDisabled.value) return
    const sent = await store.sendMessage(inputValue.value.trim(), selectedModelId.value)
    if (!sent) {
      showWarning(t('sendBlockedOffline'))
      return
    }
    inputValue.value = ''
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
    () => store.connectionStatus,
    (status, prev) => {
      if (status === 'connected') {
        hasConnectedOnce.value = true
        return
      }
      if (status === prev || !hasConnectedOnce.value) return
      showWarning(t(status === 'reconnecting' ? 'networkReconnecting' : 'networkDisconnected'))
    },
  )

  watch(
    () => store.currentSessionId,
    () => {
      queueScrollMessagesToBottom()
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
    getReplyTimeline,
    getReplyToolItem,
    isReplyToolCollapsed,
    isEmptyPlaceholder,
    openToolDetail,
    toolDetailItems,
    toolDetailToolTimeline,
    toggleSidebar,
    toggleSessionMenu,
    refreshModelOptions,
    openRename,
    confirmRename,
    removeSession,
    pickSession,
    createSession,
    sendMessage,
    renameFromFloatingMenu,
    deleteFromFloatingMenu,
    onModelChange,
  }
}
