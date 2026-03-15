<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  mdiAlert,
  mdiChevronDown,
  mdiClose,
  mdiCogOutline,
  mdiDeleteOutline,
  mdiDotsHorizontal,
  mdiMenu,
  mdiPencilOutline,
  mdiPlus,
  mdiSend,
  mdiWeatherNight,
  mdiWeatherSunny,
} from '@mdi/js'

import MdiIcon from '@/components/MdiIcon.vue'
import TypingDots from '@/components/chat/TypingDots.vue'
import SettingsPanel from '@/components/settings/SettingsPanel.vue'
import AccountEditDialog from '@/components/settings/AccountEditDialog.vue'
import ToolCallCard from '@/components/chat/ToolCallCard.vue'
import ToolExecutionDetailDialog from '@/components/chat/ToolExecutionDetailDialog.vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import SlimeBotLogo from '@/components/ui/SlimeBotLogo.vue'
import { renderMarkdown } from '@/utils/markdown'
import { useHomeChatPage } from '@/composables/useHomeChatPage'
import { useTheme } from '@/composables/useTheme'
import { useAuthStore } from '@/stores/auth'

const {
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
  modelSelectOptions,
  setMessagesRef,
  setSidebarListRef,
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
} = useHomeChatPage()

const { isDark, toggleTheme } = useTheme()
const authStore = useAuthStore()
const route = useRoute()
const accountDialogVisible = ref(false)

const CURSOR_BLINK_MS = 180
const CURSOR_BLINK_CYCLES = 2
const PUNCTUATION_PAUSE_MS = 140
const TYPING_BASE_MS = 58
const TYPING_FAST_MS = 42
const LOGIN_HOME_TRANSITION_TOKEN = 'slimebot:transition:login-home'
const HOME_ENTER_ANIMATION_MS = 460
const CHAT_SWITCH_ANIMATION_MS = 210
const FORCE_PASSWORD_DIALOG_DELAY_MS = 500

const titlePhase = ref<'cursor' | 'typing' | 'done'>('done')
const displayedWelcomeTitle = ref('')
const welcomeTimers: number[] = []
const playHomeLoginEnter = ref(false)
const playChatContentSwitch = ref(false)
const chatContentSwitchDirection = ref<'forward' | 'backward'>('forward')
let homeEnterTimer: number | null = null
let chatContentSwitchTimer: number | null = null
let forcePasswordDialogTimer: number | null = null

const fullWelcomeTitle = computed(() => t('welcomeTitle'))
const isNewChatRoute = computed(() => route.name === 'new-chat' || route.path === '/chat/new_chat')
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

onBeforeUnmount(() => {
  clearWelcomeTimers()
  clearChatContentSwitchTimer()
  clearForcePasswordDialogTimer()
  if (typeof homeEnterTimer === 'number') {
    window.clearTimeout(homeEnterTimer)
    homeEnterTimer = null
  }
})

onMounted(() => {
  if (shouldReduceMotion() || !hasLoginToHomeTransitionToken()) return
  playHomeLoginEnter.value = true
  homeEnterTimer = window.setTimeout(() => {
    playHomeLoginEnter.value = false
    homeEnterTimer = null
  }, HOME_ENTER_ANIMATION_MS)
})

function onAccountUpdated() {
  clearForcePasswordDialogTimer()
  authStore.markPasswordChanged()
  accountDialogVisible.value = false
}

function onTextareaKeydown(e: KeyboardEvent) {
  if (e.isComposing) return
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    sendMessage()
  }
}

function onTextareaInput(e: Event) {
  const el = e.target as HTMLTextAreaElement
  el.style.height = 'auto'
  el.style.height = `${el.scrollHeight}px`
}
</script>

