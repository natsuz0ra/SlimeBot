<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppTextInput from '@/components/ui/AppTextInput.vue'
import { promptTemplatesAPI } from '@/api/promptTemplates'
import { validTemplateSlash, type PromptTemplate } from '@/utils/slashCommands'

const { t } = useI18n()
const templates = ref<PromptTemplate[]>([])
const draft = ref<PromptTemplate>()
const deleting = ref<PromptTemplate>()
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const dialogError = ref('')
const cursor = '{{cursor}}'

async function load() {
  loading.value = true
  error.value = ''
  try { templates.value = await promptTemplatesAPI.list() }
  catch { error.value = t('templateLoadFailed') }
  finally { loading.value = false }
}

onMounted(load)

function edit(template?: PromptTemplate) {
  dialogError.value = ''
  draft.value = template ? { ...template } : { id: '', slash: '', title: '', content: '' }
}

function askDelete(template: PromptTemplate) {
  dialogError.value = ''
  deleting.value = template
}

async function save(items: PromptTemplate[]) {
  if (saving.value) return
  saving.value = true
  dialogError.value = ''
  try {
    templates.value = await promptTemplatesAPI.save(items)
    draft.value = undefined
    deleting.value = undefined
  } catch { dialogError.value = t('templateSaveFailed') }
  finally { saving.value = false }
}

function saveDraft() {
  if (!draft.value) return
  const item = { ...draft.value, slash: draft.value.slash.trim(), title: draft.value.title.trim() }
  const encoder = new TextEncoder()
  if (!validTemplateSlash(item.slash) || templates.value.some(template => template.id !== item.id && template.slash === item.slash)) {
    dialogError.value = t('templateInvalidSlash')
    return
  }
  if (!item.title || !item.content.trim() || encoder.encode(item.title).length > 128
    || encoder.encode(item.description ?? '').length > 512 || encoder.encode(item.content).length > 16 * 1024
    || item.content.includes('\0') || item.content.split(cursor).length > 2) {
    dialogError.value = t('templateInvalidContent')
    return
  }
  const result = item.id ? templates.value.map(template => template.id === item.id ? item : template) : [...templates.value, item]
  void save(result)
}
</script>

<template>
  <section class="settings-card rounded-xl px-4 py-3.5 mt-2">
    <div class="flex items-center justify-between gap-3 mb-2">
      <h3 class="settings-field-label">{{ t('promptTemplates') }}</h3>
      <button type="button" class="settings-action-text rounded-lg px-2 py-1 cursor-pointer" :disabled="loading || !!error || templates.length >= 100" @click="edit()">{{ t('templateAdd') }}</button>
    </div>
    <p v-if="loading" class="settings-item-sub">{{ t('loading') }}</p>
    <div v-else-if="error" role="alert" class="settings-item-sub">
      {{ error }} <button type="button" class="settings-action-text cursor-pointer" @click="load">{{ t('templateRetry') }}</button>
    </div>
    <p v-else-if="!templates.length" class="settings-item-sub">{{ t('templateEmpty') }}</p>
    <ul v-else class="divide-y divide-[var(--border-subtle)]">
      <li v-for="item in templates" :key="item.id" class="flex justify-between items-center gap-2 py-2">
        <div class="min-w-0">
          <div class="truncate text-sm sb-text-primary"><code>/{{ item.slash }}</code> · {{ item.title }}</div>
          <p v-if="item.description" class="settings-item-sub truncate" :title="item.description">{{ item.description }}</p>
        </div>
        <div class="flex gap-2 shrink-0">
          <button type="button" class="settings-action-text cursor-pointer" @click="edit(item)">{{ t('edit') }}</button>
          <button type="button" class="text-[var(--color-danger)] text-xs cursor-pointer" @click="askDelete(item)">{{ t('delete') }}</button>
        </div>
      </li>
    </ul>
  </section>

  <AppDialog
    v-if="draft" :visible="true" :title="t(draft.id ? 'templateEdit' : 'templateAdd')" width="520px"
    :confirm-text="t('save')" :confirm-loading="saving" :close-on-esc="!saving" :close-on-mask="!saving"
    :show-close="!saving" :show-cancel="!saving" @confirm="saveDraft" @cancel="draft = undefined"
  >
    <div class="template-fields">
      <label for="prompt-template-slash">{{ t('templateSlash') }}</label>
      <AppTextInput id="prompt-template-slash" v-model="draft.slash" maxlength="32" placeholder="review" spellcheck="false" />
      <label for="prompt-template-title">{{ t('templateTitle') }}</label>
      <AppTextInput id="prompt-template-title" v-model="draft.title" maxlength="128" />
      <label for="prompt-template-description">{{ t('templateDescription') }}</label>
      <AppTextInput id="prompt-template-description" :model-value="draft.description ?? ''" maxlength="512" @update:model-value="draft.description = $event" />
      <label for="prompt-template-content">{{ t('templateContent') }}</label>
      <textarea id="prompt-template-content" v-model="draft.content" rows="6" maxlength="16384" />
      <p>{{ t('templateCursor', { cursor }) }}</p>
      <p v-if="dialogError" role="alert" class="draft-error">{{ dialogError }}</p>
    </div>
  </AppDialog>
  <AppDialog
    v-if="deleting" :visible="true" :title="t('templateDelete')" :confirm-text="t('delete')" confirm-danger
    :confirm-loading="saving" :close-on-mask="!saving" :close-on-esc="!saving" :show-close="!saving" :show-cancel="!saving"
    @confirm="save(templates.filter(item => item.id !== deleting?.id))" @cancel="deleting = undefined"
  >
    <p class="sb-text-primary text-sm">{{ t('templateDeleteQuestion', { slash: deleting.slash }) }}</p>
    <p v-if="dialogError" role="alert" class="draft-error">{{ dialogError }}</p>
  </AppDialog>
</template>

<style scoped>
.template-fields { display: grid; gap: 6px; font-size: 13px; color: var(--text-primary); }
.template-fields label:not(:first-child) { margin-top: 6px; }
.template-fields textarea { padding: 8px; min-height: 100px; resize: vertical; border: 1px solid var(--border-default); border-radius: 8px; background: var(--bg-subtle); outline-color: var(--focus-ring); }
.template-fields p { color: var(--text-secondary); font-size: 12px; }
.draft-error, .template-fields .draft-error { color: var(--color-danger); }
</style>
