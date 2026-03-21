<script setup lang="ts">
import { toastState, type ToastItem } from '@/composables/useToast'

function iconPath(type: ToastItem['type']) {
  switch (type) {
    case 'success': return 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z'
    case 'error': return 'M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z'
    case 'warning': return 'M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z'
    default: return 'M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z'
  }
}

function typeClass(type: ToastItem['type']) {
  switch (type) {
    case 'success': return 'toast-success'
    case 'error': return 'toast-error'
    case 'warning': return 'toast-warning'
    default: return 'toast-info'
  }
}

function iconClass(type: ToastItem['type']) {
  switch (type) {
    case 'success': return 'icon-success'
    case 'error': return 'icon-error'
    case 'warning': return 'icon-warning'
    default: return 'icon-info'
  }
}
</script>

<template>
  <Teleport to="body">
    <div class="fixed top-4 right-4 z-[9999] flex flex-col gap-2 pointer-events-none" style="min-width: 280px; max-width: 400px">
      <TransitionGroup name="toast">
        <div
          v-for="item in toastState.items"
          :key="item.id"
          class="toast-item flex items-start gap-3 px-4 py-3.5 pointer-events-auto"
          :class="typeClass(item.type)"
        >
          <div class="toast-icon-wrap flex-shrink-0 w-7 h-7 rounded-lg flex items-center justify-center mt-0.5" :class="iconClass(item.type)">
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" :d="iconPath(item.type)" />
            </svg>
          </div>
          <span class="sb-text-primary text-sm leading-relaxed break-words flex-1 pt-0.5">{{ item.message }}</span>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
.toast-item {
  border-radius: 14px;
  backdrop-filter: blur(16px);
  border: 1px solid var(--card-border);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.2), 0 2px 8px rgba(0, 0, 0, 0.1);
  background: var(--menu-bg);
}

.toast-success {
  border-color: rgba(16, 185, 129, 0.25);
}
.toast-error {
  border-color: rgba(239, 68, 68, 0.25);
}
.toast-warning {
  border-color: rgba(245, 158, 11, 0.25);
}
.toast-info {
  border-color: rgba(99, 102, 241, 0.25);
}

/* Icon wrappers */
.icon-success {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}
.icon-error {
  background: rgba(239, 68, 68, 0.12);
  color: #ef4444;
}
.icon-warning {
  background: rgba(245, 158, 11, 0.12);
  color: #f59e0b;
}
.icon-info {
  background: var(--primary-alpha-12);
  color: var(--sb-brand);
}

/* Transition */
.toast-enter-active {
  transition: all 250ms cubic-bezier(0.16, 1, 0.3, 1);
}
.toast-leave-active {
  transition: all 180ms ease-in;
}
.toast-enter-from {
  opacity: 0;
  transform: translateX(20px) scale(0.96);
}
.toast-leave-to {
  opacity: 0;
  transform: translateX(20px) scale(0.96);
}
</style>
