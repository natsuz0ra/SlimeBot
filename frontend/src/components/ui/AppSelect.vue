<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { mdiChevronDown } from '@mdi/js'
import MdiIcon from '@/components/MdiIcon.vue'

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
  // 使用捕获阶段，避免被父元素的 @click.stop 阻断（如弹窗容器）
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
      class="flex items-center gap-1.5 px-2 py-1 rounded-lg bg-white text-gray-700 cursor-pointer transition-all duration-150 select-none"
      :class="[
        size === 'xs' ? 'text-xs' : 'text-sm',
        variant === 'default' ? 'border border-gray-200 px-3 py-1.5' : 'px-2 py-1',
        disabled ? 'opacity-50 cursor-not-allowed' : 'hover:bg-gray-100',
        open && variant === 'default' ? 'border-blue-400 ring-2 ring-blue-100' : '',
      ]"
      @click.stop="toggle"
    >
      <span>{{ selectedLabel() }}</span>
      <MdiIcon
        :path="mdiChevronDown"
        :size="size === 'xs' ? 12 : 14"
        class="text-gray-400 flex-shrink-0 transition-transform duration-150"
        :class="open ? 'rotate-180' : ''"
      />
    </button>

    <!-- 下拉列表（Teleport 到 body，避免 overflow-hidden 截断） -->
    <Teleport to="body">
      <Transition :name="goUp ? 'select-dropdown-top' : 'select-dropdown'">
        <div
          v-if="open"
          ref="dropdownRef"
          :style="dropdownStyle"
          class="z-[9999] bg-white border border-gray-200 rounded-xl shadow-lg py-1 overflow-hidden"
          @click.stop
        >
          <button
            v-for="opt in options"
            :key="opt.value"
            type="button"
            class="w-full text-left px-3 py-2 cursor-pointer transition-colors duration-100 flex items-center gap-2"
            :class="[
              size === 'xs' ? 'text-xs' : 'text-sm',
              opt.value === modelValue
                ? 'bg-blue-50 text-blue-700 font-medium'
                : 'text-gray-700 hover:bg-gray-50',
            ]"
            @click="select(opt.value)"
          >
            <!-- 选中指示点 -->
            <span
              class="w-1.5 h-1.5 rounded-full flex-shrink-0"
              :class="opt.value === modelValue ? 'bg-blue-500' : 'bg-transparent'"
            />
            {{ opt.label }}
          </button>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.select-dropdown-enter-active,
.select-dropdown-leave-active {
  transition: opacity 120ms ease-out, transform 120ms ease-out;
}
.select-dropdown-enter-from,
.select-dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px) scale(0.98);
}
.select-dropdown-top-enter-active,
.select-dropdown-top-leave-active {
  transition: opacity 120ms ease-out, transform 120ms ease-out;
}
.select-dropdown-top-enter-from,
.select-dropdown-top-leave-to {
  opacity: 0;
  transform: translateY(4px) scale(0.98);
}
</style>
