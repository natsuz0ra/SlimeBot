import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useToast } from '@/composables/useToast'
import { MESSAGE_PLATFORM_SESSION_ID } from '@/api/chat'
import { useHomeModelSelector } from '@/composables/home/useHomeModelSelector'
import { useHomeNetworkNotice } from '@/composables/home/useHomeNetworkNotice'
import { useHomeScroll } from '@/composables/home/useHomeScroll'
import { useHomeSessionActions } from '@/composables/home/useHomeSessionActions'
import { useHomeToolDetail } from '@/composables/home/useHomeToolDetail'
import { useHomeUiState } from '@/composables/home/useHomeUiState'
import { useChatStore } from '@/stores/chat'

export function useHomeChatPage() {
  const { t } = useI18n()
  const store = useChatStore()
  const toast = useToast()
  const uiState = useHomeUiState()
  const modelState = useHomeModelSelector()
  const isEmptySession = computed(() => !uiState.loading.value && store.messages.length === 0)
  const scrollState = useHomeScroll({
    store,
    isEmptySession,
  })
  const toolDetailState = useHomeToolDetail({
    t: (key, params) => t(key, params as never),
    store,
  })
  const isMessagePlatformSession = computed(() => store.currentSessionId === MESSAGE_PLATFORM_SESSION_ID)
  const canSend = computed(() => {
    if (isMessagePlatformSession.value) return false
    const hasInput = uiState.inputValue.value.trim() !== '' || uiState.pendingFiles.value.length > 0
    return modelState.hasModel.value && !!modelState.selectedModelId.value && hasInput && !store.waiting && store.isSocketReady
  })
  const sendDisabled = computed(() => !canSend.value)
  const stopDisabled = computed(() => !store.waiting || !store.isSocketReady)
  const sessionActions = useHomeSessionActions({
    t: (key, params) => t(key, params as never),
    store,
    toast,
    uiState: {
      drawerOpen: uiState.drawerOpen,
      renameVisible: uiState.renameVisible,
      renameValue: uiState.renameValue,
      renameTargetId: uiState.renameTargetId,
      inputValue: uiState.inputValue,
      pendingFiles: uiState.pendingFiles,
      loading: uiState.loading,
      activeSessionMenu: uiState.activeSessionMenu,
      topMenuVisible: uiState.topMenuVisible,
      deleteConfirmVisible: uiState.deleteConfirmVisible,
      deleteTargetId: uiState.deleteTargetId,
    },
    modelState: {
      selectedModelId: modelState.selectedModelId,
      refreshModelOptions: modelState.refreshModelOptions,
    },
    scrollState: {
      autoStickToBottom: scrollState.autoStickToBottom,
      scrollMessagesToBottom: scrollState.scrollMessagesToBottom,
      queueScrollMessagesToBottom: scrollState.queueScrollMessagesToBottom,
    },
    sendDisabled,
  })
  const networkState = useHomeNetworkNotice({
    t: (key, params) => t(key, params as never),
    store,
    toast,
  })

  watch(
    () => store.currentSessionId,
    (id) => {
      const targetPath = id ? `/chat/${id}` : '/chat/new_chat'
      if (sessionActions.route.path !== targetPath) {
        void sessionActions.router.replace(targetPath)
      }
    },
  )

  return {
    t,
    store,
    hasMoreHistory: store.hasMoreHistory,
    loadingOlderHistory: store.loadingOlderHistory,
    drawerOpen: uiState.drawerOpen,
    renameVisible: uiState.renameVisible,
    renameValue: uiState.renameValue,
    inputValue: uiState.inputValue,
    pendingFiles: uiState.pendingFiles,
    loading: uiState.loading,
    isEmptySession,
    showScrollToBottom: scrollState.showScrollToBottom,
    settingsVisible: uiState.settingsVisible,
    toolDetailVisible: toolDetailState.toolDetailVisible,
    toolDetailDialogWidth: toolDetailState.toolDetailDialogWidth,
    activeSessionMenu: uiState.activeSessionMenu,
    topMenuVisible: uiState.topMenuVisible,
    modelOptions: modelState.modelOptions,
    selectedModelId: modelState.selectedModelId,
    setMessagesRef: scrollState.setMessagesRef,
    currentSession: sessionActions.currentSession,
    sendDisabled,
    stopDisabled,
    networkStatusText: networkState.networkStatusText,
    isMessagePlatformSession: sessionActions.isMessagePlatformSession,
    canManageCurrentSession: sessionActions.canManageCurrentSession,
    getReplyToolCount: toolDetailState.getReplyToolCount,
    getReplyToolSummary: toolDetailState.getReplyToolSummary,
    getReplyTimeline: toolDetailState.getReplyTimeline,
    getReplyToolItem: toolDetailState.getReplyToolItem,
    shouldShowInlineToolCall: toolDetailState.shouldShowInlineToolCall,
    isReplyToolCollapsed: toolDetailState.isReplyToolCollapsed,
    isEmptyPlaceholder: toolDetailState.isEmptyPlaceholder,
    openToolDetail: toolDetailState.openToolDetail,
    toolDetailItems: toolDetailState.toolDetailItems,
    toolDetailToolTimeline: toolDetailState.toolDetailToolTimeline,
    modelSelectOptions: modelState.modelSelectOptions,
    setSidebarListRef: scrollState.setSidebarListRef,
    toggleSidebar: uiState.toggleSidebar,
    toggleSessionMenu: uiState.toggleSessionMenu,
    refreshModelOptions: modelState.refreshModelOptions,
    openRename: sessionActions.openRename,
    confirmRename: sessionActions.confirmRename,
    removeSession: sessionActions.removeSession,
    confirmDeleteSession: sessionActions.confirmDeleteSession,
    deleteConfirmVisible: uiState.deleteConfirmVisible,
    pickSession: sessionActions.pickSession,
    createSession: sessionActions.createSession,
    sendMessage: sessionActions.sendMessage,
    stopMessage: sessionActions.stopMessage,
    onSelectFiles: sessionActions.onSelectFiles,
    removePendingFile: sessionActions.removePendingFile,
    scrollToBottomByButton: scrollState.scrollToBottomByButton,
    renameFromFloatingMenu: sessionActions.renameFromFloatingMenu,
    deleteFromFloatingMenu: sessionActions.deleteFromFloatingMenu,
    onModelChange: modelState.onModelChange,
  }
}
