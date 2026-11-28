<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppDialog from '@/components/ui/AppDialog.vue'
import type { SessionItem } from '@/api/chat'
import { downloadSessionExport, exportFilename, type ExportFormat } from '@/api/sessionExport'

const props = defineProps<{ session: SessionItem }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const format = ref<ExportFormat>('markdown')
const includeTools = ref(true)
const includeThinking = ref(false)
const busy = ref(false)
const failed = ref(false)
const filename = computed(() => exportFilename(props.session.name, format.value))

async function download() {
  if (busy.value) return
  busy.value = true
  failed.value = false
  try {
    await downloadSessionExport(props.session.id, {
      format: format.value, includeTools: includeTools.value, includeThinking: includeThinking.value,
    }, filename.value)
    emit('close')
  } catch {
    failed.value = true
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AppDialog
    :visible="true" :title="t('sessionExport')" width="440px"
    :confirm-text="t('sessionExportDownload')" :confirm-loading="busy"
    :close-on-esc="!busy" :close-on-mask="!busy" :show-close="!busy" :show-cancel="!busy"
    @confirm="download" @cancel="emit('close')"
  >
    <div class="export-formats">
      <button type="button" :aria-pressed="format === 'markdown'" @click="format = 'markdown'">
        <strong>Markdown (.md)</strong><span>{{ t('sessionExportMarkdown') }}</span>
      </button>
      <button type="button" :aria-pressed="format === 'json'" @click="format = 'json'">
        <strong>JSON (.json)</strong><span>{{ t('sessionExportJSON') }}</span>
      </button>
    </div>
    <label class="export-option"><input v-model="includeTools" type="checkbox" /> {{ t('sessionExportTools') }}</label>
    <label class="export-option"><input v-model="includeThinking" type="checkbox" /> {{ t('sessionExportThinking') }}</label>
    <code class="export-name" :title="filename">{{ filename }}</code>
    <p v-if="failed" class="export-error" role="alert">{{ t('sessionExportFailed') }}</p>
  </AppDialog>
</template>

<style scoped>
.export-formats { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; margin-bottom: 14px; }
.export-formats button { display: grid; gap: 4px; padding: 10px; text-align: left; border: 1px solid var(--border-default); border-radius: 8px; font-size: 13px; color: var(--text-primary); cursor: pointer; }
.export-formats button[aria-pressed="true"] { border-color: var(--sb-brand); background: var(--primary-alpha-06); }
.export-formats span { color: var(--text-secondary); font-size: 12px; }
.export-option { display: flex; align-items: center; gap: 8px; margin: 8px 0; font-size: 13px; color: var(--text-primary); }
.export-option input { accent-color: var(--sb-brand); }
.export-name { display: block; padding: 8px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; border-radius: 6px; background: var(--bg-subtle); color: var(--text-secondary); font: 12px var(--font-mono); }
.export-error { margin-top: 8px; color: var(--color-danger); font-size: 13px; }
</style>
