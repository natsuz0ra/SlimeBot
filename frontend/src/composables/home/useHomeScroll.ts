import { computed, nextTick, onUnmounted, ref, watch, type Ref } from 'vue'
import { useChatStore } from '@/stores/chat'

const BOTTOM_STICK_THRESHOLD_PX = 32
const TOP_LOAD_THRESHOLD_PX = 24
const SCROLL_TO_BOTTOM_PENDING_MAX_MS = 2000

export function useHomeScroll(options: {
  store: ReturnType<typeof useChatStore>
  isEmptySession: Ref<boolean>
}) {
  const { store, isEmptySession } = options

  const messagesRef = ref<HTMLElement | null>(null)
  const sidebarListRef = ref<HTMLElement | null>(null)
  const autoStickToBottom = ref(true)
  const scrollToBottomPending = ref(false)
  const scrollToBottomPendingTimer = ref<number | null>(null)
  const scrollToBottomEndHandler = ref<(() => void) | null>(null)
  const loadingOlderFromScroll = ref(false)
  const scrollTimers = new Map<HTMLElement, ReturnType<typeof setTimeout>>()
  const scrollHandlers = new Map<HTMLElement, () => void>()

  const showScrollToBottom = computed(() => !isEmptySession.value && !autoStickToBottom.value)

  function setMessagesRef(el: unknown) {
    messagesRef.value = (el as { $el?: HTMLElement } | null)?.$el ?? (el as HTMLElement | null)
  }

  function setSidebarListRef(el: unknown) {
    sidebarListRef.value = (el as { $el?: HTMLElement } | null)?.$el ?? (el as HTMLElement | null)
  }

  function isNearBottom(el: HTMLElement, threshold = BOTTOM_STICK_THRESHOLD_PX) {
    const distanceToBottom = el.scrollHeight - (el.scrollTop + el.clientHeight)
    return distanceToBottom <= threshold
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

  async function maybeLoadOlderMessages(el: HTMLElement) {
    if (loadingOlderFromScroll.value) return
    if (store.loadingOlderHistory || !store.hasMoreHistory) return
    if (el.scrollTop > TOP_LOAD_THRESHOLD_PX) return
    loadingOlderFromScroll.value = true
    const previousScrollHeight = el.scrollHeight
    try {
      const loaded = await store.loadOlderMessages()
      if (!loaded) return
      await nextTick()
      const addedHeight = el.scrollHeight - previousScrollHeight
      if (addedHeight > 0) {
        el.scrollTop = el.scrollTop + addedHeight
      }
    } finally {
      loadingOlderFromScroll.value = false
    }
  }

  watch(messagesRef, (el, prev) => {
    if (prev) unbindScrollFade(prev)
    if (el) {
      bindScrollFade(el, () => {
        syncAutoStickToBottom(el)
        void maybeLoadOlderMessages(el)
      })
      syncAutoStickToBottom(el)
    }
  })

  watch(sidebarListRef, (el, prev) => {
    if (prev) unbindScrollFade(prev)
    if (el) bindScrollFade(el)
  })

  watch(
    () => store.currentSessionId,
    () => {
      autoStickToBottom.value = true
      loadingOlderFromScroll.value = false
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
  })

  return {
    autoStickToBottom,
    showScrollToBottom,
    setMessagesRef,
    setSidebarListRef,
    scrollMessagesToBottom,
    queueScrollMessagesToBottom,
    scrollToBottomByButton,
  }
}
