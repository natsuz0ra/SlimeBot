<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
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
import SettingsAgentsTab from '@/components/settings/SettingsAgentsTab.vue'
import SettingsMemoryTab from '@/components/settings/SettingsMemoryTab.vue'
import SettingsPlatformTab from '@/components/settings/SettingsPlatformTab.vue'
import SettingsUpdateTab from '@/components/settings/SettingsUpdateTab.vue'
import SettingsAboutTab from '@/components/settings/SettingsAboutTab.vue'
import AccountEditDialog from '@/components/settings/AccountEditDialog.vue'
import { llmAPI } from '@/api/llm'
import { mcpAPI } from '@/api/mcp'
import { settingAPI } from '@/api/settings'
import { memoryAPI } from '@/api/memory'
import { agentsInstructionsAPI } from '@/api/agentsInstructions'
import { skillsAPI } from '@/api/skills'
import { messagePlatformAPI } from '@/api/messagePlatform'
import type { AppSettings, ApprovalMode, LLMConfig, LLMProvider, MCPConfig, MemorySnapshot, MemoryTarget, MessagePlatformConfig, SandboxMode, SettingsTabKey, SkillItem, ThinkingLevel } from '@/types/settings'
import { useToast } from '@/composables/useToast'
import { useSettingsLLM } from '@/composables/settings/useSettingsLLM'
import { useSettingsMCP } from '@/composables/settings/useSettingsMCP'
import { useSettingsSkills } from '@/composables/settings/useSettingsSkills'
import { sortSkillRows } from '@/utils/skills'
import { useSettingsMessagePlatform } from '@/composables/settings/useSettingsMessagePlatform'
import { useSettingsConfirmDialog } from '@/composables/settings/useSettingsConfirmDialog'
import { useSettingsWebSearch } from '@/composables/settings/useSettingsWebSearch'
import { useLanguagePreference, type LanguageCode } from '@/composables/useLanguagePreference'
import { createWebSandboxModeOptions, toWebSandboxMode } from '@/utils/sandboxSettings'
import { useAuthStore } from '@/stores/auth'
import { useChatStore } from '@/stores/chat'
import { useRouter } from 'vue-router'
import type { UpdateCheckResult } from '@/types/update'

const emit = defineEmits<{
  close: []
  llmChanged: []
}>()

const props = defineProps<{
  hasUpdateNotice: boolean
  markUpdateNoticeRead: () => void
  setUpdateCheckResult: (result: UpdateCheckResult) => void
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
  { key: 'agents', labelKey: 'agentsSettings' },
  { key: 'memory', labelKey: 'memorySettings' },
  { key: 'platform', labelKey: 'messagePlatformSettings' },
  { key: 'update', labelKey: 'updateSettings' },
  { key: 'about', labelKey: 'aboutSettings' },
]

const tab = ref<SettingsTabKey>('basic')
const hasUpdateNotice = computed(() => props.hasUpdateNotice)
const llmList = ref<LLMConfig[]>([])
const providerList = ref<LLMProvider[]>([])
const mcpList = ref<MCPConfig[]>([])
const skillsList = ref<SkillItem[]>([])
const messagePlatformList = ref<MessagePlatformConfig[]>([])
const loading = ref(false)
const llmDialogVisible = ref(false)
const providerDialogVisible = ref(false)
const mcpDialogVisible = ref(false)
const llmSubmitting = ref(false)
const providerSubmitting = ref(false)
const confirmDeleteDescription = ref('confirmDeleteItem')
const mcpSubmitting = ref(false)
const skillsUploading = ref(false)
const skillsDropActive = ref(false)
const agentsInstructionsContent = ref('')
const agentsInstructionsPath = ref('')
const agentsInstructionsSaving = ref(false)
const sandboxMode = ref<SandboxMode>('workspace-write')
const sandboxNetworkEnabled = ref(true)
const skillsFileInputRef = ref<HTMLInputElement | null>(null)
const accountDialogVisible = ref(false)
const messagePlatformDialogVisible = ref(false)
const messagePlatformSubmitting = ref(false)
const messagePlatformDefaultModel = ref('')
watch(llmList, (models) => {
  if (messagePlatformDefaultModel.value && !models.some((model) => model.id === messagePlatformDefaultModel.value)) {
    messagePlatformDefaultModel.value = ''
  }
})
const messagePlatformThinkingLevel = ref<ThinkingLevel>('off')
const messagePlatformApprovalMode = ref<ApprovalMode>('standard')
const memoryEnabled = ref(true)
const memoryUserProfileEnabled = ref(true)
const memoryCharLimit = ref(2200)
const memoryUserCharLimit = ref(1375)
const memoryNudgeInterval = ref(10)
const memorySnapshot = ref<MemorySnapshot | null>(null)
const sandboxModeOptions = computed(() => createWebSandboxModeOptions((key) => t(key)))
const { confirmDialogVisible, openConfirmDialog, runConfirmDialog } = useSettingsConfirmDialog()
const {
  webSearchDialogVisible,
  webSearchKey,
  savingWebSearch,
  openWebSearchDialog,
  closeWebSearchDialog,
  saveWebSearch,
} = useSettingsWebSearch({ toast, t: (key) => t(key) })

