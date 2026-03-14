<script setup lang="ts">
import { computed, ref, useAttrs } from 'vue'
import { mdiEyeOffOutline, mdiEyeOutline } from '@mdi/js'
import MdiIcon from '@/components/MdiIcon.vue'

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
  showLabel: '显示密码',
  hideLabel: '隐藏密码',
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const attrs = useAttrs()
const showPassword = ref(false)

const inputType = computed(() => (showPassword.value ? 'text' : 'password'))
const toggleLabel = computed(() => (showPassword.value ? props.hideLabel : props.showLabel))
const toggleIcon = computed(() => (showPassword.value ? mdiEyeOffOutline : mdiEyeOutline))
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
      <MdiIcon :path="toggleIcon" :size="18" />
    </button>
  </div>
</template>

<style scoped>
.app-password-wrap {
  position: relative;
}

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
  background: rgba(99, 102, 241, 0.1);
}

.password-toggle-btn:focus-visible {
  outline: 2px solid rgba(99, 102, 241, 0.5);
  outline-offset: 1px;
}

.password-toggle-btn:active {
  transform: translateY(-50%) scale(0.97);
}
</style>
