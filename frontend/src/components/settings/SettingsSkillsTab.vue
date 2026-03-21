<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { mdiDeleteOutline, mdiPlus } from '@mdi/js'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'

defineProps<{
  skillsRows: { id: string; name: string; description?: string; relativePath?: string }[]
  skillsUploading: boolean
  skillsDropActive: boolean
}>()

const emit = defineEmits<{
  openPicker: []
  drop: [event: DragEvent]
  dragOver: [event: DragEvent]
  dragLeave: [event: DragEvent]
  delete: [id: string]
}>()

const { t } = useI18n()
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-1">
      <p class="section-label mb-0">{{ t('skillsSettings') }}</p>
      <button
        type="button"
        class="btn-primary action-btn flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-xl cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
        :disabled="skillsUploading"
        @click="emit('openPicker')"
      >
        <LoadingSpinner v-if="skillsUploading" size-class="w-3 h-3" />
        <MdiIcon v-else :path="mdiPlus" :size="13" />
        {{ t('skillsUploadButton') }}
      </button>
    </div>
    <p class="text-xs mb-3 settings-item-sub">{{ t('skillsDescription') }}</p>

    <div
      class="drop-zone rounded-xl px-4 py-6 text-center text-sm cursor-pointer transition-all duration-200 mb-3"
      :class="skillsDropActive ? 'drop-zone-active' : 'drop-zone-idle'"
      @dragover="emit('dragOver', $event)"
      @dragleave="emit('dragLeave', $event)"
      @drop="emit('drop', $event)"
      @click="emit('openPicker')"
    >
      {{ t('skillsUploadHint') }}
    </div>
    <slot name="file-input" />

    <div v-if="skillsRows.length === 0" class="empty-state text-center py-8 text-sm rounded-xl">{{ t('skillsEmpty') }}</div>
    <div v-else class="flex flex-col gap-2">
      <div v-for="item in skillsRows" :key="item.id" class="settings-card flex items-start gap-3 px-4 py-3.5 rounded-xl">
        <div class="flex-1 min-w-0">
          <div class="text-sm font-medium settings-item-name">{{ item.name }}</div>
          <div v-if="item.description" class="text-xs settings-item-sub mt-0.5">{{ item.description }}</div>
          <div v-if="item.relativePath" class="text-xs mt-0.5 font-mono sb-text-muted">{{ item.relativePath }}</div>
        </div>
        <button type="button" class="flex-shrink-0 w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer delete-btn" @click="emit('delete', item.id)">
          <MdiIcon :path="mdiDeleteOutline" :size="15" />
        </button>
      </div>
    </div>
  </div>
</template>
