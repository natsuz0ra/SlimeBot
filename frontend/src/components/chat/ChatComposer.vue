<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { mdiChevronLeft, mdiChevronRight, mdiClose, mdiPaperclip, mdiSend, mdiStop, mdiTuneVariant } from '@mdi/js'
import { useI18n } from 'vue-i18n'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import TruncationTooltip from '@/components/ui/TruncationTooltip.vue'
import type { SelectOption } from '@/components/ui/AppSelect.vue'
import { formatSize } from '@/utils/format'

const props = defineProps<{
  modelValue: string
  selectedModelId: string
  modelSelectOptions: SelectOption[]
  selectedThinkingLevel: string
  thinkingSelectOptions: SelectOption[]
  selectedSubagentModelId: string
  subagentModelSelectOptions: SelectOption[]
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
  subagentModelChange: [modelId: string]
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

// --- Settings menu ---
const menuOpen = ref(false)
const submenuKey = ref<string | null>(null)
const menuTriggerRef = ref<HTMLElement | null>(null)
const menuPanelRef = ref<HTMLElement | null>(null)
const menuStyle = ref<Record<string, string>>({})

const currentModelLabel = computed(() => {
  const found = props.modelSelectOptions.find((o) => o.value === props.selectedModelId)
  return found?.label || props.selectedModelId
})

const currentThinkingLabel = computed(() => {
  const found = props.thinkingSelectOptions.find((o) => o.value === props.selectedThinkingLevel)
  return found?.label || props.selectedThinkingLevel
})

const currentSubagentModelLabel = computed(() => {
  const found = props.subagentModelSelectOptions.find((o) => o.value === props.selectedSubagentModelId)
  return found?.label || props.selectedSubagentModelId
})

function calcMenuStyle() {
  if (!menuTriggerRef.value) return
  const rect = menuTriggerRef.value.getBoundingClientRect()
  menuStyle.value = {
    position: 'fixed',
    bottom: `${window.innerHeight - rect.top + 6}px`,
    left: `${rect.left}px`,
  }
}

function toggleMenu() {
  if (!menuOpen.value) {
    calcMenuStyle()
    submenuKey.value = null
  }
  menuOpen.value = !menuOpen.value
}

function closeMenu() {
  menuOpen.value = false
  submenuKey.value = null
}

function onSelectModel(value: string) {
  emit('modelChange', value)
  closeMenu()
}

function onSelectThinking(value: string) {
  emit('thinkingChange', value)
  closeMenu()
}

function onSelectSubagentModel(value: string) {
  emit('subagentModelChange', value)
  closeMenu()
}

function onGlobalMenuClick(e: MouseEvent) {
  const target = e.target as Node
  if (
    menuTriggerRef.value && !menuTriggerRef.value.contains(target) &&
    menuPanelRef.value && !menuPanelRef.value.contains(target)
  ) {
    closeMenu()
  }
}

function onMenuKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    if (submenuKey.value) {
      submenuKey.value = null
    } else if (menuOpen.value) {
      closeMenu()
    }
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

function onScrollCapture(e: Event) {
  const target = e.target as Node
  if (menuPanelRef.value && menuPanelRef.value.contains(target)) return
  closeMenu()
}

onMounted(() => {
  document.addEventListener('click', onGlobalMenuClick, true)
  document.addEventListener('keydown', onMenuKeydown)
  window.addEventListener('resize', closeMenu)
  window.addEventListener('scroll', onScrollCapture, true)
})

onUnmounted(() => {
  document.removeEventListener('click', onGlobalMenuClick, true)
  document.removeEventListener('keydown', onMenuKeydown)
  window.removeEventListener('resize', closeMenu)
  window.removeEventListener('scroll', onScrollCapture, true)
})
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
      <div class="flex items-center gap-1.5">
        <button
          ref="menuTriggerRef"
          type="button"
          class="composer-settings-btn w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer"
          @click.stop="toggleMenu"
        >
          <MdiIcon :path="mdiTuneVariant" :size="14" />
        </button>
        <button
          v-if="planMode"
          type="button"
          class="composer-plan-chip group/plan-chip"
          @click="emit('planToggle')"
        >
          <span class="composer-plan-chip-label">{{ t('planModeLabel') }}</span>
          <MdiIcon :path="mdiClose" :size="12" class="composer-plan-chip-icon" />
        </button>
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

  <!-- Settings menu panel -->
  <Teleport to="body">
    <Transition name="composer-menu">
      <div
        v-if="menuOpen"
        ref="menuPanelRef"
        :style="menuStyle"
        class="composer-settings-panel z-[9999] rounded-xl overflow-hidden"
        @click.stop
      >
        <!-- Submenu: model -->
        <Transition name="composer-submenu">
          <div v-if="submenuKey === 'model'" class="composer-submenu sb-scrollbar">
            <button type="button" class="composer-submenu-header" @click="submenuKey = null">
              <MdiIcon :path="mdiChevronLeft" :size="16" />
              <span>{{ t('model') }}</span>
            </button>
            <button
              v-for="opt in modelSelectOptions"
              :key="opt.value"
              type="button"
              class="composer-submenu-option"
              :class="opt.value === selectedModelId ? 'composer-submenu-option-active' : ''"
              @click="onSelectModel(opt.value)"
            >
              <span
                class="w-1.5 h-1.5 rounded-full flex-shrink-0"
                :class="opt.value === selectedModelId ? 'bg-indigo-500' : 'bg-transparent'"
              />
              {{ opt.label }}
            </button>
          </div>
        </Transition>

        <!-- Submenu: subagent model -->
        <Transition name="composer-submenu">
          <div v-if="submenuKey === 'subagent'" class="composer-submenu sb-scrollbar">
            <button type="button" class="composer-submenu-header" @click="submenuKey = null">
              <MdiIcon :path="mdiChevronLeft" :size="16" />
              <span>{{ t('subagentModelLabel') }}</span>
            </button>
            <button
              v-for="opt in subagentModelSelectOptions"
              :key="opt.value"
              type="button"
              class="composer-submenu-option"
              :class="opt.value === selectedSubagentModelId ? 'composer-submenu-option-active' : ''"
              @click="onSelectSubagentModel(opt.value)"
            >
              <span
                class="w-1.5 h-1.5 rounded-full flex-shrink-0"
                :class="opt.value === selectedSubagentModelId ? 'bg-indigo-500' : 'bg-transparent'"
              />
              {{ opt.label }}
            </button>
          </div>
        </Transition>

        <!-- Submenu: thinking -->
        <Transition name="composer-submenu">
          <div v-if="submenuKey === 'thinking'" class="composer-submenu sb-scrollbar">
            <button type="button" class="composer-submenu-header" @click="submenuKey = null">
              <MdiIcon :path="mdiChevronLeft" :size="16" />
              <span>{{ t('thinkingStrength') }}</span>
            </button>
            <button
              v-for="opt in thinkingSelectOptions"
              :key="opt.value"
              type="button"
              class="composer-submenu-option"
              :class="opt.value === selectedThinkingLevel ? 'composer-submenu-option-active' : ''"
              @click="onSelectThinking(opt.value)"
            >
              <span
                class="w-1.5 h-1.5 rounded-full flex-shrink-0"
                :class="opt.value === selectedThinkingLevel ? 'bg-indigo-500' : 'bg-transparent'"
              />
              {{ opt.label }}
            </button>
          </div>
        </Transition>

        <!-- Main menu body -->
        <div class="composer-menu-body" :class="submenuKey ? 'invisible' : ''">
          <button
            type="button"
            class="composer-menu-item"
            :disabled="modelOptionsCount === 0"
            @click="submenuKey = 'model'"
          >
            <div>
              <div class="composer-menu-item-label">{{ t('model') }}</div>
              <div class="composer-menu-item-value">{{ currentModelLabel }}</div>
            </div>
            <MdiIcon :path="mdiChevronRight" :size="14" class="sb-text-muted" />
          </button>
          <div class="composer-menu-divider" />
          <button
            type="button"
            class="composer-menu-item"
            :disabled="modelOptionsCount === 0"
            @click="submenuKey = 'subagent'"
          >
            <div>
              <div class="composer-menu-item-label">{{ t('subagentModelLabel') }}</div>
              <div class="composer-menu-item-value">{{ currentSubagentModelLabel }}</div>
            </div>
            <MdiIcon :path="mdiChevronRight" :size="14" class="sb-text-muted" />
          </button>
          <div class="composer-menu-divider" />
          <button type="button" class="composer-menu-item" @click="submenuKey = 'thinking'">
            <div>
              <div class="composer-menu-item-label">{{ t('thinkingStrength') }}</div>
              <div class="composer-menu-item-value">{{ currentThinkingLabel }}</div>
            </div>
            <MdiIcon :path="mdiChevronRight" :size="14" class="sb-text-muted" />
          </button>
          <div class="composer-menu-divider" />
          <button type="button" class="composer-menu-item" @click="emit('planToggle')">
            <div>
              <div class="composer-menu-item-label">{{ t('planModeLabel') }}</div>
            </div>
            <ToggleSwitch
              :model-value="planMode"
              @click.stop
              @update:model-value="emit('planToggle')"
            />
          </button>
        </div>
      </div>
    </Transition>
  </Teleport>
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

/* --- Settings menu --- */

.composer-settings-btn {
  color: var(--text-secondary);
}
.composer-settings-btn:hover {
  background: var(--primary-alpha-08);
  color: var(--text-primary);
}

.composer-plan-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 24px;
  padding: 0 8px;
  border-radius: 6px;
  border: 1px solid rgba(251, 191, 36, 0.22);
  background: linear-gradient(90deg, rgba(251, 191, 36, 0.1), rgba(251, 191, 36, 0.04));
  cursor: pointer;
  transition: background 150ms ease, border-color 150ms ease;
}

