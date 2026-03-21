<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { mdiClose } from '@mdi/js'
import CodeMirror from 'vue-codemirror6'
import { json } from '@codemirror/lang-json'
import { oneDark } from '@codemirror/theme-one-dark'
import { lineNumbers } from '@codemirror/view'

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
import AccountEditDialog from '@/components/settings/AccountEditDialog.vue'
import { settingAPI } from '@/api/settings'
import { llmAPI } from '@/api/llm'
import { mcpAPI } from '@/api/mcp'
import { skillsAPI } from '@/api/skills'
import { messagePlatformAPI } from '@/api/messagePlatform'
import type { AppSettings } from '@/types/settings'
import { useToast } from '@/composables/useToast'
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

const tab = ref<'basic' | 'llm' | 'mcp' | 'skills' | 'platform'>('basic')
const llmList = ref<any[]>([])
const mcpList = ref<any[]>([])
const skillsList = ref<any[]>([])
const messagePlatformList = ref<any[]>([])
const loading = ref(false)
const llmDialogVisible = ref(false)
const mcpDialogVisible = ref(false)
const llmSubmitting = ref(false)
const mcpSubmitting = ref(false)
const mcpEditingID = ref('')
const skillsUploading = ref(false)
const skillsDropActive = ref(false)
const skillsFileInputRef = ref<HTMLInputElement | null>(null)
const accountDialogVisible = ref(false)
const messagePlatformDialogVisible = ref(false)
const messagePlatformSubmitting = ref(false)
const messagePlatformDefaultModel = ref('')

const llmForm = ref({ name: '', baseUrl: '', apiKey: '', model: '' })
const mcpForm = ref({ name: '', config: '', isEnabled: true })
const messagePlatformForm = ref({
  id: '',
  platform: 'telegram',
  displayName: 'Telegram',
  botToken: '',
  isEnabled: true,
})
const confirmDialogVisible = ref(false)
const confirmDialogCallback = ref<(() => Promise<void>) | null>(null)
const mcpTemplateType = ref<'stdio' | 'sse' | 'streamable_http'>('stdio')
const mcpEditorExtensions = [lineNumbers(), json(), oneDark]

const llmRows = computed(() => llmList.value || [])
const mcpRows = computed(() => mcpList.value || [])
const skillsRows = computed(() =>
  [...(skillsList.value || [])].sort((a, b) => {
    const aTime = new Date(a.uploadedAt || 0).getTime()
    const bTime = new Date(b.uploadedAt || 0).getTime()
    return bTime - aTime
  }),
)
const mcpDialogTitle = computed(() => (mcpEditingID.value ? t('editMcp') : t('addMcp')))
const messagePlatformModelOptions = computed(() => {
  const base = llmRows.value.map((item) => ({ value: item.id, label: item.name }))
  return [{ value: '', label: t('messagePlatformModelUnset') }, ...base]
})
const telegramConfig = computed(() => messagePlatformList.value.find((item: any) => item.platform === 'telegram'))

function getBotTokenFromAuthConfig(raw: string) {
  try {
    const parsed = JSON.parse(raw || '{}')
    return String(parsed?.botToken || '')
  } catch {
    return ''
  }
}

function buildPlatformAuthConfigJson(botToken: string) {
  return JSON.stringify({ botToken: botToken.trim() })
}

