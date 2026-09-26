<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  mdiClose,
  mdiChevronDown,
  mdiCogOutline,
  mdiDotsHorizontal,
  mdiMagnify,
  mdiFolderOutline,
  mdiPlus,
  mdiWeatherNight,
  mdiWeatherSunny,
} from '@mdi/js'
import type { SessionItem } from '@/api/chat'
import { MESSAGE_PLATFORM_SESSION_ID } from '@/api/chat'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import AppLogo from '@/components/ui/AppLogo.vue'
import TruncationTooltip from '@/components/ui/TruncationTooltip.vue'
import { useChatStore } from '@/stores/chat'
import { groupSessionsByDirectory, type SessionGroup } from '@/utils/sessionGroups'

const INITIAL_GROUP_SESSION_COUNT = 6

const props = defineProps<{
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
}>()

const { t } = useI18n()
const store = useChatStore()

const searchOpen = ref(false)
const searchInput = ref('')
let searchDebounceTimer: number | null = null

const collapsed = ref<string[]>([])
const searchCollapsed = ref<string[]>([])
const groupSessionLimits = ref<Record<string, number>>({})
const groupedSessions = computed(() => groupSessionsByDirectory(props.sessions, t('workspaceUnclassified'), MESSAGE_PLATFORM_SESSION_ID))
const searching = computed(() => searchOpen.value && !!store.sessionSearchQuery.trim())

function sessionLimit(path: string) {
  return groupSessionLimits.value[path] || INITIAL_GROUP_SESSION_COUNT
}

function visibleSessions(group: SessionGroup) {
  return searching.value
    ? group.sessions
    : group.sessions.slice(0, sessionLimit(group.path))
}

function isGroupExpanded(path: string) {
  return searching.value ? !searchCollapsed.value.includes(path) : !collapsed.value.includes(path)
}

function showMoreSessions(group: SessionGroup) {
  groupSessionLimits.value[group.path] = Math.min(group.sessions.length, sessionLimit(group.path) + INITIAL_GROUP_SESSION_COUNT)
}

function showFewerSessions(path: string) {
  delete groupSessionLimits.value[path]
}

function toggleGroup(path: string) {
  const target = searching.value ? searchCollapsed : collapsed
  if (isGroupExpanded(path)) {
    target.value = [...target.value, path]
    showFewerSessions(path)
  } else {
    target.value = target.value.filter((item) => item !== path)
  }
}

watch(() => store.sessionSearchQuery, () => { searchCollapsed.value = [] })

function clearSearchDebounce() {
  if (searchDebounceTimer !== null) {
    window.clearTimeout(searchDebounceTimer)
    searchDebounceTimer = null
  }
}

function openSearch() {
  searchOpen.value = true
  searchInput.value = store.sessionSearchQuery
}

function closeSearch() {
  searchOpen.value = false
  searchInput.value = ''
  clearSearchDebounce()
  void store.searchSessions('')
}

watch(searchInput, (v) => {
  if (!searchOpen.value) return
  clearSearchDebounce()
  searchDebounceTimer = window.setTimeout(() => {
    searchDebounceTimer = null
    void store.searchSessions(v)
  }, 300)
})

onUnmounted(() => {
  clearSearchDebounce()
})
</script>

