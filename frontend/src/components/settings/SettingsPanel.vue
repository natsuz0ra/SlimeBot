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
  <div class="h-full bg-gray-50 flex flex-col overflow-hidden">
    <!-- 面板顶栏 -->
    <div class="flex items-center justify-between px-4 h-11 border-b border-gray-200 bg-white flex-shrink-0">
      <span class="text-sm font-semibold text-gray-800">{{ t('settings') }}</span>
      <button
        type="button"
        class="w-7 h-7 flex items-center justify-center rounded-md text-gray-400 hover:text-gray-600 hover:bg-gray-100 transition-colors duration-150 cursor-pointer"
        @click="emit('close')"
      >
        <MdiIcon :path="mdiClose" :size="16" />
      </button>
    </div>

    <!-- 面板主体 -->
    <div class="flex flex-1 overflow-hidden">
      <!-- 左侧 Tab 菜单 -->
      <aside class="w-40 border-r border-gray-200 flex-shrink-0 bg-gray-50 p-2 flex flex-col gap-1 overflow-y-auto">
        <button
          v-for="item in [
            { key: 'basic', label: t('basicSettings') },
            { key: 'llm', label: t('llmSettings') },
            { key: 'mcp', label: t('mcpSettings') },
            { key: 'skills', label: t('skillsSettings') },
          ]"
          :key="item.key"
          type="button"
          class="w-full text-left px-3 h-9 rounded-lg text-sm transition-colors duration-150 cursor-pointer"
          :class="tab === item.key
            ? 'bg-gray-200 text-gray-900 font-medium'
            : 'text-gray-600 hover:bg-gray-100 hover:text-gray-800'"
          @click="tab = item.key as any"
        >
          {{ item.label }}
        </button>
      </aside>

      <!-- 右侧内容 -->
      <section class="flex-1 overflow-y-auto px-5 py-4 relative">
        <!-- 加载遮罩 -->
        <div v-if="loading" class="absolute inset-0 flex items-center justify-center bg-white/70 z-10">
          <svg class="animate-spin w-5 h-5 text-blue-500" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
          </svg>
        </div>

        <!-- 基础设置 -->
        <div v-if="tab === 'basic'">
          <div class="flex items-center justify-between mb-3 h-8">
            <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide">{{ t('basicSettings') }}</p>
          </div>
          <div class="flex items-center gap-4 py-3 border-b border-gray-100">
            <span class="text-sm text-gray-800 flex-shrink-0">{{ t('language') }}</span>
            <AppSelect
              :model-value="language"
              :options="[{ value: 'zh-CN', label: t('chinese') }, { value: 'en-US', label: t('english') }]"
              :disabled="savingLanguage"
              @update:model-value="onLanguageChange($event as any)"
            />
          </div>
        </div>

        <!-- LLM 设置 -->
        <div v-if="tab === 'llm'">
          <div class="flex items-center justify-between mb-3">
            <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide">{{ t('llmSettings') }}</p>
            <button
              type="button"
              class="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg bg-blue-600 text-white hover:bg-blue-700 transition-colors duration-150 cursor-pointer"
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
              class="flex items-center gap-3 px-4 py-3 border border-gray-200 rounded-xl bg-white"
            >
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium text-gray-800 truncate">{{ item.name }} · <span class="font-normal text-gray-500">{{ item.model }}</span></div>
                <div class="text-xs text-gray-400 truncate mt-0.5">{{ item.baseUrl }}</div>
              </div>
              <button
                type="button"
                class="flex-shrink-0 w-7 h-7 flex items-center justify-center rounded-lg text-red-400 hover:text-red-600 hover:bg-red-50 transition-colors duration-150 cursor-pointer"
                @click="deleteLLM(item.id)"
              >
                <MdiIcon :path="mdiDeleteOutline" :size="16" />
              </button>
            </div>
            <div v-if="llmRows.length === 0" class="text-center py-8 text-sm text-gray-400 border border-dashed border-gray-200 rounded-xl">
              {{ t('add') }} LLM
            </div>
          </div>
        </div>

        <!-- MCP 设置 -->
        <div v-if="tab === 'mcp'">
          <div class="flex items-center justify-between mb-3">
            <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide">{{ t('mcpSettings') }}</p>
            <button
              type="button"
              class="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg bg-blue-600 text-white hover:bg-blue-700 transition-colors duration-150 cursor-pointer"
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
              class="flex items-center gap-3 px-4 py-3 border border-gray-200 rounded-xl bg-white"
            >
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium text-gray-800">{{ item.name }}</div>
                <div class="text-xs text-gray-400 mt-0.5">{{ mcpPreview(item) }}</div>
              </div>
              <div class="flex items-center gap-2 flex-shrink-0">
                <!-- 开关 -->
                <button
                  type="button"
                  class="relative w-9 h-5 rounded-full transition-colors duration-200 cursor-pointer flex-shrink-0"
                  :class="item.isEnabled ? 'bg-blue-500' : 'bg-gray-300'"
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
                  class="w-7 h-7 flex items-center justify-center rounded-lg text-gray-400 hover:text-blue-600 hover:bg-blue-50 transition-colors duration-150 cursor-pointer"
                  @click="openMCPEditDialog(item)"
                >
                  <MdiIcon :path="mdiPencilOutline" :size="15" />
                </button>
                <!-- 删除 -->
                <button
                  type="button"
                  class="w-7 h-7 flex items-center justify-center rounded-lg text-red-400 hover:text-red-600 hover:bg-red-50 transition-colors duration-150 cursor-pointer"
                  @click="deleteMCP(item.id)"
                >
                  <MdiIcon :path="mdiDeleteOutline" :size="16" />
                </button>
              </div>
            </div>
            <div v-if="mcpRows.length === 0" class="text-center py-8 text-sm text-gray-400 border border-dashed border-gray-200 rounded-xl">
              {{ t('add') }} MCP
            </div>
          </div>
        </div>

        <!-- Skills 设置 -->
        <div v-if="tab === 'skills'">
          <div class="flex items-center justify-between mb-1">
            <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide">{{ t('skillsSettings') }}</p>
            <button
              type="button"
              class="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg bg-blue-600 text-white hover:bg-blue-700 transition-colors duration-150 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
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
          <p class="text-xs text-gray-400 mb-3">{{ t('skillsDescription') }}</p>

          <!-- 拖拽上传区 -->
          <div
            class="border border-dashed rounded-xl px-4 py-5 text-center text-sm text-gray-400 cursor-pointer transition-all duration-200 mb-3"
            :class="skillsDropActive ? 'border-blue-400 bg-blue-50 text-blue-600' : 'border-gray-200 bg-gray-50 hover:border-gray-300 hover:bg-gray-100'"
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

          <div v-if="skillsRows.length === 0" class="text-center py-6 text-sm text-gray-400 border border-dashed border-gray-200 rounded-xl">
            {{ t('skillsEmpty') }}
          </div>
          <div v-else class="flex flex-col gap-2">
            <div
              v-for="item in skillsRows"
              :key="item.id"
              class="flex items-start gap-3 px-4 py-3 border border-gray-200 rounded-xl bg-white"
            >
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium text-gray-800">{{ item.name }}</div>
                <div v-if="item.description" class="text-xs text-gray-400 mt-0.5">{{ item.description }}</div>
                <div v-if="item.relativePath" class="text-xs text-gray-300 mt-0.5 font-mono">{{ item.relativePath }}</div>
              </div>
              <button
                type="button"
                class="flex-shrink-0 w-7 h-7 flex items-center justify-center rounded-lg text-red-400 hover:text-red-600 hover:bg-red-50 transition-colors duration-150 cursor-pointer"
                @click="deleteSkill(item.id)"
              >
                <MdiIcon :path="mdiDeleteOutline" :size="16" />
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
      <div v-for="field in [
        { key: 'name', label: t('name'), type: 'text' },
        { key: 'model', label: t('model'), type: 'text' },
        { key: 'baseUrl', label: t('baseUrl'), type: 'text' },
        { key: 'apiKey', label: t('apiKey'), type: 'password' },
      ]" :key="field.key" class="flex flex-col gap-1.5">
        <label class="text-xs font-medium text-gray-500">{{ field.label }}</label>
        <input
          v-model="(llmForm as any)[field.key]"
          :type="field.type"
          class="px-3 py-2 text-sm border border-gray-200 rounded-lg outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-100 transition-all duration-150"
        />
      </div>
    </div>
  </BaseDialog>

  <!-- 通用删除确认弹窗 -->
  <BaseDialog
    v-model:visible="confirmDialogVisible"
    :title="t('delete')"
    :confirm-text="t('confirm')"
    :cancel-text="t('cancel')"
    :confirm-danger="true"
    width="360px"
    @confirm="runConfirmDialog"
  >
    <p class="text-sm text-gray-700">{{ t('confirmDeleteItem') }}</p>
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
        <label class="text-xs font-medium text-gray-500">{{ t('name') }}</label>
        <input
          v-model="mcpForm.name"
          type="text"
          class="px-3 py-2 text-sm border border-gray-200 rounded-lg outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-100 transition-all duration-150"
        />
      </div>

      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label class="text-xs font-medium text-gray-500">{{ t('mcpConfigJson') }}</label>
          <div class="flex items-center gap-1">
            <button
              v-for="tpl in ['stdio', 'sse', 'streamable_http'] as const"
              :key="tpl"
              type="button"
              class="px-2.5 py-1 text-xs rounded-md border transition-colors duration-150 cursor-pointer"
              :class="mcpTemplateType === tpl
                ? 'bg-blue-600 text-white border-blue-600'
                : 'border-gray-200 text-gray-600 hover:bg-gray-100'"
              @click="applyTemplate(tpl)"
            >
              {{ tpl }}
            </button>
          </div>
        </div>
        <div class="rounded-xl overflow-hidden border border-gray-700">
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

@media (max-width: 640px) {
  .flex.flex-1.overflow-hidden {
    flex-direction: column;
  }
  aside {
    width: 100% !important;
    flex-direction: row !important;
    overflow-x: auto;
    border-right: none !important;
    border-bottom: 1px solid #e5e7eb;
  }
}
</style>