.composer-plan-chip-label {
  font-size: 11px;
  font-weight: 600;
  line-height: 1;
  color: #d97706;
  transition: opacity 120ms ease;
}

.composer-plan-chip-icon {
  color: #d97706;
  position: absolute;
  opacity: 0;
  transition: opacity 120ms ease;
}

.composer-plan-chip:hover {
  background: rgba(251, 191, 36, 0.22);
  border-color: rgba(245, 158, 11, 0.4);
}

.composer-plan-chip:hover .composer-plan-chip-label {
  opacity: 0;
}

.composer-plan-chip:hover .composer-plan-chip-icon {
  opacity: 1;
}

:root:not(.dark) .composer-plan-chip {
  border-color: rgba(251, 191, 36, 0.28);
}

.dark .composer-plan-chip {
  border-color: rgba(253, 224, 71, 0.24);
  background: linear-gradient(90deg, rgba(253, 224, 71, 0.14), rgba(253, 224, 71, 0.06));
}

.dark .composer-plan-chip:hover {
  background: rgba(253, 224, 71, 0.26);
  border-color: rgba(253, 224, 71, 0.4);
}

.dark .composer-plan-chip-label,
.dark .composer-plan-chip-icon {
  color: #fcd34d;
}

.composer-settings-panel {
  width: 220px;
  background: var(--menu-bg);
  border: 1px solid var(--menu-border);
  box-shadow: var(--menu-shadow);
  backdrop-filter: blur(16px);
}

