<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  mdiChevronDown,
  mdiCalendarClockOutline,
  mdiCogOutline,
  mdiDotsHorizontal,
  mdiMagnify,
  mdiFolderOutline,
  mdiMessageTextOutline,
  mdiSlack,
  mdiWechat,
  mdiPlus,
  mdiWeatherNight,
  mdiWeatherSunny,
} from '@mdi/js'
import type { SessionItem } from '@/api/chat'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import AppLogo from '@/components/ui/AppLogo.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import TruncationTooltip from '@/components/ui/TruncationTooltip.vue'
import { useChatStore } from '@/stores/chat'
import { groupSessionsByDirectory, type SessionGroup } from '@/utils/sessionGroups'
import { isMessagePlatformSessionId, listPlatformSessions, platformFromSessionId, platformSessionName } from '@/utils/messagePlatformSessions'

const INITIAL_GROUP_SESSION_COUNT = 6

const props = defineProps<{
  tasksActive?: boolean
  sessions: SessionItem[]
  currentSessionId?: string
  isDark: boolean
  hasUpdateNotice: boolean
  setSidebarListRef: (el: unknown) => void
}>()

const emit = defineEmits<{
  createSession: [workingDirectory?: string]
  pickSession: [sessionId: string]
  toggleSessionMenu: [sessionId: string, event: MouseEvent]
  toggleTheme: []
  openSettings: []
  openSearch: []
  openTasks: []
}>()

const { t } = useI18n()
const store = useChatStore()

const collapsed = ref<string[]>([])
const platformCollapsed = ref(false)
const groupSessionLimits = ref<Record<string, number>>({})
const groupedSessions = computed(() => groupSessionsByDirectory(props.sessions.filter((session) => !isMessagePlatformSessionId(session.id)), t('workspaceUnclassified')))
const platformSessions = computed(() => listPlatformSessions(props.sessions))
function platformIcon(id: string) {
  switch (platformFromSessionId(id)) {
    case 'slack': return mdiSlack
    case 'wechat': return mdiWechat
    default: return mdiMessageTextOutline
  }
}

function sessionLimit(path: string) {
  return groupSessionLimits.value[path] || INITIAL_GROUP_SESSION_COUNT
}

function visibleSessions(group: SessionGroup) {
  return group.sessions.slice(0, sessionLimit(group.path))
}

function isGroupExpanded(path: string) {
  return !collapsed.value.includes(path)
}

function showMoreSessions(group: SessionGroup) {
  groupSessionLimits.value[group.path] = Math.min(group.sessions.length, sessionLimit(group.path) + INITIAL_GROUP_SESSION_COUNT)
}

function showFewerSessions(path: string) {
  delete groupSessionLimits.value[path]
}

function toggleGroup(path: string) {
  const target = collapsed
  if (isGroupExpanded(path)) {
    target.value = [...target.value, path]
    showFewerSessions(path)
  } else {
    target.value = target.value.filter((item) => item !== path)
  }
}

</script>

