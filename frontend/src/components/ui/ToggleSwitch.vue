<script setup lang="ts">
const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

function onClick() {
  emit('update:modelValue', !props.modelValue)
}
</script>

<template>
  <button
    type="button"
    class="relative w-9 h-5 rounded-full transition-all duration-200 cursor-pointer flex-shrink-0 toggle-switch"
    :class="modelValue ? 'toggle-switch-on' : 'toggle-switch-off'"
    @click="onClick"
  >
    <span
      class="absolute top-0.5 left-0.5 w-4 h-4 bg-white rounded-full shadow-sm transition-transform duration-200"
      :class="modelValue ? 'translate-x-4' : 'translate-x-0'"
    />
  </button>
</template>

<style scoped>
button.toggle-switch.toggle-switch-on {
  background: var(--sb-brand);
  box-shadow: 0 0 0 2px var(--primary-alpha-20);
}
button.toggle-switch.toggle-switch-off {
  background: var(--primary-alpha-15);
}
</style>
