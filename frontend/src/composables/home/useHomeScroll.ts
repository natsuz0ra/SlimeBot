import { computed, nextTick, onMounted, onUnmounted, ref, watch, type Ref } from 'vue'
import { useChatStore } from '@/stores/chat'
import { getLiveReplyContentSignature } from '@/utils/liveReplyTimeline'

const BOTTOM_STICK_THRESHOLD_PX = 32
const TOP_LOAD_THRESHOLD_PX = 200
const SIDEBAR_BOTTOM_LOAD_THRESHOLD_PX = 80
const SCROLL_TO_BOTTOM_PENDING_MAX_MS = 2000
const SIDEBAR_SESSION_ITEM_HEIGHT_PX = 38
const SIDEBAR_SCROLL_AREA_PADDING_PX = 8

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
  let messagesResizeObserver: ResizeObserver | null = null
  let observedMessagesContent: Element | null = null

  const showScrollToBottom = computed(() => !isEmptySession.value && !autoStickToBottom.value)

  function setMessagesRef(el: unknown) {
    messagesRef.value = (el as { $el?: HTMLElement } | null)?.$el ?? (el as HTMLElement | null)
  }

  function setSidebarListRef(el: unknown) {
    sidebarListRef.value = (el as { $el?: HTMLElement } | null)?.$el ?? (el as HTMLElement | null)
  }

  function syncSessionPageSizeFromSidebar(el: HTMLElement | null) {
    if (!el) return
    const visibleCount = Math.floor((el.clientHeight - SIDEBAR_SCROLL_AREA_PADDING_PX) / SIDEBAR_SESSION_ITEM_HEIGHT_PX)
    store.setSessionPageSize(visibleCount)
  }

  function onWindowResizeForSidebarPageSize() {
    syncSessionPageSizeFromSidebar(sidebarListRef.value)
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

  function unobserveMessagesContentSize() {
    if (messagesResizeObserver && observedMessagesContent) {
      messagesResizeObserver.unobserve(observedMessagesContent)
    }
    observedMessagesContent = null
  }

  function observeMessagesContentSize(el: HTMLElement | null) {
    unobserveMessagesContentSize()
    if (!el || typeof ResizeObserver === 'undefined') return
    if (!messagesResizeObserver) {
      messagesResizeObserver = new ResizeObserver(() => {
        if (!autoStickToBottom.value) return
        scrollMessagesToBottom()
      })
    }
    observedMessagesContent = el.firstElementChild ?? el
    messagesResizeObserver.observe(observedMessagesContent)
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

  function getCurrentSessionPlanConfirmationId() {
    const pendingPlanConfirmation = store.pendingPlanConfirmation
    if (!pendingPlanConfirmation) return ''
    return pendingPlanConfirmation.sessionId === store.currentSessionId
      ? pendingPlanConfirmation.planId
      : ''
  }

  function hasNewActionRequest(next: string[], previous: string[] | undefined) {
    if (!previous) return next.some(Boolean)
    return next.some((value, index) => value !== '' && value !== previous[index])
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
    const heightBeforeLoad = el.scrollHeight
    const loadingPromise = store.loadOlderMessages()
    await nextTick()
    const spinnerHeight = el.scrollHeight - heightBeforeLoad
    if (spinnerHeight > 0) el.scrollTop += spinnerHeight
    const heightWithSpinner = el.scrollHeight
    try {
      await loadingPromise
      await nextTick()
      const addedHeight = el.scrollHeight - heightWithSpinner
      if (addedHeight !== 0) el.scrollTop += addedHeight
    } finally {
      loadingOlderFromScroll.value = false
    }
  }

  async function tryFillOlderUntilScrollable() {
    const el = messagesRef.value
    if (!el || isEmptySession.value) return
    let safety = 0
    while (safety++ < 100) {
      if (el.scrollHeight > el.clientHeight) return
      if (!store.hasMoreHistory) return
      if (store.loadingOlderHistory || loadingOlderFromScroll.value) {
        await new Promise<void>((r) => requestAnimationFrame(() => r()))
        continue
      }
      await maybeLoadOlderMessages(el)
      await nextTick()
    }
  }

  async function maybeLoadMoreSessionsOnScroll(el: HTMLElement) {
    if (store.loadingMoreSessions || !store.hasMoreSessions) return
    const distanceToBottom = el.scrollHeight - (el.scrollTop + el.clientHeight)
    if (distanceToBottom > SIDEBAR_BOTTOM_LOAD_THRESHOLD_PX) return
    await store.loadMoreSessions()
  }

  watch(messagesRef, (el, prev) => {
    if (prev) unbindScrollFade(prev)
    if (prev) unobserveMessagesContentSize()
    if (el) {
      bindScrollFade(el, () => {
        syncAutoStickToBottom(el)
        void maybeLoadOlderMessages(el)
      })
      observeMessagesContentSize(el)
      syncAutoStickToBottom(el)
      void nextTick(() => {
        void tryFillOlderUntilScrollable()
      })
    }
  })

  watch(sidebarListRef, (el, prev) => {
    if (prev) unbindScrollFade(prev)
    if (el) {
      syncSessionPageSizeFromSidebar(el)
      bindScrollFade(el, () => {
        void maybeLoadMoreSessionsOnScroll(el)
      })
    }
  })

  onMounted(() => {
    window.addEventListener('resize', onWindowResizeForSidebarPageSize)
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
      void nextTick(() => {
        void tryFillOlderUntilScrollable()
      })
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

  watch(
    () => {
      const batchId = store.currentBatchId
      if (!batchId) return ''
      return getLiveReplyContentSignature(store.replyBatches.find((b) => b.id === batchId))
    },
    () => {
      queueScrollMessagesToBottom()
    },
  )

  watch(
    () => [
      store.pendingApprovalToolCallIds.join('|'),
      store.pendingQuestions?.toolCallId ?? '',
      getCurrentSessionPlanConfirmationId(),
    ],
    (next, previous) => {
      if (hasNewActionRequest(next, previous)) {
        queueScrollMessagesToBottom(true)
      }
    },
  )

  onUnmounted(() => {
    window.removeEventListener('resize', onWindowResizeForSidebarPageSize)
    clearScrollToBottomPendingTimer()
    clearScrollToBottomEndHandler()
    unobserveMessagesContentSize()
    messagesResizeObserver?.disconnect()
    messagesResizeObserver = null
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
