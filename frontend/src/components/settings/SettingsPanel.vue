<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { mdiClose } from '@mdi/js'
import CodeMirror from 'vue-codemirror6'

import MdiIcon from '@/components/ui/MdiIcon.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppTextInput from '@/components/ui/AppTextInput.vue'
import AppPasswordInput from '@/components/ui/AppPasswordInput.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import SettingsBasicTab from '@/components/settings/SettingsBasicTab.vue'
import SettingsLLMTab from '@/components/settings/SettingsLLMTab.vue'
import SettingsMCPTab from '@/components/settings/SettingsMCPTab.vue'
import SettingsSkillsTab from '@/components/settings/SettingsSkillsTab.vue'
import SettingsPlatformTab from '@/components/settings/SettingsPlatformTab.vue'
import SettingsAboutTab from '@/components/settings/SettingsAboutTab.vue'
import AccountEditDialog from '@/components/settings/AccountEditDialog.vue'
import { llmAPI } from '@/api/llm'
import { mcpAPI } from '@/api/mcp'
import { settingAPI } from '@/api/settings'
import { skillsAPI } from '@/api/skills'
import { messagePlatformAPI } from '@/api/messagePlatform'
import type { AppSettings, LLMConfig, MCPConfig, MessagePlatformConfig, SettingsTabKey, SkillItem } from '@/types/settings'
import { useToast } from '@/composables/useToast'
import { useSettingsLLM } from '@/composables/settings/useSettingsLLM'
import { useSettingsMCP } from '@/composables/settings/useSettingsMCP'
import { useSettingsSkills } from '@/composables/settings/useSettingsSkills'
import { useSettingsMessagePlatform } from '@/composables/settings/useSettingsMessagePlatform'
import { useSettingsConfirmDialog } from '@/composables/settings/useSettingsConfirmDialog'
import { useSettingsWebSearch } from '@/composables/settings/useSettingsWebSearch'
import { useSettingsApprovalMode } from '@/composables/settings/useSettingsApprovalMode'
import { useLanguagePreference, type LanguageCode } from '@/composables/useLanguagePreference'
import { useAuthStore } from '@/stores/auth'
import { useChatStore } from '@/stores/chat'
import { useRouter } from 'vue-router'

const emit = defineEmits<{
  close: []
  llmChanged: []
}>()

const { t } = useI18n()
const toast = useToast()
const authStore = useAuthStore()
const chatStore = useChatStore()
const router = useRouter()
const { language, languageSelectOptions, savingLanguage, loadLanguage, changeLanguage } = useLanguagePreference()

const settingsTabs: { key: SettingsTabKey; labelKey: string }[] = [
  { key: 'basic', labelKey: 'basicSettings' },
  { key: 'llm', labelKey: 'llmSettings' },
  { key: 'mcp', labelKey: 'mcpSettings' },
  { key: 'skills', labelKey: 'skillsSettings' },
  { key: 'platform', labelKey: 'messagePlatformSettings' },
  { key: 'about', labelKey: 'aboutSettings' },
]

const tab = ref<SettingsTabKey>('basic')
const llmList = ref<LLMConfig[]>([])
const mcpList = ref<MCPConfig[]>([])
const skillsList = ref<SkillItem[]>([])
const messagePlatformList = ref<MessagePlatformConfig[]>([])
const loading = ref(false)
const llmDialogVisible = ref(false)
const mcpDialogVisible = ref(false)
const llmSubmitting = ref(false)
const mcpSubmitting = ref(false)
const skillsUploading = ref(false)
const skillsDropActive = ref(false)
const skillsFileInputRef = ref<HTMLInputElement | null>(null)
const accountDialogVisible = ref(false)
const messagePlatformDialogVisible = ref(false)
const messagePlatformSubmitting = ref(false)
const messagePlatformDefaultModel = ref('')
const { confirmDialogVisible, openConfirmDialog, runConfirmDialog } = useSettingsConfirmDialog()
const {
  webSearchDialogVisible,
  webSearchKey,
  savingWebSearch,
  openWebSearchDialog,
  closeWebSearchDialog,
  saveWebSearch,
} = useSettingsWebSearch({ toast, t: (key) => t(key) })
const { approvalMode, onApprovalModeChange } = useSettingsApprovalMode({ toast, t: (key) => t(key) })

