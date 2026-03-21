<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

interface LanguageOption {
  value: string
  label: string
}

const props = withDefaults(
  defineProps<{
    modelValue: string
    options: LanguageOption[]
    ariaLabel?: string
    disabled?: boolean
    placement?: 'left' | 'right'
    shadowMode?: 'normal' | 'none'
  }>(),
  {
    ariaLabel: 'Select language',
    disabled: false,
    placement: 'right',
    shadowMode: 'normal',
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const menuOpen = ref(false)
const switcherRef = ref<HTMLElement | null>(null)

function getMenuItems() {
  return Array.from(
    switcherRef.value?.querySelectorAll<HTMLButtonElement>('[data-language-item="true"]') || [],
  )
}

const selectedLabel = computed(() => {
  const selected = props.options.find((item) => item.value === props.modelValue)
  return selected?.label || props.options[0]?.label || ''
})

const menuAlignClass = computed(() =>
  props.placement === 'left' ? 'language-switcher-menu--left' : 'language-switcher-menu--right',
)

const switcherClass = computed(() => ({
  'language-switcher--no-shadow': props.shadowMode === 'none',
}))

async function openMenu(focusFirstItem = false) {
  if (menuOpen.value || props.disabled) return
  menuOpen.value = true
  await nextTick()
  if (focusFirstItem) getMenuItems()[0]?.focus()
}

function closeMenu() {
  menuOpen.value = false
}

function toggleMenu() {
  if (props.disabled) return
  if (menuOpen.value) {
    closeMenu()
    return
  }
  void openMenu()
}

function onSelect(value: string) {
  closeMenu()
  if (value === props.modelValue) return
  emit('update:modelValue', value)
}

function handleTriggerKeydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown' || event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    void openMenu(true)
    return
  }
  if (event.key === 'Escape') {
    closeMenu()
  }
}

function handleMenuKeydown(event: KeyboardEvent) {
  const items = getMenuItems()
  if (!items.length) return
  const activeIndex = items.findIndex((item) => item === document.activeElement)

  if (event.key === 'ArrowDown') {
    event.preventDefault()
    const nextIndex = activeIndex < 0 ? 0 : (activeIndex + 1) % items.length
    items[nextIndex]?.focus()
    return
  }
  if (event.key === 'ArrowUp') {
    event.preventDefault()
    const nextIndex = activeIndex <= 0 ? items.length - 1 : activeIndex - 1
    items[nextIndex]?.focus()
    return
  }
  if (event.key === 'Home') {
    event.preventDefault()
    items[0]?.focus()
    return
  }
  if (event.key === 'End') {
    event.preventDefault()
    items[items.length - 1]?.focus()
    return
  }
  if (event.key === 'Escape' || event.key === 'Tab') {
    closeMenu()
  }
}

function handleDocumentPointerDown(event: PointerEvent) {
  if (!menuOpen.value) return
  const target = event.target as Node | null
  if (!target) return
  if (!switcherRef.value?.contains(target)) closeMenu()
}

watch(
  () => props.disabled,
  (disabled) => {
    if (disabled) closeMenu()
  },
)

onMounted(() => {
  document.addEventListener('pointerdown', handleDocumentPointerDown)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', handleDocumentPointerDown)
})
</script>

