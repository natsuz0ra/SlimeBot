<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import AppSelect from '@/components/ui/AppSelect.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import type { SelectOption } from '@/components/ui/AppSelect.vue'
import type { ApprovalMode, ThinkingLevel } from '@/types/settings'

defineProps<{
  messagePlatformDefaultModel: string
  messagePlatformModelOptions: SelectOption[]
  messagePlatformThinkingLevel: ThinkingLevel
  messagePlatformThinkingOptions: SelectOption[]
  messagePlatformApprovalMode: ApprovalMode
  messagePlatformApprovalOptions: SelectOption[]
  llmRowsEmpty: boolean
  telegramConfig?: { id: string; platform: string; displayName: string; authConfigJson: string; isEnabled: boolean }
}>()

const emit = defineEmits<{
  'update:messagePlatformDefaultModel': [value: string]
  'update:messagePlatformThinkingLevel': [value: ThinkingLevel]
  'update:messagePlatformApprovalMode': [value: ApprovalMode]
  toggleTelegram: []
  openBind: []
}>()

const { t } = useI18n()
</script>

<template>
  <div>
    <div class="settings-card flex items-center justify-between px-4 py-3.5 rounded-xl mb-2">
      <span class="settings-field-label">{{ t('messagePlatformDefaultModel') }}</span>
      <AppSelect
        :model-value="messagePlatformDefaultModel"
        :options="messagePlatformModelOptions"
        :disabled="llmRowsEmpty"
        @update:model-value="emit('update:messagePlatformDefaultModel', $event)"
      />
    </div>
    <div class="settings-card flex items-center justify-between px-4 py-3.5 rounded-xl mb-2">
      <span class="settings-field-label">{{ t('messagePlatformThinkingLevel') }}</span>
      <AppSelect
        :model-value="messagePlatformThinkingLevel"
        :options="messagePlatformThinkingOptions"
        @update:model-value="emit('update:messagePlatformThinkingLevel', $event as ThinkingLevel)"
      />
    </div>
    <div class="settings-card flex items-center justify-between px-4 py-3.5 rounded-xl mb-2">
      <span class="settings-field-label">{{ t('messagePlatformApprovalMode') }}</span>
      <AppSelect
        :model-value="messagePlatformApprovalMode"
        :options="messagePlatformApprovalOptions"
        @update:model-value="emit('update:messagePlatformApprovalMode', $event as ApprovalMode)"
      />
    </div>
    <div class="settings-card flex items-center gap-3 px-4 py-3.5 rounded-xl">
      <img src="/im_icon/telegram.svg" alt="telegram" class="w-5 h-5 flex-shrink-0" />
      <div class="flex-1 min-w-0">
        <div class="settings-item-name">{{ t('telegram') }}</div>
      </div>
      <template v-if="telegramConfig">
        <ToggleSwitch :model-value="telegramConfig.isEnabled" @update:model-value="() => emit('toggleTelegram')" />
        <button type="button" class="px-3 py-1.5 rounded-lg cursor-pointer account-edit-btn settings-action-text" @click="emit('openBind')">
          {{ t('editConfig') }}
        </button>
      </template>
      <button v-else type="button" class="btn-primary action-btn px-3 py-1.5 rounded-xl cursor-pointer settings-action-text" @click="emit('openBind')">
        {{ t('bind') }}
      </button>
    </div>
  </div>
</template>