const {
  providerForm,
  providerEditingId,
  modelForm,
  providerDialogTitleKey,
  modelDialogTitleKey,
  contextSizeDisplay,
  contextSizeManual,
  setContextSizeManual,
  llmRows,
  providerRows,
  discoveryProviderId,
  discoveredModels,
  visibleDiscovered,
  discovering,
  discoverySaving,
  discoveryQuery,
  selectedModelIds,
  openProviderDialog,
  openProviderEditDialog,
  saveProvider,
  deleteProvider: removeProvider,
  openModelDialog,
  openModelEditDialog,
  saveModel,
  deleteModel: removeLLM,
  discover,
  closeDiscovery,
  addDiscovered,
} = useSettingsLLM({
  llmList,
  providerList,
  providerDialogVisible,
  modelDialogVisible: llmDialogVisible,
  llmSubmitting,
  providerSubmitting,
  toast,
  t: (key) => t(key),
  onChanged: () => emit('llmChanged'),
})

const {
  mcpForm,
  mcpRows,
  mcpDialogTitle,
  mcpTemplateType,
  expandedMCPToolsID,
  mcpToolLoading,
  mcpToolResponses,
  mcpToolQueries,
  mcpEditorExtensions,
  applyTemplate,
  openMCPDialog,
  openMCPEditDialog,
  saveMCP,
  updateMCP,
  deleteMCP: removeMCP,
  mcpPreview,
  loadMCPTools,
  toggleMCPTools,
  setMCPToolsQuery,
  visibleMCPTools,
} = useSettingsMCP({
  mcpList,
  mcpDialogVisible,
  mcpSubmitting,
  toast,
  t: (key) => t(key),
})

const skillsRows = computed(() => sortSkillRows(skillsList.value || []))

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
  setSkillEnabled,
} = skillsActions

const {
  messagePlatformForm,
  telegramConfig,
  messagePlatformModelOptions,
  messagePlatformThinkingOptions,
  messagePlatformApprovalOptions,
  openMessagePlatformDialog,
  saveMessagePlatformConfig,
  toggleTelegramEnabled,
  saveMessagePlatformDefaultModel,
  saveMessagePlatformThinkingLevel,
  saveMessagePlatformApprovalMode,
} = useSettingsMessagePlatform({
  messagePlatformList,
  messagePlatformDialogVisible,
  messagePlatformSubmitting,
  messagePlatformDefaultModel,
  messagePlatformThinkingLevel,
  messagePlatformApprovalMode,
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
    messagePlatformThinkingLevel.value = appSettings.messagePlatformThinkingLevel || 'off'
    messagePlatformApprovalMode.value = appSettings.messagePlatformApprovalMode || 'standard'
    webSearchKey.value = appSettings.webSearchKey || ''
    sandboxMode.value = toWebSandboxMode(appSettings.sandboxMode || 'workspace-write')
    sandboxNetworkEnabled.value = appSettings.sandboxNetworkEnabled !== undefined ? appSettings.sandboxNetworkEnabled : true
    memoryEnabled.value = appSettings.memoryEnabled !== undefined ? appSettings.memoryEnabled : true
    memoryUserProfileEnabled.value = appSettings.memoryUserProfileEnabled !== undefined ? appSettings.memoryUserProfileEnabled : true
    memoryCharLimit.value = appSettings.memoryCharLimit || 2200
    memoryUserCharLimit.value = appSettings.memoryUserCharLimit || 1375
    memoryNudgeInterval.value = appSettings.memoryNudgeInterval || 10
    const [providers, models] = await Promise.all([llmAPI.providers(), llmAPI.list()])
    providerList.value = providers
    llmList.value = models
    if (messagePlatformDefaultModel.value && !models.some((model) => model.id === messagePlatformDefaultModel.value)) {
      messagePlatformDefaultModel.value = ''
      await settingAPI.update({ messagePlatformDefaultModel: '' })
    }
    mcpList.value = await mcpAPI.list()
    skillsList.value = await skillsAPI.list()
    const agentsInstructions = await agentsInstructionsAPI.get()
    agentsInstructionsContent.value = agentsInstructions.content
    agentsInstructionsPath.value = agentsInstructions.path
    messagePlatformList.value = await messagePlatformAPI.list()
    await loadMemorySnapshot()
  } finally {
    loading.value = false
  }
}