.composer-menu-body {
  padding: 4px 0;
}

.composer-menu-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 8px 12px;
  text-align: left;
  cursor: pointer;
  transition: background 100ms ease;
  border: none;
  background: transparent;
  color: inherit;
  font: inherit;
}
.composer-menu-item:hover {
  background: var(--primary-alpha-07);
}
.composer-menu-item:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.composer-menu-item-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.composer-menu-item-value {
  font-size: 11px;
  color: var(--text-muted);
  margin-top: 1px;
}

.composer-menu-divider {
  height: 1px;
  margin: 2px 12px;
  background: var(--menu-border);
}

.composer-submenu {
  position: absolute;
  inset: 0;
  background: var(--menu-bg);
  border-radius: inherit;
  padding: 4px 0;
  overflow-y: auto;
  z-index: 1;
}

.composer-submenu-header {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 8px 12px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  cursor: pointer;
  border: none;
  background: transparent;
}
.composer-submenu-header:hover {
  background: var(--primary-alpha-07);
}

.composer-submenu-option {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  text-align: left;
  padding: 6px 12px;
  font-size: 13px;
  cursor: pointer;
  border: none;
  background: transparent;
  color: var(--text-secondary);
  transition: background 100ms ease;
}
.composer-submenu-option:hover {
  background: var(--primary-alpha-07);
  color: var(--text-primary);
}

.composer-submenu-option-active {
  background: var(--primary-alpha-10);
  color: var(--sb-brand);
  font-weight: 500;
}

/* --- Menu transitions --- */

.composer-menu-enter-active,
.composer-menu-leave-active {
  transition: opacity 130ms ease-out, transform 130ms cubic-bezier(0.16, 1, 0.3, 1);
}
.composer-menu-enter-from,
.composer-menu-leave-to {
  opacity: 0;
  transform: translateY(4px) scale(0.97);
}

.composer-submenu-enter-active,
.composer-submenu-leave-active {
  transition: transform 160ms cubic-bezier(0.16, 1, 0.3, 1), opacity 120ms ease-out;
}
.composer-submenu-enter-from,
.composer-submenu-leave-to {
  transform: translateX(30px);
  opacity: 0;
}
</style>
