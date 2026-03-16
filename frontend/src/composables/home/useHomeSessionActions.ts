import { computed, nextTick, onMounted, onUnmounted, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { MESSAGE_PLATFORM_SESSION_ID, sessionAPI } from '@/api/chat'
import { useToast } from '@/composables/useToast'
import { useChatStore } from '@/stores/chat'

type UiState = {
  drawerOpen: Ref<boolean>
  renameVisible: Ref<boolean>
  renameValue: Ref<string>
  renameTargetId: Ref<string>
  inputValue: Ref<string>
  loading: Ref<boolean>
  activeSessionMenu: Ref<{ id: string; x: number; y: number } | null>
  topMenuVisible: Ref<boolean>
  deleteConfirmVisible: Ref<boolean>
  deleteTargetId: Ref<string>
}

type ModelState = {
  selectedModelId: Ref<string>
  refreshModelOptions: (useRemembered?: boolean) => Promise<void>
}

type ScrollState = {
  autoStickToBottom: Ref<boolean>
  scrollMessagesToBottom: (force?: boolean) => void
  queueScrollMessagesToBottom: (force?: boolean) => void
}

export function useHomeSessionActions(options: {
  t: (key: string, params?: Record<string, unknown>) => string
  store: ReturnType<typeof useChatStore>
  toast: ReturnType<typeof useToast>
  uiState: UiState
  modelState: ModelState
  scrollState: ScrollState
  sendDisabled: Ref<boolean>
}) {
  const { t, store, toast, uiState, modelState, scrollState, sendDisabled } = options

  const router = useRouter()
  const route = useRoute()

  const isMessagePlatformSession = computed(() => store.currentSessionId === MESSAGE_PLATFORM_SESSION_ID)
  const currentSession = computed(() => {
    const current = store.sessions.find((item) => item.id === store.currentSessionId)
    if (current) return current
    if (isMessagePlatformSession.value) {
      return {
        id: MESSAGE_PLATFORM_SESSION_ID,
        name: t('messagePlatformSession'),
        updatedAt: '',
      }
    }
    return undefined
  })
  const canManageCurrentSession = computed(() => !isMessagePlatformSession.value)

  function showWarning(message: string) {
    toast.warning(message)
  }

  function showError(message: string) {
    toast.error(message)
  }

  async function boot() {
    uiState.loading.value = true
    try {
      await modelState.refreshModelOptions(true)

      await store.loadSessions()
      const routeSessionId = route.params.sessionId as string | undefined
      const isNewChatRoute = route.name === 'new-chat' || route.name === 'home'
      if (routeSessionId && routeSessionId !== 'new_chat') {
        if (routeSessionId === MESSAGE_PLATFORM_SESSION_ID) {
          await store.selectSession(MESSAGE_PLATFORM_SESSION_ID)
        } else {
          const matched = store.sessions.find((s) => s.id === routeSessionId)
          if (matched) {
            await store.selectSession(matched.id)
          } else if (store.sessions.length > 0) {
            const first = store.sessions[0]
            if (first) await store.selectSession(first.id)
          } else {
            store.resetToNewSession()
          }
        }
      } else if (isNewChatRoute || store.sessions.length === 0) {
        store.resetToNewSession()
      } else {
        const first = store.sessions[0]
        if (first) await store.selectSession(first.id)
      }
      await nextTick()
      scrollState.scrollMessagesToBottom(true)
      store.connectSocket()
    } finally {
      uiState.loading.value = false
    }
  }

  function openRename(sessionId: string, oldName: string) {
    if (sessionId === MESSAGE_PLATFORM_SESSION_ID) return
    uiState.renameTargetId.value = sessionId
    uiState.renameValue.value = oldName
    uiState.renameVisible.value = true
  }

  async function confirmRename() {
    if (!uiState.renameTargetId.value || !uiState.renameValue.value.trim()) return
    await sessionAPI.rename(uiState.renameTargetId.value, uiState.renameValue.value.trim())
    await store.loadSessions()
    uiState.renameVisible.value = false
  }

  function removeSession(id: string) {
    if (id === MESSAGE_PLATFORM_SESSION_ID) return
    uiState.deleteTargetId.value = id
    uiState.deleteConfirmVisible.value = true
    uiState.activeSessionMenu.value = null
    uiState.topMenuVisible.value = false
  }

  async function confirmDeleteSession() {
    if (!uiState.deleteTargetId.value) return
    try {
      const isDeletingCurrent = uiState.deleteTargetId.value === store.currentSessionId
      await sessionAPI.remove(uiState.deleteTargetId.value)
      await store.loadSessions()
      if (isDeletingCurrent) {
        store.resetToNewSession()
      }
    } catch {
      showError('删除失败')
    } finally {
      uiState.deleteConfirmVisible.value = false
      uiState.deleteTargetId.value = ''
    }
  }

  async function pickSession(id: string) {
    if (id === MESSAGE_PLATFORM_SESSION_ID) {
      await store.selectSession(id)
      await nextTick()
      scrollState.scrollMessagesToBottom(true)
      uiState.drawerOpen.value = false
      return
    }
    await store.selectSession(id)
    await nextTick()
    scrollState.scrollMessagesToBottom(true)
    uiState.drawerOpen.value = false
  }

  async function createSession() {
    store.resetToNewSession()
    scrollState.autoStickToBottom.value = true
    scrollState.queueScrollMessagesToBottom(true)
    uiState.drawerOpen.value = false
    void router.push('/chat/new_chat')
  }

  async function sendMessage() {
    if (sendDisabled.value) return
    scrollState.autoStickToBottom.value = true
    scrollState.queueScrollMessagesToBottom(true)
    const sent = await store.sendMessage(uiState.inputValue.value.trim(), modelState.selectedModelId.value)
    if (!sent) {
      showWarning(t('sendBlockedOffline'))
      return
    }
    uiState.inputValue.value = ''
    void store.loadSessions()
  }

  function renameFromFloatingMenu() {
    const menu = uiState.activeSessionMenu.value
    if (!menu) return
    const name = store.sessions.find((s) => s.id === menu.id)?.name || ''
    openRename(menu.id, name)
    uiState.activeSessionMenu.value = null
  }

  function deleteFromFloatingMenu() {
    const menu = uiState.activeSessionMenu.value
    if (!menu) return
    void removeSession(menu.id)
  }

  onMounted(() => {
    void boot()
  })

  onUnmounted(() => {
    store.disconnectSocket()
  })

  return {
    currentSession,
    isMessagePlatformSession,
    canManageCurrentSession,
    openRename,
    confirmRename,
    removeSession,
    confirmDeleteSession,
    pickSession,
    createSession,
    sendMessage,
    renameFromFloatingMenu,
    deleteFromFloatingMenu,
    route,
    router,
  }
}