async function loadMemorySnapshot() {
  memorySnapshot.value = await memoryAPI.get()
}

async function onLanguageChange(nextLanguage: LanguageCode) {
  await changeLanguage(nextLanguage, { allowRemote: true, showSuccessToast: true })
}

async function onSandboxModeChange(nextMode: SandboxMode) {
  const previousMode = sandboxMode.value
  sandboxMode.value = nextMode
  try {
    await settingAPI.update({ sandboxMode: nextMode })
  } catch (err: unknown) {
    sandboxMode.value = previousMode
    const response = err as { response?: { data?: { error?: string } } }
    toast.error(response.response?.data?.error || t('sandboxSaveFailed'))
  }
}

async function onSandboxNetworkChange(enabled: boolean) {
  const previousEnabled = sandboxNetworkEnabled.value
  sandboxNetworkEnabled.value = enabled
  try {
    await settingAPI.update({ sandboxNetworkEnabled: enabled })
  } catch (err: unknown) {
    sandboxNetworkEnabled.value = previousEnabled
    const response = err as { response?: { data?: { error?: string } } }
    toast.error(response.response?.data?.error || t('sandboxSaveFailed'))
  }
}

async function saveMemorySetting<K extends keyof AppSettings>(key: K, value: AppSettings[K], rollback: () => void) {
  try {
    await settingAPI.update({ [key]: value } as Partial<AppSettings>)
    await loadMemorySnapshot()
  } catch (err: unknown) {
    rollback()
    const response = err as { response?: { data?: { error?: string } } }
    toast.error(response.response?.data?.error || t('memorySaveFailed'))
  }
}

function onMemoryEnabledChange(enabled: boolean) {
  const previous = memoryEnabled.value
  memoryEnabled.value = enabled
  void saveMemorySetting('memoryEnabled', enabled, () => {
    memoryEnabled.value = previous
  })
}

function onMemoryUserProfileEnabledChange(enabled: boolean) {
  const previous = memoryUserProfileEnabled.value
  memoryUserProfileEnabled.value = enabled
  void saveMemorySetting('memoryUserProfileEnabled', enabled, () => {
    memoryUserProfileEnabled.value = previous
  })
}

function onMemoryCharLimitChange(value: number) {
  const previous = memoryCharLimit.value
  memoryCharLimit.value = value
  void saveMemorySetting('memoryCharLimit', value, () => {
    memoryCharLimit.value = previous
  })
}

function onMemoryUserCharLimitChange(value: number) {
  const previous = memoryUserCharLimit.value
  memoryUserCharLimit.value = value
  void saveMemorySetting('memoryUserCharLimit', value, () => {
    memoryUserCharLimit.value = previous
  })
}

function onMemoryNudgeIntervalChange(value: number) {
  const previous = memoryNudgeInterval.value
  memoryNudgeInterval.value = value
  void saveMemorySetting('memoryNudgeInterval', value, () => {
    memoryNudgeInterval.value = previous
  })
}

function clearMemoryTarget(target: MemoryTarget | 'all') {
  openConfirmDialog(async () => {
    await memoryAPI.clear(target)
    await loadMemorySnapshot()
    toast.success(t('memoryUpdated'))
  })
}

