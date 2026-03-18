<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { mdiSend } from '@mdi/js'
import MdiIcon from '@/components/MdiIcon.vue'
import AppSelect, { type SelectOption } from '@/components/ui/AppSelect.vue'

const props = defineProps<{
  modelValue: string
  selectedModelId: string
  modelSelectOptions: SelectOption[]
  modelOptionsCount: number
  sendDisabled: boolean
  placeholder: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  send: []
  modelChange: [modelId: string]
}>()

const textareaRef = ref<HTMLTextAreaElement | null>(null)

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
    emit('send')
  }
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
  <div class="input-container rounded-2xl overflow-hidden">
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
      <button
        type="button"
        class="w-8 h-8 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer flex-shrink-0"
        :class="sendDisabled ? 'send-btn-disabled' : 'send-btn'"
        :disabled="sendDisabled"
        @click="emit('send')"
      >
        <MdiIcon :path="mdiSend" :size="15" />
      </button>
    </div>
  </div>
</template>
