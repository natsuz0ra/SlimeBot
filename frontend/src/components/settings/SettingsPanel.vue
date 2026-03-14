<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { mdiClose, mdiDeleteOutline, mdiPencilOutline, mdiPlus } from '@mdi/js'
import CodeMirror from 'vue-codemirror6'
import { json } from '@codemirror/lang-json'
import { oneDark } from '@codemirror/theme-one-dark'
import { lineNumbers } from '@codemirror/view'

import MdiIcon from '@/components/MdiIcon.vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import { llmAPI, mcpAPI, settingAPI, skillsAPI } from '@/api/settings'
import { useToast } from '@/composables/useToast'

const emit = defineEmits<{
  close: []
  llmChanged: []
}>()

const { t, locale } = useI18n()
const toast = useToast()

const tab = ref<'basic' | 'llm' | 'mcp' | 'skills'>('basic')
const language = ref<'zh-CN' | 'en-US'>('zh-CN')
const llmList = ref<any[]>([])
const mcpList = ref<any[]>([])
const skillsList = ref<any[]>([])
const loading = ref(false)
const savingLanguage = ref(false)
const llmDialogVisible = ref(false)
const mcpDialogVisible = ref(false)
const llmSubmitting = ref(false)
const mcpSubmitting = ref(false)
const mcpEditingID = ref('')
const skillsUploading = ref(false)
const skillsDropActive = ref(false)
const skillsFileInputRef = ref<HTMLInputElement | null>(null)

const llmForm = ref({ name: '', baseUrl: '', apiKey: '', model: '' })
const mcpForm = ref({ name: '', config: '', isEnabled: true })
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

async function loadData() {
  loading.value = true
  try {
    const settings = await settingAPI.get()
    language.value = settings.language || 'zh-CN'
    locale.value = language.value
    llmList.value = await llmAPI.list()
    mcpList.value = await mcpAPI.list()
    skillsList.value = await skillsAPI.list()
  } finally {
    loading.value = false
  }
}

