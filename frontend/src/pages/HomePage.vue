<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, toRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { isAxiosError } from 'axios'
import { apiClient } from '@/api/client'
import { CalendarClock, Download, Folder, GitBranch, Menu, Paperclip, Pencil, Pin, PinOff, Terminal, Trash2 } from 'lucide-vue-next'

import QuestionAnswerDrawer from '@/components/chat/QuestionAnswerDrawer.vue'
import TodoPanel from '@/components/chat/TodoPanel.vue'
import ChatComposer from '@/components/chat/ChatComposer.vue'
import TerminalDrawer from '@/components/chat/TerminalDrawer.vue'
import SessionExportDialog from '@/components/chat/SessionExportDialog.vue'
import WorkspaceDrawer from '@/components/chat/WorkspaceDrawer.vue'
import ChatCheckpointDialog from '@/components/chat/ChatCheckpointDialog.vue'
import CooperativePanel from '@/components/chat/CooperativePanel.vue'
import ChatMessageList from '@/components/chat/ChatMessageList.vue'
import HomeDialogs from '@/components/home/HomeDialogs.vue'
import HomeHeaderBar from '@/components/home/HomeHeaderBar.vue'
import ChatSearchDialog from '@/components/home/ChatSearchDialog.vue'
import ScheduledTasksWorkspace from '@/components/home/ScheduledTasksWorkspace.vue'
import HomeSidebar from '@/components/home/HomeSidebar.vue'
import AppLogo from '@/components/ui/AppLogo.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import { provideChatContext } from '@/composables/chat/useChatContext'
import { useHomeChatPage } from '@/composables/home/useHomeChatPage'
import { useHomeShortcuts } from '@/composables/home/useHomeShortcuts'
import { useHomeTransitions } from '@/composables/home/useHomeTransitions'
import { useUpdateNotice } from '@/composables/useUpdateNotice'
import { useTheme } from '@/composables/useTheme'
import { useToast } from '@/composables/useToast'
import { useAuthStore } from '@/stores/auth'
import { useTerminalStore } from '@/stores/terminal'
import { extractTerminalCommand, canStageTerminalCommand } from '@/utils/terminalLink'
import { acpAPI } from '@/api/acp'
import { skillsAPI } from '@/api/skills'
import type { SessionItem } from '@/api/chat'
import type { SkillItem } from '@/types/settings'
import { getSlashCommands, type WorkspaceCommand } from '@/utils/slashCommands'
import { workspaceAPI, type WorkspaceGitStatus } from '@/api/workspace'
import type { SettingsTabKey } from '@/types/settings'

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
const shortcutsHelpVisible = ref(false)
const settingsInitialTab = ref<SettingsTabKey>('basic')
const composerRef = ref<InstanceType<typeof ChatComposer> | null>(null)
const terminalRef = ref<InstanceType<typeof TerminalDrawer>>()
const terminal = useTerminalStore()
const dragDepth = ref(0)
const route = useRoute()
const router = useRouter()
const isTasksWorkspace = computed(() => route.name === 'tasks')
const canAcceptChatDrop = computed(() => (
  !isTasksWorkspace.value
  && !sessions.isMessagePlatformSession
  && !composer.currentSessionPlanConfirmationVisible
))
const isDragOver = computed(() => dragDepth.value > 0 && canAcceptChatDrop.value)
const checkpointSessionId = ref<string>()
const checkpointSession = computed(() => {
  const session = sessions.currentSession
  if (sessions.canManageCurrentSession && session?.workingDirectory?.trim() && !isTasksWorkspace.value) return session
})
const privacyMappingTarget = ref<{ id: string; name: string; busy: boolean; error: string }>()
const announcePrivacyMappingCleared = ref(false)
const toast = useToast()
const exportSession = ref<SessionItem>()
const pinning = new Set<string>()
const skills = ref<SkillItem[]>([])
const slashCommands = computed(() => getSlashCommands(
  skills.value, command => t(`slashCommand.${command}`), name => t('slashSkillPrompt', { name }),
))
const floatingSession = computed(() => store.sessions.find(session => session.id === sessions.activeSessionMenu?.id))
const acpAgentName = computed(() => {
  const agentId = sessions.currentSession?.acpAgentId
  if (!agentId) return undefined
  return models.acpAgents.find(agent => agent.id === agentId)?.name || agentId
})
const resettingACPSession = ref(false)