function deleteMemoryEntry(target: MemoryTarget, index: number) {
  openConfirmDialog(async () => {
    await memoryAPI.deleteEntry(target, index)
    await loadMemorySnapshot()
    toast.success(t('memoryUpdated'))
  })
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
  confirmDeleteDescription.value = 'confirmDeleteItem'
  openConfirmDialog(async () => {
    await removeLLM(id)
  })
}

function deleteProvider(id: string) {
  confirmDeleteDescription.value = 'confirmDeleteProvider'
  openConfirmDialog(async () => {
    await removeProvider(id)
  })
}

function toggleDiscoveredModel(id: string) {
  selectedModelIds.value = selectedModelIds.value.includes(id)
    ? selectedModelIds.value.filter(item => item !== id)
    : [...selectedModelIds.value, id]
}

function toggleAllDiscovered(ids: string[]) {
  const selected = new Set(selectedModelIds.value)
  const allSelected = ids.every(id => selected.has(id))
  ids.forEach(id => allSelected ? selected.delete(id) : selected.add(id))
  selectedModelIds.value = [...selected]
}

function deleteMCP(id: string) {
  confirmDeleteDescription.value = 'confirmDeleteItem'
  openConfirmDialog(async () => {
    await removeMCP(id)
  })
}

function deleteSkill(id: string) {
  confirmDeleteDescription.value = 'confirmDeleteItem'
  openConfirmDialog(async () => {
    await removeSkill(id)
  })
}

function toggleSkillEnabled(id: string, enabled: boolean) {
  void setSkillEnabled(id, enabled)
}

async function saveAgentsInstructions() {
  agentsInstructionsSaving.value = true
  try {
    await agentsInstructionsAPI.update(agentsInstructionsContent.value)
    toast.success(t('saveSuccess'))
  } catch (err: unknown) {
    const response = err as { response?: { data?: { error?: string } } }
    toast.error(response.response?.data?.error || t('agentsSaveFailed'))
  } finally {
    agentsInstructionsSaving.value = false
  }
}

onMounted(loadData)

