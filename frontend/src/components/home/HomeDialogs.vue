<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallItem } from '@/api/chat'
import type { ToolTimelineEntry } from '@/types/chat'
import type { SettingsTabKey } from '@/types/settings'
import type { UpdateCheckResult } from '@/types/update'
import AccountEditDialog from '@/components/settings/AccountEditDialog.vue'
import SettingsPanel from '@/components/settings/SettingsPanel.vue'
import ToolExecutionDetailDialog from '@/components/chat/ToolExecutionDetailDialog.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import { isMaskSelfEvent, shouldCloseOnMaskInteraction } from '@/utils/dialogMask'
import { getShortcutHelpItems } from '@/utils/shortcuts'

const props = defineProps<{
  renameVisible: boolean
  renameValue: string
  deleteConfirmVisible: boolean
  toolDetailVisible: boolean
  toolDetailDialogWidth: string
  toolDetailItems: ToolCallItem[]
  toolDetailToolTimeline: ToolTimelineEntry[]
  settingsVisible: boolean
  settingsInitialTab?: SettingsTabKey
  hasUpdateNotice: boolean
  markUpdateNoticeRead: () => void
  setUpdateCheckResult: (result: UpdateCheckResult) => void
  accountDialogVisible: boolean
  shortcutsHelpVisible?: boolean
  isMac?: boolean
  isDesktop?: boolean
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
  openSession: [sessionId: string]
  budgetSaved: []
  'update:accountDialogVisible': [value: boolean]
  accountUpdated: []
  'update:shortcutsHelpVisible': [value: boolean]
}>()

const { t } = useI18n()
const settingsPointerDownStartedOnMask = ref(false)
const shortcutHelpItems = computed(() => getShortcutHelpItems(Boolean(props.isMac), Boolean(props.isDesktop)))

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
        <SettingsPanel
          :has-update-notice="hasUpdateNotice"
          :mark-update-notice-read="markUpdateNoticeRead"
          :set-update-check-result="setUpdateCheckResult"
          :initial-tab="settingsInitialTab"
          @close="emit('update:settingsVisible', false)"
          @llm-changed="emit('refreshModelOptions')"
          @open-session="emit('openSession', $event)"
          @budget-saved="emit('budgetSaved')"
        />
      </div>
    </div>
  </Transition>

  <AccountEditDialog
    :visible="accountDialogVisible"
    :force-mode="true"
    @update:visible="emit('update:accountDialogVisible', $event)"
    @success="emit('accountUpdated')"
  />

  <AppDialog
    :visible="Boolean(shortcutsHelpVisible)"
    :title="t('shortcutsTitle')"
    width="420px"
    hide-footer
    @update:visible="emit('update:shortcutsHelpVisible', $event)"
  >
    <div class="shortcuts-list">
      <div v-for="item in shortcutHelpItems" :key="item.action" class="shortcut-row">
        <span class="shortcut-label">{{ t(item.labelKey) }}</span>
        <kbd class="shortcut-kbd">{{ item.keys }}</kbd>
      </div>
    </div>
  </AppDialog>
</template>

<style scoped>
.shortcuts-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.shortcut-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 2px;
  font-size: 13px;
}

.shortcut-row + .shortcut-row {
  border-top: 1px solid var(--card-border);
}

.shortcut-label {
  color: var(--text-primary);
}

.shortcut-kbd {
  flex-shrink: 0;
  padding: 3px 8px;
  border-radius: 6px;
  border: 1px solid var(--input-border);
  background: var(--primary-alpha-05);
  color: var(--text-secondary);
  font-family: inherit;
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
}
</style>
