<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { mdiClose } from '@mdi/js'
import MdiIcon from '@/components/MdiIcon.vue'

const props = withDefaults(defineProps<{
  visible: boolean
  title?: string
  confirmText?: string
  cancelText?: string
  confirmLoading?: boolean
  confirmDanger?: boolean
  width?: string
  hideFooter?: boolean
}>(), {
  confirmText: '确认',
  cancelText: '取消',
  confirmLoading: false,
  confirmDanger: false,
  width: '480px',
  hideFooter: false,
})

const emit = defineEmits<{
  'update:visible': [value: boolean]
  confirm: []
  cancel: []
}>()

function close() {
  emit('update:visible', false)
  emit('cancel')
}

function onConfirm() {
  emit('confirm')
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.visible) close()
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
        style="background: rgba(0,0,0,0.45); backdrop-filter: blur(4px)"
        @click.self="close"
      >
        <div
          class="dialog-panel relative flex flex-col overflow-hidden rounded-2xl"
          :style="{ width: '100%', maxWidth: width, maxHeight: '90vh' }"
          @click.stop
        >
          <!-- 头部 -->
          <div class="flex items-center justify-between px-5 py-4 dialog-header">
            <span class="text-sm font-semibold dialog-title">{{ title }}</span>
            <button
              type="button"
              class="w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer dialog-close-btn"
              @click="close"
            >
              <MdiIcon :path="mdiClose" :size="15" />
            </button>
          </div>

          <!-- 内容 -->
          <div class="flex-1 overflow-y-auto px-5 py-4">
            <slot />
          </div>

          <!-- 底部 -->
          <div v-if="!hideFooter" class="flex items-center justify-end gap-2 px-5 py-4 dialog-footer">
            <button
              type="button"
              class="px-4 py-2 text-sm rounded-xl transition-all duration-150 cursor-pointer dialog-cancel-btn"
              @click="close"
            >
              {{ cancelText }}
            </button>
            <button
              type="button"
              class="px-4 py-2 text-sm rounded-xl text-white transition-all duration-150 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1.5"
              :class="confirmDanger ? 'dialog-confirm-danger' : 'dialog-confirm-primary'"
              :disabled="confirmLoading"
              @click="onConfirm"
            >
              <svg v-if="confirmLoading" class="animate-spin w-3.5 h-3.5" viewBox="0 0 24 24" fill="none">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
              </svg>
              {{ confirmText }}
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
  box-shadow: 0 25px 60px rgba(0, 0, 0, 0.35), 0 0 0 1px rgba(99, 102, 241, 0.08);
}

.dialog-header {
  border-bottom: 1px solid var(--card-border);
}

.dialog-title {
  color: var(--text-primary);
}

.dialog-close-btn {
  color: var(--text-muted);
}
.dialog-close-btn:hover {
  background: rgba(99, 102, 241, 0.08);
  color: var(--text-primary);
}

.dialog-footer {
  border-top: 1px solid var(--card-border);
}

.dialog-cancel-btn {
  background: var(--input-bg);
  border: 1px solid var(--input-border);
  color: var(--text-secondary);
}
.dialog-cancel-btn:hover {
  background: rgba(99, 102, 241, 0.08);
}

.dialog-confirm-primary {
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.35);
}
.dialog-confirm-primary:hover:not(:disabled) {
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.45);
  transform: translateY(-1px);
}

.dialog-confirm-danger {
  background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
  box-shadow: 0 2px 8px rgba(239, 68, 68, 0.3);
}
.dialog-confirm-danger:hover:not(:disabled) {
  box-shadow: 0 4px 12px rgba(239, 68, 68, 0.4);
  transform: translateY(-1px);
}

/* Transition */
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
