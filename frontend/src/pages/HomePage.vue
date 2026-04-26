<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import {
  mdiDeleteOutline,
  mdiPencilOutline,
} from '@mdi/js'

import QuestionAnswerDrawer from '@/components/chat/QuestionAnswerDrawer.vue'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import ChatComposer from '@/components/chat/ChatComposer.vue'
import ChatMessageList from '@/components/chat/ChatMessageList.vue'
import HomeDialogs from '@/components/home/HomeDialogs.vue'
import HomeHeaderBar from '@/components/home/HomeHeaderBar.vue'
import HomeSidebar from '@/components/home/HomeSidebar.vue'
import AppLogo from '@/components/ui/AppLogo.vue'
import { provideChatContext } from '@/composables/chat/useChatContext'
import { useHomeChatPage } from '@/composables/home/useHomeChatPage'
import { useHomeTransitions } from '@/composables/home/useHomeTransitions'
import { useTheme } from '@/composables/useTheme'
import { useAuthStore } from '@/stores/auth'

const {
  t,
  store,
  drawerOpen,
  renameVisible,
  renameValue,
  inputValue,
  pendingFiles,
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
  stopDisabled,
  networkStatusText,
  isMessagePlatformSession,
  canManageCurrentSession,
  getReplyToolCount,
  getReplyToolSummary,
  getReplyTimeline,
  getReplyToolItem,
  getSubagentChildTools,
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
  stopMessage,
  onSelectFiles,
  removePendingFile,
  scrollToBottomByButton,
  renameFromFloatingMenu,
  deleteFromFloatingMenu,
  onModelChange,
  thinkingLevel,
  thinkingSelectOptions,
  onThinkingLevelChange,
  planMode,
  onPlanToggle,
} = useHomeChatPage()

const { isDark, toggleTheme } = useTheme()
const authStore = useAuthStore()
const route = useRoute()
const {
  titlePhase,
  displayedWelcomeTitle,
  showTypeCursor,
  playHomeLoginEnter,
  playChatContentSwitch,
  chatContentSwitchDirection,
  accountDialogVisible,
  isChatAssistantAvatarAnimated,
  onAccountUpdated,
} = useHomeTransitions({
  t,
  route,
  store,
  loading,
  isEmptySession,
  authStore,
})

provideChatContext({
  waiting: computed(() => store.waiting),
  planGenerating: computed(() => store.planGenerating),
  isStreamingMessage: store.isStreamingMessage,
  getReplyToolCount,
  getReplyToolSummary,
  getReplyTimeline,
  getReplyToolItem,
  getSubagentChildTools,
  shouldShowInlineToolCall,
  isReplyToolCollapsed,
  isEmptyPlaceholder,
  openToolDetail,
  approveToolCall: store.approveToolCall,
  isFailedUserMessage: store.isFailedUserMessage,
  isAssistantErrorMessage: store.isAssistantErrorMessage,
  isChatAssistantAvatarAnimated,
  sendBlockedOfflineText: computed(() => t('sendBlockedOffline')),
  toolExecutionDetailTitle: computed(() => t('toolExecutionDetailTitle')),
})
</script>

