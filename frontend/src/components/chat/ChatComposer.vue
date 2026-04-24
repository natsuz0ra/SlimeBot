<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { mdiClose, mdiPaperclip, mdiSend, mdiStop } from '@mdi/js'
import { useI18n } from 'vue-i18n'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import TruncationTooltip from '@/components/ui/TruncationTooltip.vue'
import AppSelect, { type SelectOption } from '@/components/ui/AppSelect.vue'
import { formatSize } from '@/utils/format'

const props = defineProps<{
  modelValue: string
  selectedModelId: string
  modelSelectOptions: SelectOption[]
  selectedThinkingLevel: string
  thinkingSelectOptions: SelectOption[]
  modelOptionsCount: number
  sendDisabled: boolean
  stopDisabled: boolean
  isStreaming: boolean
  pendingFiles: File[]
  placeholder: string
  planMode: boolean
  planConfirmationVisible?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  send: []
  stop: []
  filesChange: [files: File[]]
  removeFile: [index: number]
  modelChange: [modelId: string]
  thinkingChange: [level: string]
  planToggle: []
  planExecute: []
  planCancel: []
}>()

const textareaRef = ref<HTMLTextAreaElement | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)
const { t } = useI18n()

function resizeTextarea(el: HTMLTextAreaElement) {
  el.style.height = 'auto'
  el.style.height = `${el.scrollHeight}px`
}

function onTextareaInput(e: Event) {
  const el = e.target as HTMLTextAreaElement
  resizeTextarea(el)
  emit('update:modelValue', el.value)
}

function onTextareaKeydown(e: KeyboardEvent) {
  if (e.isComposing) return
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    if (props.isStreaming) {
      emit('stop')
      return
    }
    emit('send')
  }
}

function openFilePicker() {
  fileInputRef.value?.click()
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const selected = Array.from(input.files || [])
  const merged: File[] = [...props.pendingFiles]
  for (const file of selected) {
    if (merged.length >= 5) break
    if (file.size > 10 * 1024 * 1024) continue
    merged.push(file)
  }
  emit('filesChange', merged)
  input.value = ''
}

function getFileExt(name: string) {
  const parts = name.split('.')
  if (parts.length <= 1) return ''
  return parts[parts.length - 1]?.toUpperCase() || ''
}

watch(
  () => props.modelValue,
  () => {
    void nextTick(() => {
      if (textareaRef.value) {
        resizeTextarea(textareaRef.value)
      }
    })
  },
)
</script>

