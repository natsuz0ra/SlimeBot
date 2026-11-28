<script setup lang="ts">
import { ref } from 'vue'
import { mdiChevronDown, mdiDeleteOutline, mdiDownloadOutline, mdiFolderClockOutline, mdiMenu, mdiPencilOutline, mdiPinOffOutline, mdiPinOutline, mdiShieldOffOutline } from '@mdi/js'
import { useI18n } from 'vue-i18n'
import type { SessionItem } from '@/api/chat'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import TruncationTooltip from '@/components/ui/TruncationTooltip.vue'

const props = defineProps<{
  currentSession?: SessionItem
  canManageCurrentSession: boolean
  topMenuVisible: boolean
  networkStatusText: string
  connectionStatus: string
}>()

const emit = defineEmits<{
  toggleSidebar: []
  'update:topMenuVisible': [visible: boolean]
  renameCurrent: []
  pinCurrent: []
  exportCurrent: []
  removeCurrent: []
  openCheckpoints: []
  clearPrivacyMappings: []
}>()

const { t } = useI18n()
const titleButton = ref<HTMLButtonElement>()

function toggleTopMenu() {
  emit('update:topMenuVisible', !props.topMenuVisible)
}

function onRenameClick() {
  emit('renameCurrent')
  emit('update:topMenuVisible', false)
}

function dismissTopMenu() {
  if (!props.topMenuVisible) return
  titleButton.value?.focus()
  emit('update:topMenuVisible', false)
}

function onCheckpointsClick() {
  dismissTopMenu()
  emit('openCheckpoints')
}

function onClearPrivacyClick() {
  dismissTopMenu()
  emit('clearPrivacyMappings')
}
</script>

<template>
  <header class="header-bar relative z-30 flex items-center justify-center h-12 flex-shrink-0" @keydown.esc="dismissTopMenu">
    <button
      type="button"
      class="sb-text-muted absolute left-3 w-8 h-8 flex items-center justify-center rounded-lg hover:bg-[var(--bg-hover)] hover:text-[var(--text-primary)] transition-colors duration-150 cursor-pointer"
      :aria-label="t('scheduleToggleSidebar')"
      @click.stop="emit('toggleSidebar')"
    >
      <MdiIcon :path="mdiMenu" :size="19" />
    </button>

    <button
      v-if="currentSession && canManageCurrentSession"
      ref="titleButton"
      type="button"
      :aria-expanded="topMenuVisible"
      class="group/tip flex min-w-0 items-center gap-1.5 px-2.5 py-1 rounded-lg transition-colors duration-150 cursor-pointer max-w-[280px] header-title-btn"
      @click.stop="toggleTopMenu"
    >
      <TruncationTooltip
        inherit-group
        :text="currentSession.name"
        wrapper-class="min-w-0 flex-1 text-left"
        content-class="sb-text-primary text-sm font-semibold"
      />
      <MdiIcon
        :path="mdiChevronDown"
        :size="14"
        class="sb-text-muted flex-shrink-0 transition-transform duration-200"
        :class="topMenuVisible ? 'rotate-180' : ''"
      />
    </button>
    <div
      v-else-if="currentSession"
      class="group/tip flex min-w-0 items-center gap-1.5 px-3 py-1.5 rounded-xl max-w-[260px]"
    >
      <TruncationTooltip
        inherit-group
        :text="currentSession.name"
        wrapper-class="min-w-0 flex-1"
        content-class="sb-text-primary text-sm font-semibold"
      />
      <span class="text-[10px] px-1.5 py-0.5 rounded-md platform-badge">IM</span>
    </div>

    <div class="absolute right-3 flex items-center gap-2">
      <div
        v-if="networkStatusText"
        class="flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium"
        :class="connectionStatus === 'reconnecting' ? 'status-warning' : 'status-error'"
      >
        <span class="w-1.5 h-1.5 rounded-full animate-pulse" :class="connectionStatus === 'reconnecting' ? 'bg-amber-400' : 'bg-red-400'" />
        {{ networkStatusText }}
      </div>
      <slot name="right" />
    </div>

    <Transition name="top-menu-pop">
      <div
        v-if="topMenuVisible && canManageCurrentSession"
        class="absolute top-[46px] left-1/2 -translate-x-1/2 w-max min-w-40 max-w-[calc(100vw-2rem)] rounded-lg py-1 overflow-hidden z-[85] top-menu-glass"
        @click.stop
      >
        <button
          v-if="currentSession" type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm cursor-pointer menu-item"
          @click="dismissTopMenu(); emit('pinCurrent')"
        >
          <MdiIcon :path="currentSession.isPinned ? mdiPinOffOutline : mdiPinOutline" :size="14" />
          {{ t(currentSession.isPinned ? 'sessionUnpin' : 'sessionPin') }}
        </button>
        <button
          v-if="currentSession"
          type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm transition-colors duration-150 cursor-pointer menu-item"
          @click="onRenameClick"
        >
          <MdiIcon :path="mdiPencilOutline" :size="14" />
          <span>{{ t('rename') }}</span>
        </button>
        <button
          v-if="currentSession" type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm cursor-pointer menu-item"
          @click="dismissTopMenu(); emit('exportCurrent')"
        >
          <MdiIcon :path="mdiDownloadOutline" :size="14" />
          {{ t('sessionExport') }}
        </button>
        <button
          v-if="currentSession?.workingDirectory?.trim()"
          type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm transition-colors duration-150 cursor-pointer menu-item"
          @click="onCheckpointsClick"
        >
          <MdiIcon :path="mdiFolderClockOutline" :size="14" />
          <span>{{ t('checkpoints.title') }}</span>
        </button>
        <button
          v-if="currentSession"
          type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm text-left transition-colors duration-150 cursor-pointer menu-item"
          @click="onClearPrivacyClick"
        >
          <MdiIcon :path="mdiShieldOffOutline" :size="14" class="shrink-0" />
          <span>{{ t('sessionPrivacyDestroyTitle') }}</span>
        </button>
        <button
          v-if="currentSession"
          type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm border-t border-[var(--border-subtle)] transition-colors duration-150 cursor-pointer menu-item-danger"
          @click="emit('removeCurrent')"
        >
          <MdiIcon :path="mdiDeleteOutline" :size="14" />
          <span>{{ t('delete') }}</span>
        </button>
      </div>
    </Transition>
  </header>
</template>

<style scoped>
.header-title-btn:focus-visible,
.top-menu-glass button:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: -2px;
}
</style>