<template>
  <div
    class="page-shell h-screen flex items-center justify-center p-2 sm:p-3 transition-colors duration-300"
    :class="{ 'home-login-entering': playHomeLoginEnter }"
  >
    <!-- Outer card shell -->
    <div
      class="page-card relative w-full h-full rounded-2xl flex overflow-hidden"
    >

      <!-- ───── Sidebar ───── -->
      <Transition name="sidebar">
        <HomeSidebar
          v-if="drawerOpen"
          :sessions="store.sessions"
          :current-session-id="store.currentSessionId"
          :is-dark="isDark"
          :set-sidebar-list-ref="setSidebarListRef"
          @create-session="createSession"
          @pick-session="pickSession"
          @toggle-session-menu="toggleSessionMenu"
          @toggle-theme="toggleTheme"
          @open-settings="settingsVisible = true"
        />
      </Transition>

      <!-- Sidebar overlay -->
      <Transition name="mask-fade">
        <div
          v-if="drawerOpen"
          class="sidebar-mask absolute inset-0 z-20"
          @click="drawerOpen = false"
        />
      </Transition>

      <!-- ───── Main content ───── -->
      <main class="relative z-0 flex-1 flex flex-col min-w-0">

        <HomeHeaderBar
          :current-session="currentSession"
          :can-manage-current-session="canManageCurrentSession"
          :top-menu-visible="topMenuVisible"
          :network-status-text="networkStatusText"
          :connection-status="store.connectionStatus"
          @toggle-sidebar="toggleSidebar"
          @update:top-menu-visible="topMenuVisible = $event"
          @rename-current="currentSession && openRename(currentSession.id, currentSession.name)"
          @remove-current="currentSession && removeSession(currentSession.id)"
        />

        <div
          class="chat-content-shell"
          :class="[
            playChatContentSwitch ? 'chat-content-shell--switching' : '',
            chatContentSwitchDirection === 'backward'
              ? 'chat-content-shell--from-left'
              : 'chat-content-shell--from-right',
          ]"
        >
          <!-- ───── Empty session: welcome + composer ───── -->
          <template v-if="isEmptySession">
          <div class="flex-1 flex flex-col items-center justify-center px-4 pb-8">
            <!-- Loading -->
            <div v-if="loading" class="sb-text-muted flex items-center gap-2 text-sm mb-8">
              <svg class="loading-spinner-accent animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
              </svg>
              {{ t('loading') }}
            </div>

            <template v-else>
              <!-- AI gradient logo -->
              <AppLogo :size="80" animated class="new-chat-logo mb-1.0 drop-shadow-lg" />

              <!-- Welcome title -->
              <h2 v-if="!isMessagePlatformSession" class="text-2xl font-bold mb-2 text-center welcome-title">
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
              <p class="sb-text-muted text-sm mb-6 text-center">
                {{ isMessagePlatformSession ? t('messagePlatformEmptySubtitle') : t('welcomeSubtitle') }}
              </p>

              <!-- Centered composer -->
              <div v-if="!isMessagePlatformSession" class="w-full max-w-[640px]">
                <ChatComposer
                  v-model="inputValue"
                  :selected-model-id="selectedModelId"
                  :model-select-options="modelSelectOptions"
                  :selected-thinking-level="thinkingLevel"
                  :thinking-select-options="thinkingSelectOptions"
                  :model-options-count="modelOptions.length"
                  :send-disabled="sendDisabled"
                  :stop-disabled="stopDisabled"
                  :is-streaming="store.waiting"
                  :pending-files="pendingFiles"
                  :placeholder="t('inputPlaceholder')"
                  :plan-mode="planMode"
                  :plan-confirmation-visible="!!store.pendingPlanConfirmation"
                  @send="sendMessage"
                  @stop="stopMessage"
                  @files-change="onSelectFiles"
                  @remove-file="removePendingFile"
                  @model-change="onModelChange"
                @thinking-change="onThinkingLevelChange"
                @plan-toggle="onPlanToggle"
                @plan-execute="store.approvePlan(selectedModelId, t('planExecuteUserMessage'))"
                @plan-cancel="store.rejectPlan()"
                />
              </div>
            </template>
          </div>
          </template>

          <!-- ───── With messages: list + footer composer ───── -->
          <template v-else>
          <div class="chat-content-scroll flex min-h-0 flex-1 flex-col overflow-hidden">
          <ChatMessageList
            :messages="store.messages"
            :show-scroll-to-bottom="showScrollToBottom"
            :loading-older-history="store.loadingOlderHistory"
            :set-messages-ref="setMessagesRef"
            @scroll-to-bottom="scrollToBottomByButton"
          />
          </div>

          <!-- Footer composer -->
          <footer
            v-if="!isMessagePlatformSession"
            class="composer-footer flex-shrink-0 px-4 py-3"
          >
            <div class="max-w-[680px] mx-auto">
              <ChatComposer
                v-model="inputValue"
                :selected-model-id="selectedModelId"
                :model-select-options="modelSelectOptions"
                :selected-thinking-level="thinkingLevel"
                :thinking-select-options="thinkingSelectOptions"
                :model-options-count="modelOptions.length"
                :send-disabled="sendDisabled"
                :stop-disabled="stopDisabled"
                :is-streaming="store.waiting"
                :pending-files="pendingFiles"
                :placeholder="t('inputPlaceholder')"
                :plan-mode="planMode"
                :plan-confirmation-visible="!!store.pendingPlanConfirmation"
                @send="sendMessage"
                @stop="stopMessage"
                @files-change="onSelectFiles"
                @remove-file="removePendingFile"
                @model-change="onModelChange"
              @thinking-change="onThinkingLevelChange"
              @plan-toggle="onPlanToggle"
              @plan-execute="store.approvePlan(selectedModelId, t('planExecuteUserMessage'))"
              @plan-cancel="store.rejectPlan()"
              />
            </div>
          </footer>
          </template>
        </div>
      </main>
    </div>

    <!-- ───── Floating session menu ───── -->
    <Transition name="session-menu-pop">
      <div
        v-if="activeSessionMenu && canManageCurrentSession"
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

    <QuestionAnswerDrawer
      :visible="!!store.pendingQuestions"
      :questions="store.pendingQuestions?.questions ?? []"
      :tool-call-id="store.pendingQuestions?.toolCallId ?? ''"
      @submit="store.submitQuestionAnswers"
      @cancel="store.cancelQuestionAnswers"
    />

    <HomeDialogs
      v-model:rename-visible="renameVisible"
      v-model:rename-value="renameValue"
      v-model:delete-confirm-visible="deleteConfirmVisible"
      v-model:tool-detail-visible="toolDetailVisible"
      v-model:settings-visible="settingsVisible"
      v-model:account-dialog-visible="accountDialogVisible"
      :tool-detail-dialog-width="toolDetailDialogWidth"
      :tool-detail-items="toolDetailItems"
      :tool-detail-tool-timeline="toolDetailToolTimeline"
      @confirm-rename="confirmRename"
      @confirm-delete-session="confirmDeleteSession"
      @approve-tool-call="store.approveToolCall($event, true)"
      @reject-tool-call="store.approveToolCall($event, false)"
      @refresh-model-options="refreshModelOptions"
      @account-updated="onAccountUpdated"
    />

  </div>
</template>

<style>
@import './home-page.css';
</style>