async function onLanguageChange(nextLanguage: 'zh-CN' | 'en-US') {
  if (savingLanguage.value) return
  const previousLanguage = locale.value as 'zh-CN' | 'en-US'
  if (nextLanguage === previousLanguage) return

  language.value = nextLanguage
  locale.value = nextLanguage
  savingLanguage.value = true
  try {
    await settingAPI.update({ language: nextLanguage })
    toast.success(t('saveSuccess'))
  } catch {
    language.value = previousLanguage
    locale.value = previousLanguage
    toast.error(t('languageSaveFailed'))
  } finally {
    savingLanguage.value = false
  }
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
        <!-- 加载遮罩 -->
        <div v-if="loading" class="absolute inset-0 flex items-center justify-center z-10" style="background: rgba(var(--bg-main), 0.7)">
          <svg class="animate-spin w-5 h-5" style="color: #6366f1" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
          </svg>
        </div>

        <!-- ── 基础设置 ── -->
        <div v-if="tab === 'basic'">
          <p class="section-label">{{ t('basicSettings') }}</p>
          <div class="settings-card flex items-center justify-between px-4 py-3.5 rounded-xl">
            <span class="text-sm settings-field-label">{{ t('language') }}</span>
            <AppSelect
              :model-value="language"
              :options="[{ value: 'zh-CN', label: t('chinese') }, { value: 'en-US', label: t('english') }]"
              :disabled="savingLanguage"
              @update:model-value="onLanguageChange($event as any)"
            />
          </div>
        </div>

        <!-- ── LLM 设置 ── -->
        <div v-if="tab === 'llm'">
          <div class="flex items-center justify-between mb-4">
            <p class="section-label mb-0">{{ t('llmSettings') }}</p>
            <button
              type="button"
              class="action-btn flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-xl cursor-pointer"
              @click="openLLMDialog"
            >
              <MdiIcon :path="mdiPlus" :size="13" />
              {{ t('add') }}
            </button>
          </div>
          <div class="flex flex-col gap-2">
            <div
              v-for="item in llmRows"
              :key="item.id"
              class="settings-card flex items-center gap-3 px-4 py-3.5 rounded-xl"
            >
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium settings-item-name truncate">
                  {{ item.name }}
                  <span class="font-normal settings-item-meta"> · {{ item.model }}</span>
                </div>
                <div class="text-xs settings-item-sub truncate mt-0.5">{{ item.baseUrl }}</div>
              </div>
              <button
                type="button"
                class="flex-shrink-0 w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer delete-btn"
                @click="deleteLLM(item.id)"
              >
                <MdiIcon :path="mdiDeleteOutline" :size="15" />
              </button>
            </div>
            <div v-if="llmRows.length === 0" class="empty-state text-center py-10 text-sm rounded-xl">
              {{ t('add') }} LLM
            </div>
          </div>
        </div>

        <!-- ── MCP 设置 ── -->
        <div v-if="tab === 'mcp'">
          <div class="flex items-center justify-between mb-4">
            <p class="section-label mb-0">{{ t('mcpSettings') }}</p>
            <button
              type="button"
              class="action-btn flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-xl cursor-pointer"
              @click="openMCPDialog"
            >
              <MdiIcon :path="mdiPlus" :size="13" />
              {{ t('add') }}
            </button>
          </div>
          <div class="flex flex-col gap-2">
            <div
              v-for="item in mcpRows"
              :key="item.id"
              class="settings-card flex items-center gap-3 px-4 py-3.5 rounded-xl"
            >
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium settings-item-name">{{ item.name }}</div>
                <div class="text-xs settings-item-sub mt-0.5">{{ mcpPreview(item) }}</div>
              </div>
              <div class="flex items-center gap-2 flex-shrink-0">
                <!-- 开关 -->
                <button
                  type="button"
                  class="relative w-9 h-5 rounded-full transition-all duration-200 cursor-pointer flex-shrink-0"
                  :class="item.isEnabled ? 'mcp-toggle-on' : 'mcp-toggle-off'"
                  @click="item.isEnabled = !item.isEnabled; updateMCP(item)"
                >
                  <span
                    class="absolute top-0.5 left-0.5 w-4 h-4 bg-white rounded-full shadow-sm transition-transform duration-200"
                    :class="item.isEnabled ? 'translate-x-4' : 'translate-x-0'"
                  />
                </button>
                <!-- 编辑 -->
                <button
                  type="button"
                  class="w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer edit-btn"
                  @click="openMCPEditDialog(item)"
                >
                  <MdiIcon :path="mdiPencilOutline" :size="14" />
                </button>
                <!-- 删除 -->
                <button
                  type="button"
                  class="w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer delete-btn"
                  @click="deleteMCP(item.id)"
                >
                  <MdiIcon :path="mdiDeleteOutline" :size="15" />
                </button>
              </div>
            </div>
            <div v-if="mcpRows.length === 0" class="empty-state text-center py-10 text-sm rounded-xl">
              {{ t('add') }} MCP
            </div>
          </div>
        </div>

        <!-- ── Skills 设置 ── -->
        <div v-if="tab === 'skills'">
          <div class="flex items-center justify-between mb-1">
            <p class="section-label mb-0">{{ t('skillsSettings') }}</p>
            <button
              type="button"
              class="action-btn flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-xl cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              :disabled="skillsUploading"
              @click="openSkillsPicker"
            >
              <svg v-if="skillsUploading" class="animate-spin w-3 h-3" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
              </svg>
              <MdiIcon v-else :path="mdiPlus" :size="13" />
              {{ t('skillsUploadButton') }}
            </button>
          </div>
          <p class="text-xs mb-3 settings-item-sub">{{ t('skillsDescription') }}</p>

          <!-- 拖拽上传区 -->
          <div
            class="drop-zone rounded-xl px-4 py-6 text-center text-sm cursor-pointer transition-all duration-200 mb-3"
            :class="skillsDropActive ? 'drop-zone-active' : 'drop-zone-idle'"
            @dragover="onSkillsDragOver"
            @dragleave="onSkillsDragLeave"
            @drop="onSkillsDrop"
            @click="openSkillsPicker"
          >
            {{ t('skillsUploadHint') }}
          </div>
          <input
            ref="skillsFileInputRef"
            type="file"
            class="hidden"
            accept=".zip,application/zip"
            multiple
            @change="onSkillsInputChange"
          />

          <div v-if="skillsRows.length === 0" class="empty-state text-center py-8 text-sm rounded-xl">
            {{ t('skillsEmpty') }}
          </div>
          <div v-else class="flex flex-col gap-2">
            <div
              v-for="item in skillsRows"
              :key="item.id"
              class="settings-card flex items-start gap-3 px-4 py-3.5 rounded-xl"
            >
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium settings-item-name">{{ item.name }}</div>
                <div v-if="item.description" class="text-xs settings-item-sub mt-0.5">{{ item.description }}</div>
                <div v-if="item.relativePath" class="text-xs mt-0.5 font-mono" style="color: var(--text-muted)">{{ item.relativePath }}</div>
              </div>
              <button
                type="button"
                class="flex-shrink-0 w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer delete-btn"
                @click="deleteSkill(item.id)"
              >
                <MdiIcon :path="mdiDeleteOutline" :size="15" />
              </button>
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>

  <!-- 新增 LLM 弹窗 -->
  <BaseDialog
    v-model:visible="llmDialogVisible"
    :title="t('addModel')"
    :confirm-text="t('confirm')"
    :cancel-text="t('cancel')"
    :confirm-loading="llmSubmitting"
    width="440px"
    @confirm="addLLM"
  >
    <div class="flex flex-col gap-4">
      <div
        v-for="field in [
          { key: 'name', label: t('name'), type: 'text' },
          { key: 'model', label: t('model'), type: 'text' },
          { key: 'baseUrl', label: t('baseUrl'), type: 'text' },
          { key: 'apiKey', label: t('apiKey'), type: 'password' },
        ]"
        :key="field.key"
        class="flex flex-col gap-1.5"
      >
        <label class="text-xs font-medium" style="color: var(--text-muted)">{{ field.label }}</label>
        <input
          v-model="(llmForm as any)[field.key]"
          :type="field.type"
          class="settings-input px-3 py-2.5 text-sm rounded-xl outline-none transition-all duration-150"
        />
      </div>
    </div>
  </BaseDialog>

  <!-- 通用确认弹窗 -->
  <BaseDialog
    v-model:visible="confirmDialogVisible"
    :title="t('delete')"
    :confirm-text="t('confirm')"
    :cancel-text="t('cancel')"
    :confirm-danger="true"
    width="360px"
    @confirm="runConfirmDialog"
  >
    <p class="text-sm" style="color: var(--text-secondary)">{{ t('confirmDeleteItem') }}</p>
  </BaseDialog>

  <!-- 新增/编辑 MCP 弹窗 -->
  <BaseDialog
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
        <label class="text-xs font-medium" style="color: var(--text-muted)">{{ t('name') }}</label>
        <input
          v-model="mcpForm.name"
          type="text"
          class="settings-input px-3 py-2.5 text-sm rounded-xl outline-none transition-all duration-150"
        />
      </div>

      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label class="text-xs font-medium" style="color: var(--text-muted)">{{ t('mcpConfigJson') }}</label>
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
        <div class="rounded-xl overflow-hidden" style="border: 1px solid rgba(99,102,241,0.2)">
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
  </BaseDialog>
