<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { mdiDeleteOutline, mdiPencilOutline, mdiPlus } from '@mdi/js'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'

defineProps<{
  mcpRows: { id: string; name: string; config: string; isEnabled: boolean }[]
  mcpPreview: (item: { id: string; name: string; config: string; isEnabled: boolean }) => string
  updateMcp: (item: { id: string; name: string; config: string; isEnabled: boolean }) => void
}>()

const emit = defineEmits<{
  add: []
  edit: [item: { id: string; name: string; config: string; isEnabled: boolean }]
  delete: [id: string]
}>()

const { t } = useI18n()
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <p class="section-label mb-0">{{ t('mcpSettings') }}</p>
      <button type="button" class="btn-primary action-btn flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-xl cursor-pointer" @click="emit('add')">
        <MdiIcon :path="mdiPlus" :size="13" />
        {{ t('add') }}
      </button>
    </div>
    <div class="flex flex-col gap-2">
      <div v-for="item in mcpRows" :key="item.id" class="settings-card flex items-center gap-3 px-4 py-3.5 rounded-xl">
        <div class="flex-1 min-w-0">
          <div class="text-sm font-medium settings-item-name">{{ item.name }}</div>
          <div class="text-xs settings-item-sub mt-0.5">{{ mcpPreview(item) }}</div>
        </div>
        <div class="flex items-center gap-2 flex-shrink-0">
          <ToggleSwitch
            :model-value="item.isEnabled"
            @update:model-value="
              (v) => {
                item.isEnabled = v
                updateMcp(item)
              }
            "
          />
          <button type="button" class="w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer edit-btn" @click="emit('edit', item)">
            <MdiIcon :path="mdiPencilOutline" :size="14" />
          </button>
          <button type="button" class="w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer delete-btn" @click="emit('delete', item.id)">
            <MdiIcon :path="mdiDeleteOutline" :size="15" />
          </button>
        </div>
      </div>
      <div v-if="mcpRows.length === 0" class="empty-state text-center py-10 text-sm rounded-xl">{{ t('add') }} MCP</div>
    </div>
  </div>
</template>
