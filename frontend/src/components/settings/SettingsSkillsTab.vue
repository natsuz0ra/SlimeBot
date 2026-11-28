<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Plus, Trash2 } from 'lucide-vue-next'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import { canDeleteSkill } from '@/utils/skills'
import type { SkillItem } from '@/types/settings'

defineProps<{
  skillsRows: SkillItem[]
  skillsUploading: boolean
  skillsDropActive: boolean
}>()

const emit = defineEmits<{
  openPicker: []
  drop: [event: DragEvent]
  dragOver: [event: DragEvent]
  dragLeave: [event: DragEvent]
  delete: [id: string]
  toggleEnabled: [id: string, enabled: boolean]
}>()

const { t } = useI18n()

function readChecked(event: Event) {
  return (event.target as HTMLInputElement).checked
}
</script>

<template>
  <div>
    <div class="flex flex-wrap items-center justify-between gap-2 mb-3">
      <p class="settings-item-sub">{{ t('skillsDescription') }}</p>
      <button
        type="button"
        class="btn-primary action-btn flex items-center gap-1.5 px-3 py-1.5 rounded-xl cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed settings-action-text"
        :disabled="skillsUploading"
        @click="emit('openPicker')"
      >
        <LoadingSpinner v-if="skillsUploading" size-class="w-3 h-3" />
        <Plus v-else :size="13" :stroke-width="1.75" aria-hidden="true" />
        {{ t('skillsUploadButton') }}
      </button>
    </div>

    <div
      class="drop-zone rounded-xl px-4 py-6 text-center cursor-pointer transition-all duration-200 mb-3"
      :class="skillsDropActive ? 'drop-zone-active' : 'drop-zone-idle'"
      @dragover="emit('dragOver', $event)"
      @dragleave="emit('dragLeave', $event)"
      @drop="emit('drop', $event)"
      @click="emit('openPicker')"
    >
      {{ t('skillsUploadHint') }}
    </div>
    <slot name="file-input" />

    <div v-if="skillsRows.length === 0" class="empty-state text-center py-8 rounded-xl">{{ t('skillsEmpty') }}</div>
    <div v-else class="flex flex-col gap-2">
      <div v-for="item in skillsRows" :key="item.id" class="settings-card flex items-start gap-3 px-4 py-3.5 rounded-xl">
        <div class="flex-1 min-w-0">
          <div class="settings-item-name">{{ item.name }}</div>
          <div v-if="item.description" class="settings-item-sub mt-0.5">{{ item.description }}</div>
          <div class="flex flex-wrap items-center gap-1.5 mt-2">
            <span class="skill-badge">{{ item.sourceLabel || item.provider || 'SlimeBot' }}</span>
            <span v-if="item.readOnly" class="skill-badge">{{ t('skillsReadOnly') }}</span>
            <span class="skill-badge" :class="item.enabled ? 'skill-badge-enabled' : 'skill-badge-disabled'">
              {{ item.enabled ? t('skillsEnabled') : t('skillsDisabled') }}
            </span>
          </div>
          <div v-if="item.relativePath" class="settings-item-meta mt-0.5 font-mono">{{ item.relativePath }}</div>
        </div>
        <label v-if="item.readOnly" class="skill-switch flex-shrink-0" :title="t('skillsToggle')">
          <input
            type="checkbox"
            class="sr-only"
            :checked="item.enabled"
            @change="emit('toggleEnabled', item.id, readChecked($event))"
          >
          <span class="skill-switch-track" />
        </label>
        <button v-if="canDeleteSkill(item)" type="button" class="flex-shrink-0 w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer delete-btn" @click="emit('delete', item.id)">
          <Trash2 :size="15" :stroke-width="1.75" aria-hidden="true" />
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.skill-badge {
  border: 1px solid var(--card-border);
  border-radius: 999px;
  color: var(--text-muted);
  font-size: 11px;
  line-height: 1;
  padding: 3px 7px;
}

.skill-badge-enabled {
  color: var(--color-accent);
}

.skill-badge-disabled {
  color: var(--color-danger);
}

.skill-switch {
  cursor: pointer;
  display: inline-flex;
  padding-top: 2px;
}

.skill-switch-track {
  background: var(--card-border);
  border-radius: 999px;
  height: 18px;
  position: relative;
  transition: background-color 0.15s ease;
  width: 32px;
}

.skill-switch-track::after {
  background: var(--card-bg);
  border-radius: 999px;
  content: '';
  height: 14px;
  left: 2px;
  position: absolute;
  top: 2px;
  transition: transform 0.15s ease;
  width: 14px;
}

.skill-switch input:checked + .skill-switch-track {
  background: var(--sb-brand);
}

.skill-switch input:checked + .skill-switch-track::after {
  transform: translateX(14px);
}
</style>
