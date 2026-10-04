<script setup lang="ts">
import { computed, onMounted, ref, toRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  mdiDeleteOutline,
  mdiMenu,
  mdiCalendarClockOutline,
  mdiPencilOutline,
} from '@mdi/js'

import QuestionAnswerDrawer from '@/components/chat/QuestionAnswerDrawer.vue'
import TodoPanel from '@/components/chat/TodoPanel.vue'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import ChatComposer from '@/components/chat/ChatComposer.vue'
import CooperativePanel from '@/components/chat/CooperativePanel.vue'
import ChatMessageList from '@/components/chat/ChatMessageList.vue'
import HomeDialogs from '@/components/home/HomeDialogs.vue'
import HomeHeaderBar from '@/components/home/HomeHeaderBar.vue'
import ChatSearchDialog from '@/components/home/ChatSearchDialog.vue'
import ScheduledTasksWorkspace from '@/components/home/ScheduledTasksWorkspace.vue'
import HomeSidebar from '@/components/home/HomeSidebar.vue'
import AppLogo from '@/components/ui/AppLogo.vue'
import { provideChatContext } from '@/composables/chat/useChatContext'
import { useHomeChatPage } from '@/composables/home/useHomeChatPage'
import { useHomeTransitions } from '@/composables/home/useHomeTransitions'
import { useUpdateNotice } from '@/composables/useUpdateNotice'
import { useTheme } from '@/composables/useTheme'
import { useAuthStore } from '@/stores/auth'

const {
  t,
  store,
  ui,
  models,
  sessions,
  composer,
  tools,
  network,
  scroll,
} = useHomeChatPage()

const searchVisible = ref(false)
const router = useRouter()
const isTasksWorkspace = computed(() => route.name === 'tasks')
async function openTasks() {
  ui.topMenuVisible = false
  sessions.activeSessionMenu = null
  await router.push('/tasks')
  if (window.matchMedia('(max-width: 767px)').matches) ui.drawerOpen = false
}
async function pickChat(id: string) {
  await sessions.pickSession(id)
  await router.push(`/chat/${encodeURIComponent(id)}`)
}
async function pickSearchResult(hit: Parameters<typeof sessions.pickSearchResult>[0]) {
  await sessions.pickSearchResult(hit)
  await router.push(`/chat/${encodeURIComponent(hit.sessionId)}`)
}
const { isDark, toggleTheme } = useTheme()
const {
  hasUnreadUpdate,
  checkForUpdate,
  setUpdateCheckResult,
  markUpdateNoticeRead,
} = useUpdateNotice()
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
  loading: toRef(ui, 'loading'),
  isEmptySession: toRef(ui, 'isEmptySession'),
  authStore,
})

provideChatContext({
  waiting: computed(() => store.waiting),
  planGenerating: computed(() => store.planGenerating),
  latestEditableUserMessageId: computed(() => store.latestEditableUserMessageId),
  isStreamingMessage: store.isStreamingMessage,
  getReplyToolCount: tools.getReplyToolCount,
  getReplyToolSummary: tools.getReplyToolSummary,
  getReplyTimeline: tools.getReplyTimeline,
  getVisibleReplyTimeline: tools.getVisibleReplyTimeline,
  getReplyToolItem: tools.getReplyToolItem,
  getSubagentChildTools: tools.getSubagentChildTools,
  shouldShowInlineToolCall: tools.shouldShowInlineToolCall,
  isReplyToolCollapsed: tools.isReplyToolCollapsed,
  toggleReplyCollapsed: tools.toggleReplyCollapsed,
  getReplyElapsedMs: tools.getReplyElapsedMs,
  shouldShowReplyCollapseBar: tools.shouldShowReplyCollapseBar,
  isEmptyPlaceholder: tools.isEmptyPlaceholder,
  openToolDetail: tools.openToolDetail,
  approveToolCall: store.approveToolCall,
  approveAllPendingToolCalls: store.approveAllPendingToolCalls,
  rejectAllPendingToolCalls: store.rejectAllPendingToolCalls,
  sendEditedMessage: (messageId, content) => store.sendEditedMessage(messageId, content, models.selectedModelId, models.thinkingLevel, models.subagentModelId),
  isFailedUserMessage: store.isFailedUserMessage,
  isAssistantErrorMessage: store.isAssistantErrorMessage,
  isChatAssistantAvatarAnimated,
  sendBlockedOfflineText: computed(() => t('sendBlockedOffline')),
  toolExecutionDetailTitle: computed(() => t('toolExecutionDetailTitle')),
})

onMounted(() => {
  void checkForUpdate()
})
</script>

