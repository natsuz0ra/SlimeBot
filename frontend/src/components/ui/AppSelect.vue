<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { mdiChevronDown } from '@mdi/js'
import MdiIcon from '@/components/ui/MdiIcon.vue'

export interface SelectOption {
  value: string
  label: string
}

const props = withDefaults(defineProps<{
  modelValue: string
  options: SelectOption[]
  disabled?: boolean
  placeholder?: string
  variant?: 'default' | 'ghost'
  size?: 'sm' | 'xs'
}>(), {
  disabled: false,
  placeholder: '',
  variant: 'default',
  size: 'sm',
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const open = ref(false)
const triggerRef = ref<HTMLElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const dropdownStyle = ref<Record<string, string>>({})
const goUp = ref(false)

const selectedLabel = () => {
  const found = props.options.find((opt) => opt.value === props.modelValue)
  return found?.label || props.placeholder || props.modelValue
}

function calcDropdownStyle() {
  if (!triggerRef.value) return
  const rect = triggerRef.value.getBoundingClientRect()
  const spaceBelow = window.innerHeight - rect.bottom
  const spaceAbove = rect.top
  const minWidth = `${rect.width}px`

  if (spaceBelow < 180 && spaceAbove > spaceBelow) {
    goUp.value = true
    dropdownStyle.value = {
      position: 'fixed',
      bottom: `${window.innerHeight - rect.top + 4}px`,
      left: `${rect.left}px`,
      minWidth,
    }
  } else {
    goUp.value = false
    dropdownStyle.value = {
      position: 'fixed',
      top: `${rect.bottom + 4}px`,
      left: `${rect.left}px`,
      minWidth,
    }
  }
}

function toggle() {
  if (props.disabled) return
  if (!open.value) {
    calcDropdownStyle()
  }
  open.value = !open.value
}

function select(value: string) {
  emit('update:modelValue', value)
  open.value = false
}

function closeDropdown() {
  open.value = false
}

function onGlobalClick(e: MouseEvent) {
  const target = e.target as Node
  if (
    triggerRef.value && !triggerRef.value.contains(target) &&
    dropdownRef.value && !dropdownRef.value.contains(target)
  ) {
    open.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', onGlobalClick, true)
  window.addEventListener('resize', closeDropdown)
  window.addEventListener('scroll', closeDropdown, true)
})

onUnmounted(() => {
  document.removeEventListener('click', onGlobalClick, true)
  window.removeEventListener('resize', closeDropdown)
  window.removeEventListener('scroll', closeDropdown, true)
})
</script>

<template>
  <div class="relative inline-block">
    <!-- 触发按钮 -->
    <button
      ref="triggerRef"
      type="button"
      class="select-trigger flex items-center gap-1.5 rounded-lg cursor-pointer transition-all duration-150 select-none"
      :class="[
        size === 'xs' ? 'text-xs px-2 py-1' : 'text-sm px-3 py-1.5',
        variant === 'default' ? 'select-trigger-default' : 'select-trigger-ghost',
        disabled ? 'opacity-50 cursor-not-allowed' : '',
        open && variant === 'default' ? 'select-trigger-focused' : '',
      ]"
      @click.stop="toggle"
    >
      <span>{{ selectedLabel() }}</span>
      <MdiIcon
        :path="mdiChevronDown"
        :size="size === 'xs' ? 12 : 14"
        class="sb-text-muted flex-shrink-0 transition-transform duration-200"
        :class="open ? 'rotate-180' : ''"
      />
    </button>

    <!-- 下拉列表 -->
    <Teleport to="body">
      <Transition :name="goUp ? 'select-dropdown-top' : 'select-dropdown'">
        <div
          v-if="open"
          ref="dropdownRef"
          :style="dropdownStyle"
          class="select-panel z-[9999] rounded-xl py-1 overflow-hidden"
          @click.stop
        >
          <button
            v-for="opt in options"
            :key="opt.value"
            type="button"
            class="select-option w-full text-left px-3 py-2 cursor-pointer transition-all duration-100 flex items-center gap-2"
            :class="[
              size === 'xs' ? 'text-xs' : 'text-sm',
              opt.value === modelValue ? 'select-option-active' : 'select-option-default',
            ]"
            @click="select(opt.value)"
          >
            <span
              class="w-1.5 h-1.5 rounded-full flex-shrink-0 transition-all duration-150"
              :class="opt.value === modelValue ? 'bg-indigo-500' : 'bg-transparent'"
            />
            {{ opt.label }}
          </button>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.select-trigger {
  color: var(--text-secondary);
}

.select-trigger-default {
  background: var(--input-bg);
  border: 1px solid var(--input-border);
}
.select-trigger-default:not(:disabled):hover {
  border-color: var(--sb-brand);
  background: var(--primary-alpha-05);
}

.select-trigger-ghost:not(:disabled):hover {
  background: var(--primary-alpha-08);
  color: var(--text-primary);
}

.select-trigger-focused {
  border-color: var(--sb-brand) !important;
  box-shadow: 0 0 0 2px var(--primary-alpha-15);
}

.select-panel {
  background: var(--menu-bg);
  border: 1px solid var(--menu-border);
  box-shadow: var(--menu-shadow);
  backdrop-filter: blur(16px);
}

.select-option-active {
  background: var(--primary-alpha-10);
  color: var(--sb-brand);
  font-weight: 500;
}

.select-option-default {
  color: var(--text-secondary);
}
.select-option-default:hover {
  background: var(--primary-alpha-07);
  color: var(--text-primary);
}

/* Transitions */
.select-dropdown-enter-active,
.select-dropdown-leave-active {
  transition: opacity 130ms ease-out, transform 130ms cubic-bezier(0.16, 1, 0.3, 1);
}
.select-dropdown-enter-from,
.select-dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px) scale(0.97);
}
.select-dropdown-top-enter-active,
.select-dropdown-top-leave-active {
  transition: opacity 130ms ease-out, transform 130ms cubic-bezier(0.16, 1, 0.3, 1);
}
.select-dropdown-top-enter-from,
.select-dropdown-top-leave-to {
  opacity: 0;
  transform: translateY(4px) scale(0.97);
}
</style>
