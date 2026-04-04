import { computed, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import type { RouteLocationNormalizedLoaded } from 'vue-router'
import type { ComposerTranslation } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useChatStore } from '@/stores/chat'

const CURSOR_BLINK_MS = 180
const CURSOR_BLINK_CYCLES = 2
const PUNCTUATION_PAUSE_MS = 140
const TYPING_BASE_MS = 58
const TYPING_FAST_MS = 42
const LOGIN_HOME_TRANSITION_TOKEN = 'slimebot:transition:login-home'
const HOME_ENTER_ANIMATION_MS = 460
const CHAT_SWITCH_ANIMATION_MS = 210
const FORCE_PASSWORD_DIALOG_DELAY_MS = 500

export function useHomeTransitions(options: {
  t: ComposerTranslation
  route: RouteLocationNormalizedLoaded
  store: ReturnType<typeof useChatStore>
  loading: Ref<boolean>
  isEmptySession: Ref<boolean>
  authStore: ReturnType<typeof useAuthStore>
}) {
  const { t, route, store, loading, isEmptySession, authStore } = options

  const titlePhase = ref<'cursor' | 'typing' | 'done'>('done')
  const displayedWelcomeTitle = ref('')
  const welcomeTimers: number[] = []
  const playHomeLoginEnter = ref(false)
  const playChatContentSwitch = ref(false)
  const chatContentSwitchDirection = ref<'forward' | 'backward'>('forward')
  const accountDialogVisible = ref(false)
  let homeEnterTimer: number | null = null
  let chatContentSwitchTimer: number | null = null
  let forcePasswordDialogTimer: number | null = null

  const fullWelcomeTitle = computed(() => t('welcomeTitle'))
  const isNewChatRoute = computed(() => {
    const routeSessionId = route.params.sessionId as string | undefined
    return !routeSessionId || routeSessionId === 'new_chat'
  })
  const showTypeCursor = computed(() => titlePhase.value !== 'done')
  const shouldAnimateWelcomeTitle = computed(() => isNewChatRoute.value && isEmptySession.value)
  const activeAssistantMessageId = computed(() => {
    const batchId = store.currentBatchId
    if (!batchId) return ''
    const batch = store.replyBatches.find((item) => item.id === batchId)
    return batch?.assistantMessageId || ''
  })

  function clearWelcomeTimers() {
    while (welcomeTimers.length > 0) {
      const timer = welcomeTimers.pop()
      if (typeof timer === 'number') {
        window.clearTimeout(timer)
      }
    }
  }

  function scheduleWelcomeTimeout(callback: () => void, delay: number) {
    const timer = window.setTimeout(callback, delay)
    welcomeTimers.push(timer)
  }

  function clearForcePasswordDialogTimer() {
    if (typeof forcePasswordDialogTimer === 'number') {
      window.clearTimeout(forcePasswordDialogTimer)
      forcePasswordDialogTimer = null
    }
  }

  function shouldReduceMotion() {
    return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
  }

  function clearChatContentSwitchTimer() {
    if (typeof chatContentSwitchTimer === 'number') {
      window.clearTimeout(chatContentSwitchTimer)
      chatContentSwitchTimer = null
    }
  }

  function resolveChatContentSwitchDirection(previousSessionId: string | undefined, nextSessionId: string | undefined) {
    if (!previousSessionId || !nextSessionId) return 'forward'
    const previousIndex = store.sessions.findIndex((item) => item.id === previousSessionId)
    const nextIndex = store.sessions.findIndex((item) => item.id === nextSessionId)
    if (previousIndex < 0 || nextIndex < 0 || previousIndex === nextIndex) return 'forward'
    return nextIndex > previousIndex ? 'forward' : 'backward'
  }

  function triggerChatContentSwitch(previousSessionId: string | undefined, nextSessionId: string | undefined) {
    if (loading.value || shouldReduceMotion()) return
    chatContentSwitchDirection.value = resolveChatContentSwitchDirection(previousSessionId, nextSessionId)
    clearChatContentSwitchTimer()
    playChatContentSwitch.value = false
    window.requestAnimationFrame(() => {
      playChatContentSwitch.value = true
      chatContentSwitchTimer = window.setTimeout(() => {
        playChatContentSwitch.value = false
        chatContentSwitchTimer = null
      }, CHAT_SWITCH_ANIMATION_MS)
    })
  }

  function hasLoginToHomeTransitionToken() {
    try {
      return sessionStorage.getItem(LOGIN_HOME_TRANSITION_TOKEN) === '1'
    } catch {
      return false
    }
  }

  function getTypingDelay(char: string) {
    if ('，。！？；：,.!?;:'.includes(char)) {
      return PUNCTUATION_PAUSE_MS
    }
    return /[a-zA-Z0-9]/.test(char) ? TYPING_FAST_MS : TYPING_BASE_MS
  }

  function runWelcomeTypewriter() {
    clearWelcomeTimers()
    const title = fullWelcomeTitle.value

    if (!title) {
      titlePhase.value = 'done'
      displayedWelcomeTitle.value = ''
      return
    }

    if (shouldReduceMotion()) {
      titlePhase.value = 'done'
      displayedWelcomeTitle.value = title
      return
    }

    displayedWelcomeTitle.value = ''
    titlePhase.value = 'cursor'

    const cursorDuration = CURSOR_BLINK_MS * CURSOR_BLINK_CYCLES * 2
    scheduleWelcomeTimeout(() => {
      titlePhase.value = 'typing'
      let currentIndex = 0

      const typeNext = () => {
        if (currentIndex >= title.length) {
          titlePhase.value = 'done'
          return
        }
        const char = title[currentIndex]
        if (typeof char !== 'string') {
          titlePhase.value = 'done'
          return
        }
        displayedWelcomeTitle.value += char
        currentIndex += 1
        scheduleWelcomeTimeout(typeNext, getTypingDelay(char))
      }

      typeNext()
    }, cursorDuration)
  }

  function isChatAssistantAvatarAnimated(messageId: string) {
    return store.waiting && activeAssistantMessageId.value !== '' && activeAssistantMessageId.value === messageId
  }

  function onAccountUpdated() {
    clearForcePasswordDialogTimer()
    authStore.markPasswordChanged()
    accountDialogVisible.value = false
  }

  watch(shouldAnimateWelcomeTitle, (active) => {
    if (active) {
      runWelcomeTypewriter()
      return
    }
    clearWelcomeTimers()
    titlePhase.value = 'done'
    displayedWelcomeTitle.value = fullWelcomeTitle.value
  }, { immediate: true })

  watch(fullWelcomeTitle, () => {
    if (shouldAnimateWelcomeTitle.value) {
      runWelcomeTypewriter()
      return
    }
    displayedWelcomeTitle.value = fullWelcomeTitle.value
  })

  watch(
    () => store.currentSessionId,
    (nextSessionId, previousSessionId) => {
      if (nextSessionId === previousSessionId) return
      triggerChatContentSwitch(previousSessionId, nextSessionId)
    },
  )

  watch(
    () => [loading.value, authStore.mustChangePassword] as const,
    ([isLoading, mustChangePassword]) => {
      if (isLoading || !mustChangePassword) {
        clearForcePasswordDialogTimer()
        accountDialogVisible.value = false
        return
      }
      clearForcePasswordDialogTimer()
      forcePasswordDialogTimer = window.setTimeout(() => {
        accountDialogVisible.value = true
        forcePasswordDialogTimer = null
      }, FORCE_PASSWORD_DIALOG_DELAY_MS)
    },
    { immediate: true },
  )

  onMounted(() => {
    if (shouldReduceMotion() || !hasLoginToHomeTransitionToken()) return
    playHomeLoginEnter.value = true
    homeEnterTimer = window.setTimeout(() => {
      playHomeLoginEnter.value = false
      homeEnterTimer = null
    }, HOME_ENTER_ANIMATION_MS)
  })

  onBeforeUnmount(() => {
    clearWelcomeTimers()
    clearChatContentSwitchTimer()
    clearForcePasswordDialogTimer()
    if (typeof homeEnterTimer === 'number') {
      window.clearTimeout(homeEnterTimer)
      homeEnterTimer = null
    }
  })

  return {
    titlePhase,
    displayedWelcomeTitle,
    showTypeCursor,
    playHomeLoginEnter,
    playChatContentSwitch,
    chatContentSwitchDirection,
    accountDialogVisible,
    isChatAssistantAvatarAnimated,
    onAccountUpdated,
  }
}
