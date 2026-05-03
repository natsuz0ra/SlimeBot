<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { mdiClose } from '@mdi/js'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import { isMaskSelfEvent, shouldCloseOnMaskInteraction } from '@/utils/dialogMask'

const { t } = useI18n()

const props = withDefaults(
  defineProps<{
    visible: boolean
    title?: string
    confirmText?: string
    cancelText?: string
    confirmLoading?: boolean
    confirmDanger?: boolean
    width?: string
    hideFooter?: boolean
    showClose?: boolean
    showCancel?: boolean
    closeOnMask?: boolean
    closeOnEsc?: boolean
    compact?: boolean
    largeTitle?: boolean
  }>(),
  {
    confirmText: undefined,
    cancelText: undefined,
    confirmLoading: false,
    confirmDanger: false,
    width: '480px',
    hideFooter: false,
    showClose: true,
    showCancel: true,
    closeOnMask: true,
    closeOnEsc: true,
    compact: false,
    largeTitle: false,
  },
)

const resolvedConfirmText = computed(() => props.confirmText ?? t('confirm'))
const resolvedCancelText = computed(() => props.cancelText ?? t('cancel'))

const emit = defineEmits<{
  'update:visible': [value: boolean]
  confirm: []
  cancel: []
}>()

const titleId = `dialog-title-${Math.random().toString(36).slice(2, 10)}`
const pointerDownStartedOnMask = ref(false)

function close() {
  emit('update:visible', false)
  emit('cancel')
}

function onConfirm() {
  emit('confirm')
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.visible && props.closeOnEsc) close()
}

function onMaskPointerDown(e: PointerEvent) {
  pointerDownStartedOnMask.value = isMaskSelfEvent(e)
}

function onMaskClick(e: MouseEvent) {
  if (shouldCloseOnMaskInteraction({
    closeOnMask: props.closeOnMask,
    pointerDownStartedOnMask: pointerDownStartedOnMask.value,
    eventTarget: e.target,
    eventCurrentTarget: e.currentTarget,
  })) {
    close()
  }
  pointerDownStartedOnMask.value = false
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onUnmounted(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <Transition name="dialog-fade">
      <div
        v-if="visible"
        class="fixed inset-0 z-[200] flex items-center justify-center p-4"
        style="background: rgba(0, 0, 0, 0.45); backdrop-filter: blur(4px)"
        @pointerdown="onMaskPointerDown"
        @click="onMaskClick"
      >
        <div
          class="dialog-panel relative flex flex-col overflow-hidden rounded-2xl"
          :style="{ width: '100%', maxWidth: width, maxHeight: '90vh' }"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="title ? titleId : undefined"
          @click.stop
        >
          <div
            class="flex items-center justify-between dialog-header"
            :class="compact ? 'px-4 py-2' : 'px-5 py-3'"
          >
            <span
              :id="titleId"
              class="font-semibold dialog-title"
              :class="compact ? (largeTitle ? 'text-base' : 'text-sm') : 'text-base'"
            >{{ title }}</span>
            <button
              v-if="showClose"
              type="button"
              class="w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer dialog-close-btn"
              @click="close"
            >
              <MdiIcon :path="mdiClose" :size="15" />
            </button>
          </div>

          <div class="flex-1 overflow-y-auto" :class="compact ? 'px-4 py-3' : 'px-5 py-4'">
            <slot />
          </div>

          <div
            v-if="!hideFooter"
            class="flex items-center justify-end gap-2 dialog-footer"
            :class="compact ? 'px-4 py-2' : 'px-5 py-3'"
          >
            <button
              v-if="showCancel"
              type="button"
              class="px-4 py-2 text-sm rounded-xl transition-all duration-150 cursor-pointer dialog-cancel-btn"
              @click="close"
            >
              {{ resolvedCancelText }}
            </button>
            <button
              type="button"
              class="px-4 py-2 text-sm rounded-xl text-white transition-all duration-150 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1.5"
              :class="confirmDanger ? 'dialog-confirm-danger' : 'dialog-confirm-primary'"
              :disabled="confirmLoading"
              @click="onConfirm"
            >
              <LoadingSpinner v-if="confirmLoading" size-class="w-3.5 h-3.5" />
              {{ resolvedConfirmText }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.dialog-panel {
  background: var(--bg-main);
  border: 1px solid var(--card-border);
  box-shadow: 0 25px 60px rgba(0, 0, 0, 0.35), 0 0 0 1px var(--primary-alpha-08);
}

.dialog-title {
  color: var(--text-primary);
}

.dialog-close-btn {
  color: var(--text-muted);
}
.dialog-close-btn:hover {
  background: var(--primary-alpha-08);
  color: var(--text-primary);
}

.dialog-confirm-primary {
  background: var(--sb-brand);
  box-shadow: none;
}
.dialog-confirm-primary:hover:not(:disabled) {
  background: var(--sb-brand-hover);
  box-shadow: none;
  transform: none;
}

.dialog-confirm-danger {
  background: var(--color-danger);
  box-shadow: none;
}
.dialog-confirm-danger:hover:not(:disabled) {
  background: #dc2626;
  box-shadow: none;
  transform: none;
}

.dialog-fade-enter-active,
.dialog-fade-leave-active {
  transition: opacity 180ms ease-out;
}
.dialog-fade-enter-active .dialog-panel,
.dialog-fade-leave-active .dialog-panel {
  transition: transform 200ms cubic-bezier(0.16, 1, 0.3, 1), opacity 180ms ease-out;
}
.dialog-fade-enter-from,
.dialog-fade-leave-to {
  opacity: 0;
}
.dialog-fade-enter-from .dialog-panel,
.dialog-fade-leave-to .dialog-panel {
  transform: scale(0.95) translateY(-8px);
  opacity: 0;
}
</style>
