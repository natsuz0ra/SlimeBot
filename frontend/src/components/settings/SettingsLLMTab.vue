<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { mdiDeleteOutline, mdiPlus } from '@mdi/js'
import MdiIcon from '@/components/ui/MdiIcon.vue'

defineProps<{
  llmRows: { id: string; name: string; model: string; baseUrl: string; provider?: string }[] | any[]
}>()

const emit = defineEmits<{
  add: []
  delete: [id: string]
}>()

const { t } = useI18n()
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <p class="section-label mb-0">{{ t('llmSettings') }}</p>
      <button type="button" class="btn-primary action-btn flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-xl cursor-pointer" @click="emit('add')">
        <MdiIcon :path="mdiPlus" :size="13" />
        {{ t('add') }}
      </button>
    </div>
    <div class="flex flex-col gap-2">
      <div v-for="item in llmRows" :key="item.id" class="settings-card flex items-center gap-3 px-4 py-3.5 rounded-xl">
        <div class="flex-1 min-w-0">
          <div class="text-sm font-medium settings-item-name truncate">
            {{ item.name }}
            <span class="font-normal settings-item-meta"> · {{ item.model }}</span>
            <span v-if="item.provider === 'anthropic'" class="inline-block ml-1 px-1.5 py-0.5 text-[10px] font-medium rounded-md" style="background: rgba(217,119,6,0.15); color: #d97706;">Anthropic</span>
          </div>
          <div class="text-xs settings-item-sub truncate mt-0.5">{{ item.baseUrl }}</div>
        </div>
        <button type="button" class="flex-shrink-0 w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer delete-btn" @click="emit('delete', item.id)">
          <MdiIcon :path="mdiDeleteOutline" :size="15" />
        </button>
      </div>
      <div v-if="llmRows.length === 0" class="empty-state text-center py-10 text-sm rounded-xl">{{ t('add') }} LLM</div>
    </div>
  </div>
</template>
