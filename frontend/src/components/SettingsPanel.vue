<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { mdiClose, mdiPencil, mdiPlus } from '@mdi/js'
import CodeMirror from 'vue-codemirror6'
import { json } from '@codemirror/lang-json'
import { oneDark } from '@codemirror/theme-one-dark'
import { lineNumbers } from '@codemirror/view'

import MdiIcon from './MdiIcon.vue'
import { llmAPI, mcpAPI, settingAPI } from '../api'

const emit = defineEmits<{ close: [] }>()

const { t, locale } = useI18n()

const tab = ref<'basic' | 'llm' | 'mcp'>('basic')
const language = ref<'zh-CN' | 'en-US'>('zh-CN')
const llmList = ref<any[]>([])
const mcpList = ref<any[]>([])
const loading = ref(false)
const savingLanguage = ref(false)
const llmDialogVisible = ref(false)
const mcpDialogVisible = ref(false)
const llmSubmitting = ref(false)
const mcpSubmitting = ref(false)
const mcpEditingID = ref('')

const llmForm = ref({ name: '', baseUrl: '', apiKey: '', model: '' })
const mcpForm = ref({ name: '', config: '', isEnabled: true })
const mcpTemplateType = ref<'stdio' | 'sse' | 'streamable_http'>('stdio')
const mcpEditorExtensions = [lineNumbers(), json(), oneDark]

const llmRows = computed(() => llmList.value || [])
const mcpRows = computed(() => mcpList.value || [])
const mcpDialogTitle = computed(() => (mcpEditingID.value ? t('editMcp') : t('addMcp')))

function showError(message: string) {
  MessagePlugin.error({
    content: message,
    placement: 'top-right',
  })
}

function showSuccess(message: string) {
  MessagePlugin.success({
    content: message,
    placement: 'top-right',
  })
}

async function loadData() {
  loading.value = true
  try {
    const settings = await settingAPI.get()
    language.value = settings.language || 'zh-CN'
    locale.value = language.value
    llmList.value = await llmAPI.list()
    mcpList.value = await mcpAPI.list()
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
    showSuccess(t('saveSuccess'))
  } catch {
    language.value = previousLanguage
    locale.value = previousLanguage
    showError(t('languageSaveFailed'))
  } finally {
    savingLanguage.value = false
  }
}

function handleLanguageSelectChange(value: string) {
  void onLanguageChange(value as 'zh-CN' | 'en-US')
}

function openLLMDialog() {
  llmForm.value = { name: '', baseUrl: '', apiKey: '', model: '' }
  llmDialogVisible.value = true
}

async function addLLM() {
  if (!llmForm.value.name || !llmForm.value.baseUrl || !llmForm.value.apiKey || !llmForm.value.model) {
    showError(t('llmFormIncomplete'))
    return
  }
  llmSubmitting.value = true
  try {
    await llmAPI.create(llmForm.value)
    llmForm.value = { name: '', baseUrl: '', apiKey: '', model: '' }
    llmList.value = await llmAPI.list()
    llmDialogVisible.value = false
  } finally {
    llmSubmitting.value = false
  }
}

async function deleteLLM(id: string) {
  if (!window.confirm(t('confirmDelete'))) return
  await llmAPI.remove(id)
  llmList.value = await llmAPI.list()
}

