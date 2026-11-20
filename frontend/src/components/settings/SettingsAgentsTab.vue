<script setup lang="ts">
import { EditorView, highlightActiveLine, lineNumbers } from '@codemirror/view'
import CodeMirror from 'vue-codemirror6'
import { useI18n } from 'vue-i18n'

defineProps<{
  content: string
  path: string
  saving: boolean
}>()

const emit = defineEmits<{
  'update:content': [value: string]
  save: []
}>()

const { t } = useI18n()

const agentsEditorExtensions = [
  lineNumbers(),
  highlightActiveLine(),
  EditorView.theme({
    '&': {
      backgroundColor: 'var(--agents-editor-bg)',
      color: 'var(--text-primary)',
    },
    '.cm-content': {
      caretColor: 'var(--sb-brand)',
    },
    '.cm-cursor, .cm-dropCursor': {
      borderLeftColor: 'var(--sb-brand)',
    },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
      backgroundColor: 'var(--agents-editor-selection-bg)',
    },
    '.cm-activeLine': {
      backgroundColor: 'var(--agents-editor-active-line-bg)',
    },
    '.cm-gutters': {
      backgroundColor: 'var(--agents-editor-gutter-bg)',
      color: 'var(--text-muted)',
      borderRightColor: 'var(--agents-editor-divider)',
    },
    '.cm-activeLineGutter': {
      backgroundColor: 'var(--agents-editor-active-line-bg)',
      color: 'var(--text-secondary)',
    },
  }),
]

function onContentChange(value: unknown) {
  emit('update:content', typeof value === 'string' ? value : String(value ?? ''))
}
</script>

<template>
  <div>
    <div class="flex items-center justify-between gap-3 mb-3">
      <div class="min-w-0">
        <p class="settings-item-meta truncate">{{ path }}</p>
      </div>
      <button
        type="button"
        class="px-3 py-1.5 rounded-lg cursor-pointer account-edit-btn settings-action-text"
        :disabled="saving"
        @click="emit('save')"
      >
        {{ saving ? t('saving') : t('save') }}
      </button>
    </div>
    <div class="settings-card agents-editor-card rounded-xl overflow-hidden">
      <CodeMirror
        :model-value="content"
        class="json-codemirror agents-codemirror"
        :extensions="agentsEditorExtensions"
        :indent-with-tab="true"
        :tab-size="2"
        @update:model-value="onContentChange"
      />
    </div>
  </div>
</template>
