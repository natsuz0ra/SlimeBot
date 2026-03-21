<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  mdiClose,
  mdiCogOutline,
  mdiDotsHorizontal,
  mdiPlus,
  mdiWeatherNight,
  mdiWeatherSunny,
} from '@mdi/js'
import type { SessionItem } from '@/api/chat'
import { MESSAGE_PLATFORM_SESSION_ID } from '@/api/chat'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import AppLogo from '@/components/ui/AppLogo.vue'

const props = defineProps<{
  sessions: SessionItem[]
  currentSessionId?: string
  isDark: boolean
  setSidebarListRef: (el: unknown) => void
}>()

const emit = defineEmits<{
  createSession: []
  closeSidebar: []
  pickSession: [sessionId: string]
  toggleSessionMenu: [sessionId: string, event: MouseEvent]
  toggleTheme: []
  openSettings: []
}>()

const { t } = useI18n()

const regularSessions = computed(() =>
  props.sessions.filter((session) => session.id !== MESSAGE_PLATFORM_SESSION_ID),
)
</script>

<template>
  <aside class="sidebar-panel absolute inset-y-0 left-0 w-64 flex flex-col z-30 backdrop-blur-xl">
    <div class="sidebar-header flex items-center justify-between px-4 h-14">
      <div class="flex items-center gap-2.5">
        <AppLogo :size="36" />
        <span class="sb-text-primary text-lg font-semibold tracking-wide brand-tech-font">SlimeBot</span>
      </div>

      <div class="flex items-center gap-1">
        <button
          type="button"
          class="sb-text-muted w-8 h-8 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer group"
          @click="emit('createSession')"
        >
          <MdiIcon :path="mdiPlus" :size="20" class="group-hover:scale-110 transition-transform duration-150" />
        </button>
        <button
          type="button"
          class="sb-text-muted w-8 h-8 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer"
          @click="emit('closeSidebar')"
        >
          <MdiIcon :path="mdiClose" :size="20" />
        </button>
      </div>
    </div>

    <div :ref="setSidebarListRef" class="scroll-area flex-1 overflow-y-auto py-2 px-2">
      <div
        class="group relative flex items-center gap-1 px-3 h-9 rounded-xl cursor-pointer transition-all duration-150 mb-0.5"
        :class="currentSessionId === MESSAGE_PLATFORM_SESSION_ID ? 'session-item-active' : 'session-item'"
        @click="emit('pickSession', MESSAGE_PLATFORM_SESSION_ID)"
      >
        <span
          v-if="currentSessionId === MESSAGE_PLATFORM_SESSION_ID"
          class="session-active-indicator absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 rounded-r-full"
        />
        <span class="sb-text-primary flex-1 truncate text-sm">{{ t('messagePlatformSession') }}</span>
        <span class="text-[10px] px-1.5 py-0.5 rounded-md platform-badge">IM</span>
      </div>

      <div
        v-for="item in regularSessions"
        :key="item.id"
        class="group relative flex items-center gap-1 px-3 h-9 rounded-xl cursor-pointer transition-all duration-150 mb-0.5"
        :class="item.id === currentSessionId ? 'session-item-active' : 'session-item'"
        @click="emit('pickSession', item.id)"
      >
        <span
          v-if="item.id === currentSessionId"
          class="session-active-indicator absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 rounded-r-full"
        />
        <span class="sb-text-primary flex-1 truncate text-sm">{{ item.name }}</span>
        <button
          type="button"
          class="sb-text-muted w-6 h-6 flex items-center justify-center rounded-md transition-colors duration-150 cursor-pointer opacity-0 group-hover:opacity-100 flex-shrink-0"
          :class="item.id === currentSessionId ? '!opacity-100' : ''"
          @click.stop="emit('toggleSessionMenu', item.id, $event as MouseEvent)"
        >
          <MdiIcon :path="mdiDotsHorizontal" :size="15" />
        </button>
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