function buildTemplate(transport: 'stdio' | 'sse' | 'streamable_http') {
  if (transport === 'stdio') {
    return JSON.stringify(
      {
        command: 'python',
        args: ['-m', 'your_module'],
      },
      null,
      2,
    )
  }
  return JSON.stringify(
    {
      transport,
      url: 'https://your-mcp-server-url',
      headers: {},
      timeout: 5,
      sse_read_timeout: 300,
    },
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
  mcpForm.value = {
    name: item.name,
    config: item.config,
    isEnabled: item.isEnabled,
  }
  mcpDialogVisible.value = true
}

async function saveMCP() {
  if (!mcpForm.value.name || !mcpForm.value.config) {
    showError(t('mcpFormIncomplete'))
    return
  }
  let parsed: any
  try {
    parsed = JSON.parse(mcpForm.value.config)
  } catch {
    showError(t('mcpJsonInvalid'))
    return
  }
  if (parsed?.mcpServers) {
    showError(t('mcpWrapperNotSupported'))
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
  await mcpAPI.update(item.id, {
    name: item.name,
    config: item.config,
    isEnabled: item.isEnabled,
  })
}

function mcpPreview(item: any) {
  try {
    const cfg = JSON.parse(item.config || '{}')
    const transport = cfg.transport || 'stdio'
    if (transport === 'stdio') {
      return `${transport} · ${cfg.command || '-'}`
    }
    return `${transport} · ${cfg.url || '-'}`
  } catch {
    return t('mcpJsonInvalid')
  }
}

async function deleteMCP(id: string) {
  if (!window.confirm(t('confirmDelete'))) return
  await mcpAPI.remove(id)
  mcpList.value = await mcpAPI.list()
}

onMounted(loadData)
</script>

<template>
  <div class="settings-panel">
    <div class="panel-head">
      <div class="title">{{ t('settings') }}</div>
      <button class="close-btn" type="button" @click="emit('close')">
        <MdiIcon :path="mdiClose" :size="16" />
      </button>
    </div>
    <div class="panel-body">
      <aside class="tab-menu">
        <button type="button" class="tab-item" :class="{ active: tab === 'basic' }" @click="tab = 'basic'">{{ t('basicSettings') }}</button>
        <button type="button" class="tab-item" :class="{ active: tab === 'llm' }" @click="tab = 'llm'">{{ t('llmSettings') }}</button>
        <button type="button" class="tab-item" :class="{ active: tab === 'mcp' }" @click="tab = 'mcp'">{{ t('mcpSettings') }}</button>
      </aside>
      <section class="tab-content" v-loading="loading">
        <div v-if="tab === 'basic'" class="block">
          <div class="field-label">{{ t('language') }}</div>
          <t-select v-model="language" style="width: 160px" :disabled="savingLanguage" @change="handleLanguageSelectChange">
            <t-option value="zh-CN" :label="t('chinese')" />
            <t-option value="en-US" :label="t('english')" />
          </t-select>
        </div>

        <div v-if="tab === 'llm'" class="block stack">
          <div class="section-header">
            <div class="section-title">{{ t('llmSettings') }}</div>
            <t-button size="small" theme="primary" @click="openLLMDialog">
              <template #icon><MdiIcon :path="mdiPlus" :size="14" /></template>
              {{ t('add') }}
            </t-button>
          </div>
          <div class="list">
            <div v-for="item in llmRows" :key="item.id" class="list-row">
              <div class="list-title">{{ item.name }} / {{ item.model }}</div>
              <div class="list-desc">{{ item.baseUrl }}</div>
              <button type="button" class="danger-btn" @click="deleteLLM(item.id)">{{ t('delete') }}</button>
            </div>
          </div>
        </div>

        <div v-if="tab === 'mcp'" class="block stack">
          <div class="section-header">
            <div class="section-title">{{ t('mcpSettings') }}</div>
            <t-button size="small" theme="primary" @click="openMCPDialog">
              <template #icon><MdiIcon :path="mdiPlus" :size="14" /></template>
              {{ t('add') }}
            </t-button>
          </div>
          <div class="list">
            <div v-for="item in mcpRows" :key="item.id" class="list-row">
              <div class="list-title">{{ item.name }}</div>
              <div class="list-desc">{{ mcpPreview(item) }}</div>
              <div class="actions">
                <t-switch v-model="item.isEnabled" size="small" @change="updateMCP(item)" />
                <button type="button" class="text-btn" @click="openMCPEditDialog(item)">
                  <MdiIcon :path="mdiPencil" :size="14" />
                  {{ t('editMcp') }}
                </button>
                <button type="button" class="danger-btn" @click="deleteMCP(item.id)">{{ t('delete') }}</button>
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>

    <t-dialog v-model:visible="llmDialogVisible" :header="t('addModel')" :confirm-btn="t('confirm')" :cancel-btn="t('cancel')" :confirm-loading="llmSubmitting" @confirm="addLLM">
      <div class="dialog-form">
        <div class="dialog-field">
          <div class="dialog-label">{{ t('name') }}</div>
          <t-input v-model="llmForm.name" />
        </div>
        <div class="dialog-field">
          <div class="dialog-label">{{ t('model') }}</div>
          <t-input v-model="llmForm.model" />
        </div>
        <div class="dialog-field">
          <div class="dialog-label">{{ t('baseUrl') }}</div>
          <t-input v-model="llmForm.baseUrl" />
        </div>
        <div class="dialog-field">
          <div class="dialog-label">{{ t('apiKey') }}</div>
          <t-input v-model="llmForm.apiKey" type="password" />
        </div>
      </div>
    </t-dialog>

    <t-dialog
      v-model:visible="mcpDialogVisible"
      :header="mcpDialogTitle"
      :confirm-btn="t('confirm')"
      :cancel-btn="t('cancel')"
      :confirm-loading="mcpSubmitting"
      width="760px"
      @confirm="saveMCP"
    >
      <div class="dialog-form">
        <div class="dialog-field">
          <div class="dialog-label">{{ t('name') }}</div>
          <t-input v-model="mcpForm.name" />
        </div>
        <div class="dialog-field">
          <div class="dialog-template-row">
            <div class="dialog-label">{{ t('mcpConfigJson') }}</div>
            <div class="template-actions">
              <t-button variant="outline" size="small" :theme="mcpTemplateType === 'stdio' ? 'primary' : 'default'" @click="applyTemplate('stdio')">stdio</t-button>
              <t-button variant="outline" size="small" :theme="mcpTemplateType === 'sse' ? 'primary' : 'default'" @click="applyTemplate('sse')">sse</t-button>
              <t-button variant="outline" size="small" :theme="mcpTemplateType === 'streamable_http' ? 'primary' : 'default'" @click="applyTemplate('streamable_http')">streamable_http</t-button>
            </div>
          </div>
          <div class="json-editor-wrap">
            <CodeMirror
              v-model="mcpForm.config"
              class="json-codemirror"
              :extensions="mcpEditorExtensions"
              :indent-with-tab="true"
              :tab-size="2"
              :style="{ height: '320px' }"
            />
          </div>
        </div>
      </div>
    </t-dialog>
  </div>
</template>

<style scoped>
.settings-panel {
  height: 100%;
  background: #efefef;
  border: 1px solid #d3d3d3;
  border-radius: 8px;
  overflow: hidden;
  box-sizing: border-box;
}

.panel-head {
  height: 32px;
  border-bottom: 1px solid #d7d7d7;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 10px;
}

.title {
  font-size: 14px;
  color: #333;
}

.close-btn {
  width: 22px;
  height: 22px;
  border: 0;
  background: transparent;
  cursor: pointer;
}

.panel-body {
  display: flex;
  height: calc(100% - 32px);
}

.tab-menu {
  width: 144px;
  border-right: 1px solid #d7d7d7;
  padding: 10px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.tab-item {
  height: 28px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  text-align: left;
  padding: 0 10px;
  cursor: pointer;
  color: #2e2e2e;
}

.tab-item.active {
  background: #e3e3e3;
}

.tab-content {
  flex: 1;
  padding: 14px 20px;
  overflow: auto;
}

.block {
  font-size: 14px;
}

.field-label {
  margin-bottom: 8px;
  font-weight: 600;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
}

.stack {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(120px, 1fr));
  gap: 8px;
}

.list {
  max-height: 220px;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.list-row {
  border: 1px solid #dadada;
  border-radius: 6px;
  background: #f7f7f7;
  padding: 8px 10px;
}

.list-title {
  font-size: 13px;
  font-weight: 600;
}

.list-desc {
  font-size: 12px;
  color: #6f6f6f;
  margin: 4px 0 6px;
}

.actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.text-btn {
  border: 0;
  background: transparent;
  color: #3f6fd9;
  cursor: pointer;
  padding: 0;
  display: inline-flex;
  gap: 4px;
  align-items: center;
}

.danger-btn {
  border: 0;
  background: transparent;
  color: #d54941;
  cursor: pointer;
  padding: 0;
}

.dialog-form {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.dialog-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.dialog-label {
  font-size: 12px;
  color: #555;
}

.dialog-template-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.template-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.json-editor-wrap {
  border-radius: 10px;
  overflow: hidden;
  border: 1px solid #2f2f2f;
}

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

@media (max-width: 900px) {
  .panel-body {
    flex-direction: column;
  }

  .tab-menu {
    width: 100%;
    border-right: 0;
    border-bottom: 1px solid #d7d7d7;
    flex-direction: row;
  }

  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
