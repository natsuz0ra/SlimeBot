<script setup lang="ts">
import { useAttrs } from 'vue'

defineOptions({
  inheritAttrs: false,
})

withDefaults(defineProps<{
  modelValue: string
  type?: string
  placeholder?: string
  autocomplete?: string
  disabled?: boolean
  name?: string
  id?: string
}>(), {
  type: 'text',
  placeholder: '',
  autocomplete: undefined,
  disabled: false,
  name: undefined,
  id: undefined,
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const attrs = useAttrs()

function onInput(event: Event) {
  const target = event.target as HTMLInputElement
  emit('update:modelValue', target.value)
}
</script>

<template>
  <input
    v-bind="attrs"
    :type="type"
    :value="modelValue"
    :placeholder="placeholder"
    :autocomplete="autocomplete"
    :disabled="disabled"
    :name="name"
    :id="id"
    class="app-input w-full rounded-xl px-3.5 py-2.5 text-sm outline-none"
    @input="onInput"
  />
</template>

<style scoped>
.app-input {
  background: var(--input-bg);
  border: 1px solid var(--input-border);
  color: var(--text-primary);
  transition: border-color 0.2s ease, box-shadow 0.2s ease, transform 0.2s ease;
}

.app-input:focus {
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.12);
}

.app-input::placeholder {
  color: var(--text-muted);
}
</style>