async function loadData() {
  loading.value = true
  try {
    await loadLanguage({ allowRemote: true })
    const appSettings: AppSettings = await settingAPI.get()
    messagePlatformDefaultModel.value = appSettings.messagePlatformDefaultModel || ''
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

function openLLMDialog() {
  llmForm.value = { name: '', baseUrl: '', apiKey: '', model: '' }
  llmDialogVisible.value = true
}

async function addLLM() {
  if (!llmForm.value.name || !llmForm.value.baseUrl || !llmForm.value.apiKey || !llmForm.value.model) {
    toast.error(t('llmFormIncomplete'))
    return
  }
  llmSubmitting.value = true
  try {
    await llmAPI.create(llmForm.value)
    llmForm.value = { name: '', baseUrl: '', apiKey: '', model: '' }
    llmList.value = await llmAPI.list()
    emit('llmChanged')
    llmDialogVisible.value = false
  } finally {
    llmSubmitting.value = false
  }
}

function openConfirmDialog(callback: () => Promise<void>) {
  confirmDialogCallback.value = callback
  confirmDialogVisible.value = true
}

async function runConfirmDialog() {
  if (confirmDialogCallback.value) await confirmDialogCallback.value()
  confirmDialogVisible.value = false
  confirmDialogCallback.value = null
}

function deleteLLM(id: string) {
  openConfirmDialog(async () => {
    await llmAPI.remove(id)
    llmList.value = await llmAPI.list()
    emit('llmChanged')
  })
}

function buildTemplate(transport: 'stdio' | 'sse' | 'streamable_http') {
  if (transport === 'stdio') {
    return JSON.stringify({ command: 'python', args: ['-m', 'your_module'] }, null, 2)
  }
  return JSON.stringify(
    { transport, url: 'https://your-mcp-server-url', headers: {}, timeout: 5, sse_read_timeout: 300 },
    null,
    2,
  )
}

function applyTemplate(transport: 'stdio' | 'sse' | 'streamable_http') {
  mcpTemplateType.value = transport
  mcpForm.value.config = buildTemplate(transport)
}

function openMCPDialog() {
  mcpEditingID.value = ''
  mcpForm.value = { name: '', config: buildTemplate('stdio'), isEnabled: true }
  mcpTemplateType.value = 'stdio'
  mcpDialogVisible.value = true
}

function openMCPEditDialog(item: any) {
  mcpEditingID.value = item.id
  mcpForm.value = { name: item.name, config: item.config, isEnabled: item.isEnabled }
  mcpDialogVisible.value = true
}

async function saveMCP() {
  if (!mcpForm.value.name || !mcpForm.value.config) {
    toast.error(t('mcpFormIncomplete'))
    return
  }
  let parsed: any
  try {
    parsed = JSON.parse(mcpForm.value.config)
  } catch {
    toast.error(t('mcpJsonInvalid'))
    return
  }
  if (parsed?.mcpServers) {
    toast.error(t('mcpWrapperNotSupported'))
    return
  }

  mcpSubmitting.value = true
  try {
    const payload = {
      name: mcpForm.value.name.trim(),
      config: JSON.stringify(parsed, null, 2),
      isEnabled: mcpForm.value.isEnabled,
    }
    if (mcpEditingID.value) {
      await mcpAPI.update(mcpEditingID.value, payload)
    } else {
      await mcpAPI.create(payload)
    }
    mcpForm.value = { name: '', config: buildTemplate('stdio'), isEnabled: true }
    mcpList.value = await mcpAPI.list()
    mcpDialogVisible.value = false
  } finally {
    mcpSubmitting.value = false
  }
}

async function updateMCP(item: any) {
  await mcpAPI.update(item.id, { name: item.name, config: item.config, isEnabled: item.isEnabled })
}

function mcpPreview(item: any) {
  try {
    const cfg = JSON.parse(item.config || '{}')
    const transport = cfg.transport || 'stdio'
    if (transport === 'stdio') return `${transport} · ${cfg.command || '-'}`
    return `${transport} · ${cfg.url || '-'}`
  } catch {
    return t('mcpJsonInvalid')
  }
}

function deleteMCP(id: string) {
  openConfirmDialog(async () => {
    await mcpAPI.remove(id)
    mcpList.value = await mcpAPI.list()
  })
}

function openSkillsPicker() {
  skillsFileInputRef.value?.click()
}

function getZipFiles(fileList: FileList | null | undefined) {
  if (!fileList) return []
  return Array.from(fileList).filter((file) => file.name.toLowerCase().endsWith('.zip'))
}

async function uploadSkills(files: File[]) {
  if (!files.length) return
  const invalidCount = files.filter((file) => !file.name.toLowerCase().endsWith('.zip')).length
  if (invalidCount > 0) {
    toast.error(t('onlyZipAllowed'))
    return
  }

  skillsUploading.value = true
  try {
    const result = await skillsAPI.upload(files)
    const failed = Array.isArray(result?.failed) ? result.failed : []
    if (failed.length > 0) {
      const detail = failed.map((x: any) => `${x.file}: ${x.error}`).join('\n')
      toast.error(`${t('skillsUploadPartial')}\n${detail}`)
    } else {
      toast.success(t('skillsUploadSuccess'))
    }
    skillsList.value = await skillsAPI.list()
  } catch (err: any) {
    const failed = err?.response?.data?.failed
    if (Array.isArray(failed) && failed.length > 0) {
      const detail = failed.map((x: any) => `${x.file}: ${x.error}`).join('\n')
      toast.error(`${t('skillsUploadFailed')}\n${detail}`)
    } else {
      toast.error(err?.response?.data?.error || t('skillsUploadFailed'))
    }
  } finally {
    skillsUploading.value = false
    if (skillsFileInputRef.value) skillsFileInputRef.value.value = ''
  }
}

function onSkillsInputChange(event: Event) {
  const target = event.target as HTMLInputElement
  const files = getZipFiles(target.files)
  if (!files.length && target.files?.length) {
    toast.error(t('onlyZipAllowed'))
    return
  }
  void uploadSkills(files)
}

function onSkillsDrop(event: DragEvent) {
  event.preventDefault()
  skillsDropActive.value = false
  const files = getZipFiles(event.dataTransfer?.files)
  if (!files.length && event.dataTransfer?.files?.length) {
    toast.error(t('onlyZipAllowed'))
    return
  }
  void uploadSkills(files)
}

function onSkillsDragOver(event: DragEvent) {
  event.preventDefault()
  skillsDropActive.value = true
}

function onSkillsDragLeave(event: DragEvent) {
  event.preventDefault()
  skillsDropActive.value = false
}

function deleteSkill(id: string) {
  openConfirmDialog(async () => {
    await skillsAPI.remove(id)
    skillsList.value = await skillsAPI.list()
  })
}

function openMessagePlatformDialog() {
  const row = telegramConfig.value
  if (!row) {
    messagePlatformForm.value = {
      id: '',
      platform: 'telegram',
      displayName: 'Telegram',
      botToken: '',
      isEnabled: true,
    }
  } else {
    messagePlatformForm.value = {
      id: row.id,
      platform: row.platform,
      displayName: row.displayName,
      botToken: getBotTokenFromAuthConfig(row.authConfigJson),
      isEnabled: !!row.isEnabled,
    }
  }
  messagePlatformDialogVisible.value = true
}

async function saveMessagePlatformConfig() {
  if (!messagePlatformForm.value.botToken.trim()) {
    toast.error(t('botTokenRequired'))
    return
  }
  messagePlatformSubmitting.value = true
  try {
    const payload = {
      platform: messagePlatformForm.value.platform,
      displayName: messagePlatformForm.value.displayName,
      authConfigJson: buildPlatformAuthConfigJson(messagePlatformForm.value.botToken),
      isEnabled: messagePlatformForm.value.isEnabled,
    }
    if (messagePlatformForm.value.id) {
      await messagePlatformAPI.update(messagePlatformForm.value.id, payload)
    } else {
      await messagePlatformAPI.create(payload)
    }
    messagePlatformList.value = await messagePlatformAPI.list()
    messagePlatformDialogVisible.value = false
  } finally {
    messagePlatformSubmitting.value = false
  }
}

async function toggleTelegramEnabled() {
  const row = telegramConfig.value
  if (!row) return
  await messagePlatformAPI.update(row.id, {
    platform: row.platform,
    displayName: row.displayName,
    authConfigJson: row.authConfigJson,
    isEnabled: !row.isEnabled,
  })
  messagePlatformList.value = await messagePlatformAPI.list()
}

async function saveMessagePlatformDefaultModel(modelId: string) {
  messagePlatformDefaultModel.value = modelId
  if (!modelId) return
  await settingAPI.update({ messagePlatformDefaultModel: modelId } as any)
}

onMounted(loadData)
</script>

<template>
  <div class="h-full flex flex-col overflow-hidden settings-root">
    <!-- 面板顶栏 -->
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

    <!-- 面板主体 -->
    <div class="flex flex-1 overflow-hidden settings-body">
      <!-- 左侧 Tab 导航 -->
      <aside class="w-44 flex-shrink-0 p-2.5 flex flex-col gap-1 overflow-y-auto settings-sidebar">
        <button
          v-for="item in [
            { key: 'basic', label: t('basicSettings') },
            { key: 'llm', label: t('llmSettings') },
            { key: 'mcp', label: t('mcpSettings') },
            { key: 'skills', label: t('skillsSettings') },
            { key: 'platform', label: t('messagePlatformSettings') },
          ]"
          :key="item.key"
          type="button"
          class="relative w-full text-left px-3.5 h-9 rounded-xl text-sm transition-all duration-150 cursor-pointer settings-tab"
          :class="tab === item.key ? 'settings-tab-active' : 'settings-tab-inactive'"
          @click="tab = item.key as any"
        >
          <span
            v-if="tab === item.key"
            class="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 rounded-r-full"
            style="background: #6366f1"
          />
          {{ item.label }}
        </button>
      </aside>

      <!-- 右侧内容 -->
      <section class="flex-1 min-w-0 overflow-y-auto px-5 py-5 relative settings-content">
        <div v-if="loading" class="absolute inset-0 flex items-center justify-center z-10" style="background: rgba(var(--bg-main), 0.7)">
          <LoadingSpinner />
        </div>

        <SettingsBasicTab
          v-if="tab === 'basic'"
          :language="language"
          :language-select-options="languageSelectOptions"
          :saving-language="savingLanguage"
          @open-account="openAccountDialog"
          @logout="logout"
          @language-change="onLanguageChange"
        />

        <SettingsLLMTab v-if="tab === 'llm'" :llm-rows="llmRows" @add="openLLMDialog" @delete="deleteLLM" />

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
      </section>
    </div>
  </div>

  <AppDialog
    v-model:visible="llmDialogVisible"
    :title="t('addModel')"
    :confirm-text="t('confirm')"
    :cancel-text="t('cancel')"
    :confirm-loading="llmSubmitting"
    width="440px"
    @confirm="addLLM"
  >
    <div class="flex flex-col gap-4">
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