<template>
  <div
    class="page-shell h-screen flex items-center justify-center p-2 sm:p-3 transition-colors duration-300"
    :class="{ 'home-login-entering': playHomeLoginEnter }"
  >
    <!-- 外层卡片容器 -->
    <div
      class="page-card relative w-full h-full rounded-2xl flex overflow-hidden"
    >

      <!-- ───── 侧边栏 ───── -->
      <Transition name="sidebar">
        <aside
          v-if="drawerOpen"
          class="sidebar-panel absolute inset-y-0 left-0 w-64 flex flex-col z-30 backdrop-blur-xl"
        >
          <!-- 侧边栏顶部：Logo + 新建按钮 -->
          <div class="sidebar-header flex items-center justify-between px-4 h-14">
            <!-- Logo -->
            <div class="flex items-center gap-2.5">
              <SlimeBotLogo :size="36" />
              <span class="text-primary text-lg font-semibold tracking-wide brand-tech-font">SlimeBot</span>
            </div>

            <div class="flex items-center gap-1">
              <!-- 新建会话 -->
              <button
                type="button"
                class="icon-muted w-8 h-8 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer group"
                @click="createSession"
              >
                <MdiIcon :path="mdiPlus" :size="20" class="group-hover:scale-110 transition-transform duration-150" />
              </button>
              <!-- 关闭侧边栏 -->
              <button
                type="button"
                class="icon-muted w-8 h-8 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer"
                @click="drawerOpen = false"
              >
                <MdiIcon :path="mdiClose" :size="20" />
              </button>
            </div>
          </div>

          <!-- 会话列表 -->
          <div :ref="setSidebarListRef" class="scroll-area flex-1 overflow-y-auto py-2 px-2">
            <div
              v-for="item in store.sessions"
              :key="item.id"
              class="group relative flex items-center gap-1 px-3 h-9 rounded-xl cursor-pointer transition-all duration-150 mb-0.5"
              :class="item.id === store.currentSessionId
                ? 'session-item-active'
                : 'session-item'"
              @click="pickSession(item.id)"
            >
              <!-- 激活指示条 -->
              <span
                v-if="item.id === store.currentSessionId"
                class="session-active-indicator absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 rounded-r-full"
              />
              <span class="text-primary flex-1 truncate text-sm">{{ item.name }}</span>
              <button
                type="button"
                class="icon-muted w-6 h-6 flex items-center justify-center rounded-md transition-colors duration-150 cursor-pointer opacity-0 group-hover:opacity-100 flex-shrink-0"
                :class="item.id === store.currentSessionId ? '!opacity-100' : ''"
                @click.stop="toggleSessionMenu(item.id, $event as MouseEvent)"
              >
                <MdiIcon :path="mdiDotsHorizontal" :size="15" />
              </button>
            </div>
          </div>

          <!-- 侧边栏底部：主题切换 + 设置 -->
          <div class="sidebar-footer p-2">
            <div class="flex items-center gap-1">
              <!-- 主题切换 -->
              <button
                type="button"
                class="icon-muted w-9 h-9 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer flex-shrink-0"
                @click="toggleTheme"
              >
                <MdiIcon :path="isDark ? mdiWeatherSunny : mdiWeatherNight" :size="20" />
              </button>
              <!-- 设置 -->
              <button
                type="button"
                class="settings-action-btn flex-1 flex items-center gap-2.5 px-3 h-9 rounded-xl text-sm transition-all duration-150 cursor-pointer"
                @click="settingsVisible = true"
              >
                <MdiIcon :path="mdiCogOutline" :size="19" />
                <span class="font-medium">{{ t('settings') }}</span>
              </button>
            </div>
          </div>
        </aside>
      </Transition>

      <!-- 侧边栏遮罩 -->
      <Transition name="mask-fade">
        <div
          v-if="drawerOpen"
          class="sidebar-mask absolute inset-0 z-20"
          @click="drawerOpen = false"
        />
      </Transition>

      <!-- ───── 主内容区 ───── -->
      <main class="relative z-0 flex-1 flex flex-col min-w-0">

        <!-- 顶栏 -->
        <header
          class="header-bar relative z-30 flex items-center justify-center h-14 flex-shrink-0 backdrop-blur-sm"
        >
          <!-- 左侧菜单按钮 -->
          <button
            type="button"
            class="icon-muted absolute left-3 w-9 h-9 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer"
            @click.stop="toggleSidebar"
          >
            <MdiIcon :path="mdiMenu" :size="19" />
          </button>

          <!-- 标题 + 下拉菜单 -->
          <button
            v-if="currentSession"
            type="button"
            class="flex items-center gap-1.5 px-3 py-1.5 rounded-xl transition-all duration-150 cursor-pointer max-w-[260px] header-title-btn"
            @click.stop="topMenuVisible = !topMenuVisible"
          >
            <span class="text-primary text-sm font-semibold truncate">{{ currentSession.name }}</span>
            <MdiIcon
              :path="mdiChevronDown"
              :size="14"
              class="icon-muted flex-shrink-0 transition-transform duration-200"
              :class="topMenuVisible ? 'rotate-180' : ''"
            />
          </button>

          <!-- 网络状态 badge -->
          <div
            v-if="networkStatusText"
            class="absolute right-3 flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium"
            :class="store.connectionStatus === 'reconnecting'
              ? 'status-warning'
              : 'status-error'"
          >
            <span class="w-1.5 h-1.5 rounded-full animate-pulse" :class="store.connectionStatus === 'reconnecting' ? 'bg-amber-400' : 'bg-red-400'" />
            {{ networkStatusText }}
          </div>

          <!-- 顶部下拉菜单 -->
          <Transition name="top-menu-pop">
            <div
              v-if="topMenuVisible"
              class="absolute top-[52px] left-1/2 -translate-x-1/2 w-40 rounded-xl py-1 overflow-hidden z-[85] top-menu-glass"
              @click.stop
            >
              <button
                v-if="currentSession"
                type="button"
                class="w-full flex items-center gap-2.5 px-3 h-9 text-sm transition-colors duration-150 cursor-pointer menu-item"
                @click="openRename(currentSession.id, currentSession.name); topMenuVisible = false"
              >
                <MdiIcon :path="mdiPencilOutline" :size="14" />
                <span>{{ t('rename') }}</span>
              </button>
              <button
                v-if="currentSession"
                type="button"
                class="w-full flex items-center gap-2.5 px-3 h-9 text-sm transition-colors duration-150 cursor-pointer menu-item-danger"
                @click="removeSession(currentSession.id)"
              >
                <MdiIcon :path="mdiDeleteOutline" :size="14" />
                <span>{{ t('delete') }}</span>
              </button>
            </div>
          </Transition>
        </header>

        <div
          class="chat-content-shell"
          :class="[
            playChatContentSwitch ? 'chat-content-shell--switching' : '',
            chatContentSwitchDirection === 'backward'
              ? 'chat-content-shell--from-left'
              : 'chat-content-shell--from-right',
          ]"
        >
          <!-- ───── 空会话：居中欢迎 + 输入框 ───── -->
          <template v-if="isEmptySession">
          <div class="flex-1 flex flex-col items-center justify-center px-4 pb-8">
            <!-- 加载中 -->
            <div v-if="loading" class="text-muted flex items-center gap-2 text-sm mb-8">
              <svg class="loading-spinner-accent animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
              </svg>
              {{ t('loading') }}
            </div>

            <template v-else>
              <!-- AI 渐变图标 -->
              <SlimeBotLogo :size="80" animated class="new-chat-logo mb-1.0 drop-shadow-lg" />

              <!-- 欢迎标题 -->
              <h2 class="text-2xl font-bold mb-2 text-center welcome-title" :aria-label="fullWelcomeTitle">
                {{ displayedWelcomeTitle }}
                <span
                  v-if="showTypeCursor"
                  class="welcome-cursor"
                  :class="titlePhase === 'cursor' ? 'welcome-cursor-pre' : 'welcome-cursor-typing'"
                  aria-hidden="true"
                >
                  |
                </span>
              </h2>
              <p class="text-muted text-sm mb-6 text-center">{{ t('welcomeSubtitle') }}</p>

              <!-- 居中输入框 -->
              <div class="w-full max-w-[640px]">
                <div class="input-container rounded-2xl overflow-hidden">
                  <textarea
                    v-model="inputValue"
                    class="textarea-primary w-full resize-none border-0 outline-none bg-transparent px-4 pt-3.5 pb-12 text-sm leading-relaxed min-h-[112px] max-h-[260px] overflow-y-auto"
                    :placeholder="t('inputPlaceholder')"
                    rows="1"
                    @keydown="onTextareaKeydown"
                    @input="onTextareaInput"
                  />
                  <div class="absolute bottom-2 left-3 right-3 flex items-center justify-between gap-2">
                    <AppSelect
                      :model-value="selectedModelId"
                      :options="modelSelectOptions"
                      :disabled="modelOptions.length === 0"
                      variant="ghost"
                      size="xs"
                      @update:model-value="onModelChange"
                    />
                    <button
                      type="button"
                      class="w-8 h-8 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer flex-shrink-0"
                      :class="sendDisabled ? 'send-btn-disabled' : 'send-btn'"
                      :disabled="sendDisabled"
                      @click="sendMessage"
                    >
                      <MdiIcon :path="mdiSend" :size="15" />
                    </button>
                  </div>
                </div>
              </div>
            </template>
          </div>
          </template>

          <!-- ───── 有消息：消息列表 + 底部输入框 ───── -->
          <template v-else>
          <!-- 消息区 -->
          <section
            :ref="setMessagesRef"
            class="messages-section scroll-area flex-1 overflow-y-auto px-4 py-6"
          >
            <div class="flex flex-col gap-5 max-w-[720px] mx-auto">
              <div
                v-for="item in store.messages"
                :key="item.id"
                class="flex message-animate"
                :class="[
                  item.role === 'assistant' ? 'gap-2' : 'gap-3',
                  item.role === 'user' ? 'flex-row-reverse' : 'flex-row',
                  item.role === 'user' && store.isFailedUserMessage(item.id)
                    ? 'items-end'
                    : (item.role === 'assistant' && isEmptyPlaceholder(item.id) && store.waiting
                        ? 'items-center'
                        : 'items-start'),
                ]"
              >
                <!-- AI 头像 -->
                <div
                  v-if="item.role === 'assistant'"
                  class="flex-shrink-0 w-10 h-10 flex items-center justify-center"
                >
                  <SlimeBotLogo
                    :size="40"
                    :animated="isChatAssistantAvatarAnimated(item.id)"
                    class="w-10 h-10 object-contain"
                    :class="isChatAssistantAvatarAnimated(item.id) ? 'chat-ai-avatar-animated' : 'chat-ai-avatar'"
                  />
                </div>

                <!-- 失败图标（用户消息） -->
                <div
                  v-if="item.role === 'user' && store.isFailedUserMessage(item.id)"
                  class="failed-user-icon flex-shrink-0"
                  :title="t('sendBlockedOffline')"
                >
                  <MdiIcon :path="mdiAlert" :size="15" />
                </div>

                <!-- 消息气泡 -->
                <div
                  class="text-sm leading-relaxed"
                  :class="[
                    item.role === 'user'
                      ? 'user-bubble max-w-[calc(100%-52px)] rounded-2xl rounded-tr-sm px-4 py-2.5'
                      : 'w-full',
                    item.role === 'assistant' && store.isAssistantErrorMessage(item.id)
                      ? 'error-bubble rounded-xl px-4 py-3'
                      : '',
                  ]"
                >
                  <template v-if="item.role === 'assistant'">
                    <!-- 工具调用摘要按钮 -->
                    <div v-if="getReplyToolCount(item.id) > 0" class="assistant-tool-summary-row mb-2.5">
                      <button
                        type="button"
                        class="tool-summary-btn inline-flex items-center gap-2 px-3 py-1.5 text-xs rounded-full transition-all duration-150 cursor-pointer max-w-full"
                        aria-haspopup="dialog"
                        :aria-label="`${t('toolExecutionDetailTitle')} - ${getReplyToolSummary(item.id)}`"
                        @click="openToolDetail(item.id)"
                      >
                        <svg class="w-3.5 h-3.5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                          <path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                          <path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                        </svg>
                        <span class="truncate max-w-[min(62vw,420px)]">{{ getReplyToolSummary(item.id) }}</span>
                      </button>
                    </div>

                    <!-- 工具内联列表 -->
                    <div
                      v-if="getReplyToolCount(item.id) > 0 && !isReplyToolCollapsed(item.id)"
                      class="flex flex-col gap-2 mb-3"
                    >
                      <template v-for="entry in getReplyTimeline(item.id)" :key="entry.id">
                        <div v-if="entry.kind === 'text'" class="bubble-markdown text-primary" v-html="renderMarkdown(entry.content)" />
                        <div
                          v-else-if="entry.kind === 'tool_start' && shouldShowInlineToolCall(item.id, entry.toolCallId)"
                          class="w-full"
                        >
                          <ToolCallCard
                            v-if="getReplyToolItem(item.id, entry.toolCallId)"
                            :item="getReplyToolItem(item.id, entry.toolCallId)!"
                            :show-preamble="false"
                            @approve="store.approveToolCall($event, true)"
                            @reject="store.approveToolCall($event, false)"
                          />
                        </div>
                      </template>
                    </div>

                    <!-- 普通消息内容 -->
                    <div
                      v-if="getReplyToolCount(item.id) === 0 || isReplyToolCollapsed(item.id)"
                      class="bubble-markdown text-primary"
                      v-html="renderMarkdown(item.content)"
                    />

                    <!-- 打字指示器 -->
                    <TypingDots v-if="isEmptyPlaceholder(item.id) && store.waiting" />
                  </template>

                  <template v-else>
                    {{ item.content }}
                  </template>
                </div>
              </div>
            </div>
          </section>

          <Transition name="scroll-bottom-fade">
            <div
              v-if="showScrollToBottom"
              class="pointer-events-none absolute right-6 bottom-[132px] z-20"
            >
              <button
                type="button"
                class="scroll-bottom-btn pointer-events-auto w-10 h-10 rounded-full inline-flex items-center justify-center cursor-pointer"
                aria-label="Scroll to bottom"
                @click="scrollToBottomByButton"
              >
                <span class="scroll-bottom-arrow" aria-hidden="true">↓</span>
              </button>
            </div>
          </Transition>

          <!-- 底部输入区 -->
          <footer
            class="composer-footer flex-shrink-0 px-4 py-3"
          >
            <div class="max-w-[680px] mx-auto">
              <div class="relative input-container rounded-2xl overflow-hidden">
                <textarea
                  v-model="inputValue"
                  class="textarea-primary w-full resize-none border-0 outline-none bg-transparent px-4 pt-3.5 pb-12 text-sm leading-relaxed min-h-[112px] max-h-[260px] overflow-y-auto"
                  :placeholder="t('inputPlaceholder')"
                  rows="1"
                  @keydown="onTextareaKeydown"
                  @input="onTextareaInput"
                />
                <div class="absolute bottom-2 left-3 right-3 flex items-center justify-between gap-2">
                  <AppSelect
                    :model-value="selectedModelId"
                    :options="modelSelectOptions"
                    :disabled="modelOptions.length === 0"
                    variant="ghost"
                    size="xs"
                    @update:model-value="onModelChange"
                  />
                  <button
                    type="button"
                    class="w-8 h-8 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer flex-shrink-0"
                    :class="sendDisabled ? 'send-btn-disabled' : 'send-btn'"
                    :disabled="sendDisabled"
                    @click="sendMessage"
                  >
                    <MdiIcon :path="mdiSend" :size="15" />
                  </button>
                </div>
              </div>
            </div>
          </footer>
          </template>
        </div>
      </main>
    </div>

    <!-- ───── 重命名弹窗 ───── -->
    <BaseDialog
      v-model:visible="renameVisible"
      :title="t('rename')"
      :confirm-text="t('confirm')"
      :cancel-text="t('cancel')"
      width="360px"
      @confirm="confirmRename"
    >
      <input
        v-model="renameValue"
        type="text"
        class="w-full px-3 py-2.5 text-sm rounded-xl outline-none transition-all duration-150 dialog-input"
        @keydown.enter="confirmRename"
      />
    </BaseDialog>

    <!-- ───── 删除确认弹窗 ───── -->
    <BaseDialog
      v-model:visible="deleteConfirmVisible"
      :title="t('delete')"
      :confirm-text="t('confirm')"
      :cancel-text="t('cancel')"
      :confirm-danger="true"
      width="360px"
      @confirm="confirmDeleteSession"
    >
      <p class="text-secondary text-sm">{{ t('confirmDelete') }}</p>
    </BaseDialog>

    <!-- ───── 工具调用详情弹窗 ───── -->
    <ToolExecutionDetailDialog
      v-model:visible="toolDetailVisible"
      :width="toolDetailDialogWidth"
      :items="toolDetailItems"
      :tool-timeline="toolDetailToolTimeline"
      @approve="store.approveToolCall($event, true)"
      @reject="store.approveToolCall($event, false)"
    />

    <!-- ───── 设置弹窗 ───── -->
    <Transition name="overlay-fade">
      <div
        v-if="settingsVisible"
        class="settings-overlay fixed inset-0 z-[100] flex items-center justify-center p-4 sm:p-6"
        @click.self="settingsVisible = false"
      >
        <div
          class="settings-modal settings-modal-size w-full rounded-2xl overflow-hidden"
          @click.stop
        >
          <SettingsPanel @close="settingsVisible = false" @llm-changed="refreshModelOptions" />
        </div>
      </div>
    </Transition>

    <!-- ───── 浮动会话菜单 ───── -->
    <Transition name="session-menu-pop">
      <div
        v-if="activeSessionMenu"
        class="floating-session-menu fixed z-[80] w-40 rounded-xl py-1 overflow-hidden"
        :style="{ left: `${activeSessionMenu.x}px`, top: `${activeSessionMenu.y}px` }"
        @click.stop
      >
        <button
          type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm transition-colors duration-150 cursor-pointer menu-item"
          @click="renameFromFloatingMenu"
        >
          <MdiIcon :path="mdiPencilOutline" :size="14" />
          <span>{{ t('rename') }}</span>
        </button>
        <button
          type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm transition-colors duration-150 cursor-pointer menu-item-danger"
          @click="deleteFromFloatingMenu"
        >
          <MdiIcon :path="mdiDeleteOutline" :size="14" />
          <span>{{ t('delete') }}</span>
        </button>
      </div>
    </Transition>

    <AccountEditDialog
      v-model:visible="accountDialogVisible"
      :force-mode="true"
      @success="onAccountUpdated"
    />
  </div>
</template>

<style scoped>
@import './home-page.css';
</style>