<template>
  <div ref="switcherRef" class="language-switcher" :class="switcherClass">
    <button
      type="button"
      class="language-switcher-trigger inline-flex items-center gap-2 rounded-xl px-3 py-2 text-xs font-medium cursor-pointer"
      :aria-label="ariaLabel"
      aria-haspopup="menu"
      :aria-expanded="menuOpen ? 'true' : 'false'"
      :disabled="disabled"
      @click="toggleMenu"
      @keydown="handleTriggerKeydown"
    >
      <svg class="language-switcher-globe" viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-width="1.8" />
        <path
          d="M3 12h18M12 3c2.5 2.4 3.9 5.6 3.9 9s-1.4 6.6-3.9 9M12 3c-2.5 2.4-3.9 5.6-3.9 9s1.4 6.6 3.9 9"
          stroke="currentColor"
          stroke-width="1.8"
          stroke-linecap="round"
        />
      </svg>
      <span>{{ selectedLabel }}</span>
      <svg
        class="language-switcher-caret"
        :class="{ 'language-switcher-caret-open': menuOpen }"
        viewBox="0 0 20 20"
        fill="none"
        aria-hidden="true"
      >
        <path d="M5 7.5L10 12.5L15 7.5" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </button>

    <Transition name="language-switcher-menu-pop">
      <div
        v-if="menuOpen"
        class="language-switcher-menu rounded-xl p-1"
        :class="menuAlignClass"
        role="menu"
        :aria-label="ariaLabel"
        @keydown="handleMenuKeydown"
      >
        <button
          v-for="option in options"
          :key="option.value"
          type="button"
          class="language-switcher-item w-full text-left rounded-lg px-3 py-2 text-xs cursor-pointer"
          data-language-item="true"
          role="menuitemradio"
          :aria-checked="option.value === modelValue ? 'true' : 'false'"
          @click="onSelect(option.value)"
        >
          <span>{{ option.label }}</span>
          <span v-if="option.value === modelValue" class="language-switcher-check" aria-hidden="true">✓</span>
        </button>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.language-switcher {
  position: relative;
  display: inline-block;
}

.language-switcher-trigger {
  min-height: 38px;
  color: var(--text-primary);
  border: 1px solid var(--input-border);
  background: var(--input-bg);
  box-shadow: 0 10px 26px rgba(15, 23, 42, 0.12);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  transition: border-color 0.18s ease, box-shadow 0.18s ease, background-color 0.18s ease, color 0.18s ease;
}

.language-switcher-trigger:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.language-switcher-trigger:not(:disabled):hover {
  border-color: var(--primary-alpha-38);
  box-shadow: 0 12px 30px var(--primary-alpha-18);
  background: color-mix(in srgb, var(--input-bg) 88%, var(--sb-brand) 12%);
}

.language-switcher-trigger:focus-visible {
  outline: none;
  border-color: var(--primary-alpha-62);
  box-shadow: var(--focus-ring-shadow-strong);
}

.language-switcher--no-shadow .language-switcher-trigger,
.language-switcher--no-shadow .language-switcher-trigger:not(:disabled):hover {
  box-shadow: none;
}

.language-switcher-globe {
  width: 14px;
  height: 14px;
}

.language-switcher-caret {
  width: 13px;
  height: 13px;
  transition: transform 0.18s ease;
}

.language-switcher-caret-open {
  transform: rotate(180deg);
}

.language-switcher-menu {
  position: absolute;
  top: calc(100% + 8px);
  margin-top: 0;
  min-width: 124px;
  border: 1px solid var(--menu-border);
  background: var(--menu-bg);
  box-shadow: var(--menu-shadow);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  z-index: 20;
}

.language-switcher-menu--right {
  right: 0;
}

.language-switcher-menu--left {
  left: 0;
}

.language-switcher-menu-pop-enter-active,
.language-switcher-menu-pop-leave-active {
  transition: opacity 0.16s ease, transform 0.16s ease;
}

.language-switcher-menu-pop-enter-from,
.language-switcher-menu-pop-leave-to {
  opacity: 0;
}

.language-switcher-menu-pop-enter-from {
  transform: translateY(-4px) scale(0.98);
}

.language-switcher-menu-pop-leave-to {
  transform: translateY(-2px) scale(0.985);
}

.language-switcher-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: var(--text-primary);
  transition: background-color 0.15s ease, color 0.15s ease;
}

.language-switcher-item:hover {
  background: var(--primary-alpha-14);
  color: var(--sb-brand-hover);
}

.language-switcher-item:focus-visible {
  outline: none;
  background: var(--primary-alpha-18);
  color: var(--sb-brand-hover);
}

.language-switcher-check {
  color: var(--sb-brand);
  font-weight: 700;
}

@media (prefers-reduced-motion: reduce) {
  .language-switcher-trigger,
  .language-switcher-caret,
  .language-switcher-item {
    transition: none !important;
  }

  .language-switcher-menu-pop-enter-active,
  .language-switcher-menu-pop-leave-active {
    transition: none !important;
  }

  .language-switcher-menu-pop-enter-from,
  .language-switcher-menu-pop-leave-to {
    transform: none !important;
    opacity: 1 !important;
  }
}
</style>