</template>

<style scoped>
/* Root */
.settings-root {
  background: var(--bg-main);
}

/* Header */
.settings-header {
  border-bottom: 1px solid var(--card-border);
  background: var(--header-bg);
  height: 52px;
}

.settings-title {
  color: var(--text-primary);
}

.settings-close-btn {
  color: var(--text-muted);
}
.settings-close-btn:hover {
  background: rgba(99, 102, 241, 0.08);
  color: var(--text-primary);
}

/* Sidebar */
.settings-sidebar {
  border-right: 1px solid var(--card-border);
  background: var(--sidebar-bg);
}

.settings-tab-active {
  background: rgba(99, 102, 241, 0.12);
  color: #6366f1;
  font-weight: 600;
}
.settings-tab-inactive {
  color: var(--text-secondary);
}
.settings-tab-inactive:hover {
  background: rgba(99, 102, 241, 0.07);
  color: var(--text-primary);
}

/* Content */
.settings-content {
  background: var(--bg-main);
}

/* Section label */
.section-label {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-muted);
  margin-bottom: 12px;
}

/* Cards */
.settings-card {
  background: var(--card-bg);
  border: 1px solid var(--card-border);
}

.settings-field-label {
  color: var(--text-primary);
}

.settings-item-name {
  color: var(--text-primary);
}
.settings-item-meta {
  color: var(--text-muted);
}
.settings-item-sub {
  color: var(--text-muted);
}