<template>
  <aside class="sidebar-panel absolute inset-y-0 left-0 w-64 flex flex-col z-30 backdrop-blur-xl">
    <div class="sidebar-header flex items-center justify-between px-4 h-14">
      <div class="flex items-center gap-2.5 min-w-0">
        <AppLogo :size="36" />
        <span class="sb-text-primary text-lg font-semibold tracking-wide brand-tech-font truncate">SlimeBot</span>
      </div>

      <div class="flex items-center gap-1 flex-shrink-0">
        <button
          type="button"
          class="sb-text-muted w-8 h-8 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer group"
          @click="emit('createSession')"
        >
          <MdiIcon :path="mdiPlus" :size="20" class="group-hover:scale-110 transition-transform duration-150" />
        </button>
        <button
          type="button"
          class="sb-text-muted w-8 h-8 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer group"
          :aria-label="t('chatSearch')"
          :title="t('chatSearch')"
          @click="emit('openSearch')"
        >
          <MdiIcon :path="mdiMagnify" :size="20" class="group-hover:scale-110 transition-transform duration-150" />
        </button>
      </div>
    </div>

    <nav class="sidebar-primary-nav" :aria-label="t('scheduleNavigation')">
      <button type="button" class="sidebar-task-link" :class="{ active: tasksActive }" :aria-current="tasksActive ? 'page' : undefined" @click="emit('openTasks')">
        <MdiIcon :path="mdiCalendarClockOutline" :size="19" />
        <span>{{ t('scheduleSettings') }}</span>
      </button>
    </nav>
    <div :ref="setSidebarListRef" class="scroll-area flex-1 overflow-y-auto py-2 px-1">
      <section class="mb-1 px-0.5">
        <button
          type="button"
          class="workspace-group flex h-9 w-full min-w-0 items-center gap-1.5 rounded-lg px-1.5 text-left text-sm font-semibold cursor-pointer"
          :aria-expanded="!platformCollapsed"
          @click="platformCollapsed = !platformCollapsed"
        >
          <MdiIcon :path="mdiChevronDown" :size="13" class="flex-shrink-0 transition-transform duration-150" :class="platformCollapsed ? '-rotate-90' : ''" />
          <MdiIcon :path="mdiMessageTextOutline" :size="18" class="flex-shrink-0" />
          <TruncationTooltip inherit-group :text="t('messagePlatformSession')" wrapper-class="min-w-0 flex-1" content-class="text-sm font-semibold" />
          <span class="platform-badge rounded-md px-1.5 py-0.5 text-[10px] font-medium leading-none">IM</span>
        </button>
        <div v-if="!platformCollapsed" class="workspace-session-list ml-7 mt-0.5 space-y-0.5 border-l pl-2">
          <div
            v-for="item in platformSessions"
            :key="item.id"
            class="workspace-session-row group group/tip relative flex h-9 min-w-0 items-center rounded-lg"
            :class="item.id === currentSessionId ? 'workspace-session-row-active' : ''"
          >
            <button
              type="button"
              class="workspace-session-main relative flex h-full min-w-0 flex-1 items-center rounded-lg px-2 text-left text-sm cursor-pointer"
              :aria-current="item.id === currentSessionId ? 'page' : undefined"
              @click="emit('pickSession', item.id)"
            >
              <span v-if="item.id === currentSessionId" class="session-active-indicator absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-4 rounded-r-full" />
              <img v-if="platformFromSessionId(item.id) === 'telegram'" src="/im_icon/telegram.svg" alt="" class="mr-2 h-4 w-4 flex-shrink-0" />
              <MdiIcon v-else :path="platformIcon(item.id)" :size="16" class="mr-2 flex-shrink-0" />
              <TruncationTooltip inherit-group :text="platformSessionName(item.id, t('telegram'))" wrapper-class="min-w-0 flex-1" content-class="text-sm" />
            </button>
          </div>
        </div>
      </section>

      <section v-for="group in groupedSessions" :key="group.path || 'unclassified'" class="mb-1">
        <div class="group flex items-center gap-1 px-0.5 min-w-0">
          <button
            type="button"
            class="workspace-group flex h-9 flex-1 min-w-0 items-center gap-1.5 rounded-lg px-1.5 text-left text-sm font-semibold cursor-pointer"
            :title="group.path || t('workspaceUnclassified')"
            :aria-expanded="isGroupExpanded(group.path)"
            @click="toggleGroup(group.path)"
          >
            <MdiIcon :path="mdiChevronDown" :size="13" class="flex-shrink-0 transition-transform duration-150" :class="!isGroupExpanded(group.path) ? '-rotate-90' : ''" />
            <MdiIcon :path="mdiFolderOutline" :size="18" class="flex-shrink-0" />
            <span class="min-w-0 truncate">{{ group.name }}</span>
          </button>
          <button
            v-if="group.path"
            type="button"
            class="workspace-group-add w-6 h-6 flex-shrink-0 flex items-center justify-center rounded-md cursor-pointer opacity-0 group-hover:opacity-100 focus-visible:opacity-100 max-md:opacity-100"
            :title="t('workspaceNewChat')"
            :aria-label="t('workspaceNewChat')"
            @click="emit('createSession', group.path)"
          ><MdiIcon :path="mdiPlus" :size="15" /></button>
        </div>
        <div v-if="isGroupExpanded(group.path)" class="workspace-session-list ml-7 mt-0.5 space-y-0.5 border-l pl-2">
          <div
            v-for="item in visibleSessions(group)"
            :key="item.id"
            class="workspace-session-row group group/tip relative flex h-9 min-w-0 items-center rounded-lg"
            :class="item.id === currentSessionId ? 'workspace-session-row-active' : ''"
          >
            <button
              type="button"
              class="workspace-session-main relative flex h-full min-w-0 flex-1 items-center rounded-lg px-2 pr-9 text-left cursor-pointer"
              :aria-current="item.id === currentSessionId ? 'page' : undefined"
              @click="emit('pickSession', item.id)"
            >
              <span v-if="item.id === currentSessionId" class="session-active-indicator absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-4 rounded-r-full" />
              <TruncationTooltip inherit-group :text="item.name" wrapper-class="min-w-0 flex-1" content-class="text-sm" />
            </button>
            <span
              v-if="store.runningSessionIds.has(item.id) || store.unreadSessionIds.has(item.id)"
              class="session-activity absolute right-2.5 top-1/2 flex h-4 w-4 -translate-y-1/2 items-center justify-center pointer-events-none"
              role="img"
              :aria-label="t(store.runningSessionIds.has(item.id) ? 'sessionRunning' : 'sessionUnread')"
              :title="t(store.runningSessionIds.has(item.id) ? 'sessionRunning' : 'sessionUnread')"
            >
              <LoadingSpinner v-if="store.runningSessionIds.has(item.id)" size-class="w-3.5 h-3.5" class="session-running-spinner" aria-hidden="true" />
              <span v-else class="session-unread-dot h-1.5 w-1.5 rounded-full" />
            </span>
            <button
              type="button"
              class="workspace-session-menu absolute right-1 top-1/2 -translate-y-1/2 w-7 h-7 flex items-center justify-center rounded-md transition-colors duration-150 cursor-pointer flex-shrink-0"
              :aria-label="t('sessionMenu')"
              @click.stop="emit('toggleSessionMenu', item.id, $event as MouseEvent)"
            ><MdiIcon :path="mdiDotsHorizontal" :size="15" /></button>
          </div>
          <div v-if="group.sessions.length > INITIAL_GROUP_SESSION_COUNT" class="workspace-session-actions flex items-center gap-1">
            <button
              v-if="visibleSessions(group).length < group.sessions.length"
              type="button"
              class="workspace-session-more rounded-md px-3 py-1 text-xs cursor-pointer"
              @click="showMoreSessions(group)"
            >{{ t('workspaceShowMore') }}</button>
            <button
              v-else-if="sessionLimit(group.path) > INITIAL_GROUP_SESSION_COUNT"
              type="button"
              class="workspace-session-more rounded-md px-3 py-1 text-xs cursor-pointer"
              @click="showFewerSessions(group.path)"
            >{{ t('workspaceShowLess') }}</button>
          </div>
        </div>
      </section>

      <button
        v-if="store.hasMoreSessions"
        type="button"
        class="workspace-load-more flex w-full items-center justify-center gap-2 rounded-lg py-2 text-xs cursor-pointer disabled:cursor-default"
        :disabled="store.loadingMoreSessions"
        @click="store.loadMoreSessions()"
      >
        <svg v-if="store.loadingMoreSessions" class="loading-spinner-accent animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
        </svg>
        {{ t('workspaceLoadMoreSessions') }}
      </button>
    </div>

    <div class="sidebar-footer p-2">
      <div class="flex items-center gap-1">
        <button
          type="button"
          class="sb-text-muted w-9 h-9 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer flex-shrink-0"
          @click="emit('toggleTheme')"
        >
          <MdiIcon :path="isDark ? mdiWeatherSunny : mdiWeatherNight" :size="20" />
        </button>
        <button
          type="button"
          class="settings-action-btn flex-1 flex items-center gap-2.5 px-3 h-9 rounded-xl text-sm transition-all duration-150 cursor-pointer"
          @click="emit('openSettings')"
        >
          <MdiIcon :path="mdiCogOutline" :size="19" />
          <span class="font-medium">{{ t('settings') }}</span>
          <span v-if="hasUpdateNotice" class="update-notice-dot" aria-hidden="true" />
        </button>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.sidebar-primary-nav { padding: 10px 10px 4px; }
