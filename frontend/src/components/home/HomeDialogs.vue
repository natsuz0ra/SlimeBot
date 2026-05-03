<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallItem } from '@/api/chat'
import type { ToolTimelineEntry } from '@/types/chat'
import AccountEditDialog from '@/components/settings/AccountEditDialog.vue'
import SettingsPanel from '@/components/settings/SettingsPanel.vue'
import ToolExecutionDetailDialog from '@/components/chat/ToolExecutionDetailDialog.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import { isMaskSelfEvent, shouldCloseOnMaskInteraction } from '@/utils/dialogMask'

const props = defineProps<{
  renameVisible: boolean
  renameValue: string
  deleteConfirmVisible: boolean
  toolDetailVisible: boolean
  toolDetailDialogWidth: string
  toolDetailItems: ToolCallItem[]
  toolDetailToolTimeline: ToolTimelineEntry[]
  settingsVisible: boolean
  accountDialogVisible: boolean
}>()

const emit = defineEmits<{
  'update:renameVisible': [value: boolean]
  'update:renameValue': [value: string]
  confirmRename: []
  'update:deleteConfirmVisible': [value: boolean]
  confirmDeleteSession: []
  'update:toolDetailVisible': [value: boolean]
  approveToolCall: [toolCallId: string]
  rejectToolCall: [toolCallId: string]
  'update:settingsVisible': [value: boolean]
  refreshModelOptions: []
  'update:accountDialogVisible': [value: boolean]
  accountUpdated: []
}>()

const { t } = useI18n()
const settingsPointerDownStartedOnMask = ref(false)

function onSettingsMaskPointerDown(e: PointerEvent) {
  settingsPointerDownStartedOnMask.value = isMaskSelfEvent(e)
}

function onSettingsMaskClick(e: MouseEvent) {
  if (shouldCloseOnMaskInteraction({
    closeOnMask: true,
    pointerDownStartedOnMask: settingsPointerDownStartedOnMask.value,
    eventTarget: e.target,
    eventCurrentTarget: e.currentTarget,
  })) {
    emit('update:settingsVisible', false)
  }
  settingsPointerDownStartedOnMask.value = false
}
</script>

<template>
  <AppDialog
    :visible="renameVisible"
    :title="t('rename')"
    :confirm-text="t('confirm')"
    :cancel-text="t('cancel')"
    width="360px"
    @update:visible="emit('update:renameVisible', $event)"
    @confirm="emit('confirmRename')"
  >
    <input
      :value="renameValue"
      type="text"
      class="w-full px-3 py-2.5 text-sm rounded-xl outline-none transition-all duration-150 dialog-input focus-ring"
      @input="emit('update:renameValue', ($event.target as HTMLInputElement).value)"
      @keydown.enter="emit('confirmRename')"
    />
  </AppDialog>

  <AppDialog
    :visible="deleteConfirmVisible"
    :title="t('delete')"
    :confirm-text="t('confirm')"
    :cancel-text="t('cancel')"
    :confirm-danger="true"
    width="360px"
    @update:visible="emit('update:deleteConfirmVisible', $event)"
    @confirm="emit('confirmDeleteSession')"
  >
    <p class="sb-text-secondary text-sm">{{ t('confirmDelete') }}</p>
  </AppDialog>

  <ToolExecutionDetailDialog
    :visible="toolDetailVisible"
    :width="toolDetailDialogWidth"
    :items="toolDetailItems"
    :tool-timeline="toolDetailToolTimeline"
    @update:visible="emit('update:toolDetailVisible', $event)"
    @approve="emit('approveToolCall', $event)"
    @reject="emit('rejectToolCall', $event)"
  />

  <Transition name="overlay-fade">
    <div
      v-if="settingsVisible"
      class="settings-overlay fixed inset-0 z-[100] flex items-center justify-center p-4 sm:p-6"
      @pointerdown="onSettingsMaskPointerDown"
      @click="onSettingsMaskClick"
    >
      <div
        class="settings-modal settings-modal-size w-full rounded-2xl overflow-hidden"
        @click.stop
      >
        <SettingsPanel @close="emit('update:settingsVisible', false)" @llm-changed="emit('refreshModelOptions')" />
      </div>
    </div>
  </Transition>

  <AccountEditDialog
    :visible="accountDialogVisible"
    :force-mode="true"
    @update:visible="emit('update:accountDialogVisible', $event)"
    @success="emit('accountUpdated')"
  />
</template>
