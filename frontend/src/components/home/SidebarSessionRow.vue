<script setup lang="ts">
import { Ellipsis, Pin } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import type { SessionItem } from '@/api/chat'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import TruncationTooltip from '@/components/ui/TruncationTooltip.vue'
import { useChatStore } from '@/stores/chat'

defineProps<{ item: SessionItem; active: boolean }>()
const emit = defineEmits<{ pick: []; menu: [event: MouseEvent] }>()
const { t } = useI18n()
const store = useChatStore()
</script>

<template>
  <div
    class="workspace-session-row group group/tip relative flex h-9 min-w-0 items-center rounded-lg"
    :class="active ? 'workspace-session-row-active' : ''"
  >
    <button
      type="button"
      class="workspace-session-main relative flex h-full min-w-0 flex-1 items-center rounded-lg px-2 pr-9 text-left cursor-pointer"
      :aria-current="active ? 'page' : undefined"
      @click="emit('pick')"
    >
      <span v-if="active" class="session-active-indicator absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-4 rounded-r-full" />
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
    <Pin v-else-if="item.isPinned" :size="12" :stroke-width="1.75" class="absolute right-3 top-1/2 -translate-y-1/2 sb-text-muted group-hover:opacity-0 pointer-events-none" aria-hidden="true" />
    <button
      type="button"
      class="workspace-session-menu absolute right-1 top-1/2 -translate-y-1/2 w-7 h-7 flex items-center justify-center rounded-md transition-colors duration-150 cursor-pointer flex-shrink-0"
      :aria-label="t('sessionMenu')"
      @click.stop="emit('menu', $event)"
    ><Ellipsis :size="15" :stroke-width="1.75" aria-hidden="true" /></button>
  </div>
</template>