<template>
  <ChatSearchDialog :visible="searchVisible" :pick-result="pickSearchResult" @close="searchVisible = false" />
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
          v-if="ui.drawerOpen"
          :sessions="store.sessions"
          :current-session-id="isTasksWorkspace ? undefined : store.currentSessionId"
          :tasks-active="isTasksWorkspace"
          :is-dark="isDark"
          :has-update-notice="hasUnreadUpdate"
          :set-sidebar-list-ref="sessions.setSidebarListRef"
          @create-session="sessions.createSession"
          @pick-session="pickChat"
          @toggle-session-menu="ui.toggleSessionMenu"
          @toggle-theme="toggleTheme"
          @open-settings="ui.settingsVisible = true"
          @open-search="searchVisible = true"
          @open-tasks="openTasks"
        />
      </Transition>

      <!-- Sidebar overlay -->
      <Transition name="mask-fade">
        <div
          v-if="ui.drawerOpen"
          class="sidebar-mask absolute inset-0 z-20 md:hidden"
          @click="ui.drawerOpen = false"
        />
      </Transition>

      <!-- ───── Main content ───── -->
      <main class="relative z-0 flex-1 flex flex-col min-w-0">

        <header v-if="isTasksWorkspace" class="header-bar flex items-center gap-3 h-14 shrink-0 px-3">
          <button type="button" class="sb-text-muted w-9 h-9 flex items-center justify-center rounded-xl cursor-pointer" :aria-label="t('scheduleToggleSidebar')" @click="ui.toggleSidebar"><MdiIcon :path="mdiMenu" :size="19" /></button>
          <MdiIcon :path="mdiCalendarClockOutline" :size="18" class="sb-text-muted" />
          <span class="sb-text-primary text-sm font-semibold">{{ t('scheduleSettings') }}</span>
        </header>
        <div v-if="isTasksWorkspace" class="flex-1 min-h-0 overflow-y-auto">
          <ScheduledTasksWorkspace :model-options="models.modelSelectOptions" :initial-working-directory="store.currentWorkingDirectory" />
        </div>
        <template v-else>
        <HomeHeaderBar
          :current-session="sessions.currentSession"
          :can-manage-current-session="sessions.canManageCurrentSession"
          :top-menu-visible="ui.topMenuVisible"
          :network-status-text="network.networkStatusText"
          :connection-status="store.connectionStatus"
          @toggle-sidebar="ui.toggleSidebar"
          @update:top-menu-visible="ui.topMenuVisible = $event"
          @rename-current="sessions.currentSession && sessions.openRename(sessions.currentSession.id, sessions.currentSession.name)"
          @remove-current="sessions.currentSession && sessions.removeSession(sessions.currentSession.id)"
        />

        <CooperativePanel v-if="!isTasksWorkspace" :session-id="store.currentSessionId" @settled="id => !store.waiting && store.loadNewMessagesForSession(id)" />
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
          <template v-if="ui.isEmptySession">
          <div class="flex-1 flex flex-col items-center justify-center px-4 pb-8">
            <!-- Loading -->
            <div v-if="ui.loading" class="sb-text-muted flex items-center gap-2 text-sm mb-8">
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
              <h2 v-if="!sessions.isMessagePlatformSession" class="text-2xl font-bold mb-2 text-center welcome-title">
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
                {{ sessions.isMessagePlatformSession ? t('messagePlatformEmptySubtitle') : t('welcomeSubtitle') }}
              </p>

              <!-- Centered composer -->
              <div v-if="!sessions.isMessagePlatformSession" class="w-full max-w-[640px]">
                <ChatComposer
                  v-model="composer.inputValue"
                  :selected-model-id="models.selectedModelId"
                  :model-select-options="models.modelSelectOptions"
                  :selected-thinking-level="models.thinkingLevel"
                  :thinking-select-options="models.thinkingSelectOptions"
                  :selected-subagent-model-id="models.subagentModelId"
                  :subagent-model-select-options="models.subagentModelSelectOptions"
                  :model-options-count="models.modelOptions.length"
                  :send-disabled="composer.sendDisabled"
                  :stop-disabled="composer.stopDisabled"
                  :is-streaming="store.waiting"
                  :pending-files="composer.pendingFiles"
                  :context-usage="store.contextUsage"
                  :placeholder="t('inputPlaceholder')"
                  :working-directory="store.currentWorkingDirectory"
                  :workspace-editable="!store.currentSessionId && !store.creatingSession"
                  :plan-mode="composer.planMode"
                  :approval-mode="composer.approvalMode"
                  :plan-confirmation-visible="composer.currentSessionPlanConfirmationVisible"
                  @send="composer.sendMessage"
                  @stop="composer.stopMessage"
                  @files-change="composer.onSelectFiles"
                  @remove-file="composer.removePendingFile"
                  @model-change="models.onModelChange"
                  @thinking-change="models.onThinkingLevelChange"
                  @subagent-model-change="models.onSubagentModelChange"
                  @plan-toggle="composer.onPlanToggle"
                  @approval-mode-change="composer.onApprovalModeChange"
                  @workspace-change="store.setDraftWorkingDirectory"
                  @plan-execute="store.approvePlan(models.selectedModelId, t('planExecuteUserMessage'))"
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
            :focused-message-id="store.focusedMessageId"
            :has-newer-history="store.hasNewerHistory"
            :loading-newer-messages="store.loadingNewerMessages"
            :show-scroll-to-bottom="scroll.showScrollToBottom"
            :loading-older-history="store.loadingOlderHistory"
            :set-messages-ref="scroll.setMessagesRef"
            @scroll-to-bottom="scroll.scrollToBottomByButton"
            @load-newer="store.currentSessionId && store.loadNewMessagesForSession(store.currentSessionId)"
          />
          </div>

          <!-- Footer composer -->
          <footer
            v-if="!sessions.isMessagePlatformSession"
            class="composer-footer flex-shrink-0 px-4 py-3"
          >
            <div class="max-w-[680px] mx-auto">
              <ChatComposer
                v-model="composer.inputValue"
                :selected-model-id="models.selectedModelId"
                :model-select-options="models.modelSelectOptions"
                :selected-thinking-level="models.thinkingLevel"
                :thinking-select-options="models.thinkingSelectOptions"
                :selected-subagent-model-id="models.subagentModelId"
                :subagent-model-select-options="models.subagentModelSelectOptions"
                :model-options-count="models.modelOptions.length"
                :send-disabled="composer.sendDisabled"
                :stop-disabled="composer.stopDisabled"
                :is-streaming="store.waiting"
                :pending-files="composer.pendingFiles"
                :context-usage="store.contextUsage"
                :placeholder="t('inputPlaceholder')"
                :working-directory="store.currentWorkingDirectory"
                :workspace-editable="!store.currentSessionId && !store.creatingSession"
                :plan-mode="composer.planMode"
                :approval-mode="composer.approvalMode"
                :plan-confirmation-visible="composer.currentSessionPlanConfirmationVisible"
                @send="composer.sendMessage"
                @stop="composer.stopMessage"
                @files-change="composer.onSelectFiles"
                @remove-file="composer.removePendingFile"
                @model-change="models.onModelChange"
                @thinking-change="models.onThinkingLevelChange"
                @subagent-model-change="models.onSubagentModelChange"
                @plan-toggle="composer.onPlanToggle"
                @approval-mode-change="composer.onApprovalModeChange"
                @workspace-change="store.setDraftWorkingDirectory"
                @plan-execute="store.approvePlan(models.selectedModelId, t('planExecuteUserMessage'))"
                @plan-cancel="store.rejectPlan()"
              />
            </div>
          </footer>
          </template>
        </div>
        <TodoPanel
          :items="store.runtimeTodos"
          :note="store.runtimeTodoNote"
          :open="store.todoPanelOpen"
          @toggle="store.toggleTodoPanel"
        />
        </template>
      </main>
    </div>

    <!-- ───── Floating session menu ───── -->
    <Transition name="session-menu-pop">
      <div
        v-if="sessions.activeSessionMenu && sessions.canManageCurrentSession"
        class="floating-session-menu fixed z-[80] w-40 rounded-xl py-1 overflow-hidden"
        :style="{ left: `${sessions.activeSessionMenu.x}px`, top: `${sessions.activeSessionMenu.y}px` }"
        @click.stop
      >
        <button
          type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm transition-colors duration-150 cursor-pointer menu-item"
          @click="sessions.renameFromFloatingMenu"
        >
          <MdiIcon :path="mdiPencilOutline" :size="14" />
          <span>{{ t('rename') }}</span>
        </button>
        <button
          type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm transition-colors duration-150 cursor-pointer menu-item-danger"
          @click="sessions.deleteFromFloatingMenu"
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
      v-model:rename-visible="ui.renameVisible"
      v-model:rename-value="ui.renameValue"
      v-model:delete-confirm-visible="ui.deleteConfirmVisible"
      v-model:tool-detail-visible="tools.toolDetailVisible"
      v-model:settings-visible="ui.settingsVisible"
      v-model:account-dialog-visible="accountDialogVisible"
      :tool-detail-dialog-width="tools.toolDetailDialogWidth"
      :tool-detail-items="tools.toolDetailItems"
      :tool-detail-tool-timeline="tools.toolDetailToolTimeline"
      :has-update-notice="hasUnreadUpdate"
      :mark-update-notice-read="markUpdateNoticeRead"
      :set-update-check-result="setUpdateCheckResult"
      @confirm-rename="sessions.confirmRename"
      @confirm-delete-session="sessions.confirmDeleteSession"
      @approve-tool-call="store.approveToolCall($event, true)"
      @reject-tool-call="store.approveToolCall($event, false)"
      @refresh-model-options="models.refreshModelOptions"
      @account-updated="onAccountUpdated"
    />

  </div>
</template>

<style>
@import './home-page.css';
</style>
