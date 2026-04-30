import { computed, reactive, watch } from 'vue'
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
  const currentSessionPlanConfirmationVisible = computed(() => (
    !!store.pendingPlanConfirmation &&
    store.pendingPlanConfirmation.sessionId === store.currentSessionId
  ))
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
      thinkingLevel: modelState.thinkingLevel,
      subagentModelId: modelState.subagentModelId,
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

  const ui = reactive({
    drawerOpen: uiState.drawerOpen,
    renameVisible: uiState.renameVisible,
    renameValue: uiState.renameValue,
    loading: uiState.loading,
    settingsVisible: uiState.settingsVisible,
    topMenuVisible: uiState.topMenuVisible,
    deleteConfirmVisible: uiState.deleteConfirmVisible,
    isEmptySession,
    toggleSidebar: uiState.toggleSidebar,
    toggleSessionMenu: uiState.toggleSessionMenu,
  })

  const models = reactive({
    modelOptions: modelState.modelOptions,
    selectedModelId: modelState.selectedModelId,
    modelSelectOptions: modelState.modelSelectOptions,
    thinkingLevel: modelState.thinkingLevel,
    thinkingSelectOptions: modelState.thinkingSelectOptions,
    subagentModelId: modelState.subagentModelId,
    subagentModelSelectOptions: modelState.subagentModelSelectOptions,
    refreshModelOptions: modelState.refreshModelOptions,
    onModelChange: modelState.onModelChange,
    onThinkingLevelChange: modelState.onThinkingLevelChange,
    onSubagentModelChange: modelState.onSubagentModelChange,
  })

  const sessions = reactive({
    currentSession: sessionActions.currentSession,
    activeSessionMenu: uiState.activeSessionMenu,
    canManageCurrentSession: sessionActions.canManageCurrentSession,
    isMessagePlatformSession: sessionActions.isMessagePlatformSession,
    setSidebarListRef: scrollState.setSidebarListRef,
    openRename: sessionActions.openRename,
    confirmRename: sessionActions.confirmRename,
    removeSession: sessionActions.removeSession,
    confirmDeleteSession: sessionActions.confirmDeleteSession,
    pickSession: sessionActions.pickSession,
    createSession: sessionActions.createSession,
    renameFromFloatingMenu: sessionActions.renameFromFloatingMenu,
    deleteFromFloatingMenu: sessionActions.deleteFromFloatingMenu,
  })

  const composer = reactive({
    inputValue: uiState.inputValue,
    pendingFiles: uiState.pendingFiles,
    sendDisabled,
    stopDisabled,
    currentSessionPlanConfirmationVisible,
    planMode: computed(() => store.planMode),
    sendMessage: sessionActions.sendMessage,
    stopMessage: sessionActions.stopMessage,
    onSelectFiles: sessionActions.onSelectFiles,
    removePendingFile: sessionActions.removePendingFile,
    onPlanToggle: store.togglePlanMode,
  })

  const tools = reactive({
    toolDetailVisible: toolDetailState.toolDetailVisible,
    toolDetailDialogWidth: toolDetailState.toolDetailDialogWidth,
    toolDetailItems: toolDetailState.toolDetailItems,
    toolDetailToolTimeline: toolDetailState.toolDetailToolTimeline,
    getReplyToolCount: toolDetailState.getReplyToolCount,
    getReplyToolSummary: toolDetailState.getReplyToolSummary,
    getReplyTimeline: toolDetailState.getReplyTimeline,
    getVisibleReplyTimeline: toolDetailState.getVisibleReplyTimeline,
    getReplyToolItem: toolDetailState.getReplyToolItem,
    getSubagentChildTools: toolDetailState.getSubagentChildTools,
    shouldShowInlineToolCall: toolDetailState.shouldShowInlineToolCall,
    isReplyToolCollapsed: toolDetailState.isReplyToolCollapsed,
    toggleReplyCollapsed: toolDetailState.toggleReplyCollapsed,
    getReplyElapsedMs: toolDetailState.getReplyElapsedMs,
    shouldShowReplyCollapseBar: toolDetailState.shouldShowReplyCollapseBar,
    isEmptyPlaceholder: toolDetailState.isEmptyPlaceholder,
    openToolDetail: toolDetailState.openToolDetail,
  })

  const network = reactive({
    networkStatusText: networkState.networkStatusText,
  })

  const scroll = reactive({
    showScrollToBottom: scrollState.showScrollToBottom,
    setMessagesRef: scrollState.setMessagesRef,
    scrollToBottomByButton: scrollState.scrollToBottomByButton,
  })

  return {
    t,
    store,
    ui,
    models,
    sessions,
    composer,
    tools,
    network,
    scroll,
  }
}
