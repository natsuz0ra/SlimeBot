import { computed, onUnmounted, ref, watch } from 'vue'
import { useToast } from '@/composables/useToast'
import { useChatStore } from '@/stores/chat'

const INITIAL_CONNECTION_NOTICE_DELAY_MS = 1500

export function useHomeNetworkNotice(options: {
  t: (key: string, params?: Record<string, unknown>) => string
  store: ReturnType<typeof useChatStore>
  toast: ReturnType<typeof useToast>
}) {
  const { t, store, toast } = options

  const hasConnectedOnce = ref(false)
  const showInitialConnectionNotice = ref(false)
  const suppressConnectionNoticeDisplay = ref(false)
  const initialConnectionNoticeTimer = ref<number | null>(null)

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

  function showWarning(message: string) {
    toast.warning(message)
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

  onUnmounted(() => {
    clearInitialConnectionNoticeTimer()
  })

  return {
    networkStatusText,
  }
}
