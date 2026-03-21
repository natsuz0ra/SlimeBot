<script setup lang="ts">
import { mdiChevronDown, mdiDeleteOutline, mdiMenu, mdiPencilOutline } from '@mdi/js'
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
  removeCurrent: []
}>()

const { t } = useI18n()

function toggleTopMenu() {
  emit('update:topMenuVisible', !props.topMenuVisible)
}

function onRenameClick() {
  emit('renameCurrent')
  emit('update:topMenuVisible', false)
}
</script>

<template>
  <header class="header-bar relative z-30 flex items-center justify-center h-14 flex-shrink-0 backdrop-blur-sm">
    <button
      type="button"
      class="sb-text-muted absolute left-3 w-9 h-9 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer"
      @click.stop="emit('toggleSidebar')"
    >
      <MdiIcon :path="mdiMenu" :size="19" />
    </button>

    <button
      v-if="currentSession && canManageCurrentSession"
      type="button"
      class="group/tip flex min-w-0 items-center gap-1.5 px-3 py-1.5 rounded-xl transition-all duration-150 cursor-pointer max-w-[260px] header-title-btn"
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

    <div
      v-if="networkStatusText"
      class="absolute right-3 flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium"
      :class="connectionStatus === 'reconnecting' ? 'status-warning' : 'status-error'"
    >
      <span class="w-1.5 h-1.5 rounded-full animate-pulse" :class="connectionStatus === 'reconnecting' ? 'bg-amber-400' : 'bg-red-400'" />
      {{ networkStatusText }}
    </div>

    <Transition name="top-menu-pop">
      <div
        v-if="topMenuVisible && canManageCurrentSession"
        class="absolute top-[52px] left-1/2 -translate-x-1/2 w-40 rounded-xl py-1 overflow-hidden z-[85] top-menu-glass"
        @click.stop
      >
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
          v-if="currentSession"
          type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm transition-colors duration-150 cursor-pointer menu-item-danger"
          @click="emit('removeCurrent')"
        >
          <MdiIcon :path="mdiDeleteOutline" :size="14" />
          <span>{{ t('delete') }}</span>
        </button>
      </div>
    </Transition>
  </header>
</template>
