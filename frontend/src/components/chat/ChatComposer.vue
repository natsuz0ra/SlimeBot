<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { mdiClose, mdiPaperclip, mdiSend, mdiStop } from '@mdi/js'
import { useI18n } from 'vue-i18n'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import AppSelect, { type SelectOption } from '@/components/ui/AppSelect.vue'
import { formatSize } from '@/utils/format'

const props = defineProps<{
  modelValue: string
  selectedModelId: string
  modelSelectOptions: SelectOption[]
  modelOptionsCount: number
  sendDisabled: boolean
  stopDisabled: boolean
  isStreaming: boolean
  pendingFiles: File[]
  placeholder: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  send: []
  stop: []
  filesChange: [files: File[]]
  removeFile: [index: number]
  modelChange: [modelId: string]
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
  <div class="input-container focus-ring rounded-2xl overflow-hidden">
    <div v-if="pendingFiles.length > 0" class="px-3 pt-3 pb-1 flex flex-wrap gap-2">
      <div
        v-for="(file, idx) in pendingFiles"
        :key="`${file.name}-${idx}`"
        class="chat-upload-chip inline-flex items-center gap-2 rounded-lg px-2 py-1 max-w-[260px]"
      >
        <span class="truncate text-xs">{{ file.name }}</span>
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
    <div class="absolute bottom-2 left-3 right-3 flex items-center justify-between gap-2">
      <AppSelect
        :model-value="selectedModelId"
        :options="modelSelectOptions"
        :disabled="modelOptionsCount === 0"
        variant="ghost"
        size="xs"
        @update:model-value="emit('modelChange', $event)"
      />
      <div class="flex items-center gap-2">
        <div class="relative group/upload-tip">
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
            class="pointer-events-none absolute bottom-full right-0 mb-2 w-[220px] rounded-lg px-3 py-2 text-[11px] leading-4 text-white bg-black/78 opacity-0 translate-y-1 transition-all duration-150 shadow-lg group-hover/upload-tip:opacity-100 group-hover/upload-tip:translate-y-0 group-focus-within/upload-tip:opacity-100 group-focus-within/upload-tip:translate-y-0"
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
  </div>
</template>