const {
  llmForm,
  llmDialogTitleKey,
  llmContextSizeDisplay,
  llmContextSizeSlider,
  llmRows,
  openLLMDialog,
  openLLMEditDialog,
  saveLLM,
  deleteLLM: removeLLM,
} = useSettingsLLM({
  llmList,
  llmDialogVisible,
  llmSubmitting,
  toast,
  t: (key) => t(key),
  onChanged: () => emit('llmChanged'),
})

const {
  mcpForm,
  mcpRows,
  mcpDialogTitle,
  mcpTemplateType,
  mcpEditorExtensions,
  applyTemplate,
  openMCPDialog,
  openMCPEditDialog,
  saveMCP,
  updateMCP,
  deleteMCP: removeMCP,
  mcpPreview,
} = useSettingsMCP({
  mcpList,
  mcpDialogVisible,
  mcpSubmitting,
  toast,
  t: (key) => t(key),
})

const skillsRows = computed(() =>
  [...(skillsList.value || [])].sort((a, b) => {
    const aTime = new Date(a.uploadedAt || 0).getTime()
    const bTime = new Date(b.uploadedAt || 0).getTime()
    return bTime - aTime
  }),
)

const skillsActions = useSettingsSkills({
  skillsList,
  skillsUploading,
  skillsDropActive,
  skillsFileInputRef,
  toast,
  t: (key) => t(key),
})

const {
  openSkillsPicker,
  onSkillsInputChange,
  onSkillsDrop,
  onSkillsDragOver,
  onSkillsDragLeave,
  deleteSkill: removeSkill,
} = skillsActions

const {
  messagePlatformForm,
  telegramConfig,
  messagePlatformModelOptions,
  openMessagePlatformDialog,
  saveMessagePlatformConfig,
  toggleTelegramEnabled,
  saveMessagePlatformDefaultModel,
} = useSettingsMessagePlatform({
  messagePlatformList,
  messagePlatformDialogVisible,
  messagePlatformSubmitting,
  messagePlatformDefaultModel,
  llmRows,
  toast,
  t: (key) => t(key),
})

async function loadData() {
  loading.value = true
  try {
    await loadLanguage({ allowRemote: true })
    const appSettings: AppSettings = await settingAPI.get()
    messagePlatformDefaultModel.value = appSettings.messagePlatformDefaultModel || ''
    webSearchKey.value = appSettings.webSearchKey || ''
    approvalMode.value = appSettings.approvalMode || 'standard'
    llmList.value = await llmAPI.list()
    mcpList.value = await mcpAPI.list()
    skillsList.value = await skillsAPI.list()
    messagePlatformList.value = await messagePlatformAPI.list()
  } finally {
    loading.value = false
  }
}

async function onLanguageChange(nextLanguage: LanguageCode) {
  await changeLanguage(nextLanguage, { allowRemote: true, showSuccessToast: true })
}

function openAccountDialog() {
  accountDialogVisible.value = true
}

function logout() {
  chatStore.disconnectSocket({ silentConnectionNotice: true })
  chatStore.resetToNewSession()
  authStore.clearAuth()
  void router.replace('/login')
}

function onAccountUpdated() {
  accountDialogVisible.value = false
}

function deleteLLM(id: string) {
  openConfirmDialog(async () => {
    await removeLLM(id)
  })
}

function deleteMCP(id: string) {
  openConfirmDialog(async () => {
    await removeMCP(id)
  })
}

function deleteSkill(id: string) {
  openConfirmDialog(async () => {
    await removeSkill(id)
  })
}

onMounted(loadData)
</script>