/* Buttons */
.action-btn {
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  color: white;
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.3);
  transition: box-shadow 0.15s, transform 0.15s;
}
.action-btn:not(:disabled):hover {
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.4);
  transform: translateY(-1px);
}

.edit-btn {
  color: var(--text-muted);
}
.edit-btn:hover {
  background: rgba(99, 102, 241, 0.1);
  color: #6366f1;
}

.delete-btn {
  color: var(--text-muted);
}
.delete-btn:hover {
  background: rgba(239, 68, 68, 0.1);
  color: #ef4444;
}

/* MCP toggle */
.mcp-toggle-on {
  background: #6366f1;
  box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.2);
}
.mcp-toggle-off {
  background: rgba(99, 102, 241, 0.15);
}

/* Empty state */
.empty-state {
  border: 1px dashed var(--card-border);
  color: var(--text-muted);
}

/* Drop zone */
.drop-zone {
  border: 1.5px dashed var(--card-border);
  color: var(--text-muted);
}
.drop-zone-idle:hover {
  border-color: rgba(99, 102, 241, 0.4);
  background: rgba(99, 102, 241, 0.04);
  color: #6366f1;
}
.drop-zone-active {
  border-color: #6366f1;
  background: rgba(99, 102, 241, 0.08);
  color: #6366f1;
}

/* Input fields */
.settings-input {
  background: var(--input-bg);
  border: 1px solid var(--input-border);
  color: var(--text-primary);
  width: 100%;
}
.settings-input:focus {
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.12);
}
.settings-input::placeholder {
  color: var(--text-muted);
}

/* Template buttons */
.tpl-btn-active {
  background: rgba(99, 102, 241, 0.15);
  color: #6366f1;
  border: 1px solid rgba(99, 102, 241, 0.3);
  font-weight: 500;
}
.tpl-btn-inactive {
  background: var(--input-bg);
  border: 1px solid var(--input-border);
  color: var(--text-secondary);
}
.tpl-btn-inactive:hover {
  background: rgba(99, 102, 241, 0.07);
  color: var(--text-primary);
}

/* CodeMirror */
.json-codemirror :deep(.cm-editor) {
  height: 100%;
  background: #1e1e1e;
}
.json-codemirror :deep(.cm-scroller) {
  font-family: 'Consolas', 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.6;
}
.json-codemirror :deep(.cm-gutters) {
  background: #1e1e1e;
  border-right: 1px solid #313131;
}

/* Responsive */
@media (max-width: 640px) {
  .settings-body {
    flex-direction: column;
    min-height: 0;
  }

  .settings-sidebar {
    width: 100% !important;
    flex-direction: row !important;
    overflow-x: auto;
    overflow-y: hidden;
    padding: 8px 10px;
    gap: 6px;
    border-right: none !important;
    border-bottom: 1px solid var(--card-border);
  }

  .settings-tab {
    width: auto;
    min-width: max-content;
    flex: 0 0 auto;
    white-space: nowrap;
  }

  .settings-content {
    min-width: 0;
    min-height: 0;
    flex: 1 1 auto;
    padding: 12px 14px;
  }
}
</style>
