<script setup lang="ts">
import { computed, ref, useAttrs } from 'vue'
import { useI18n } from 'vue-i18n'
import { Eye, EyeOff } from 'lucide-vue-next'

const { t } = useI18n()

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(defineProps<{
  modelValue: string
  placeholder?: string
  autocomplete?: string
  disabled?: boolean
  name?: string
  id?: string
  showLabel?: string
  hideLabel?: string
}>(), {
  placeholder: '',
  autocomplete: undefined,
  disabled: false,
  name: undefined,
  id: undefined,
  showLabel: undefined,
  hideLabel: undefined,
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const attrs = useAttrs()
const showPassword = ref(false)

const showLabelResolved = computed(() => props.showLabel ?? t('showPassword'))
const hideLabelResolved = computed(() => props.hideLabel ?? t('hidePassword'))

const inputType = computed(() => (showPassword.value ? 'text' : 'password'))
const toggleLabel = computed(() => (showPassword.value ? hideLabelResolved.value : showLabelResolved.value))
const toggleIcon = computed(() => (showPassword.value ? EyeOff : Eye))
const inputAttrs = computed(() => {
  const { class: _class, ...rest } = attrs
  return rest
})

function onInput(event: Event) {
  const target = event.target as HTMLInputElement
  emit('update:modelValue', target.value)
}

function togglePasswordVisibility() {
  showPassword.value = !showPassword.value
}
</script>

<template>
  <div class="app-password-wrap">
    <input
      v-bind="inputAttrs"
      :type="inputType"
      :value="modelValue"
      :placeholder="placeholder"
      :autocomplete="autocomplete"
      :disabled="disabled"
      :name="name"
      :id="id"
      :class="['app-input app-password-input w-full rounded-xl px-3.5 py-2.5 text-sm outline-none', attrs.class]"
      @input="onInput"
    />
    <button
      type="button"
      class="password-toggle-btn"
      :aria-label="toggleLabel"
      :title="toggleLabel"
      @click="togglePasswordVisibility"
    >
      <component :is="toggleIcon" :size="18" :stroke-width="1.75" aria-hidden="true" />
    </button>
  </div>
</template>

<style scoped>
.app-password-wrap {
  position: relative;
}

input.app-input {
  background: var(--input-bg);
  border: 1px solid var(--input-border);
  color: var(--text-primary);
  transition: border-color 0.2s ease, box-shadow 0.2s ease, transform 0.2s ease;
}

input.app-input:focus {
  border-color: var(--sb-brand);
  box-shadow: var(--focus-ring-shadow);
}

input.app-input::placeholder {
  color: var(--text-muted);
}

.app-password-input {
  padding-right: 44px;
}

.password-toggle-btn {
  position: absolute;
  right: 10px;
  top: 50%;
  transform: translateY(-50%);
  border: none;
  background: transparent;
  color: var(--text-muted);
  line-height: 0;
  padding: 4px;
  border-radius: 8px;
  cursor: pointer;
  transition: color 0.2s ease, background-color 0.2s ease, transform 0.2s ease;
}

.password-toggle-btn:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.password-toggle-btn:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 1px;
}

.password-toggle-btn:active {
  transform: translateY(-50%) scale(0.97);
}
</style>