<template>
  <aside class="sidebar-panel absolute inset-y-0 left-0 w-64 flex flex-col z-30 backdrop-blur-xl">
    <div v-if="!searchOpen" class="sidebar-header flex items-center justify-between px-4 h-14">
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
          @click="openSearch"
        >
          <MdiIcon :path="mdiMagnify" :size="20" class="group-hover:scale-110 transition-transform duration-150" />
        </button>
      </div>
    </div>

    <div v-else class="sidebar-header flex items-center gap-2 px-3 h-14">
      <input
        v-model="searchInput"
        type="search"
        autocomplete="off"
        class="sidebar-search-input flex-1 min-w-0 h-9 px-2.5 rounded-lg text-sm outline-none transition-colors duration-150"
        :placeholder="t('searchSessionsPlaceholder')"
      />
      <button
        type="button"
        class="sb-text-muted w-8 h-8 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer flex-shrink-0"
        @click="closeSearch"
      >
        <MdiIcon :path="mdiClose" :size="20" />
      </button>
    </div>

    <div :ref="setSidebarListRef" class="scroll-area flex-1 overflow-y-auto py-2 px-1">
      <div
        class="group group/tip relative flex min-w-0 items-center gap-1 px-2 h-9 rounded-xl cursor-pointer transition-all duration-150 mb-3"
        :class="currentSessionId === MESSAGE_PLATFORM_SESSION_ID ? 'session-item-active' : 'session-item'"
        @click="emit('pickSession', MESSAGE_PLATFORM_SESSION_ID)"
      >
        <span
          v-if="currentSessionId === MESSAGE_PLATFORM_SESSION_ID"
          class="session-active-indicator absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 rounded-r-full"
        />
        <TruncationTooltip
          inherit-group
          :text="t('messagePlatformSession')"
          wrapper-class="min-w-0 flex-1"
          content-class="sb-text-primary text-sm"
        />
        <span class="text-[10px] px-1.5 py-0.5 rounded-md platform-badge">IM</span>
      </div>

      <section v-for="group in groupedSessions" :key="group.path || 'unclassified'" class="workspace-section mb-3">
        <div class="group flex items-center gap-1 px-0.5 min-w-0">
          <button
            type="button"
            class="workspace-group flex h-10 flex-1 min-w-0 items-center gap-2 rounded-lg px-1.5 text-left text-[15px] font-semibold cursor-pointer"
            :title="group.path || t('workspaceUnclassified')"
            :aria-expanded="isGroupExpanded(group.path)"
            @click="toggleGroup(group.path)"
          >
            <MdiIcon :path="mdiChevronDown" :size="13" class="flex-shrink-0 transition-transform duration-150" :class="!isGroupExpanded(group.path) ? '-rotate-90' : ''" />
            <MdiIcon :path="mdiFolderOutline" :size="19" class="flex-shrink-0" />
            <span class="min-w-0 truncate">{{ group.name }}</span>
          </button>
          <button
            v-if="group.path && !searchOpen"
            type="button"
            class="workspace-group-add w-6 h-6 flex-shrink-0 flex items-center justify-center rounded-md cursor-pointer opacity-0 group-hover:opacity-100 focus-visible:opacity-100 max-md:opacity-100"
            :title="t('workspaceNewChat')"
            :aria-label="t('workspaceNewChat')"
            @click="emit('createSession', group.path)"
          ><MdiIcon :path="mdiPlus" :size="15" /></button>
        </div>
        <div v-if="isGroupExpanded(group.path)" class="workspace-session-list ml-7 mt-1 space-y-0.5 border-l pl-2">
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
            <button
              type="button"
              class="workspace-session-menu absolute right-1 top-1/2 -translate-y-1/2 w-7 h-7 flex items-center justify-center rounded-md transition-colors duration-150 cursor-pointer opacity-0 group-hover:opacity-100 focus-visible:opacity-100 flex-shrink-0"
              :class="item.id === currentSessionId ? '!opacity-100' : ''"
              @click.stop="emit('toggleSessionMenu', item.id, $event as MouseEvent)"
            ><MdiIcon :path="mdiDotsHorizontal" :size="15" /></button>
          </div>
          <div v-if="!searching && group.sessions.length > INITIAL_GROUP_SESSION_COUNT" class="workspace-session-actions flex items-center gap-1">
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
        v-if="store.hasMoreSessions || store.loadingMoreSessions"
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
.workspace-section + .workspace-section { padding-top: 8px; border-top: 1px solid var(--sidebar-border); }
.workspace-group { color: var(--text-primary); transition: background 150ms ease, color 150ms ease; }
.workspace-group:hover, .workspace-group:focus-visible, .workspace-group-add:hover, .workspace-group-add:focus-visible { color: var(--text-primary); background: var(--primary-alpha-08); }
.workspace-group-add { color: var(--text-muted); }
.workspace-session-list { border-color: var(--primary-alpha-15); }
.workspace-session-row { color: var(--text-secondary); border: 1px solid transparent; transition: background 150ms ease, color 150ms ease, border-color 150ms ease; }
.workspace-session-row:hover, .workspace-session-row:focus-within { color: var(--text-primary); background: var(--primary-alpha-06); }
.workspace-session-row-active { color: var(--text-primary); background: var(--primary-alpha-12); border-color: var(--primary-alpha-15); }
.workspace-session-row-active:hover, .workspace-session-row-active:focus-within { background: var(--primary-alpha-15); }
.workspace-session-row-active .workspace-session-main { font-weight: 600; }
.workspace-session-main:focus-visible { outline: 2px solid var(--sb-brand); outline-offset: -2px; }
.workspace-session-menu { color: var(--text-muted); }
.workspace-session-menu:hover, .workspace-session-menu:focus-visible { color: var(--text-primary); background: var(--primary-alpha-10); }
.workspace-session-more, .workspace-load-more { color: var(--text-muted); transition: background 150ms ease, color 150ms ease; }
.workspace-session-more:hover, .workspace-session-more:focus-visible, .workspace-load-more:hover:not(:disabled), .workspace-load-more:focus-visible { color: var(--text-primary); background: var(--primary-alpha-08); }
.sidebar-search-input {
  background: var(--sidebar-bg);
  color: var(--text-primary);
  border: 1px solid var(--sidebar-border);
}
.sidebar-search-input::placeholder {
  color: var(--text-muted);
}
.sidebar-search-input:focus {
  border-color: var(--sb-brand);
  box-shadow: 0 0 0 2px var(--primary-alpha-12);
}

.update-notice-dot {
  width: 7px;
  height: 7px;
  flex: 0 0 auto;
  border-radius: 999px;
  background: #ef4444;
  box-shadow: 0 0 0 2px var(--sidebar-bg);
}
</style>
