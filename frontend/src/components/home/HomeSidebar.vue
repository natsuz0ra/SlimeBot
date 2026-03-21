<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  mdiClose,
  mdiCogOutline,
  mdiDotsHorizontal,
  mdiMagnify,
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

const props = defineProps<{
  sessions: SessionItem[]
  currentSessionId?: string
  isDark: boolean
  setSidebarListRef: (el: unknown) => void
}>()

const emit = defineEmits<{
  createSession: []
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

const regularSessions = computed(() =>
  props.sessions.filter((session) => session.id !== MESSAGE_PLATFORM_SESSION_ID),
)

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

    <div :ref="setSidebarListRef" class="scroll-area flex-1 overflow-y-auto py-2 px-2">
      <div
        class="group group/tip relative flex min-w-0 items-center gap-1 px-3 h-9 rounded-xl cursor-pointer transition-all duration-150 mb-0.5"
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

      <div
        v-for="item in regularSessions"
        :key="item.id"
        class="group group/tip relative flex min-w-0 items-center gap-1 px-3 h-9 rounded-xl cursor-pointer transition-all duration-150 mb-0.5"
        :class="item.id === currentSessionId ? 'session-item-active' : 'session-item'"
        @click="emit('pickSession', item.id)"
      >
        <span
          v-if="item.id === currentSessionId"
          class="session-active-indicator absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 rounded-r-full"
        />
        <TruncationTooltip
          inherit-group
          :text="item.name"
          wrapper-class="min-w-0 flex-1"
          content-class="sb-text-primary text-sm"
        />
        <button
          type="button"
          class="sb-text-muted w-6 h-6 flex items-center justify-center rounded-md transition-colors duration-150 cursor-pointer opacity-0 group-hover:opacity-100 flex-shrink-0"
          :class="item.id === currentSessionId ? '!opacity-100' : ''"
          @click.stop="emit('toggleSessionMenu', item.id, $event as MouseEvent)"
        >
          <MdiIcon :path="mdiDotsHorizontal" :size="15" />
        </button>
      </div>

      <div v-if="store.loadingMoreSessions" class="flex justify-center py-2">
        <svg class="loading-spinner-accent animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
        </svg>
      </div>
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
        </button>
      </div>
    </div>
  </aside>
</template>

<style scoped>
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
</style>