async function resetACPSession() {
  const sessionId = store.currentSessionId
  if (!sessionId || resettingACPSession.value) return
  resettingACPSession.value = true
  try {
    await acpAPI.reset(sessionId)
    toast.success(t('acpRestartSuccess'))
    await store.loadSessions()
  } catch {
    toast.error(t('acpRestartFailed'))
  } finally {
    resettingACPSession.value = false
  }
}
const workspaceOpen = ref(false)
const workspaceTab = ref<'git' | 'files'>('git')
const workspaceSwitch = ref<HTMLButtonElement>()
const cooperative = ref<InstanceType<typeof CooperativePanel>>()
const gitStatus = ref<WorkspaceGitStatus>()
const gitLoading = ref(false)
const gitFailed = ref(false)
const workspaceDirectory = computed(() => sessions.currentSession?.workingDirectory?.trim() || '')
const workspaceName = computed(() => workspaceDirectory.value.split(/[\\/]/).filter(Boolean).pop() || workspaceDirectory.value)
const drawerMedia = window.matchMedia('(max-width: 1023px)')
const workspaceModal = ref(drawerMedia.matches)
const workspaceOverlay = computed(() => workspaceOpen.value && workspaceModal.value)
let statusRequest: AbortController | undefined

function updateWorkspaceMedia(event: MediaQueryListEvent) {
  workspaceModal.value = event.matches
}

async function refreshGitStatus() {
  statusRequest?.abort()
  const id = store.currentSessionId
  if (isTasksWorkspace.value || !id || !workspaceDirectory.value) return
  const request = new AbortController()
  statusRequest = request
  gitLoading.value = true
  gitFailed.value = false
  try {
    const status = await workspaceAPI.status(id, request.signal)
    if (!request.signal.aborted) gitStatus.value = status
  } catch {
    if (!request.signal.aborted) gitFailed.value = true
  } finally {
    if (!request.signal.aborted) gitLoading.value = false
  }
}

watch([() => store.currentSessionId, workspaceDirectory, isTasksWorkspace], () => {
  workspaceOpen.value = false
  gitStatus.value = undefined
  gitLoading.value = false
  gitFailed.value = false
  void refreshGitStatus()
}, { immediate: true })
watch(() => store.waiting, (waiting, wasWaiting) => {
  if (!waiting && wasWaiting) void refreshGitStatus()
})

async function closeWorkspace(restoreFocus = true) {
  workspaceOpen.value = false
  if (restoreFocus) {
    await nextTick()
    workspaceSwitch.value?.focus()
  }
}

function openWorkspace(toggle = false) {
  if (!workspaceDirectory.value || !store.currentSessionId) {
    toast.warning(t('slashNeedsWorkspace'))
    return
  }
  if (workspaceOpen.value && toggle) {
    void closeWorkspace()
    return
  }
  cooperative.value?.close()
  workspaceTab.value = gitStatus.value?.isRepo ? 'git' : 'files'
  workspaceOpen.value = true
}