watch(tab, (nextTab) => {
  if (nextTab === 'update') {
    props.markUpdateNoticeRead()
  }
})
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
        <span class="settings-title">{{ t('settings') }}</span>
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
          class="relative w-full text-left px-3.5 h-9 rounded-xl transition-all duration-150 cursor-pointer settings-tab flex items-center gap-2"
          :class="tab === item.key ? 'settings-tab-active' : 'settings-tab-inactive'"
          @click="tab = item.key"
        >
          <span
            v-if="tab === item.key"
            class="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 rounded-r-full"
            style="background: #6366f1"
          />
          <span class="settings-tab-label">{{ t(item.labelKey) }}</span>
          <span v-if="item.key === 'update' && hasUpdateNotice" class="update-notice-dot" aria-hidden="true" />
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
          :sandbox-mode="sandboxMode"
          :sandbox-mode-options="sandboxModeOptions"
          :sandbox-network-enabled="sandboxNetworkEnabled"
          @open-account="openAccountDialog"
          @open-web-search="openWebSearchDialog"
          @logout="logout"
          @language-change="onLanguageChange"
          @sandbox-mode-change="onSandboxModeChange"
          @sandbox-network-change="onSandboxNetworkChange"
        />

        <SettingsLLMTab
          v-if="tab === 'llm'"
          :providers="providerRows"
          :models="llmRows"
          :discovery-provider-id="discoveryProviderId"
          :discovered-models="discoveredModels"
          :visible-discovered="visibleDiscovered"
          :discovering="discovering"
          :discovery-saving="discoverySaving"
          :discovery-query="discoveryQuery"
          :selected-model-ids="selectedModelIds"
          @add-provider="openProviderDialog"
          @edit-provider="openProviderEditDialog"
          @delete-provider="deleteProvider"
          @add-model="openModelDialog"
          @edit-model="openModelEditDialog"
          @delete-model="deleteLLM"
          @discover="discover"
          @close-discovery="closeDiscovery"
          @update-query="discoveryQuery = $event"
          @toggle-model="toggleDiscoveredModel"
          @toggle-all="toggleAllDiscovered"
          @add-selected="addDiscovered"
        />

        <SettingsMCPTab
          v-if="tab === 'mcp'"
          :mcp-rows="mcpRows"
          :mcp-preview="mcpPreview"
          :update-mcp="updateMCP"
          :expanded-mcp-tools-id="expandedMCPToolsID"
          :mcp-tool-loading="mcpToolLoading"
          :mcp-tool-responses="mcpToolResponses"
          :mcp-tool-queries="mcpToolQueries"
          :visible-mcp-tools="visibleMCPTools"
          :toggle-mcp-tools="toggleMCPTools"
          :load-mcp-tools="loadMCPTools"
          :set-mcp-tools-query="setMCPToolsQuery"
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
          @toggle-enabled="toggleSkillEnabled"
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

        <SettingsAgentsTab
          v-if="tab === 'agents'"
          v-model:content="agentsInstructionsContent"
          :path="agentsInstructionsPath"
          :saving="agentsInstructionsSaving"
          @save="saveAgentsInstructions"
        />

        <SettingsMemoryTab
          v-if="tab === 'memory'"
          :memory-enabled="memoryEnabled"
          :memory-user-profile-enabled="memoryUserProfileEnabled"
          :memory-char-limit="memoryCharLimit"
          :memory-user-char-limit="memoryUserCharLimit"
          :memory-nudge-interval="memoryNudgeInterval"
          :memory-snapshot="memorySnapshot"
          @memory-enabled-change="onMemoryEnabledChange"
          @memory-user-profile-enabled-change="onMemoryUserProfileEnabledChange"
          @memory-char-limit-change="onMemoryCharLimitChange"
          @memory-user-char-limit-change="onMemoryUserCharLimitChange"
          @memory-nudge-interval-change="onMemoryNudgeIntervalChange"
          @delete-entry="deleteMemoryEntry"
          @clear-target="clearMemoryTarget"
        />

        <SettingsPlatformTab
          v-if="tab === 'platform'"
          :message-platform-default-model="messagePlatformDefaultModel"
          :message-platform-model-options="messagePlatformModelOptions"
          :message-platform-thinking-level="messagePlatformThinkingLevel"
          :message-platform-thinking-options="messagePlatformThinkingOptions"
          :message-platform-approval-mode="messagePlatformApprovalMode"
          :message-platform-approval-options="messagePlatformApprovalOptions"
          :llm-rows-empty="llmRows.length === 0"
          :telegram-config="telegramConfig"
          @update:message-platform-default-model="saveMessagePlatformDefaultModel($event)"
          @update:message-platform-thinking-level="saveMessagePlatformThinkingLevel($event)"
          @update:message-platform-approval-mode="saveMessagePlatformApprovalMode($event)"
          @toggle-telegram="toggleTelegramEnabled"
          @open-bind="openMessagePlatformDialog"
        />

        <SettingsUpdateTab v-if="tab === 'update'" @update-check-loaded="props.setUpdateCheckResult" />

        <SettingsAboutTab v-if="tab === 'about'" />
      </section>
    </div>
  </div>

  <AppDialog
    v-model:visible="providerDialogVisible"
    :title="t(providerDialogTitleKey)"
    :confirm-text="t('confirm')"
    :cancel-text="t('cancel')"
    :confirm-loading="providerSubmitting"
    width="440px"
    @confirm="saveProvider"
  >
    <div class="flex flex-col gap-4">
      <div class="flex flex-col gap-1.5">
        <label class="settings-dialog-label">{{ t('provider') }}</label>
        <div class="flex gap-3">
          <label class="flex items-center gap-1.5 cursor-pointer settings-radio-label">
            <input type="radio" v-model="providerForm.protocol" value="openai" class="accent-[#6366f1]" />
            {{ t('providerOpenAI') }}
          </label>
          <label class="flex items-center gap-1.5 cursor-pointer settings-radio-label">
            <input type="radio" v-model="providerForm.protocol" value="anthropic" class="accent-[#6366f1]" />
            {{ t('providerAnthropic') }}
          </label>
          <label class="flex items-center gap-1.5 cursor-pointer settings-radio-label">
            <input type="radio" v-model="providerForm.protocol" value="deepseek" class="accent-[#6366f1]" />
            {{ t('providerDeepSeek') }}
          </label>
        </div>
      </div>
      <div class="flex flex-col gap-1.5">
        <label for="llm-provider-name" class="settings-dialog-label">{{ t('name') }}</label>
        <AppTextInput id="llm-provider-name" v-model="providerForm.name" />
      </div>
      <div class="flex flex-col gap-1.5">
        <label for="llm-provider-url" class="settings-dialog-label">{{ t('baseUrl') }}</label>
        <AppTextInput id="llm-provider-url" v-model="providerForm.baseUrl" placeholder="https://api.example.com/v1" />
      </div>
      <div class="flex flex-col gap-1.5">
        <label for="llm-provider-key" class="settings-dialog-label">{{ t('apiKey') }}</label>
        <AppPasswordInput id="llm-provider-key" v-model="providerForm.apiKey" :disabled="providerForm.clearApiKey" autocomplete="new-password" />
        <span class="settings-item-meta">{{ t(providerEditingId ? 'providerKeyHint' : 'providerKeyOptionalHint') }}</span>
        <label v-if="providerEditingId && providerRows.find(item => item.id === providerEditingId)?.hasApiKey" class="flex items-center gap-2 settings-radio-label cursor-pointer">
          <input v-model="providerForm.clearApiKey" type="checkbox" class="accent-[#6366f1]" />
          {{ t('clearProviderKey') }}
        </label>
      </div>
    </div>
  </AppDialog>

  <AppDialog
    v-model:visible="llmDialogVisible"
    :title="t(modelDialogTitleKey)"
    :confirm-text="t('confirm')"
    :cancel-text="t('cancel')"
    :confirm-loading="llmSubmitting"
    width="440px"
    @confirm="saveModel"
  >
    <div class="flex flex-col gap-4">
      <div class="flex flex-col gap-1.5">
        <label for="llm-model-name" class="settings-dialog-label">{{ t('name') }}</label>
        <AppTextInput id="llm-model-name" v-model="modelForm.name" />
      </div>
      <div class="flex flex-col gap-1.5">
        <label for="llm-model-id" class="settings-dialog-label">{{ t('modelId') }}</label>
        <AppTextInput id="llm-model-id" v-model="modelForm.model" />
      </div>
      <div class="flex flex-col gap-2">
        <div class="flex items-center justify-between gap-3">
          <label class="settings-dialog-label">{{ t('contextSize') }}</label>
          <span class="settings-item-meta">{{ contextSizeDisplay }}</span>
        </div>
        <div class="context-size-modes" role="group" :aria-label="t('contextSize')">
          <button type="button" :class="{ active: !contextSizeManual }" @click="setContextSizeManual(false)">{{ t('contextSizeAuto') }}</button>
          <button type="button" :class="{ active: contextSizeManual }" @click="setContextSizeManual(true)">{{ t('contextSizeCustom') }}</button>
        </div>
        <div v-if="contextSizeManual" class="flex items-center gap-2">
          <input
            v-model.number="modelForm.contextSize"
            type="number"
            min="8000"
            max="1000000"
            step="1000"
            class="context-size-input"
          />
          <span class="settings-item-meta">tokens</span>
        </div>
        <span v-else class="settings-item-meta">{{ t('contextSizeAutoHint') }}</span>
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
    <p class="settings-item-sub">{{ t(confirmDeleteDescription) }}</p>
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
        <label class="settings-dialog-label">{{ t('name') }}</label>
        <AppTextInput v-model="mcpForm.name" />
      </div>

      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label class="settings-dialog-label">{{ t('mcpConfigJson') }}</label>
          <div class="flex items-center gap-1">
            <button
              v-for="tpl in ['stdio', 'sse', 'streamable_http'] as const"
              :key="tpl"
              type="button"
              class="px-2.5 py-1 rounded-lg transition-all duration-150 cursor-pointer settings-action-text"
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
            class="json-codemirror mcp-codemirror"
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
      <label class="settings-dialog-label">{{ t('apiKey') }}</label>
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
      <p class="settings-item-sub">{{ t('messagePlatformBindDesc') }}</p>
      <div class="flex flex-col gap-1.5">
        <label class="settings-dialog-label">{{ t('botToken') }}</label>
        <AppPasswordInput v-model="messagePlatformForm.botToken" />
      </div>
    </div>
  </AppDialog>
</template>

<style>
@import './settings-panel.css';
</style>