<template>
  <div class="input-container focus-ring rounded-2xl">
    <div v-if="planConfirmationVisible" class="plan-confirm-inline">
      <div class="plan-confirm-copy">{{ t('planConfirmPrompt') }}</div>
      <div class="plan-confirm-actions">
        <button
          type="button"
          class="plan-confirm-btn plan-confirm-btn--cancel"
          @click="emit('planCancel')"
        >
          {{ t('planConfirmCancel') }}
        </button>
        <button
          type="button"
          class="plan-confirm-btn plan-confirm-btn--execute"
          @click="emit('planExecute')"
        >
          {{ t('planConfirmExecute') }}
        </button>
      </div>
    </div>
    <template v-else>
    <div v-if="pendingFiles.length > 0" class="px-3 pt-3 pb-1 flex flex-wrap gap-2">
      <div
        v-for="(file, idx) in pendingFiles"
        :key="`${file.name}-${idx}`"
        class="chat-upload-chip group/tip inline-flex min-w-0 items-center gap-2 rounded-lg px-2 py-1 max-w-[260px]"
      >
        <TruncationTooltip inherit-group :text="file.name" wrapper-class="min-w-0 flex-1" content-class="text-xs" />
        <span class="text-[10px] opacity-70">{{ getFileExt(file.name) }} {{ formatSize(file.size) }}</span>
        <button type="button" class="opacity-80 hover:opacity-100 cursor-pointer" @click="emit('removeFile', idx)">
          <MdiIcon :path="mdiClose" :size="12" />
        </button>
      </div>
    </div>
    <input
      ref="fileInputRef"
      type="file"
      class="hidden"
      multiple
      @change="onFileChange"
    >
    <textarea
      ref="textareaRef"
      :value="modelValue"
      class="textarea-primary w-full resize-none border-0 outline-none bg-transparent px-4 pt-3.5 pb-12 text-sm leading-relaxed min-h-[112px] max-h-[260px] overflow-y-auto"
      :placeholder="placeholder"
      rows="1"
      @keydown="onTextareaKeydown"
      @input="onTextareaInput"
    />
    <div class="absolute bottom-2 left-3 right-3 flex items-center justify-between gap-2 z-10">
      <div class="flex items-center gap-2">
        <AppSelect
          :model-value="selectedModelId"
          :options="modelSelectOptions"
          :disabled="modelOptionsCount === 0"
          variant="ghost"
          size="xs"
          @update:model-value="emit('modelChange', $event)"
        />
        <AppSelect
          :model-value="selectedThinkingLevel"
          :options="thinkingSelectOptions"
          variant="ghost"
          size="xs"
          @update:model-value="emit('thinkingChange', $event)"
        />
        <div class="flex items-center gap-1 text-xs text-gray-400">
          <ToggleSwitch :model-value="planMode" @update:model-value="emit('planToggle')" />
          <span>{{ t('planModeLabel') }}</span>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <div class="relative z-[120] group/upload-tip">
          <button
            type="button"
            class="w-8 h-8 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer flex-shrink-0 attach-btn"
            :disabled="pendingFiles.length >= 5"
            :aria-label="t('uploadTipLine1')"
            @click="openFilePicker"
          >
            <MdiIcon :path="mdiPaperclip" :size="15" />
          </button>
          <div
            class="pointer-events-none absolute bottom-full right-0 mb-2 w-[240px] rounded-lg px-3 py-2 text-sm leading-5 text-white bg-black/78 opacity-0 translate-y-1 transition-all duration-150 shadow-lg group-hover/upload-tip:opacity-100 group-hover/upload-tip:translate-y-0 group-focus-within/upload-tip:opacity-100 group-focus-within/upload-tip:translate-y-0"
          >
            <div>{{ t('uploadTipLine1') }}</div>
            <div class="mt-1 opacity-90">{{ t('uploadTipLine2') }}</div>
            <div class="absolute -bottom-1 right-3 h-2 w-2 rotate-45 bg-black/78" />
          </div>
        </div>
        <button
          v-if="isStreaming"
          type="button"
          class="w-8 h-8 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer flex-shrink-0"
          :class="stopDisabled ? 'send-btn-disabled' : 'stop-btn'"
          :disabled="stopDisabled"
          @click="emit('stop')"
        >
          <MdiIcon :path="mdiStop" :size="15" />
        </button>
        <button
          v-else
          type="button"
          class="w-8 h-8 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer flex-shrink-0"
          :class="sendDisabled ? 'send-btn-disabled' : 'send-btn btn-primary'"
          :disabled="sendDisabled"
          @click="emit('send')"
        >
          <MdiIcon :path="mdiSend" :size="15" />
        </button>
      </div>
    </div>
    </template>
  </div>
</template>

<style scoped>
.plan-confirm-inline {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 88px;
  padding: 16px;
}

.plan-confirm-copy {
  min-width: 0;
  color: var(--text-primary);
  font-size: 14px;
  font-weight: 650;
  line-height: 1.4;
}

.plan-confirm-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.plan-confirm-btn {
  min-height: 36px;
  border-radius: 10px;
  padding: 8px 16px;
  border: 1px solid transparent;
  font-size: 14px;
  font-weight: 650;
  cursor: pointer;
  transition: background-color 160ms ease, border-color 160ms ease, box-shadow 160ms ease;
}

.plan-confirm-btn:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 2px;
}

.plan-confirm-btn--cancel {
  color: var(--text-primary);
  border-color: var(--tool-section-border);
  background: var(--tool-section-bg);
}

.plan-confirm-btn--cancel:hover {
  border-color: var(--tool-error-border);
  background: var(--tool-error-bg);
}

.plan-confirm-btn--execute {
  color: var(--tool-success-text);
  border-color: var(--tool-success-border);
  background: var(--tool-success-bg);
}

.plan-confirm-btn--execute:hover {
  background: var(--tool-success-bg-hover);
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.22);
}

@media (max-width: 640px) {
  .plan-confirm-inline {
    align-items: stretch;
    flex-direction: column;
  }

  .plan-confirm-actions {
    justify-content: flex-end;
  }
}
</style>