.sidebar-task-link { width: 100%; display: flex; align-items: center; gap: 10px; padding: 10px 12px; border-radius: 10px; font-size: 13px; font-weight: 550; color: var(--text-secondary); cursor: pointer; transition: background 150ms; }
.sidebar-task-link:hover { background: var(--primary-alpha-08); }
.sidebar-task-link.active { background: var(--primary-alpha-12); color: var(--sb-brand); }
.sidebar-task-link:focus-visible { outline: 2px solid var(--sb-brand); outline-offset: -2px; }

.workspace-group { color: var(--text-primary); transition: background 150ms ease, color 150ms ease; }
.workspace-group:hover, .workspace-group:focus-visible, .workspace-group-add:hover, .workspace-group-add:focus-visible { color: var(--text-primary); background: var(--primary-alpha-08); }
.workspace-group-add { color: var(--text-muted); }
.workspace-session-list { border-color: var(--primary-alpha-15); }
.workspace-session-row { color: var(--text-secondary); border: 1px solid transparent; transition: background 150ms ease, color 150ms ease, border-color 150ms ease; }
.workspace-session-row:hover, .workspace-session-row:focus-within { color: var(--text-primary); background: var(--primary-alpha-06); }
.workspace-session-row-active { color: var(--text-primary); background: var(--primary-alpha-12); border-color: var(--primary-alpha-15); }
.workspace-session-row-active:hover, .workspace-session-row-active:focus-within { background: var(--primary-alpha-15); }
.session-running-spinner.loading-spinner-accent { color: var(--text-muted); }
.session-unread-dot { background: var(--sb-brand); }
@media (prefers-reduced-motion: reduce) { .session-running-spinner { animation-duration: 2.5s; } }
.workspace-session-row-active .workspace-session-main { font-weight: 600; }
.workspace-session-main:focus-visible { outline: 2px solid var(--sb-brand); outline-offset: -2px; }
.workspace-session-menu { color: var(--text-muted); opacity: 0; }
.workspace-session-row:hover .workspace-session-menu, .workspace-session-menu:focus-visible { opacity: 1; }
.workspace-session-row:hover .session-activity, .workspace-session-row:has(.workspace-session-menu:focus-visible) .session-activity { opacity: 0; }
.workspace-session-menu:hover, .workspace-session-menu:focus-visible { color: var(--text-primary); background: var(--primary-alpha-10); }
.workspace-session-more, .workspace-load-more { color: var(--text-muted); transition: background 150ms ease, color 150ms ease; }
.workspace-session-more:hover, .workspace-session-more:focus-visible, .workspace-load-more:hover:not(:disabled), .workspace-load-more:focus-visible { color: var(--text-primary); background: var(--primary-alpha-08); }
.update-notice-dot {
  width: 7px;
  height: 7px;
  flex: 0 0 auto;
  border-radius: 999px;
  background: #ef4444;
  box-shadow: 0 0 0 2px var(--sidebar-bg);
}
</style>