async function quoteWorkspacePath(path: string) {
  const longest = Math.max(0, ...(path.match(/`+/g) ?? []).map(match => match.length))
  const fence = '`'.repeat(longest + 1)
  const text = path.replace(/[\r\n]/g, ' ')
  if (workspaceOverlay.value) await closeWorkspace(false)
  await appendToComposer(t('workspaceQuote', { path: `${fence} ${text} ${fence}` }))
}

async function refreshSlashSkills() {
  try {
    skills.value = await skillsAPI.list()
  } catch {
    // Keep the last loaded skills if the request fails.
  }
}

watch(() => ui.settingsVisible, visible => { if (!visible) void refreshSlashSkills() })

async function toggleSessionPin(session: SessionItem) {
  sessions.activeSessionMenu = null
  if (pinning.has(session.id)) return
  pinning.add(session.id)
  try { await store.setSessionPinned(session.id, !session.isPinned) }
  catch { toast.error(t('sessionPinFailed')) }
  finally { pinning.delete(session.id) }
}

function showSessionExport(session?: SessionItem) {
  sessions.activeSessionMenu = null
  if (session) exportSession.value = session
  else toast.warning(t('slashNeedsSession'))
}

function runSlashAction(command: WorkspaceCommand) {
  switch (command) {
    case 'terminal': void toggleTerminal(); break
    case 'plan': composer.onPlanToggle(); break
    case 'help': shortcutsHelpVisible.value = true; break
    case 'export': showSessionExport(sessions.currentSession); break
    case 'compact':
      if (store.currentSessionId) void store.compactContext(models.selectedModelId)
      else toast.warning(t('slashNeedsSession'))
      break
    case 'checkpoints':
      if (checkpointSession.value) checkpointSessionId.value = checkpointSession.value.id
      else toast.warning(t('slashNeedsWorkspace'))
      break
    case 'clear-privacy':
      if (sessions.currentSession) openPrivacyMappingConfirmation()
      else toast.warning(t('slashNeedsSession'))
      break
    case 'workspace': openWorkspace(); break
  }
}

function terminalError() {
  toast.error(t('terminalError'))
}

async function toggleTerminal() {
  if (terminal.isOpen) {
    terminal.isOpen = false
    await nextTick()
    composerRef.value?.focus()
    return
  }
  try {
    await terminal.openFor({ chatSessionId: store.currentSessionId || undefined, cwd: store.currentWorkingDirectory })
  } catch { terminalError() }
}

async function sendToTerminal(code: string, language: string) {
  const command = extractTerminalCommand(code, language)
  if (!canStageTerminalCommand(command, true)) {
    toast.error(t('terminalUnsafeCommand'))
    return
  }
  try {
    const tab = await terminal.openFor({ chatSessionId: store.currentSessionId || undefined, cwd: store.currentWorkingDirectory }, true)
    await nextTick()
    if (tab) terminalRef.value?.stage(tab.id, command)
  } catch { terminalError() }
}

async function appendToComposer(text: string) {
  terminal.isMaximized = false
  composer.inputValue = composer.inputValue ? `${composer.inputValue}\n\n${text}` : text
  await nextTick()
  composerRef.value?.focusAtEnd()
}

watch([() => store.currentSessionId, isTasksWorkspace], () => {
  checkpointSessionId.value = undefined
  privacyMappingTarget.value = undefined
  announcePrivacyMappingCleared.value = false
})

function openPrivacyMappingConfirmation() {
  const session = sessions.currentSession
  if (session && sessions.canManageCurrentSession && !isTasksWorkspace.value) {
    announcePrivacyMappingCleared.value = false
    privacyMappingTarget.value = { id: session.id, name: session.name, busy: false, error: '' }
  }
}

async function destroyPrivacyMappings() {
  const target = privacyMappingTarget.value
  if (!target || target.busy || target.id !== store.currentSessionId) return
  target.busy = true
  target.error = ''
  try {
    await apiClient.delete(`/api/sessions/${encodeURIComponent(target.id)}/privacy-mappings`)
    if (privacyMappingTarget.value === target) {
      privacyMappingTarget.value = undefined
      announcePrivacyMappingCleared.value = true
      toast.success(t('sessionPrivacyDestroyed'))
    }
  } catch (error) {
    if (privacyMappingTarget.value === target) {
      const message = isAxiosError<{ error?: string }>(error) ? error.response?.data?.error : undefined
      target.error = typeof message === 'string' && message.trim() ? message : t('sessionPrivacyDestroyFailed')
    }
  } finally {
    target.busy = false
  }
}

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
function openSettings(initialTab: SettingsTabKey = 'basic') {
  settingsInitialTab.value = initialTab
  ui.settingsVisible = true
}
async function openSessionFromSettings(sessionId: string) {
  ui.settingsVisible = false
  await pickChat(sessionId)
}
function onUsageBudgetSaved() {
  if (models.selectedModelId) {
    void store.refreshContextUsage(models.selectedModelId)
  }
}
function hasFileTransfer(event: DragEvent): boolean {
  const types = event.dataTransfer?.types
  if (!types) return false
  return Array.from(types).includes('Files')
}
function resetDragDepth() {
  dragDepth.value = 0
}
function onChatDragEnter(event: DragEvent) {
  if (!canAcceptChatDrop.value || !hasFileTransfer(event)) return
  event.preventDefault()
  dragDepth.value += 1
}
function onChatDragOver(event: DragEvent) {
  if (!canAcceptChatDrop.value || !hasFileTransfer(event)) return
  event.preventDefault()
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = store.waiting ? 'none' : 'copy'
  }
}
function onChatDragLeave(event: DragEvent) {
  if (!hasFileTransfer(event)) return
  dragDepth.value = Math.max(0, dragDepth.value - 1)
}
function onChatDrop(event: DragEvent) {
  if (!hasFileTransfer(event)) return
  event.preventDefault()
  dragDepth.value = 0
  if (!canAcceptChatDrop.value) return
  if (store.waiting) {
    toast.warning(t('uploadBlockedStreaming'))
    return
  }
  const dt = event.dataTransfer
  if (!dt) return
  const droppedFiles: File[] = []
  let hasDirectory = false
  if (dt.items && dt.items.length > 0) {
    for (let i = 0; i < dt.items.length; i += 1) {
      const item = dt.items[i]
      if (!item || item.kind !== 'file') continue
      const entry = (item as DataTransferItem & { webkitGetAsEntry?: () => { isDirectory?: boolean } | null }).webkitGetAsEntry?.()
      if (entry?.isDirectory) {
        hasDirectory = true
        continue
      }
      const file = item.getAsFile()
      if (file) droppedFiles.push(file)
    }
  } else if (dt.files && dt.files.length > 0) {
    droppedFiles.push(...Array.from(dt.files))
  }
  if (hasDirectory) {
    toast.warning(t('uploadFolderNotSupported'))
  }
  if (droppedFiles.length > 0) {
    composerRef.value?.addFiles(droppedFiles)
  }
}
function onWindowDragOver(event: DragEvent) {
  if (hasFileTransfer(event)) event.preventDefault()
}
function onWindowDrop(event: DragEvent) {
  if (hasFileTransfer(event)) event.preventDefault()
  resetDragDepth()
}
const { isDark, toggleTheme } = useTheme()
const {
  hasUnreadUpdate,
  checkForUpdate,
  setUpdateCheckResult,
  markUpdateNoticeRead,
} = useUpdateNotice()
const authStore = useAuthStore()
const {
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
  sendToTerminal,
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

const { isMac, isDesktop } = useHomeShortcuts({
  hasModalOpen: () => Boolean(
    searchVisible.value
    || ui.settingsVisible
    || shortcutsHelpVisible.value
    || ui.renameVisible
    || ui.deleteConfirmVisible
    || tools.toolDetailVisible
    || accountDialogVisible.value
    || checkpointSessionId.value
    || exportSession.value
    || workspaceOverlay.value
    || privacyMappingTarget.value
    || store.pendingQuestions,
  ),
  searchVisible: () => searchVisible.value,
  settingsVisible: () => ui.settingsVisible,
  shortcutsHelpVisible: () => shortcutsHelpVisible.value,
  hasPopoverOrMenuOpen: () => Boolean(ui.topMenuVisible || sessions.activeSessionMenu),
  editingMessage: () => false,
  pendingPlanConfirmation: () => composer.currentSessionPlanConfirmationVisible,
  isTasksWorkspace: () => isTasksWorkspace.value,
  isStreaming: () => store.waiting,
  stopDisabled: () => composer.stopDisabled,
  isMessagePlatformSession: () => sessions.isMessagePlatformSession,
  onAction: (action) => {
    switch (action) {
      case 'toggle-search':
        searchVisible.value = !searchVisible.value
        break
      case 'new-session':
        void sessions.createSession().then(() => nextTick(() => composerRef.value?.focus()))
        break
      case 'toggle-sidebar':
        ui.toggleSidebar()
        break
      case 'toggle-terminal':
        void toggleTerminal()
        break
      case 'toggle-settings':
        if (ui.settingsVisible) ui.settingsVisible = false
        else openSettings('basic')
        break
      case 'toggle-plan-mode':
        composer.onPlanToggle()
        break
      case 'stop-stream':
        composer.stopMessage()
        break
      case 'toggle-shortcuts-help':
        shortcutsHelpVisible.value = !shortcutsHelpVisible.value
        break
    }
  },
})

onMounted(() => {
  drawerMedia.addEventListener('change', updateWorkspaceMedia)
  void refreshSlashSkills()
  void terminal.refresh().catch(() => {})
  void checkForUpdate()
  window.addEventListener('dragover', onWindowDragOver)
  window.addEventListener('drop', onWindowDrop)
  window.addEventListener('dragend', resetDragDepth)
  window.addEventListener('blur', resetDragDepth)
})

onUnmounted(() => {
  drawerMedia.removeEventListener('change', updateWorkspaceMedia)
  statusRequest?.abort()
  window.removeEventListener('dragover', onWindowDragOver)
  window.removeEventListener('drop', onWindowDrop)
  window.removeEventListener('dragend', resetDragDepth)
  window.removeEventListener('blur', resetDragDepth)
})
</script>

<template>
  <SessionExportDialog v-if="exportSession" :session="exportSession" @close="exportSession = undefined" />
  <ChatSearchDialog :visible="searchVisible" :pick-result="pickSearchResult" @close="searchVisible = false" />
  <ChatCheckpointDialog
    v-if="checkpointSession?.workingDirectory"
    :key="checkpointSession.id"
    :visible="checkpointSessionId === checkpointSession.id"
    :session-id="checkpointSession.id"
    :working-directory="checkpointSession.workingDirectory"
    :turn-running="store.waiting"
    :workspace-running="store.waiting || store.backgroundTaskCount > 0"
    @close="checkpointSessionId = undefined"
    @restored="refreshGitStatus"
  />
  <AppDialog
    :visible="!!privacyMappingTarget"
    :title="t('sessionPrivacyDestroyTitle')"
    :confirm-text="t('sessionPrivacyDestroyConfirm')"
    :confirm-loading="privacyMappingTarget?.busy"
    :show-cancel="!privacyMappingTarget?.busy"
    :show-close="!privacyMappingTarget?.busy"
    :close-on-esc="!privacyMappingTarget?.busy"
    :close-on-mask="!privacyMappingTarget?.busy"
    confirm-danger
    @update:visible="!$event && !privacyMappingTarget?.busy && (privacyMappingTarget = undefined)"
    @confirm="destroyPrivacyMappings"
  >
    <p v-if="privacyMappingTarget" class="sb-text-secondary text-sm break-words">
      {{ t('sessionPrivacyDestroyHint', { name: privacyMappingTarget.name }) }}
    </p>
    <p v-if="privacyMappingTarget?.busy" class="sb-text-secondary mt-2 text-sm" role="status">{{ t('sessionPrivacyDestroying') }}</p>
    <p v-if="privacyMappingTarget?.error" class="mt-2 text-sm break-words [color:var(--tool-error-text)]" role="alert">{{ privacyMappingTarget.error }}</p>
  </AppDialog>
  <p class="sr-only" role="status">{{ announcePrivacyMappingCleared ? t('sessionPrivacyDestroyed') : '' }}</p>
  <div
    class="page-shell h-screen w-screen flex overflow-hidden transition-colors duration-200"
    :class="{ 'home-login-entering': playHomeLoginEnter }"
  >
    <!-- Full-bleed edge-to-edge workspace shell -->
    <div
      class="page-card relative w-full h-full flex overflow-hidden"
    >

      <!-- ───── Sidebar ───── -->
      <Transition name="sidebar">
        <HomeSidebar
          v-if="ui.drawerOpen"
          :inert="workspaceOverlay"
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
          @open-settings="openSettings('basic')"
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
      <main class="relative z-0 flex-1 flex flex-col min-w-0" :inert="workspaceOverlay">

        <header v-if="isTasksWorkspace" class="header-bar flex items-center gap-2.5 h-12 shrink-0 px-3">
          <button type="button" class="sb-text-muted w-8 h-8 flex items-center justify-center rounded-lg cursor-pointer" :aria-label="t('scheduleToggleSidebar')" @click="ui.toggleSidebar"><Menu :size="18" :stroke-width="1.75" aria-hidden="true" /></button>
          <CalendarClock :size="17" :stroke-width="1.75" class="sb-text-muted" aria-hidden="true" />
          <span class="sb-text-primary text-sm font-semibold">{{ t('scheduleSettings') }}</span>
        </header>
        <div v-if="isTasksWorkspace" class="sb-scrollbar flex-1 min-h-0 overflow-y-auto">
          <ScheduledTasksWorkspace :model-options="models.modelSelectOptions" :initial-working-directory="store.currentWorkingDirectory" />
        </div>
        <template v-else>
        <HomeHeaderBar
          :current-session="sessions.currentSession"
          :can-manage-current-session="sessions.canManageCurrentSession"
          :top-menu-visible="ui.topMenuVisible"
          :network-status-text="network.networkStatusText"
          :connection-status="store.connectionStatus"
          :acp-agent-name="acpAgentName"
          @toggle-sidebar="ui.toggleSidebar"
          @update:top-menu-visible="ui.topMenuVisible = $event"
          @rename-current="sessions.currentSession && sessions.openRename(sessions.currentSession.id, sessions.currentSession.name)"
          @pin-current="sessions.currentSession && toggleSessionPin(sessions.currentSession)"
          @export-current="showSessionExport(sessions.currentSession)"
          @remove-current="sessions.currentSession && sessions.removeSession(sessions.currentSession.id)"
          @open-checkpoints="checkpointSessionId = checkpointSession?.id"
          @clear-privacy-mappings="openPrivacyMappingConfirmation"
          @reset-acp-session="resetACPSession"
        >
          <template #right>
            <button
              v-if="workspaceDirectory" ref="workspaceSwitch" type="button" class="workspace-header-button"
              :aria-pressed="workspaceOpen" :aria-label="t('workspaceDrawer')" :title="workspaceDirectory"
              @click="openWorkspace(true)"
            >
              <component :is="gitStatus?.isRepo ? GitBranch : Folder" :size="15" :stroke-width="1.75" aria-hidden="true" />
              <code>{{ gitStatus?.isRepo ? gitStatus.branch : workspaceName }}</code>
              <span v-if="gitStatus?.changes.length" class="workspace-header-count">{{ gitStatus.changes.length }}{{ gitStatus.truncated ? '+' : '' }}</span>
            </button>
            <button type="button" class="terminal-header-button" :aria-pressed="terminal.isOpen" :title="t('terminalTitle') + (isMac ? ' (⌘`)' : ' (Ctrl+`)')" :aria-label="t('terminalTitle')" @click="toggleTerminal">
              <Terminal :size="17" :stroke-width="1.75" aria-hidden="true" />
              <span v-if="terminal.hasRunning && !terminal.isOpen" class="terminal-header-dot" />
            </button>
            <CooperativePanel
              ref="cooperative"
              header
              :session-id="store.currentSessionId"
              @opened="closeWorkspace(false)"
              @settled="id => { if (!store.waiting) { void store.loadNewMessagesForSession(id); void refreshGitStatus() } }"
            />
          </template>
        </HomeHeaderBar>
        <div
          v-show="!(terminal.isOpen && terminal.isMaximized)"
          class="chat-content-shell relative"
          :class="[
            playChatContentSwitch ? 'chat-content-shell--switching' : '',
            chatContentSwitchDirection === 'backward'
              ? 'chat-content-shell--from-left'
              : 'chat-content-shell--from-right',
          ]"
          @dragenter="onChatDragEnter"
          @dragover="onChatDragOver"
          @dragleave="onChatDragLeave"
          @drop="onChatDrop"
        >
          <Transition name="overlay-fade">
            <div v-if="isDragOver" class="chat-drop-overlay pointer-events-none">
              <div class="chat-drop-card">
                <Paperclip :size="24" :stroke-width="1.75" aria-hidden="true" />
                <div class="chat-drop-title">{{ store.waiting ? t('uploadDropBlockedStreaming') : t('uploadDropTitle') }}</div>
                <div v-if="!store.waiting" class="chat-drop-hint">{{ t('uploadTipLine2') }}</div>
              </div>
            </div>
          </Transition>
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
              <AppLogo :size="64" class="mb-1" />

              <!-- Welcome title -->
              <h2 v-if="!sessions.isMessagePlatformSession" class="text-xl font-semibold mb-1.5 text-center welcome-title">
                {{ t('welcomeTitle') }}
              </h2>
              <p class="sb-text-muted text-sm mb-6 text-center">
                {{ sessions.isMessagePlatformSession ? t('messagePlatformEmptySubtitle') : t('welcomeSubtitle') }}
              </p>

              <!-- Centered composer -->
              <div v-if="!sessions.isMessagePlatformSession" class="w-full max-w-[680px]">
                <ChatComposer
                  ref="composerRef"
                  v-model="composer.inputValue"
                  :selected-model-id="models.selectedModelId"
                  :model-select-options="models.engineSelectOptions"
                  :selected-thinking-level="models.thinkingLevel"
                  :thinking-select-options="models.thinkingSelectOptions"
                  :selected-subagent-model-id="models.subagentModelId"
                  :subagent-model-select-options="models.subagentModelSelectOptions"
                  :model-options-count="models.engineCount"
                  :send-disabled="composer.sendDisabled"
                  :stop-disabled="composer.stopDisabled"
                  :is-streaming="store.waiting"
                  :background-task-count="store.backgroundTaskCount"
                  :pending-files="composer.pendingFiles"
                  :context-usage="models.externalEngine ? null : store.contextUsage"
                :context-compacting="store.contextCompacting"
                  :placeholder="t('inputPlaceholder')"
                  :working-directory="store.currentWorkingDirectory"
                  :workspace-editable="!store.currentSessionId && !store.creatingSession"
                  :plan-mode="composer.planMode"
                  :approval-mode="composer.approvalMode"
                  :plan-confirmation-visible="composer.currentSessionPlanConfirmationVisible"
                    :slash-commands="slashCommands"
                  :external-engine="models.externalEngine"
                  @slash-action="runSlashAction"
                  @send="composer.sendMessage"
                  @stop="composer.stopMessage"
                @stop-background="store.stopBackgroundTasks"
                  @compact-context="store.compactContext(models.selectedModelId)"
                  @open-usage-settings="openSettings('usage')"
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
          <div class="chat-content-scroll relative flex min-h-0 flex-1 flex-col overflow-hidden">
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
              @jump-to-message="scroll.jumpToMessage"
            />
          </div>

          <!-- Footer composer -->
          <footer
            v-if="!sessions.isMessagePlatformSession"
            class="composer-footer flex-shrink-0 px-4 py-3"
          >
            <div class="max-w-[760px] mx-auto">
              <ChatComposer
                ref="composerRef"
                v-model="composer.inputValue"
                :selected-model-id="models.selectedModelId"
                :model-select-options="models.engineSelectOptions"
                :selected-thinking-level="models.thinkingLevel"
                :thinking-select-options="models.thinkingSelectOptions"
                :selected-subagent-model-id="models.subagentModelId"
                :subagent-model-select-options="models.subagentModelSelectOptions"
                :model-options-count="models.engineCount"
                :send-disabled="composer.sendDisabled"
                :stop-disabled="composer.stopDisabled"
                :is-streaming="store.waiting"
                :background-task-count="store.backgroundTaskCount"
                :pending-files="composer.pendingFiles"
                :context-usage="models.externalEngine ? null : store.contextUsage"
                :context-compacting="store.contextCompacting"
                :placeholder="t('inputPlaceholder')"
                :working-directory="store.currentWorkingDirectory"
                :workspace-editable="!store.currentSessionId && !store.creatingSession"
                :plan-mode="composer.planMode"
                :approval-mode="composer.approvalMode"
                :plan-confirmation-visible="composer.currentSessionPlanConfirmationVisible"
                :slash-commands="slashCommands"
                :external-engine="models.externalEngine"
                @slash-action="runSlashAction"
                @send="composer.sendMessage"
                @stop="composer.stopMessage"
                @stop-background="store.stopBackgroundTasks"
                @compact-context="store.compactContext(models.selectedModelId)"
                @open-usage-settings="openSettings('usage')"
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
        <TerminalDrawer
          ref="terminalRef"
          :session-id="store.currentSessionId || ''"
          :cwd="store.currentWorkingDirectory"
          :dark="isDark"
          :can-send-output="!sessions.isMessagePlatformSession"
          @output="appendToComposer"
          @error="terminalError"
          @staged="toast.success(t('terminalStaged'))"
        />
        <TodoPanel
          :items="store.runtimeTodos"
          :note="store.runtimeTodoNote"
          :open="store.todoPanelOpen"
          @toggle="store.toggleTodoPanel"
        />
        </template>
      </main>
      <WorkspaceDrawer
        v-if="workspaceOpen && store.currentSessionId && !isTasksWorkspace" v-model:tab="workspaceTab"
        :session-id="store.currentSessionId" :directory="workspaceDirectory"
        :status="gitStatus" :status-loading="gitLoading" :status-failed="gitFailed"
        :can-quote="!sessions.isMessagePlatformSession" :modal="workspaceModal"
        @close="closeWorkspace()" @refresh="refreshGitStatus" @quote="quoteWorkspacePath"
      />
    </div>

    <!-- ───── Floating session menu ───── -->
    <Transition name="session-menu-pop">
      <div
        v-if="sessions.activeSessionMenu && floatingSession"
        class="floating-session-menu fixed z-[80] w-40 rounded-xl py-1 overflow-hidden"
        :style="{ left: `${sessions.activeSessionMenu.x}px`, top: `${sessions.activeSessionMenu.y}px` }"
        @click.stop
      >
        <button
          type="button" class="w-full flex items-center gap-2.5 px-3 h-9 text-sm cursor-pointer menu-item"
          @click="toggleSessionPin(floatingSession)"
        >
          <component :is="floatingSession.isPinned ? PinOff : Pin" :size="14" :stroke-width="1.75" aria-hidden="true" />
          {{ t(floatingSession.isPinned ? 'sessionUnpin' : 'sessionPin') }}
        </button>
        <button
          type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm transition-colors duration-150 cursor-pointer menu-item"
          @click="sessions.renameFromFloatingMenu"
        >
          <Pencil :size="14" :stroke-width="1.75" aria-hidden="true" />
          <span>{{ t('rename') }}</span>
        </button>
        <button
          type="button" class="w-full flex items-center gap-2.5 px-3 h-9 text-sm cursor-pointer menu-item"
          @click="showSessionExport(floatingSession)"
        >
          <Download :size="14" :stroke-width="1.75" aria-hidden="true" />
          {{ t('sessionExport') }}
        </button>
        <button
          type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm border-t border-[var(--border-subtle)] transition-colors duration-150 cursor-pointer menu-item-danger"
          @click="sessions.deleteFromFloatingMenu"
        >
          <Trash2 :size="14" :stroke-width="1.75" aria-hidden="true" />
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
      v-model:shortcuts-help-visible="shortcutsHelpVisible"
      :settings-initial-tab="settingsInitialTab"
      :is-mac="isMac"
      :is-desktop="isDesktop"
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
      @open-session="openSessionFromSettings"
      @budget-saved="onUsageBudgetSaved"
      @account-updated="onAccountUpdated"
    />

  </div>
</template>

<style>
@import './home-page.css';

.chat-drop-overlay {
  position: absolute;
  inset: 12px;
  z-index: 40;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  background: color-mix(in srgb, var(--bg-main) 88%, transparent);
}

.chat-drop-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 24px 32px;
  border-radius: 12px;
  border: 1.5px dashed var(--sb-brand);
  background: var(--primary-alpha-06);
  color: var(--text-primary);
  text-align: center;
}

.chat-drop-title {
  font-size: 14px;
  font-weight: 650;
}

.chat-drop-hint {
  font-size: 12px;
  color: var(--text-muted);
}
</style>