<template>
  <div class="h-full flex flex-col overflow-hidden settings-root">
    <div class="flex items-center justify-between px-5 h-13 flex-shrink-0 settings-header">
      <div class="flex items-center gap-2.5">
        <div class="w-6 h-6 rounded-lg flex items-center justify-center" style="background: linear-gradient(135deg, #6366f1 0%, #a78bfa 100%)">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none">
            <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" stroke="white" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </div>
        <span class="text-sm font-semibold settings-title">{{ t('settings') }}</span>
      </div>
      <button
        type="button"
        class="w-8 h-8 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer settings-close-btn"
        @click="emit('close')"
      >
        <MdiIcon :path="mdiClose" :size="15" />
      </button>
    </div>

    <div class="flex flex-1 overflow-hidden settings-body">
      <aside class="w-44 flex-shrink-0 p-2.5 flex flex-col gap-1 overflow-y-auto settings-sidebar">
        <button
          v-for="item in settingsTabs"
          :key="item.key"
          type="button"
          class="relative w-full text-left px-3.5 h-9 rounded-xl text-sm transition-all duration-150 cursor-pointer settings-tab"
          :class="tab === item.key ? 'settings-tab-active' : 'settings-tab-inactive'"
          @click="tab = item.key"
        >
          <span
            v-if="tab === item.key"
            class="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 rounded-r-full"
            style="background: #6366f1"
          />
          {{ t(item.labelKey) }}
        </button>
      </aside>

      <section class="flex-1 min-w-0 overflow-y-auto px-5 py-5 relative settings-content">
        <div v-if="loading" class="absolute inset-0 flex items-center justify-center z-10" style="background: rgba(var(--bg-main), 0.7)">
          <LoadingSpinner />
        </div>

        <SettingsBasicTab
          v-if="tab === 'basic'"
          :language="language"
          :language-select-options="languageSelectOptions"
          :saving-language="savingLanguage"
          :approval-mode="approvalMode"
          @open-account="openAccountDialog"
          @open-web-search="openWebSearchDialog"
          @logout="logout"
          @language-change="onLanguageChange"
          @approval-mode-change="onApprovalModeChange"
        />

        <SettingsLLMTab v-if="tab === 'llm'" :llm-rows="llmRows" @add="openLLMDialog" @edit="openLLMEditDialog" @delete="deleteLLM" />

        <SettingsMCPTab
          v-if="tab === 'mcp'"
          :mcp-rows="mcpRows"
          :mcp-preview="mcpPreview"
          :update-mcp="updateMCP"
          @add="openMCPDialog"
          @edit="openMCPEditDialog"
          @delete="deleteMCP"
        />

        <SettingsSkillsTab
          v-if="tab === 'skills'"
          :skills-rows="skillsRows"
          :skills-uploading="skillsUploading"
          :skills-drop-active="skillsDropActive"
          @open-picker="openSkillsPicker"
          @drop="onSkillsDrop"
          @drag-over="onSkillsDragOver"
          @drag-leave="onSkillsDragLeave"
          @delete="deleteSkill"
        >
          <template #file-input>
            <input
              ref="skillsFileInputRef"
              type="file"
              class="hidden"
              accept=".zip,application/zip"
              multiple
              @change="onSkillsInputChange"
            />
          </template>
        </SettingsSkillsTab>

        <SettingsPlatformTab
          v-if="tab === 'platform'"
          :message-platform-default-model="messagePlatformDefaultModel"
          :message-platform-model-options="messagePlatformModelOptions"
          :llm-rows-empty="llmRows.length === 0"
          :telegram-config="telegramConfig"
          @update:message-platform-default-model="saveMessagePlatformDefaultModel($event)"
          @toggle-telegram="toggleTelegramEnabled"
          @open-bind="openMessagePlatformDialog"
        />

        <SettingsAboutTab v-if="tab === 'about'" />
      </section>
    </div>
  </div>

  <AppDialog
    v-model:visible="llmDialogVisible"
    :title="t(llmDialogTitleKey)"
    :confirm-text="t('confirm')"
    :cancel-text="t('cancel')"
    :confirm-loading="llmSubmitting"
    width="440px"
    @confirm="saveLLM"
  >
    <div class="flex flex-col gap-4">
      <div class="flex flex-col gap-1.5">
        <label class="text-xs font-medium sb-text-muted">{{ t('provider') }}</label>
        <div class="flex gap-3">
          <label class="flex items-center gap-1.5 text-sm cursor-pointer">
            <input type="radio" v-model="llmForm.provider" value="openai" class="accent-[#6366f1]" />
            {{ t('providerOpenAI') }}
          </label>
          <label class="flex items-center gap-1.5 text-sm cursor-pointer">
            <input type="radio" v-model="llmForm.provider" value="anthropic" class="accent-[#6366f1]" />
            {{ t('providerAnthropic') }}
          </label>
          <label class="flex items-center gap-1.5 text-sm cursor-pointer">
            <input type="radio" v-model="llmForm.provider" value="deepseek" class="accent-[#6366f1]" />
            {{ t('providerDeepSeek') }}
          </label>
        </div>
      </div>
      <div class="flex flex-col gap-1.5">
        <label class="text-xs font-medium sb-text-muted">{{ t('name') }}</label>
        <AppTextInput v-model="llmForm.name" />
      </div>
      <div class="flex flex-col gap-1.5">
        <label class="text-xs font-medium sb-text-muted">{{ t('model') }}</label>
        <AppTextInput v-model="llmForm.model" />
      </div>
      <div class="flex flex-col gap-1.5">
        <label class="text-xs font-medium sb-text-muted">{{ t('baseUrl') }}</label>
        <AppTextInput v-model="llmForm.baseUrl" />
      </div>
      <div class="flex flex-col gap-1.5">
        <label class="text-xs font-medium sb-text-muted">{{ t('apiKey') }}</label>
        <AppPasswordInput v-model="llmForm.apiKey" />
      </div>
      <div class="flex flex-col gap-2">
        <div class="flex items-center justify-between gap-3">
          <label class="text-xs font-medium sb-text-muted">{{ t('contextSize') }}</label>
          <span class="text-xs settings-item-meta">{{ llmContextSizeDisplay }}</span>
        </div>
        <input
          v-model.number="llmContextSizeSlider"
          type="range"
          min="0"
          max="100"
          step="1"
          class="context-size-slider"
        />
        <div class="flex items-center gap-2">
          <input
            v-model.number="llmForm.contextSize"
            type="number"
            min="8000"
            max="1000000"
            step="1000"
            class="context-size-input"
          />
          <span class="text-xs sb-text-muted">{{ t('contextSizeHint') }}</span>
        </div>
      </div>
    </div>
  </AppDialog>

  <AppDialog
    v-model:visible="confirmDialogVisible"
    :title="t('delete')"
    :confirm-text="t('confirm')"
    :cancel-text="t('cancel')"
    :confirm-danger="true"
    width="360px"
    @confirm="runConfirmDialog"
  >
    <p class="text-sm sb-text-secondary">{{ t('confirmDeleteItem') }}</p>
  </AppDialog>

  <AppDialog
    v-model:visible="mcpDialogVisible"
    :title="mcpDialogTitle"
    :confirm-text="t('confirm')"
    :cancel-text="t('cancel')"
    :confirm-loading="mcpSubmitting"
    width="760px"
    @confirm="saveMCP"
  >
    <div class="flex flex-col gap-4">
      <div class="flex flex-col gap-1.5">
        <label class="text-xs font-medium sb-text-muted">{{ t('name') }}</label>
        <AppTextInput v-model="mcpForm.name" />
      </div>

      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label class="text-xs font-medium sb-text-muted">{{ t('mcpConfigJson') }}</label>
          <div class="flex items-center gap-1">
            <button
              v-for="tpl in ['stdio', 'sse', 'streamable_http'] as const"
              :key="tpl"
              type="button"
              class="px-2.5 py-1 text-xs rounded-lg transition-all duration-150 cursor-pointer"
              :class="mcpTemplateType === tpl ? 'tpl-btn-active' : 'tpl-btn-inactive'"
              @click="applyTemplate(tpl)"
            >
              {{ tpl }}
            </button>
          </div>
        </div>
        <div class="rounded-xl overflow-hidden" style="border: 1px solid var(--primary-alpha-20)">
          <CodeMirror
            v-model="mcpForm.config"
            class="json-codemirror"
            :extensions="mcpEditorExtensions"
            :indent-with-tab="true"
            :tab-size="2"
            :style="{ height: '300px' }"
          />
        </div>
      </div>
    </div>
  </AppDialog>

  <AccountEditDialog
    v-model:visible="accountDialogVisible"
    @success="onAccountUpdated"
  />

  <AppDialog
    v-model:visible="webSearchDialogVisible"
    :title="t('webSearchSetting')"
    :confirm-text="t('confirm')"
    :cancel-text="t('cancel')"
    :confirm-loading="savingWebSearch"
    width="420px"
    @confirm="saveWebSearch"
    @cancel="closeWebSearchDialog"
  >
    <div class="flex flex-col gap-1.5">
      <label class="text-xs font-medium sb-text-muted">{{ t('apiKey') }}</label>
      <AppPasswordInput
        v-model="webSearchKey"
      />
    </div>
  </AppDialog>

  <AppDialog
    v-model:visible="messagePlatformDialogVisible"
    :title="t('messagePlatformBindTitle')"
    :confirm-text="t('confirm')"
    :cancel-text="t('cancel')"
    :confirm-loading="messagePlatformSubmitting"
    width="480px"
    @confirm="saveMessagePlatformConfig"
  >
    <div class="flex flex-col gap-3">
      <p class="text-xs settings-item-sub">{{ t('messagePlatformBindDesc') }}</p>
      <div class="flex flex-col gap-1.5">
        <label class="text-xs font-medium sb-text-muted">{{ t('botToken') }}</label>
        <AppPasswordInput v-model="messagePlatformForm.botToken" />
      </div>
    </div>
  </AppDialog>
</template>

<style>
@import './settings-panel.css';
</style>
